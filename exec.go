package fastjq

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

var (
	errExpectedObjectField = errors.New("expected object for field access")
	errExpectedArrayIndex  = errors.New("expected array for index access")
	errExpectedIterable    = errors.New("expected array or object for .[]")
	errBreak               = errors.New("stop iteration") // sentinel for first/limit
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

// bTrue / bFalse / bNull are package-level literals returned directly when buf == nil,
// avoiding heap allocation for boolean and null results in zero-scratch evaluation paths.
var bTrue = []byte("true")
var bFalse = []byte("false")
var bNull = []byte("null")

var (
	tryScopeMu    sync.Mutex
	tryScopeByGID = make(map[uint64]int)
)

// execMulti executes an op against input, calling fn for each result.
// Single-output ops call fn once. Iterators call fn per element.
// buf is used as scratch space and may be reused across fn calls.
func execMulti(node *op, input []byte, buf []byte, fn func([]byte) error) error {
	switch node.typ {
	case opIdentity:
		result, err := execIdentity(input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opField:
		return execFieldMulti(node, input, buf, fn)
	case opDelete:
		result, err := execDelete(node, input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opPipe:
		return execPipeMulti(node, input, buf, fn)
	case opBind:
		baseCtx := currentExecContext()
		return execMulti(node.left, input, nil, func(bound []byte) error {
			return withExecContext(baseCtx.bindVar(node.name, bound), func() error {
				return execMulti(node.right, input, buf, fn)
			})
		})
	case opVar:
		ctx := currentExecContext()
		value, ok := ctx.lookupVar(node.name)
		if !ok {
			return fmt.Errorf("$%s is not defined", node.name)
		}
		if node.child != nil {
			return execMulti(node.child, value, buf, fn)
		}
		if buf == nil {
			return fn(value[:len(value):len(value)])
		}
		return fn(append(buf, value...))
	case opIndex:
		return execIndexMulti(node, input, buf, fn)
	case opIterator:
		return execIterator(node, input, buf, fn)
	case opConstruct:
		if node.multiValuePairs {
			return execConstructMulti(node, input, buf, fn)
		}
		result, err := execConstruct(node, input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opArrayConstruct:
		result, err := execArrayConstruct(node, input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opLiteral:
		return fn(normalizeOutputValue(node.literal, buf))
	case opTypeBuiltin:
		return execType(input, buf, fn)
	case opCompare:
		return execCompare(node, input, buf, fn)
	case opAnd:
		return execAnd(node, input, buf, fn)
	case opOr:
		return execOr(node, input, buf, fn)
	case opNot:
		if isFalsy(input) {
			return fn(append(buf, "true"...))
		}
		return fn(append(buf, "false"...))
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
		err := execMulti(node.child, input, buf, func(result []byte) error {
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
		return execLast(node, input, buf, fn)
	case opLimit:
		return execLimit(node, input, buf, fn)
	case opSkip:
		return execSkip(node, input, buf, fn)
	case opKeys:
		result, err := execKeys(input, buf)
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
	case opAny:
		return execAnyAll(node, input, buf, fn, false)
	case opAll:
		return execAnyAll(node, input, buf, fn, true)
	case opAdd:
		return execAdd(input, buf, fn)
	case opIndex1:
		return fn(execFindIndex(node, input, buf, false, false))
	case opRIndex1:
		return fn(execFindIndex(node, input, buf, true, false))
	case opIndicesN:
		return fn(execFindIndex(node, input, buf, false, true))
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
		return fn(execIn(node, input, buf))
	case opSlice:
		result, err := execSlice(node, input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opPlus:
		// Use execMulti for left side to support generators as operands (.[] + x etc.)
		return execMulti(node.left, input, nil, func(leftVal []byte) error {
			rightVal, err := execSingle(node.right, input, nil)
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
		result, err := execFlattenInto(input, buf, node)
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
	case opIsNaN:
		return fn(execIsNaN(input, buf))
	case opIsInfinite:
		return fn(execIsInfinite(input, buf))
	case opIsFinite:
		return fn(execIsFinite(input, buf))
	case opIsNormal:
		return fn(execIsNormal(input, buf))
	case opPow:
		result, err := execPow(node, input, buf)
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
		return fn(execTrimStr(input, buf, node.field, true))
	case opRtrimStr:
		return fn(execTrimStr(input, buf, node.field, false))
	case opEmpty:
		return nil // produce zero outputs — never call fn
	case opHas:
		return execHas(node, input, buf, fn)
	case opIf:
		return execIf(node, input, buf, fn)
	case opSelect:
		return execSelect(node, input, buf, fn)
	case opAlternative:
		return execAlternative(node, input, buf, fn)
	case opMinus, opMul, opDiv, opMod:
		// Supports generators as either operand (e.g. range(3) * 2 or 1 * range(3)).
		// Single-output right side: use execSingle to avoid fn being captured in a
		// nested execMulti call (which would create an escape analysis cycle).
		// Multi-output right side: collect values first without fn, then compute.
		if !hasMultiOutput(node.right) {
			return execMulti(node.left, input, nil, func(leftVal []byte) error {
				rightVal, err := execSingle(node.right, input, nil)
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
		var rightVals [][]byte
		if err := execMulti(node.right, input, nil, func(rightVal []byte) error {
			rightVals = append(rightVals, rightVal)
			return nil
		}); err != nil {
			return err
		}
		return execMulti(node.left, input, nil, func(leftVal []byte) error {
			for _, rv := range rightVals {
				result, err := execArithValues(node.typ, leftVal, rv, buf)
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
		result, err := execMinMax(input, buf, node, false)
		if err != nil {
			return err
		}
		return fn(result)
	case opMax:
		result, err := execMinMax(input, buf, node, true)
		if err != nil {
			return err
		}
		return fn(result)
	case opMinBy:
		result, err := execMinMax(input, buf, node, false)
		if err != nil {
			return err
		}
		return fn(result)
	case opMaxBy:
		result, err := execMinMax(input, buf, node, true)
		if err != nil {
			return err
		}
		return fn(result)
	case opSort:
		return execSort(input, buf, fn)
	case opSortBy:
		return execSortBy(node, input, buf, fn)
	case opUnique:
		return execUnique(input, buf, fn)
	case opUniqueBy:
		return execUniqueBy(node, input, buf, fn)
	case opGroupBy:
		return execGroupBy(node, input, buf, fn)
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
		err := withTryScope(func() error {
			return execMulti(node.left, input, buf, wrappedFn)
		})
		if err == nil {
			return nil
		}
		if de, ok := err.(*downstreamError); ok {
			return de.err
		}
		if err == errBreak {
			return err
		}
		// Real error — suppress or run catch handler
		if node.right == nil {
			return nil
		}
		err = execMulti(node.right, catchInputFromError(err), buf, wrappedFn)
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
		argVal, err := execSingle(node.child, input, nil)
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
		return execRange(node, input, fn)
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
			val, err := execSingle(node.child, input, nil)
			if err != nil {
				return err
			}
			payload = append([]byte(nil), trimWhitespace(val)...)
		} else {
			payload = append([]byte(nil), trimWhitespace(input)...)
		}
		return &jsonError{payload: payload}
	case opStringInterp:
		// String interpolation "\(expr1)...\(expr2)".
		// Each segs[i] is a literal segment; each elems[i] is an expression.
		// Segments and expressions are interleaved: segs[0] expr[0] segs[1] ... segs[n].
		buf = append(buf, '"')
		for i, expr := range node.elems {
			buf = append(buf, node.segs[i]...)
			result, err := execSingle(expr, input, nil)
			if err != nil {
				return err
			}
			result = trimWhitespace(result)
			if len(result) > 0 && result[0] == '"' {
				// JSON string: embed raw content bytes (already valid JSON string content).
				sc := scanner{data: result}
				buf = append(buf, sc.readString()...)
			} else {
				// Non-string (number, bool, null, object, array): embed bytes, escaping " and \.
				for _, b := range result {
					if b == '"' {
						buf = append(buf, '\\', '"')
					} else if b == '\\' {
						buf = append(buf, '\\', '\\')
					} else {
						buf = append(buf, b)
					}
				}
			}
		}
		buf = append(buf, node.segs[len(node.elems)]...) // final literal segment
		buf = append(buf, '"')
		return fn(buf)
	case opIsEmpty:
		// isempty(expr): true if expr produces no outputs, false otherwise.
		produced := false
		err := execMulti(node.child, input, buf, func(_ []byte) error {
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
			v, _ := execSingle(node.left, input, nil)
			return v
		}()))
		if !ok {
			return nil
		}
		nInt := int(nf)
		if nInt < 0 {
			return nil
		}
		count := 0
		var found []byte
		err := execMulti(node.child, input, nil, func(val []byte) error {
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
			if err := execMulti(elem, input, buf, fn); err != nil {
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
func execSingle(node *op, input []byte, buf []byte) ([]byte, error) {
	switch node.typ {
	case opLiteral:
		return normalizeOutputValue(node.literal, buf), nil
	case opIdentity:
		return execIdentity(input, buf)
	case opField:
		return execField(node, input, buf)
	case opIndex:
		return execIndex(node, input, buf)
	case opVar:
		ctx := currentExecContext()
		value, ok := ctx.lookupVar(node.name)
		if !ok {
			return nil, fmt.Errorf("$%s is not defined", node.name)
		}
		if node.child != nil {
			return execSingle(node.child, value, buf)
		}
		if buf == nil {
			return value[:len(value):len(value)], nil
		}
		return append(buf, value...), nil
	case opTypeBuiltin:
		return execTypeSingle(input, buf)
	case opCompare:
		return execCompareSingle(node, input, buf)
	case opAnd:
		leftVal, err := execSingle(node.left, input, buf)
		if err != nil {
			return nil, err
		}
		if isFalsy(leftVal) {
			if buf == nil {
				return bFalse, nil
			}
			return append(buf[:0], "false"...), nil
		}
		rightVal, err := execSingle(node.right, input, buf)
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
		leftVal, err := execSingle(node.left, input, buf)
		if err != nil {
			return nil, err
		}
		if !isFalsy(leftVal) {
			if buf == nil {
				return bTrue, nil
			}
			return append(buf[:0], "true"...), nil
		}
		rightVal, err := execSingle(node.right, input, buf)
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
	case opLength:
		return execLengthSingle(input, buf)
	case opAbs:
		return execAbs(input, buf)
	case opToEntries:
		return execToEntries(input, buf)
	case opFromEntries:
		return execFromEntries(input, buf)
	case opFirst:
		// exec already returns the first result via execMulti
		return exec(node.child, input, buf)
	case opLast:
		return execLastSingle(node, input, buf)
	case opSkip:
		return execFirstResult(node, input, buf)
	case opKeys:
		return execKeys(input, buf)
	case opKeysUnsorted:
		return execKeysUnsorted(input, buf)
	case opAny:
		return execAnyAllSingle(node, input, buf, false)
	case opAll:
		return execAnyAllSingle(node, input, buf, true)
	case opIndex1:
		return execFindIndex(node, input, buf, false, false), nil
	case opRIndex1:
		return execFindIndex(node, input, buf, true, false), nil
	case opIndicesN:
		return execFindIndex(node, input, buf, false, true), nil
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
		return execIn(node, input, buf), nil
	case opSlice:
		return execSlice(node, input, buf)
	case opPlus:
		return execPlusSingle(node, input, buf)
	case opFlatten:
		return execFlattenInto(input, buf, node)
	case opSplit:
		return execSplit(input, buf, node.field), nil
	case opJoin:
		return execJoin(input, buf, node.field)
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
	case opLtrimStr:
		return execTrimStr(input, buf, node.field, true), nil
	case opRtrimStr:
		return execTrimStr(input, buf, node.field, false), nil
	case opExplode:
		return execExplode(input, buf)
	case opImplode:
		return execImplode(input, buf)
	case opIsNaN:
		return execIsNaN(input, buf), nil
	case opIsInfinite:
		return execIsInfinite(input, buf), nil
	case opIsFinite:
		return execIsFinite(input, buf), nil
	case opIsNormal:
		return execIsNormal(input, buf), nil
	case opPow:
		return execPow(node, input, buf)
	case opMinus, opMul, opDiv, opMod:
		return execArith(node, input, buf)
	case opMin, opMinBy:
		return execMinMax(input, buf, node, false)
	case opMax, opMaxBy:
		return execMinMax(input, buf, node, true)
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
		return execDelete(node, input, buf)

	case opConstruct:
		if node.multiValuePairs {
			return execFirstResult(node, input, buf) // Tier 2 — Cartesian product
		}
		return execConstruct(node, input, buf)

	case opPipe:
		// Fast path when left is single-output: chain execSingle calls directly.
		// When left is multi-output (e.g. .[] | select(...)), fall through to
		// execFirstResult so the full pipeline is evaluated and only the first
		// passing result is returned (correct for first(.[] | select(...)) etc.).
		if isSingleOutputOp(node.left) {
			intermediate, err := execSingle(node.left, input, nil)
			if err != nil {
				return nil, err
			}
			return execSingle(node.right, intermediate, buf)
		}
		return execFirstResult(node, input, buf)

	case opSelect:
		// Pass buf as scratch so condition expressions (e.g. ascii_downcase, construct)
		// don't need to allocate their own buffer. condVal may be written into buf,
		// but we only test truthiness and then reset buf for the actual output.
		condVal, err := execSingle(node.child, input, buf)
		if err != nil {
			return nil, err
		}
		if isFalsy(condVal) {
			return nil, nil // condition false — no output
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
		condVal, err := execSingle(node.left, input, buf)
		if err != nil {
			return nil, err
		}
		if !isFalsy(condVal) {
			return execSingle(node.right, input, buf[:0])
		}
		if node.child != nil {
			return execSingle(node.child, input, buf[:0])
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
		leftVal, err := execSingle(node.left, input, nil)
		if err == nil && !isFalsy(leftVal) {
			if buf == nil {
				return leftVal, nil
			}
			return append(buf, leftVal...), nil
		}
		return execSingle(node.right, input, buf)

	case opTry:
		var result []byte
		err := withTryScope(func() error {
			var execErr error
			result, execErr = execSingle(node.left, input, buf)
			return execErr
		})
		if err == nil {
			return result, nil
		}
		if err == errBreak {
			return nil, err // propagate break signal
		}
		// Error: suppress or run catch handler.
		if node.right == nil {
			return nil, nil // no catch — produce no output
		}
		return execSingle(node.right, catchInputFromError(err), buf)

	case opStringInterp:
		// String interpolation — inline exactly as in execMulti but return directly.
		buf = append(buf, '"')
		for i, expr := range node.elems {
			buf = append(buf, node.segs[i]...)
			result, err := execSingle(expr, input, nil)
			if err != nil {
				return nil, err
			}
			result = trimWhitespace(result)
			if len(result) > 0 && result[0] == '"' {
				sc := scanner{data: result}
				buf = append(buf, sc.readString()...)
			} else {
				for _, b := range result {
					if b == '"' {
						buf = append(buf, '\\', '"')
					} else if b == '\\' {
						buf = append(buf, '\\', '\\')
					} else {
						buf = append(buf, b)
					}
				}
			}
		}
		buf = append(buf, node.segs[len(node.elems)]...)
		return append(buf, '"'), nil

	case opContains:
		argVal, err := execSingle(node.child, input, nil)
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
		return execFirstResult(node, input, buf)
	}
}

// execFirstResult executes node via the full execMulti callback machinery and
// returns the first result. Used as the fallback for multi-output ops (iterators,
// pipes with multi-output left sides, scan, etc.) that execSingle cannot handle
// directly without closures.
func execFirstResult(node *op, input []byte, buf []byte) ([]byte, error) {
	var result []byte
	err := execMulti(node, input, buf, func(r []byte) error {
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
func exec(node *op, input []byte, buf []byte) ([]byte, error) {
	return execSingle(node, input, buf)
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
func execFieldMulti(node *op, input []byte, buf []byte, fn func([]byte) error) error {
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
		return execMulti(node.child, value, buf, fn)
	}
	if buf == nil {
		return fn(value[:len(value):len(value)])
	}
	return fn(append(buf, value...))
}

// execField extracts a field value from a JSON object (single-result convenience).
func execField(node *op, input []byte, buf []byte) ([]byte, error) {
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
		return exec(node.child, value, buf)
	}
	if buf == nil {
		return value[:len(value):len(value)], nil
	}
	return append(buf, value...), nil
}

// execIndexMulti accesses an array element by index, then recurses into
// the child (if any) via execMulti to support multi-output children.
func execIndexMulti(node *op, input []byte, buf []byte, fn func([]byte) error) error {
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
		return execMulti(node.child, result, buf, fn)
	}
	if buf == nil {
		return fn(result[:len(result):len(result)])
	}
	return fn(append(buf, result...))
}

// execIndex accesses an array element by index. Negative indices count from end.
func execIndex(node *op, input []byte, buf []byte) ([]byte, error) {
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
		return exec(node.child, result, buf)
	}
	if buf == nil {
		return result[:len(result):len(result)], nil
	}
	return append(buf, result...), nil
}

// execIterator iterates all elements of an array or values of an object.
func execIterator(node *op, input []byte, buf []byte, fn func([]byte) error) error {
	s := &scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) {
		if node.optional {
			return nil
		}
		return errExpectedIterable
	}

	switch s.data[s.pos] {
	case '[':
		var caught error
		s.arrayIter(func(index int, elemStart, elemEnd int) bool {
			if err := fn(input[elemStart:elemEnd]); err != nil {
				// Propagate jsonError (from `error` builtin) and errBreak so
				// try-catch and limit/first work correctly. Regular errors
				// (e.g. field access on wrong type) are dropped, preserving
				// the existing lenient multi-output behaviour.
				if _, ok := err.(*jsonError); ok {
					if !tryScopeActive() {
						return false
					}
					caught = err
				} else if err == errBreak {
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
			if err := fn(input[valueStart:valueEnd]); err != nil {
				if _, ok := err.(*jsonError); ok {
					if !tryScopeActive() {
						return false
					}
					caught = err
				} else if err == errBreak {
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
		return errExpectedIterable
	}
}

// execDelete removes specified fields from a JSON object or elements from an array.
// Never copies commas from input — reconstructs with our own commas.
func execDelete(node *op, input []byte, buf []byte) ([]byte, error) {
	s := &scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) {
		return nil, fmt.Errorf("expected object or array for del()")
	}

	switch s.data[s.pos] {
	case '{':
		return execDeleteObject(node, input, buf, s)
	case '[':
		return execDeleteArray(node, input, buf, s)
	default:
		return nil, fmt.Errorf("expected object or array for del()")
	}
}

// execDeleteObject removes specified fields from a JSON object.
func execDeleteObject(node *op, input []byte, buf []byte, s *scanner) ([]byte, error) {
	// Validate all del() arguments are field accesses
	for i := range node.fields {
		if node.fields[i].typ != opField {
			return nil, fmt.Errorf("del() argument must be a field access for object input")
		}
	}

	buf = append(buf, '{')
	first := true

	s.objectIter(func(key []byte, valueStart, valueEnd int) bool {
		for i := range node.fields {
			d := &node.fields[i]
			if bytesEqualStr(key, d.field) {
				if d.child == nil {
					return true // simple delete — skip this pair
				}
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
				nestedNode := &op{typ: opDelete, fields: []op{*d.child}}
				var err error
				buf, err = execDelete(nestedNode, input[valueStart:valueEnd], buf)
				if err != nil {
					// Nested target is not an object/array — keep original value.
					buf = append(preDel, input[valueStart:valueEnd]...)
				}
				return true
			}
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
func execDeleteArray(node *op, input []byte, buf []byte, s *scanner) ([]byte, error) {
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
			start, end, err := resolveDelSliceBounds(d, input, length)
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
func resolveDelSliceBounds(node *op, input []byte, length int) (start, end int, err error) {
	start = 0
	end = length
	if node.left != nil {
		sv, e := execSingle(node.left, input, nil)
		if e != nil {
			return 0, 0, e
		}
		f, ok := parseJSONFloat(sv)
		if !ok {
			return 0, 0, fmt.Errorf("slice index must be a number")
		}
		start = int(f)
		if start < 0 {
			start += length
		}
		if start < 0 {
			start = 0
		}
		if start > length {
			start = length
		}
	}
	if node.right != nil {
		sv, e := execSingle(node.right, input, nil)
		if e != nil {
			return 0, 0, e
		}
		f, ok := parseJSONFloat(sv)
		if !ok {
			return 0, 0, fmt.Errorf("slice index must be a number")
		}
		end = int(f)
		if end < 0 {
			end += length
		}
		if end < 0 {
			end = 0
		}
		if end > length {
			end = length
		}
	}
	if start > end {
		start = end
	}
	return start, end, nil
}

// execPipeMulti runs left, then feeds each result into right.
func execPipeMulti(node *op, input []byte, buf []byte, fn func([]byte) error) error {
	return execMulti(node.left, input, nil, func(intermediate []byte) error {
		return execMulti(node.right, intermediate, buf, fn)
	})
}

// execConstruct builds a JSON object from key-expression pairs (single-output per pair).
// Used by the execSingle fast path via execFirstResult for common cases.
func execConstruct(node *op, input []byte, buf []byte) ([]byte, error) {
	buf = append(buf, '{')
	for i, p := range node.pairs {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = append(buf, '"')
		buf = append(buf, p.key...)
		buf = append(buf, '"', ':')
		val, err := exec(p.expr, input, buf[len(buf):len(buf):cap(buf)])
		if err != nil {
			return nil, fmt.Errorf("in object construction for key %q: %w", p.key, err)
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
func execConstructMulti(node *op, input []byte, buf []byte, fn func([]byte) error) error {
	var combos [][]byte
	if err := collectPairCombos(node.pairs, 0, input, append(buf[:0], '{'), &combos); err != nil {
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
func collectPairCombos(pairs []pair, idx int, input []byte, prefix []byte, out *[][]byte) error {
	if idx == len(pairs) {
		*out = append(*out, append(prefix, '}'))
		return nil
	}

	p := pairs[idx]
	isFirst := prefix[len(prefix)-1] == '{'

	return execMulti(p.expr, input, nil, func(val []byte) error {
		// Build this pair's key:value, branching independently for each val.
		var obj []byte
		obj = append(obj, prefix...)
		if !isFirst {
			obj = append(obj, ',')
		}
		obj = append(obj, '"')
		obj = append(obj, p.key...)
		obj = append(obj, '"', ':')
		obj = append(obj, normalizeNaNInf(val)...)
		return collectPairCombos(pairs, idx+1, input, obj, out)
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
func execArrayConstruct(node *op, input []byte, buf []byte) ([]byte, error) {
	buf = append(buf, '[')
	first := true
	for _, elem := range node.elems {
		err := execMulti(elem, input, nil, func(val []byte) error {
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
func execCompareSingle(node *op, input []byte, buf []byte) ([]byte, error) {
	leftVal, err := execSingle(node.left, input, buf)
	if err != nil {
		return nil, err
	}
	// When buf is nil, leftVal is a cap-limited sub-slice (no spare capacity).
	// Use nil scratch for the right side to avoid writing into input's backing array.
	var rightBuf []byte
	if buf != nil {
		rightBuf = leftVal[len(leftVal):len(leftVal):cap(leftVal)]
	}
	rightVal, err := execSingle(node.right, input, rightBuf)
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
func execCompare(node *op, input []byte, buf []byte, fn func([]byte) error) error {
	if !hasMultiOutput(node.right) {
		// Single-output right side: evaluate once per left value with execSingle.
		return execMulti(node.left, input, nil, func(leftVal []byte) error {
			rightVal, err := execSingle(node.right, input, nil)
			if err != nil {
				return err
			}
			return fn(boolResult(buf, evalCmpOp(node.cmpOp, leftVal, rightVal)))
		})
	}
	// Multi-output right side: collect right values first (fn not captured here),
	// then iterate left, calling fn directly without any nested execMulti call.
	var rightVals [][]byte
	if err := execMulti(node.right, input, nil, func(rightVal []byte) error {
		rightVals = append(rightVals, rightVal)
		return nil
	}); err != nil {
		return err
	}
	return execMulti(node.left, input, nil, func(leftVal []byte) error {
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
func execAnd(node *op, input []byte, buf []byte, fn func([]byte) error) error {
	leftVal, err := execSingle(node.left, input, buf)
	if err != nil {
		return err
	}
	if isFalsy(leftVal) {
		return fn(append(buf[:0], "false"...))
	}
	rightVal, err := execSingle(node.right, input, buf)
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
func execOr(node *op, input []byte, buf []byte, fn func([]byte) error) error {
	leftVal, err := execSingle(node.left, input, buf)
	if err != nil {
		return err
	}
	if !isFalsy(leftVal) {
		return fn(append(buf[:0], "true"...))
	}
	rightVal, err := execSingle(node.right, input, buf)
	if err != nil {
		return err
	}
	if !isFalsy(rightVal) {
		return fn(append(buf[:0], "true"...))
	}
	return fn(append(buf[:0], "false"...))
}

// execSelect evaluates a condition and emits the input if truthy, nothing if falsy.
func execSelect(node *op, input []byte, buf []byte, fn func([]byte) error) error {
	condVal, err := execSingle(node.child, input, buf)
	if err != nil {
		return err
	}
	if isFalsy(condVal) {
		return nil // zero outputs
	}
	return fn(input)
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
func execLast(node *op, input []byte, buf []byte, fn func([]byte) error) error {
	result, err := execLastSingle(node, input, buf)
	if err != nil {
		return err
	}
	if result == nil {
		return nil // no outputs
	}
	return fn(result)
}

func execLastSingle(node *op, input []byte, buf []byte) ([]byte, error) {
	// Keep a reference to the last result — no copy needed.
	// Results from execMulti with nil buf are either sub-slices of input
	// (safe for lifetime of this call) or global literals (bTrue/bFalse).
	var lastResult []byte
	err := execMulti(node.child, input, nil, func(result []byte) error {
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
func execLimit(node *op, input []byte, buf []byte, fn func([]byte) error) error {
	nVal, err := execSingle(node.left, input, nil)
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
	err = execMulti(node.child, input, buf, func(result []byte) error {
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

func execSkip(node *op, input []byte, buf []byte, fn func([]byte) error) error {
	return execMulti(node.left, input, nil, func(countVal []byte) error {
		nf, ok := parseJSONFloat(countVal)
		if !ok {
			return fmt.Errorf("skip: count must be a number")
		}
		n := int(nf)
		if n < 0 {
			return fmt.Errorf("skip doesn't support negative count")
		}
		skipped := 0
		return execMulti(node.child, input, buf, func(result []byte) error {
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
func execFlattenInto(input []byte, buf []byte, node *op) ([]byte, error) {
	maxDepth := node.index
	if node.child != nil {
		// flatten(n) — evaluate n
		depthVal, err := execSingle(node.child, input, nil)
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

// execSlice implements .[n:m], .[:m], .[n:] on arrays and strings.
// node.left = start expr (nil = 0), node.right = end expr (nil = length).
// Negative indices count from the end. Zero-alloc: writes directly into buf.
func execSlice(node *op, input []byte, buf []byte) ([]byte, error) {
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
				p++
			}
			length++
		}
	default:
		return append(buf, "null"...), nil
	}

	// Resolve start bound
	start := 0
	if node.left != nil {
		sv, err := execSingle(node.left, input, nil)
		if err != nil {
			return nil, err
		}
		f, ok := parseJSONFloat(sv)
		if !ok {
			return nil, fmt.Errorf("slice index must be a number")
		}
		start = int(f)
		if start < 0 {
			start += length
		}
		if start < 0 {
			start = 0
		}
		if start > length {
			start = length
		}
	}

	// Resolve end bound
	end := length
	if node.right != nil {
		sv, err := execSingle(node.right, input, nil)
		if err != nil {
			return nil, err
		}
		f, ok := parseJSONFloat(sv)
		if !ok {
			return nil, fmt.Errorf("slice index must be a number")
		}
		end = int(f)
		if end < 0 {
			end += length
		}
		if end < 0 {
			end = 0
		}
		if end > length {
			end = length
		}
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
				s.pos++
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

// execPlus implements expr + expr: null identity, string concat, array concat, numeric add.
// Uses nil scratch for operands so results are input sub-slices (zero-alloc).
func execPlus(node *op, input []byte, buf []byte, fn func([]byte) error) error {
	result, err := execPlusSingle(node, input, buf)
	if err != nil {
		return err
	}
	return fn(result)
}

func execPlusSingle(node *op, input []byte, buf []byte) ([]byte, error) {
	// Evaluate both operands with nil scratch — cap-limited sub-slices, no alloc.
	leftVal, err := execSingle(node.left, input, nil)
	if err != nil {
		return nil, err
	}
	rightVal, err := execSingle(node.right, input, nil)
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
// Handles both standard (+/) and URL-safe (-_) base64. Strips whitespace.
func execBase64Decode(input []byte, buf []byte) ([]byte, error) {
	s := &scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '"' {
		return nil, fmt.Errorf("@base64d input must be a string")
	}
	content := s.readString() // raw base64 bytes between quotes

	buf = append(buf, '"')

	// Decode base64: collect 4-char groups, track padding separately.
	// Handles standard, URL-safe (-/_), and unpadded inputs.
	var vals [4]byte
	var pads [4]bool // true if the char was '=' padding
	n := 0
	for _, ch := range content {
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			continue
		}
		if ch == '=' {
			pads[n] = true
			vals[n] = 0
		} else {
			v, ok := base64DecodeChar(ch)
			if !ok {
				return nil, fmt.Errorf("@base64d: invalid character %q", ch)
			}
			vals[n] = v
		}
		n++
		if n == 4 {
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
	if n == 3 {
		buf = appendJSONByte(buf, (vals[0]<<2)|(vals[1]>>4))
		buf = appendJSONByte(buf, ((vals[1]&0x0F)<<4)|(vals[2]>>2))
	} else if n == 2 {
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
func execIn(node *op, input []byte, buf []byte) []byte {
	// Evaluate the container expression
	container, err := execSingle(node.child, input, nil)
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

// execDebug prints the input to stderr and is used by opDebug.
func execDebug(input []byte) {
	fmt.Fprintf(os.Stderr, "[DEBUG]: %s\n", input)
}

// execFindIndex implements index(s), rindex(s), and indices(s).
//
//	last=true, all=false  → rindex: last occurrence
//	last=false, all=false → index:  first occurrence, null if not found
//	last=false, all=true  → indices: all occurrences as array
func execFindIndex(node *op, input []byte, buf []byte, last, all bool) []byte {
	// Evaluate the search value
	searchVal, err := execSingle(node.child, input, nil)
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

// execAnyAll implements any/all with optional expr argument.
// wantAll=false → any (true if at least one match), wantAll=true → all (true if all match).
func execAnyAll(node *op, input []byte, buf []byte, fn func([]byte) error, wantAll bool) error {
	result, err := execAnyAllSingle(node, input, buf, wantAll)
	if err != nil {
		return err
	}
	return fn(result)
}

func execAnyAllSingle(node *op, input []byte, buf []byte, wantAll bool) ([]byte, error) {
	// Two-arg form: any(gen; cond) / all(gen; cond)
	if node.left != nil {
		breakFlag := false
		var iterErr error
		err := execMulti(node.left, input, nil, func(elem []byte) error {
			condVal, _ := execSingle(node.child, elem, nil)
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
			condVal, _ := execSingle(node.child, elem, nil)
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
			if s.pos+1 < len(s.data) {
				buf = append(buf, ch, s.data[s.pos+1])
				s.pos += 2
			}
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
func execPow(node *op, input []byte, buf []byte) ([]byte, error) {
	xVal, err := execSingle(node.left, input, nil)
	if err != nil {
		return nil, err
	}
	yVal, err := execSingle(node.right, input, nil)
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

// execTrimStr implements ltrimstr (left=true) and rtrimstr (left=false).
// If the input string starts/ends with s, returns the trimmed string.
// If no match, returns the input unchanged (cap-limited zero-alloc sub-slice when buf is nil).
func execTrimStr(input []byte, buf []byte, s string, left bool) []byte {
	sc := &scanner{data: input}
	sc.skipWhitespace()
	start := sc.pos
	if sc.pos >= len(sc.data) || sc.data[sc.pos] != '"' {
		if buf == nil {
			end := sc.pos
			return input[start:end:end]
		}
		return append(buf, input[start:]...)
	}
	content := sc.readString()
	end := sc.pos

	var match bool
	var trimmed []byte
	if left {
		match = len(content) >= len(s) && bytesEqualStr(content[:len(s)], s)
		if match {
			trimmed = content[len(s):]
		}
	} else {
		match = len(content) >= len(s) && bytesEqualStr(content[len(content)-len(s):], s)
		if match {
			trimmed = content[:len(content)-len(s)]
		}
	}

	if !match {
		if buf == nil {
			return input[start:end:end]
		}
		return append(buf, input[start:end]...)
	}

	buf = append(buf, '"')
	buf = append(buf, trimmed...)
	buf = append(buf, '"')
	return buf
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
	case opIdentity, opField, opIndex, opLiteral, opCompare, opAnd, opOr, opNot,
		opLength, opHas, opIn, opSlice, opPlus, opMinus, opMul, opDiv, opMod,
		opAdd, opFlatten, opSelect, opAlternative, opTypeBuiltin, opToEntries,
		opFromEntries, opToJSON, opFromJSON, opToString, opToNumber,
		opBase64, opBase64D, opAsciiDowncase, opAsciiUpcase,
		opStartsWith, opEndsWith, opSplit, opJoin, opURIEncode,
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
func execIf(node *op, input []byte, buf []byte, fn func([]byte) error) error {
	// Fast path for single-output conditions (common case): avoid closure allocation.
	if isSingleOutputOp(node.left) {
		condVal, err := execSingle(node.left, input, nil)
		if err != nil {
			return err
		}
		if !isFalsy(condVal) {
			return execMulti(node.right, input, buf, fn)
		}
		if node.child != nil {
			return execMulti(node.child, input, buf, fn)
		}
		return fn(input)
	}
	return execMulti(node.left, input, nil, func(condVal []byte) error {
		if !isFalsy(condVal) {
			return execMulti(node.right, input, buf, fn)
		}
		if node.child != nil {
			return execMulti(node.child, input, buf, fn)
		}
		// default else: identity
		return fn(input)
	})
}

// execAlternative collects all truthy outputs from left; if none, evaluates right.
// jq semantics: (null, false, 3) // 18 → 3 (truthy outputs pass through).
//
//	(null, false) // 18 → 18 (no truthy outputs, use right).
func execAlternative(node *op, input []byte, buf []byte, fn func([]byte) error) error {
	anyTruthy := false
	err := execMulti(node.left, input, buf, func(result []byte) error {
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
		return execMulti(node.right, input, buf, fn)
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
		return nil, fmt.Errorf("cannot subtract %q from %q", rightVal, leftVal)

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
func execArith(node *op, input []byte, buf []byte) ([]byte, error) {
	leftVal, err := execSingle(node.left, input, nil)
	if err != nil {
		return nil, err
	}
	rightVal, err := execSingle(node.right, input, nil)
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
func execMinMax(input []byte, buf []byte, node *op, wantMax bool) ([]byte, error) {
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
			k, err := execSingle(node.child, elem, nil)
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
func collectElemKeys(node *op, input []byte) (elems [][]byte, keys [][][]byte, err error) {
	s := scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '[' {
		return nil, nil, fmt.Errorf("sort_by/group_by/unique_by input must be an array")
	}
	s.arrayIter(func(_ int, start, end int) bool {
		elem := input[start:end:end]
		var ks [][]byte
		execErr := execMulti(node, elem, nil, func(k []byte) error {
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
func execSortBy(node *op, input []byte, buf []byte, fn func([]byte) error) error {
	elems, keys, err := collectElemKeys(node.child, input)
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
func execUniqueBy(node *op, input []byte, buf []byte, fn func([]byte) error) error {
	elems, keys, err := collectElemKeys(node.child, input)
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
func execGroupBy(node *op, input []byte, buf []byte, fn func([]byte) error) error {
	elems, keys, err := collectElemKeys(node.child, input)
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
		af, _ := parseJSONFloat(a)
		bf, _ := parseJSONFloat(b)
		if af < bf {
			return -1
		}
		if af > bf {
			return 1
		}
		return 0
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
	for i := 0; i < len(value); {
		b := value[i]
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
	if !json.Valid(result) {
		return nil, fromJSONError(result)
	}
	return buf, nil
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

func withTryScope(fn func() error) error {
	gid := currentGID()
	tryScopeMu.Lock()
	tryScopeByGID[gid]++
	tryScopeMu.Unlock()
	defer func() {
		tryScopeMu.Lock()
		if tryScopeByGID[gid] <= 1 {
			delete(tryScopeByGID, gid)
		} else {
			tryScopeByGID[gid]--
		}
		tryScopeMu.Unlock()
	}()
	return fn()
}

func tryScopeActive() bool {
	gid := currentGID()
	tryScopeMu.Lock()
	active := tryScopeByGID[gid] > 0
	tryScopeMu.Unlock()
	return active
}

func currentGID() uint64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	var id uint64
	for i := len("goroutine "); i < n; i++ {
		b := buf[i]
		if b < '0' || b > '9' {
			break
		}
		id = id*10 + uint64(b-'0')
	}
	return id
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
	raw := s.readString() // raw JSON string content, e.g. %CE%BC

	// Percent-decode to raw bytes
	decoded := make([]byte, 0, len(raw))
	for i := 0; i < len(raw); i++ {
		if raw[i] == '%' && i+2 < len(raw) {
			hi := hexNibble(raw[i+1])
			lo := hexNibble(raw[i+2])
			decoded = append(decoded, byte(hi<<4)|byte(lo))
			i += 2
		} else {
			decoded = append(decoded, raw[i])
		}
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
func execRange(node *op, input []byte, fn func([]byte) error) error {
	fromBytes, err := execSingle(node.left, input, nil)
	if err != nil {
		return err
	}
	toBytes, err := execSingle(node.right, input, nil)
	if err != nil {
		return err
	}

	from, ok := parseJSONFloat(fromBytes)
	if !ok {
		return fmt.Errorf("range: 'from' must be a number, got %s", fromBytes)
	}
	to, ok := parseJSONFloat(toBytes)
	if !ok {
		return fmt.Errorf("range: 'to' must be a number, got %s", toBytes)
	}

	step := 1.0
	if node.child != nil {
		stepBytes, err := execSingle(node.child, input, nil)
		if err != nil {
			return err
		}
		step, ok = parseJSONFloat(stepBytes)
		if !ok {
			return fmt.Errorf("range: 'step' must be a number, got %s", stepBytes)
		}
		if step == 0 {
			return fmt.Errorf("range: step cannot be zero")
		}
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
	return nil
}
