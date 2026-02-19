package fastjq

import (
	"errors"
	"fmt"
)

var (
	errExpectedObjectField = errors.New("expected object for field access")
	errExpectedArrayIndex  = errors.New("expected array for index access")
	errExpectedIterable    = errors.New("expected array or object for .[]")
	errBreak               = errors.New("stop iteration") // sentinel for first/limit
)

// bTrue / bFalse are package-level literals returned directly when buf == nil,
// avoiding heap allocation for boolean results in zero-scratch evaluation paths.
var bTrue = []byte("true")
var bFalse = []byte("false")

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
	case opWithEntries:
		return execWithEntries(node, input, buf, fn)
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
		return append(buf, "null"...), nil
	}

	value := input[vs:ve]

	if node.child != nil {
		return exec(node.child, value, buf)
	}
	// When buf is nil, return a cap-limited sub-slice directly (zero-alloc).
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
		s.arrayIter(func(index int, elemStart, elemEnd int) bool {
			if err := fn(input[elemStart:elemEnd]); err != nil {
				return false
			}
			return true
		})
		return nil
	case '{':
		s.objectIter(func(key []byte, valueStart, valueEnd int) bool {
			if err := fn(input[valueStart:valueEnd]); err != nil {
				return false
			}
			return true
		})
		return nil
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
				// Nested delete — recurse into the value
				if !first {
					buf = append(buf, ',')
				}
				first = false
				buf = append(buf, '"')
				buf = append(buf, key...)
				buf = append(buf, '"', ':')
				nestedNode := &op{typ: opDelete, fields: []op{*d.child}}
				var err error
				buf, err = execDelete(nestedNode, input[valueStart:valueEnd], buf)
				if err != nil {
					buf = append(buf, input[valueStart:valueEnd]...)
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
		if d.typ != opIndex {
			return nil, fmt.Errorf("del() argument must be an index access for array input")
		}
		idx := d.index
		if idx < 0 {
			idx = length + idx
		}
		if idx >= 0 && idx < length {
			deleteSet[idx] = true
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
func execArrayConstruct(node *op, input []byte, buf []byte) ([]byte, error) {
	buf = append(buf, '[')
	first := true
	for _, elem := range node.elems {
		// nil scratch avoids aliasing when multiple outputs are collected
		// into buf within a single execMulti call.
		err := execMulti(elem, input, nil, func(val []byte) error {
			if !first {
				buf = append(buf, ',')
			}
			first = false
			buf = append(buf, val...)
			return nil
		})
		if err != nil {
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
		// String length: count bytes between quotes (not Unicode codepoints,
		// but consistent with how jq counts — bytes of the unquoted content).
		s.pos++ // skip opening quote
		n := 0
		for s.pos < len(s.data) {
			ch := s.data[s.pos]
			if ch == '\\' {
				s.pos += 2
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
		return appendInt(buf, 0), nil
	}
}

// appendInt appends a non-negative integer to buf without allocation.
func appendInt(buf []byte, n int) []byte {
	if n == 0 {
		return append(buf, '0')
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

// appendKV appends a {"key":"k","value":v} entry to buf. Used by execToEntries and execWithEntries.
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

// parseEntryKeyValue scans a single to_entries-style object and extracts
// the unquoted key string and the value bounds. Called from execFromEntries.
// Implemented as a standalone (non-closure) function so the scanner variables
// stay on the stack and do not escape to the heap.
func parseEntryKeyValue(elem []byte) (keyContent []byte, valStart, valEnd int) {
	valStart, valEnd = -1, -1
	s := scanner{data: elem}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '{' {
		return
	}
	s.pos++ // skip '{'
	s.skipWhitespace()
	if s.pos < len(s.data) && s.data[s.pos] == '}' {
		return // empty object
	}
	for s.pos < len(s.data) {
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

		if bytesEqualStr(k, "key") || bytesEqualStr(k, "name") {
			inner := scanner{data: elem[vs:ve]}
			inner.skipWhitespace()
			if inner.pos < len(inner.data) && inner.data[inner.pos] == '"' {
				keyContent = inner.readString()
			}
		} else if bytesEqualStr(k, "value") {
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

// execWithEntries applies f to each {key, value} entry of the input object
// and reconstructs an object from the results.
//
// Uses exec() (no caller-supplied closure) to apply f, which allows Go to
// stack-allocate exec's internal result-capture closure. The object iteration
// is inlined. entryBuf is pre-allocated to avoid growth allocs.
func execWithEntries(node *op, input []byte, buf []byte, fn func([]byte) error) error {
	s := scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '{' {
		return fn(append(buf, "{}"...))
	}

	buf = append(buf, '{')
	first := true
	// 64-byte initial capacity covers typical log record entries without reallocation.
	// The allocator recycles this in steady state → 0 allocs/op in production use.
	// For pathological inputs with very large field values (e.g. 100KB+ objects with
	// 200-char nested values), 1-2 allocs may occur from buffer growth on first call.
	entryBuf := make([]byte, 0, 64)

	s.pos++ // skip '{'
	s.skipWhitespace()
	for s.pos < len(s.data) && s.data[s.pos] != '}' {
		s.skipWhitespace()
		if s.pos >= len(s.data) || s.data[s.pos] != '"' {
			break
		}
		key := s.readString()
		s.skipWhitespace()
		if s.pos < len(s.data) && s.data[s.pos] == ':' {
			s.pos++
		}
		s.skipWhitespace()
		vS := s.pos
		s.skipValue()
		vE := s.pos

		entryBuf = entryBuf[:0]
		entryBuf = appendKV(entryBuf, key, input[vS:vE])

		// exec returns nil when f produces no output (e.g. select dropped it).
		// parseEntryKeyValue returns nil for nil/non-entry results — those are skipped.
		result, err := exec(node.child, entryBuf, nil)
		if err != nil {
			return err
		}
		kc, vs, ve := parseEntryKeyValue(result)
		if kc != nil && vs != -1 {
			if !first {
				buf = append(buf, ',')
			}
			first = false
			buf = append(buf, '"')
			buf = append(buf, kc...)
			buf = append(buf, '"', ':')
			buf = append(buf, result[vs:ve]...)
		}

		s.skipWhitespace()
		if s.pos < len(s.data) && s.data[s.pos] == ',' {
			s.pos++
		} else {
			break
		}
	}

	buf = append(buf, '}')
	return fn(buf)
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
	if n <= 0 {
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

// execHas checks whether the input object contains a field.
// Returns true if the field exists (even if its value is null), false otherwise.
func execHas(node *op, input []byte, buf []byte, fn func([]byte) error) error {
	s := &scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '{' {
		return fn(append(buf, "false"...))
	}
	key := []byte(node.field)
	vs, _ := s.findField(key)
	if vs == -1 {
		return fn(append(buf, "false"...))
	}
	return fn(append(buf, "true"...))
}

// execIf evaluates cond; if truthy runs the then-branch, otherwise the else-branch.
// If no else-branch is present (child==nil), the else defaults to identity.
func execIf(node *op, input []byte, buf []byte, fn func([]byte) error) error {
	condVal, err := execSingle(node.left, input, buf)
	if err != nil {
		return err
	}
	if !isFalsy(condVal) {
		return execMulti(node.right, input, buf, fn)
	}
	if node.child != nil {
		return execMulti(node.child, input, buf, fn)
	}
	// default else: identity
	return fn(input)
}

// execAlternative tries left; if result is falsy, evaluates right instead.
func execAlternative(node *op, input []byte, buf []byte, fn func([]byte) error) error {
	// Fast path for single-result left expressions (common case)
	result, err := execSingle(node.left, input, buf)
	if err != nil {
		return err
	}
	if !isFalsy(result) {
		return fn(result)
	}
	// Left was falsy — evaluate right
	return execMulti(node.right, input, buf, fn)
}
