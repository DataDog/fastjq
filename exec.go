package fastjq

import (
	"errors"
	"fmt"
)

var (
	errExpectedObjectField = errors.New("expected object for field access")
	errExpectedArrayIndex  = errors.New("expected array for index access")
	errExpectedIterable    = errors.New("expected array or object for .[]")
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
			return append(buf[:0], "false"...), nil
		}
		rightVal, err := execSingle(node.right, input, buf)
		if err != nil {
			return nil, err
		}
		if isFalsy(rightVal) {
			return append(buf[:0], "false"...), nil
		}
		return append(buf[:0], "true"...), nil
	case opOr:
		leftVal, err := execSingle(node.left, input, buf)
		if err != nil {
			return nil, err
		}
		if !isFalsy(leftVal) {
			return append(buf[:0], "true"...), nil
		}
		rightVal, err := execSingle(node.right, input, buf)
		if err != nil {
			return nil, err
		}
		if !isFalsy(rightVal) {
			return append(buf[:0], "true"...), nil
		}
		return append(buf[:0], "false"...), nil
	case opNot:
		if isFalsy(input) {
			return append(buf, "true"...), nil
		}
		return append(buf, "false"...), nil
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
func execIdentity(input []byte, buf []byte) ([]byte, error) {
	s := &scanner{data: input}
	s.skipWhitespace()
	start := s.pos
	s.skipValue()
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
func execArrayConstruct(node *op, input []byte, buf []byte) ([]byte, error) {
	buf = append(buf, '[')
	for i, elem := range node.elems {
		if i > 0 {
			buf = append(buf, ',')
		}
		val, err := exec(elem, input, buf[len(buf):len(buf):cap(buf)])
		if err != nil {
			return nil, fmt.Errorf("in array construction: %w", err)
		}
		buf = append(buf, val...)
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
	rightBuf := leftVal[len(leftVal):len(leftVal):cap(leftVal)]
	rightVal, err := execSingle(node.right, input, rightBuf)
	if err != nil {
		return nil, err
	}
	if evalCmpOp(node.cmpOp, leftVal, rightVal) {
		return append(buf[:0], "true"...), nil
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
