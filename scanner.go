package fastjq

import (
	"bytes"
	"errors"
)

// Pre-allocated sentinel errors — zero allocation even on invalid input.
var (
	errUnterminatedString   = errors.New("unterminated string")
	errInvalidEscape        = errors.New("invalid escape sequence in string")
	errInvalidControlChar   = errors.New("unescaped control character in string")
	errInvalidUnicodeEscape = errors.New("invalid \\uXXXX escape in string")
	errInvalidKeyword       = errors.New("invalid keyword (expected true/false/null)")
	errInvalidNumber        = errors.New("invalid number")
	errMismatchedBracket    = errors.New("mismatched bracket")
	errInvalidValueStart    = errors.New("invalid character at start of value")
	errUnterminatedContainer = errors.New("unterminated object or array")
	errInvalidJSON          = errors.New("invalid JSON")
	errTrailingContent      = errors.New("trailing content after JSON value")
)

// scanner is a zero-allocation JSON scanner that operates on raw bytes.
// It never copies data — all string reads return sub-slices of the input.
// When invalid JSON is encountered, err is set and all methods short-circuit.
// On valid JSON, err stays nil — zero allocations.
type scanner struct {
	data []byte
	pos  int
	err  error
}

// setErr records the first validation error. Subsequent errors are ignored.
func (s *scanner) setErr(e error) {
	if s.err == nil {
		s.err = e
	}
}

// skipWhitespace advances past spaces, tabs, newlines, carriage returns.
func (s *scanner) skipWhitespace() {
	// Fast path: all JSON whitespace (space, tab, LF, CR) is <= 0x20,
	// while all structural/value characters are > 0x20.
	// In compact JSON (the common case), this returns immediately.
	if s.pos < len(s.data) && s.data[s.pos] > ' ' {
		return
	}
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
// Uses byte-by-byte scanning — readString is called on field keys which are
// typically short (3-20 bytes), where function call overhead of bytes.IndexByte
// outweighs its SIMD benefit. skipString handles long values separately.
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
	s.setErr(errUnterminatedString)
	return s.data[start:s.pos]
}

// skipValue skips a complete JSON value (object, array, string, number, bool, null).
// Uses depth-counting for objects/arrays — no recursion needed.
// Validates value start byte and delegates to type-specific skipping.
func (s *scanner) skipValue() {
	if s.err != nil {
		return
	}
	s.skipWhitespace()
	if s.pos >= len(s.data) {
		return
	}
	switch s.data[s.pos] {
	case '"':
		s.skipString()
	case '{', '[':
		s.skipContainer()
	case 't', 'f', 'n', '-',
		'0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		s.skipPrimitive()
	default:
		s.setErr(errInvalidValueStart)
	}
}

// skipString skips a JSON string including its quotes.
// Short strings (<=16 bytes) are scanned byte-by-byte to avoid function call
// overhead. Longer strings use bytes.IndexByte for SIMD-accelerated scanning.
func (s *scanner) skipString() {
	s.pos++ // skip opening '"'
	// Fast path: scan short strings byte-by-byte (common for field keys/short values)
	limit := s.pos + 16
	if limit > len(s.data) {
		limit = len(s.data)
	}
	for s.pos < limit {
		ch := s.data[s.pos]
		if ch == '\\' {
			s.pos += 2
			continue
		}
		if ch == '"' {
			s.pos++
			return
		}
		s.pos++
	}
	// SIMD path: use bytes.IndexByte for remaining long strings (x86/ARM64 accelerated)
	for s.pos < len(s.data) {
		rest := s.data[s.pos:]
		qi := bytes.IndexByte(rest, '"')
		if qi == -1 {
			break
		}
		bi := bytes.IndexByte(rest[:qi], '\\')
		if bi == -1 {
			s.pos += qi + 1
			return
		}
		s.pos += bi + 2
	}
	// Reached EOF without finding closing '"'
	s.pos = len(s.data)
	s.setErr(errUnterminatedString)
}

