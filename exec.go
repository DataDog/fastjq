package fastjq

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
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

// bTrue / bFalse / bNull are package-level literals returned directly when buf == nil,
// avoiding heap allocation for boolean and null results in zero-scratch evaluation paths.
var bTrue = []byte("true")
var bFalse = []byte("false")
var bNull = []byte("null")

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
	case opIndex:
		return execIndexMulti(node, input, buf, fn)
	case opIterator:
		return execIterator(node, input, buf, fn)
	case opConstruct:
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
		return fn(append(buf, node.literal...))
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
	case opStartsWith:
		return fn(execStringPredicate(input, buf, node.field, true, false))
	case opEndsWith:
		return fn(execStringPredicate(input, buf, node.field, false, true))
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
		// Use execMulti for left side to support generators as operands (.[] % 7 etc.)
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
	case opURIEncode:
		result, err := execURIEncode(input, buf)
		if err != nil {
			return err
		}
		return fn(result)
	case opTry:
		err := execMulti(node.left, input, buf, fn)
		if err == nil || err == errBreak {
			return err
		}
		// Real error — suppress or run catch handler
		if node.right == nil {
			return nil
		}
		// Determine what to pass to the catch handler.
		// If the error was thrown by jq's `error` builtin, pass the raw JSON value.
		// Otherwise, wrap the error message as a JSON string.
		var catchInput []byte
		if je, ok := err.(*jsonError); ok {
			catchInput = je.payload
		} else {
			// Build error message as JSON string (exceptional path, alloc is fine)
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
			catchInput = append(msg, '"')
		}
		return execMulti(node.right, catchInput, buf, fn)
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
		// When buf is nil, return the compile-time literal bytes directly (zero-alloc).
		// Safe since callers only read the result, never append beyond len.
		if buf == nil {
			return node.literal, nil
		}
		return append(buf, node.literal...), nil
	case opIdentity:
		return execIdentity(input, buf)
	case opField:
		return execField(node, input, buf)
	case opIndex:
		return execIndex(node, input, buf)
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
	case opToEntries:
		return execToEntries(input, buf)
	case opFromEntries:
		return execFromEntries(input, buf)
	case opFirst:
		// exec already returns the first result via execMulti
		return exec(node.child, input, buf)
	case opLast:
		return execLastSingle(node, input, buf)
	case opKeysUnsorted:
		return execKeysUnsorted(input, buf)
	case opAny:
		return execAnyAllSingle(node, input, buf, false)
	case opAll:
		return execAnyAllSingle(node, input, buf, true)
	case opAdd:
		return exec(node, input, buf)
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
	case opLtrimStr:
		return execTrimStr(input, buf, node.field, true), nil
	case opRtrimStr:
		return execTrimStr(input, buf, node.field, false), nil
	case opMinus, opMul, opDiv, opMod:
		return execArith(node, input, buf)
	case opMin, opMinBy:
		return execMinMax(input, buf, node, false)
	case opMax, opMaxBy:
		return execMinMax(input, buf, node, true)
	case opURIEncode:
		return execURIEncode(input, buf)
	case opAlternative:
		// Fall through to execMulti — alternative needs multi-output left side support
		return exec(node, input, buf)
	case opTry:
		// Fall through to execMulti for try (handles errBreak propagation correctly)
		return exec(node, input, buf)
	case opToJSON:
		return execToJSON(input, buf), nil
	case opFromJSON:
		return execFromJSON(input, buf)
	case opToString:
		return execToString(input, buf), nil
	case opToNumber:
		return execToNumber(input, buf)
	default:
		return exec(node, input, buf)
	}
}

// exec executes an op against input, writing the result into buf.
// Returns the result as a sub-slice of buf. For single-output ops only.
func exec(node *op, input []byte, buf []byte) ([]byte, error) {
	var result []byte
	var firstErr error
	err := execMulti(node, input, buf, func(r []byte) error {
		if result == nil {
			result = r
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if firstErr != nil {
		return nil, firstErr
	}
	if result == nil {
		return append(buf, "null"...), nil
	}
	return result, nil
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
	if buf == nil {
		end := s.pos
		return input[start:end:end], nil
	}
	return append(buf, input[start:s.pos]...), nil
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
		return errExpectedObjectField
	}

	fieldName := []byte(node.field)
	vs, ve := s.findField(fieldName)
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
	// When buf is nil, pass a cap-limited sub-slice directly (zero-alloc).
	// Cap-limited prevents callers from treating spare capacity as scratch.
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
		return nil, errExpectedObjectField
	}

	fieldName := []byte(node.field)
	vs, ve := s.findField(fieldName)
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
		return errExpectedArrayIndex
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
		return nil, errExpectedArrayIndex
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

// execConstruct builds a JSON object from key-expression pairs.
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
		buf = append(buf, val...)
	}
	buf = append(buf, '}')
	return buf, nil
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
			buf = append(buf, val...)
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
func execCompare(node *op, input []byte, buf []byte, fn func([]byte) error) error {
	leftVal, err := execSingle(node.left, input, buf)
	if err != nil {
		return err
	}
	rightBuf := leftVal[len(leftVal):len(leftVal):cap(leftVal)]
	rightVal, err := execSingle(node.right, input, rightBuf)
	if err != nil {
		return err
	}
	if evalCmpOp(node.cmpOp, leftVal, rightVal) {
		return fn(append(buf[:0], "true"...))
	}
	return fn(append(buf[:0], "false"...))
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

/// execLength returns the length of a JSON value:
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
			buf = append(buf, lc...)
			buf = append(buf, rc...)
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
	raw := s.readString()                    // raw bytes between quotes
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
//   last=true, all=false  → rindex: last occurrence
//   last=false, all=false → index:  first occurrence, null if not found
//   last=false, all=true  → indices: all occurrences as array
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
		content := ss.readString()      // raw bytes of input string content
		needle := sv.readString()       // raw bytes of search string content

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

	found := false   // for any: true if a match was found
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
//               (null, false) // 18 → 18 (no truthy outputs, use right).
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
			if numF < 0 {
				// negative: null
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

// compareJSONOrder returns -1, 0, or +1 for ordering two raw JSON values.
// Numbers: float comparison. Strings: lexicographic byte order.
// Cross-type ordering: number < string < array < object < boolean < null.
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
	}
	return 0
}

// jsonTypeOrderVal maps the first byte of a JSON value to a sort order integer.
// number < string < array < object < boolean < null
func jsonTypeOrderVal(b byte) int {
	switch b {
	case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		return 0 // number
	case '"':
		return 1 // string
	case '[':
		return 2 // array
	case '{':
		return 3 // object
	case 't', 'f':
		return 4 // boolean
	default:
		return 5 // null
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
	// Generic error
	return fmt.Errorf("Invalid numeric literal at EOF at line 1, column %d (while parsing '%s')", col, content)
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

// execURIEncode percent-encodes a JSON string per RFC 3986 unreserved characters.
// The JSON string is first decoded (escape sequences resolved) so that e.g.
// "\u03bc" encodes as %CE%BC (the UTF-8 bytes for μ) rather than %5Cu03bc.
func execURIEncode(input []byte, buf []byte) ([]byte, error) {
	s := scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '"' {
		return nil, fmt.Errorf("@uri input must be a string")
	}
	raw := s.readString()                    // raw bytes between JSON quotes
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
	roundFloor   roundMode = iota
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
	mathSqrt      mathFuncType = iota
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
// Internal single quotes are escaped as '\'' (end-quote, backslash-quote, reopen-quote).
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
