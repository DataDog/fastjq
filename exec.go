package fastjq

import "fmt"

// exec executes an op against input, writing the result into buf.
// Returns the result as a sub-slice of buf.
func exec(node *op, input []byte, buf []byte) ([]byte, error) {
	switch node.typ {
	case opIdentity:
		return execIdentity(input, buf)
	case opField:
		return execField(node, input, buf)
	case opDelete:
		return execDelete(node, input, buf)
	case opPipe:
		return execPipe(node, input, buf)
	default:
		return nil, fmt.Errorf("unknown op type: %d", node.typ)
	}
}

// execIdentity copies the input to the output buffer (trimmed of whitespace).
func execIdentity(input []byte, buf []byte) ([]byte, error) {
	s := &scanner{data: input}
	s.skipWhitespace()
	start := s.pos
	s.skipValue()
	return append(buf, input[start:s.pos]...), nil
}

// execField extracts a field value from a JSON object, supporting nested access.
func execField(node *op, input []byte, buf []byte) ([]byte, error) {
	s := &scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '{' {
		return nil, fmt.Errorf("expected object for field access .%s", node.field)
	}

	fieldName := []byte(node.field)
	vs, ve := s.findField(fieldName)
	if vs == -1 {
		return append(buf, "null"...), nil
	}

	value := input[vs:ve]

	// If there's a chained child field, recurse into the value
	if node.child != nil {
		return execField(node.child, value, buf)
	}

	return append(buf, value...), nil
}

// execDelete removes specified fields from a JSON object.
// Never copies commas from input — reconstructs with our own commas.
func execDelete(node *op, input []byte, buf []byte) ([]byte, error) {
	s := &scanner{data: input}
	s.skipWhitespace()
	if s.pos >= len(s.data) || s.data[s.pos] != '{' {
		return nil, fmt.Errorf("expected object for del()")
	}

	// Build a set of top-level field names to delete, and track nested deletions
	// Validate all del() arguments are field accesses
	for i := range node.fields {
		if node.fields[i].typ != opField {
			return nil, fmt.Errorf("del() argument must be a field access")
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

// execPipe runs left, then feeds the result into right.
func execPipe(node *op, input []byte, buf []byte) ([]byte, error) {
	// Execute left side into a temporary buffer
	intermediate := make([]byte, 0, len(input))
	var err error
	intermediate, err = exec(node.left, input, intermediate)
	if err != nil {
		return nil, fmt.Errorf("pipe left: %w", err)
	}

	// Execute right side with the intermediate result
	return exec(node.right, intermediate, buf)
}
