// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License 2.0.
//
// This product includes software developed at Datadog (https://www.datadoghq.com/)
// Copyright 2026-Present Datadog, Inc.

package fastjq

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	errExpectedObjectField = errors.New("expected object for field access")
	errExpectedArrayIndex  = errors.New("expected array for index access")
	errExpectedIterable    = errors.New("expected array or object for .[]")
	errBreak               = errors.New("stop iteration") // sentinel for first/limit
	errRecurseCycle        = errors.New("recurse() produced its input again")
	errRecurseDepthLimit   = errors.New("recurse() exceeded depth limit")
)

const (
	jsonParseDepthLimit         = 10000
	jsonStringifySkipDepthLimit = jsonParseDepthLimit + 1
	maxArrayUpdateIndex         = 1 << 20
	maxRecurseDepth             = jsonParseDepthLimit
)

// jsonError carries a JSON value thrown by the `error` builtin.
// jq's catch handler receives the actual JSON value, not a string representation.
// This is only allocated on exceptional (error-throwing) code paths.
type jsonError struct {
	payload []byte
}

func (e *jsonError) Error() string { return string(e.payload) }

// downstreamError marks an error returned by the callback chain outside the
// lexical scope of a try body, so opTry can propagate it instead of catching it.
type downstreamError struct {
	err error
}

func (e *downstreamError) Error() string { return e.err.Error() }

// callbackError marks an error returned by a RunFunc callback, so iterators can
// tell it apart from their own evaluation errors, which they drop on purpose.
// Allocated only when a callback fails.
type callbackError struct {
	err error
}

func (e *callbackError) Error() string { return e.err.Error() }

func (e *callbackError) Unwrap() error { return e.err }

// callbackErrorCause returns the caller's error if err carries a callbackError,
// and reports whether it found one. Every opTry the output passed through wraps
// the error again on its way out, so the loop peels all of them: an iterator
// nested in two try bodies sees a doubly wrapped signal.
//
// Type assertions, not errors.As: errors.As takes an interface{}, so its target
// escapes and would allocate on every dropped engine error.
func callbackErrorCause(err error) (error, bool) {
	for {
		switch e := err.(type) {
		case *callbackError:
			return e.err, true
		case *downstreamError:
			err = e.err
		default:
			return nil, false
		}
	}
}

// isCallbackError reports whether err is a caller's stop signal, including one
// opTry has wrapped on its way out.
func isCallbackError(err error) bool {
	_, ok := callbackErrorCause(err)
	return ok
}

type transparentError struct {
	err error
}

func (e *transparentError) Error() string { return e.err.Error() }

type breakSignal struct {
	label string
}

func (e *breakSignal) Error() string { return "break $" + e.label }

// bTrue / bFalse / bNull are package-level literals returned directly when buf == nil,
// avoiding heap allocation for boolean and null results in zero-scratch evaluation paths.
var bTrue = []byte("true")
var bFalse = []byte("false")
var bNull = []byte("null")

func isControlFlowError(err error) bool {
	if err == errBreak {
		return true
	}
	if isCallbackError(err) {
		return true
	}
	var bs *breakSignal
	return errors.As(err, &bs)
}

