package fastjq

import (
	"encoding/json"
	"testing"
)

// --- Validate() tests ---

func TestValidateAcceptsValidJSON(t *testing.T) {
	valid := []string{
		// Scalars
		`null`, `true`, `false`,
		`0`, `1`, `-1`, `123`, `-456`,
		`0.5`, `-0.5`, `1.23e10`, `1.23E-5`, `1e+2`,
		`""`, `"hello"`, `"hello world"`,
		// Strings with escapes
		`"a\"b"`, `"a\\b"`, `"a\/b"`, `"a\bb"`, `"a\fb"`, `"a\nb"`, `"a\rb"`, `"a\tb"`,
		`"a\u0041b"`, `"\u0000"`, `"\uFFFF"`, `"\uabcd"`,
		// Objects
		`{}`, `{"a":1}`, `{"a":1,"b":2}`,
		`{"key":"value","nested":{"x":true}}`,
		// Arrays
		`[]`, `[1]`, `[1,2,3]`,
		`[1,"two",true,null,[],{}]`,
		// Nested
		`{"a":[1,2,{"b":"c"}]}`,
		`[{"x":1},{"y":2}]`,
		// Whitespace
		` null `, ` { "a" : 1 } `,
		"\t\n{\"a\":1}\n",
		// UTF-8 BOM
		"\xEF\xBB\xBF{\"a\":1}",
		// Large number
		`12345678901234567890`,
	}

	for _, input := range valid {
		if err := Validate([]byte(input)); err != nil {
			t.Errorf("Validate(%q) = %v, want nil", input, err)
		}
	}
}

