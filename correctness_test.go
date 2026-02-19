package fastjq

import (
	"fmt"
	"strings"
	"testing"
)

// --- No-panic: malformed/edge-case inputs ---

// TestNoPanicMalformedInput runs every compiled operation against a corpus of
// malformed, truncated, and edge-case inputs. None must panic.
func TestNoPanicMalformedInput(t *testing.T) {
	queries := []string{
		".", ".a", ".a.b", ".[0]", ".[-1]", ".[]",
		"del(.a)", `select(.a == "x")`, `select(has("a"))`,
		"{a}", "map(.a)", "to_entries", "from_entries",
		`with_entries(select(.value != null))`,
		"length", "keys_unsorted", "type",
		"ascii_downcase", `startswith("x")`, `ltrimstr("x")`,
		"any", `any(. > 0)`, "first", `limit(2; .[])`,
		`if .a then .b else .c end`, `.a // "default"`,
		"not", "empty",
	}

	malformedInputs := []string{
		// Empty / whitespace
		"", "   ", "\t\n",
		// Truncated
		"{", "}", "[", "]",
		`{"a":`, `{"a":"b`, `[1,2,`,
		`{"a":1`, `[1,2,3`,
		`{"a":{"b":`, `[[[`,
		// Invalid structure
		`{null}`, `[,]`, `{:1}`, `{1:2}`,
		// Stray values
		`undefined`, `NaN`, `Infinity`, `-`,
		// Bare scalars (valid JSON but not objects/arrays)
		`null`, `true`, `false`, `0`, `42`, `-1`, `3.14`,
		`""`, `"hello"`, `"line\nbreak"`,
		// Unicode
		`"héllo"`, `"日本語"`, `"\u0041\u0042"`, `"\u0000"`,
		// Deeply nested
		strings.Repeat(`{"a":`, 50) + `1` + strings.Repeat(`}`, 50),
		strings.Repeat(`[`, 50) + `1` + strings.Repeat(`]`, 50),
		// Large string
		`"` + strings.Repeat("x", 10000) + `"`,
		// Object with many keys
		func() string {
			var b strings.Builder
			b.WriteString("{")
			for i := 0; i < 100; i++ {
				if i > 0 {
					b.WriteString(",")
				}
				fmt.Fprintf(&b, `"k%d":%d`, i, i)
			}
			b.WriteString("}")
			return b.String()
		}(),
		// Null bytes (technically invalid JSON but should not panic)
		"\x00", `{"a":\x00}`,
	}

	for _, q := range queries {
		p, err := Compile(q)
		if err != nil {
			continue // compile error is fine; panic is not
		}
		for _, input := range malformedInputs {
			// Use recover to catch any panic and fail the test cleanly
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("PANIC: query=%q input=%q: %v", q, input, r)
					}
				}()
				p.Run([]byte(input))
			}()
		}
	}
}

// TestNoPanicInvalidQueries ensures Compile never panics, only returns errors.
func TestNoPanicInvalidQueries(t *testing.T) {
	badQueries := []string{
		"", "   ",
		"del()", "select()", "has()", "limit()",
		`del(.a | .b)`,
		`limit(-1; .[])`,
		`if then end`,
		`if .a then end end`,
		`.a == .b == .c`, // chaining not supported
		`(`, `)`, `[`, `{`, `|`, `//`,
		`select(.a == )`,
		`."key with spaces"`,
		`.a..b`,
		`..`,
		strings.Repeat(".a", 1000), // very deep chain
		strings.Repeat("(", 100) + "." + strings.Repeat(")", 100),
	}
	for _, q := range badQueries {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("PANIC on Compile(%q): %v", q, r)
				}
			}()
			Compile(q)
		}()
	}
}

// --- JSON edge case correctness ---

func TestEdgeCaseEmptyInput(t *testing.T) {
	p, _ := Compile(".")
	// Empty input: identity should return empty (not panic)
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("panic on empty input: %v", r)
			}
		}()
		p.Run([]byte{})
	}()
}

func TestEdgeCaseScalarInputs(t *testing.T) {
	// fastjq operates on any JSON value, not just objects
	cases := []struct{ query, input, want string }{
		{".", `null`, `null`},
		{".", `true`, `true`},
		{".", `false`, `false`},
		{".", `42`, `42`},
		{".", `3.14`, `3.14`},
		{".", `"hello"`, `"hello"`},
		{"type", `null`, `"null"`},
		{"type", `true`, `"boolean"`},
		{"type", `42`, `"number"`},
		{"type", `"hello"`, `"string"`},
		{"length", `"hello"`, `5`},
		{"length", `null`, `0`},
	}
	for _, tc := range cases {
		assertQuery(t, tc.query, tc.input, tc.want)
	}
}