// skipContainer skips a JSON object or array using depth counting.
func (s *scanner) skipContainer() {
	if s.err != nil {
		return
	}
	open := s.data[s.pos]
	depth := 1
	s.pos++ // skip opening '{' or '['
	for s.pos < len(s.data) && depth > 0 {
		ch := s.data[s.pos]
		switch ch {
		case '"':
			s.skipString()
			if s.err != nil {
				return
			}
			continue
		case '{', '[':
			depth++
		case '}':
			depth--
			if depth == 0 && open != '{' {
				s.setErr(errMismatchedBracket)
				return
			}
		case ']':
			depth--
			if depth == 0 && open != '[' {
				s.setErr(errMismatchedBracket)
				return
			}
		}
		s.pos++
	}
	if depth > 0 {
		s.setErr(errUnterminatedContainer)
	}
}

// skipPrimitive skips a JSON primitive (number, bool, null) by scanning to
// the next delimiter. Keyword and number format validation is performed only
// by the dedicated Validate() path (validateValue).
func (s *scanner) skipPrimitive() {
	for s.pos < len(s.data) {
		switch s.data[s.pos] {
		case ' ', '\t', '\n', '\r', ',', '}', ']':
			return
		}
		s.pos++
	}
}

// skipKeyword validates and skips an exact JSON keyword (true, false, null).
// Direct byte comparison — faster than the old scan-to-delimiter loop for
// these fixed-length tokens.
func (s *scanner) skipKeyword(kw string) {
	if s.pos+len(kw) > len(s.data) {
		s.setErr(errInvalidKeyword)
		s.pos = len(s.data)
		return
	}
	for i := 0; i < len(kw); i++ {
		if s.data[s.pos+i] != kw[i] {
			s.setErr(errInvalidKeyword)
			s.pos += i
			return
		}
	}
	s.pos += len(kw)
}

// skipNumber validates and skips a JSON number.
// Validates: no leading zeros, valid fraction/exponent format.
func (s *scanner) skipNumber() {
	// Optional minus
	if s.data[s.pos] == '-' {
		s.pos++
		if s.pos >= len(s.data) || s.data[s.pos] < '0' || s.data[s.pos] > '9' {
			s.setErr(errInvalidNumber)
			return
		}
	}
	// Integer part: '0' must not be followed by another digit (no leading zeros)
	if s.data[s.pos] == '0' {
		s.pos++
		if s.pos < len(s.data) && s.data[s.pos] >= '0' && s.data[s.pos] <= '9' {
			s.setErr(errInvalidNumber)
			return
		}
	} else {
		s.skipDigits()
	}
	// Fraction
	if s.pos < len(s.data) && s.data[s.pos] == '.' {
		s.pos++
		if s.pos >= len(s.data) || s.data[s.pos] < '0' || s.data[s.pos] > '9' {
			s.setErr(errInvalidNumber)
			return
		}
		s.skipDigits()
	}
	// Exponent
	if s.pos < len(s.data) && (s.data[s.pos] == 'e' || s.data[s.pos] == 'E') {
		s.pos++
		if s.pos < len(s.data) && (s.data[s.pos] == '+' || s.data[s.pos] == '-') {
			s.pos++
		}
		if s.pos >= len(s.data) || s.data[s.pos] < '0' || s.data[s.pos] > '9' {
			s.setErr(errInvalidNumber)
			return
		}
		s.skipDigits()
	}
}

// skipDigits advances past a run of ASCII digits.
func (s *scanner) skipDigits() {
	for s.pos < len(s.data) && s.data[s.pos] >= '0' && s.data[s.pos] <= '9' {
		s.pos++
	}
}

// objectIter iterates over key-value pairs of a JSON object.
// Assumes pos is at the opening '{'.
// Calls fn with each key (unquoted content) and the start/end positions of the value.
// If fn returns false, iteration stops early.
func (s *scanner) objectIter(fn func(key []byte, valueStart, valueEnd int) bool) {
	if s.err != nil {
		return
	}
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
			s.setErr(errInvalidJSON)
			return
		}
		key := s.readString()
		if s.err != nil {
			return
		}
		s.skipWhitespace()
		// skip ':'
		if s.pos >= len(s.data) || s.data[s.pos] != ':' {
			s.setErr(errInvalidJSON)
			return
		}
		s.pos++
		s.skipWhitespace()
		// record value boundaries
		valueStart := s.pos
		s.skipValue()
		if s.err != nil {
			return
		}
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
	} else if s.err == nil {
		s.setErr(errUnterminatedContainer)
	}
}