func TestValidateRejectsInvalidJSON(t *testing.T) {
	tests := []struct {
		input string
		err   error
	}{
		// Empty/whitespace
		{"", errInvalidJSON},
		{"   ", errInvalidJSON},
		// Truncated strings
		{`"hello`, errUnterminatedString},
		{`"hello\`, errInvalidEscape},
		// Bad escapes
		{`"hello\x"`, errInvalidEscape},
		{`"hello\a"`, errInvalidEscape},
		{`"\u00"`, errInvalidUnicodeEscape},
		{`"\u00GG"`, errInvalidUnicodeEscape},
		{`"\uXXXX"`, errInvalidUnicodeEscape},
		// Control chars
		{"\"\x00\"", errInvalidControlChar},
		{"\"\x1F\"", errInvalidControlChar},
		{"\"\t\"", errInvalidControlChar}, // raw tab, not \t
		{"\"\n\"", errInvalidControlChar}, // raw newline, not \n
		// Bad keywords
		{"tru", errInvalidKeyword},
		{"fals", errInvalidKeyword},
		{"nul", errInvalidKeyword},
		{"True", errInvalidValueStart},
		{"FALSE", errInvalidValueStart},
		{"NULL", errInvalidValueStart},
		// Bad numbers
		{"01", errInvalidNumber},        // leading zero
		{"00", errInvalidNumber},        // leading zero
		{"-", errInvalidNumber},         // minus only
		{"1.", errInvalidNumber},        // trailing dot
		{"1e", errInvalidNumber},        // trailing exponent
		{"1e+", errInvalidNumber},       // incomplete exponent
		// Invalid value start
		{"@", errInvalidValueStart},
		{"!", errInvalidValueStart},
		{",", errInvalidValueStart},
		// Trailing content
		{"1 2", errTrailingContent},
		{`{} []`, errTrailingContent},
		{"null null", errTrailingContent},
		// Unterminated containers
		{`{`, errInvalidJSON},
		{`[`, errInvalidJSON},
		{`{"a":1`, errUnterminatedContainer},
		{`[1,2`, errUnterminatedContainer},
		// Missing colons/quotes
		{`{"a" 1}`, errInvalidJSON},
		{`{a:1}`, errInvalidJSON},
		// Mismatched brackets (via validateObject/validateArray)
		{`{"a":1]`, errInvalidJSON},
		{`[1}`, errInvalidJSON},
	}

	for _, tt := range tests {
		err := Validate([]byte(tt.input))
		if err == nil {
			t.Errorf("Validate(%q) = nil, want error", tt.input)
			continue
		}
		if err != tt.err {
			t.Errorf("Validate(%q) = %v, want %v", tt.input, err, tt.err)
		}
	}
}

func TestValidateMatchesJsonValid(t *testing.T) {
	// Validate and json.Valid should agree on validity for well-formed inputs.
	inputs := []string{
		`null`, `true`, `false`, `1`, `-1`, `0.5`, `""`, `"hello"`,
		`{}`, `{"a":1}`, `[]`, `[1,2,3]`,
		`{"nested":{"a":[1,2,3]}}`,
		// Invalid
		``, `{`, `[`, `"unclosed`, `{"a":}`, `[,]`,
		`tru`, `fals`, `nul`,
	}
	for _, input := range inputs {
		stdValid := json.Valid([]byte(input))
		ourValid := Validate([]byte(input)) == nil
		if stdValid != ourValid {
			t.Errorf("input %q: json.Valid=%v, Validate=%v", input, stdValid, ourValid)
		}
	}
}

func TestValidateZeroAllocs(t *testing.T) {
	inputs := [][]byte{
		[]byte(`{"a":1,"b":"hello","c":[1,2,3],"d":true,"e":null}`),
		[]byte(`[1,2,3,4,5,6,7,8,9,10]`),
		[]byte(`"hello world with some content"`),
		[]byte(`12345`),
	}
	for _, input := range inputs {
		allocs := testing.AllocsPerRun(100, func() {
			Validate(input)
		})
		if allocs != 0 {
			t.Errorf("Validate(%q) allocs = %v, want 0", input, allocs)
		}
	}
}

// --- Progressive validation: error propagation per exec path ---
//
// These tests verify that malformed JSON produces errors (not wrong results)
// during normal query execution. Each test targets a specific exec function's
// scanner error check.

func mustCompile(t *testing.T, query string) *Program {
	t.Helper()
	p, err := Compile(query)
	if err != nil {
		t.Fatalf("Compile(%q): %v", query, err)
	}
	return p
}

func assertRunError(t *testing.T, p *Program, query, input string) {
	t.Helper()
	_, err := p.Run([]byte(input))
	if err == nil {
		t.Errorf("Run(%q, %q) = nil, want error", query, input)
	}
}

func assertRunFuncError(t *testing.T, p *Program, query, input string) {
	t.Helper()
	err := p.RunFunc([]byte(input), func(_ []byte) error { return nil })
	if err == nil {
		t.Errorf("RunFunc(%q, %q) = nil, want error", query, input)
	}
}

func TestProgressiveValidation_Identity(t *testing.T) {
	// execIdentity: skipValue detects mismatched brackets
	p := mustCompile(t, `.`)
	assertRunError(t, p, `.`, `{"a":1]`)        // mismatch
	assertRunError(t, p, `.`, `[1,2}`)           // mismatch
	assertRunError(t, p, `.`, `@`)               // invalid value start
	assertRunError(t, p, `.`, `{"a":"unterminated`) // unterminated string in skipContainer
}

func TestProgressiveValidation_FieldAccess(t *testing.T) {
	// execField / execFieldMulti: findFieldStr detects errors
	// Note: findFieldStr returns early on match, so the error must be
	// encountered BEFORE or DURING field lookup, not after.
	p := mustCompile(t, `.b`)
	assertRunError(t, p, `.b`, `{"a":"unterminated`)   // unterminated string while scanning past "a"'s value
	assertRunError(t, p, `.b`, `{"a":1,"b":@}`)        // invalid value start in target field

	// Unterminated key string
	p2 := mustCompile(t, `.a`)
	assertRunError(t, p2, `.a`, `{"unterminated_key`)   // unterminated key

	// Nested field access — error in inner object
	p3 := mustCompile(t, `.a.b`)
	assertRunError(t, p3, `.a.b`, `{"a":{"b":@}}`)     // invalid value in nested field
}

func TestProgressiveValidation_Iterator(t *testing.T) {
	// execIterator: arrayIter/objectIter detect unterminated containers
	pa := mustCompile(t, `.[]`)
	assertRunFuncError(t, pa, `.[]`, `[1,2`)         // unterminated array
	assertRunFuncError(t, pa, `.[]`, `[1,2,@]`)      // invalid value in array
	assertRunFuncError(t, pa, `.[]`, `{"a":1,"b":2`) // unterminated object
	assertRunFuncError(t, pa, `.[]`, `{a:1}`)         // missing quote on key
}

func TestProgressiveValidation_Delete(t *testing.T) {
	// execDeleteObject: must scan full object to reconstruct without deleted field
	po := mustCompile(t, `del(.a)`)
	assertRunError(t, po, `del(.a)`, `{"a":1,"b":"unterminated`) // unterminated in remaining fields
	assertRunError(t, po, `del(.a)`, `{"a":1,"b":@}`)            // invalid value in remaining field

	// execDeleteArray
	pa := mustCompile(t, `del(.[0])`)
	assertRunError(t, pa, `del(.[0])`, `[1,2,@]`) // invalid value in remaining elements
	assertRunError(t, pa, `del(.[0])`, `[1,2`)     // unterminated
}

func TestProgressiveValidation_Length(t *testing.T) {
	// execLengthSingle: arrayIter/objectIter counting pass
	p := mustCompile(t, `length`)
	assertRunError(t, p, `length`, `[1,2,3`)          // unterminated array
	assertRunError(t, p, `length`, `{"a":1,"b":2`)    // unterminated object
	assertRunError(t, p, `length`, `[1,@,3]`)          // invalid value in array
}

func TestProgressiveValidation_ToEntries(t *testing.T) {
	// execToEntries: objectIter over all key-value pairs
	p := mustCompile(t, `to_entries`)
	assertRunError(t, p, `to_entries`, `{"a":1,"b":2`) // unterminated
	assertRunError(t, p, `to_entries`, `{"a":1]`)       // mismatch
	assertRunError(t, p, `to_entries`, `{a:1}`)          // missing quote on key
}

func TestProgressiveValidation_FromEntries(t *testing.T) {
	// execFromEntries: arrayIter over entry objects
	p := mustCompile(t, `from_entries`)
	assertRunError(t, p, `from_entries`, `[{"key":"a","value":1`)  // unterminated
	assertRunError(t, p, `from_entries`, `[{"key":"a","value":1}`) // unterminated outer array
}

func TestProgressiveValidation_Add(t *testing.T) {
	// execAdd: arrayIter over elements to sum/concat/merge
	p := mustCompile(t, `add`)
	assertRunError(t, p, `add`, `[1,2,3`)         // unterminated
	assertRunError(t, p, `add`, `["a","b"`)        // unterminated string array
	assertRunError(t, p, `add`, `[{"a":1},{"b":2`) // unterminated object in array
}

func TestProgressiveValidation_KeysUnsorted(t *testing.T) {
	// execKeysUnsorted: objectIter over all keys
	p := mustCompile(t, `keys_unsorted`)
	assertRunError(t, p, `keys_unsorted`, `{"a":1,"b":2`)  // unterminated
	assertRunError(t, p, `keys_unsorted`, `{a:1}`)          // missing quote on key
}

func TestProgressiveValidation_Select(t *testing.T) {
	// select evaluates condition via execSingle which uses field access
	// The error must be in the field being accessed
	p := mustCompile(t, `select(.b == 1)`)
	assertRunError(t, p, `select(.b == 1)`, `{"a":"unterminated`) // unterminated scanning past "a"

	p2 := mustCompile(t, `select(.a == 1)`)
	assertRunError(t, p2, `select(.a == 1)`, `{"a":@}`) // invalid value in target field
}

func TestProgressiveValidation_Pipe(t *testing.T) {
	// Pipe propagates errors from left or right side
	p := mustCompile(t, `.a | .b`)
	assertRunError(t, p, `.a | .b`, `{"a":{"b":1]`) // mismatch in inner object

	p2 := mustCompile(t, `.[] | .a`)
	assertRunFuncError(t, p2, `.[] | .a`, `[{"a":1},{"a":2`) // unterminated
}

func TestProgressiveValidation_Construction(t *testing.T) {
	// Object construction reads fields — error must be in a field being read
	p := mustCompile(t, `{a}`)
	assertRunError(t, p, `{a}`, `{"a":@}`) // invalid value in target field

	// Array construction — second field encounters error
	p2 := mustCompile(t, `[.a, .b]`)
	assertRunError(t, p2, `[.a, .b]`, `{"a":1,"b":@}`) // invalid value in second field
}

func TestProgressiveValidation_Map(t *testing.T) {
	// map desugars to [.[] | expr] — needs RunFunc for multi-output
	p := mustCompile(t, `map(.a)`)
	_, err := p.RunAll([]byte(`[{"a":1},{"a":2`))
	if err == nil {
		t.Error("RunAll(map, unterminated) = nil, want error")
	}
}

func TestProgressiveValidation_TryCatchBypass(t *testing.T) {
	// Validation errors should NOT be caught by try-catch
	tests := []struct {
		query string
		input string
	}{
		{`try . catch "caught"`, `{"a":1]`},              // mismatch in identity
		{`try .b catch "caught"`, `{"a":"unterminated`},   // unterminated scanning past "a"
		{`try .[] catch "caught"`, `[1,2`},                // unterminated array
		{`try length catch "caught"`, `[1,2,3`},           // unterminated in length
	}
	for _, tt := range tests {
		p := mustCompile(t, tt.query)
		_, err := p.Run([]byte(tt.input))
		if err == nil {
			t.Errorf("Run(%q, %q) = nil, validation error should bypass try-catch", tt.query, tt.input)
		}
		if err != nil && !isValidationError(err) {
			t.Errorf("Run(%q, %q) = %v, want validation error", tt.query, tt.input, err)
		}
	}
}

func TestProgressiveValidation_NoPanic(t *testing.T) {
	// Comprehensive: no query should panic on malformed input.
	malformed := []string{
		`{"a":1`, `[1,2,`, `"unterminated`,
		`{"a":@}`, `[1,2,@]`, `{"a":1]`, `[1}`,
		`{a:1}`, `{"a" 1}`, ``, `   `,
	}
	queries := []string{
		".", ".a", ".a.b", ".[0]", ".[]",
		"del(.a)", "del(.[0])",
		"length", "type", "keys_unsorted",
		"to_entries", "from_entries",
		"add", `select(.a == 1)`, `.a // "default"`,
		"map(.a)", `if .a then .b else .c end`,
		"tojson", "not",
	}

	for _, q := range queries {
		p, err := Compile(q)
		if err != nil {
			continue
		}
		for _, input := range malformed {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("panic on query=%q input=%q: %v", q, input, r)
					}
				}()
				p.Run([]byte(input))
				p.RunFunc([]byte(input), func(_ []byte) error { return nil })
			}()
		}
	}
}