func TestEdgeCaseStringEscapes(t *testing.T) {
	// All standard JSON escape sequences
	assertQuery(t, ".a", `{"a":"line1\nline2"}`, `"line1\nline2"`)
	assertQuery(t, ".a", `{"a":"tab\there"}`, `"tab\there"`)
	assertQuery(t, ".a", `{"a":"quote\"here"}`, `"quote\"here"`)
	assertQuery(t, ".a", `{"a":"back\\slash"}`, `"back\\slash"`)
	assertQuery(t, ".a", `{"a":"\u0048ello"}`, `"\u0048ello"`) // \u0048 = H
	assertQuery(t, ".a", `{"a":"\b\f\r"}`, `"\b\f\r"`)

	// length counts chars, escape sequences = 1 char each
	assertQuery(t, `length`, `"he\"llo"`, `6`)   // he"llo = 6 chars
	assertQuery(t, `length`, `"a\nb"`, `3`)       // a\nb = 3 chars
	assertQuery(t, `length`, `"\u0041BC"`, `3`)   // ABC = 3 chars
}

func TestEdgeCaseUnicode(t *testing.T) {
	assertQuery(t, ".name", `{"name":"héllo"}`, `"héllo"`)
	assertQuery(t, ".name", `{"name":"日本語"}`, `"日本語"`)
	assertQuery(t, `has("日本語")`, `{"日本語":1}`, `true`)
	assertQuery(t, `has("日本語")`, `{"other":1}`, `false`)
	assertQuery(t, `startswith("hé")`, `"héllo"`, `true`)
	assertQuery(t, `startswith("he")`, `"héllo"`, `false`) // byte mismatch
}

func TestEdgeCaseNumbers(t *testing.T) {
	assertQuery(t, ".", `0`, `0`)
	assertQuery(t, ".", `-0`, `-0`)
	assertQuery(t, ".", `1e10`, `1e10`)
	assertQuery(t, ".", `1.5e-3`, `1.5e-3`)
	assertQuery(t, ".a == 1", `{"a":1.0}`, `true`)    // 1.0 == 1
	assertQuery(t, ".a == 1", `{"a":1e0}`, `true`)    // 1e0 == 1
	assertQuery(t, ".a > 0", `{"a":-1}`, `false`)
	assertQuery(t, ".a > 0", `{"a":0}`, `false`)
	assertQuery(t, ".a >= 0", `{"a":0}`, `true`)
}

func TestEdgeCaseDeepNesting(t *testing.T) {
	// 10 levels deep — should work fine
	input := `{"a":{"b":{"c":{"d":{"e":{"f":{"g":{"h":{"i":{"j":42}}}}}}}}}}`
	assertQuery(t, ".a.b.c.d.e.f.g.h.i.j", input, `42`)
}

func TestEdgeCaseLargeObject(t *testing.T) {
	// 100-key object — all operations should work
	var b strings.Builder
	b.WriteString("{")
	for i := 0; i < 100; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `"field_%d":%d`, i, i)
	}
	b.WriteString("}")
	input := b.String()

	assertQuery(t, ".field_0", input, `0`)
	assertQuery(t, ".field_99", input, `99`)
	assertQuery(t, ".field_50", input, `50`)
	assertQuery(t, `has("field_99")`, input, `true`)
	assertQuery(t, `has("field_100")`, input, `false`)
}

func TestEdgeCaseEmptyCollections(t *testing.T) {
	assertQuery(t, "length", `{}`, `0`)
	assertQuery(t, "length", `[]`, `0`)
	assertQuery(t, "keys_unsorted", `{}`, `[]`)
	assertQuery(t, "keys_unsorted", `[]`, `[]`)
	assertQuery(t, "to_entries", `{}`, `[]`)
	assertQuery(t, "map(.x)", `[]`, `[]`)
	assertQuery(t, "any", `[]`, `false`)
	assertQuery(t, "all", `[]`, `true`) // vacuous truth
}

func TestEdgeCaseMissingFieldsReturnNull(t *testing.T) {
	assertQuery(t, ".missing", `{"a":1}`, `null`)
	assertQuery(t, ".a.b", `{}`, `null`)
	assertQuery(t, ".[5]", `[1,2,3]`, `null`)

	// Note: fastjq errors (not returns null) when traversing into a non-object.
	// .a.b where .a=1 (not an object) returns errExpectedObjectField.
	// Use the optional operator to get null in that case:
	assertQuery(t, ".a.b?", `{"a":1}`, `null`)
}

func TestEdgeCaseNullAndFalseAreFalsy(t *testing.T) {
	assertNoOutput(t, `select(.)`, `null`)
	assertNoOutput(t, `select(.)`, `false`)
	assertQuery(t, `select(.)`, `0`, `0`)     // 0 is truthy
	assertQuery(t, `select(.)`, `""`, `""`)   // "" is truthy
	assertQuery(t, `select(.)`, `[]`, `[]`)   // [] is truthy
	assertQuery(t, `select(.)`, `{}`, `{}`)   // {} is truthy
}

func TestEdgeCaseWhitespaceInput(t *testing.T) {
	// Whitespace-padded valid JSON
	assertQuery(t, ".a", `  { "a" : 1 }  `, `1`)
	assertQuery(t, ".[]", `  [ 1 , 2 , 3 ]  `, `1`) // first result via Run
}