// execMulti executes an op against input, calling fn for each result.
// Single-output ops call fn once. Iterators call fn per element.
// buf is used as scratch space and may be reused across fn calls.
func execMulti(state execState, node *op, input []byte, buf []byte, fn func([]byte) error) error {
	switch node.typ {
	case opIdentity:
		result, err := execIdentity(input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opField:
		return execFieldMulti(state, node, input, buf, fn)
	case opDelete:
		result, err := execDelete(state, node, input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opPipe:
		return execPipeMulti(state, node, input, buf, fn)
	case opApply:
		return execApplyMulti(state, node, input, buf, fn)
	case opBind:
		baseCtx := state.currentExecContext()
		return execMulti(state, node.left, input, nil, func(bound []byte) error {
			if node.pattern != nil && len(node.altPatterns) > 0 {
				return execBindAlternatives(state, baseCtx, node, input, bound, fn)
			}
			nextCtx, err := bindOpValue(baseCtx, node, bound)
			if err != nil {
				return err
			}
			return state.withExecContext(nextCtx, func(state execState) error {
				return execMulti(state, node.right, input, buf, fn)
			})
		})
	case opLabel:
		err := execMulti(state, node.child, input, buf, fn)
		var bs *breakSignal
		if errors.As(err, &bs) && bs.label == node.name {
			return nil
		}
		return err
	case opBreakOp:
		return &breakSignal{label: node.name}
	case opVar:
		if node.name == "__loc__" {
			value := []byte(`{"file":"<top-level>","line":1}`)
			if node.child != nil {
				return execMulti(state, node.child, value, buf, fn)
			}
			if buf == nil {
				return fn(value[:len(value):len(value)])
			}
			return fn(append(buf, value...))
		}
		ctx := state.currentExecContext()
		value, ok := ctx.lookupVar(node.name)
		if !ok {
			return fmt.Errorf("$%s is not defined", node.name)
		}
		if node.child != nil {
			return execMulti(state, node.child, value, buf, fn)
		}
		if buf == nil {
			return fn(value[:len(value):len(value)])
		}
		return fn(append(buf, value...))
	case opIndex:
		return execIndexMulti(state, node, input, buf, fn)
	case opIndexExpr:
		return execIndexExprMulti(state, node, input, buf, fn)
	case opIterator:
		return execIterator(state, node, input, buf, fn)
	case opRecursiveDescent:
		return execRecursiveDescent(input, buf, fn)
	case opRecurse:
		return execRecurse(state, node, input, buf, fn)
	case opWalk:
		return execWalk(state, node, input, buf, fn)
	case opConstruct:
		if node.multiValuePairs {
			return execConstructMulti(state, node, input, buf, fn)
		}
		result, err := execConstruct(state, node, input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opArrayConstruct:
		result, err := execArrayConstruct(state, node, input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opLiteral:
		return fn(normalizeOutputValue(node.literal, buf))
	case opTypeBuiltin:
		return execType(input, buf, fn)
	case opCompare:
		return execCompare(state, node, input, buf, fn)
	case opAnd:
		return execAnd(state, node, input, buf, fn)
	case opOr:
		return execOr(state, node, input, buf, fn)
	case opNot:
		if isFalsy(input) {
			return fn(append(buf, "true"...))
		}
		return fn(append(buf, "false"...))
	case opOptional:
		if node.child != nil && node.child.typ == opRepeat {
			err := state.withOptionalScope(func(state execState) error {
				return execMulti(state, node.child, input, buf, fn)
			})
			if err != nil {
				var bs *breakSignal
				if errors.As(err, &bs) || isCallbackError(err) {
					return err
				}
				return nil
			}
			return nil
		}
		var outputs [][]byte
		err := state.withOptionalScope(func(state execState) error {
			return execMulti(state, node.child, input, nil, func(result []byte) error {
				outputs = append(outputs, cloneExecBytes(result))
				return nil
			})
		})
		if err != nil {
			var bs *breakSignal
			if errors.As(err, &bs) {
				return err
			}
			return nil
		}
		for _, out := range outputs {
			if err := fn(append(buf[:0], out...)); err != nil {
				return err
			}
		}
		return nil
	case opNeg:
		return execNeg(state, node, input, buf, fn)
	case opLength:
		return execLength(input, buf, fn)
	case opAbs:
		result, err := execAbs(input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opToEntries:
		result, err := execToEntries(input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opFromEntries:
		result, err := execFromEntries(input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opFirst:
		err := execMulti(state, node.child, input, buf, func(result []byte) error {
			if err := fn(result); err != nil {
				return err
			}
			return errBreak
		})
		if err == errBreak {
			return nil
		}
		return err
	case opLast:
		return execLast(state, node, input, buf, fn)
	case opLimit:
		return execLimit(state, node, input, buf, fn)
	case opSkip:
		return execSkip(state, node, input, buf, fn)
	case opReduce:
		return execReduce(state, node, input, buf, fn)
	case opForeach:
		return execForeach(state, node, input, buf, fn)
	case opWhile:
		return execWhile(state, node, input, buf, fn)
	case opRepeat:
		return execRepeat(state, node, input, buf, fn)
	case opUntil:
		result, err := execUntil(state, node, input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opDefScope:
		return execDefScope(state, node, input, buf, fn)
	case opCall:
		return execCall(state, node, input, buf, fn)
	case opAssign:
		return execAssign(state, node, input, buf, fn)
	case opUpdate:
		return execUpdate(state, node, input, buf, fn)
	case opUpdateAlt:
		return execUpdateAlt(state, node, input, buf, fn)
	case opUpdateMath:
		return execUpdateMath(state, node, input, buf, fn)
	case opPath:
		return execPath(state, node, input, buf, fn)
	case opPick:
		result, err := execPick(state, node, input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opKeys:
		result, err := execKeys(input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opPaths:
		return execPaths(state, node, input, buf, fn)
	case opGetPath:
		return execGetPath(state, node, input, buf, fn)
	case opSetPath:
		result, err := execSetPath(state, node, input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opDelPaths:
		result, err := execDelPaths(state, node, input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opKeysUnsorted:
		result, err := execKeysUnsorted(input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opBuiltins:
		result := execBuiltins(buf)
		return fn(result)
	case opHaveDecnum:
		return fn(append(buf, "false"...))
	case opStrftime:
		return execStrftimeBuiltin(state, node, input, buf, false, fn)
	case opStrfLocaltime:
		return execStrftimeBuiltin(state, node, input, buf, true, fn)
	case opStrptime:
		return execStrptimeBuiltin(state, node, input, buf, fn)
	case opMktime:
		result, err := execMktime(input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opGmtime:
		result, err := execGmtime(input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opFromdate:
		result, err := execFromdate(input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opTodate:
		result, err := execTodate(input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opNow:
		return fn(execNow(buf))
	case opToStream:
		return execToStream(input, buf, fn)
	case opTruncateStream:
		return execTruncateStream(state, node, input, buf, fn)
	case opFromStream:
		result, err := execFromStream(state, node, input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opCombinations:
		return execCombinations(state, node, input, buf, fn)
	case opAny:
		return execAnyAll(state, node, input, buf, fn, false)
	case opAll:
		return execAnyAll(state, node, input, buf, fn, true)
	case opAdd:
		return execAdd(input, buf, fn)
	case opIndex1:
		return execFindIndexMulti(state, node, input, buf, fn, false, false)
	case opRIndex1:
		return execFindIndexMulti(state, node, input, buf, fn, true, false)
	case opIndicesN:
		return execFindIndexMulti(state, node, input, buf, fn, false, true)
	case opDebug:
		execDebug(input)
		return fn(input)
	case opBase64:
		result, err := execBase64Encode(input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opBase64D:
		result, err := execBase64Decode(input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opValues:
		return execValues(input, buf, fn)
	case opIn:
		return fn(execIn(state, node, input, buf))
	case opINBuiltin:
		result, err := execINBuiltin(state, node, input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opINDEXBuiltin:
		result, err := execINDEXBuiltin(state, node, input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opJOINBuiltin:
		result, err := execJOINBuiltin(state, node, input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opSlice:
		result, err := execSlice(state, node, input, buf)
		if err != nil {
			return err
		}
		if result == nil {
			return nil
		}
		if node.child != nil {
			return state.withIndexScope(state.chainedIndexScope(input), func(state execState) error {
				return execMulti(state, node.child, result, buf, fn)
			})
		}
		return fn(result)
	case opPlus:
		// Use execMulti for left side to support generators as operands (.[] + x etc.)
		return execMulti(state, node.left, input, nil, func(leftVal []byte) error {
			rightVal, err := execSingle(state, node.right, input, nil)
			if err != nil {
				return err
			}
			result, err := execPlusValues(leftVal, rightVal, buf)
			if err != nil {
				return err
			}
			return fn(result)
		})
	case opFlatten:
		result, err := execFlattenInto(state, input, buf, node)
		if err != nil {
			return err
		}
		return fn(result)
	case opSplit:
		return fn(execSplit(input, buf, node.field))
	case opJoin:
		result, joinErr := execJoin(input, buf, node.field)
		if joinErr != nil {
			return joinErr
		}
		return fn(result)
	case opTrimStr:
		result, err := execTrimStrBoth(state, node, input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opBsearch:
		return execBsearch(state, node, input, buf, fn)
	case opAsciiDowncase:
		result, err := execAsciiCase(input, buf, false)
		if err != nil {
			return err
		}
		return fn(result)
	case opAsciiUpcase:
		result, err := execAsciiCase(input, buf, true)
		if err != nil {
			return err
		}
		return fn(result)
	case opExplode:
		result, err := execExplode(input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opImplode:
		result, err := execImplode(input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opUTF8ByteLength:
		result, err := execUTF8ByteLength(input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opReverse:
		result, err := execReverse(input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opIsNaN:
		return fn(execIsNaN(input, buf))
	case opIsInfinite:
		return fn(execIsInfinite(input, buf))
	case opIsFinite:
		return fn(execIsFinite(input, buf))
	case opIsNormal:
		return fn(execIsNormal(input, buf))
	case opPow:
		result, err := execPow(state, node, input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opHypot:
		result, err := execHypot(state, node, input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opFMA:
		result, err := execFMA(state, node, input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opStartsWith:
		return fn(execStringPredicate(input, buf, node.field, true, false))
	case opEndsWith:
		return fn(execStringPredicate(input, buf, node.field, false, true))
	case opTrim:
		result, err := execTrim(input, buf, trimBoth)
		if err != nil {
			return err
		}
		return fn(result)
	case opLtrim:
		result, err := execTrim(input, buf, trimLeft)
		if err != nil {
			return err
		}
		return fn(result)
	case opRtrim:
		result, err := execTrim(input, buf, trimRight)
		if err != nil {
			return err
		}
		return fn(result)
	case opLtrimStr:
		result, err := execTrimStr(state, node, input, buf, true)
		if err != nil {
			return err
		}
		return fn(result)
	case opRtrimStr:
		result, err := execTrimStr(state, node, input, buf, false)
		if err != nil {
			return err
		}
		return fn(result)
	case opEmpty:
		return nil // produce zero outputs — never call fn
	case opHas:
		return execHas(node, input, buf, fn)
	case opIf:
		return execIf(state, node, input, buf, fn)
	case opSelect:
		return execSelect(state, node, input, buf, fn)
	case opAlternative:
		return execAlternative(state, node, input, buf, fn)
	case opMinus, opMul, opDiv, opMod:
		// Supports generators as either operand (e.g. range(3) * 2 or 1 * range(3)).
		// Single-output right side: use execSingle to avoid fn being captured in a
		// nested execMulti call (which would create an escape analysis cycle).
		// Multi-output right side: collect values first without fn, then compute.
		if !hasMultiOutput(node.right) {
			return execMulti(state, node.left, input, nil, func(leftVal []byte) error {
				rightVal, err := execSingle(state, node.right, input, nil)
				if err != nil {
					return err
				}
				result, err := execArithValues(node.typ, leftVal, rightVal, buf)
				if err != nil {
					return err
				}
				return fn(result)
			})
		}
		var leftVals [][]byte
		if err := execMulti(state, node.left, input, nil, func(leftVal []byte) error {
			leftVals = append(leftVals, leftVal)
			return nil
		}); err != nil {
			return err
		}
		return execMulti(state, node.right, input, nil, func(rightVal []byte) error {
			for _, lv := range leftVals {
				result, err := execArithValues(node.typ, lv, rightVal, buf)
				if err != nil {
					return err
				}
				if err := fn(result); err != nil {
					return err
				}
			}
			return nil
		})
	case opMin:
		result, err := execMinMax(state, input, buf, node, false)
		if err != nil {
			return err
		}
		return fn(result)
	case opMax:
		result, err := execMinMax(state, input, buf, node, true)
		if err != nil {
			return err
		}
		return fn(result)
	case opMinBy:
		result, err := execMinMax(state, input, buf, node, false)
		if err != nil {
			return err
		}
		return fn(result)
	case opMaxBy:
		result, err := execMinMax(state, input, buf, node, true)
		if err != nil {
			return err
		}
		return fn(result)
	case opSort:
		return execSort(input, buf, fn)
	case opSortBy:
		return execSortBy(state, node, input, buf, fn)
	case opUnique:
		return execUnique(input, buf, fn)
	case opUniqueBy:
		return execUniqueBy(state, node, input, buf, fn)
	case opGroupBy:
		return execGroupBy(state, node, input, buf, fn)
	case opTranspose:
		return execTranspose(input, buf, fn)
	case opURIEncode:
		result, err := execURIEncode(input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opTry:
		wrappedFn := func(result []byte) error {
			if err := fn(result); err != nil {
				return &downstreamError{err: err}
			}
			return nil
		}
		err := state.withTryScope(func(state execState) error {
			return execMulti(state, node.left, input, buf, wrappedFn)
		})
		if err == nil {
			return nil
		}
		if de, ok := err.(*downstreamError); ok {
			return de.err
		}
		if isControlFlowError(err) {
			return err
		}
		// Real error — suppress or run catch handler
		if node.right == nil {
			return nil
		}
		err = execMulti(state, node.right, catchInputFromError(err), buf, wrappedFn)
		if de, ok := err.(*downstreamError); ok {
			return de.err
		}
		return err
	case opToJSON:
		return fn(execToJSON(input, buf))
	case opFromJSON:
		result, err := execFromJSON(input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opToString:
		return fn(execToString(input, buf))
	case opToNumber:
		result, err := execToNumber(input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opToBoolean:
		result, err := execToBoolean(input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opContains:
		argVal, err := execSingle(state, node.child, input, nil)
		if err != nil {
			return fn(append(buf, "false"...))
		}
		var contains bool
		if node.optional { // inside(b): b contains input
			contains = jsonContains(argVal, input)
		} else { // contains(b): input contains b
			contains = jsonContains(input, argVal)
		}
		if contains {
			return fn(append(buf, "true"...))
		}
		return fn(append(buf, "false"...))
	case opTest:
		return fn(boolResult(buf, execTest(node, input)))
	case opMatchRe:
		result, err := execMatchRe(node, input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opCapture:
		result, err := execCapture(node, input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opScan:
		return execScan(node, input, buf, fn)
	case opSub:
		result, err := execSub(node, input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opGSub:
		result, err := execGSub(node, input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opRange:
		return execRange(state, node, input, fn)
	case opFloor:
		return fn(execRoundMode(input, buf, roundFloor))
	case opCeil:
		return fn(execRoundMode(input, buf, roundCeil))
	case opRound:
		return fn(execRoundMode(input, buf, roundNearest))
	case opMathSqrt:
		return fn(execMathFunc(input, buf, mathSqrt))
	case opMathFabs:
		return fn(execMathFunc(input, buf, mathFabs))
	case opMathAtan:
		return fn(execMathFunc(input, buf, mathAtan))
	case opMathLog:
		return fn(execMathFunc(input, buf, mathLog))
	case opMathLog2:
		return fn(execMathFunc(input, buf, mathLog2))
	case opMathLog10:
		return fn(execMathFunc(input, buf, mathLog10))
	case opMathExp:
		return fn(execMathFunc(input, buf, mathExp))
	case opMathExp2:
		return fn(execMathFunc(input, buf, mathExp2))
	case opMathExp10:
		return fn(execMathFunc(input, buf, mathExp10))
	case opMathCbrt:
		return fn(execMathFunc(input, buf, mathCbrt))
	case opMathLogb:
		return fn(execMathFunc(input, buf, mathLogb))
	case opMathNearbyint:
		return fn(execMathFunc(input, buf, mathNearbyint))
	case opMathJ0:
		return fn(execMathFunc(input, buf, mathJ0))
	case opMathJ1:
		return fn(execMathFunc(input, buf, mathJ1))
	case opMathSin:
		return fn(execMathFunc(input, buf, mathSin))
	case opMathCos:
		return fn(execMathFunc(input, buf, mathCos))
	case opMathTan:
		return fn(execMathFunc(input, buf, mathTan))
	case opMathAsin:
		return fn(execMathFunc(input, buf, mathAsin))
	case opMathAcos:
		return fn(execMathFunc(input, buf, mathAcos))
	case opMathTgamma:
		return fn(execMathFunc(input, buf, mathTgamma))
	case opMathLgamma:
		return fn(execMathFunc(input, buf, mathLgamma))
	case opError:
		// Throw as a jsonError so try-catch handlers receive the original JSON value.
		// 0-arg: throw input. 1-arg error(expr): throw the evaluated expression.
		var payload []byte
		if node.child != nil {
			val, err := execSingle(state, node.child, input, nil)
			if err != nil {
				return err
			}
			payload = append([]byte(nil), trimWhitespace(val)...)
		} else {
			payload = append([]byte(nil), trimWhitespace(input)...)
		}
		return &jsonError{payload: payload}
	case opStringInterp:
		result, err := execStringTemplate(state, node, input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opFormatTemplate:
		result, err := execFormatTemplate(state, node, input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opIsEmpty:
		// isempty(expr): true if expr produces no outputs, false otherwise.
		produced := false
		err := execMulti(state, node.child, input, buf, func(_ []byte) error {
			produced = true
			return errBreak
		})
		if err != nil && err != errBreak {
			return err
		}
		if produced {
			return fn(append(buf, "false"...))
		}
		return fn(append(buf, "true"...))
	case opNth:
		// nth(n; gen): emit the nth output of gen (0-indexed). No output if not enough.
		nf, ok := parseJSONFloat(trimWhitespace(func() []byte {
			v, _ := execSingle(state, node.left, input, nil)
			return v
		}()))
		if !ok {
			return nil
		}
		nInt := int(nf)
		if nInt < 0 {
			return fmt.Errorf("nth doesn't support negative indices")
		}
		count := 0
		var found []byte
		err := execMulti(state, node.child, input, nil, func(val []byte) error {
			if count == nInt {
				found = val
				return errBreak
			}
			count++
			return nil
		})
		if err != nil && err != errBreak {
			return err
		}
		if found == nil {
			return nil // not enough outputs — produce nothing
		}
		return fn(append(buf, found...))
	case opGenerator:
		for _, elem := range node.elems {
			if err := execMulti(state, elem, input, buf, fn); err != nil {
				return err
			}
		}
		return nil
	case opHTMLEncode:
		result, err := execHTMLEncode(input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opCSVEncode:
		result, err := execCSVEncode(input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opTSVEncode:
		result, err := execTSVEncode(input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opShEncode:
		result, err := execShEncode(input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opURIDecode:
		result, err := execURIDecode(input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	default:
		return fmt.Errorf("unknown op type: %d", node.typ)
	}
}

// execSingle evaluates a node that is expected to produce a single result,
// without creating a closure. Handles common op types directly.
// Falls back to exec for complex/multi-output types.
func execSingle(state execState, node *op, input []byte, buf []byte) ([]byte, error) {
	switch node.typ {
	case opLiteral:
		return normalizeOutputValue(node.literal, buf), nil
	case opIdentity:
		return execIdentity(input, buf)
	case opField:
		return execField(state, node, input, buf)
	case opIndex:
		return execIndex(state, node, input, buf)
	case opIndexExpr:
		return execIndexExpr(state, node, input, buf)
	case opVar:
		if node.name == "__loc__" {
			value := []byte(`{"file":"<top-level>","line":1}`)
			if node.child != nil {
				return exec(state, node.child, value, buf)
			}
			if buf == nil {
				return value[:len(value):len(value)], nil
			}
			return append(buf, value...), nil
		}
		ctx := state.currentExecContext()
		value, ok := ctx.lookupVar(node.name)
		if !ok {
			return nil, fmt.Errorf("$%s is not defined", node.name)
		}
		if node.child != nil {
			return execSingle(state, node.child, value, buf)
		}
		if buf == nil {
			return value[:len(value):len(value)], nil
		}
		return append(buf, value...), nil
	case opTypeBuiltin:
		return execTypeSingle(input, buf)
	case opCompare:
		return execCompareSingle(state, node, input, buf)
	case opAnd:
		leftVal, err := execSingle(state, node.left, input, buf)
		if err != nil {
			return nil, err
		}
		if isFalsy(leftVal) {
			if buf == nil {
				return bFalse, nil
			}
			return append(buf[:0], "false"...), nil
		}
		rightVal, err := execSingle(state, node.right, input, buf)
		if err != nil {
			return nil, err
		}
		if isFalsy(rightVal) {
			if buf == nil {
				return bFalse, nil
			}
			return append(buf[:0], "false"...), nil
		}
		if buf == nil {
			return bTrue, nil
		}
		return append(buf[:0], "true"...), nil
	case opOr:
		leftVal, err := execSingle(state, node.left, input, buf)
		if err != nil {
			return nil, err
		}
		if !isFalsy(leftVal) {
			if buf == nil {
				return bTrue, nil
			}
			return append(buf[:0], "true"...), nil
		}
		rightVal, err := execSingle(state, node.right, input, buf)
		if err != nil {
			return nil, err
		}
		if !isFalsy(rightVal) {
			if buf == nil {
				return bTrue, nil
			}
			return append(buf[:0], "true"...), nil
		}
		if buf == nil {
			return bFalse, nil
		}
		return append(buf[:0], "false"...), nil
	case opNot:
		if isFalsy(input) {
			if buf == nil {
				return bTrue, nil
			}
			return append(buf, "true"...), nil
		}
		if buf == nil {
			return bFalse, nil
		}
		return append(buf, "false"...), nil
	case opOptional:
		result, err := execSingle(state, node.child, input, buf)
		if err != nil {
			if buf == nil {
				return bNull, nil
			}
			return append(buf[:0], "null"...), nil
		}
		return result, nil
	case opNeg:
		return execNegSingle(state, node, input, buf)
	case opLength:
		return execLengthSingle(input, buf)
	case opAbs:
		return execAbs(input, buf)
	case opToEntries:
		return execToEntries(input, buf)
	case opFromEntries:
		return execFromEntries(input, buf)
	case opFirst:
		return execFirstResult(state, node, input, buf)
	case opLast:
		return execLastSingle(state, node, input, buf)
	case opSkip:
		return execFirstResult(state, node, input, buf)
	case opReduce:
		return execFirstResult(state, node, input, buf)
	case opForeach:
		return execFirstResult(state, node, input, buf)
	case opWhile:
		return execFirstResult(state, node, input, buf)
	case opRepeat:
		return execFirstResult(state, node, input, buf)
	case opRecursiveDescent:
		return execFirstResult(state, node, input, buf)
	case opRecurse:
		return execFirstResult(state, node, input, buf)
	case opWalk:
		return execFirstResult(state, node, input, buf)
	case opUntil:
		return execUntil(state, node, input, buf)
	case opDefScope:
		return execFirstResult(state, node, input, buf)
	case opCall:
		return execFirstResult(state, node, input, buf)
	case opAssign:
		return execFirstResult(state, node, input, buf)
	case opUpdate:
		return execFirstResult(state, node, input, buf)
	case opUpdateAlt:
		return execFirstResult(state, node, input, buf)
	case opUpdateMath:
		return execFirstResult(state, node, input, buf)
	case opPath:
		return execFirstResult(state, node, input, buf)
	case opKeys:
		return execKeys(input, buf)
	case opPaths:
		return execFirstResult(state, node, input, buf)
	case opPick:
		return execPick(state, node, input, buf)
	case opGetPath:
		return execFirstResult(state, node, input, buf)
	case opSetPath:
		return execSetPath(state, node, input, buf)
	case opDelPaths:
		return execDelPaths(state, node, input, buf)
	case opKeysUnsorted:
		return execKeysUnsorted(input, buf)
	case opBuiltins:
		return execBuiltins(buf), nil
	case opHaveDecnum:
		return append(buf, "false"...), nil
	case opStrftime, opStrfLocaltime, opStrptime:
		return execFirstResult(state, node, input, buf)
	case opMktime:
		return execMktime(input, buf)
	case opGmtime:
		return execGmtime(input, buf)
	case opFromdate:
		return execFromdate(input, buf)
	case opTodate:
		return execTodate(input, buf)
	case opNow:
		return execNow(buf), nil
	case opToStream:
		return execFirstResult(state, node, input, buf)
	case opTruncateStream:
		return execFirstResult(state, node, input, buf)
	case opFromStream:
		return execFromStream(state, node, input, buf)
	case opCombinations:
		return execFirstResult(state, node, input, buf)
	case opAny:
		return execAnyAllSingle(state, node, input, buf, false)
	case opAll:
		return execAnyAllSingle(state, node, input, buf, true)
	case opIndex1:
		return execFindIndex(state, node, input, buf, false, false), nil
	case opRIndex1:
		return execFindIndex(state, node, input, buf, true, false), nil
	case opIndicesN:
		return execFindIndex(state, node, input, buf, false, true), nil
	case opDebug:
		execDebug(input)
		if buf == nil {
			return input[:len(input):len(input)], nil
		}
		return append(buf, input...), nil
	case opBase64:
		return execBase64Encode(input, buf)
	case opBase64D:
		return execBase64Decode(input, buf)
	case opValues:
		if isNull(input) {
			return bNull, nil // null → null in single-result context
		}
		if buf == nil {
			return input[:len(input):len(input)], nil
		}
		return append(buf, input...), nil
	case opIn:
		return execIn(state, node, input, buf), nil
	case opINBuiltin:
		return execINBuiltin(state, node, input, buf)
	case opINDEXBuiltin:
		return execINDEXBuiltin(state, node, input, buf)
	case opJOINBuiltin:
		return execJOINBuiltin(state, node, input, buf)
	case opSlice:
		result, err := execSlice(state, node, input, buf)
		if err != nil {
			return nil, err
		}
		if result == nil {
			if buf == nil {
				return bNull, nil
			}
			return append(buf, "null"...), nil
		}
		if node.child != nil {
			var out []byte
			err := state.withIndexScope(state.chainedIndexScope(input), func(state execState) error {
				var execErr error
				out, execErr = execSingle(state, node.child, result, buf)
				return execErr
			})
			return out, err
		}
		return result, nil
	case opPlus:
		return execPlusSingle(state, node, input, buf)
	case opFlatten:
		return execFlattenInto(state, input, buf, node)
	case opSplit:
		return execSplit(input, buf, node.field), nil
	case opJoin:
		return execJoin(input, buf, node.field)
	case opBsearch:
		return execFirstResult(state, node, input, buf)
	case opAsciiDowncase:
		return execAsciiCase(input, buf, false)
	case opAsciiUpcase:
		return execAsciiCase(input, buf, true)
	case opStartsWith:
		return execStringPredicate(input, buf, node.field, true, false), nil
	case opEndsWith:
		return execStringPredicate(input, buf, node.field, false, true), nil
	case opTrim:
		return execTrim(input, buf, trimBoth)
	case opLtrim:
		return execTrim(input, buf, trimLeft)
	case opRtrim:
		return execTrim(input, buf, trimRight)
	case opTrimStr:
		return execTrimStrBoth(state, node, input, buf)
	case opLtrimStr:
		return execTrimStr(state, node, input, buf, true)
	case opRtrimStr:
		return execTrimStr(state, node, input, buf, false)
	case opExplode:
		return execExplode(input, buf)
	case opImplode:
		return execImplode(input, buf)
	case opUTF8ByteLength:
		return execUTF8ByteLength(input, buf)
	case opReverse:
		return execReverse(input, buf)
	case opIsNaN:
		return execIsNaN(input, buf), nil
	case opIsInfinite:
		return execIsInfinite(input, buf), nil
	case opIsFinite:
		return execIsFinite(input, buf), nil
	case opIsNormal:
		return execIsNormal(input, buf), nil
	case opPow:
		return execPow(state, node, input, buf)
	case opHypot:
		return execHypot(state, node, input, buf)
	case opFMA:
		return execFMA(state, node, input, buf)
	case opMinus, opMul, opDiv, opMod:
		return execArith(state, node, input, buf)
	case opMin, opMinBy:
		return execMinMax(state, input, buf, node, false)
	case opMax, opMaxBy:
		return execMinMax(state, input, buf, node, true)
	case opURIEncode:
		return execURIEncode(input, buf)
	case opTest:
		return boolResult(buf, execTest(node, input)), nil
	case opMatchRe:
		return execMatchRe(node, input, buf)
	case opCapture:
		return execCapture(node, input, buf)
	case opSub:
		return execSub(node, input, buf)
	case opGSub:
		return execGSub(node, input, buf)
	// opScan is intentionally absent — it is multi-output only; falls through to exec()

	// --- Tier 0 ops added here to bypass execFirstResult/execMulti entirely ---
	// execMulti's fn parameter has been marked as "escapes to heap" by Go's escape
	// analysis due to d44ce30's changes (execCompare using nested execMulti closures,
	// constructPairsInto capturing fn recursively). Any op routed through
	// execFirstResult→execMulti incurs 3 allocs/op even for simple operations.
	// Adding these ops directly to execSingle with no-closure implementations
	// restores 0 allocs/op for all Tier 0 operations.

	case opDelete:
		return execDelete(state, node, input, buf)

	case opConstruct:
		if node.multiValuePairs {
			return execFirstResult(state, node, input, buf) // Tier 2 — Cartesian product
		}
		return execConstruct(state, node, input, buf)

	case opPipe:
		// Fast path when left is single-output: chain execSingle calls directly.
		// When left is multi-output (e.g. .[] | select(...)), fall through to
		// execFirstResult so the full pipeline is evaluated and only the first
		// passing result is returned (correct for first(.[] | select(...)) etc.).
		if isSingleOutputOp(node.left) {
			intermediate, err := execSingle(state, node.left, input, nil)
			if err != nil {
				return nil, err
			}
			return execSingle(state, node.right, intermediate, buf)
		}
		return execFirstResult(state, node, input, buf)
	case opApply:
		if isSingleOutputOp(node.left) {
			intermediate, err := execSingle(state, node.left, input, nil)
			if err != nil {
				return nil, err
			}
			var result []byte
			err = state.withIndexScope(state.chainedIndexScope(input), func(state execState) error {
				var execErr error
				result, execErr = execSingle(state, node.right, intermediate, buf)
				return execErr
			})
			return result, err
		}
		return execFirstResult(state, node, input, buf)

	case opSelect:
		if isSingleOutputOp(node.child) {
			// Pass buf as scratch so condition expressions (e.g. ascii_downcase, construct)
			// don't need to allocate their own buffer. condVal may be written into buf,
			// but we only test truthiness and then reset buf for the actual output.
			condVal, err := execSingle(state, node.child, input, buf)
			if err != nil {
				return nil, err
			}
			if isFalsy(condVal) {
				return nil, nil // condition false — no output
			}
		} else {
			foundTruthy := false
			err := execMulti(state, node.child, input, nil, func(condVal []byte) error {
				if isFalsy(condVal) {
					return nil
				}
				foundTruthy = true
				return errBreak
			})
			if err != nil && err != errBreak {
				return nil, err
			}
			if !foundTruthy {
				return nil, nil
			}
		}
		if buf == nil {
			return input[:len(input):len(input)], nil
		}
		// Reset buf (discard condition scratch) and write the output (the input value).
		return append(buf[:0], input...), nil

	case opHas:
		s := scanner{data: input}
		s.skipWhitespace()
		if len(node.literal) > 0 && node.literal[0] == 'a' {
			// has(n) on array
			if s.pos >= len(s.data) || s.data[s.pos] != '[' {
				return boolResult(buf, false), nil
			}
			length := s.arrayLen()
			return boolResult(buf, node.index >= 0 && node.index < length), nil
		}
		// has("key") on object
		if s.pos >= len(s.data) || s.data[s.pos] != '{' {
			return boolResult(buf, false), nil
		}
		vs, _ := s.findFieldStr(node.field)
		return boolResult(buf, vs != -1), nil

	case opIf:
		// Pass buf as scratch for condition evaluation; reset for branch output.
		condVal, err := execSingle(state, node.left, input, buf)
		if err != nil {
			return nil, err
		}
		if !isFalsy(condVal) {
			return execSingle(state, node.right, input, buf[:0])
		}
		if node.child != nil {
			return execSingle(state, node.child, input, buf[:0])
		}
		return execIdentity(input, buf[:0])

	case opAdd:
		// Call execAdd directly (not through execMulti) so execAdd's internal
		// arrayIter closures don't inherit execMulti's fn escape contamination.
		var addResult []byte
		if err := execAdd(input, buf, func(r []byte) error { addResult = r; return nil }); err != nil {
			return nil, err
		}
		if addResult == nil {
			return append(buf, "null"...), nil
		}
		return addResult, nil

	case opAlternative:
		// Single-result path: if left is truthy return it, else evaluate right.
		leftVal, err := execSingle(state, node.left, input, nil)
		if err == nil && !isFalsy(leftVal) {
			if buf == nil {
				return leftVal, nil
			}
			return append(buf, leftVal...), nil
		}
		return execSingle(state, node.right, input, buf)

	case opTry:
		var result []byte
		err := state.withTryScope(func(state execState) error {
			var execErr error
			result, execErr = execSingle(state, node.left, input, buf)
			return execErr
		})
		if err == nil {
			return result, nil
		}
		if isControlFlowError(err) {
			return nil, err // propagate break signal
		}
		// Error: suppress or run catch handler.
		if node.right == nil {
			return nil, nil // no catch — produce no output
		}
		return execSingle(state, node.right, catchInputFromError(err), buf)

	case opStringInterp:
		return execStringTemplate(state, node, input, buf)
	case opFormatTemplate:
		return execFormatTemplate(state, node, input, buf)

	case opContains:
		argVal, err := execSingle(state, node.child, input, nil)
		if err != nil {
			return boolResult(buf, false), nil
		}
		var contains bool
		if node.optional {
			contains = jsonContains(argVal, input)
		} else {
			contains = jsonContains(input, argVal)
		}
		return boolResult(buf, contains), nil

	case opHTMLEncode:
		return execHTMLEncode(input, buf)
	case opCSVEncode:
		return execCSVEncode(input, buf)
	case opTSVEncode:
		return execTSVEncode(input, buf)
	case opShEncode:
		return execShEncode(input, buf)
	case opURIDecode:
		return execURIDecode(input, buf)

	case opFloor:
		return execRoundMode(input, buf, roundFloor), nil
	case opCeil:
		return execRoundMode(input, buf, roundCeil), nil
	case opRound:
		return execRoundMode(input, buf, roundNearest), nil

	case opMathSqrt:
		return execMathFunc(input, buf, mathSqrt), nil
	case opMathFabs:
		return execMathFunc(input, buf, mathFabs), nil
	case opMathAtan:
		return execMathFunc(input, buf, mathAtan), nil
	case opMathLog:
		return execMathFunc(input, buf, mathLog), nil
	case opMathLog2:
		return execMathFunc(input, buf, mathLog2), nil
	case opMathLog10:
		return execMathFunc(input, buf, mathLog10), nil
	case opMathExp:
		return execMathFunc(input, buf, mathExp), nil
	case opMathExp2:
		return execMathFunc(input, buf, mathExp2), nil
	case opMathExp10:
		return execMathFunc(input, buf, mathExp10), nil
	case opMathCbrt:
		return execMathFunc(input, buf, mathCbrt), nil
	case opMathLogb:
		return execMathFunc(input, buf, mathLogb), nil
	case opMathNearbyint:
		return execMathFunc(input, buf, mathNearbyint), nil
	case opMathJ0:
		return execMathFunc(input, buf, mathJ0), nil
	case opMathJ1:
		return execMathFunc(input, buf, mathJ1), nil
	case opMathSin:
		return execMathFunc(input, buf, mathSin), nil
	case opMathCos:
		return execMathFunc(input, buf, mathCos), nil
	case opMathTan:
		return execMathFunc(input, buf, mathTan), nil
	case opMathAsin:
		return execMathFunc(input, buf, mathAsin), nil
	case opMathAcos:
		return execMathFunc(input, buf, mathAcos), nil
	case opMathTgamma:
		return execMathFunc(input, buf, mathTgamma), nil
	case opMathLgamma:
		return execMathFunc(input, buf, mathLgamma), nil

	case opToJSON:
		return execToJSON(input, buf), nil
	case opFromJSON:
		return execFromJSON(input, buf)
	case opToString:
		return execToString(input, buf), nil
	case opToNumber:
		return execToNumber(input, buf)
	case opToBoolean:
		return execToBoolean(input, buf)
	default:
		return execFirstResult(state, node, input, buf)
	}
}

// execFirstResult executes node via the full execMulti callback machinery and
// returns the first result. Used as the fallback for multi-output ops (iterators,
// pipes with multi-output left sides, scan, etc.) that execSingle cannot handle
// directly without closures.
func execFirstResult(state execState, node *op, input []byte, buf []byte) ([]byte, error) {
	var result []byte
	err := execMulti(state, node, input, buf, func(r []byte) error {
		if result == nil {
			result = r
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return append(buf, "null"...), nil
	}
	return result, nil
}

// exec returns the first result of executing node against input.
// It routes through execSingle which has a direct (no-closure) fast path for
// all common single-output operations (field access, comparison, arithmetic,
// math, etc.). Multi-output ops fall back to execFirstResult via execSingle's
// default case.
func exec(state execState, node *op, input []byte, buf []byte) ([]byte, error) {
	return execSingle(state, node, input, buf)
}

// execIdentity copies the input to the output buffer (trimmed of whitespace).
// When buf is nil, returns a cap-limited sub-slice of input directly (zero-alloc).
// Cap-limited prevents callers from using spare capacity as scratch, which would
// corrupt the input bytes.
func execIdentity(input []byte, buf []byte) ([]byte, error) {
	s := &scanner{data: input}
	s.skipWhitespace()
	start := s.pos
	s.skipValue()
	return normalizeOutputValue(input[start:s.pos], buf), nil
}

// execFieldMulti extracts a field value from a JSON object, then recurses
// into the child (if any) via execMulti to support multi-output children like iterators.
func execFieldMulti(state execState, node *op, input []byte, buf []byte, fn func([]byte) error) error {
	s := &scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '{' {
		if node.optional {
			return nil
		}
		// jq returns null for null | .field; all other non-object types error.
		if s.pos < len(s.data) && s.data[s.pos] == 'n' {
			if buf == nil {
				return fn(bNull)
			}
			return fn(append(buf, "null"...))
		}
		return fieldAccessError(input, node.field)
	}

	vs, ve := s.findFieldStr(node.field)
	if vs == -1 {
		// Missing field: return null without following the child chain.
		if buf == nil {
			return fn(bNull)
		}
		return fn(append(buf, "null"...))
	}

	value := input[vs:ve]

	if node.child != nil {
		return state.withIndexScope(state.chainedIndexScope(input), func(state execState) error {
			return execMulti(state, node.child, value, buf, fn)
		})
	}
	if buf == nil {
		return fn(value[:len(value):len(value)])
	}
	return fn(append(buf, value...))
}

// execField extracts a field value from a JSON object (single-result convenience).
func execField(state execState, node *op, input []byte, buf []byte) ([]byte, error) {
	s := &scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '{' {
		if node.optional {
			return append(buf, "null"...), nil
		}
		// jq returns null for null | .field; all other non-object types error.
		if s.pos < len(s.data) && s.data[s.pos] == 'n' {
			if buf == nil {
				return bNull, nil
			}
			return append(buf, "null"...), nil
		}
		return nil, fieldAccessError(input, node.field)
	}

	vs, ve := s.findFieldStr(node.field)
	if vs == -1 {
		if buf == nil {
			return bNull, nil
		}
		return append(buf, "null"...), nil
	}

	value := input[vs:ve]

	if node.child != nil {
		var result []byte
		err := state.withIndexScope(state.chainedIndexScope(input), func(state execState) error {
			var execErr error
			result, execErr = exec(state, node.child, value, buf)
			return execErr
		})
		return result, err
	}
	if buf == nil {
		return value[:len(value):len(value)], nil
	}
	return append(buf, value...), nil
}

// execIndexMulti accesses an array element by index, then recurses into
// the child (if any) via execMulti to support multi-output children.
func execIndexMulti(state execState, node *op, input []byte, buf []byte, fn func([]byte) error) error {
	s := &scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '[' {
		if node.optional {
			return nil
		}
		return indexAccessError(input, node.index)
	}

	idx := node.index
	if idx < 0 {
		length := s.arrayLen()
		idx = length + idx
		if idx < 0 {
			return fn(append(buf, "null"...))
		}
	}

	var result []byte
	s.arrayIter(func(i int, elemStart, elemEnd int) bool {
		if i == idx {
			result = input[elemStart:elemEnd]
			return false
		}
		return true
	})

	if result == nil {
		return fn(append(buf, "null"...))
	}

	if node.child != nil {
		return state.withIndexScope(state.chainedIndexScope(input), func(state execState) error {
			return execMulti(state, node.child, result, buf, fn)
		})
	}
	if buf == nil {
		return fn(result[:len(result):len(result)])
	}
	return fn(append(buf, result...))
}

// execIndex accesses an array element by index. Negative indices count from end.
func execIndex(state execState, node *op, input []byte, buf []byte) ([]byte, error) {
	s := &scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '[' {
		if node.optional {
			return append(buf, "null"...), nil
		}
		return nil, indexAccessError(input, node.index)
	}

	idx := node.index
	if idx < 0 {
		// Negative index: count elements first
		length := s.arrayLen()
		idx = length + idx
		if idx < 0 {
			return append(buf, "null"...), nil
		}
	}

	var result []byte
	s.arrayIter(func(i int, elemStart, elemEnd int) bool {
		if i == idx {
			result = input[elemStart:elemEnd]
			return false
		}
		return true
	})

	if result == nil {
		return append(buf, "null"...), nil
	}

	if node.child != nil {
		var out []byte
		err := state.withIndexScope(state.chainedIndexScope(input), func(state execState) error {
			var execErr error
			out, execErr = exec(state, node.child, result, buf)
			return execErr
		})
		return out, err
	}
	if buf == nil {
		return result[:len(result):len(result)], nil
	}
	return append(buf, result...), nil
}

func execIndexExprMulti(state execState, node *op, input []byte, buf []byte, fn func([]byte) error) error {
	scopeInput := state.currentIndexScope()
	if scopeInput == nil {
		scopeInput = input
	}
	return execMulti(state, node.left, scopeInput, nil, func(key []byte) error {
		result, err := execIndexExprAccess(input, key, buf)
		if err != nil {
			return err
		}
		if node.child != nil {
			return state.withIndexScope(state.chainedIndexScope(input), func(state execState) error {
				return execMulti(state, node.child, result, buf, fn)
			})
		}
		return fn(result)
	})
}

func execIndexExpr(state execState, node *op, input []byte, buf []byte) ([]byte, error) {
	scopeInput := state.currentIndexScope()
	if scopeInput == nil {
		scopeInput = input
	}
	key, err := execSingle(state, node.left, scopeInput, nil)
	if err != nil {
		return nil, err
	}
	result, err := execIndexExprAccess(input, key, buf)
	if err != nil {
		return nil, err
	}
	if node.child != nil {
		var out []byte
		err := state.withIndexScope(state.chainedIndexScope(input), func(state execState) error {
			var execErr error
			out, execErr = exec(state, node.child, result, buf)
			return execErr
		})
		return out, err
	}
	return result, nil
}

func execIndexExprAccess(input, key, buf []byte) ([]byte, error) {
	key = trimWhitespace(key)
	if len(key) == 0 {
		return nil, fmt.Errorf("Cannot index %s with invalid", jsonTypeName(input))
	}
	ks := scanner{data: key}
	ks.skipWhitespace()
	if ks.pos < len(ks.data) && ks.data[ks.pos] == '"' {
		field := ks.readString()
		return execFieldValue(input, field, buf)
	}
	if f, ok := parseJSONFloat(key); ok {
		if math.IsNaN(f) || math.IsInf(f, 0) {
			s := scanner{data: input}
			s.skipWhitespace()
			if s.pos < len(s.data) && (s.data[s.pos] == '[' || s.data[s.pos] == 'n') {
				if buf == nil {
					return bNull, nil
				}
				return append(buf, "null"...), nil
			}
			return nil, dynamicIndexNumberAccessError(input)
		}
		return execIndexValue(input, int(f), buf)
	}
	return nil, fmt.Errorf("Cannot index %s with %s", jsonTypeName(input), jsonTypeName(key))
}

func execFieldValue(input, field, buf []byte) ([]byte, error) {
	s := &scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '{' {
		if s.pos < len(s.data) && s.data[s.pos] == 'n' {
			if buf == nil {
				return bNull, nil
			}
			return append(buf, "null"...), nil
		}
		return nil, fieldAccessError(input, string(field))
	}
	vs, ve := s.findField(field)
	if vs == -1 {
		if buf == nil {
			return bNull, nil
		}
		return append(buf, "null"...), nil
	}
	return normalizeOutputValue(input[vs:ve], buf), nil
}

func execIndexValue(input []byte, index int, buf []byte) ([]byte, error) {
	s := &scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '[' {
		return nil, dynamicIndexNumberAccessError(input)
	}
	idx := index
	if idx < 0 {
		length := s.arrayLen()
		idx = length + idx
		if idx < 0 {
			if buf == nil {
				return bNull, nil
			}
			return append(buf, "null"...), nil
		}
	}
	var result []byte
	s.arrayIter(func(i int, elemStart, elemEnd int) bool {
		if i == idx {
			result = input[elemStart:elemEnd]
			return false
		}
		return true
	})
	if result == nil {
		if buf == nil {
			return bNull, nil
		}
		return append(buf, "null"...), nil
	}
	return normalizeOutputValue(result, buf), nil
}

// execIterator iterates all elements of an array or values of an object.
func execIterator(state execState, node *op, input []byte, buf []byte, fn func([]byte) error) error {
	s := &scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) {
		if node.optional {
			return nil
		}
		return fmt.Errorf("Cannot iterate over %s (%s)", jsonTypeName(input), previewJSONValue(input))
	}

	switch s.data[s.pos] {
	case '[':
		var caught error
		s.arrayIter(func(index int, elemStart, elemEnd int) bool {
			var err error
			if node.child != nil {
				scope := state.chainedIndexScope(input)
				err = state.withIndexScope(scope, func(state execState) error {
					return execMulti(state, node.child, input[elemStart:elemEnd], buf, fn)
				})
			} else {
				err = fn(input[elemStart:elemEnd])
			}
			if err != nil {
				// Propagate jsonError (from `error` builtin) and errBreak so
				// try-catch and limit/first work correctly. Regular errors
				// (e.g. field access on wrong type) are dropped, preserving
				// the existing lenient multi-output behaviour.
				if _, ok := err.(*jsonError); ok {
					if !state.tryScopeActive() {
						return false
					}
					caught = err
				} else if isControlFlowError(err) {
					caught = err
				} else if state.optionalScopeActive() {
					caught = err
				}
				return false
			}
			return true
		})
		return caught
	case '{':
		var caught error
		s.objectIter(func(key []byte, valueStart, valueEnd int) bool {
			var err error
			if node.child != nil {
				scope := state.chainedIndexScope(input)
				err = state.withIndexScope(scope, func(state execState) error {
					return execMulti(state, node.child, input[valueStart:valueEnd], buf, fn)
				})
			} else {
				err = fn(input[valueStart:valueEnd])
			}
			if err != nil {
				if _, ok := err.(*jsonError); ok {
					if !state.tryScopeActive() {
						return false
					}
					caught = err
				} else if isControlFlowError(err) {
					caught = err
				} else if state.optionalScopeActive() {
					caught = err
				}
				return false
			}
			return true
		})
		return caught
	default:
		if node.optional {
			return nil
		}
		return fmt.Errorf("Cannot iterate over %s (%s)", jsonTypeName(input), previewJSONValue(input))
	}
}

func execRecursiveDescent(input []byte, buf []byte, fn func([]byte) error) error {
	return execRecursiveValue(trimWhitespace(input), buf, fn)
}

func execRecursiveValue(value []byte, buf []byte, fn func([]byte) error) error {
	if err := fn(append(buf[:0], value...)); err != nil {
		return err
	}

	s := scanner{data: value}
	s.skipWhitespace()
	if s.pos >= len(s.data) {
		return nil
	}

	switch s.data[s.pos] {
	case '[':
		var walkErr error
		s.arrayIter(func(_ int, elemStart, elemEnd int) bool {
			walkErr = execRecursiveValue(value[elemStart:elemEnd], buf, fn)
			return walkErr == nil
		})
		return walkErr
	case '{':
		var walkErr error
		s.objectIter(func(_ []byte, valueStart, valueEnd int) bool {
			walkErr = execRecursiveValue(value[valueStart:valueEnd], buf, fn)
			return walkErr == nil
		})
		return walkErr
	default:
		return nil
	}
}

func execRecurse(state execState, node *op, input []byte, buf []byte, fn func([]byte) error) error {
	return execRecurseWithDepth(state, node, input, buf, fn, 0)
}

func execRecurseWithDepth(state execState, node *op, input []byte, buf []byte, fn func([]byte) error, depth int) error {
	if depth > maxRecurseDepth {
		return errRecurseDepthLimit
	}
	current := trimWhitespace(input)
	if err := fn(append(buf[:0], current...)); err != nil {
		return err
	}
	if node.left == nil {
		return execRecurseDefaultChildren(state, node, current, buf, fn, depth+1)
	}
	nextVals, err := collectExecOutputs(state, node.left, current)
	if err != nil {
		return err
	}
	for _, next := range nextVals {
		next = trimWhitespace(next)
		if node.child != nil {
			cond, err := execSingle(state, node.child, next, nil)
			if err != nil {
				return err
			}
			if isFalsy(cond) {
				continue
			}
		}
		if bytes.Equal(next, current) {
			return errRecurseCycle
		}
		if err := execRecurseWithDepth(state, node, next, buf, fn, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func execRecurseDefaultChildren(state execState, node *op, current []byte, buf []byte, fn func([]byte) error, depth int) error {
	s := scanner{data: current}
	s.skipWhitespace()
	if s.pos >= len(s.data) {
		return nil
	}
	switch s.data[s.pos] {
	case '[':
		var walkErr error
		s.arrayIter(func(_ int, elemStart, elemEnd int) bool {
			walkErr = execRecurseWithDepth(state, node, current[elemStart:elemEnd], buf, fn, depth)
			return walkErr == nil
		})
		return walkErr
	case '{':
		var walkErr error
		s.objectIter(func(_ []byte, valueStart, valueEnd int) bool {
			walkErr = execRecurseWithDepth(state, node, current[valueStart:valueEnd], buf, fn, depth)
			return walkErr == nil
		})
		return walkErr
	default:
		return nil
	}
}

func execWalk(state execState, node *op, input []byte, buf []byte, fn func([]byte) error) error {
	outputs, err := walkOutputs(state, node.child, trimWhitespace(input))
	if err != nil {
		return err
	}
	for _, out := range outputs {
		if err := fn(append(buf[:0], out...)); err != nil {
			return err
		}
	}
	return nil
}

func walkOutputs(state execState, filter *op, value []byte) ([][]byte, error) {
	rebuilt, err := walkRebuild(state, filter, trimWhitespace(value))
	if err != nil {
		return nil, err
	}
	return collectExecOutputs(state, filter, rebuilt)
}

func walkFirstOutput(state execState, filter *op, value []byte) ([]byte, bool, error) {
	outputs, err := walkOutputs(state, filter, value)
	if err != nil {
		return nil, false, err
	}
	if len(outputs) == 0 {
		return nil, false, nil
	}
	return outputs[0], true, nil
}

func walkRebuild(state execState, filter *op, value []byte) ([]byte, error) {
	s := scanner{data: value}
	s.skipWhitespace()
	if s.pos >= len(s.data) {
		return cloneExecBytes(value), nil
	}
	switch s.data[s.pos] {
	case '[':
		return walkRebuildArray(state, filter, value)
	case '{':
		return walkRebuildObject(state, filter, value)
	default:
		return cloneExecBytes(value), nil
	}
}

func walkRebuildArray(state execState, filter *op, value []byte) ([]byte, error) {
	s := scanner{data: value}
	s.skipWhitespace()
	out := make([]byte, 0, len(value))
	out = append(out, '[')
	first := true
	var walkErr error
	s.arrayIter(func(_ int, elemStart, elemEnd int) bool {
		child, keep, err := walkFirstOutput(state, filter, value[elemStart:elemEnd])
		if err != nil {
			walkErr = err
			return false
		}
		if !keep {
			return true
		}
		if !first {
			out = append(out, ',')
		}
		first = false
		out = append(out, child...)
		return true
	})
	if walkErr != nil {
		return nil, walkErr
	}
	out = append(out, ']')
	return out, nil
}

func walkRebuildObject(state execState, filter *op, value []byte) ([]byte, error) {
	s := scanner{data: value}
	s.skipWhitespace()
	out := make([]byte, 0, len(value))
	out = append(out, '{')
	first := true
	var walkErr error
	s.objectIter(func(key []byte, valueStart, valueEnd int) bool {
		child, keep, err := walkFirstOutput(state, filter, value[valueStart:valueEnd])
		if err != nil {
			walkErr = err
			return false
		}
		if !keep {
			return true
		}
		if !first {
			out = append(out, ',')
		}
		first = false
		out = append(out, '"')
		out = appendJSONStringContent(out, key)
		out = append(out, '"', ':')
		out = append(out, child...)
		return true
	})
	if walkErr != nil {
		return nil, walkErr
	}
	out = append(out, '}')
	return out, nil
}

// execDelete removes specified fields from a JSON object or elements from an array.
// Never copies commas from input — reconstructs with our own commas.
func execDelete(state execState, node *op, input []byte, buf []byte) ([]byte, error) {
	if deleteNeedsGenericPaths(node) {
		return execDeleteGeneric(state, node, input, buf)
	}
	s := &scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) {
		return nil, fmt.Errorf("expected object or array for del()")
	}

	switch s.data[s.pos] {
	case '{':
		return execDeleteObject(state, node, input, buf, s)
	case '[':
		return execDeleteArray(state, node, input, buf, s)
	default:
		return nil, fmt.Errorf("expected object or array for del()")
	}
}

func deleteNeedsGenericPaths(node *op) bool {
	for i := range node.fields {
		if deletePathNeedsGeneric(&node.fields[i]) {
			return true
		}
	}
	return false
}

func deletePathNeedsGeneric(node *op) bool {
	if node == nil {
		return false
	}
	switch node.typ {
	case opField, opIndex, opSlice:
		return deletePathNeedsGeneric(node.child)
	default:
		return true
	}
}

func execDeleteGeneric(state execState, node *op, input []byte, buf []byte) ([]byte, error) {
	mutations := make([]pathMutation, 0, len(node.fields))
	for i := range node.fields {
		paths, err := collectAssignPaths(state, &node.fields[i], input)
		if err != nil {
			return nil, err
		}
		for _, steps := range paths {
			if len(steps) == 0 {
				if buf == nil {
					return bNull, nil
				}
				return append(buf[:0], "null"...), nil
			}
			mutations = append(mutations, pathMutation{steps: steps, order: len(mutations), delete: true})
		}
	}
	if len(mutations) == 0 {
		return normalizeOutputValue(input, buf), nil
	}
	sortPathMutations(mutations)
	current := normalizeOutputValue(input, nil)
	var err error
	for _, mutation := range mutations {
		current, err = deletePathDecoded(current, mutation.steps)
		if err != nil {
			return nil, err
		}
	}
	return normalizeOutputValue(current, buf), nil
}

// execDeleteObject removes specified fields from a JSON object.
func execDeleteObject(state execState, node *op, input []byte, buf []byte, s *scanner) ([]byte, error) {
	// Validate all del() arguments are field accesses
	for i := range node.fields {
		if node.fields[i].typ != opField {
			return nil, fmt.Errorf("del() argument must be a field access for object input")
		}
	}

	buf = append(buf, '{')
	first := true

	s.objectIter(func(key []byte, valueStart, valueEnd int) bool {
		var nestedFields []op
		for i := range node.fields {
			d := &node.fields[i]
			if !bytesEqualStr(key, d.field) {
				continue
			}
			if d.child == nil {
				return true // simple delete wins over any nested delete for this key
			}
			nestedFields = append(nestedFields, *d.child)
		}
		if len(nestedFields) > 0 {
			// Nested delete — recurse into the value.
			// Save the slice header before the call so we can recover if
			// the nested target is not an object/array (execDelete returns nil).
			if !first {
				buf = append(buf, ',')
			}
			first = false
			buf = append(buf, '"')
			buf = append(buf, key...)
			buf = append(buf, '"', ':')
			preDel := buf // full slice header — survives buf=nil from error
			nestedNode := &op{typ: opDelete, fields: nestedFields}
			var err error
			buf, err = execDelete(state, nestedNode, input[valueStart:valueEnd], buf)
			if err != nil {
				// Nested target is not an object/array — keep original value.
				buf = append(preDel, input[valueStart:valueEnd]...)
			}
			return true
		}

		// Keep this pair — copy verbatim
		if !first {
			buf = append(buf, ',')
		}
		first = false
		buf = append(buf, '"')
		buf = append(buf, key...)
		buf = append(buf, '"', ':')
		buf = append(buf, input[valueStart:valueEnd]...)
		return true
	})

	buf = append(buf, '}')
	return buf, nil
}

// execDeleteArray removes specified elements from a JSON array.
func execDeleteArray(state execState, node *op, input []byte, buf []byte, s *scanner) ([]byte, error) {
	// Build set of indices to delete
	deleteSet := make(map[int]bool)
	length := s.arrayLen()

	for i := range node.fields {
		d := &node.fields[i]
		switch d.typ {
		case opIndex:
			idx := d.index
			if idx < 0 {
				idx = length + idx
			}
			if idx >= 0 && idx < length {
				deleteSet[idx] = true
			}
		case opSlice:
			start, end, err := resolveDelSliceBounds(state, d, input, length)
			if err != nil {
				return nil, err
			}
			for j := start; j < end; j++ {
				deleteSet[j] = true
			}
		default:
			return nil, fmt.Errorf("del() argument must be an index or slice access for array input")
		}
	}

	buf = append(buf, '[')
	first := true

	s.arrayIter(func(index int, elemStart, elemEnd int) bool {
		if deleteSet[index] {
			return true // skip this element
		}
		if !first {
			buf = append(buf, ',')
		}
		first = false
		buf = append(buf, input[elemStart:elemEnd]...)
		return true
	})

	buf = append(buf, ']')
	return buf, nil
}

// resolveDelSliceBounds evaluates the start/end bounds of an opSlice node
// against the given array length, applying the same clamping rules as execSlice.
func resolveDelSliceBounds(state execState, node *op, input []byte, length int) (start, end int, err error) {
	start = 0
	end = length
	if node.left != nil {
		sv, e := execSingle(state, node.left, input, nil)
		if e != nil {
			return 0, 0, e
		}
		f, ok := parseJSONFloat(sv)
		if !ok {
			return 0, 0, fmt.Errorf("slice index must be a number")
		}
		start = resolveSliceBound(f, length, false)
	}
	if node.right != nil {
		sv, e := execSingle(state, node.right, input, nil)
		if e != nil {
			return 0, 0, e
		}
		f, ok := parseJSONFloat(sv)
		if !ok {
			return 0, 0, fmt.Errorf("slice index must be a number")
		}
		end = resolveSliceBound(f, length, true)
	}
	if start > end {
		start = end
	}
	return start, end, nil
}

// execPipeMulti runs left, then feeds each result into right.
func execPipeMulti(state execState, node *op, input []byte, buf []byte, fn func([]byte) error) error {
	return execMulti(state, node.left, input, nil, func(intermediate []byte) error {
		return execMulti(state, node.right, intermediate, buf, fn)
	})
}

func execApplyMulti(state execState, node *op, input []byte, buf []byte, fn func([]byte) error) error {
	scope := state.chainedIndexScope(input)
	return execMulti(state, node.left, input, nil, func(intermediate []byte) error {
		return state.withIndexScope(scope, func(state execState) error {
			return execMulti(state, node.right, intermediate, buf, fn)
		})
	})
}

func objectKeyStringValue(value []byte) (string, error) {
	value = trimWhitespace(value)
	if len(value) == 0 {
		return "", fmt.Errorf("expected object key expression")
	}
	vs := scanner{data: value}
	vs.skipWhitespace()
	if vs.pos >= len(vs.data) {
		return "", fmt.Errorf("expected object key expression")
	}
	if vs.data[vs.pos] != '"' {
		return "", fmt.Errorf("Cannot use %s (%s) as object key", jsonTypeName(value), string(value))
	}
	raw := vs.readString()
	var out []byte
	out = appendCanonicalRawJSONStringContent(out[:0], raw)
	return string(out), nil
}

func execConstructKey(state execState, p pair, input []byte) (string, error) {
	if p.keyExpr == nil {
		return p.key, nil
	}
	keyValue, err := execSingle(state, p.keyExpr, input, nil)
	if err != nil {
		return "", err
	}
	return objectKeyStringValue(keyValue)
}

// execConstruct builds a JSON object from key-expression pairs (single-output per pair).
// Used by the execSingle fast path via execFirstResult for common cases.
func execConstruct(state execState, node *op, input []byte, buf []byte) ([]byte, error) {
	buf = append(buf, '{')
	for i, p := range node.pairs {
		key, err := execConstructKey(state, p, input)
		if err != nil {
			return nil, err
		}
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = append(buf, '"')
		buf = append(buf, key...)
		buf = append(buf, '"', ':')
		val, err := exec(state, p.expr, input, buf[len(buf):len(buf):cap(buf)])
		if err != nil {
			var te *transparentError
			if errors.As(err, &te) {
				return nil, err
			}
			return nil, fmt.Errorf("in object construction for key %q: %w", key, err)
		}
		buf = append(buf, normalizeNaNInf(val)...)
	}
	buf = append(buf, '}')
	return buf, nil
}

// execConstructMulti builds JSON objects from key-expression pairs where any pair value
// may produce multiple outputs. Produces the Cartesian product across all pair values:
// {a: .x[], b: .y[]} emits one object per combination of .x[] and .y[] values.
//
// Allocation model: each output combination requires its own prefix copy —
// allocations are proportional to output count (Tier 2).
//
// Design note: fn is intentionally NOT passed into the recursive collectPairCombos
// helper. The old constructPairsInto approach captured fn in a closure passed to
// execMulti, which created a recursive escape analysis cycle causing Go's escape
// analysis to mark execMulti's fn parameter as always escaping to the heap. This
// cascaded to all callers of execMulti (execFirstResult, execIterator, etc.),
// introducing 3 allocs/op for every Tier 0 operation. By collecting combinations
// first and then calling fn, fn stays out of the recursive machinery entirely.
func execConstructMulti(state execState, node *op, input []byte, buf []byte, fn func([]byte) error) error {
	var combos [][]byte
	if err := collectPairCombos(state, node.pairs, 0, input, append(buf[:0], '{'), &combos); err != nil {
		return err
	}
	for _, combo := range combos {
		if err := fn(combo); err != nil {
			return err
		}
	}
	return nil
}

// collectPairCombos recursively collects all Cartesian product combinations of pair
// values into out. It does NOT take fn so that fn from execConstructMulti stays out
// of any closure captured by execMulti, preventing the escape analysis contamination
// described in execConstructMulti's comment.
func collectPairCombos(state execState, pairs []pair, idx int, input []byte, prefix []byte, out *[][]byte) error {
	if idx == len(pairs) {
		*out = append(*out, append(prefix, '}'))
		return nil
	}

	p := pairs[idx]
	isFirst := prefix[len(prefix)-1] == '{'
	emitWithKey := func(key string) error {
		return execMulti(state, p.expr, input, nil, func(val []byte) error {
			// Build this pair's key:value, branching independently for each val.
			var obj []byte
			obj = append(obj, prefix...)
			if !isFirst {
				obj = append(obj, ',')
			}
			obj = append(obj, '"')
			obj = append(obj, key...)
			obj = append(obj, '"', ':')
			obj = append(obj, normalizeNaNInf(val)...)
			return collectPairCombos(state, pairs, idx+1, input, obj, out)
		})
	}
	if p.keyExpr == nil {
		return emitWithKey(p.key)
	}
	return execMulti(state, p.keyExpr, input, nil, func(keyValue []byte) error {
		key, err := objectKeyStringValue(keyValue)
		if err != nil {
			return err
		}
		return emitWithKey(key)
	})
}

// execArrayConstruct builds a JSON array from expressions.
// Each element expression may produce multiple outputs (e.g. .items[]),
// all of which are collected into the array.
//
// Allocation note: nil scratch is REQUIRED here. If we passed buf's spare
// capacity as scratch, an iterator element (e.g. .items[]) would call the
// callback multiple times. Callback #1 writes result1 to buf[k:], extends
// buf to cover it. Callback #2 would then write result2 ALSO starting at
// buf[k:], overwriting result1 before the comma is inserted. This aliasing
// cannot be avoided structurally.
//
// Consequence: element expressions that BUILD new data (object construction,
// arithmetic, string concatenation) allocate ~1 buffer per element. Elements
// that return INPUT sub-slices (field access, identity, comparisons) remain
// zero-alloc because they don't use the scratch at all.
func execArrayConstruct(state execState, node *op, input []byte, buf []byte) ([]byte, error) {
	buf = append(buf, '[')
	first := true
	for _, elem := range node.elems {
		err := execMulti(state, elem, input, nil, func(val []byte) error {
			if !first {
				buf = append(buf, ',')
			}
			first = false
			buf = append(buf, normalizeNaNInf(val)...)
			return nil
		})
		if err != nil {
			// Propagate jsonErrors unwrapped so try-catch receives the original value.
			if _, ok := err.(*jsonError); ok {
				return nil, err
			}
			var te *transparentError
			if errors.As(err, &te) {
				return nil, err
			}
			return nil, fmt.Errorf("in array construction: %w", err)
		}
	}
	buf = append(buf, ']')
	return buf, nil
}

// execTypeSingle returns the JSON type name as a single result (no callback).
func execTypeSingle(input []byte, buf []byte) ([]byte, error) {
	s := &scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) {
		return append(buf, `"null"`...), nil
	}
	switch s.data[s.pos] {
	case '{':
		return append(buf, `"object"`...), nil
	case '[':
		return append(buf, `"array"`...), nil
	case '"':
		return append(buf, `"string"`...), nil
	case 't', 'f':
		return append(buf, `"boolean"`...), nil
	case 'n':
		return append(buf, `"null"`...), nil
	default:
		return append(buf, `"number"`...), nil
	}
}

// execCompareSingle evaluates a comparison without callbacks (zero-alloc path).
func execCompareSingle(state execState, node *op, input []byte, buf []byte) ([]byte, error) {
	leftVal, err := execSingle(state, node.left, input, buf)
	if err != nil {
		return nil, err
	}
	// When buf is nil, leftVal is a cap-limited sub-slice (no spare capacity).
	// Use nil scratch for the right side to avoid writing into input's backing array.
	var rightBuf []byte
	if buf != nil {
		rightBuf = leftVal[len(leftVal):len(leftVal):cap(leftVal)]
	}
	rightVal, err := execSingle(state, node.right, input, rightBuf)
	if err != nil {
		return nil, err
	}
	if evalCmpOp(node.cmpOp, leftVal, rightVal) {
		if buf == nil {
			return bTrue, nil
		}
		return append(buf[:0], "true"...), nil
	}
	if buf == nil {
		return bFalse, nil
	}
	return append(buf[:0], "false"...), nil
}

// execType returns the JSON type name of the input value.
func execType(input []byte, buf []byte, fn func([]byte) error) error {
	s := &scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) {
		return fn(append(buf, `"null"`...))
	}
	switch s.data[s.pos] {
	case '{':
		return fn(append(buf, `"object"`...))
	case '[':
		return fn(append(buf, `"array"`...))
	case '"':
		return fn(append(buf, `"string"`...))
	case 't', 'f':
		return fn(append(buf, `"boolean"`...))
	case 'n':
		return fn(append(buf, `"null"`...))
	default:
		// number
		return fn(append(buf, `"number"`...))
	}
}

// execCompare evaluates a comparison between two expressions.
// The left operand is evaluated via execMulti to support multi-output left sides
// (e.g. ".[] == 1" produces one boolean per element). For single-output right sides
// (the common case), execSingle is used directly to avoid fn being captured inside
// a nested execMulti call, which would create an escape cycle. For multi-output right
// sides (e.g. range(2) == range(2)), right values are collected first without fn,
// then fn is called from the left-side closure without any sub-execMulti nesting.
// For single-output operands the fast path is execCompareSingle via execSingle.
func execCompare(state execState, node *op, input []byte, buf []byte, fn func([]byte) error) error {
	if !hasMultiOutput(node.right) {
		// Single-output right side: evaluate once per left value with execSingle.
		return execMulti(state, node.left, input, nil, func(leftVal []byte) error {
			rightVal, err := execSingle(state, node.right, input, nil)
			if err != nil {
				return err
			}
			return fn(boolResult(buf, evalCmpOp(node.cmpOp, leftVal, rightVal)))
		})
	}
	// Multi-output right side: collect right values first (fn not captured here),
	// then iterate left, calling fn directly without any nested execMulti call.
	var rightVals [][]byte
	if err := execMulti(state, node.right, input, nil, func(rightVal []byte) error {
		rightVals = append(rightVals, rightVal)
		return nil
	}); err != nil {
		return err
	}
	return execMulti(state, node.left, input, nil, func(leftVal []byte) error {
		for _, rightVal := range rightVals {
			if err := fn(boolResult(buf, evalCmpOp(node.cmpOp, leftVal, rightVal))); err != nil {
				return err
			}
		}
		return nil
	})
}

// execAnd evaluates left and right; returns true only if both are truthy.
// Short-circuits: right is not evaluated if left is falsy.
func execAnd(state execState, node *op, input []byte, buf []byte, fn func([]byte) error) error {
	if hasMultiOutput(node.left) || hasMultiOutput(node.right) {
		if !hasMultiOutput(node.right) {
			return execMulti(state, node.left, input, nil, func(leftVal []byte) error {
				if isFalsy(leftVal) {
					return fn(append(buf[:0], "false"...))
				}
				rightVal, err := execSingle(state, node.right, input, nil)
				if err != nil {
					return err
				}
				if isFalsy(rightVal) {
					return fn(append(buf[:0], "false"...))
				}
				return fn(append(buf[:0], "true"...))
			})
		}
		var rightVals [][]byte
		if err := execMulti(state, node.right, input, nil, func(rightVal []byte) error {
			rightVals = append(rightVals, rightVal)
			return nil
		}); err != nil {
			return err
		}
		return execMulti(state, node.left, input, nil, func(leftVal []byte) error {
			for _, rightVal := range rightVals {
				if isFalsy(leftVal) || isFalsy(rightVal) {
					if err := fn(append(buf[:0], "false"...)); err != nil {
						return err
					}
					continue
				}
				if err := fn(append(buf[:0], "true"...)); err != nil {
					return err
				}
			}
			return nil
		})
	}
	leftVal, err := execSingle(state, node.left, input, buf)
	if err != nil {
		return err
	}
	if isFalsy(leftVal) {
		return fn(append(buf[:0], "false"...))
	}
	rightVal, err := execSingle(state, node.right, input, buf)
	if err != nil {
		return err
	}
	if isFalsy(rightVal) {
		return fn(append(buf[:0], "false"...))
	}
	return fn(append(buf[:0], "true"...))
}

// execOr evaluates left or right; returns true if either is truthy.
// Short-circuits: right is not evaluated if left is truthy.
func execOr(state execState, node *op, input []byte, buf []byte, fn func([]byte) error) error {
	if hasMultiOutput(node.left) || hasMultiOutput(node.right) {
		if !hasMultiOutput(node.right) {
			return execMulti(state, node.left, input, nil, func(leftVal []byte) error {
				if !isFalsy(leftVal) {
					return fn(append(buf[:0], "true"...))
				}
				rightVal, err := execSingle(state, node.right, input, nil)
				if err != nil {
					return err
				}
				if !isFalsy(rightVal) {
					return fn(append(buf[:0], "true"...))
				}
				return fn(append(buf[:0], "false"...))
			})
		}
		var rightVals [][]byte
		if err := execMulti(state, node.right, input, nil, func(rightVal []byte) error {
			rightVals = append(rightVals, rightVal)
			return nil
		}); err != nil {
			return err
		}
		return execMulti(state, node.left, input, nil, func(leftVal []byte) error {
			for _, rightVal := range rightVals {
				if !isFalsy(leftVal) || !isFalsy(rightVal) {
					if err := fn(append(buf[:0], "true"...)); err != nil {
						return err
					}
					continue
				}
				if err := fn(append(buf[:0], "false"...)); err != nil {
					return err
				}
			}
			return nil
		})
	}
	leftVal, err := execSingle(state, node.left, input, buf)
	if err != nil {
		return err
	}
	if !isFalsy(leftVal) {
		return fn(append(buf[:0], "true"...))
	}
	rightVal, err := execSingle(state, node.right, input, buf)
	if err != nil {
		return err
	}
	if !isFalsy(rightVal) {
		return fn(append(buf[:0], "true"...))
	}
	return fn(append(buf[:0], "false"...))
}

// execSelect evaluates a condition and emits the input if truthy, nothing if falsy.
func execSelect(state execState, node *op, input []byte, buf []byte, fn func([]byte) error) error {
	if isSingleOutputOp(node.child) {
		condVal, err := execSingle(state, node.child, input, nil)
		if err != nil {
			return err
		}
		if isFalsy(condVal) {
			return nil
		}
		return fn(input)
	}
	return execMulti(state, node.child, input, nil, func(condVal []byte) error {
		if isFalsy(condVal) {
			return nil
		}
		return fn(input)
	})
}

// execLength returns the length of a JSON value:
// string → number of bytes between quotes, array → element count,
// object → key count, null → 0.
func execLength(input []byte, buf []byte, fn func([]byte) error) error {
	n, err := execLengthSingle(input, buf)
	if err != nil {
		return err
	}
	return fn(n)
}

func execLengthSingle(input []byte, buf []byte) ([]byte, error) {
	s := &scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) {
		return appendInt(buf, 0), nil
	}
	switch s.data[s.pos] {
	case '"':
		// String length: count logical characters. Each escape sequence counts
		// as 1 character: \uXXXX (6 bytes) = 1, \n / \" / etc. (2 bytes) = 1.
		s.pos++ // skip opening quote
		n := 0
		for s.pos < len(s.data) {
			ch := s.data[s.pos]
			if ch == '\\' {
				s.pos++ // skip backslash
				if s.pos < len(s.data) && s.data[s.pos] == 'u' && s.pos+4 < len(s.data) {
					s.pos += 5 // skip u + 4 hex digits (\uXXXX)
				} else if s.pos < len(s.data) {
					s.pos++ // skip single escaped char
				}
				n++
				continue
			}
			if ch == '"' {
				break
			}
			s.pos++
			n++
		}
		return appendInt(buf, n), nil
	case '[':
		count := 0
		s.arrayIter(func(i, _, _ int) bool { count++; return true })
		return appendInt(buf, count), nil
	case '{':
		count := 0
		s.objectIter(func(_ []byte, _, _ int) bool { count++; return true })
		return appendInt(buf, count), nil
	case 'n': // null
		return appendInt(buf, 0), nil
	default:
		// Number: return absolute value (|n|), matching jq semantics.
		if isNumberByte(s.data[s.pos]) {
			f, ok := parseJSONFloat(input)
			if ok {
				abs := math.Abs(f)
				if abs == math.Trunc(abs) && !math.IsInf(abs, 0) && !math.IsNaN(abs) {
					return appendInt(buf, int(abs)), nil
				}
				return strconv.AppendFloat(buf, abs, 'g', -1, 64), nil
			}
		}
		return appendInt(buf, 0), nil
	}
}

// appendInt appends an integer (positive, negative, or zero) to buf without allocation.
func appendInt(buf []byte, n int) []byte {
	if n == 0 {
		return append(buf, '0')
	}
	if n < 0 {
		buf = append(buf, '-')
		n = -n
	}
	// Write digits in reverse then flip.
	start := len(buf)
	for n > 0 {
		buf = append(buf, byte('0'+n%10))
		n /= 10
	}
	// Reverse the digits we just appended.
	end := len(buf) - 1
	for i := start; i < end; i, end = i+1, end-1 {
		buf[i], buf[end] = buf[end], buf[i]
	}
	return buf
}

// appendNumber formats a float64 as compact JSON: integer form when the value is whole,
// otherwise strconv.AppendFloat with shortest representation.
func appendNumber(buf []byte, f float64) []byte {
	if math.IsNaN(f) {
		return append(buf, "NaN"...)
	}
	if math.IsInf(f, 1) {
		return append(buf, "infinite"...)
	}
	if math.IsInf(f, -1) {
		return append(buf, "-infinite"...)
	}
	if f == float64(int64(f)) && f >= -1e15 && f <= 1e15 {
		return appendInt(buf, int(f))
	}
	return strconv.AppendFloat(buf, f, 'f', -1, 64)
}

// parseEntryKeyValue scans a single {key/name, value} entry object and returns
// the unquoted key string and the value bounds within elem. Used by execFromEntries.
func parseEntryKeyValue(elem []byte) (keyContent []byte, valStart, valEnd int) {
	valStart, valEnd = -1, -1
	s := scanner{data: elem}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '{' {
		return
	}
	s.pos++ // skip '{'
	s.skipWhitespace()
	for s.pos < len(s.data) && s.data[s.pos] != '}' {
		s.skipWhitespace()
		if s.pos >= len(s.data) || s.data[s.pos] != '"' {
			break
		}
		k := s.readString()
		s.skipWhitespace()
		if s.pos < len(s.data) && s.data[s.pos] == ':' {
			s.pos++
		}
		s.skipWhitespace()
		vs := s.pos
		s.skipValue()
		ve := s.pos

		// jq accepts "key", "Key", "name", "Name" as the key field,
		// and "value", "Value" as the value field.
		if bytesEqualStr(k, "key") || bytesEqualStr(k, "Key") ||
			bytesEqualStr(k, "name") || bytesEqualStr(k, "Name") {
			inner := scanner{data: elem[vs:ve]}
			inner.skipWhitespace()
			if inner.pos < len(inner.data) && inner.data[inner.pos] == '"' {
				keyContent = inner.readString()
			}
		} else if bytesEqualStr(k, "value") || bytesEqualStr(k, "Value") {
			valStart, valEnd = vs, ve
		}
		s.skipWhitespace()
		if s.pos < len(s.data) && s.data[s.pos] == ',' {
			s.pos++
		} else {
			break
		}
	}
	return
}

// appendKV appends a {"key":"k","value":v} entry to buf. Used by execToEntries.
func appendKV(buf []byte, key []byte, value []byte) []byte {
	buf = append(buf, `{"key":"`...)
	buf = append(buf, key...)
	buf = append(buf, `","value":`...)
	buf = append(buf, value...)
	return append(buf, '}')
}

// execToEntries converts a JSON object to [{key, value}] array.
// Non-object input produces an empty array.
func execToEntries(input []byte, buf []byte) ([]byte, error) {
	s := &scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '{' {
		return append(buf, "[]"...), nil
	}
	buf = append(buf, '[')
	first := true
	s.objectIter(func(key []byte, valueStart, valueEnd int) bool {
		if !first {
			buf = append(buf, ',')
		}
		first = false
		buf = appendKV(buf, key, input[valueStart:valueEnd])
		return true
	})
	buf = append(buf, ']')
	return buf, nil
}

// execFromEntries converts a [{key,value}] array back to a JSON object.
// Each entry may use "key" or "name" as the key field.
// Non-array input produces an empty object.
func execFromEntries(input []byte, buf []byte) ([]byte, error) {
	s := &scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '[' {
		return append(buf, "{}"...), nil
	}
	buf = append(buf, '{')
	first := true
	s.arrayIter(func(_ int, elemStart, elemEnd int) bool {
		keyContent, valStart, valEnd := parseEntryKeyValue(input[elemStart:elemEnd])
		if keyContent == nil || valStart == -1 {
			return true // skip malformed entry
		}
		if !first {
			buf = append(buf, ',')
		}
		first = false
		buf = append(buf, '"')
		buf = append(buf, keyContent...)
		buf = append(buf, '"', ':')
		buf = append(buf, input[elemStart:elemEnd][valStart:valEnd]...)
		return true
	})
	buf = append(buf, '}')
	return buf, nil
}

// execLast runs the child expression to completion and emits only the last result.
func execLast(state execState, node *op, input []byte, buf []byte, fn func([]byte) error) error {
	result, err := execLastSingle(state, node, input, buf)
	if err != nil {
		return err
	}
	if result == nil {
		return nil // no outputs
	}
	return fn(result)
}

func execLastSingle(state execState, node *op, input []byte, buf []byte) ([]byte, error) {
	// Keep a reference to the last result — no copy needed.
	// Results from execMulti with nil buf are either sub-slices of input
	// (safe for lifetime of this call) or global literals (bTrue/bFalse).
	var lastResult []byte
	err := execMulti(state, node.child, input, nil, func(result []byte) error {
		lastResult = result
		return nil
	})
	if err != nil {
		return nil, err
	}
	if lastResult == nil {
		return nil, nil
	}
	if buf == nil {
		return lastResult, nil
	}
	return append(buf, lastResult...), nil
}

// execLimit emits at most n results from the child expression.
// n is evaluated from node.left; the generator is node.child.
func execLimit(state execState, node *op, input []byte, buf []byte, fn func([]byte) error) error {
	nVal, err := execSingle(state, node.left, input, nil)
	if err != nil {
		return err
	}
	nf, ok := parseJSONFloat(nVal)
	if !ok {
		return fmt.Errorf("limit: count must be a number")
	}
	n := int(nf)
	if n < 0 {
		return fmt.Errorf("limit doesn't support negative count")
	}
	if n == 0 {
		return nil
	}
	count := 0
	err = execMulti(state, node.child, input, buf, func(result []byte) error {
		if err := fn(result); err != nil {
			return err
		}
		count++
		if count >= n {
			return errBreak
		}
		return nil
	})
	if err == errBreak {
		return nil
	}
	return err
}

func execSkip(state execState, node *op, input []byte, buf []byte, fn func([]byte) error) error {
	return execMulti(state, node.left, input, nil, func(countVal []byte) error {
		nf, ok := parseJSONFloat(countVal)
		if !ok {
			return fmt.Errorf("skip: count must be a number")
		}
		n := int(nf)
		if n < 0 {
			return fmt.Errorf("skip doesn't support negative count")
		}
		skipped := 0
		return execMulti(state, node.child, input, buf, func(result []byte) error {
			if skipped < n {
				skipped++
				return nil
			}
			return fn(result)
		})
	})
}

// execAdd reduces an array by summing numbers, concatenating strings/arrays,
// or merging objects. Returns null for empty/null input.
func execAdd(input []byte, buf []byte, fn func([]byte) error) error {
	s := &scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '[' {
		return fn(append(buf, "null"...))
	}

	// Determine element type from first non-null element.
	elemType := byte(0)
	s2 := scanner{data: input}
	s2.arrayIter(func(_ int, start, end int) bool {
		es := scanner{data: input[start:end]}
		es.skipWhitespace()
		if es.pos < len(es.data) && es.data[es.pos] != 'n' {
			elemType = es.data[es.pos]
			return false
		}
		return true
	})
	if elemType == 0 {
		return fn(append(buf, "null"...)) // empty or all-null
	}

	switch elemType {
	case '"': // string concatenation
		buf = append(buf, '"')
		s.arrayIter(func(_ int, start, end int) bool {
			es := scanner{data: input[start:end]}
			es.skipWhitespace()
			if es.pos < len(es.data) && es.data[es.pos] == '"' {
				buf = append(buf, es.readString()...)
			}
			return true
		})
		buf = append(buf, '"')
	case '[': // array concatenation
		buf = append(buf, '[')
		first := true
		s.arrayIter(func(_ int, outerStart, outerEnd int) bool {
			inner := scanner{data: input[outerStart:outerEnd]}
			inner.skipWhitespace()
			if inner.pos < len(inner.data) && inner.data[inner.pos] == '[' {
				inner.arrayIter(func(_ int, iStart, iEnd int) bool {
					if !first {
						buf = append(buf, ',')
					}
					first = false
					buf = append(buf, input[outerStart:outerEnd][iStart:iEnd]...)
					return true
				})
			}
			return true
		})
		buf = append(buf, ']')
	case '{': // object merge (last-wins, first-occurrence key order)
		// Collect all object byte slices (allocates a small slice of pointers).
		var objects [][]byte
		s.arrayIter(func(_ int, oStart, oEnd int) bool {
			es := scanner{data: input[oStart:oEnd]}
			es.skipWhitespace()
			if es.pos < len(es.data) && es.data[es.pos] == '{' {
				objects = append(objects, input[oStart:oEnd])
			}
			return true
		})
		buf = append(buf, '{')
		first := true
		// Iterate keys in first-occurrence order: for each key in objects[0], then
		// keys in objects[1] not already in objects[0], etc. For each key, emit the
		// value from the LAST object that contains it.
		for i, obj := range objects {
			os := scanner{data: obj}
			os.objectIter(func(key []byte, vStart, vEnd int) bool {
				// Only emit from first occurrence of this key
				for j := 0; j < i; j++ {
					if objectContainsKey(objects[j], key) {
						return true // already emitted from an earlier object
					}
				}
				// Find the LAST value for this key across all objects
				lastVal := obj[vStart:vEnd]
				for j := i + 1; j < len(objects); j++ {
					lvs, lve := func() (int, int) {
						ls := scanner{data: objects[j]}
						ls.skipWhitespace()
						return ls.findField(key)
					}()
					if lvs != -1 {
						lastVal = objects[j][lvs:lve]
					}
				}
				if !first {
					buf = append(buf, ',')
				}
				first = false
				buf = append(buf, '"')
				buf = append(buf, key...)
				buf = append(buf, '"', ':')
				buf = append(buf, lastVal...)
				return true
			})
		}
		buf = append(buf, '}')
	default: // numeric sum
		sum := 0.0
		s.arrayIter(func(_ int, start, end int) bool {
			f, ok := parseJSONFloat(input[start:end])
			if ok {
				sum += f
			}
			return true
		})
		buf = appendNumber(buf, sum)
	}
	return fn(buf)
}

// execFlattenInto flattens a nested array into a single-level array.
// node.index = -1 means unlimited depth; >= 0 means flatten that many levels.
func execFlattenInto(state execState, input []byte, buf []byte, node *op) ([]byte, error) {
	maxDepth := node.index
	if node.child != nil {
		// flatten(n) — evaluate n
		depthVal, err := execSingle(state, node.child, input, nil)
		if err != nil {
			return nil, err
		}
		d, ok := parseJSONFloat(depthVal)
		if !ok {
			return nil, fmt.Errorf("flatten: depth must be a number")
		}
		if d < 0 {
			return nil, fmt.Errorf("flatten depth must not be negative")
		}
		maxDepth = int(d)
	}
	buf = append(buf, '[')
	first := true
	flattenLevel(input, &buf, &first, maxDepth, 0)
	buf = append(buf, ']')
	return buf, nil
}

func flattenLevel(input []byte, buf *[]byte, first *bool, maxDepth, curDepth int) {
	s := scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '[' {
		// Not an array — emit as-is
		if !*first {
			*buf = append(*buf, ',')
		}
		*first = false
		*buf = append(*buf, input...)
		return
	}
	if maxDepth >= 0 && curDepth > maxDepth {
		// Exceeded max depth — emit the array as a single element unchanged.
		if !*first {
			*buf = append(*buf, ',')
		}
		*first = false
		*buf = append(*buf, input...)
		return
	}
	s.arrayIter(func(_ int, start, end int) bool {
		elem := input[start:end]
		es := scanner{data: elem}
		es.skipWhitespace()
		if es.pos < len(es.data) && es.data[es.pos] == '[' {
			flattenLevel(elem, buf, first, maxDepth, curDepth+1)
		} else {
			if !*first {
				*buf = append(*buf, ',')
			}
			*first = false
			*buf = append(*buf, elem...)
		}
		return true
	})
}

// execSplit splits a JSON string by a literal separator.
// Returns a JSON array of strings. Non-string input returns null.
func execSplit(input []byte, buf []byte, sep string) []byte {
	s := &scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '"' {
		return append(buf, "null"...)
	}
	content := s.readString() // raw bytes between quotes

	buf = append(buf, '[')
	first := true
	start := 0
	slen := len(sep)
	for i := 0; i <= len(content)-slen; {
		if bytesEqualStr(content[i:i+slen], sep) {
			if !first {
				buf = append(buf, ',')
			}
			first = false
			buf = append(buf, '"')
			buf = append(buf, content[start:i]...)
			buf = append(buf, '"')
			start = i + slen
			i = start
		} else {
			i++
		}
	}
	// Last segment
	if !first {
		buf = append(buf, ',')
	}
	buf = append(buf, '"')
	buf = append(buf, content[start:]...)
	buf = append(buf, '"')
	buf = append(buf, ']')
	return buf
}

// execJoin joins a JSON array of strings/numbers/nulls with a separator.
// Returns a JSON string. Non-array input returns null.
// Errors on objects or arrays in the input.
func execJoin(input []byte, buf []byte, sep string) ([]byte, error) {
	s := &scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '[' {
		return append(buf, "null"...), nil
	}
	buf = append(buf, '"')
	first := true
	var joinErr error
	s.arrayIter(func(_ int, start, end int) bool {
		elem := scanner{data: input[start:end]}
		elem.skipWhitespace()
		if elem.pos >= len(elem.data) {
			return true
		}
		switch elem.data[elem.pos] {
		case '{', '[': // object or array — error
			elemType := "object"
			if elem.data[elem.pos] == '[' {
				elemType = "array"
			}
			// Build accumulated string content for error: buf[1:] (skip leading '"') + pending sep
			accContent := string(buf[1:])
			if !first {
				accContent += sep
			}
			rawElem := string(input[start:end])
			// jq truncates element representation to 14 chars total (11 + "...")
			if len(rawElem) > 14 {
				rawElem = rawElem[:11] + "..."
			}
			joinErr = fmt.Errorf("string (%q) and %s (%s) cannot be added", accContent, elemType, rawElem)
			return false // stop iteration
		}
		// Add separator between elements
		if !first {
			buf = append(buf, sep...)
		}
		first = false
		switch elem.data[elem.pos] {
		case '"': // string: append unquoted content
			buf = append(buf, elem.readString()...)
		case 'n': // null: empty string
		default: // number/bool: append raw bytes
			buf = append(buf, input[start:end]...)
		}
		return true
	})
	if joinErr != nil {
		return nil, joinErr
	}
	buf = append(buf, '"')
	return buf, nil
}

func execStrftimeBuiltin(state execState, node *op, input []byte, buf []byte, local bool, fn func([]byte) error) error {
	name := "strftime/1"
	loc := time.UTC
	if local {
		name = "strflocaltime/1"
		loc = time.Local
	}
	tm, err := decodeTimeValue(input, loc, true)
	if err != nil {
		return &transparentError{err: fmt.Errorf("%s requires parsed datetime inputs", name)}
	}
	formats, err := collectExecOutputs(state, node.child, input)
	if err != nil {
		return err
	}
	for _, formatVal := range formats {
		format, err := decodeJSONStringValue(formatVal)
		if err != nil {
			return &transparentError{err: fmt.Errorf("%s requires a string format", name)}
		}
		layout, err := jqTimeLayout(format)
		if err != nil {
			return err
		}
		out := append(buf[:0], '"')
		out = appendJSONStringContent(out, []byte(tm.In(loc).Format(layout)))
		out = append(out, '"')
		if err := fn(out); err != nil {
			return err
		}
	}
	return nil
}

func execStrptimeBuiltin(state execState, node *op, input []byte, buf []byte, fn func([]byte) error) error {
	value, err := decodeJSONStringValue(input)
	if err != nil {
		return &transparentError{err: fmt.Errorf("strptime/1 requires string inputs")}
	}
	formats, err := collectExecOutputs(state, node.child, input)
	if err != nil {
		return err
	}
	for _, formatVal := range formats {
		format, err := decodeJSONStringValue(formatVal)
		if err != nil {
			return &transparentError{err: fmt.Errorf("strptime/1 requires a string format")}
		}
		layout, err := jqTimeLayout(format)
		if err != nil {
			return err
		}
		tm, err := time.Parse(layout, value)
		if err != nil {
			return err
		}
		out, err := appendParsedTimeArray(buf[:0], tm.UTC())
		if err != nil {
			return err
		}
		if err := fn(out); err != nil {
			return err
		}
	}
	return nil
}

func execMktime(input []byte, buf []byte) ([]byte, error) {
	tm, err := decodeParsedTimeArray(input, time.UTC)
	if err != nil {
		return nil, &transparentError{err: fmt.Errorf("mktime requires parsed datetime inputs")}
	}
	return appendJSONFloat(buf, float64(tm.Unix())), nil
}

func execGmtime(input []byte, buf []byte) ([]byte, error) {
	tm, err := decodeTimeNumber(input, time.UTC)
	if err != nil {
		return nil, err
	}
	return appendParsedTimeArray(buf, tm.UTC())
}

func execFromdate(input []byte, buf []byte) ([]byte, error) {
	value, err := decodeJSONStringValue(input)
	if err != nil {
		return nil, &transparentError{err: fmt.Errorf("fromdate requires string inputs")}
	}
	tm, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, err
	}
	return appendJSONFloat(buf, float64(tm.Unix())), nil
}

func execTodate(input []byte, buf []byte) ([]byte, error) {
	tm, err := decodeTimeNumber(input, time.UTC)
	if err != nil {
		return nil, &transparentError{err: fmt.Errorf("todate requires numeric inputs")}
	}
	buf = append(buf, '"')
	buf = tm.UTC().AppendFormat(buf, time.RFC3339)
	buf = append(buf, '"')
	return buf, nil
}

func execNow(buf []byte) []byte {
	return appendJSONFloat(buf, float64(time.Now().UnixNano())/1e9)
}

func decodeTimeValue(input []byte, loc *time.Location, allowNumber bool) (time.Time, error) {
	if allowNumber {
		if tm, err := decodeTimeNumber(input, loc); err == nil {
			return tm, nil
		}
	}
	return decodeParsedTimeArray(input, loc)
}

func decodeTimeNumber(input []byte, loc *time.Location) (time.Time, error) {
	value := trimWhitespace(input)
	f, ok := parseJSONFloat(value)
	if !ok {
		return time.Time{}, fmt.Errorf("expected number")
	}
	return time.Unix(int64(f), 0).In(loc), nil
}

func decodeParsedTimeArray(input []byte, loc *time.Location) (time.Time, error) {
	value := trimWhitespace(input)
	s := scanner{data: value}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '[' {
		return time.Time{}, fmt.Errorf("expected parsed datetime array")
	}
	parts := make([]int, 0, 8)
	var parseErr error
	s.arrayIter(func(_ int, start, end int) bool {
		f, ok := parseJSONFloat(trimWhitespace(value[start:end]))
		if !ok {
			parseErr = fmt.Errorf("expected numeric datetime parts")
			return false
		}
		parts = append(parts, int(f))
		return true
	})
	if parseErr != nil {
		return time.Time{}, parseErr
	}
	if len(parts) < 3 {
		return time.Time{}, fmt.Errorf("expected parsed datetime array")
	}
	year, month0, day := parts[0], parts[1], parts[2]
	hour, minute, second := 0, 0, 0
	if len(parts) > 3 {
		hour = parts[3]
	}
	if len(parts) > 4 {
		minute = parts[4]
	}
	if len(parts) > 5 {
		second = parts[5]
	}
	return time.Date(year, time.Month(month0+1), day, hour, minute, second, 0, loc), nil
}

func appendParsedTimeArray(buf []byte, tm time.Time) ([]byte, error) {
	buf = append(buf, '[')
	parts := []int{
		tm.Year(),
		int(tm.Month()) - 1,
		tm.Day(),
		tm.Hour(),
		tm.Minute(),
		tm.Second(),
		int(tm.Weekday()),
		tm.YearDay() - 1,
	}
	for i, part := range parts {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = strconv.AppendInt(buf, int64(part), 10)
	}
	buf = append(buf, ']')
	return buf, nil
}

func decodeJSONStringValue(input []byte) (string, error) {
	value := trimWhitespace(input)
	s := scanner{data: value}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '"' {
		return "", fmt.Errorf("expected string")
	}
	return string(decodeJSONStringContent(nil, s.readString())), nil
}

func jqTimeLayout(format string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			b.WriteByte(format[i])
			continue
		}
		i++
		if i >= len(format) {
			return "", fmt.Errorf("invalid strftime format")
		}
		switch format[i] {
		case '%':
			b.WriteByte('%')
		case 'Y':
			b.WriteString("2006")
		case 'm':
			b.WriteString("01")
		case 'd':
			b.WriteString("02")
		case 'e':
			b.WriteString("_2")
		case 'H':
			b.WriteString("15")
		case 'M':
			b.WriteString("04")
		case 'S':
			b.WriteString("05")
		case 'A':
			b.WriteString("Monday")
		case 'B':
			b.WriteString("January")
		default:
			return "", fmt.Errorf("unsupported strftime format %%%c", format[i])
		}
	}
	return b.String(), nil
}

func execCombinations(state execState, node *op, input []byte, buf []byte, fn func([]byte) error) error {
	if node.child != nil {
		nVal, err := execSingle(state, node.child, input, nil)
		if err != nil {
			return err
		}
		f, ok := parseJSONFloat(trimWhitespace(nVal))
		if !ok {
			return fmt.Errorf("combinations/1 requires a numeric count")
		}
		n := int(f)
		if n < 0 {
			return nil
		}
		elems, err := decodeArrayElements(input)
		if err != nil {
			return err
		}
		current := make([][]byte, n)
		var emit func(int) error
		emit = func(depth int) error {
			if depth == n {
				return fn(appendJSONArrayValues(buf[:0], current))
			}
			for _, elem := range elems {
				current[depth] = elem
				if err := emit(depth + 1); err != nil {
					return err
				}
			}
			return nil
		}
		return emit(0)
	}

	outer, err := decodeArrayElements(input)
	if err != nil {
		return err
	}
	sources := make([][][]byte, len(outer))
	for i, elem := range outer {
		inner, err := decodeArrayElements(elem)
		if err != nil {
			return err
		}
		sources[i] = inner
	}
	current := make([][]byte, len(sources))
	var emit func(int) error
	emit = func(depth int) error {
		if depth == len(sources) {
			return fn(appendJSONArrayValues(buf[:0], current))
		}
		for _, elem := range sources[depth] {
			current[depth] = elem
			if err := emit(depth + 1); err != nil {
				return err
			}
		}
		return nil
	}
	return emit(0)
}

func appendJSONArrayValues(buf []byte, elems [][]byte) []byte {
	buf = append(buf, '[')
	for i, elem := range elems {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = append(buf, normalizeOutputValue(elem, nil)...)
	}
	buf = append(buf, ']')
	return buf
}

// execSlice implements .[n:m], .[:m], .[n:] on arrays and strings.
// node.left = start expr (nil = 0), node.right = end expr (nil = length).
// Negative indices count from the end. Zero-alloc: writes directly into buf.
func execSlice(state execState, node *op, input []byte, buf []byte) ([]byte, error) {
	s := &scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) {
		return append(buf, "null"...), nil
	}

	// Compute logical length for bound resolution
	var length int
	switch s.data[s.pos] {
	case '[':
		length = s.arrayLen()
	case '"':
		// Count logical characters (escape sequences = 1 each)
		p := s.pos + 1
		for p < len(s.data) {
			if s.data[p] == '"' {
				break
			}
			if s.data[p] == '\\' && p+1 < len(s.data) && s.data[p+1] == 'u' {
				p += 6
			} else if s.data[p] == '\\' {
				p += 2
			} else {
				_, size := utf8.DecodeRune(s.data[p:])
				if size < 1 {
					size = 1
				}
				p += size
			}
			length++
		}
	default:
		if node.optional && s.data[s.pos] != 'n' {
			return nil, nil
		}
		return append(buf, "null"...), nil
	}

	// Resolve start bound
	start := 0
	scopeInput := state.currentIndexScope()
	if scopeInput == nil {
		scopeInput = input
	}
	if node.left != nil {
		sv, err := execSingle(state, node.left, scopeInput, nil)
		if err != nil {
			return nil, err
		}
		f, ok := parseJSONFloat(sv)
		if !ok {
			return nil, fmt.Errorf("slice index must be a number")
		}
		start = resolveSliceBound(f, length, false)
	}

	// Resolve end bound
	end := length
	if node.right != nil {
		sv, err := execSingle(state, node.right, scopeInput, nil)
		if err != nil {
			return nil, err
		}
		f, ok := parseJSONFloat(sv)
		if !ok {
			return nil, fmt.Errorf("slice index must be a number")
		}
		end = resolveSliceBound(f, length, true)
	}
	if start > end {
		start = end
	}

	switch s.data[s.pos] {
	case '[':
		buf = append(buf, '[')
		first := true
		i := 0
		s.arrayIter(func(_ int, elemStart, elemEnd int) bool {
			if i >= start && i < end {
				if !first {
					buf = append(buf, ',')
				}
				first = false
				buf = append(buf, input[elemStart:elemEnd]...)
			}
			i++
			return true
		})
		buf = append(buf, ']')
	case '"':
		buf = append(buf, '"')
		s.pos++ // skip opening '"'
		i := 0
		for s.pos < len(s.data) && s.data[s.pos] != '"' {
			charStart := s.pos
			if s.data[s.pos] == '\\' && s.pos+1 < len(s.data) && s.data[s.pos+1] == 'u' {
				s.pos += 6
			} else if s.data[s.pos] == '\\' {
				s.pos += 2
			} else {
				_, size := utf8.DecodeRune(s.data[s.pos:])
				if size < 1 {
					size = 1
				}
				s.pos += size
			}
			// A truncated escape (\ or a partial \uXXXX) put pos past the end,
			// and unlike the scan loops this one slices as it goes.
			if s.pos > len(s.data) {
				break
			}
			if i >= start && i < end {
				buf = append(buf, input[charStart:s.pos]...)
			}
			i++
		}
		buf = append(buf, '"')
	}
	return buf, nil
}

func resolveSliceBound(f float64, length int, isEnd bool) int {
	if math.IsNaN(f) {
		if isEnd {
			return length
		}
		return 0
	}
	if isEnd {
		f = math.Ceil(f)
	} else {
		f = math.Floor(f)
	}
	idx := int(f)
	if idx < 0 {
		idx += length
	}
	if idx < 0 {
		return 0
	}
	if idx > length {
		return length
	}
	return idx
}

func execNeg(state execState, node *op, input []byte, buf []byte, fn func([]byte) error) error {
	return execMulti(state, node.child, input, nil, func(value []byte) error {
		result, err := negateJSONValue(value, buf)
		if err != nil {
			return err
		}
		return fn(result)
	})
}

func execNegSingle(state execState, node *op, input []byte, buf []byte) ([]byte, error) {
	value, err := execSingle(state, node.child, input, nil)
	if err != nil {
		return nil, err
	}
	return negateJSONValue(value, buf)
}

func negateJSONValue(value []byte, buf []byte) ([]byte, error) {
	if f, ok := parseJSONFloat(value); ok {
		return appendNumber(buf, -f), nil
	}
	raw := previewJSONValue(value)
	return nil, fmt.Errorf("%s (%s) cannot be negated", jsonTypeName(value), raw)
}

func previewJSONValue(value []byte) string {
	raw := string(trimWhitespace(value))
	if len(raw) > 11 {
		raw = raw[:11]
		for len(raw) > 0 && !utf8.ValidString(raw) {
			raw = raw[:len(raw)-1]
		}
		raw += "..."
	}
	return raw
}

// execPlus implements expr + expr: null identity, string concat, array concat, numeric add.
// Uses nil scratch for operands so results are input sub-slices (zero-alloc).
func execPlus(state execState, node *op, input []byte, buf []byte, fn func([]byte) error) error {
	result, err := execPlusSingle(state, node, input, buf)
	if err != nil {
		return err
	}
	return fn(result)
}

func execPlusSingle(state execState, node *op, input []byte, buf []byte) ([]byte, error) {
	// Evaluate both operands with nil scratch — cap-limited sub-slices, no alloc.
	leftVal, err := execSingle(state, node.left, input, nil)
	if err != nil {
		return nil, err
	}
	rightVal, err := execSingle(state, node.right, input, nil)
	if err != nil {
		return nil, err
	}
	return execPlusValues(leftVal, rightVal, buf)
}

// execPlusValues computes addition on pre-evaluated values.
// Separated from execPlusSingle to support multi-output left sides (.[] + x etc.).
func execPlusValues(leftVal, rightVal, buf []byte) ([]byte, error) {
	ls := scanner{data: leftVal}
	ls.skipWhitespace()
	rs := scanner{data: rightVal}
	rs.skipWhitespace()

	// null is identity for +: null + x = x, x + null = x
	if ls.pos >= len(ls.data) || ls.data[ls.pos] == 'n' {
		if buf == nil {
			return rightVal, nil
		}
		return append(buf, rightVal...), nil
	}
	if rs.pos >= len(rs.data) || rs.data[rs.pos] == 'n' {
		if buf == nil {
			return leftVal, nil
		}
		return append(buf, leftVal...), nil
	}

	switch ls.data[ls.pos] {
	case '"': // string concatenation
		if rs.pos < len(rs.data) && rs.data[rs.pos] == '"' {
			lc := ls.readString()
			rc := rs.readString()
			buf = append(buf, '"')
			buf = appendCanonicalRawJSONStringContent(buf, lc)
			buf = appendCanonicalRawJSONStringContent(buf, rc)
			buf = append(buf, '"')
			return buf, nil
		}
	case '[': // array concatenation
		if rs.pos < len(rs.data) && rs.data[rs.pos] == '[' {
			buf = append(buf, '[')
			first := true
			ls.arrayIter(func(_ int, start, end int) bool {
				if !first {
					buf = append(buf, ',')
				}
				first = false
				buf = append(buf, leftVal[start:end]...)
				return true
			})
			rs.arrayIter(func(_ int, start, end int) bool {
				if !first {
					buf = append(buf, ',')
				}
				first = false
				buf = append(buf, rightVal[start:end]...)
				return true
			})
			buf = append(buf, ']')
			return buf, nil
		}
	case '{': // object merge (right wins for duplicate keys, left key order preserved)
		if rs.pos < len(rs.data) && rs.data[rs.pos] == '{' {
			buf = append(buf, '{')
			first := true
			// Emit ALL left keys — use right's value when it exists (right wins).
			// This preserves left-object key order even when right overrides a value.
			ls.objectIter(func(key []byte, vStart, vEnd int) bool {
				if !first {
					buf = append(buf, ',')
				}
				first = false
				buf = append(buf, '"')
				buf = append(buf, key...)
				buf = append(buf, '"', ':')
				// Look up this key in right; use its value if present.
				rs2 := scanner{data: rightVal}
				rs2.skipWhitespace()
				rvStart, rvEnd := rs2.findField(key)
				if rvStart != -1 {
					buf = append(buf, rightVal[rvStart:rvEnd]...)
				} else {
					buf = append(buf, leftVal[vStart:vEnd]...)
				}
				return true
			})
			// Emit right keys not already in left (new keys added by right)
			rs.objectIter(func(key []byte, vStart, vEnd int) bool {
				if objectContainsKey(leftVal, key) {
					return true // already emitted with right's value in the left pass
				}
				if !first {
					buf = append(buf, ',')
				}
				first = false
				buf = append(buf, '"')
				buf = append(buf, key...)
				buf = append(buf, '"', ':')
				buf = append(buf, rightVal[vStart:vEnd]...)
				return true
			})
			buf = append(buf, '}')
			return buf, nil
		}
	default: // numeric add
		lf, lok := parseJSONFloat(leftVal)
		rf, rok := parseJSONFloat(rightVal)
		if lok && rok {
			return appendNumber(buf, lf+rf), nil
		}
	}
	return nil, fmt.Errorf("cannot add %q and %q", leftVal, rightVal)
}

// base64Alphabet is the standard base64 encoding alphabet.
const base64Alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"

// execBase64Encode encodes a JSON string to base64.
// The JSON string is first decoded (escape sequences resolved) before encoding,
// matching jq's behaviour where "\n" becomes a newline byte in the base64 output.
func execBase64Encode(input []byte, buf []byte) ([]byte, error) {
	s := &scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '"' {
		return nil, fmt.Errorf("@base64 input must be a string")
	}
	raw := s.readString()                        // raw bytes between quotes
	content := decodeJSONStringContent(nil, raw) // resolve escape sequences

	buf = append(buf, '"')
	for i := 0; i < len(content); i += 3 {
		rem := len(content) - i
		b0 := content[i]
		var b1, b2 byte
		if rem > 1 {
			b1 = content[i+1]
		}
		if rem > 2 {
			b2 = content[i+2]
		}
		buf = append(buf, base64Alphabet[(b0>>2)&0x3F])
		buf = append(buf, base64Alphabet[((b0&0x03)<<4)|((b1>>4)&0x0F)])
		if rem > 1 {
			buf = append(buf, base64Alphabet[((b1&0x0F)<<2)|((b2>>6)&0x03)])
		} else {
			buf = append(buf, '=')
		}
		if rem > 2 {
			buf = append(buf, base64Alphabet[b2&0x3F])
		} else {
			buf = append(buf, '=')
		}
	}
	buf = append(buf, '"')
	return buf, nil
}

// base64DecodeChar maps a base64 character to its 6-bit value.
// Handles both standard (+/) and URL-safe (-_) variants.
func base64DecodeChar(ch byte) (byte, bool) {
	switch {
	case ch >= 'A' && ch <= 'Z':
		return ch - 'A', true
	case ch >= 'a' && ch <= 'z':
		return ch - 'a' + 26, true
	case ch >= '0' && ch <= '9':
		return ch - '0' + 52, true
	case ch == '+' || ch == '-':
		return 62, true
	case ch == '/' || ch == '_':
		return 63, true
	case ch == '=':
		return 0, true // padding
	default:
		return 0, false
	}
}

// appendJSONByte appends a single byte to buf with proper JSON string escaping.
func appendJSONByte(buf []byte, b byte) []byte {
	switch b {
	case '"':
		return append(buf, '\\', '"')
	case '\\':
		return append(buf, '\\', '\\')
	case '\n':
		return append(buf, '\\', 'n')
	case '\r':
		return append(buf, '\\', 'r')
	case '\t':
		return append(buf, '\\', 't')
	case '\b':
		return append(buf, '\\', 'b')
	case '\f':
		return append(buf, '\\', 'f')
	default:
		if b < 0x20 {
			// Other control character → \u00XX
			hi := b >> 4
			lo := b & 0x0F
			buf = append(buf, '\\', 'u', '0', '0')
			if hi < 10 {
				buf = append(buf, '0'+hi)
			} else {
				buf = append(buf, 'a'+hi-10)
			}
			if lo < 10 {
				buf = append(buf, '0'+lo)
			} else {
				buf = append(buf, 'a'+lo-10)
			}
			return buf
		}
		return append(buf, b)
	}
}

// execBase64Decode decodes a base64 JSON string back to a JSON string.
// Handles both standard (+/) and URL-safe (-_) base64, plus unpadded final groups.
func execBase64Decode(input []byte, buf []byte) ([]byte, error) {
	s := &scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '"' {
		return nil, fmt.Errorf("@base64d input must be a string")
	}
	content := decodeJSONStringContent(nil, s.readString())
	inputText := string(trimWhitespace(input))
	invalidBase64 := func() error {
		return fmt.Errorf("string (%s) is not valid base64 data", inputText)
	}
	trailingByte := func() error {
		return fmt.Errorf("string (%s) trailing base64 byte found", inputText)
	}

	buf = append(buf, '"')

	// Decode base64: collect 4-char groups, track padding separately.
	// Handles standard, URL-safe (-/_), and unpadded inputs.
	var vals [4]byte
	var pads [4]bool // true if the char was '=' padding
	n := 0
	for _, ch := range content {
		if ch == '=' {
			pads[n] = true
			vals[n] = 0
		} else {
			v, ok := base64DecodeChar(ch)
			if !ok {
				return nil, invalidBase64()
			}
			vals[n] = v
		}
		n++
		if n == 4 {
			if pads[0] || pads[1] || (pads[2] && !pads[3]) {
				return nil, invalidBase64()
			}
			buf = appendJSONByte(buf, (vals[0]<<2)|(vals[1]>>4))
			if !pads[2] {
				buf = appendJSONByte(buf, ((vals[1]&0x0F)<<4)|(vals[2]>>2))
			}
			if !pads[3] {
				buf = appendJSONByte(buf, ((vals[2]&0x03)<<6)|vals[3])
			}
			n = 0
			pads = [4]bool{}
		}
	}
	// Remaining chars (unpadded input: 2 or 3 chars in last group)
	if n == 1 {
		if !pads[0] {
			return nil, trailingByte()
		}
	} else if n == 3 {
		if pads[0] || pads[1] || pads[2] {
			return nil, invalidBase64()
		}
		buf = appendJSONByte(buf, (vals[0]<<2)|(vals[1]>>4))
		buf = appendJSONByte(buf, ((vals[1]&0x0F)<<4)|(vals[2]>>2))
	} else if n == 2 {
		if pads[0] || pads[1] {
			return nil, invalidBase64()
		}
		buf = appendJSONByte(buf, (vals[0]<<2)|(vals[1]>>4))
	}

	buf = append(buf, '"')
	return buf, nil
}

// isNull reports whether a JSON value (possibly whitespace-padded) is null.
func isNull(v []byte) bool {
	s := scanner{data: v}
	s.skipWhitespace()
	return s.pos < len(s.data) && s.data[s.pos] == 'n'
}

// execValues implements `values` = select(. != null).
// Emits input unchanged if it is not null; emits nothing if null.
// Use as `.[] | values` to filter nulls from a stream.
func execValues(input []byte, buf []byte, fn func([]byte) error) error {
	if isNull(input) {
		return nil // null → produce 0 outputs
	}
	return fn(input)
}

// execIn implements in(obj): tests whether the input value is a key in obj (objects)
// or an index in range for arrays. E.g. "foo" | in({"foo":1}) = true.
func execIn(state execState, node *op, input []byte, buf []byte) []byte {
	// Evaluate the container expression
	container, err := execSingle(state, node.child, input, nil)
	if err != nil {
		return boolResult(buf, false)
	}

	cs := scanner{data: container}
	cs.skipWhitespace()
	if cs.pos >= len(cs.data) {
		return boolResult(buf, false)
	}

	is := scanner{data: input}
	is.skipWhitespace()

	switch cs.data[cs.pos] {
	case '{': // "key" | in(obj) — check if input string is a key
		if is.pos < len(is.data) && is.data[is.pos] == '"' {
			key := is.readString()
			vs, _ := cs.findField(key)
			return boolResult(buf, vs != -1)
		}
	case '[': // n | in(arr) — check if input integer is a valid index
		if is.pos < len(is.data) {
			f, ok := parseJSONFloat(input)
			if ok {
				idx := int(f)
				if idx >= 0 {
					length := cs.arrayLen()
					return boolResult(buf, idx < length)
				}
			}
		}
	}
	return boolResult(buf, false)
}

func execINBuiltin(state execState, node *op, input []byte, buf []byte) ([]byte, error) {
	rightVals, err := collectExecOutputs(state, node.child, input)
	if err != nil {
		return nil, err
	}
	if node.left == nil {
		for _, right := range rightVals {
			if jsonEqual(input, right) {
				return boolResult(buf, true), nil
			}
		}
		return boolResult(buf, false), nil
	}
	leftVals, err := collectExecOutputs(state, node.left, input)
	if err != nil {
		return nil, err
	}
	for _, left := range leftVals {
		for _, right := range rightVals {
			if jsonEqual(left, right) {
				return boolResult(buf, true), nil
			}
		}
	}
	return boolResult(buf, false), nil
}

func execBsearch(state execState, node *op, input []byte, buf []byte, fn func([]byte) error) error {
	input = trimWhitespace(input)
	s := scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '[' {
		return &transparentError{err: fmt.Errorf("%s (%s) cannot be searched from", jsonTypeName(input), input)}
	}
	var elems [][]byte
	s.arrayIter(func(_ int, elemStart, elemEnd int) bool {
		elems = append(elems, input[elemStart:elemEnd])
		return true
	})
	return execMulti(state, node.child, input, nil, func(search []byte) error {
		idx := binarySearchJSON(elems, search)
		return fn(appendInt(buf[:0], idx))
	})
}

func binarySearchJSON(elems [][]byte, needle []byte) int {
	lo, hi := 0, len(elems)
	for lo < hi {
		mid := lo + (hi-lo)/2
		if compareJSONOrder(elems[mid], needle) < 0 {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo < len(elems) && compareJSONOrder(elems[lo], needle) == 0 {
		return lo
	}
	return -(lo + 1)
}

type indexedObjectEntry struct {
	key   []byte
	value []byte
}

func execINDEXBuiltin(state execState, node *op, input []byte, buf []byte) ([]byte, error) {
	streamVals, err := collectExecOutputs(state, node.left, input)
	if err != nil {
		return nil, err
	}
	var entries []indexedObjectEntry
	positions := make(map[string]int)
	for _, val := range streamVals {
		keyVals, err := collectExecOutputs(state, node.child, val)
		if err != nil {
			return nil, err
		}
		for _, keyVal := range keyVals {
			keyBytes := jsonObjectKeyBytes(keyVal)
			keyStr := string(keyBytes)
			if idx, ok := positions[keyStr]; ok {
				entries[idx].value = cloneExecBytes(val)
				continue
			}
			positions[keyStr] = len(entries)
			entries = append(entries, indexedObjectEntry{
				key:   cloneExecBytes(keyBytes),
				value: cloneExecBytes(val),
			})
		}
	}
	buf = append(buf, '{')
	for i, entry := range entries {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = append(buf, '"')
		buf = appendJSONStringContent(buf, entry.key)
		buf = append(buf, '"', ':')
		buf = append(buf, entry.value...)
	}
	buf = append(buf, '}')
	return buf, nil
}

func execJOINBuiltin(state execState, node *op, input []byte, buf []byte) ([]byte, error) {
	indexVal, err := execSingle(state, node.left, input, nil)
	if err != nil {
		return nil, err
	}
	indexVal = trimWhitespace(indexVal)
	s := scanner{data: indexVal}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '{' {
		return nil, fmt.Errorf("JOIN index must be an object")
	}
	out := append(buf[:0], '[')
	first := true
	appendJoin := func(elem []byte) error {
		keyVals, err := collectExecOutputs(state, node.child, elem)
		if err != nil {
			return err
		}
		for _, keyVal := range keyVals {
			key := jsonObjectKeyBytes(keyVal)
			obj := scanner{data: indexVal}
			obj.skipWhitespace()
			valueStart, valueEnd := obj.findField(key)
			match := bNull
			if valueStart != -1 {
				match = indexVal[valueStart:valueEnd]
			}
			if !first {
				out = append(out, ',')
			}
			first = false
			out = append(out, '[')
			out = append(out, trimWhitespace(elem)...)
			out = append(out, ',')
			out = append(out, match...)
			out = append(out, ']')
		}
		return nil
	}
	in := scanner{data: trimWhitespace(input)}
	in.skipWhitespace()
	if in.pos < len(in.data) && in.data[in.pos] == '[' {
		if err := execMulti(state, &op{typ: opIterator}, input, nil, appendJoin); err != nil {
			return nil, err
		}
	} else {
		if err := appendJoin(input); err != nil {
			return nil, err
		}
	}
	out = append(out, ']')
	return out, nil
}

func jsonObjectKeyBytes(val []byte) []byte {
	val = trimWhitespace(val)
	s := scanner{data: val}
	s.skipWhitespace()
	if s.pos < len(s.data) && s.data[s.pos] == '"' {
		return decodeJSONStringContent(nil, s.readString())
	}
	return val
}

// execDebug prints the input to stderr and is used by opDebug.
func execDebug(input []byte) {
	fmt.Fprintf(os.Stderr, "[DEBUG]: %s\n", input)
}

// execFindIndex implements index(s), rindex(s), and indices(s).
//
//	last=true, all=false  → rindex: last occurrence
//	last=false, all=false → index:  first occurrence, null if not found
//	last=false, all=true  → indices: all occurrences as array
func execFindIndex(state execState, node *op, input []byte, buf []byte, last, all bool) []byte {
	// Evaluate the search value
	searchVal, err := execSingle(state, node.child, input, nil)
	if err != nil {
		if all {
			return append(buf, "[]"...)
		}
		return append(buf, "null"...)
	}

	ss := scanner{data: input}
	ss.skipWhitespace()
	if ss.pos >= len(ss.data) {
		if all {
			return append(buf, "[]"...)
		}
		return append(buf, "null"...)
	}

	switch ss.data[ss.pos] {
	case '"': // string: search for substring
		// Both input and searchVal should be JSON strings
		sv := scanner{data: searchVal}
		sv.skipWhitespace()
		if sv.pos >= len(sv.data) || sv.data[sv.pos] != '"' {
			break
		}
		content := ss.readString() // raw bytes of input string content
		needle := sv.readString()  // raw bytes of search string content

		// Empty needle: jq returns null for index/rindex, [] for indices
		if len(needle) == 0 {
			if all {
				return append(buf, "[]"...)
			}
			return append(buf, "null"...)
		}

		if all {
			buf = append(buf, '[')
			first := true
			// Advance by 1 each iteration to find overlapping matches.
			// Report codepoint positions (not byte positions) for Unicode correctness.
			for pos := 0; pos <= len(content)-len(needle); pos++ {
				if bytesEqual(content[pos:pos+len(needle)], needle) {
					if !first {
						buf = append(buf, ',')
					}
					first = false
					buf = appendInt(buf, byteOffsetToCodepointOffset(content, pos))
				}
			}
			return append(buf, ']')
		}
		// index or rindex — report codepoint positions
		found := -1
		if last {
			for i := len(content) - len(needle); i >= 0; i-- {
				if bytesEqual(content[i:i+len(needle)], needle) {
					found = i
					break
				}
			}
		} else {
			for i := 0; i <= len(content)-len(needle); i++ {
				if bytesEqual(content[i:i+len(needle)], needle) {
					found = i
					break
				}
			}
		}
		if found == -1 {
			return append(buf, "null"...)
		}
		return appendInt(buf, byteOffsetToCodepointOffset(content, found))

	case '[': // array input
		sv2 := scanner{data: searchVal}
		sv2.skipWhitespace()
		if sv2.pos < len(sv2.data) && sv2.data[sv2.pos] == '[' {
			// searchVal is also an array — find all positions where the
			// input array contains searchVal as a contiguous subsequence.
			var needle [][]byte
			sv2.arrayIter(func(_ int, start, end int) bool {
				needle = append(needle, searchVal[start:end])
				return true
			})
			if len(needle) == 0 {
				if all {
					return append(buf, "[]"...)
				}
				return append(buf, "null"...)
			}
			var elems [][]byte
			ss.arrayIter(func(_ int, start, end int) bool {
				elems = append(elems, input[start:end])
				return true
			})
			if all {
				buf = append(buf, '[')
				first := true
				for i := 0; i <= len(elems)-len(needle); i++ {
					if arraySubseqMatch(elems[i:], needle) {
						if !first {
							buf = append(buf, ',')
						}
						first = false
						buf = appendInt(buf, i)
					}
				}
				return append(buf, ']')
			}
			found := -1
			for i := 0; i <= len(elems)-len(needle); i++ {
				if arraySubseqMatch(elems[i:], needle) {
					if last {
						found = i
					} else {
						return appendInt(buf, i)
					}
				}
			}
			if found == -1 {
				return append(buf, "null"...)
			}
			return appendInt(buf, found)
		}
		// searchVal is a scalar — search for matching element
		if all {
			buf = append(buf, '[')
			first := true
			i := 0
			ss.arrayIter(func(_ int, start, end int) bool {
				if jsonEqual(input[start:end], searchVal) {
					if !first {
						buf = append(buf, ',')
					}
					first = false
					buf = appendInt(buf, i)
				}
				i++
				return true
			})
			return append(buf, ']')
		}
		// index or rindex
		found := -1
		i := 0
		ss.arrayIter(func(_ int, start, end int) bool {
			if jsonEqual(input[start:end], searchVal) {
				if last {
					found = i // keep updating for rindex
				} else if found == -1 {
					found = i // stop at first for index
					return false
				}
			}
			i++
			return true
		})
		if found == -1 {
			return append(buf, "null"...)
		}
		return appendInt(buf, found)
	}

	if all {
		return append(buf, "[]"...)
	}
	return append(buf, "null"...)
}

// arraySubseqMatch reports whether arr starts with all elements equal to needle.
func arraySubseqMatch(arr, needle [][]byte) bool {
	if len(arr) < len(needle) {
		return false
	}
	for i, n := range needle {
		if !jsonEqual(arr[i], n) {
			return false
		}
	}
	return true
}

// execKeysUnsorted returns object keys (insertion order) or array indices as a JSON array.
func execKeysUnsorted(input []byte, buf []byte) ([]byte, error) {
	s := &scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) {
		return append(buf, "[]"...), nil
	}
	buf = append(buf, '[')
	first := true
	switch s.data[s.pos] {
	case '{':
		s.objectIter(func(key []byte, _, _ int) bool {
			if !first {
				buf = append(buf, ',')
			}
			first = false
			buf = append(buf, '"')
			buf = append(buf, key...)
			buf = append(buf, '"')
			return true
		})
	case '[':
		count := s.arrayLen()
		for i := 0; i < count; i++ {
			if !first {
				buf = append(buf, ',')
			}
			first = false
			buf = appendInt(buf, i)
		}
	default:
		return nil, fmt.Errorf("keys_unsorted input must be an object or array")
	}
	buf = append(buf, ']')
	return buf, nil
}

func execKeys(input []byte, buf []byte) ([]byte, error) {
	s := &scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) {
		return append(buf, "[]"...), nil
	}
	if s.data[s.pos] == '[' {
		return execKeysUnsorted(input, buf)
	}
	if s.data[s.pos] != '{' {
		return nil, fmt.Errorf("keys input must be an object or array")
	}

	var keys [][]byte
	s.objectIter(func(key []byte, _, _ int) bool {
		keys = append(keys, key)
		return true
	})
	sort.Slice(keys, func(i, j int) bool {
		return bytesCompare(keys[i], keys[j]) < 0
	})

	buf = append(buf, '[')
	for i, key := range keys {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = append(buf, '"')
		buf = append(buf, key...)
		buf = append(buf, '"')
	}
	buf = append(buf, ']')
	return buf, nil
}

var supportedBuiltinNames = []string{
	"IN/1", "INDEX/2", "JOIN/2", "abs/0", "add/0", "all/0", "all/1", "any/0", "any/1",
	"ascii_downcase/0", "ascii_upcase/0", "bsearch/1", "builtins/0", "capture/1", "ceil/0",
	"combinations/0", "combinations/1", "contains/1", "date/0", "debug/0", "delpaths/1", "endswith/1",
	"error/0", "explode/0", "first/0", "flatten/0", "flatten/1", "floor/0", "fma/3", "foreach/3",
	"from_entries/0", "fromdate/0", "fromjson/0", "getpath/1", "gmtime/0", "group_by/1",
	"gsub/2", "has/1", "have_decnum/0", "hypot/2", "implode/0", "in/1", "index/1", "indices/1",
	"isempty/1", "join/1", "keys/0", "keys_unsorted/0", "last/0", "length/0", "limit/2",
	"leaf_paths/0", "ltrim/0", "ltrimstr/1", "map/1", "map_values/1", "match/1", "max/0", "max_by/1",
	"mktime/0", "min/0", "min_by/1", "nearbyint/0", "nth/2", "path/1", "paths/0", "paths/1",
	"now/0", "pick/1", "pow/2", "range/1", "range/2", "range/3", "reduce/3", "repeat/1", "reverse/0",
	"recurse/0", "recurse/1", "recurse/2", "rindex/1", "round/0", "rtrim/0", "rtrimstr/1", "scan/1", "select/1", "setpath/2",
	"skip/2", "sort/0", "sort_by/1", "split/1", "sqrt/0", "startswith/1", "strflocaltime/1",
	"strftime/1", "strptime/1", "sub/2", "test/1", "to_entries/0", "toboolean/0", "todate/0",
	"tojson/0", "tonumber/0", "tostream/0", "tostring/0", "transpose/0", "trim/0", "trimstr/1",
	"truncate_stream/1", "type/0", "unique/0", "unique_by/1", "until/2", "utf8bytelength/0",
	"values/0", "walk/1", "while/2", "fromstream/1",
	"with_entries/1",
}

func execBuiltins(buf []byte) []byte {
	buf = append(buf, '[')
	for i, name := range supportedBuiltinNames {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = append(buf, '"')
		buf = appendJSONStringContent(buf, []byte(name))
		buf = append(buf, '"')
	}
	buf = append(buf, ']')
	return buf
}

func execToStream(input []byte, buf []byte, fn func([]byte) error) error {
	return execToStreamValue(trimWhitespace(input), nil, buf, fn)
}

func execToStreamValue(value []byte, frame *pathFrame, buf []byte, fn func([]byte) error) error {
	s := scanner{data: value}
	s.skipWhitespace()
	if s.pos >= len(s.data) {
		return fn(appendStreamEvent(buf[:0], frame, value, true))
	}
	switch s.data[s.pos] {
	case '[':
		if s.arrayLen() == 0 {
			return fn(appendStreamEvent(buf[:0], frame, []byte("[]"), true))
		}
		var lastChild *pathFrame
		var err error
		s.arrayIter(func(index int, elemStart, elemEnd int) bool {
			childFrame := &pathFrame{parent: frame, kind: pathStepNumber, index: index}
			lastChild = childFrame
			err = execToStreamValue(value[elemStart:elemEnd], childFrame, buf, fn)
			return err == nil
		})
		if err != nil {
			return err
		}
		return fn(appendStreamEvent(buf[:0], lastChild, nil, false))
	case '{':
		empty := true
		var lastChild *pathFrame
		var err error
		s.objectIter(func(key []byte, valueStart, valueEnd int) bool {
			empty = false
			childFrame := &pathFrame{parent: frame, kind: pathStepString, rawKey: key}
			lastChild = childFrame
			err = execToStreamValue(value[valueStart:valueEnd], childFrame, buf, fn)
			return err == nil
		})
		if err != nil {
			return err
		}
		if empty {
			return fn(appendStreamEvent(buf[:0], frame, []byte("{}"), true))
		}
		return fn(appendStreamEvent(buf[:0], lastChild, nil, false))
	default:
		return fn(appendStreamEvent(buf[:0], frame, value, true))
	}
}

func execFindIndexMulti(state execState, node *op, input []byte, buf []byte, fn func([]byte) error, last, all bool) error {
	searchVals, err := collectExecOutputs(state, node.child, input)
	if err != nil {
		if all {
			return fn(append(buf[:0], "[]"...))
		}
		return fn(append(buf[:0], "null"...))
	}
	if len(searchVals) == 0 {
		if all {
			return fn(append(buf[:0], "[]"...))
		}
		return fn(append(buf[:0], "null"...))
	}
	for _, searchVal := range searchVals {
		searchNode := &op{child: &op{typ: opLiteral, literal: searchVal}}
		if err := fn(execFindIndex(state, searchNode, input, buf[:0], last, all)); err != nil {
			return err
		}
	}
	return nil
}

func appendStreamEvent(dst []byte, frame *pathFrame, value []byte, withValue bool) []byte {
	dst = append(dst, '[')
	dst = appendPathFrameJSON(dst, frame)
	if withValue {
		dst = append(dst, ',')
		dst = append(dst, normalizeOutputValue(value, nil)...)
	}
	dst = append(dst, ']')
	return dst
}

func execTruncateStream(state execState, node *op, input []byte, buf []byte, fn func([]byte) error) error {
	depthVal, err := execSingle(state, &op{typ: opIdentity}, input, nil)
	if err != nil {
		return err
	}
	f, ok := parseJSONFloat(trimWhitespace(depthVal))
	if !ok {
		return fmt.Errorf("truncate_stream depth must be numeric")
	}
	depth := int(f)
	return execMulti(state, node.child, input, nil, func(event []byte) error {
		truncated, keep, err := truncateStreamEvent(event, depth, buf[:0])
		if err != nil {
			return err
		}
		if !keep {
			return nil
		}
		return fn(truncated)
	})
}

func truncateStreamEvent(event []byte, depth int, buf []byte) ([]byte, bool, error) {
	pathVal, valueVal, hasValue, err := decodeStreamEvent(event)
	if err != nil {
		return nil, false, err
	}
	steps, err := decodePath(pathVal)
	if err != nil {
		return nil, false, err
	}
	if len(steps) <= depth {
		return nil, false, nil
	}
	steps = steps[depth:]
	out := append(buf, '[')
	out = append(out, '[')
	for i, step := range steps {
		if i > 0 {
			out = append(out, ',')
		}
		switch step.kind {
		case pathStepString:
			out = append(out, '"')
			out = appendCanonicalRawJSONStringContent(out, step.key)
			out = append(out, '"')
		case pathStepNumber:
			out = appendInt(out, step.index)
		default:
			return nil, false, fmt.Errorf("Paths must be specified as an array")
		}
	}
	out = append(out, ']')
	if hasValue {
		out = append(out, ',')
		out = append(out, normalizeOutputValue(valueVal, nil)...)
	}
	out = append(out, ']')
	return out, true, nil
}

func execFromStream(state execState, node *op, input []byte, buf []byte) ([]byte, error) {
	events, err := collectExecOutputs(state, node.child, input)
	if err != nil {
		return nil, err
	}
	current := []byte("null")
	seen := false
	for _, event := range events {
		pathVal, valueVal, hasValue, err := decodeStreamEvent(event)
		if err != nil {
			return nil, err
		}
		if !hasValue {
			continue
		}
		steps, err := decodePath(pathVal)
		if err != nil {
			return nil, err
		}
		if len(steps) == 0 {
			current = cloneExecBytes(valueVal)
			seen = true
			continue
		}
		current, err = setPathDecoded(current, steps, 0, valueVal, nil)
		if err != nil {
			return nil, err
		}
		seen = true
	}
	if !seen {
		return append(buf, "null"...), nil
	}
	return normalizeOutputValue(current, buf), nil
}

func decodeStreamEvent(event []byte) (pathVal []byte, valueVal []byte, hasValue bool, err error) {
	event = trimWhitespace(event)
	s := scanner{data: event}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '[' {
		return nil, nil, false, fmt.Errorf("Stream event must be an array")
	}
	parts := make([][]byte, 0, 2)
	s.arrayIter(func(_ int, start, end int) bool {
		parts = append(parts, cloneExecBytes(event[start:end]))
		return len(parts) < 3
	})
	if len(parts) == 0 {
		return nil, nil, false, fmt.Errorf("Stream event must be an array")
	}
	if len(parts) > 2 {
		return nil, nil, false, fmt.Errorf("Stream event must be an array")
	}
	pathVal = parts[0]
	if len(parts) == 2 {
		valueVal = parts[1]
		hasValue = true
	}
	return pathVal, valueVal, hasValue, nil
}

func execGetPath(state execState, node *op, input []byte, buf []byte, fn func([]byte) error) error {
	return execMulti(state, node.child, input, nil, func(pathVal []byte) error {
		result, err := execGetPathOne(input, pathVal, buf)
		if err != nil {
			return err
		}
		return fn(result)
	})
}

type pathFrame struct {
	parent *pathFrame
	kind   pathStepKind
	rawKey []byte
	index  int
}

type pathTraceState struct {
	value  []byte
	frame  *pathFrame
	opaque bool
}

func execPath(state execState, node *op, input []byte, buf []byte, fn func([]byte) error) error {
	root := pathTraceState{value: trimWhitespace(input)}
	return execPathExpr(state, node.child, root, buf, func(trace pathTraceState) error {
		if trace.opaque {
			return invalidPathResultError(trace.value)
		}
		out := appendPathFrameJSON(buf[:0], trace.frame)
		return fn(out)
	})
}

func execPathExpr(state execState, node *op, trace pathTraceState, buf []byte, emit func(pathTraceState) error) error {
	if node == nil {
		return emit(trace)
	}

	switch node.typ {
	case opIdentity:
		return emit(trace)
	case opField:
		if trace.opaque {
			return invalidPathFieldError(trace.value, node.field)
		}
		next := pathTraceState{
			value: pathFieldTraceValue(trace.value, node.field),
			frame: &pathFrame{
				parent: trace.frame,
				kind:   pathStepString,
				rawKey: []byte(node.field),
			},
		}
		if node.child != nil {
			return state.withIndexScope(state.chainedIndexScope(trace.value), func(state execState) error {
				return execPathExpr(state, node.child, next, buf, emit)
			})
		}
		return emit(next)
	case opIndex:
		if trace.opaque {
			return invalidPathIndexError(trace.value, node.index)
		}
		next := pathTraceState{
			value: pathIndexTraceValue(trace.value, node.index),
			frame: &pathFrame{
				parent: trace.frame,
				kind:   pathStepNumber,
				index:  node.index,
			},
		}
		if node.child != nil {
			return state.withIndexScope(state.chainedIndexScope(trace.value), func(state execState) error {
				return execPathExpr(state, node.child, next, buf, emit)
			})
		}
		return emit(next)
	case opIndexExpr:
		scopeInput := state.currentIndexScope()
		if scopeInput == nil {
			scopeInput = trace.value
		}
		keys, err := collectExecOutputs(state, node.left, scopeInput)
		if err != nil {
			return err
		}
		for _, key := range keys {
			if trace.opaque {
				return invalidPathDynamicIndexError(trace.value, key)
			}
			next, err := pathTraceDynamicIndex(trace, key)
			if err != nil {
				return err
			}
			if node.child != nil {
				if err := state.withIndexScope(state.chainedIndexScope(trace.value), func(state execState) error {
					return execPathExpr(state, node.child, next, buf, emit)
				}); err != nil {
					return err
				}
				continue
			}
			if err := emit(next); err != nil {
				return err
			}
		}
		return nil
	case opIterator:
		if trace.opaque {
			return invalidPathIteratorError(trace.value)
		}
		return state.withIndexScope(state.chainedIndexScope(trace.value), func(state execState) error {
			return execPathIterator(state, node, trace, buf, emit)
		})
	case opRecursiveDescent:
		if err := emit(trace); err != nil {
			return err
		}
		return execPathRecursive(trace, buf, emit)
	case opPipe:
		return execPathExpr(state, node.left, trace, buf, func(next pathTraceState) error {
			return execPathExpr(state, node.right, next, buf, emit)
		})
	case opApply:
		return execPathExpr(state, node.left, trace, buf, func(next pathTraceState) error {
			return state.withIndexScope(state.chainedIndexScope(trace.value), func(state execState) error {
				return execPathExpr(state, node.right, next, buf, emit)
			})
		})
	case opSelect:
		match, err := pathsFilterMatch(state, node.child, trace.value)
		if err != nil {
			return err
		}
		if match {
			return emit(trace)
		}
		return nil
	case opGenerator:
		for _, elem := range node.elems {
			if err := execPathExpr(state, elem, trace, buf, emit); err != nil {
				return err
			}
		}
		return nil
	default:
		return execMulti(state, node, trace.value, nil, func(out []byte) error {
			return emit(pathTraceState{
				value:  cloneExecBytes(out),
				frame:  trace.frame,
				opaque: true,
			})
		})
	}
}

func execPathIterator(state execState, node *op, trace pathTraceState, buf []byte, emit func(pathTraceState) error) error {
	s := scanner{data: trace.value}
	s.skipWhitespace()
	if s.pos >= len(s.data) {
		return nil
	}

	switch s.data[s.pos] {
	case '[':
		var iterErr error
		s.arrayIter(func(index int, elemStart, elemEnd int) bool {
			next := pathTraceState{
				value: trace.value[elemStart:elemEnd],
				frame: &pathFrame{
					parent: trace.frame,
					kind:   pathStepNumber,
					index:  index,
				},
			}
			if node.child != nil {
				iterErr = execPathExpr(state, node.child, next, buf, emit)
			} else {
				iterErr = emit(next)
			}
			return iterErr == nil
		})
		return iterErr
	case '{':
		var iterErr error
		s.objectIter(func(key []byte, valueStart, valueEnd int) bool {
			next := pathTraceState{
				value: trace.value[valueStart:valueEnd],
				frame: &pathFrame{
					parent: trace.frame,
					kind:   pathStepString,
					rawKey: key,
				},
			}
			if node.child != nil {
				iterErr = execPathExpr(state, node.child, next, buf, emit)
			} else {
				iterErr = emit(next)
			}
			return iterErr == nil
		})
		return iterErr
	default:
		return nil
	}
}

func execPathRecursive(state pathTraceState, buf []byte, emit func(pathTraceState) error) error {
	s := scanner{data: state.value}
	s.skipWhitespace()
	if s.pos >= len(s.data) {
		return nil
	}

	switch s.data[s.pos] {
	case '[':
		var walkErr error
		s.arrayIter(func(index int, elemStart, elemEnd int) bool {
			next := pathTraceState{
				value: state.value[elemStart:elemEnd],
				frame: &pathFrame{
					parent: state.frame,
					kind:   pathStepNumber,
					index:  index,
				},
			}
			walkErr = execPathRecursiveNode(next, buf, emit)
			return walkErr == nil
		})
		return walkErr
	case '{':
		var walkErr error
		s.objectIter(func(key []byte, valueStart, valueEnd int) bool {
			next := pathTraceState{
				value: state.value[valueStart:valueEnd],
				frame: &pathFrame{
					parent: state.frame,
					kind:   pathStepString,
					rawKey: key,
				},
			}
			walkErr = execPathRecursiveNode(next, buf, emit)
			return walkErr == nil
		})
		return walkErr
	default:
		return nil
	}
}

func execPathRecursiveNode(state pathTraceState, buf []byte, emit func(pathTraceState) error) error {
	if err := emit(state); err != nil {
		return err
	}
	return execPathRecursive(state, buf, emit)
}

func execPick(state execState, node *op, input []byte, buf []byte) ([]byte, error) {
	current := bNull
	for _, expr := range node.elems {
		if hasNegativePathIndex(expr) {
			return nil, fmt.Errorf("Out of bounds negative array index")
		}
		paths, err := collectAssignPaths(state, expr, input)
		if err != nil {
			return nil, err
		}
		for _, steps := range paths {
			value, err := getPathDecoded(input, steps, nil)
			if err != nil {
				return nil, err
			}
			current, err = setPathDecoded(current, steps, 0, value, nil)
			if err != nil {
				return nil, err
			}
		}
	}
	return normalizeOutputValue(current, buf), nil
}

func hasNegativePathIndex(node *op) bool {
	if node == nil {
		return false
	}
	if node.typ == opIndex && node.index < 0 {
		return true
	}
	for _, elem := range node.elems {
		if hasNegativePathIndex(elem) {
			return true
		}
	}
	for _, p := range node.pairs {
		if hasNegativePathIndex(p.expr) {
			return true
		}
	}
	return hasNegativePathIndex(node.left) || hasNegativePathIndex(node.right) || hasNegativePathIndex(node.child) || hasNegativePathIndex(node.extra)
}

func execAssign(state execState, node *op, input []byte, buf []byte, fn func([]byte) error) error {
	if node.left != nil && node.left.typ == opSlice {
		return execAssignSlice(state, node, input, buf, fn)
	}
	if isNaNIndexAssign(node.left) {
		return &transparentError{err: fmt.Errorf("Cannot set array element at NaN index")}
	}
	paths, err := collectAssignPaths(state, node.left, input)
	if err != nil {
		return err
	}
	values, err := collectExecOutputs(state, node.right, input)
	if err != nil {
		return err
	}
	for _, value := range values {
		current := normalizeOutputValue(input, nil)
		for _, steps := range paths {
			current, err = setPathDecoded(current, steps, 0, value, nil)
			if err != nil {
				return err
			}
		}
		if err := fn(append(buf[:0], current...)); err != nil {
			return err
		}
	}
	return nil
}

func isNaNIndexAssign(node *op) bool {
	if node == nil || node.typ != opIndexExpr || node.left == nil || node.left.typ != opLiteral {
		return false
	}
	lit := strings.TrimSpace(string(node.left.literal))
	return lit == "NaN" || lit == "infinite" || lit == "-infinite"
}

func execAssignSlice(state execState, node *op, input []byte, buf []byte, fn func([]byte) error) error {
	values, err := collectExecOutputs(state, node.right, input)
	if err != nil {
		return err
	}
	for _, value := range values {
		current, err := setRootSliceValue(state, input, node.left, value, nil)
		if err != nil {
			return err
		}
		if err := fn(append(buf[:0], current...)); err != nil {
			return err
		}
	}
	return nil
}

func setRootSliceValue(state execState, input []byte, slice *op, value []byte, buf []byte) ([]byte, error) {
	s := scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) {
		return nil, fmt.Errorf("Cannot update string slices")
	}
	if s.data[s.pos] == '"' {
		return nil, &transparentError{err: fmt.Errorf("Cannot update string slices")}
	}
	if s.data[s.pos] != '[' {
		return nil, dynamicIndexNumberAccessError(input)
	}
	length := s.arrayLen()
	start, end, err := resolveDelSliceBounds(state, slice, input, length)
	if err != nil {
		return nil, err
	}
	valueElems, err := decodeArrayElements(value)
	if err != nil {
		valueElems = [][]byte{normalizeOutputValue(value, nil)}
	}
	buf = append(buf, '[')
	first := true
	emit := func(elem []byte) {
		if !first {
			buf = append(buf, ',')
		}
		first = false
		buf = append(buf, normalizeOutputValue(elem, nil)...)
	}
	idx := 0
	s.arrayIter(func(i int, elemStart, elemEnd int) bool {
		if i == start {
			for _, repl := range valueElems {
				emit(repl)
			}
		}
		if i < start || i >= end {
			emit(input[elemStart:elemEnd])
		}
		idx = i + 1
		return true
	})
	if start == length {
		for _, repl := range valueElems {
			emit(repl)
		}
	}
	_ = idx
	buf = append(buf, ']')
	return buf, nil
}

func decodeArrayElements(value []byte) ([][]byte, error) {
	trimmed := trimWhitespace(value)
	s := scanner{data: trimmed}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '[' {
		return nil, fmt.Errorf("not array")
	}
	var elems [][]byte
	s.arrayIter(func(_ int, start, end int) bool {
		elems = append(elems, cloneExecBytes(trimmed[start:end]))
		return true
	})
	return elems, nil
}

func execUpdate(state execState, node *op, input []byte, buf []byte, fn func([]byte) error) error {
	paths, err := collectAssignPaths(state, node.left, input)
	if err != nil {
		return err
	}
	mutations := make([]pathMutation, 0, len(paths))
	needsDeleteOrdering := false
	for i, steps := range paths {
		oldVal, err := getPathDecoded(input, steps, nil)
		if err != nil {
			return err
		}
		values, err := collectExecOutputs(state, node.right, oldVal)
		if err != nil {
			return err
		}
		if len(values) == 0 {
			mutations = append(mutations, pathMutation{steps: steps, order: i, delete: true})
			needsDeleteOrdering = true
			continue
		}
		mutations = append(mutations, pathMutation{steps: steps, order: i, value: values[0]})
	}
	if needsDeleteOrdering {
		sortPathMutations(mutations)
	}
	current := normalizeOutputValue(input, nil)
	for _, mutation := range mutations {
		if mutation.delete {
			current, err = deletePathDecoded(current, mutation.steps)
		} else {
			current, err = setPathDecoded(current, mutation.steps, 0, mutation.value, nil)
		}
		if err != nil {
			return err
		}
	}
	return fn(append(buf[:0], current...))
}

func execUpdateAlt(state execState, node *op, input []byte, buf []byte, fn func([]byte) error) error {
	paths, err := collectAssignPaths(state, node.left, input)
	if err != nil {
		return err
	}
	mutations := make([]pathMutation, 0, len(paths))
	needsDeleteOrdering := false
	for i, steps := range paths {
		oldVal, err := getPathDecoded(input, steps, nil)
		if err != nil {
			return err
		}
		if !isFalsy(oldVal) {
			mutations = append(mutations, pathMutation{steps: steps, order: i, value: cloneExecBytes(oldVal)})
			continue
		}
		values, err := collectExecOutputs(state, node.right, input)
		if err != nil {
			return err
		}
		if len(values) == 0 {
			mutations = append(mutations, pathMutation{steps: steps, order: i, delete: true})
			needsDeleteOrdering = true
			continue
		}
		mutations = append(mutations, pathMutation{steps: steps, order: i, value: values[0]})
	}
	if needsDeleteOrdering {
		sortPathMutations(mutations)
	}
	current := normalizeOutputValue(input, nil)
	for _, mutation := range mutations {
		if mutation.delete {
			current, err = deletePathDecoded(current, mutation.steps)
		} else {
			current, err = setPathDecoded(current, mutation.steps, 0, mutation.value, nil)
		}
		if err != nil {
			return err
		}
	}
	return fn(append(buf[:0], current...))
}

func execUpdateMath(state execState, node *op, input []byte, buf []byte, fn func([]byte) error) error {
	paths, err := collectAssignPaths(state, node.left, input)
	if err != nil {
		return err
	}
	rhs, err := execSingle(state, node.right, input, nil)
	if err != nil {
		return err
	}
	current := normalizeOutputValue(input, nil)
	opTyp := opPlus
	switch node.updateOp {
	case updatePlus:
		opTyp = opPlus
	case updateMinus:
		opTyp = opMinus
	case updateMul:
		opTyp = opMul
	case updateDiv:
		opTyp = opDiv
	case updateMod:
		opTyp = opMod
	}
	for _, steps := range paths {
		oldVal, err := getPathDecoded(current, steps, nil)
		if err != nil {
			return err
		}
		var newVal []byte
		if node.updateOp == updatePlus {
			newVal, err = execPlusValues(oldVal, rhs, nil)
		} else {
			newVal, err = execArithValues(opTyp, oldVal, rhs, nil)
		}
		if err != nil {
			return err
		}
		current, err = setPathDecoded(current, steps, 0, newVal, nil)
		if err != nil {
			return err
		}
	}
	return fn(append(buf[:0], current...))
}

func collectAssignPaths(state execState, node *op, input []byte) ([][]pathStep, error) {
	if node == nil {
		return nil, nil
	}
	if node.typ == opBind {
		baseCtx := state.currentExecContext()
		values, err := collectExecOutputs(state, node.left, input)
		if err != nil {
			return nil, err
		}
		var all [][]pathStep
		for _, value := range values {
			nextCtx, err := bindOpValue(baseCtx, node, value)
			if err != nil {
				return nil, err
			}
			var nested [][]pathStep
			if err := state.withExecContext(nextCtx, func(state execState) error {
				var innerErr error
				nested, innerErr = collectAssignPaths(state, node.right, input)
				return innerErr
			}); err != nil {
				return nil, err
			}
			all = append(all, nested...)
		}
		return all, nil
	}
	if node.typ == opCall {
		callerCtx := state.currentExecContext()
		def := callerCtx.lookupFunc(funcKey(node.name, len(node.elems)))
		if def == nil {
			return nil, fmt.Errorf("%s is not defined", funcKey(node.name, len(node.elems)))
		}
		callCtxs, err := bindCallContexts(state, def, callerCtx, node.elems, input)
		if err != nil {
			return nil, err
		}
		var all [][]pathStep
		for _, callCtx := range callCtxs {
			var nested [][]pathStep
			if err := state.withExecContext(callCtx, func(state execState) error {
				var innerErr error
				nested, innerErr = collectAssignPaths(state, def.body, input)
				return innerErr
			}); err != nil {
				return nil, err
			}
			all = append(all, nested...)
		}
		return all, nil
	}
	if node.typ == opGetPath {
		pathVals, err := collectExecOutputs(state, node.child, input)
		if err != nil {
			return nil, err
		}
		paths := make([][]pathStep, 0, len(pathVals))
		for _, pathVal := range pathVals {
			steps, err := decodePath(pathVal)
			if err != nil {
				return nil, err
			}
			paths = append(paths, steps)
		}
		return paths, nil
	}

	var paths [][]pathStep
	err := execPath(state, &op{typ: opPath, child: node}, input, nil, func(pathVal []byte) error {
		steps, err := decodePath(pathVal)
		if err != nil {
			return err
		}
		paths = append(paths, steps)
		return nil
	})
	return paths, err
}

type pathMutation struct {
	steps  []pathStep
	order  int
	delete bool
	value  []byte
}

func sortPathMutations(mutations []pathMutation) {
	sort.SliceStable(mutations, func(i, j int) bool {
		return pathStepsLessDesc(mutations[i].steps, mutations[j].steps, mutations[i].order, mutations[j].order)
	})
}

func pathStepsLessDesc(a, b []pathStep, aOrder, bOrder int) bool {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i].kind != b[i].kind {
			return aOrder < bOrder
		}
		switch a[i].kind {
		case pathStepNumber:
			if a[i].index != b[i].index {
				return a[i].index > b[i].index
			}
		case pathStepString:
			ak := string(a[i].key)
			bk := string(b[i].key)
			if ak != bk {
				return aOrder < bOrder
			}
		default:
			ar := string(a[i].raw)
			br := string(b[i].raw)
			if ar != br {
				return aOrder < bOrder
			}
		}
	}
	if len(a) != len(b) {
		return len(a) > len(b)
	}
	return aOrder < bOrder
}

func getPathDecoded(input []byte, steps []pathStep, buf []byte) ([]byte, error) {
	current := input
	for _, step := range steps {
		if isNull(current) {
			if buf == nil {
				return bNull, nil
			}
			return append(buf, "null"...), nil
		}
		var err error
		current, err = getPathDecodedStep(current, step)
		if err != nil {
			return nil, err
		}
	}
	return normalizeOutputValue(current, buf), nil
}

func getPathDecodedStep(current []byte, step pathStep) ([]byte, error) {
	switch step.kind {
	case pathStepString:
		cs := scanner{data: current}
		cs.skipWhitespace()
		if cs.pos >= len(cs.data) || cs.data[cs.pos] != '{' {
			return nil, setpathAccessError(current, step)
		}
		vs, ve := cs.findField(step.key)
		if vs == -1 {
			return bNull, nil
		}
		return current[vs:ve], nil
	case pathStepNumber:
		cs := scanner{data: current}
		cs.skipWhitespace()
		if cs.pos >= len(cs.data) || cs.data[cs.pos] != '[' {
			return nil, setpathAccessError(current, step)
		}
		idx := step.index
		if idx < 0 {
			idx = cs.arrayLen() + idx
			if idx < 0 {
				return bNull, nil
			}
		}
		var result []byte
		cs.arrayIter(func(i int, elemStart, elemEnd int) bool {
			if i == idx {
				result = current[elemStart:elemEnd]
				return false
			}
			return true
		})
		if result == nil {
			return bNull, nil
		}
		return result, nil
	default:
		return nil, setpathAccessError(current, step)
	}
}

func deletePathDecoded(input []byte, steps []pathStep) ([]byte, error) {
	out, _, err := delPathDecoded(input, steps, 0, nil)
	return out, err
}

func execPaths(state execState, node *op, input []byte, buf []byte, fn func([]byte) error) error {
	return execPathsValue(state, node.child, trimWhitespace(input), nil, buf, fn)
}

func execReduce(state execState, node *op, input []byte, buf []byte, fn func([]byte) error) error {
	states, err := collectExecOutputs(state, node.right, input)
	if err != nil {
		return err
	}
	if len(states) == 0 {
		return nil
	}

	finalStates := make([][]byte, 0, len(states))
	baseCtx := state.currentExecContext()
	for _, initState := range states {
		currentStates := [][]byte{initState}
		err = execMulti(state, node.left, input, nil, func(item []byte) error {
			nextStates := make([][]byte, 0, len(currentStates))
			for _, currentState := range currentStates {
				nextCtx, err := bindOpValue(baseCtx, node, item)
				if err != nil {
					return err
				}
				err = state.withExecContext(nextCtx, func(state execState) error {
					var last []byte
					if err := execMulti(state, node.child, currentState, nil, func(out []byte) error {
						last = cloneExecBytes(out)
						return nil
					}); err != nil {
						return err
					}
					if last != nil {
						nextStates = append(nextStates, last)
					}
					return nil
				})
				if err != nil {
					return err
				}
			}
			currentStates = nextStates
			return nil
		})
		if err != nil {
			return err
		}
		finalStates = append(finalStates, currentStates...)
	}
	for _, state := range finalStates {
		if err := fn(append(buf[:0], state...)); err != nil {
			return err
		}
	}
	return nil
}

func execForeach(state execState, node *op, input []byte, buf []byte, fn func([]byte) error) error {
	states, err := collectExecOutputs(state, node.right, input)
	if err != nil {
		return err
	}
	if len(states) == 0 {
		return nil
	}

	baseCtx := state.currentExecContext()
	for _, initState := range states {
		currentStates := [][]byte{initState}
		err = execMulti(state, node.left, input, nil, func(item []byte) error {
			nextStates := make([][]byte, 0, len(currentStates))
			for _, currentState := range currentStates {
				var last []byte
				nextCtx, err := bindOpValue(baseCtx, node, item)
				if err != nil {
					return err
				}
				err = state.withExecContext(nextCtx, func(state execState) error {
					return execMulti(state, node.child, currentState, nil, func(updated []byte) error {
						updatedState := cloneExecBytes(updated)
						last = updatedState
						if err := execMulti(state, node.extra, updatedState, nil, func(out []byte) error {
							return fn(append(buf[:0], out...))
						}); err != nil {
							return err
						}
						return nil
					})
				})
				if err != nil {
					return err
				}
				if last != nil {
					nextStates = append(nextStates, last)
				}
			}
			currentStates = nextStates
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func execWhile(state execState, node *op, input []byte, buf []byte, fn func([]byte) error) error {
	current := cloneExecBytes(input)
	for {
		condVal, err := execSingle(state, node.left, current, nil)
		if err != nil {
			return err
		}
		if isFalsy(condVal) {
			return nil
		}
		if err := fn(append(buf[:0], current...)); err != nil {
			return err
		}
		next, err := execSingle(state, node.child, current, nil)
		if err != nil {
			return err
		}
		current = cloneExecBytes(next)
	}
}

func execRepeat(state execState, node *op, input []byte, buf []byte, fn func([]byte) error) error {
	for {
		if err := execMulti(state, node.child, input, buf, fn); err != nil {
			return err
		}
	}
}

func execUntil(state execState, node *op, input []byte, buf []byte) ([]byte, error) {
	current := cloneExecBytes(input)
	for {
		condVal, err := execSingle(state, node.left, current, nil)
		if err != nil {
			return nil, err
		}
		if !isFalsy(condVal) {
			return append(buf[:0], current...), nil
		}
		next, err := execSingle(state, node.child, current, nil)
		if err != nil {
			return nil, err
		}
		current = cloneExecBytes(next)
	}
}

func execDefScope(state execState, node *op, input []byte, buf []byte, fn func([]byte) error) error {
	baseCtx := state.currentExecContext()
	table := &funcTable{defs: map[string]*funcDef{}, parent: nil}
	if baseCtx != nil {
		table.parent = baseCtx.funcs
	}
	def := *node.fn
	def.capturedFuncs = table
	if baseCtx != nil {
		def.capturedEnv = baseCtx.env
	}
	table.defs[funcKey(def.name, len(def.params))] = &def

	var nextCtx *execContext
	if baseCtx == nil {
		nextCtx = &execContext{funcs: table}
	} else {
		out := *baseCtx
		out.funcs = table
		nextCtx = &out
	}
	return state.withExecContext(nextCtx, func(state execState) error {
		return execMulti(state, node.child, input, buf, fn)
	})
}

func execCall(state execState, node *op, input []byte, buf []byte, fn func([]byte) error) error {
	callerCtx := state.currentExecContext()
	def := callerCtx.lookupFunc(funcKey(node.name, len(node.elems)))
	if def == nil {
		return fmt.Errorf("%s is not defined", funcKey(node.name, len(node.elems)))
	}
	callCtxs, err := bindCallContexts(state, def, callerCtx, node.elems, input)
	if err != nil {
		return err
	}
	for _, callCtx := range callCtxs {
		var outputs [][]byte
		if err := state.withExecContext(callCtx, func(state execState) error {
			return execMulti(state, def.body, input, nil, func(out []byte) error {
				outputs = append(outputs, cloneExecBytes(out))
				return nil
			})
		}); err != nil {
			return err
		}
		for _, out := range outputs {
			emitted := out
			if buf != nil {
				emitted = append(buf[:0], out...)
			}
			if err := fn(emitted); err != nil {
				return err
			}
		}
	}
	return nil
}

func bindCallContexts(state execState, def *funcDef, callerCtx *execContext, args []*op, input []byte) ([]*execContext, error) {
	var callCtx *execContext
	if callerCtx != nil {
		callCtx = &execContext{
			env:    def.capturedEnv,
			funcs:  def.capturedFuncs,
			labels: callerCtx.labels,
			depth:  callerCtx.depth + 1,
		}
	} else {
		callCtx = &execContext{
			env:   def.capturedEnv,
			funcs: def.capturedFuncs,
		}
	}

	var filterDefs map[string]*funcDef
	for i, name := range def.params {
		if !def.valueParams[i] {
			if filterDefs == nil {
				filterDefs = make(map[string]*funcDef)
			}
			filterDefs[funcKey(name, 0)] = &funcDef{
				name:          name,
				body:          args[i],
				capturedEnv:   callerCtxEnv(callerCtx),
				capturedFuncs: callerCtxFuncs(callerCtx),
			}
		}
	}
	if len(filterDefs) > 0 {
		callCtx.funcs = &funcTable{
			defs:   filterDefs,
			parent: callCtx.funcs,
		}
	}

	contexts := []*execContext{callCtx}
	for i, name := range def.params {
		if !def.valueParams[i] {
			continue
		}
		values, err := collectExecOutputs(state, args[i], input)
		if err != nil {
			return nil, err
		}
		if len(values) == 0 {
			return nil, nil
		}
		next := make([]*execContext, 0, len(contexts)*len(values))
		for _, ctx := range contexts {
			for _, value := range values {
				next = append(next, ctx.bindVar(name, value))
			}
		}
		contexts = next
	}
	return contexts, nil
}

func callerCtxEnv(ctx *execContext) *envFrame {
	if ctx == nil {
		return nil
	}
	return ctx.env
}

func callerCtxFuncs(ctx *execContext) *funcTable {
	if ctx == nil {
		return nil
	}
	return ctx.funcs
}

func collectExecOutputs(state execState, node *op, input []byte) ([][]byte, error) {
	var out [][]byte
	err := execMulti(state, node, input, nil, func(val []byte) error {
		out = append(out, cloneExecBytes(val))
		return nil
	})
	return out, err
}

func cloneExecBytes(src []byte) []byte {
	return append([]byte(nil), src...)
}

func execBindAlternatives(state execState, baseCtx *execContext, node *op, input, value []byte, fn func([]byte) error) error {
	base := prebindAlternationNulls(baseCtx, node.pattern, node.altPatterns)
	targets := make([]*bindPattern, 0, 1+len(node.altPatterns))
	targets = append(targets, node.pattern)
	targets = append(targets, node.altPatterns...)

	var lastErr error
	for _, target := range targets {
		nextCtx, err := bindPatternValue(base, target, value)
		if err != nil {
			lastErr = err
			continue
		}
		outputs := make([][]byte, 0, 1)
		err = state.withExecContext(nextCtx, func(state execState) error {
			return execMulti(state, node.right, input, nil, func(val []byte) error {
				outputs = append(outputs, cloneExecBytes(val))
				return nil
			})
		})
		if err != nil {
			lastErr = err
			continue
		}
		for _, out := range outputs {
			if err := fn(out); err != nil {
				return err
			}
		}
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return nil
}

func bindOpValue(ctx *execContext, node *op, value []byte) (*execContext, error) {
	if node.pattern != nil && len(node.altPatterns) > 0 {
		base := prebindAlternationNulls(ctx, node.pattern, node.altPatterns)
		targets := make([]*bindPattern, 0, 1+len(node.altPatterns))
		targets = append(targets, node.pattern)
		targets = append(targets, node.altPatterns...)
		var lastErr error
		for _, target := range targets {
			next, err := bindPatternValue(base, target, value)
			if err == nil {
				return next, nil
			}
			lastErr = err
		}
		if lastErr != nil {
			return nil, lastErr
		}
		return base, nil
	}
	if node.pattern != nil {
		return bindPatternValue(ctx, node.pattern, value)
	}
	return ctx.bindVar(node.name, value), nil
}

func prebindAlternationNulls(ctx *execContext, primary *bindPattern, alts []*bindPattern) *execContext {
	next := ctx
	names := make(map[string]struct{})
	collectPatternVarNames(primary, names)
	for _, alt := range alts {
		collectPatternVarNames(alt, names)
	}
	for name := range names {
		next = next.bindVar(name, bNull)
	}
	return next
}

func collectPatternVarNames(pattern *bindPattern, names map[string]struct{}) {
	if pattern == nil {
		return
	}
	switch pattern.kind {
	case bindPatternVar:
		names[pattern.name] = struct{}{}
	case bindPatternArray:
		for _, elem := range pattern.elems {
			collectPatternVarNames(elem, names)
		}
	case bindPatternObject:
		for _, field := range pattern.fields {
			if field.bindName != "" {
				names[field.bindName] = struct{}{}
			}
			collectPatternVarNames(field.pattern, names)
		}
	}
}

func bindPatternValue(ctx *execContext, pattern *bindPattern, value []byte) (*execContext, error) {
	if pattern == nil {
		return ctx, nil
	}
	switch pattern.kind {
	case bindPatternVar:
		return ctx.bindVar(pattern.name, value), nil
	case bindPatternArray:
		value = trimWhitespace(value)
		if isNull(value) {
			return bindPatternNull(ctx, pattern), nil
		}
		s := scanner{data: value}
		s.skipWhitespace()
		if s.pos >= len(s.data) || s.data[s.pos] != '[' {
			return nil, dynamicIndexNumberAccessError(value)
		}
		elems := make([][]byte, len(pattern.elems))
		s.arrayIter(func(index int, elemStart, elemEnd int) bool {
			if index < len(elems) {
				elems[index] = value[elemStart:elemEnd]
			}
			return index+1 < len(elems)
		})
		next := ctx
		for i, elem := range pattern.elems {
			elemVal := bNull
			if elems[i] != nil {
				elemVal = elems[i]
			}
			var err error
			next, err = bindPatternValue(next, elem, elemVal)
			if err != nil {
				return nil, err
			}
		}
		return next, nil
	case bindPatternObject:
		value = trimWhitespace(value)
		if isNull(value) {
			return bindPatternNull(ctx, pattern), nil
		}
		s := scanner{data: value}
		s.skipWhitespace()
		if s.pos >= len(s.data) || s.data[s.pos] != '{' {
			key := ""
			if len(pattern.fields) > 0 {
				key = pattern.fields[0].key
			}
			return nil, fieldAccessError(value, key)
		}
		next := ctx
		for _, field := range pattern.fields {
			fs := scanner{data: value}
			fs.skipWhitespace()
			vs, ve := fs.findFieldStr(field.key)
			fieldVal := bNull
			if vs != -1 {
				fieldVal = value[vs:ve]
			}
			if field.bindName != "" {
				next = next.bindVar(field.bindName, fieldVal)
			}
			var err error
			next, err = bindPatternValue(next, field.pattern, fieldVal)
			if err != nil {
				return nil, err
			}
		}
		return next, nil
	default:
		return ctx, nil
	}
}

func bindPatternNull(ctx *execContext, pattern *bindPattern) *execContext {
	if pattern == nil {
		return ctx
	}
	switch pattern.kind {
	case bindPatternVar:
		return ctx.bindVar(pattern.name, bNull)
	case bindPatternArray:
		next := ctx
		for _, elem := range pattern.elems {
			next = bindPatternNull(next, elem)
		}
		return next
	case bindPatternObject:
		next := ctx
		for _, field := range pattern.fields {
			if field.bindName != "" {
				next = next.bindVar(field.bindName, bNull)
			}
			next = bindPatternNull(next, field.pattern)
		}
		return next
	default:
		return ctx
	}
}

func execPathsValue(state execState, filter *op, value []byte, frame *pathFrame, buf []byte, fn func([]byte) error) error {
	if frame != nil {
		match, err := pathsFilterMatch(state, filter, value)
		if err != nil {
			return err
		}
		if match {
			out := appendPathFrameJSON(buf[:0], frame)
			if err := fn(out); err != nil {
				return err
			}
		}
	}

	s := scanner{data: value}
	s.skipWhitespace()
	if s.pos >= len(s.data) {
		return nil
	}

	switch s.data[s.pos] {
	case '{':
		var walkErr error
		s.objectIter(func(key []byte, valueStart, valueEnd int) bool {
			childFrame := pathFrame{
				parent: frame,
				kind:   pathStepString,
				rawKey: key,
			}
			walkErr = execPathsValue(state, filter, value[valueStart:valueEnd], &childFrame, buf, fn)
			return walkErr == nil
		})
		return walkErr
	case '[':
		var walkErr error
		s.arrayIter(func(index int, elemStart, elemEnd int) bool {
			childFrame := pathFrame{
				parent: frame,
				kind:   pathStepNumber,
				index:  index,
			}
			walkErr = execPathsValue(state, filter, value[elemStart:elemEnd], &childFrame, buf, fn)
			return walkErr == nil
		})
		return walkErr
	default:
		return nil
	}
}

func pathsFilterMatch(state execState, filter *op, value []byte) (bool, error) {
	if filter == nil {
		return true, nil
	}
	result, err := execSingle(state, filter, value, nil)
	if err != nil {
		return false, err
	}
	return !isFalsy(result), nil
}

func appendPathFrameJSON(dst []byte, frame *pathFrame) []byte {
	dst = append(dst, '[')
	dst = appendPathFrameElems(dst, frame)
	dst = append(dst, ']')
	return dst
}

func appendPathFrameElems(dst []byte, frame *pathFrame) []byte {
	if frame == nil {
		return dst
	}
	if frame.parent != nil {
		dst = appendPathFrameElems(dst, frame.parent)
		dst = append(dst, ',')
	}
	switch frame.kind {
	case pathStepString:
		dst = append(dst, '"')
		dst = appendCanonicalRawJSONStringContent(dst, frame.rawKey)
		dst = append(dst, '"')
	case pathStepNumber:
		dst = appendInt(dst, frame.index)
	}
	return dst
}

func execGetPathOne(input, pathVal, buf []byte) ([]byte, error) {
	ps := scanner{data: pathVal}
	ps.skipWhitespace()
	if ps.pos >= len(ps.data) || ps.data[ps.pos] != '[' {
		return nil, fmt.Errorf("Path must be specified as an array")
	}

	current := input
	var iterErr error
	ps.arrayIter(func(_ int, elemStart, elemEnd int) bool {
		if isNull(current) {
			return true
		}
		next, err := execGetPathStep(current, pathVal[elemStart:elemEnd])
		if err != nil {
			iterErr = err
			return false
		}
		current = next
		return true
	})
	if iterErr != nil {
		return nil, iterErr
	}
	return normalizeOutputValue(current, buf), nil
}

func execGetPathStep(current, step []byte) ([]byte, error) {
	ss := scanner{data: step}
	ss.skipWhitespace()
	if ss.pos >= len(ss.data) {
		return bNull, nil
	}
	if ss.data[ss.pos] == '"' {
		key := ss.readString()
		cs := scanner{data: current}
		cs.skipWhitespace()
		if cs.pos >= len(cs.data) || cs.data[cs.pos] != '{' {
			return nil, getpathAccessError(current, step)
		}
		vs, ve := cs.findField(key)
		if vs == -1 {
			return bNull, nil
		}
		return current[vs:ve], nil
	}

	if f, ok := parseJSONFloat(step); ok {
		idx := int(f)
		cs := scanner{data: current}
		cs.skipWhitespace()
		if cs.pos >= len(cs.data) || cs.data[cs.pos] != '[' {
			return nil, getpathAccessError(current, step)
		}
		if idx < 0 {
			idx = cs.arrayLen() + idx
			if idx < 0 {
				return bNull, nil
			}
		}
		var result []byte
		cs.arrayIter(func(i int, elemStart, elemEnd int) bool {
			if i == idx {
				result = current[elemStart:elemEnd]
				return false
			}
			return true
		})
		if result == nil {
			return bNull, nil
		}
		return result, nil
	}

	return nil, getpathAccessError(current, step)
}

func pathFieldTraceValue(current []byte, field string) []byte {
	cs := scanner{data: current}
	cs.skipWhitespace()
	if cs.pos < len(cs.data) && cs.data[cs.pos] == '{' {
		vs, ve := cs.findFieldStr(field)
		if vs != -1 {
			return current[vs:ve]
		}
	}
	return bNull
}

func pathIndexTraceValue(current []byte, index int) []byte {
	cs := scanner{data: current}
	cs.skipWhitespace()
	if cs.pos >= len(cs.data) || cs.data[cs.pos] != '[' {
		return bNull
	}
	idx := index
	if idx < 0 {
		idx = cs.arrayLen() + idx
		if idx < 0 {
			return bNull
		}
	}
	var result []byte
	cs.arrayIter(func(i int, elemStart, elemEnd int) bool {
		if i == idx {
			result = current[elemStart:elemEnd]
			return false
		}
		return true
	})
	if result == nil {
		return bNull
	}
	return result
}

func pathTraceDynamicIndex(state pathTraceState, key []byte) (pathTraceState, error) {
	key = trimWhitespace(key)
	ks := scanner{data: key}
	ks.skipWhitespace()
	if ks.pos < len(ks.data) && ks.data[ks.pos] == '"' {
		rawKey := ks.readString()
		return pathTraceState{
			value: pathFieldTraceValue(state.value, string(rawKey)),
			frame: &pathFrame{
				parent: state.frame,
				kind:   pathStepString,
				rawKey: rawKey,
			},
		}, nil
	}
	if f, ok := parseJSONFloat(key); ok {
		idx := int(f)
		return pathTraceState{
			value: pathIndexTraceValue(state.value, idx),
			frame: &pathFrame{
				parent: state.frame,
				kind:   pathStepNumber,
				index:  idx,
			},
		}, nil
	}
	return pathTraceState{}, fmt.Errorf("Invalid path expression with result %s", string(trimWhitespace(key)))
}

func invalidPathResultError(value []byte) error {
	return fmt.Errorf("Invalid path expression with result %s", string(trimWhitespace(value)))
}

func invalidPathFieldError(value []byte, field string) error {
	return fmt.Errorf("Invalid path expression near attempt to access element %q of %s", field, string(trimWhitespace(value)))
}

func invalidPathIndexError(value []byte, index int) error {
	return fmt.Errorf("Invalid path expression near attempt to access element %d of %s", index, string(trimWhitespace(value)))
}

func invalidPathDynamicIndexError(value, key []byte) error {
	key = trimWhitespace(key)
	ks := scanner{data: key}
	ks.skipWhitespace()
	if ks.pos < len(ks.data) && ks.data[ks.pos] == '"' {
		return invalidPathFieldError(value, string(ks.readString()))
	}
	if f, ok := parseJSONFloat(key); ok {
		return invalidPathIndexError(value, int(f))
	}
	return fmt.Errorf("Invalid path expression near attempt to access element %s of %s", string(trimWhitespace(key)), string(trimWhitespace(value)))
}

func invalidPathIteratorError(value []byte) error {
	return fmt.Errorf("Invalid path expression near attempt to iterate through %s", string(trimWhitespace(value)))
}

func getpathAccessError(current, step []byte) error {
	ss := scanner{data: step}
	ss.skipWhitespace()
	if ss.pos < len(ss.data) && ss.data[ss.pos] == '"' {
		return fmt.Errorf("Cannot index %s with string %q", jsonTypeName(current), ss.readString())
	}
	return fmt.Errorf("Cannot index %s with %s", jsonTypeName(current), jsonTypeName(step))
}

type pathStepKind int

const (
	pathStepString pathStepKind = iota
	pathStepNumber
	pathStepOther
)

type pathStep struct {
	kind     pathStepKind
	key      []byte
	index    int
	raw      []byte
	typeName string
}

func decodePath(pathVal []byte) ([]pathStep, error) {
	ps := scanner{data: pathVal}
	ps.skipWhitespace()
	if ps.pos >= len(ps.data) || ps.data[ps.pos] != '[' {
		return nil, fmt.Errorf("Path must be specified as an array")
	}
	steps := make([]pathStep, 0, ps.arrayLen())
	ps = scanner{data: pathVal}
	ps.arrayIter(func(_ int, elemStart, elemEnd int) bool {
		raw := trimWhitespace(pathVal[elemStart:elemEnd])
		ss := scanner{data: raw}
		ss.skipWhitespace()
		if ss.pos < len(ss.data) && ss.data[ss.pos] == '"' {
			steps = append(steps, pathStep{
				kind: pathStepString,
				key:  ss.readString(),
				raw:  raw,
			})
			return true
		}
		if f, ok := parseJSONFloat(raw); ok {
			steps = append(steps, pathStep{
				kind:  pathStepNumber,
				index: int(f),
				raw:   raw,
			})
			return true
		}
		steps = append(steps, pathStep{
			kind:     pathStepOther,
			raw:      raw,
			typeName: jsonTypeName(raw),
		})
		return true
	})
	return steps, nil
}

func execSetPath(state execState, node *op, input []byte, buf []byte) ([]byte, error) {
	pathVal, err := execSingle(state, node.left, input, nil)
	if err != nil {
		return nil, err
	}
	value, err := execSingle(state, node.child, input, nil)
	if err != nil {
		return nil, err
	}
	steps, err := decodePath(pathVal)
	if err != nil {
		return nil, &transparentError{err: err}
	}
	out, err := setPathDecoded(input, steps, 0, value, buf)
	if err != nil {
		return nil, &transparentError{err: err}
	}
	return out, nil
}

func execDelPaths(state execState, node *op, input []byte, buf []byte) ([]byte, error) {
	pathsVal, err := execSingle(state, node.child, input, nil)
	if err != nil {
		return nil, err
	}
	ps := scanner{data: pathsVal}
	ps.skipWhitespace()
	if ps.pos >= len(ps.data) || ps.data[ps.pos] != '[' {
		return nil, &transparentError{err: fmt.Errorf("Paths must be specified as an array")}
	}
	current := normalizeOutputValue(input, nil)
	var iterErr error
	ps = scanner{data: pathsVal}
	ps.arrayIter(func(_ int, elemStart, elemEnd int) bool {
		steps, err := decodePath(pathsVal[elemStart:elemEnd])
		if err != nil {
			iterErr = &transparentError{err: fmt.Errorf("Paths must be specified as an array")}
			return false
		}
		next, _, err := delPathDecoded(current, steps, 0, nil)
		if err != nil {
			iterErr = err
			return false
		}
		current = next
		return true
	})
	if iterErr != nil {
		return nil, iterErr
	}
	return normalizeOutputValue(current, buf), nil
}

func setPathDecoded(input []byte, steps []pathStep, depth int, value []byte, buf []byte) ([]byte, error) {
	input = trimWhitespace(input)
	if depth >= len(steps) {
		if buf == nil {
			return trimWhitespace(value), nil
		}
		return append(buf, trimWhitespace(value)...), nil
	}
	step := steps[depth]
	s := scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) {
		input = bNull
		s = scanner{data: input}
		s.skipWhitespace()
	}

	switch step.kind {
	case pathStepString:
		if s.data[s.pos] == 'n' {
			return setPathObjectFromNull(step, steps, depth, value, buf)
		}
		if s.data[s.pos] != '{' {
			return nil, setpathAccessError(input, step)
		}
		return setPathObject(input, step, steps, depth, value, buf)
	case pathStepNumber:
		if s.data[s.pos] == 'n' {
			return setPathArray(bNull, step, steps, depth, value, buf)
		}
		if s.data[s.pos] != '[' {
			return nil, setpathAccessError(input, step)
		}
		return setPathArray(input, step, steps, depth, value, buf)
	default:
		return nil, setpathAccessError(input, step)
	}
}

func setPathObjectFromNull(step pathStep, steps []pathStep, depth int, value []byte, buf []byte) ([]byte, error) {
	child, err := setPathDecoded(bNull, steps, depth+1, value, nil)
	if err != nil {
		return nil, err
	}
	buf = append(buf, '{', '"')
	buf = append(buf, step.key...)
	buf = append(buf, '"', ':')
	buf = append(buf, child...)
	buf = append(buf, '}')
	return buf, nil
}

func setPathObject(input []byte, step pathStep, steps []pathStep, depth int, value []byte, buf []byte) ([]byte, error) {
	s := scanner{data: input}
	s.skipWhitespace()
	buf = append(buf, '{')
	first := true
	found := false
	var iterErr error
	s.objectIter(func(key []byte, valueStart, valueEnd int) bool {
		outVal := input[valueStart:valueEnd]
		if bytesEqual(key, step.key) {
			found = true
			var err error
			outVal, err = setPathDecoded(outVal, steps, depth+1, value, nil)
			if err != nil {
				iterErr = err
				return false
			}
		}
		if !first {
			buf = append(buf, ',')
		}
		first = false
		buf = append(buf, '"')
		buf = append(buf, key...)
		buf = append(buf, '"', ':')
		buf = append(buf, outVal...)
		return true
	})
	if iterErr != nil {
		return nil, iterErr
	}
	if !found {
		outVal, err := setPathDecoded(bNull, steps, depth+1, value, nil)
		if err != nil {
			return nil, err
		}
		if !first {
			buf = append(buf, ',')
		}
		buf = append(buf, '"')
		buf = append(buf, step.key...)
		buf = append(buf, '"', ':')
		buf = append(buf, outVal...)
	}
	buf = append(buf, '}')
	return buf, nil
}

func setPathArray(input []byte, step pathStep, steps []pathStep, depth int, value []byte, buf []byte) ([]byte, error) {
	elems := make([][]byte, 0)
	if !isNull(input) {
		s := scanner{data: input}
		s.skipWhitespace()
		s.arrayIter(func(_ int, elemStart, elemEnd int) bool {
			elems = append(elems, input[elemStart:elemEnd])
			return true
		})
	}
	idx := step.index
	if idx < 0 {
		idx = len(elems) + idx
		if idx < 0 {
			return nil, fmt.Errorf("Out of bounds negative array index")
		}
	}
	if idx > maxArrayUpdateIndex {
		return nil, fmt.Errorf("Array index too large")
	}
	buf = append(buf, '[')
	first := true
	for i := 0; i < idx; i++ {
		var outVal []byte
		if i < len(elems) {
			outVal = elems[i]
		} else {
			outVal = bNull
		}
		if !first {
			buf = append(buf, ',')
		}
		first = false
		buf = append(buf, outVal...)
	}
	var target []byte
	if idx < len(elems) {
		target = elems[idx]
	} else {
		target = bNull
	}
	newVal, err := setPathDecoded(target, steps, depth+1, value, nil)
	if err != nil {
		return nil, err
	}
	if !first {
		buf = append(buf, ',')
	}
	first = false
	buf = append(buf, newVal...)
	for i := idx + 1; i < len(elems); i++ {
		if !first {
			buf = append(buf, ',')
		}
		first = false
		buf = append(buf, elems[i]...)
	}
	buf = append(buf, ']')
	return buf, nil
}

func delPathDecoded(input []byte, steps []pathStep, depth int, buf []byte) ([]byte, bool, error) {
	input = trimWhitespace(input)
	if depth >= len(steps) {
		return nil, true, nil
	}
	s := scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) {
		return input, false, nil
	}
	if s.data[s.pos] == 'n' {
		if buf == nil {
			return bNull, false, nil
		}
		return append(buf, "null"...), false, nil
	}

	step := steps[depth]
	switch step.kind {
	case pathStepString:
		if s.data[s.pos] == '{' {
			return delPathObject(input, step, steps, depth, buf)
		}
		if s.data[s.pos] == '[' {
			return nil, false, fmt.Errorf("Cannot delete string element of array")
		}
		return nil, false, fmt.Errorf("Cannot delete fields from %s", jsonTypeName(input))
	case pathStepNumber:
		if s.data[s.pos] == '[' {
			return delPathArray(input, step, steps, depth, buf)
		}
		if s.data[s.pos] == '{' {
			return nil, false, fmt.Errorf("Cannot delete number field of object")
		}
		return nil, false, fmt.Errorf("Cannot delete fields from %s", jsonTypeName(input))
	default:
		if s.data[s.pos] == '[' {
			return nil, false, fmt.Errorf("Cannot delete %s element of array", step.typeName)
		}
		if s.data[s.pos] == '{' {
			return nil, false, fmt.Errorf("Cannot delete %s field of object", step.typeName)
		}
		return nil, false, fmt.Errorf("Cannot delete fields from %s", jsonTypeName(input))
	}
}

func delPathObject(input []byte, step pathStep, steps []pathStep, depth int, buf []byte) ([]byte, bool, error) {
	s := scanner{data: input}
	s.skipWhitespace()
	buf = append(buf, '{')
	first := true
	found := false
	var iterErr error
	s.objectIter(func(key []byte, valueStart, valueEnd int) bool {
		outVal := input[valueStart:valueEnd]
		omit := false
		if bytesEqual(key, step.key) {
			found = true
			if depth+1 >= len(steps) {
				omit = true
			} else {
				var deleted bool
				var err error
				outVal, deleted, err = delPathDecoded(outVal, steps, depth+1, nil)
				if err != nil {
					iterErr = err
					return false
				}
				omit = deleted
			}
		}
		if omit {
			return true
		}
		if !first {
			buf = append(buf, ',')
		}
		first = false
		buf = append(buf, '"')
		buf = append(buf, key...)
		buf = append(buf, '"', ':')
		buf = append(buf, outVal...)
		return true
	})
	if iterErr != nil {
		return nil, false, iterErr
	}
	if !found {
		return normalizeOutputValue(input, buf[:0]), false, nil
	}
	buf = append(buf, '}')
	return buf, false, nil
}

func delPathArray(input []byte, step pathStep, steps []pathStep, depth int, buf []byte) ([]byte, bool, error) {
	elems := make([][]byte, 0)
	s := scanner{data: input}
	s.skipWhitespace()
	s.arrayIter(func(_ int, elemStart, elemEnd int) bool {
		elems = append(elems, input[elemStart:elemEnd])
		return true
	})
	idx := step.index
	if idx < 0 {
		idx = len(elems) + idx
		if idx < 0 {
			return normalizeOutputValue(input, buf[:0]), false, nil
		}
	}
	if idx >= len(elems) {
		return normalizeOutputValue(input, buf[:0]), false, nil
	}
	buf = append(buf, '[')
	first := true
	for i, elem := range elems {
		omit := false
		outVal := elem
		if i == idx {
			if depth+1 >= len(steps) {
				omit = true
			} else {
				var deleted bool
				var err error
				outVal, deleted, err = delPathDecoded(elem, steps, depth+1, nil)
				if err != nil {
					return nil, false, err
				}
				omit = deleted
			}
		}
		if omit {
			continue
		}
		if !first {
			buf = append(buf, ',')
		}
		first = false
		buf = append(buf, outVal...)
	}
	buf = append(buf, ']')
	return buf, false, nil
}

func setpathAccessError(current []byte, step pathStep) error {
	if step.kind == pathStepString {
		return fmt.Errorf("Cannot index %s with string %q", jsonTypeName(current), step.key)
	}
	if step.kind == pathStepNumber {
		return fmt.Errorf("Cannot index %s with number", jsonTypeName(current))
	}
	if step.typeName == "array" && isArrayValue(current) {
		return fmt.Errorf("Cannot update field at array index of array")
	}
	if step.typeName == "object" && (isNull(current) || isArrayValue(current)) {
		return fmt.Errorf("Array/string slice indices must be integers")
	}
	return fmt.Errorf("Cannot index %s with %s", jsonTypeName(current), step.typeName)
}

func isArrayValue(input []byte) bool {
	s := scanner{data: input}
	s.skipWhitespace()
	return s.pos < len(s.data) && s.data[s.pos] == '['
}

// execAnyAll implements any/all with optional expr argument.
// wantAll=false → any (true if at least one match), wantAll=true → all (true if all match).
func execAnyAll(state execState, node *op, input []byte, buf []byte, fn func([]byte) error, wantAll bool) error {
	result, err := execAnyAllSingle(state, node, input, buf, wantAll)
	if err != nil {
		return err
	}
	return fn(result)
}

func execAnyAllSingle(state execState, node *op, input []byte, buf []byte, wantAll bool) ([]byte, error) {
	// Two-arg form: any(gen; cond) / all(gen; cond)
	if node.left != nil {
		breakFlag := false
		var iterErr error
		err := execMulti(state, node.left, input, nil, func(elem []byte) error {
			condVal, _ := execSingle(state, node.child, elem, nil)
			truthy := !isFalsy(condVal)
			if (!wantAll && truthy) || (wantAll && !truthy) {
				breakFlag = true
				return errBreak
			}
			return nil
		})
		if err == errBreak {
			err = nil
		}
		if iterErr != nil {
			return nil, iterErr
		}
		if err != nil {
			return nil, err
		}
		if wantAll {
			return boolResult(buf, !breakFlag), nil
		}
		return boolResult(buf, breakFlag), nil
	}

	s := &scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) {
		// empty input: any→false, all→true (vacuous truth)
		return boolResult(buf, wantAll), nil
	}

	found := false      // for any: true if a match was found
	falseFound := false // for all: true if a non-match was found

	check := func(elem []byte) bool {
		var truthy bool
		if node.child == nil {
			truthy = !isFalsy(elem)
		} else {
			condVal, _ := execSingle(state, node.child, elem, nil)
			truthy = !isFalsy(condVal)
		}
		if !wantAll && truthy {
			found = true
			return false // stop early
		}
		if wantAll && !truthy {
			falseFound = true
			return false // stop early
		}
		return true
	}

	switch s.data[s.pos] {
	case '[':
		s.arrayIter(func(_ int, start, end int) bool {
			return check(input[start:end])
		})
	case '{':
		s.objectIter(func(_ []byte, start, end int) bool {
			return check(input[start:end])
		})
	default:
		return boolResult(buf, false), nil
	}

	if wantAll {
		return boolResult(buf, !falseFound), nil
	}
	return boolResult(buf, found), nil
}

// boolResult returns bTrue/bFalse when buf is nil, or appends "true"/"false" to buf.
func boolResult(buf []byte, v bool) []byte {
	if v {
		if buf == nil {
			return bTrue
		}
		return append(buf, "true"...)
	}
	if buf == nil {
		return bFalse
	}
	return append(buf, "false"...)
}

// execAsciiCase converts a JSON string to lower (upcase=false) or upper (upcase=true) case.
// Escape sequences are copied unchanged. Non-string input returns an error.
func execAsciiCase(input []byte, buf []byte, upcase bool) ([]byte, error) {
	s := &scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '"' {
		if upcase {
			return nil, fmt.Errorf("ascii_upcase input must be a string")
		}
		return nil, fmt.Errorf("ascii_downcase input must be a string")
	}
	buf = append(buf, '"')
	s.pos++ // skip opening '"'
	for s.pos < len(s.data) {
		ch := s.data[s.pos]
		if ch == '\\' {
			if s.pos+1 >= len(s.data) {
				// Truncated escape: there is no second byte to copy, and
				// continuing without advancing pos spins forever. Stop and let
				// the closing quote below terminate the output, which drops the
				// dangling backslash rather than emitting an unclosed escape.
				break
			}
			buf = append(buf, ch, s.data[s.pos+1])
			s.pos += 2
			continue
		}
		if ch == '"' {
			s.pos++
			break
		}
		if upcase {
			if ch >= 'a' && ch <= 'z' {
				ch -= 32
			}
		} else {
			if ch >= 'A' && ch <= 'Z' {
				ch += 32
			}
		}
		buf = append(buf, ch)
		s.pos++
	}
	buf = append(buf, '"')
	return buf, nil
}

// execExplode converts a JSON string to a JSON array of Unicode codepoints.
// "ABC" → [65,66,67]. JSON escape sequences are decoded first.
// Allocation model: Tier 2 — O(n) proportional to string length.
func execExplode(input []byte, buf []byte) ([]byte, error) {
	s := &scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '"' {
		return nil, fmt.Errorf("explode input must be a string")
	}
	content := s.readString() // raw bytes between quotes (may contain \uXXXX etc.)
	// Decode JSON escapes → UTF-8 bytes
	decoded := decodeJSONStringContent(nil, content)
	// Walk UTF-8 bytes, emit codepoints as JSON array
	buf = append(buf[:0], '[')
	first := true
	for len(decoded) > 0 {
		var r rune
		var size int
		b := decoded[0]
		switch {
		case b < 0x80:
			r, size = rune(b), 1
		case b < 0xE0:
			if len(decoded) >= 2 {
				r = rune(b&0x1F)<<6 | rune(decoded[1]&0x3F)
				size = 2
			} else {
				r, size = 0xFFFD, 1
			}
		case b < 0xF0:
			if len(decoded) >= 3 {
				r = rune(b&0x0F)<<12 | rune(decoded[1]&0x3F)<<6 | rune(decoded[2]&0x3F)
				size = 3
			} else {
				r, size = 0xFFFD, 1
			}
		default:
			if len(decoded) >= 4 {
				r = rune(b&0x07)<<18 | rune(decoded[1]&0x3F)<<12 | rune(decoded[2]&0x3F)<<6 | rune(decoded[3]&0x3F)
				size = 4
			} else {
				r, size = 0xFFFD, 1
			}
		}
		decoded = decoded[size:]
		if !first {
			buf = append(buf, ',')
		}
		first = false
		buf = strconv.AppendInt(buf, int64(r), 10)
	}
	buf = append(buf, ']')
	return buf, nil
}

// execImplode converts a JSON array of Unicode codepoints to a JSON string.
// [65,66,67] → "ABC". Invalid codepoints (negative, > U+10FFFF, surrogates) → U+FFFD.
// Non-integer floats are truncated toward zero.
// Allocation model: Tier 2 — O(n) proportional to array length.
func execImplode(input []byte, buf []byte) ([]byte, error) {
	s := &scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '[' {
		return nil, fmt.Errorf("implode input must be an array")
	}
	// Build raw UTF-8 bytes, then re-encode as JSON string
	var utf8Bytes []byte
	var iterErr error
	s.arrayIter(func(_ int, start, end int) bool {
		elem := input[start:end]
		f, ok := parseJSONFloat(elem)
		if !ok || math.IsNaN(f) || math.IsInf(f, 0) {
			iterErr = fmt.Errorf("implode: element is not a valid unicode codepoint")
			return false
		}
		// Truncate toward zero (1.9 → 1, -1.9 → -1)
		cp := int32(f)
		// Replace invalid codepoints with U+FFFD
		if cp < 0 || cp > 0x10FFFF || (cp >= 0xD800 && cp <= 0xDFFF) {
			cp = 0xFFFD
		}
		utf8Bytes = appendRuneUTF8(utf8Bytes, rune(cp))
		return true
	})
	if iterErr != nil {
		return nil, iterErr
	}
	// Encode UTF-8 bytes as JSON string
	buf = append(buf[:0], '"')
	buf = appendJSONStringContent(buf, utf8Bytes)
	buf = append(buf, '"')
	return buf, nil
}

// execIsNaN returns true if the input is the NaN sentinel or a float that is NaN.
func execIsNaN(input, buf []byte) []byte {
	f, ok := parseJSONFloat(trimWhitespace(input))
	if ok && math.IsNaN(f) {
		if buf == nil {
			return bTrue
		}
		return append(buf[:0], "true"...)
	}
	if buf == nil {
		return bFalse
	}
	return append(buf[:0], "false"...)
}

// execIsInfinite returns true if the input is ±infinite.
func execIsInfinite(input, buf []byte) []byte {
	f, ok := parseJSONFloat(trimWhitespace(input))
	if ok && math.IsInf(f, 0) {
		if buf == nil {
			return bTrue
		}
		return append(buf[:0], "true"...)
	}
	if buf == nil {
		return bFalse
	}
	return append(buf[:0], "false"...)
}

// execIsFinite returns true if the input is a finite number (not NaN, not infinite).
func execIsFinite(input, buf []byte) []byte {
	f, ok := parseJSONFloat(trimWhitespace(input))
	if ok && !math.IsNaN(f) && !math.IsInf(f, 0) {
		if buf == nil {
			return bTrue
		}
		return append(buf[:0], "true"...)
	}
	if buf == nil {
		return bFalse
	}
	return append(buf[:0], "false"...)
}

// execIsNormal returns true if the input is a normal number (finite, nonzero, not subnormal).
func execIsNormal(input, buf []byte) []byte {
	f, ok := parseJSONFloat(trimWhitespace(input))
	if ok && !math.IsNaN(f) && !math.IsInf(f, 0) && f != 0 {
		if buf == nil {
			return bTrue
		}
		return append(buf[:0], "true"...)
	}
	if buf == nil {
		return bFalse
	}
	return append(buf[:0], "false"...)
}

// execPow implements pow(x; y) = math.Pow(x, y).
func execPow(state execState, node *op, input []byte, buf []byte) ([]byte, error) {
	xVal, err := execSingle(state, node.left, input, nil)
	if err != nil {
		return nil, err
	}
	yVal, err := execSingle(state, node.right, input, nil)
	if err != nil {
		return nil, err
	}
	xf, xok := parseJSONFloat(xVal)
	yf, yok := parseJSONFloat(yVal)
	if !xok || !yok {
		return nil, fmt.Errorf("pow inputs must be numbers")
	}
	return appendNumber(buf[:0], math.Pow(xf, yf)), nil
}

func execHypot(state execState, node *op, input []byte, buf []byte) ([]byte, error) {
	xVal, err := execSingle(state, node.left, input, nil)
	if err != nil {
		return nil, err
	}
	yVal, err := execSingle(state, node.right, input, nil)
	if err != nil {
		return nil, err
	}
	xf, xok := parseJSONFloat(xVal)
	yf, yok := parseJSONFloat(yVal)
	if !xok || !yok {
		return nil, fmt.Errorf("hypot inputs must be numbers")
	}
	return appendNumber(buf[:0], math.Hypot(xf, yf)), nil
}

func execFMA(state execState, node *op, input []byte, buf []byte) ([]byte, error) {
	xVal, err := execSingle(state, node.left, input, nil)
	if err != nil {
		return nil, err
	}
	yVal, err := execSingle(state, node.right, input, nil)
	if err != nil {
		return nil, err
	}
	zVal, err := execSingle(state, node.child, input, nil)
	if err != nil {
		return nil, err
	}
	xf, xok := parseJSONFloat(xVal)
	yf, yok := parseJSONFloat(yVal)
	zf, zok := parseJSONFloat(zVal)
	if !xok || !yok || !zok {
		return nil, fmt.Errorf("fma inputs must be numbers")
	}
	return appendNumber(buf[:0], math.FMA(xf, yf, zf)), nil
}

// execStringPredicate implements startswith and endswith.
// Returns bTrue/bFalse when buf is nil (zero-alloc in condition context).
func execStringPredicate(input []byte, buf []byte, s string, start, end bool) []byte {
	sc := &scanner{data: input}
	sc.skipWhitespace()
	if sc.pos >= len(sc.data) || sc.data[sc.pos] != '"' {
		return boolResult(buf, false)
	}
	content := sc.readString()
	var match bool
	if start {
		match = len(content) >= len(s) && bytesEqualStr(content[:len(s)], s)
	} else { // end
		match = len(content) >= len(s) && bytesEqualStr(content[len(content)-len(s):], s)
	}
	return boolResult(buf, match)
}

func resolveStringLikeArg(state execState, node *op, input []byte) ([]byte, error) {
	if node.child == nil {
		return []byte(node.field), nil
	}
	value, err := execSingle(state, node.child, input, nil)
	if err != nil {
		return nil, err
	}
	sc := &scanner{data: value}
	sc.skipWhitespace()
	if sc.pos >= len(sc.data) || sc.data[sc.pos] != '"' {
		return nil, fmt.Errorf("string argument must be a string")
	}
	return sc.readString(), nil
}

// execTrimStr implements ltrimstr (left=true) and rtrimstr (left=false).
// If the input string starts/ends with s, returns the trimmed string.
// If no match, returns the input unchanged (cap-limited zero-alloc sub-slice when buf is nil).
func execTrimStr(state execState, node *op, input []byte, buf []byte, left bool) ([]byte, error) {
	s, err := resolveStringLikeArg(state, node, input)
	if err != nil {
		if left {
			return nil, &transparentError{err: fmt.Errorf("startswith() requires string inputs")}
		}
		return nil, &transparentError{err: fmt.Errorf("endswith() requires string inputs")}
	}
	sc := &scanner{data: input}
	sc.skipWhitespace()
	start := sc.pos
	if sc.pos >= len(sc.data) || sc.data[sc.pos] != '"' {
		if left {
			return nil, &transparentError{err: fmt.Errorf("startswith() requires string inputs")}
		}
		return nil, &transparentError{err: fmt.Errorf("endswith() requires string inputs")}
	}
	content := sc.readString()
	end := sc.pos

	var match bool
	var trimmed []byte
	if left {
		match = len(content) >= len(s) && bytesEqualStr(content[:len(s)], string(s))
		if match {
			trimmed = content[len(s):]
		}
	} else {
		match = len(content) >= len(s) && bytesEqualStr(content[len(content)-len(s):], string(s))
		if match {
			trimmed = content[:len(content)-len(s)]
		}
	}

	if !match {
		if buf == nil {
			return input[start:end:end], nil
		}
		return append(buf, input[start:end]...), nil
	}

	buf = append(buf, '"')
	buf = append(buf, trimmed...)
	buf = append(buf, '"')
	return buf, nil
}

func execTrimStrBoth(state execState, node *op, input []byte, buf []byte) ([]byte, error) {
	s, err := resolveStringLikeArg(state, node, input)
	if err != nil {
		return nil, &transparentError{err: fmt.Errorf("startswith() requires string inputs")}
	}
	leftNode := &op{field: string(s)}
	left, err := execTrimStr(state, leftNode, input, nil, true)
	if err != nil {
		return nil, err
	}
	rightNode := &op{field: string(s)}
	return execTrimStr(state, rightNode, left, buf, false)
}

type trimMode int

const (
	trimBoth trimMode = iota
	trimLeft
	trimRight
)

func execTrim(input []byte, buf []byte, mode trimMode) ([]byte, error) {
	sc := &scanner{data: input}
	sc.skipWhitespace()
	if sc.pos >= len(sc.data) || sc.data[sc.pos] != '"' {
		return nil, fmt.Errorf("trim input must be a string")
	}
	raw := sc.readString()
	decoded := decodeJSONStringContent(nil, raw)

	var trimmed string
	switch mode {
	case trimLeft:
		trimmed = strings.TrimLeftFunc(string(decoded), unicode.IsSpace)
	case trimRight:
		trimmed = strings.TrimRightFunc(string(decoded), unicode.IsSpace)
	default:
		trimmed = strings.TrimFunc(string(decoded), unicode.IsSpace)
	}

	buf = append(buf, '"')
	buf = appendJSONStringContent(buf, []byte(trimmed))
	buf = append(buf, '"')
	return buf, nil
}

func execUTF8ByteLength(input []byte, buf []byte) ([]byte, error) {
	sc := &scanner{data: input}
	sc.skipWhitespace()
	if sc.pos >= len(sc.data) || sc.data[sc.pos] != '"' {
		raw := trimWhitespace(input)
		return nil, fmt.Errorf("%s (%s) only strings have UTF-8 byte length", jsonTypeName(raw), raw)
	}
	raw := sc.readString()
	decoded := decodeJSONStringContent(nil, raw)
	return appendInt(buf[:0], len(decoded)), nil
}

func execReverse(input []byte, buf []byte) ([]byte, error) {
	input = trimWhitespace(input)
	s := scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '[' {
		return nil, fmt.Errorf("reverse input must be an array")
	}
	var elems [][]byte
	s.arrayIter(func(_ int, elemStart, elemEnd int) bool {
		elems = append(elems, input[elemStart:elemEnd])
		return true
	})
	buf = append(buf, '[')
	for i := len(elems) - 1; i >= 0; i-- {
		if i != len(elems)-1 {
			buf = append(buf, ',')
		}
		buf = append(buf, elems[i]...)
	}
	buf = append(buf, ']')
	return buf, nil
}

// execHas checks field membership (object) or index bounds (array).
// For objects: true if the field exists, even if its value is null.
// For arrays: true if the index is within bounds (node.literal == "array").
func execHas(node *op, input []byte, buf []byte, fn func([]byte) error) error {
	s := &scanner{data: input}
	s.skipWhitespace()
	if len(node.literal) > 0 && node.literal[0] == 'a' {
		// Array index check: has(n)
		if s.pos >= len(s.data) || s.data[s.pos] != '[' {
			return fn(boolResult(buf, false))
		}
		// jq: has(n) on array requires n >= 0; negative indices are not bounds-adjusted
		length := s.arrayLen()
		idx := node.index
		return fn(boolResult(buf, idx >= 0 && idx < length))
	}
	// Object key check: has("field")
	if s.pos >= len(s.data) || s.data[s.pos] != '{' {
		return fn(boolResult(buf, false))
	}
	key := []byte(node.field)
	vs, _ := s.findField(key)
	return fn(boolResult(buf, vs != -1))
}

// isSingleOutputOp reports whether op always produces exactly one output.
// Used by execIf and execAlternative to avoid closure allocation in the common case.
func isSingleOutputOp(o *op) bool {
	switch o.typ {
	case opOptional:
		return isSingleOutputOp(o.child)
	case opIdentity, opField, opIndex, opLiteral, opCompare, opAnd, opOr, opNot,
		opLength, opHas, opIn, opINBuiltin, opSlice, opPlus, opMinus, opMul, opDiv, opMod,
		opAdd, opFlatten, opSelect, opAlternative, opTypeBuiltin, opToEntries,
		opFromEntries, opToJSON, opFromJSON, opToString, opToNumber, opToBoolean,
		opBase64, opBase64D, opAsciiDowncase, opAsciiUpcase,
		opStartsWith, opEndsWith, opSplit, opJoin, opTrim, opLtrim, opRtrim,
		opTrimStr, opLtrimStr, opRtrimStr, opHaveDecnum, opUTF8ByteLength,
		opReverse, opPick, opINDEXBuiltin, opUntil, opURIEncode, opTodate, opNow,
		opHypot, opFMA,
		opDebug:
		return true
	case opPipe:
		// Pipe is single-output if both sides are
		return isSingleOutputOp(o.left) && isSingleOutputOp(o.right)
	default:
		return false
	}
}

// execIf evaluates cond; if truthy runs the then-branch, otherwise the else-branch.
// If no else-branch is present (child==nil), the else defaults to identity.
// Uses execMulti for the condition so that empty conditions produce no outputs,
// and multiple condition outputs each independently select their branch.
func execIf(state execState, node *op, input []byte, buf []byte, fn func([]byte) error) error {
	// Fast path for single-output conditions (common case): avoid closure allocation.
	if isSingleOutputOp(node.left) {
		condVal, err := execSingle(state, node.left, input, nil)
		if err != nil {
			return err
		}
		if !isFalsy(condVal) {
			return execMulti(state, node.right, input, buf, fn)
		}
		if node.child != nil {
			return execMulti(state, node.child, input, buf, fn)
		}
		return fn(input)
	}
	return execMulti(state, node.left, input, nil, func(condVal []byte) error {
		if !isFalsy(condVal) {
			return execMulti(state, node.right, input, buf, fn)
		}
		if node.child != nil {
			return execMulti(state, node.child, input, buf, fn)
		}
		// default else: identity
		return fn(input)
	})
}

// execAlternative collects all truthy outputs from left; if none, evaluates right.
// jq semantics: (null, false, 3) // 18 → 3 (truthy outputs pass through).
//
//	(null, false) // 18 → 18 (no truthy outputs, use right).
func execAlternative(state execState, node *op, input []byte, buf []byte, fn func([]byte) error) error {
	anyTruthy := false
	err := execMulti(state, node.left, input, buf, func(result []byte) error {
		if isFalsy(result) {
			return nil // skip falsy outputs
		}
		anyTruthy = true
		return fn(result)
	})
	if err != nil {
		return err
	}
	if !anyTruthy {
		// No truthy outputs from left — use right side
		return execMulti(state, node.right, input, buf, fn)
	}
	return nil
}

// execArithValues computes a binary arithmetic operation on pre-evaluated values.
// This is the core logic, separated from operand evaluation to support multi-output left sides.
func execArithValues(typ opType, leftVal, rightVal, buf []byte) ([]byte, error) {
	ls := scanner{data: leftVal}
	ls.skipWhitespace()
	rs := scanner{data: rightVal}
	rs.skipWhitespace()

	// null propagates through arithmetic (null op x = null)
	lIsNull := ls.pos >= len(ls.data) || ls.data[ls.pos] == 'n'
	rIsNull := rs.pos >= len(rs.data) || rs.data[rs.pos] == 'n'
	if lIsNull || rIsNull {
		if buf == nil {
			return bNull, nil
		}
		return append(buf, "null"...), nil
	}

	// Only call parseJSONFloat when the value looks like a number — avoids
	// strconv.ParseFloat allocating error objects on non-numeric inputs.
	var lf, rf float64
	var lok, rok bool
	if ls.pos < len(ls.data) && isNumberByte(ls.data[ls.pos]) {
		lf, lok = parseJSONFloat(leftVal)
	}
	if rs.pos < len(rs.data) && isNumberByte(rs.data[rs.pos]) {
		rf, rok = parseJSONFloat(rightVal)
	}

	switch typ {
	case opMinus:
		if lok && rok {
			return appendNumber(buf, lf-rf), nil
		}
		// array difference: elements of left not present in right (O(n²), zero-alloc)
		if ls.pos < len(ls.data) && ls.data[ls.pos] == '[' &&
			rs.pos < len(rs.data) && rs.data[rs.pos] == '[' {
			return execArrayDiff(leftVal, rightVal, buf), nil
		}
		return nil, fmt.Errorf("%s and %s cannot be subtracted", jqTypeValueForError(leftVal), jqTypeValueForError(rightVal))

	case opMul:
		if lok && rok {
			return appendNumber(buf, lf*rf), nil
		}
		// string * n or n * string: repeat string n times; negative → null, 0 or 0<n<1 → ""
		var strVal []byte
		var numF float64
		if ls.pos < len(ls.data) && ls.data[ls.pos] == '"' && rok {
			strVal = leftVal
			numF = rf
		} else if rs.pos < len(rs.data) && rs.data[rs.pos] == '"' && lok {
			strVal = rightVal
			numF = lf
		}
		if strVal != nil {
			if math.IsNaN(numF) || math.IsInf(numF, 0) || numF < 0 {
				// nan, infinite, or negative count: null
				if buf == nil {
					return bNull, nil
				}
				return append(buf, "null"...), nil
			}
			n := int(numF) // floor
			if n == 0 {
				return append(buf, `""`...), nil
			}
			sv := scanner{data: strVal}
			sv.skipWhitespace()
			strContent := sv.readString()
			buf = append(buf, '"')
			for i := 0; i < n; i++ {
				buf = append(buf, strContent...)
			}
			return append(buf, '"'), nil
		}
		// object * object: recursive merge
		if ls.pos < len(ls.data) && ls.data[ls.pos] == '{' &&
			rs.pos < len(rs.data) && rs.data[rs.pos] == '{' {
			return execObjectMerge(leftVal, rightVal, buf), nil
		}
		return nil, fmt.Errorf("cannot multiply %q and %q", leftVal, rightVal)

	case opDiv:
		if lok && rok {
			if rf == 0 {
				return nil, fmt.Errorf("number (%s) and number (%s) cannot be divided because the divisor is zero",
					string(leftVal), string(rightVal))
			}
			return appendNumber(buf, lf/rf), nil
		}
		// string / string: split (same as split builtin)
		if ls.pos < len(ls.data) && ls.data[ls.pos] == '"' &&
			rs.pos < len(rs.data) && rs.data[rs.pos] == '"' {
			sep := rs.readString()
			return execSplit(leftVal, buf, string(sep)), nil
		}
		return nil, fmt.Errorf("cannot divide %q by %q", leftVal, rightVal)

	case opMod:
		if lok && rok {
			if rf == 0 {
				return nil, fmt.Errorf("number (%s) and number (%s) cannot be divided (remainder) because the divisor is zero",
					string(leftVal), string(rightVal))
			}
			if math.IsInf(lf, 0) {
				if math.IsInf(rf, 1) {
					if math.IsInf(lf, -1) {
						return appendInt(buf, -1), nil
					}
					return appendInt(buf, 0), nil
				}
				if !math.IsInf(rf, 0) && !math.IsNaN(rf) {
					return appendInt(buf, 0), nil
				}
			}
			// integer modulo when both operands are integral
			li, ri := int64(lf), int64(rf)
			if lf == float64(li) && rf == float64(ri) {
				return appendInt(buf, int(li%ri)), nil
			}
			return appendNumber(buf, math.Mod(lf, rf)), nil
		}
		return nil, fmt.Errorf("cannot modulo %q by %q", leftVal, rightVal)
	}
	return nil, fmt.Errorf("unknown arithmetic op %d", typ)
}

// execArith implements binary arithmetic: -, *, /, %.
// Both operands are evaluated with nil scratch (zero-alloc sub-slices).
// Supports: number op number, array - array (difference), string * n (repeat), string / string (split).
func execArith(state execState, node *op, input []byte, buf []byte) ([]byte, error) {
	leftVal, err := execSingle(state, node.left, input, nil)
	if err != nil {
		return nil, err
	}
	rightVal, err := execSingle(state, node.right, input, nil)
	if err != nil {
		return nil, err
	}
	return execArithValues(node.typ, leftVal, rightVal, buf)
}

// execObjectMerge performs a recursive merge of two JSON objects (obj1 * obj2).
// Key ordering: all left keys first (using right value if present, recursively merging if both objects),
// then right-only keys appended at end.
func execObjectMerge(left, right, buf []byte) []byte {
	buf = append(buf, '{')
	first := true

	ls := scanner{data: left}
	ls.skipWhitespace()
	rs := scanner{data: right}
	rs.skipWhitespace()

	// Pass 1: emit all left keys (with merged/overridden values)
	ls.objectIter(func(key []byte, lvStart, lvEnd int) bool {
		leftVal := left[lvStart:lvEnd]
		// Check if right has this key
		rls := scanner{data: right}
		rls.skipWhitespace()
		rvs, rve := rls.findField(key)
		if !first {
			buf = append(buf, ',')
		}
		first = false
		buf = append(buf, '"')
		buf = append(buf, key...)
		buf = append(buf, '"', ':')
		if rvs == -1 {
			// Right doesn't have this key — use left value
			buf = append(buf, leftVal...)
		} else {
			rightVal := right[rvs:rve]
			// Both have the key — check if both values are objects (recursive merge)
			lv := scanner{data: leftVal}
			lv.skipWhitespace()
			rv := scanner{data: rightVal}
			rv.skipWhitespace()
			if lv.pos < len(lv.data) && lv.data[lv.pos] == '{' &&
				rv.pos < len(rv.data) && rv.data[rv.pos] == '{' {
				buf = execObjectMerge(leftVal, rightVal, buf)
			} else {
				// Right wins
				buf = append(buf, rightVal...)
			}
		}
		return true
	})

	// Pass 2: emit right-only keys (keys not in left)
	rs.objectIter(func(key []byte, rvStart, rvEnd int) bool {
		rightVal := right[rvStart:rvEnd]
		// Skip if left has this key (already emitted in pass 1)
		lls := scanner{data: left}
		lls.skipWhitespace()
		lvs, _ := lls.findField(key)
		if lvs != -1 {
			return true // already handled
		}
		if !first {
			buf = append(buf, ',')
		}
		first = false
		buf = append(buf, '"')
		buf = append(buf, key...)
		buf = append(buf, '"', ':')
		buf = append(buf, rightVal...)
		return true
	})

	buf = append(buf, '}')
	return buf
}

// execArrayDiff returns elements of left that do not appear in right (O(n²), zero-alloc).
// Both inputs must be JSON arrays.
func execArrayDiff(left, right, buf []byte) []byte {
	buf = append(buf, '[')
	first := true
	ls := scanner{data: left}
	ls.arrayIter(func(_ int, lStart, lEnd int) bool {
		leftElem := left[lStart:lEnd]
		if !arrayContainsElem(right, leftElem) {
			if !first {
				buf = append(buf, ',')
			}
			first = false
			buf = append(buf, leftElem...)
		}
		return true
	})
	return append(buf, ']')
}

// arrayContainsElem reports whether the JSON array arr contains an element equal to elem.
// Uses a manual scan loop (no closure) to avoid any heap allocation.
func arrayContainsElem(arr, elem []byte) bool {
	s := scanner{data: arr}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '[' {
		return false
	}
	s.pos++ // skip '['
	for s.pos < len(s.data) && s.data[s.pos] != ']' {
		s.skipWhitespace()
		start := s.pos
		s.skipValue()
		end := s.pos
		if jsonEqual(arr[start:end], elem) {
			return true
		}
		s.skipWhitespace()
		if s.pos < len(s.data) && s.data[s.pos] == ',' {
			s.pos++
		} else {
			break
		}
	}
	return false
}

// execMinMax finds the minimum or maximum element of a JSON array.
// node.child, if non-nil, is the key function (for min_by/max_by).
// Empty array returns null. Non-array input returns an error.
func execMinMax(state execState, input []byte, buf []byte, node *op, wantMax bool) ([]byte, error) {
	s := scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '[' {
		return nil, fmt.Errorf("min/max input must be an array")
	}

	var best []byte    // best element (sub-slice of input)
	var bestKey []byte // key bytes used for comparison

	var iterErr error
	s.arrayIter(func(_ int, start, end int) bool {
		elem := input[start:end]
		var key []byte
		if node.child != nil {
			k, err := execSingle(state, node.child, elem, nil)
			if err != nil {
				iterErr = err
				return false
			}
			key = k
		} else {
			key = elem
		}
		if best == nil {
			best = elem
			bestKey = key
			return true
		}
		cmp := compareJSONOrder(key, bestKey)
		// wantMax: update on >=0 to keep the last element among equals (stable max).
		// !wantMax: update on <0 to keep the first element among equals (stable min).
		if (wantMax && cmp >= 0) || (!wantMax && cmp < 0) {
			best = elem
			bestKey = key
		}
		return true
	})
	if iterErr != nil {
		return nil, iterErr
	}
	if best == nil {
		return append(buf, "null"...), nil // empty array
	}
	if buf == nil {
		return best[:len(best):len(best)], nil
	}
	return append(buf, best...), nil
}

// --- sort / sort_by / unique / unique_by / group_by / transpose (Tier 2) ---

// collectArrayElems gathers all elements of a JSON array as sub-slices of input.
// Returns an error if input is not an array.
// Allocates one [][]byte proportional to the element count (Tier 2).
func collectArrayElems(input []byte) ([][]byte, error) {
	s := scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '[' {
		return nil, fmt.Errorf("sort input must be an array")
	}
	var elems [][]byte
	s.arrayIter(func(_ int, start, end int) bool {
		elems = append(elems, input[start:end:end])
		return true
	})
	return elems, nil
}

// collectElemKeys evaluates key function node against each element of a JSON array
// and returns parallel slices: original elements (sub-slices of input) and their
// computed key sequences.
// Keys are collected with nil buf so field-access and identity keys are sub-slices
// of input (no copy). Keys that require computation (arithmetic, etc.) allocate
// their own buffers. Elements are also sub-slices of input (no copy).
func collectElemKeys(state execState, node *op, input []byte) (elems [][]byte, keys [][][]byte, err error) {
	s := scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '[' {
		return nil, nil, fmt.Errorf("sort_by/group_by/unique_by input must be an array")
	}
	s.arrayIter(func(_ int, start, end int) bool {
		elem := input[start:end:end]
		var ks [][]byte
		execErr := execMulti(state, node, elem, nil, func(k []byte) error {
			// k is valid for the lifetime of this call stack; since we collect
			// it into ks which persists, we must ensure it stays valid.
			// With nil buf, field-access returns a cap-limited sub-slice of elem
			// (which is a sub-slice of input — valid for the whole sort).
			// Constructed values allocate their own buffers and are also safe.
			ks = append(ks, k)
			return nil
		})
		if execErr != nil {
			err = execErr
			return false
		}
		elems = append(elems, elem)
		keys = append(keys, ks)
		return true
	})
	return
}

// compareKeySeqs compares two key sequences lexicographically using compareJSONOrder.
func compareKeySeqs(a, b [][]byte) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if c := compareJSONOrder(a[i], b[i]); c != 0 {
			return c
		}
	}
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	return 0
}

// keySeqsEqual returns true if two key sequences are equal by compareJSONOrder.
func keySeqsEqual(a, b [][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if compareJSONOrder(a[i], b[i]) != 0 {
			return false
		}
	}
	return true
}

// emitArray writes elements as a JSON array into buf and calls fn.
func emitArray(elems [][]byte, buf []byte, fn func([]byte) error) error {
	buf = append(buf[:0], '[')
	for i, e := range elems {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = append(buf, e...)
	}
	buf = append(buf, ']')
	return fn(buf)
}

// execSort sorts a JSON array using jq's canonical type ordering.
func execSort(input []byte, buf []byte, fn func([]byte) error) error {
	elems, err := collectArrayElems(input)
	if err != nil {
		return err
	}
	sort.SliceStable(elems, func(i, j int) bool {
		return compareJSONOrder(elems[i], elems[j]) < 0
	})
	return emitArray(elems, buf, fn)
}

// execSortBy sorts a JSON array by a key function.
// sort_by(.a, .b) uses a generator as key; elements are ordered by the
// tuple of key values, compared lexicographically.
func execSortBy(state execState, node *op, input []byte, buf []byte, fn func([]byte) error) error {
	elems, keys, err := collectElemKeys(state, node.child, input)
	if err != nil {
		return err
	}
	// Sort an index array to avoid mismatching precomputed keys with reordered elements.
	idx := make([]int, len(elems))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool {
		return compareKeySeqs(keys[idx[a]], keys[idx[b]]) < 0
	})
	// Emit in sorted order
	buf = append(buf[:0], '[')
	for i, si := range idx {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = append(buf, elems[si]...)
	}
	buf = append(buf, ']')
	return fn(buf)
}

// execUnique removes duplicate elements from a sorted JSON array.
func execUnique(input []byte, buf []byte, fn func([]byte) error) error {
	elems, err := collectArrayElems(input)
	if err != nil {
		return err
	}
	sort.SliceStable(elems, func(i, j int) bool {
		return compareJSONOrder(elems[i], elems[j]) < 0
	})
	// Remove consecutive duplicates
	out := elems[:0]
	for i, e := range elems {
		if i == 0 || compareJSONOrder(elems[i-1], e) != 0 {
			out = append(out, e)
		}
	}
	return emitArray(out, buf, fn)
}

// execUniqueBy removes elements whose key function produces the same value as
// the preceding element (after sorting by key).
func execUniqueBy(state execState, node *op, input []byte, buf []byte, fn func([]byte) error) error {
	elems, keys, err := collectElemKeys(state, node.child, input)
	if err != nil {
		return err
	}
	// Build index slice and sort by key
	idx := make([]int, len(elems))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool {
		return compareKeySeqs(keys[idx[a]], keys[idx[b]]) < 0
	})
	// Emit first of each key group
	out := buf[:0]
	out = append(out, '[')
	first := true
	for i, si := range idx {
		if i == 0 || !keySeqsEqual(keys[idx[i-1]], keys[si]) {
			if !first {
				out = append(out, ',')
			}
			first = false
			out = append(out, elems[si]...)
		}
	}
	out = append(out, ']')
	return fn(out)
}

// execGroupBy groups array elements by a key function.
// Returns an array of arrays: each sub-array contains elements with the same key.
// Elements within each group preserve their original order; groups are in key order.
func execGroupBy(state execState, node *op, input []byte, buf []byte, fn func([]byte) error) error {
	elems, keys, err := collectElemKeys(state, node.child, input)
	if err != nil {
		return err
	}
	// Sort by key, stable to preserve original order within groups
	idx := make([]int, len(elems))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool {
		return compareKeySeqs(keys[idx[a]], keys[idx[b]]) < 0
	})
	// Build output: [[group1_elems], [group2_elems], ...]
	buf = append(buf[:0], '[')
	first := true
	groupStart := 0
	for i := 0; i <= len(idx); i++ {
		if i == len(idx) || (i > 0 && !keySeqsEqual(keys[idx[i-1]], keys[idx[i]])) {
			// Emit current group
			if !first {
				buf = append(buf, ',')
			}
			first = false
			buf = append(buf, '[')
			for k, si := range idx[groupStart:i] {
				if k > 0 {
					buf = append(buf, ',')
				}
				buf = append(buf, elems[si]...)
			}
			buf = append(buf, ']')
			groupStart = i
		}
	}
	buf = append(buf, ']')
	return fn(buf)
}

// execTranspose transposes a matrix (array of arrays).
// Short rows are padded with null to match the longest row.
// transpose [] → [] ; transpose [[1],[2,3]] → [[1,2],[null,3]]
func execTranspose(input []byte, buf []byte, fn func([]byte) error) error {
	// Collect rows
	var rows [][]byte
	s := scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '[' {
		return fmt.Errorf("transpose input must be an array")
	}
	s.arrayIter(func(_ int, start, end int) bool {
		row := make([]byte, end-start)
		copy(row, input[start:end])
		rows = append(rows, row)
		return true
	})
	if len(rows) == 0 {
		return fn(append(buf[:0], "[]"...))
	}
	// Find max row length
	maxLen := 0
	for _, row := range rows {
		n := 0
		rs := scanner{data: row}
		rs.skipWhitespace()
		if rs.pos < len(rs.data) && rs.data[rs.pos] == '[' {
			rs.arrayIter(func(_ int, _, _ int) bool { n++; return true })
		}
		if n > maxLen {
			maxLen = n
		}
	}
	// Pre-collect all row elements for random access
	rowElems := make([][][]byte, len(rows))
	for i, row := range rows {
		rs := scanner{data: row}
		rs.skipWhitespace()
		if rs.pos >= len(rs.data) || rs.data[rs.pos] != '[' {
			continue
		}
		rs.arrayIter(func(_ int, start, end int) bool {
			e := make([]byte, end-start)
			copy(e, row[start:end])
			rowElems[i] = append(rowElems[i], e)
			return true
		})
	}
	// Build transposed matrix
	buf = append(buf[:0], '[')
	for col := 0; col < maxLen; col++ {
		if col > 0 {
			buf = append(buf, ',')
		}
		buf = append(buf, '[')
		for ri, re := range rowElems {
			if ri > 0 {
				buf = append(buf, ',')
			}
			if col < len(re) {
				buf = append(buf, re[col]...)
			} else {
				buf = append(buf, "null"...)
			}
		}
		buf = append(buf, ']')
	}
	buf = append(buf, ']')
	return fn(buf)
}

// compareJSONOrder returns -1, 0, or +1 for ordering two raw JSON values.
// Numbers: float comparison. Strings: lexicographic byte order.
// Cross-type ordering follows jq: null < false < true < numbers < strings < arrays < objects.
func compareJSONOrder(a, b []byte) int {
	as := scanner{data: a}
	bs := scanner{data: b}
	as.skipWhitespace()
	bs.skipWhitespace()

	aFirst := byte('n')
	if as.pos < len(as.data) {
		aFirst = as.data[as.pos]
	}
	bFirst := byte('n')
	if bs.pos < len(bs.data) {
		bFirst = bs.data[bs.pos]
	}

	aOrd := jsonTypeOrderVal(aFirst)
	bOrd := jsonTypeOrderVal(bFirst)
	if aOrd != bOrd {
		if aOrd < bOrd {
			return -1
		}
		return 1
	}

	// Same type — compare values
	switch aFirst {
	case 'n': // null == null
		return 0
	case 'f': // false == false
		return 0
	case 't': // true == true
		return 0
	case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9': // number
		if cmp, ok := compareJSONNumberLiterals(a, b); ok {
			return cmp
		}
		af, aOk := parseJSONFloat(a)
		bf, bOk := parseJSONFloat(b)
		if aOk && bOk {
			if af < bf {
				return -1
			}
			if af > bf {
				return 1
			}
			return 0
		}
		return bytesCompare(trimWhitespace(a), trimWhitespace(b))
	case '"': // string: lexicographic byte comparison of content
		ac := as.readString()
		bc := bs.readString()
		for i := 0; i < len(ac) && i < len(bc); i++ {
			if ac[i] < bc[i] {
				return -1
			}
			if ac[i] > bc[i] {
				return 1
			}
		}
		if len(ac) < len(bc) {
			return -1
		}
		if len(ac) > len(bc) {
			return 1
		}
		return 0
	case '[': // array: element-by-element comparison (zero-alloc parallel scan)
		as.pos++ // skip '['
		bs.pos++ // skip '['
		for {
			as.skipWhitespace()
			bs.skipWhitespace()
			aEnd := as.pos >= len(as.data) || as.data[as.pos] == ']'
			bEnd := bs.pos >= len(bs.data) || bs.data[bs.pos] == ']'
			if aEnd && bEnd {
				return 0
			}
			if aEnd {
				return -1 // shorter array sorts first
			}
			if bEnd {
				return 1
			}
			aElemStart := as.pos
			as.skipValue()
			bElemStart := bs.pos
			bs.skipValue()
			// Malformed input (e.g. `[}`) can leave skipValue parked on a
			// delimiter with zero progress, which would otherwise loop forever.
			if as.pos == aElemStart || bs.pos == bElemStart {
				return bytesCompare(a[aElemStart:], b[bElemStart:])
			}
			if c := compareJSONOrder(a[aElemStart:as.pos], b[bElemStart:bs.pos]); c != 0 {
				return c
			}
			as.skipWhitespace()
			if as.pos < len(as.data) && as.data[as.pos] == ',' {
				as.pos++
			}
			bs.skipWhitespace()
			if bs.pos < len(bs.data) && bs.data[bs.pos] == ',' {
				bs.pos++
			}
		}
	case '{': // object: compare as sorted (key, value) pair sequences
		// jq orders objects by their entries sorted by key; compare pair-by-pair.
		// Allocates to collect and sort entries (acceptable: sort is already Tier 2).
		aPairs := collectSortedObjectPairs(a)
		bPairs := collectSortedObjectPairs(b)
		for i := 0; i < len(aPairs) && i < len(bPairs); i++ {
			// Compare keys first
			if c := compareJSONOrder(aPairs[i][0], bPairs[i][0]); c != 0 {
				return c
			}
			// Same key: compare values
			if c := compareJSONOrder(aPairs[i][1], bPairs[i][1]); c != 0 {
				return c
			}
		}
		if len(aPairs) < len(bPairs) {
			return -1
		}
		if len(aPairs) > len(bPairs) {
			return 1
		}
		return 0
	}
	return 0
}

// collectSortedObjectPairs collects (key, value) sub-slices from a JSON object,
// sorted by key. Allocates proportionally to the number of keys.
// Used by compareJSONOrder for object comparison (Tier 2 path).
func collectSortedObjectPairs(obj []byte) [][2][]byte {
	s := scanner{data: obj}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '{' {
		return nil
	}
	s.pos++ // skip '{'
	var pairs [][2][]byte
	for s.pos < len(s.data) && s.data[s.pos] != '}' {
		s.skipWhitespace()
		if s.pos >= len(s.data) || s.data[s.pos] != '"' {
			break
		}
		keyStart := s.pos
		s.pos++ // skip opening '"'
		for s.pos < len(s.data) {
			if s.data[s.pos] == '\\' {
				s.pos += 2
			} else if s.data[s.pos] == '"' {
				s.pos++
				break
			} else {
				s.pos++
			}
		}
		s.clampPos() // a truncated trailing escape overshot the end
		keyBytes := obj[keyStart:s.pos]
		s.skipWhitespace()
		if s.pos < len(s.data) && s.data[s.pos] == ':' {
			s.pos++
		}
		s.skipWhitespace()
		valStart := s.pos
		s.skipValue()
		valBytes := obj[valStart:s.pos]
		pairs = append(pairs, [2][]byte{keyBytes, valBytes})
		s.skipWhitespace()
		if s.pos < len(s.data) && s.data[s.pos] == ',' {
			s.pos++
		}
	}
	// Sort by key (string comparison of the raw key bytes including quotes)
	sort.Slice(pairs, func(i, j int) bool {
		return compareJSONOrder(pairs[i][0], pairs[j][0]) < 0
	})
	return pairs
}

// jsonTypeOrderVal maps the first byte of a JSON value to a sort order integer.
// Matches jq's canonical type ordering for sort: null < false < true < numbers < strings < arrays < objects.
func jsonTypeOrderVal(b byte) int {
	switch b {
	case 'n': // null
		return 0
	case 'f': // false
		return 1
	case 't': // true
		return 2
	case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9',
		'N', // NaN  (internal sentinel, first byte 'N')
		'i': // infinite (internal sentinel, first byte 'i')
		return 3 // number
	case '"':
		return 4 // string
	case '[':
		return 5 // array
	case '{':
		return 6 // object
	default:
		return 7 // unknown (treat as null)
	}
}

// objectContainsKey reports whether JSON object obj has the given key.
// Uses a manual scan loop (no closure/callback) to avoid heap allocation.
func objectContainsKey(obj, key []byte) bool {
	s := scanner{data: obj}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '{' {
		return false
	}
	s.pos++ // skip '{'
	for s.pos < len(s.data) && s.data[s.pos] != '}' {
		s.skipWhitespace()
		if s.pos >= len(s.data) || s.data[s.pos] != '"' {
			break
		}
		k := s.readString()
		s.skipWhitespace()
		if s.pos < len(s.data) && s.data[s.pos] == ':' {
			s.pos++
		}
		s.skipWhitespace()
		s.skipValue()
		if bytesEqual(k, key) {
			return true
		}
		s.skipWhitespace()
		if s.pos < len(s.data) && s.data[s.pos] == ',' {
			s.pos++
		} else {
			break
		}
	}
	return false
}

// execToJSON wraps the input value as a JSON string, escaping " and \.
// nan and inf values are converted to null (not valid JSON per JSON spec).
func execToJSON(input []byte, buf []byte) []byte {
	s := scanner{data: input}
	s.skipWhitespace()
	start := s.pos
	s.skipValue()
	value := input[start:s.pos]
	buf = append(buf, '"')
	depth := 0
	truncated := false
	for i := 0; i < len(value) && !truncated; {
		b := value[i]
		switch b {
		case '{', '[':
			depth++
			if depth > jsonStringifySkipDepthLimit {
				buf = append(buf, "<skipped: too deep>"...)
				truncated = true
				continue
			}
		case '}', ']':
			if depth > 0 {
				depth--
			}
		}
		// Replace nan token with null
		if b == 'n' && i+3 <= len(value) && value[i+1] == 'a' && value[i+2] == 'n' {
			buf = append(buf, "null"...)
			i += 3
			continue
		}
		// Replace inf token with null
		if b == 'i' && i+3 <= len(value) && value[i+1] == 'n' && value[i+2] == 'f' {
			buf = append(buf, "null"...)
			i += 3
			continue
		}
		// Replace -inf token with null
		if b == '-' && i+4 <= len(value) && value[i+1] == 'i' && value[i+2] == 'n' && value[i+3] == 'f' {
			buf = append(buf, "null"...)
			i += 4
			continue
		}
		if b == '"' {
			buf = append(buf, '\\', '"')
		} else if b == '\\' {
			buf = append(buf, '\\', '\\')
		} else {
			buf = append(buf, b)
		}
		i++
	}
	return append(buf, '"')
}

// execFromJSON parses a JSON string and returns its content as raw JSON bytes.
// Returns an error if the resulting value is not valid JSON.
func execFromJSON(input []byte, buf []byte) ([]byte, error) {
	s := scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '"' {
		return nil, fmt.Errorf("fromjson input must be a string")
	}
	s.pos++ // skip opening '"'
	startLen := len(buf)
	for s.pos < len(s.data) && s.data[s.pos] != '"' {
		if s.data[s.pos] == '\\' && s.pos+1 < len(s.data) {
			next := s.data[s.pos+1]
			if next == '"' || next == '\\' {
				buf = append(buf, next)
				s.pos += 2
				continue
			}
		}
		buf = append(buf, s.data[s.pos])
		s.pos++
	}
	result := buf[startLen:]
	// Recognise jq special numeric strings that aren't valid JSON.
	// "nan" / "NaN" / "-NaN" / "-nan" → our internal NaN sentinel "NaN"
	// "infinite" / "Inf" / "+Inf" → "infinite"; "-infinite" / "-Inf" → "-infinite"
	if len(result) == 3 && (result[0] == 'n' || result[0] == 'N') &&
		(result[1] == 'a' || result[1] == 'A') && (result[2] == 'n' || result[2] == 'N') {
		buf = buf[:startLen]
		return append(buf, "NaN"...), nil
	}
	if len(result) == 4 && result[0] == '-' && (result[1] == 'N' || result[1] == 'n') &&
		(result[2] == 'a' || result[2] == 'A') && (result[3] == 'N' || result[3] == 'n') {
		buf = buf[:startLen]
		return append(buf, "NaN"...), nil
	}
	if len(result) == 8 && result[0] == 'i' && result[1] == 'n' && result[2] == 'f' {
		buf = buf[:startLen]
		return append(buf, "infinite"...), nil
	}
	if len(result) == 9 && result[0] == '-' && result[1] == 'i' && result[2] == 'n' {
		buf = buf[:startLen]
		return append(buf, "-infinite"...), nil
	}
	if exceedsJSONDepthLimit(result, jsonParseDepthLimit) {
		return nil, fmt.Errorf("Exceeds depth limit for parsing")
	}
	if !json.Valid(result) {
		return nil, fromJSONError(result)
	}
	return buf, nil
}

func exceedsJSONDepthLimit(data []byte, limit int) bool {
	depth := 0
	inString := false
	for i := 0; i < len(data); i++ {
		ch := data[i]
		if inString {
			if ch == '\\' {
				i++
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{', '[':
			depth++
			if depth > limit {
				return true
			}
		case '}', ']':
			if depth > 0 {
				depth--
			}
		}
	}
	return false
}

// fromJSONError generates a jq-compatible error message for invalid JSON.
func fromJSONError(data []byte) error {
	content := string(data)
	// Scan for the first problematic character to produce a jq-style message.
	// jq detects single-quoted strings (') as "Invalid string literal".
	// For other cases it reports "Invalid numeric literal at EOF".
	col := 1
	inString := false
	for i := 0; i < len(data); i++ {
		ch := data[i]
		if inString {
			if ch == '"' {
				inString = false
			} else if ch == '\\' {
				i++ // skip escaped char
				col++
			}
			col++
			continue
		}
		// Not in string
		if ch == '"' {
			inString = true
			col++
			continue
		}
		if ch == '\'' {
			// jq tries to lex 'token' (single-quoted), scanning until the closing '.
			// It reports the error at the column AFTER the closing ', where the next
			// char is unexpected. Find the matching closing '.
			i++ // skip opening '
			col++
			for i < len(data) && data[i] != '\'' {
				i++
				col++
			}
			// Now at closing ' (or end of data)
			if i < len(data) {
				col++ // skip closing '
			}
			// col is now at the char after the closing ' — this is the error column
			return fmt.Errorf("Invalid string literal; expected \", but got ' at line 1, column %d (while parsing '%s')", col, content)
		}
		col++
	}
	// Generic error — col is 1-past-end; jq reports the last valid column (col-1).
	return fmt.Errorf("Invalid numeric literal at EOF at line 1, column %d (while parsing '%s')", col-1, content)
}

// execToString returns the input unchanged if it is already a JSON string,
// otherwise calls execToJSON to wrap it.
func execToString(input []byte, buf []byte) []byte {
	s := scanner{data: input}
	s.skipWhitespace()
	if s.pos < len(s.data) && s.data[s.pos] == '"' {
		// Already a string — pass through (including quotes)
		end := s.pos
		s.skipValue()
		if buf == nil {
			return input[end:s.pos:s.pos]
		}
		return append(buf, input[end:s.pos]...)
	}
	return execToJSON(input[s.pos:], buf)
}

// execToNumber converts a JSON string or number to a JSON number.
// Strings are parsed as floats and re-emitted. Numbers pass through unchanged.
func execToNumber(input []byte, buf []byte) ([]byte, error) {
	s := scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) {
		return nil, fmt.Errorf("tonumber: expected number or string")
	}
	switch s.data[s.pos] {
	case '"': // string → parse as number
		content := s.readString()
		f, ok := parseJSONFloat(content)
		if !ok {
			return nil, fmt.Errorf("tonumber: cannot parse %q as number", content)
		}
		return appendNumber(buf, f), nil
	default:
		if isNumberByte(s.data[s.pos]) {
			// Already a number — pass through
			start := s.pos
			s.skipValue()
			if buf == nil {
				return input[start:s.pos:s.pos], nil
			}
			return append(buf, input[start:s.pos]...), nil
		}
		return nil, fmt.Errorf("tonumber: expected number or string, got %q", string(s.data[s.pos:s.pos+1]))
	}
}

func execToBoolean(input []byte, buf []byte) ([]byte, error) {
	s := scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) {
		return nil, fmt.Errorf("null () cannot be parsed as a boolean")
	}
	switch s.data[s.pos] {
	case 't':
		return append(buf, "true"...), nil
	case 'f':
		return append(buf, "false"...), nil
	case '"':
		raw := trimWhitespace(input)
		content := s.readString()
		if bytesEqualStr(content, "true") {
			return append(buf, "true"...), nil
		}
		if bytesEqualStr(content, "false") {
			return append(buf, "false"...), nil
		}
		return nil, fmt.Errorf("string (%s) cannot be parsed as a boolean", raw)
	default:
		raw := trimWhitespace(input)
		typeName := jsonTypeName(raw)
		if isNumberByte(s.data[s.pos]) {
			typeName = "number"
		}
		return nil, fmt.Errorf("%s (%s) cannot be parsed as a boolean", typeName, raw)
	}
}

func execAbs(input []byte, buf []byte) ([]byte, error) {
	s := scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) {
		return nil, fmt.Errorf("abs input must be a number or string")
	}
	start := s.pos
	switch s.data[s.pos] {
	case '"':
		s.skipValue()
		if buf == nil {
			return input[start:s.pos:s.pos], nil
		}
		return append(buf, input[start:s.pos]...), nil
	case '-':
		s.skipValue()
		if buf == nil {
			return input[start+1 : s.pos : s.pos], nil
		}
		return append(buf, input[start+1:s.pos]...), nil
	default:
		if isNumberByte(s.data[s.pos]) {
			s.skipValue()
			if buf == nil {
				return input[start:s.pos:s.pos], nil
			}
			return append(buf, input[start:s.pos]...), nil
		}
		return nil, fmt.Errorf("abs input must be a number or string")
	}
}

// execURIEncode percent-encodes a JSON string per RFC 3986 unreserved characters.
// The JSON string is first decoded (escape sequences resolved) so that e.g.
// "\u03bc" encodes as %CE%BC (the UTF-8 bytes for μ) rather than %5Cu03bc.
func execURIEncode(input []byte, buf []byte) ([]byte, error) {
	s := scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '"' {
		return nil, fmt.Errorf("@uri input must be a string")
	}
	raw := s.readString()                        // raw bytes between JSON quotes
	content := decodeJSONStringContent(nil, raw) // resolve escape sequences

	buf = append(buf, '"')
	for _, b := range content {
		if (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') ||
			b == '-' || b == '_' || b == '.' || b == '~' {
			buf = append(buf, b)
		} else {
			hi := (b >> 4) & 0x0F
			lo := b & 0x0F
			buf = append(buf, '%')
			if hi < 10 {
				buf = append(buf, '0'+hi)
			} else {
				buf = append(buf, 'A'+hi-10)
			}
			if lo < 10 {
				buf = append(buf, '0'+lo)
			} else {
				buf = append(buf, 'A'+lo-10)
			}
		}
	}
	return append(buf, '"'), nil
}

// hexNibble converts an ASCII hex digit to its numeric value (0–15).
func hexNibble(b byte) rune {
	switch {
	case b >= '0' && b <= '9':
		return rune(b - '0')
	case b >= 'a' && b <= 'f':
		return rune(b-'a') + 10
	case b >= 'A' && b <= 'F':
		return rune(b-'A') + 10
	}
	return 0
}

func parseHexNibbleByte(b byte) (byte, bool) {
	switch {
	case b >= '0' && b <= '9':
		return b - '0', true
	case b >= 'a' && b <= 'f':
		return b - 'a' + 10, true
	case b >= 'A' && b <= 'F':
		return b - 'A' + 10, true
	default:
		return 0, false
	}
}

// appendRuneUTF8 appends the UTF-8 encoding of r to dst.
func appendRuneUTF8(dst []byte, r rune) []byte {
	switch {
	case r < 0x80:
		return append(dst, byte(r))
	case r < 0x800:
		return append(dst, byte(0xC0|(r>>6)), byte(0x80|(r&0x3F)))
	case r < 0x10000:
		return append(dst, byte(0xE0|(r>>12)), byte(0x80|((r>>6)&0x3F)), byte(0x80|(r&0x3F)))
	default:
		return append(dst, byte(0xF0|(r>>18)), byte(0x80|((r>>12)&0x3F)), byte(0x80|((r>>6)&0x3F)), byte(0x80|(r&0x3F)))
	}
}

// decodeJSONStringContent decodes raw JSON string content (as returned by
// scanner.readString) into its actual byte values, handling all standard
// JSON escape sequences. Raw UTF-8 bytes pass through unchanged.
// \\ \" \/ \n \r \t \b \f → single byte. \uXXXX → UTF-8 encoded codepoint.
// Surrogate pairs (\uD800\uDC00) are decoded to the combined codepoint.
// The result is appended to dst and returned.
func decodeJSONStringContent(dst, content []byte) []byte {
	for i := 0; i < len(content); {
		if content[i] != '\\' {
			dst = append(dst, content[i])
			i++
			continue
		}
		i++ // skip '\'
		if i >= len(content) {
			break
		}
		switch content[i] {
		case '"':
			dst = append(dst, '"')
		case '\\':
			dst = append(dst, '\\')
		case '/':
			dst = append(dst, '/')
		case 'n':
			dst = append(dst, '\n')
		case 'r':
			dst = append(dst, '\r')
		case 't':
			dst = append(dst, '\t')
		case 'b':
			dst = append(dst, '\b')
		case 'f':
			dst = append(dst, '\f')
		case 'u':
			if i+4 < len(content) {
				r := hexNibble(content[i+1])<<12 |
					hexNibble(content[i+2])<<8 |
					hexNibble(content[i+3])<<4 |
					hexNibble(content[i+4])
				// Handle UTF-16 surrogate pairs
				if r >= 0xD800 && r <= 0xDBFF && i+10 < len(content) &&
					content[i+5] == '\\' && content[i+6] == 'u' {
					r2 := hexNibble(content[i+7])<<12 |
						hexNibble(content[i+8])<<8 |
						hexNibble(content[i+9])<<4 |
						hexNibble(content[i+10])
					if r2 >= 0xDC00 && r2 <= 0xDFFF {
						r = 0x10000 + (r-0xD800)<<10 + (r2 - 0xDC00)
						dst = appendRuneUTF8(dst, r)
						i += 10 // skip XXXX\uYYYY; outer i++ skips past last Y
						break
					}
				}
				dst = appendRuneUTF8(dst, r)
				i += 4 // skip XXXX; outer i++ moves past last X
			}
		default:
			dst = append(dst, content[i])
		}
		i++
	}
	return dst
}

// jsonHexChars is the lowercase hex digit table for JSON \uXXXX encoding.
const jsonHexChars = "0123456789abcdef"

// appendCanonicalRawJSONStringContent normalizes raw JSON string content into jq's
// preferred output form without rewriting already-raw UTF-8 bytes. Control escapes
// become \u00xx, printable ASCII escapes become literal bytes, and \uXXXX escapes
// are normalized to lowercase hex (or surrogate pairs above U+FFFF).
func appendCanonicalRawJSONStringContent(dst, raw []byte) []byte {
	for i := 0; i < len(raw); i++ {
		if raw[i] != '\\' {
			if raw[i] < 0x20 {
				dst = append(dst, '\\', 'u', '0', '0',
					jsonHexChars[raw[i]>>4], jsonHexChars[raw[i]&0xF])
			} else {
				dst = append(dst, raw[i])
			}
			continue
		}
		i++
		if i >= len(raw) {
			break
		}
		switch raw[i] {
		case '"':
			dst = append(dst, '\\', '"')
		case '\\':
			dst = append(dst, '\\', '\\')
		case '/':
			dst = append(dst, '/')
		case 'n':
			dst = append(dst, '\\', 'u', '0', '0', '0', 'a')
		case 'r':
			dst = append(dst, '\\', 'u', '0', '0', '0', 'd')
		case 't':
			dst = append(dst, '\\', 'u', '0', '0', '0', '9')
		case 'b':
			dst = append(dst, '\\', 'u', '0', '0', '0', '8')
		case 'f':
			dst = append(dst, '\\', 'u', '0', '0', '0', 'c')
		case 'u':
			if i+4 >= len(raw) {
				continue
			}
			r := hexNibble(raw[i+1])<<12 |
				hexNibble(raw[i+2])<<8 |
				hexNibble(raw[i+3])<<4 |
				hexNibble(raw[i+4])
			i += 4
			if r >= 0xD800 && r <= 0xDBFF && i+6 < len(raw) &&
				raw[i+1] == '\\' && raw[i+2] == 'u' {
				r2 := hexNibble(raw[i+3])<<12 |
					hexNibble(raw[i+4])<<8 |
					hexNibble(raw[i+5])<<4 |
					hexNibble(raw[i+6])
				if r2 >= 0xDC00 && r2 <= 0xDFFF {
					dst = appendHex4Escape(dst, r)
					dst = appendHex4Escape(dst, r2)
					i += 6
					continue
				}
			}
			switch {
			case r < 0x20:
				dst = appendHex4Escape(dst, r)
			case r == '"':
				dst = append(dst, '\\', '"')
			case r == '\\':
				dst = append(dst, '\\', '\\')
			case r < 0x80:
				dst = append(dst, byte(r))
			default:
				dst = appendHex4Escape(dst, r)
			}
		default:
			dst = append(dst, raw[i])
		}
	}
	return dst
}

func normalizeOutputValue(value, buf []byte) []byte {
	s := scanner{data: value}
	s.skipWhitespace()
	start := s.pos
	s.skipValue()
	value = value[start:s.pos]
	if len(value) == 0 {
		if buf == nil {
			return value
		}
		return append(buf, value...)
	}
	if value[0] != '"' {
		if buf == nil {
			return value[:len(value):len(value)]
		}
		return append(buf, value...)
	}
	out := buf
	if out == nil {
		out = make([]byte, 0, len(value))
	}
	ss := scanner{data: value}
	raw := ss.readString()
	out = append(out, '"')
	out = appendCanonicalRawJSONStringContent(out, raw)
	return append(out, '"')
}

func execStringTemplate(state execState, node *op, input, buf []byte) ([]byte, error) {
	return execInterpolatedTemplate(state, node, input, buf, false)
}

func execFormatTemplate(state execState, node *op, input, buf []byte) ([]byte, error) {
	return execInterpolatedTemplate(state, node, input, buf, true)
}

func execInterpolatedTemplate(state execState, node *op, input, buf []byte, applyFormat bool) ([]byte, error) {
	buf = append(buf, '"')
	for i, expr := range node.elems {
		buf = append(buf, node.segs[i]...)
		result, err := execSingle(state, expr, input, nil)
		if err != nil {
			return nil, err
		}
		if applyFormat {
			result, err = execTemplateFormatter(node.format, result, nil)
			if err != nil {
				return nil, err
			}
		}
		buf = appendEmbeddedJSONValue(buf, trimWhitespace(result))
	}
	buf = append(buf, node.segs[len(node.elems)]...)
	return append(buf, '"'), nil
}

func appendEmbeddedJSONValue(dst, result []byte) []byte {
	if len(result) > 0 && result[0] == '"' {
		sc := scanner{data: result}
		return append(dst, sc.readString()...)
	}
	for _, b := range result {
		if b == '"' {
			dst = append(dst, '\\', '"')
		} else if b == '\\' {
			dst = append(dst, '\\', '\\')
		} else {
			dst = append(dst, b)
		}
	}
	return dst
}

func execTemplateFormatter(format opType, input, buf []byte) ([]byte, error) {
	switch format {
	case opBase64:
		return execBase64Encode(input, buf)
	case opBase64D:
		return execBase64Decode(input, buf)
	case opURIEncode:
		return execURIEncode(input, buf)
	case opToJSON:
		return execToJSON(input, buf), nil
	case opToString:
		return execToString(input, buf), nil
	case opHTMLEncode:
		return execHTMLEncode(input, buf)
	case opCSVEncode:
		return execCSVEncode(input, buf)
	case opTSVEncode:
		return execTSVEncode(input, buf)
	case opShEncode:
		return execShEncode(input, buf)
	case opURIDecode:
		return execURIDecode(input, buf)
	default:
		return nil, fmt.Errorf("unsupported template formatter: %v", format)
	}
}

func jqTypeValueForError(value []byte) string {
	s := scanner{data: value}
	s.skipWhitespace()
	typeName := "number"
	if s.pos < len(s.data) {
		switch s.data[s.pos] {
		case '"':
			typeName = "string"
		case 'n':
			typeName = "null"
		case 't', 'f':
			typeName = "boolean"
		case '[':
			typeName = "array"
		case '{':
			typeName = "object"
		}
	}
	raw := string(trimWhitespace(value))
	if len(raw) > 14 {
		raw = raw[:11] + "..."
	}
	return fmt.Sprintf("%s (%s)", typeName, raw)
}

func catchInputFromError(err error) []byte {
	if je, ok := err.(*jsonError); ok {
		return je.payload
	}
	msg := make([]byte, 0, 64)
	msg = append(msg, '"')
	for _, b := range []byte(err.Error()) {
		if b == '"' {
			msg = append(msg, '\\', '"')
		} else if b == '\\' {
			msg = append(msg, '\\', '\\')
		} else {
			msg = append(msg, b)
		}
	}
	return append(msg, '"')
}

func jsonTypeName(input []byte) string {
	s := scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) {
		return "null"
	}
	switch s.data[s.pos] {
	case '{':
		return "object"
	case '[':
		return "array"
	case '"':
		return "string"
	case 't', 'f':
		return "boolean"
	case 'n':
		return "null"
	default:
		if isNumberByte(s.data[s.pos]) {
			return "number"
		}
		return "invalid"
	}
}

func fieldAccessError(input []byte, field string) error {
	return fmt.Errorf("Cannot index %s with string %q", jsonTypeName(input), field)
}

func indexAccessError(input []byte, index int) error {
	return fmt.Errorf("Cannot index %s with number %d", jsonTypeName(input), index)
}

func dynamicIndexNumberAccessError(input []byte) error {
	return fmt.Errorf("Cannot index %s with number", jsonTypeName(input))
}

// appendJSONStringContent appends decoded bytes re-encoded as JSON string content
// (without outer quotes). Handles standard escaping for control chars, " and \.
func appendJSONStringContent(dst, decoded []byte) []byte {
	for i := 0; i < len(decoded); {
		b := decoded[i]
		switch {
		case b == '"':
			dst = append(dst, '\\', '"')
			i++
		case b == '\\':
			dst = append(dst, '\\', '\\')
			i++
		case b == '\n':
			dst = append(dst, '\\', 'n')
			i++
		case b == '\r':
			dst = append(dst, '\\', 'r')
			i++
		case b == '\t':
			dst = append(dst, '\\', 't')
			i++
		case b == '\b':
			dst = append(dst, '\\', 'b')
			i++
		case b == '\f':
			dst = append(dst, '\\', 'f')
			i++
		case b < 0x20:
			dst = append(dst, '\\', 'u', '0', '0',
				jsonHexChars[b>>4], jsonHexChars[b&0xF])
			i++
		case b >= 0x80:
			// Non-ASCII UTF-8: pass raw bytes through (valid JSON)
			dst = append(dst, b)
			i++
		default:
			dst = append(dst, b)
			i++
		}
	}
	return dst
}

// appendJSONStringContentUnicodeEscaped is like appendJSONStringContent but
// encodes non-ASCII codepoints as \uXXXX (needed for @urid output).
func appendJSONStringContentUnicodeEscaped(dst, decoded []byte) []byte {
	for i := 0; i < len(decoded); {
		b := decoded[i]
		if b < 0x80 {
			// ASCII — use standard JSON escaping
			switch b {
			case '"':
				dst = append(dst, '\\', '"')
			case '\\':
				dst = append(dst, '\\', '\\')
			case '\n':
				dst = append(dst, '\\', 'n')
			case '\r':
				dst = append(dst, '\\', 'r')
			case '\t':
				dst = append(dst, '\\', 't')
			case '\b':
				dst = append(dst, '\\', 'b')
			case '\f':
				dst = append(dst, '\\', 'f')
			default:
				if b < 0x20 {
					dst = append(dst, '\\', 'u', '0', '0',
						jsonHexChars[b>>4], jsonHexChars[b&0xF])
				} else {
					dst = append(dst, b)
				}
			}
			i++
		} else {
			// Decode UTF-8 sequence to a codepoint, then output as \uXXXX
			var r rune
			var size int
			if b < 0xE0 && i+1 < len(decoded) {
				r = rune(b&0x1F)<<6 | rune(decoded[i+1]&0x3F)
				size = 2
			} else if b < 0xF0 && i+2 < len(decoded) {
				r = rune(b&0x0F)<<12 | rune(decoded[i+1]&0x3F)<<6 | rune(decoded[i+2]&0x3F)
				size = 3
			} else if i+3 < len(decoded) {
				r = rune(b&0x07)<<18 | rune(decoded[i+1]&0x3F)<<12 |
					rune(decoded[i+2]&0x3F)<<6 | rune(decoded[i+3]&0x3F)
				size = 4
			} else {
				dst = append(dst, '?')
				i++
				continue
			}
			if r > 0xFFFF {
				// Emit as surrogate pair
				r -= 0x10000
				hi := rune(0xD800) + (r >> 10)
				lo := rune(0xDC00) + (r & 0x3FF)
				dst = appendHex4Escape(dst, hi)
				dst = appendHex4Escape(dst, lo)
			} else {
				dst = appendHex4Escape(dst, r)
			}
			i += size
		}
	}
	return dst
}

// appendHex4Escape appends \uXXXX for the given codepoint.
func appendHex4Escape(dst []byte, r rune) []byte {
	return append(dst, '\\', 'u',
		jsonHexChars[(r>>12)&0xF],
		jsonHexChars[(r>>8)&0xF],
		jsonHexChars[(r>>4)&0xF],
		jsonHexChars[r&0xF])
}

// --- floor / ceil / round ---

type roundMode int

const (
	roundFloor roundMode = iota
	roundCeil
	roundNearest
)

// execRoundMode applies floor/ceil/round to a JSON number.
func execRoundMode(input, buf []byte, mode roundMode) []byte {
	s := scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) {
		return append(buf, "null"...)
	}
	if !isNumberByte(s.data[s.pos]) {
		return append(buf, input...)
	}
	f, ok := parseJSONFloat(input)
	if !ok {
		return append(buf, input...)
	}
	var result float64
	switch mode {
	case roundFloor:
		result = math.Floor(f)
	case roundCeil:
		result = math.Ceil(f)
	case roundNearest:
		result = math.Round(f)
	}
	// Output as integer if the result is a whole number.
	if result == math.Trunc(result) && !math.IsInf(result, 0) && !math.IsNaN(result) {
		return strconv.AppendInt(buf, int64(result), 10)
	}
	return strconv.AppendFloat(buf, result, 'g', -1, 64)
}

// --- 1-arg floating-point math builtins ---

type mathFuncType int

const (
	mathSqrt mathFuncType = iota
	mathFabs
	mathAtan
	mathLog
	mathLog2
	mathLog10
	mathExp
	mathExp2
	mathExp10
	mathCbrt
	mathLogb
	mathNearbyint
	mathJ0
	mathJ1
	mathSin
	mathCos
	mathTan
	mathAsin
	mathAcos
	mathTgamma
	mathLgamma
)

// appendJSONFloat appends a float64 to buf as a JSON number.
// NaN and Inf are not valid JSON; both are serialised as null (matching tojson behaviour).
// Whole-number results are emitted as integers to match jq output.
func appendJSONFloat(buf []byte, f float64) []byte {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return append(buf, "null"...)
	}
	if f == math.Trunc(f) && f >= -1e15 && f <= 1e15 {
		return strconv.AppendInt(buf, int64(f), 10)
	}
	return strconv.AppendFloat(buf, f, 'g', -1, 64)
}

// execMathFunc applies a 1-arg floating-point function to a JSON number input.
// Non-number input produces null. Zero-alloc: uses appendJSONFloat into buf.
func execMathFunc(input, buf []byte, fn mathFuncType) []byte {
	s := scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) || !isNumberByte(s.data[s.pos]) {
		return append(buf, "null"...)
	}
	f, ok := parseJSONFloat(input)
	if !ok {
		return append(buf, "null"...)
	}
	var result float64
	switch fn {
	case mathSqrt:
		result = math.Sqrt(f)
	case mathFabs:
		result = math.Abs(f)
	case mathAtan:
		result = math.Atan(f)
	case mathLog:
		result = math.Log(f)
	case mathLog2:
		result = math.Log2(f)
	case mathLog10:
		result = math.Log10(f)
	case mathExp:
		result = math.Exp(f)
	case mathExp2:
		result = math.Exp2(f)
	case mathExp10:
		result = math.Pow(10, f) // Go has no math.Exp10
	case mathCbrt:
		result = math.Cbrt(f)
	case mathLogb:
		result = math.Logb(f)
	case mathNearbyint:
		// nearbyint uses round-to-nearest-even in IEEE 754; Go's math.Round uses
		// round-half-away-from-zero. They differ only for exactly .5 values.
		result = math.Round(f)
	case mathJ0:
		result = math.J0(f)
	case mathJ1:
		result = math.J1(f)
	case mathSin:
		result = math.Sin(f)
	case mathCos:
		result = math.Cos(f)
	case mathTan:
		result = math.Tan(f)
	case mathAsin:
		result = math.Asin(f)
	case mathAcos:
		result = math.Acos(f)
	case mathTgamma:
		result = math.Gamma(f)
	case mathLgamma:
		result, _ = math.Lgamma(f)
	}
	return appendJSONFloat(buf, result)
}

// --- @html ---

// execHTMLEncode HTML-escapes a JSON string.
func execHTMLEncode(input, buf []byte) ([]byte, error) {
	s := scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '"' {
		return nil, fmt.Errorf("@html input must be a string")
	}
	raw := s.readString()
	decoded := decodeJSONStringContent(nil, raw)

	buf = append(buf, '"')
	for _, b := range decoded {
		switch b {
		case '&':
			buf = append(buf, "&amp;"...)
		case '<':
			buf = append(buf, "&lt;"...)
		case '>':
			buf = append(buf, "&gt;"...)
		case '\'':
			buf = append(buf, "&apos;"...)
		case '"':
			buf = append(buf, "&quot;"...)
		default:
			// Re-encode for JSON output
			switch b {
			case '\\':
				buf = append(buf, '\\', '\\')
			case '\n':
				buf = append(buf, '\\', 'n')
			case '\r':
				buf = append(buf, '\\', 'r')
			case '\t':
				buf = append(buf, '\\', 't')
			case '\b':
				buf = append(buf, '\\', 'b')
			case '\f':
				buf = append(buf, '\\', 'f')
			default:
				if b < 0x20 {
					buf = append(buf, '\\', 'u', '0', '0',
						jsonHexChars[b>>4], jsonHexChars[b&0xF])
				} else {
					buf = append(buf, b)
				}
			}
		}
	}
	return append(buf, '"'), nil
}

// --- @csv ---

// execCSVEncode formats a JSON array as a CSV line.
// Strings are double-quoted with internal quotes doubled.
// Numbers are emitted as-is. Null produces an empty field.
func execCSVEncode(input, buf []byte) ([]byte, error) {
	s := scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '[' {
		return nil, fmt.Errorf("@csv input must be an array")
	}
	buf = append(buf, '"') // open outer JSON string
	first := true
	s.arrayIter(func(_ int, start, end int) bool {
		if !first {
			buf = append(buf, ',')
		}
		first = false
		elem := trimWhitespace(input[start:end])
		if len(elem) == 0 {
			return true
		}
		switch elem[0] {
		case '"':
			// String: decode, wrap in CSV " (encoded as \" in JSON), double internal "
			esc := scanner{data: elem}
			raw := esc.readString()
			decoded := decodeJSONStringContent(nil, raw)
			buf = append(buf, '\\', '"') // CSV open quote: \" in JSON
			for _, b := range decoded {
				if b == '"' {
					// Double the quote: "" in CSV → \"\" in JSON
					buf = append(buf, '\\', '"', '\\', '"')
				} else {
					switch b {
					case '\\':
						buf = append(buf, '\\', '\\')
					case '\n':
						buf = append(buf, '\\', 'n')
					case '\r':
						buf = append(buf, '\\', 'r')
					case '\t':
						buf = append(buf, '\\', 't')
					default:
						if b < 0x20 {
							buf = append(buf, '\\', 'u', '0', '0',
								jsonHexChars[b>>4], jsonHexChars[b&0xF])
						} else {
							buf = append(buf, b)
						}
					}
				}
			}
			buf = append(buf, '\\', '"') // CSV close quote: \" in JSON
		case 'n': // null → empty field
		case 't':
			buf = append(buf, "true"...)
		case 'f':
			buf = append(buf, "false"...)
		default:
			// Number: copy raw bytes
			buf = append(buf, elem...)
		}
		return true
	})
	return append(buf, '"'), nil // close outer JSON string
}

// --- @tsv ---

// execTSVEncode formats a JSON array as a TSV line.
// Strings have tabs, newlines, carriage returns, and backslashes escaped.
// Numbers are emitted as-is. Null produces an empty field.
func execTSVEncode(input, buf []byte) ([]byte, error) {
	s := scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '[' {
		return nil, fmt.Errorf("@tsv input must be an array")
	}
	buf = append(buf, '"') // open outer JSON string
	first := true
	var iterErr error
	s.arrayIter(func(_ int, start, end int) bool {
		if !first {
			buf = append(buf, '\\', 't') // TSV separator as JSON \t
		}
		first = false
		elem := trimWhitespace(input[start:end])
		if len(elem) == 0 {
			return true
		}
		switch elem[0] {
		case '"':
			esc := scanner{data: elem}
			raw := esc.readString()
			decoded := decodeJSONStringContent(nil, raw)
			for _, b := range decoded {
				switch b {
				case '\t':
					// TSV escape for tab: \t (backslash+t). In JSON: \\t
					buf = append(buf, '\\', '\\', 't')
				case '\n':
					// TSV escape for newline: \n. In JSON: \\n
					buf = append(buf, '\\', '\\', 'n')
				case '\r':
					// TSV escape for CR: \r. In JSON: \\r
					buf = append(buf, '\\', '\\', 'r')
				case '\\':
					// TSV escape for backslash: \\. In JSON: \\\\
					buf = append(buf, '\\', '\\', '\\', '\\')
				case '"':
					buf = append(buf, '\\', '"') // JSON escape for "
				default:
					if b < 0x20 {
						buf = append(buf, '\\', 'u', '0', '0',
							jsonHexChars[b>>4], jsonHexChars[b&0xF])
					} else {
						buf = append(buf, b)
					}
				}
			}
		case 'n': // null → empty
		case 't':
			buf = append(buf, "true"...)
		case 'f':
			buf = append(buf, "false"...)
		default:
			buf = append(buf, elem...)
		}
		return true
	})
	if iterErr != nil {
		return nil, iterErr
	}
	return append(buf, '"'), nil
}

// --- @sh ---

// execShEncode shell-quotes a JSON string as a single-quoted POSIX sh string.
// Internal single quotes are escaped as '\” (end-quote, backslash-quote, reopen-quote).
func execShEncode(input, buf []byte) ([]byte, error) {
	s := scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '"' {
		return nil, fmt.Errorf("@sh input must be a string")
	}
	raw := s.readString()
	decoded := decodeJSONStringContent(nil, raw)

	buf = append(buf, '"', '\'') // open JSON string, then sh single-quote
	for _, b := range decoded {
		if b == '\'' {
			// POSIX sh: end single-quote, literal backslash-escaped ', reopen quote: '\''
			// Backslash must be escaped in JSON: \\
			// Final JSON bytes: ' \\ ' '  → '\''
			buf = append(buf, '\'', '\\', '\\', '\'', '\'')
		} else {
			switch b {
			case '"':
				buf = append(buf, '\\', '"')
			case '\\':
				buf = append(buf, '\\', '\\')
			case '\n':
				buf = append(buf, '\\', 'n')
			case '\r':
				buf = append(buf, '\\', 'r')
			case '\t':
				buf = append(buf, '\\', 't')
			case '\b':
				buf = append(buf, '\\', 'b')
			case '\f':
				buf = append(buf, '\\', 'f')
			default:
				if b < 0x20 {
					buf = append(buf, '\\', 'u', '0', '0',
						jsonHexChars[b>>4], jsonHexChars[b&0xF])
				} else {
					buf = append(buf, b)
				}
			}
		}
	}
	buf = append(buf, '\'', '"') // sh close-quote, JSON close-quote
	return buf, nil
}

// --- @urid ---

// execURIDecode percent-decodes a URI-encoded JSON string.
// Non-ASCII codepoints in the decoded output are encoded as \uXXXX.
func execURIDecode(input, buf []byte) ([]byte, error) {
	s := scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '"' {
		return nil, fmt.Errorf("@urid input must be a string")
	}
	raw := decodeJSONStringContent(nil, s.readString())
	inputText := string(trimWhitespace(input))
	invalidURI := func() error {
		return fmt.Errorf("string (%s) is not a valid uri encoding", inputText)
	}

	// Percent-decode to raw bytes
	decoded := make([]byte, 0, len(raw))
	for i := 0; i < len(raw); i++ {
		if raw[i] != '%' {
			decoded = append(decoded, raw[i])
			continue
		}
		if i+2 >= len(raw) {
			return nil, invalidURI()
		}
		hi, okHi := parseHexNibbleByte(raw[i+1])
		lo, okLo := parseHexNibbleByte(raw[i+2])
		if !okHi || !okLo {
			return nil, invalidURI()
		}
		decoded = append(decoded, (hi<<4)|lo)
		i += 2
	}
	if !utf8.Valid(decoded) {
		return nil, invalidURI()
	}

	buf = append(buf, '"')
	buf = appendJSONStringContentUnicodeEscaped(buf, decoded)
	return append(buf, '"'), nil
}

// --- Regex operations ---
// All patterns are compiled once at Compile() time (stored in node.re).
// Go's RE2 engine guarantees linear-time matching — immune to ReDoS.

// execTest implements test(re) / test(re; flags).
// Zero-alloc: re.Match operates on the raw string content bytes without
// converting to string. Simple patterns use a one-pass NFA with no heap use;
// complex patterns use a pooled machine managed by the standard library.
// Non-string input returns false (not an error) so test() composes cleanly
// with select(): select(.msg | test("error")).
func execTest(node *op, input []byte) bool {
	s := scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '"' {
		return false
	}
	return node.re.Match(s.readString())
}

// execMatchRe implements match(re) / match(re; flags).
// Returns a JSON object: {"offset":N,"length":M,"string":"...","captures":[...]}.
// Non-string input → null. No match → null.
// Allocation: FindSubmatchIndex allocates one []int on a match; nil on miss (0 allocs).
func execMatchRe(node *op, input []byte, buf []byte) ([]byte, error) {
	s := scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '"' {
		return append(buf, "null"...), nil
	}
	content := s.readString()

	idx := node.re.FindSubmatchIndex(content)
	if idx == nil {
		return append(buf, "null"...), nil
	}

	offset, end := idx[0], idx[1]
	buf = append(buf, `{"offset":`...)
	buf = appendInt(buf, offset)
	buf = append(buf, `,"length":`...)
	buf = appendInt(buf, end-offset)
	buf = append(buf, `,"string":"`...)
	buf = append(buf, content[offset:end]...)
	buf = append(buf, `","captures":[`...)

	names := node.re.SubexpNames()
	first := true
	for i := 1; i < len(idx)/2; i++ {
		if !first {
			buf = append(buf, ',')
		}
		first = false
		capStart, capEnd := idx[2*i], idx[2*i+1]
		buf = append(buf, `{"offset":`...)
		if capStart == -1 {
			buf = append(buf, "-1"...)
			buf = append(buf, `,"length":0,"string":"","name":`...)
		} else {
			buf = appendInt(buf, capStart)
			buf = append(buf, `,"length":`...)
			buf = appendInt(buf, capEnd-capStart)
			buf = append(buf, `,"string":"`...)
			buf = append(buf, content[capStart:capEnd]...)
			buf = append(buf, `","name":`...)
		}
		if names[i] == "" {
			buf = append(buf, "null"...)
		} else {
			buf = append(buf, '"')
			buf = append(buf, names[i]...)
			buf = append(buf, '"')
		}
		buf = append(buf, '}')
	}
	return append(buf, ']', '}'), nil
}

// execCapture implements capture(re) / capture(re; flags).
// Returns only named captures as a flat JSON object.
// Non-string input → null. No match → null.
// Allocation: FindSubmatchIndex allocates one []int on a match; nil on miss.
func execCapture(node *op, input []byte, buf []byte) ([]byte, error) {
	s := scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '"' {
		return append(buf, "null"...), nil
	}
	content := s.readString()

	idx := node.re.FindSubmatchIndex(content)
	if idx == nil {
		return append(buf, "null"...), nil
	}

	names := node.re.SubexpNames()
	buf = append(buf, '{')
	first := true
	for i := 1; i < len(idx)/2; i++ {
		if names[i] == "" {
			continue
		}
		if !first {
			buf = append(buf, ',')
		}
		first = false
		capStart, capEnd := idx[2*i], idx[2*i+1]
		buf = append(buf, '"')
		buf = append(buf, names[i]...)
		buf = append(buf, `":"`...)
		if capStart >= 0 {
			buf = append(buf, content[capStart:capEnd]...)
		}
		buf = append(buf, '"')
	}
	return append(buf, '}'), nil
}

// execScan implements scan(re) / scan(re; flags).
// Emits all non-overlapping matches as a stream. No groups → JSON strings.
// With groups → JSON arrays of captured strings. Non-string input → no outputs.
// Allocation: FindAllSubmatch allocates per match — inherently multi-output, unavoidable.
func execScan(node *op, input []byte, buf []byte, fn func([]byte) error) error {
	s := scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '"' {
		return nil
	}
	content := s.readString()

	numGroups := node.re.NumSubexp()
	for _, match := range node.re.FindAllSubmatch(content, -1) {
		var out []byte
		if numGroups == 0 {
			out = append(out, '"')
			out = append(out, match[0]...)
			out = append(out, '"')
		} else {
			out = append(out, '[')
			for i, cap := range match[1:] {
				if i > 0 {
					out = append(out, ',')
				}
				if cap == nil {
					out = append(out, "null"...)
				} else {
					out = append(out, '"')
					out = append(out, cap...)
					out = append(out, '"')
				}
			}
			out = append(out, ']')
		}
		if err := fn(out); err != nil {
			return err
		}
	}
	return nil
}

// execSub implements sub(re; "literal").
// Replaces the first match with node.field (the literal replacement).
// Non-string input → pass through unchanged. No match → pass through unchanged.
// Allocation: FindIndex allocates one []int on a match; nil on miss (0 allocs).
func execSub(node *op, input []byte, buf []byte) ([]byte, error) {
	s := scanner{data: input}
	s.skipWhitespace()
	start := s.pos
	if s.pos >= len(s.data) || s.data[s.pos] != '"' {
		s.skipValue()
		return append(buf, input[start:s.pos]...), nil
	}
	content := s.readString()
	idx := node.re.FindIndex(content)
	if idx == nil {
		return append(buf, input[start:s.pos]...), nil
	}
	buf = append(buf, '"')
	buf = append(buf, content[:idx[0]]...)
	buf = append(buf, node.field...)
	buf = append(buf, content[idx[1]:]...)
	return append(buf, '"'), nil
}

// execGSub implements gsub(re; "literal").
// Replaces all matches with node.field (the literal replacement).
// Non-string input → pass through unchanged.
// Allocation: FindAllIndex allocates proportional to match count.
func execGSub(node *op, input []byte, buf []byte) ([]byte, error) {
	s := scanner{data: input}
	s.skipWhitespace()
	start := s.pos
	if s.pos >= len(s.data) || s.data[s.pos] != '"' {
		s.skipValue()
		return append(buf, input[start:s.pos]...), nil
	}
	content := s.readString()
	indices := node.re.FindAllIndex(content, -1)
	if indices == nil {
		return append(buf, input[start:s.pos]...), nil
	}
	buf = append(buf, '"')
	prev := 0
	for _, idx := range indices {
		buf = append(buf, content[prev:idx[0]]...)
		buf = append(buf, node.field...)
		prev = idx[1]
	}
	buf = append(buf, content[prev:]...)
	return append(buf, '"'), nil
}

// --- range ---

// execRange implements range(n), range(from;to), range(from;to;step).
//
// Tier 2 allocation: 1 alloc per generated value (the output byte slice).
// The allocation is proportional to the count the caller requested, not to
// the input being scanned.
//
// Supports errBreak for early exit: limit(3; range(100)) stops after 3 values.
// Float steps and negative steps are supported. Step of 0 returns an error.
func execRange(state execState, node *op, input []byte, fn func([]byte) error) error {
	fromVals, err := collectExecOutputs(state, node.left, input)
	if err != nil {
		return err
	}
	toVals, err := collectExecOutputs(state, node.right, input)
	if err != nil {
		return err
	}
	if len(fromVals) == 0 || len(toVals) == 0 {
		return nil
	}

	stepVals := [][]byte{[]byte("1")}
	if node.child != nil {
		stepVals, err = collectExecOutputs(state, node.child, input)
		if err != nil {
			return err
		}
		if len(stepVals) == 0 {
			return nil
		}
	}

	for _, fromBytes := range fromVals {
		from, ok := parseJSONFloat(fromBytes)
		if !ok {
			return fmt.Errorf("range: 'from' must be a number, got %s", fromBytes)
		}
		for _, toBytes := range toVals {
			to, ok := parseJSONFloat(toBytes)
			if !ok {
				return fmt.Errorf("range: 'to' must be a number, got %s", toBytes)
			}
			for _, stepBytes := range stepVals {
				step, ok := parseJSONFloat(stepBytes)
				if !ok {
					return fmt.Errorf("range: 'step' must be a number, got %s", stepBytes)
				}
				if step == 0 {
					return fmt.Errorf("range: step cannot be zero")
				}

				for i := from; (step > 0 && i < to) || (step < 0 && i > to); i += step {
					// 1 alloc per value — proportional to what was asked for (Tier 2).
					out := appendJSONFloat(nil, i)
					if err := fn(out); err != nil {
						if err == errBreak {
							return nil
						}
						return err
					}
				}
			}
		}
	}
	return nil
}