// skipToEndOfObject skips to the closing '}' of the current object,
// accounting for nesting. Used after early-exit from objectIter.
func (s *scanner) skipToEndOfObject() {
	if s.err != nil {
		return
	}
	depth := 1
	for s.pos < len(s.data) && depth > 0 {
		ch := s.data[s.pos]
		switch ch {
		case '"':
			s.skipString()
			if s.err != nil {
				return
			}
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
	if s.err != nil {
		return
	}
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
		if s.err != nil {
			return
		}
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
	} else if s.err == nil {
		s.setErr(errUnterminatedContainer)
	}
}

// skipToEndOfArray skips to the closing ']' of the current array,
// accounting for nesting. Used after early-exit from arrayIter.
func (s *scanner) skipToEndOfArray() {
	if s.err != nil {
		return
	}
	depth := 1
	for s.pos < len(s.data) && depth > 0 {
		ch := s.data[s.pos]
		switch ch {
		case '"':
			s.skipString()
			if s.err != nil {
				return
			}
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
// Stops scanning immediately when the field is found — does NOT scan the
// rest of the object. When the scanner position after the call doesn't matter,
// this avoids wasted work proportional to the remaining fields.
func (s *scanner) findField(name []byte) (valueStart, valueEnd int) {
	if s.err != nil {
		return -1, -1
	}
	if s.pos >= len(s.data) || s.data[s.pos] != '{' {
		return -1, -1
	}
	s.pos++ // skip '{'
	for s.pos < len(s.data) {
		s.skipWhitespace()
		if s.pos >= len(s.data) || s.data[s.pos] != '"' {
			return -1, -1
		}
		key := s.readString()
		if s.err != nil {
			return -1, -1
		}
		s.skipWhitespace()
		if s.pos < len(s.data) && s.data[s.pos] == ':' {
			s.pos++
		}
		s.skipWhitespace()
		vs := s.pos
		s.skipValue()
		if s.err != nil {
			return -1, -1
		}
		ve := s.pos

		if bytesEqual(key, name) {
			return vs, ve // found — stop here, no need to scan the rest
		}

		s.skipWhitespace()
		if s.pos < len(s.data) && s.data[s.pos] == ',' {
			s.pos++
		} else {
			break
		}
	}
	return -1, -1
}

// findFieldStr is like findField but takes a string key directly, avoiding the
// []byte(string) conversion at each call site. Crucially, it also stops scanning
// as soon as the field is found — it does NOT call skipToEndOfObject, saving the
// cost of scanning all remaining fields in the object.
// Assumes pos is at the opening '{'.
func (s *scanner) findFieldStr(name string) (valueStart, valueEnd int) {
	if s.err != nil {
		return -1, -1
	}
	if s.pos >= len(s.data) || s.data[s.pos] != '{' {
		return -1, -1
	}
	s.pos++ // skip '{'
	for s.pos < len(s.data) {
		s.skipWhitespace()
		if s.pos >= len(s.data) || s.data[s.pos] != '"' {
			return -1, -1
		}
		key := s.readString()
		if s.err != nil {
			return -1, -1
		}
		s.skipWhitespace()
		if s.pos < len(s.data) && s.data[s.pos] == ':' {
			s.pos++
		}
		s.skipWhitespace()
		vs := s.pos
		s.skipValue()
		if s.err != nil {
			return -1, -1
		}
		ve := s.pos

		if bytesEqualStr(key, name) {
			return vs, ve // found — stop here, skip scanning the rest of the object
		}

		s.skipWhitespace()
		if s.pos < len(s.data) && s.data[s.pos] == ',' {
			s.pos++
		} else {
			break
		}
	}
	return -1, -1
}

// bytesEqual compares two byte slices for equality using the runtime's
// assembly-optimized memequal (SIMD on x86/ARM64).
func bytesEqual(a, b []byte) bool {
	return bytes.Equal(a, b)
}

// bytesEqualStr compares a byte slice to a string without allocation.
// Go compiler optimizes string(a) == s to use runtime memequal directly
// when the result is used only in a comparison (no heap allocation).
func bytesEqualStr(a []byte, s string) bool {
	return string(a) == s
}

// isFalsy returns true if the JSON value is null or false.
func isFalsy(v []byte) bool {
	if len(v) == 0 {
		return true // empty = falsy
	}
	// Fast path: first byte is not whitespace (common in compact JSON).
	b := v[0]
	if b > ' ' {
		return b == 'n' || b == 'f'
	}
	// Slow path: skip leading whitespace.
	for i := 0; i < len(v); i++ {
		switch v[i] {
		case ' ', '\t', '\n', '\r':
			continue
		case 'n': // null
			return true
		case 'f': // false
			return true
		default:
			return false
		}
	}
	return true // all whitespace = falsy
}

// jsonEqual compares two raw JSON values for equality.
func jsonEqual(a, b []byte) bool {
	a = trimWhitespace(a)
	b = trimWhitespace(b)
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) == 0 || len(b) == 0 {
		return false
	}

	// Fast path: identical bytes
	if bytesEqual(a, b) {
		return true
	}

	aCh := a[0]
	bCh := b[0]

	// Strings: compare unquoted content
	if aCh == '"' && bCh == '"' {
		sa := &scanner{data: a}
		aContent := sa.readString()
		sb := &scanner{data: b}
		bContent := sb.readString()
		return bytesEqual(aContent, bContent)
	}

	// Booleans and null: first byte determines identity
	// null vs null, true vs true, false vs false
	if (aCh == 'n' || aCh == 't' || aCh == 'f') && (bCh == 'n' || bCh == 't' || bCh == 'f') {
		return aCh == bCh
	}

	// Numbers: try byte comparison of normalized form, then float fallback
	if isNumberByte(aCh) && isNumberByte(bCh) {
		return compareNumbers(a, b)
	}

	// Objects: order-independent key-value comparison.
	// Every key in a must exist in b with an equal value, and both must have
	// the same number of keys. Arrays still use byte comparison (order matters).
	if aCh == '{' && bCh == '{' {
		// Count b's keys first, then verify each of a's keys matches.
		countB := 0
		bCount := scanner{data: b}
		bCount.objectIter(func(_ []byte, _, _ int) bool { countB++; return true })

		countA := 0
		equal := true
		sa2 := scanner{data: a}
		sa2.objectIter(func(key []byte, vStart, vEnd int) bool {
			countA++
			sb2 := scanner{data: b}
			sb2.skipWhitespace()
			bvStart, bvEnd := sb2.findField(key)
			if bvStart == -1 || !jsonEqual(a[vStart:vEnd], b[bvStart:bvEnd]) {
				equal = false
				return false
			}
			return true
		})
		return equal && countA == countB
	}

	// Arrays and other types: byte-for-byte only (already tried above)
	return false
}

// compareNumbers compares two JSON number byte sequences.
func compareNumbers(a, b []byte) bool {
	// Fast path: byte-identical
	if bytesEqual(a, b) {
		return true
	}
	// Slow path: parse as float
	af, aOk := parseJSONFloat(a)
	bf, bOk := parseJSONFloat(b)
	if aOk && bOk {
		return af == bf
	}
	return false
}

// parseJSONFloat parses a JSON number from raw bytes.
// Uses integer fast path (no alloc for simple integers).
func parseJSONFloat(b []byte) (float64, bool) {
	if len(b) == 0 {
		return 0, false
	}

	// Integer fast path: optional minus, then all digits
	neg := false
	start := 0
	if b[0] == '-' {
		neg = true
		start = 1
	}
	isInt := true
	for i := start; i < len(b); i++ {
		if b[i] < '0' || b[i] > '9' {
			isInt = false
			break
		}
	}
	if isInt && start < len(b) {
		var n int64
		for i := start; i < len(b); i++ {
			n = n*10 + int64(b[i]-'0')
		}
		if neg {
			n = -n
		}
		return float64(n), true
	}

	// Slow path: use strconv via unsafe.String to avoid allocation
	f, err := parseFloatUnsafe(b)
	if err != nil {
		return 0, false
	}
	return f, true
}

func isNumberByte(ch byte) bool {
	return (ch >= '0' && ch <= '9') || ch == '-'
}

// isHexDigit reports whether ch is a valid hexadecimal digit.
func isHexDigit(ch byte) bool {
	return (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
}

// jsonCompare compares two raw JSON values for ordering.
// Returns (-1, true) if a < b, (0, true) if equal, (1, true) if a > b.
// Returns (0, false) if the types are incompatible or not orderable.
// Supports numbers and strings only.
func jsonCompare(a, b []byte) (int, bool) {
	a = trimWhitespace(a)
	b = trimWhitespace(b)
	if len(a) == 0 || len(b) == 0 {
		return 0, false
	}

	aCh, bCh := a[0], b[0]

	if isNumberByte(aCh) && isNumberByte(bCh) {
		af, aOk := parseJSONFloat(a)
		bf, bOk := parseJSONFloat(b)
		if !aOk || !bOk {
			return 0, false
		}
		if af < bf {
			return -1, true
		}
		if af > bf {
			return 1, true
		}
		return 0, true
	}

	if aCh == '"' && bCh == '"' {
		sa := &scanner{data: a}
		aContent := sa.readString()
		sb := &scanner{data: b}
		bContent := sb.readString()
		return bytesCompare(aContent, bContent), true
	}

	return 0, false
}

// bytesCompare compares two byte slices lexicographically.
// Returns -1, 0, or 1. Uses bytes.Compare for assembly-optimized comparison.
func bytesCompare(a, b []byte) int {
	return bytes.Compare(a, b)
}

// evalCmpOp evaluates a comparison operator against two raw JSON values.
func evalCmpOp(op cmpOperator, a, b []byte) bool {
	switch op {
	case cmpEq:
		return jsonEqual(a, b)
	case cmpNeq:
		return !jsonEqual(a, b)
	}
	cmp, ok := jsonCompare(a, b)
	if !ok {
		return false
	}
	switch op {
	case cmpLt:
		return cmp < 0
	case cmpLe:
		return cmp <= 0
	case cmpGt:
		return cmp > 0
	case cmpGe:
		return cmp >= 0
	}
	return false
}

// bytesContainBytes reports whether haystack contains needle as a contiguous byte subsequence.
func bytesContainBytes(haystack, needle []byte) bool {
	if len(needle) == 0 {
		return true
	}
	if len(haystack) < len(needle) {
		return false
	}
	for i := 0; i <= len(haystack)-len(needle); i++ {
		if bytesEqual(haystack[i:i+len(needle)], needle) {
			return true
		}
	}
	return false
}

// jsonContains reports whether haystack recursively contains needle
// using jq's containment semantics:
//   - string: haystack has needle as a raw-byte substring
//   - object: every key-value pair in needle is present in haystack (recursively)
//   - array:  every element of needle is contained in some element of haystack (recursively)
//   - other:  exact equality (jsonEqual)
func jsonContains(haystack, needle []byte) bool {
	hs := trimWhitespace(haystack)
	ns := trimWhitespace(needle)
	if len(hs) == 0 || len(ns) == 0 {
		return false
	}
	switch ns[0] {
	case '"':
		if hs[0] != '"' {
			return false
		}
		hsc := scanner{data: hs}
		nsc := scanner{data: ns}
		return bytesContainBytes(hsc.readString(), nsc.readString())
	case '{':
		if hs[0] != '{' {
			return false
		}
		ok := true
		nsc := scanner{data: ns}
		nsc.objectIter(func(nKey []byte, nValStart, nValEnd int) bool {
			hsc := scanner{data: hs}
			hValStart, hValEnd := hsc.findField(nKey)
			if hValStart == -1 {
				ok = false
				return false
			}
			if !jsonContains(hs[hValStart:hValEnd], ns[nValStart:nValEnd]) {
				ok = false
				return false
			}
			return true
		})
		return ok
	case '[':
		if hs[0] != '[' {
			return false
		}
		ok := true
		nsc := scanner{data: ns}
		nsc.arrayIter(func(_ int, nStart, nEnd int) bool {
			needleElem := ns[nStart:nEnd]
			found := false
			hsc := scanner{data: hs}
			hsc.arrayIter(func(_ int, hStart, hEnd int) bool {
				if jsonContains(hs[hStart:hEnd], needleElem) {
					found = true
					return false
				}
				return true
			})
			if !found {
				ok = false
				return false
			}
			return true
		})
		return ok
	default:
		return jsonEqual(haystack, needle)
	}
}

// byteOffsetToCodepointOffset converts a byte offset within raw JSON string
// content (as returned by readString) to the equivalent Unicode codepoint
// offset. JSON escape sequences (\n, \t, \uXXXX etc.) count as 1 codepoint
// each. Multi-byte UTF-8 sequences count as 1 codepoint.
func byteOffsetToCodepointOffset(content []byte, byteOff int) int {
	cp := 0
	i := 0
	for i < byteOff && i < len(content) {
		if content[i] == '\\' && i+1 < len(content) {
			if content[i+1] == 'u' {
				i += 6
			} else {
				i += 2
			}
		} else {
			b := content[i]
			switch {
			case b < 0x80:
				i++
			case b < 0xE0:
				i += 2
			case b < 0xF0:
				i += 3
			default:
				i += 4
			}
		}
		cp++
	}
	return cp
}

// trimWhitespace trims leading whitespace from a byte slice.
func trimWhitespace(b []byte) []byte {
	// Fast path: first byte is not whitespace (common in compact JSON).
	if len(b) > 0 && b[0] > ' ' {
		return b
	}
	for len(b) > 0 {
		switch b[0] {
		case ' ', '\t', '\n', '\r':
			b = b[1:]
		default:
			return b
		}
	}
	return b
}

// --- Validate: full RFC 8259 validation ---

// validateString validates a JSON string including escape sequences and control
// characters. Used by Validate() for full validation.
// Assumes pos is at the opening '"'.
func (s *scanner) validateString() {
	s.pos++ // skip opening '"'
	for s.pos < len(s.data) {
		ch := s.data[s.pos]
		if ch == '\\' {
			s.pos++
			if s.pos >= len(s.data) {
				s.setErr(errInvalidEscape)
				return
			}
			esc := s.data[s.pos]
			switch esc {
			case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
				s.pos++
			case 'u':
				s.pos++
				if s.pos+4 > len(s.data) {
					s.setErr(errInvalidUnicodeEscape)
					return
				}
				for i := 0; i < 4; i++ {
					if !isHexDigit(s.data[s.pos]) {
						s.setErr(errInvalidUnicodeEscape)
						return
					}
					s.pos++
				}
			default:
				s.setErr(errInvalidEscape)
				return
			}
			continue
		}
		if ch == '"' {
			s.pos++
			return
		}
		if ch < 0x20 {
			s.setErr(errInvalidControlChar)
			return
		}
		s.pos++
	}
	s.setErr(errUnterminatedString)
}

// validateValue validates a complete JSON value with full RFC 8259 checking.
// Used by Validate() for full validation.
func (s *scanner) validateValue() {
	if s.err != nil {
		return
	}
	s.skipWhitespace()
	if s.pos >= len(s.data) {
		s.setErr(errInvalidJSON)
		return
	}
	ch := s.data[s.pos]
	switch ch {
	case '"':
		s.validateString()
	case '{':
		s.validateObject()
	case '[':
		s.validateArray()
	case 't':
		s.skipKeyword("true")
	case 'f':
		s.skipKeyword("false")
	case 'n':
		s.skipKeyword("null")
	case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		s.skipNumber()
	default:
		s.setErr(errInvalidValueStart)
	}
}

// validateObject validates a JSON object with full checking.
func (s *scanner) validateObject() {
	s.pos++ // skip '{'
	s.skipWhitespace()
	if s.pos < len(s.data) && s.data[s.pos] == '}' {
		s.pos++
		return
	}
	for {
		s.skipWhitespace()
		if s.pos >= len(s.data) || s.data[s.pos] != '"' {
			s.setErr(errInvalidJSON)
			return
		}
		s.validateString()
		if s.err != nil {
			return
		}
		s.skipWhitespace()
		if s.pos >= len(s.data) || s.data[s.pos] != ':' {
			s.setErr(errInvalidJSON)
			return
		}
		s.pos++
		s.validateValue()
		if s.err != nil {
			return
		}
		s.skipWhitespace()
		if s.pos >= len(s.data) {
			s.setErr(errUnterminatedContainer)
			return
		}
		if s.data[s.pos] == '}' {
			s.pos++
			return
		}
		if s.data[s.pos] != ',' {
			s.setErr(errInvalidJSON)
			return
		}
		s.pos++
	}
}

// validateArray validates a JSON array with full checking.
func (s *scanner) validateArray() {
	s.pos++ // skip '['
	s.skipWhitespace()
	if s.pos < len(s.data) && s.data[s.pos] == ']' {
		s.pos++
		return
	}
	for {
		s.validateValue()
		if s.err != nil {
			return
		}
		s.skipWhitespace()
		if s.pos >= len(s.data) {
			s.setErr(errUnterminatedContainer)
			return
		}
		if s.data[s.pos] == ']' {
			s.pos++
			return
		}
		if s.data[s.pos] != ',' {
			s.setErr(errInvalidJSON)
			return
		}
		s.pos++
	}
}
