package fastjq

// scanner is a zero-allocation JSON scanner that operates on raw bytes.
// It never copies data — all string reads return sub-slices of the input.
type scanner struct {
	data []byte
	pos  int
}

// skipWhitespace advances past spaces, tabs, newlines, carriage returns.
func (s *scanner) skipWhitespace() {
	for s.pos < len(s.data) {
		switch s.data[s.pos] {
		case ' ', '\t', '\n', '\r':
			s.pos++
		default:
			return
		}
	}
}

// readString reads a JSON string and returns the raw content between quotes
// as a sub-slice (zero allocation). Advances pos past the closing quote.
// Assumes pos is at the opening '"'.
func (s *scanner) readString() []byte {
	s.pos++ // skip opening '"'
	start := s.pos
	for s.pos < len(s.data) {
		ch := s.data[s.pos]
		if ch == '\\' {
			s.pos += 2 // skip escaped char
			continue
		}
		if ch == '"' {
			result := s.data[start:s.pos]
			s.pos++ // skip closing '"'
			return result
		}
		s.pos++
	}
	return s.data[start:s.pos]
}

// skipValue skips a complete JSON value (object, array, string, number, bool, null).
// Uses depth-counting for objects/arrays — no recursion needed.
func (s *scanner) skipValue() {
	s.skipWhitespace()
	if s.pos >= len(s.data) {
		return
	}
	ch := s.data[s.pos]
	switch ch {
	case '"':
		s.skipString()
	case '{', '[':
		s.skipContainer()
	default:
		// number, bool, null — scan until delimiter
		s.skipPrimitive()
	}
}

// skipString skips a JSON string including its quotes.
func (s *scanner) skipString() {
	s.pos++ // skip opening '"'
	for s.pos < len(s.data) {
		ch := s.data[s.pos]
		if ch == '\\' {
			s.pos += 2
			continue
		}
		if ch == '"' {
			s.pos++ // skip closing '"'
			return
		}
		s.pos++
	}
}

// skipContainer skips a JSON object or array using depth counting.
func (s *scanner) skipContainer() {
	depth := 1
	s.pos++ // skip opening '{' or '['
	for s.pos < len(s.data) && depth > 0 {
		ch := s.data[s.pos]
		switch ch {
		case '"':
			s.skipString()
			continue
		case '{', '[':
			depth++
		case '}', ']':
			depth--
		}
		s.pos++
	}
}

// skipPrimitive skips a JSON primitive (number, bool, null).
func (s *scanner) skipPrimitive() {
	for s.pos < len(s.data) {
		switch s.data[s.pos] {
		case ' ', '\t', '\n', '\r', ',', '}', ']':
			return
		}
		s.pos++
	}
}

// objectIter iterates over key-value pairs of a JSON object.
// Assumes pos is at the opening '{'.
// Calls fn with each key (unquoted content) and the start/end positions of the value.
// If fn returns false, iteration stops early.
func (s *scanner) objectIter(fn func(key []byte, valueStart, valueEnd int) bool) {
	s.pos++ // skip '{'
	s.skipWhitespace()
	if s.pos < len(s.data) && s.data[s.pos] == '}' {
		s.pos++ // empty object
		return
	}
	for s.pos < len(s.data) {
		s.skipWhitespace()
		// read key
		if s.pos >= len(s.data) || s.data[s.pos] != '"' {
			return
		}
		key := s.readString()
		s.skipWhitespace()
		// skip ':'
		if s.pos < len(s.data) && s.data[s.pos] == ':' {
			s.pos++
		}
		s.skipWhitespace()
		// record value boundaries
		valueStart := s.pos
		s.skipValue()
		valueEnd := s.pos

		if !fn(key, valueStart, valueEnd) {
			// caller asked to stop — we still need to skip to end of object
			s.skipToEndOfObject()
			return
		}

		s.skipWhitespace()
		if s.pos < len(s.data) && s.data[s.pos] == ',' {
			s.pos++
		} else {
			break
		}
	}
	s.skipWhitespace()
	if s.pos < len(s.data) && s.data[s.pos] == '}' {
		s.pos++
	}
}

// skipToEndOfObject skips to the closing '}' of the current object,
// accounting for nesting. Used after early-exit from objectIter.
func (s *scanner) skipToEndOfObject() {
	depth := 1
	for s.pos < len(s.data) && depth > 0 {
		ch := s.data[s.pos]
		switch ch {
		case '"':
			s.skipString()
			continue
		case '{', '[':
			depth++
		case '}', ']':
			depth--
		}
		s.pos++
	}
}

// arrayIter iterates over elements of a JSON array.
// Assumes pos is at the opening '['.
// Calls fn with each element's index and start/end positions.
// If fn returns false, iteration stops early.
func (s *scanner) arrayIter(fn func(index int, elemStart, elemEnd int) bool) {
	s.pos++ // skip '['
	s.skipWhitespace()
	if s.pos < len(s.data) && s.data[s.pos] == ']' {
		s.pos++ // empty array
		return
	}
	idx := 0
	for s.pos < len(s.data) {
		s.skipWhitespace()
		elemStart := s.pos
		s.skipValue()
		elemEnd := s.pos

		if !fn(idx, elemStart, elemEnd) {
			// caller asked to stop — skip to end of array
			s.skipToEndOfArray()
			return
		}

		idx++
		s.skipWhitespace()
		if s.pos < len(s.data) && s.data[s.pos] == ',' {
			s.pos++
		} else {
			break
		}
	}
	s.skipWhitespace()
	if s.pos < len(s.data) && s.data[s.pos] == ']' {
		s.pos++
	}
}

// skipToEndOfArray skips to the closing ']' of the current array,
// accounting for nesting. Used after early-exit from arrayIter.
func (s *scanner) skipToEndOfArray() {
	depth := 1
	for s.pos < len(s.data) && depth > 0 {
		ch := s.data[s.pos]
		switch ch {
		case '"':
			s.skipString()
			continue
		case '{', '[':
			depth++
		case '}', ']':
			depth--
		}
		s.pos++
	}
}

// arrayLen counts the number of elements in a JSON array without copying.
// Assumes pos is at the opening '['. Resets pos after counting.
func (s *scanner) arrayLen() int {
	saved := s.pos
	count := 0
	s.arrayIter(func(index int, elemStart, elemEnd int) bool {
		count++
		return true
	})
	s.pos = saved
	return count
}

// findField scans an object for a specific field name and returns
// the start and end positions of its value. Returns -1, -1 if not found.
// Assumes pos is at the opening '{'.
func (s *scanner) findField(name []byte) (valueStart, valueEnd int) {
	valueStart, valueEnd = -1, -1
	s.objectIter(func(key []byte, vs, ve int) bool {
		if bytesEqual(key, name) {
			valueStart, valueEnd = vs, ve
			return false // found it, stop
		}
		return true
	})
	return
}

// bytesEqual compares two byte slices for equality without allocation.
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// bytesEqualStr compares a byte slice to a string without allocation.
func bytesEqualStr(a []byte, s string) bool {
	if len(a) != len(s) {
		return false
	}
	for i := range a {
		if a[i] != s[i] {
			return false
		}
	}
	return true
}