func TestEdgeCaseStringComparisonWithEscapes(t *testing.T) {
	// Compare strings containing escape sequences
	assertQuery(t, `.a == "line\nbreak"`, `{"a":"line\nbreak"}`, `true`)
	assertQuery(t, `.a == "tab\there"`, `{"a":"tab\there"}`, `true`)
	assertQuery(t, `startswith("he\nllo")`, `"he\nllo world"`, `true`)
}

func TestEdgeCaseAlternativeWithZeroAndEmpty(t *testing.T) {
	// 0, "", [], {} are all truthy — alternative should NOT trigger
	assertQuery(t, `.x // "default"`, `{"x":0}`, `0`)
	assertQuery(t, `.x // "default"`, `{"x":""}`, `""`)
	assertQuery(t, `.x // "default"`, `{"x":[]}`, `[]`)
	assertQuery(t, `.x // "default"`, `{"x":{}}`, `{}`)
	// Only null and false trigger alternative
	assertQuery(t, `.x // "default"`, `{"x":null}`, `"default"`)
	assertQuery(t, `.x // "default"`, `{"x":false}`, `"default"`)
	assertQuery(t, `.x // "default"`, `{}`, `"default"`) // missing = null
}

func TestEdgeCaseDelNonexistentNested(t *testing.T) {
	// del of non-existent nested field is a no-op
	assertQuery(t, `del(.a.b)`, `{"a":1}`, `{"a":1}`)
	assertQuery(t, `del(.x.y.z)`, `{"a":1}`, `{"a":1}`)
}

func TestEdgeCaseLimitZeroAndNegative(t *testing.T) {
	assertNoOutput(t, `limit(0; .[])`, `[1,2,3]`)
}

func TestEdgeCaseFirstLastOnEmpty(t *testing.T) {
	// first/last with no matching results produce no output
	assertNoOutput(t, `first(.[] | select(. > 10))`, `[1,2,3]`)
	assertNoOutput(t, `last(.[] | select(. > 10))`, `[1,2,3]`)
}

func TestEdgeCaseTypeOnAllTypes(t *testing.T) {
	cases := []struct{ input, want string }{
		{`null`, `"null"`},
		{`true`, `"boolean"`},
		{`false`, `"boolean"`},
		{`0`, `"number"`},
		{`-1.5`, `"number"`},
		{`""`, `"string"`},
		{`"hello"`, `"string"`},
		{`{}`, `"object"`},
		{`{"a":1}`, `"object"`},
		{`[]`, `"array"`},
		{`[1,2,3]`, `"array"`},
	}
	for _, tc := range cases {
		assertQuery(t, "type", tc.input, tc.want)
	}
}

func TestEdgeCaseKeyWithSpecialChars(t *testing.T) {
	// Keys with spaces, colons, dots — accessed via has/to_entries
	input := `{"key with spaces":1,"key:colon":2}`
	assertQuery(t, `has("key with spaces")`, input, `true`)
	assertQuery(t, `has("key:colon")`, input, `true`)
	assertQuery(t, `has("missing")`, input, `false`)
}

func TestEdgeCaseAsciiCasePreservesEscapes(t *testing.T) {
	// Escape sequences should pass through unchanged during case conversion
	assertQuery(t, `ascii_downcase`, `"HELLO\nWORLD"`, `"hello\nworld"`)
	assertQuery(t, `ascii_upcase`, `"hello\nworld"`, `"HELLO\nWORLD"`)
	assertQuery(t, `ascii_downcase`, `"ABC\u0041"`, `"abc\u0041"`) // \u preserved
}

func TestEdgeCaseHasVsNullCheck(t *testing.T) {
	// has(key) returns true even when value is null — != null returns false
	assertQuery(t, `has("x")`, `{"x":null}`, `true`)
	assertQuery(t, `.x != null`, `{"x":null}`, `false`)
	assertQuery(t, `has("x")`, `{}`, `false`)
	assertQuery(t, `.x != null`, `{}`, `false`) // missing → null → equals null
}

func TestEdgeCasePipeMultiOutput(t *testing.T) {
	// Pipes propagate multi-output correctly
	p, _ := Compile(`.items[] | select(.active == true) | .name`)
	input := []byte(`{"items":[{"name":"a","active":true},{"name":"b","active":false},{"name":"c","active":true}]}`)
	results, err := p.RunAll(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || string(results[0]) != `"a"` || string(results[1]) != `"c"` {
		t.Errorf("got %v", results)
	}
}

func TestEdgeCaseGroupingChangesEval(t *testing.T) {
	// Grouping affects precedence
	assertQuery(t, `(.a == 1) and (.b == 2)`, `{"a":1,"b":2}`, `true`)
	assertQuery(t, `(.a == 1) or (.b == 3)`, `{"a":1,"b":2}`, `true`)
}

func TestEdgeCaseLongString(t *testing.T) {
	// 10000-char string — no panic, correct length
	s := strings.Repeat("x", 10000)
	input := `"` + s + `"`
	assertQuery(t, "length", input, `10000`)
	assertQuery(t, `startswith("xxx")`, input, `true`)
	assertQuery(t, `endswith("xxx")`, input, `true`)
}
