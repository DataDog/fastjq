// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License 2.0.
//
// This product includes software developed at Datadog (https://www.datadoghq.com/)
// Copyright 2026-Present Datadog, Inc.

package fastjq

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// --- No-panic: malformed/edge-case inputs ---

// TestNoPanicMalformedInput runs every compiled operation against a corpus of
// malformed, truncated, and edge-case inputs. None must panic.
// This is a hard contract: fastjq never panics, even on invalid input.
func TestNoPanicMalformedInput(t *testing.T) {
	queries := []string{
		// Core access
		".", ".a", ".a.b", ".[0]", ".[-1]", ".[]", ".a?", ".[0]?", ".[]?",
		".[1:3]", ".[:2]", ".[1:]",
		// Modification
		"del(.a)", "del(.a, .b)", "del(.[0])",
		"{a}", "{a: .b}", "[.a, .b]",
		// Logic & control
		`select(.a == "x")`, `select(has("a"))`,
		`if .a then .b else .c end`,
		`if .a == 1 then "x" elif .a == 2 then "y" else "z" end`,
		`.a // "default"`, "not", "empty",
		"try .a", `try .a catch "err"`,
		`try (.a / .b) catch 0`,
		// Arithmetic
		".a + .b", ".a - .b", ".a * .b", ".a / .b", ".a % .b",
		// Array ops
		"map(.a)", "add", "flatten", `flatten(1)`,
		"min", "max", `min_by(.a)`, `max_by(.a)`,
		"any", "all", `any(. > 0)`, `all(. > 0)`,
		`any(.[]; . > 0)`, `all(.[]; . > 0)`,
		"first", "last", `first(.[] | select(. > 0))`,
		`limit(2; .[])`,
		"values", "numbers", "strings", "arrays", "objects",
		"booleans", "nulls", "iterables", "scalars",
		`index(",")`, `rindex(",")`, `indices(",")`,
		// Object transforms
		"to_entries", "from_entries", "length", "keys_unsorted",
		// Type & conversion
		"type", "tojson", "fromjson", "tostring", "tonumber",
		"@base64", "@base64d", "@uri", "@json",
		// Strings
		"ascii_downcase", "ascii_upcase",
		`startswith("x")`, `endswith("x")`,
		`ltrimstr("x")`, `rtrimstr("x")`,
		`split(",")`, `join(",")`,
		// Misc
		`has("key")`, `has(0)`, `in({"a":1})`,
		"debug", `.a // .b // "default"`,
		// Complex combinations
		`[.[] | try .x catch null]`,
		`to_entries | map(select(.value != null)) | from_entries`,
		`[.[] | if .a > 0 then .a elif .b > 0 then .b else 0 end]`,
	}

	malformedInputs := []string{
		// Empty / whitespace
		"", "   ", "\t\n",
		// Truncated
		"{", "}", "[", "]",
		`{"a":`, `{"a":"b`, `[1,2,`,
		`{"a":1`, `[1,2,3`,
		`{"a":{"b":`, `[[[`,
		// Truncated on a backslash escape — the escape skip has no second byte
		// to consume, in a string value, an object key, and a \u form.
		`"ab\`, `"000000\`, `{"ab\`, `{"a":"b\`, `[1,"x\`, `{"a":1,"b\`,
		`"ab\u`, `"ab\u00`, `"abcdefghijklmnop\`,
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

// TestNoReadPastInputLength is the sharp form of the truncated-escape contract:
// the scanner must never read beyond len(input), even when the slice has spare
// capacity holding unrelated bytes.
//
// This is the realistic shape — a record sliced out of a larger read buffer, as
// cmd/fastjq does with bufio lines. Before the escape skip was clamped, a string
// truncated on a backslash advanced past the end and the output picked up a byte
// of whatever followed in the caller's buffer. When the slice had no spare
// capacity the same overshoot was a panic instead.
func TestNoReadPastInputLength(t *testing.T) {
	// Everything after the truncation point must never appear in any output.
	const tail = `","secret":"hunter2"}`
	backing := []byte(`{"ab\` + tail)

	queries := []string{
		".", "keys", "keys_unsorted", "to_entries", "tojson", "length", "type",
		"ascii_downcase", "ascii_upcase", ".[0:3]", ".[1:]", ".a", ".[]",
		"with_entries(.)", "paths", "[..]", "@json", "@base64",
	}
	for _, q := range queries {
		p, err := Compile(q)
		if err != nil {
			t.Fatalf("Compile(%q): %v", q, err)
		}
		for n := 1; n <= len(`{"ab\`); n++ {
			input := backing[:n]
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("PANIC: query=%q input=%q: %v", q, input, r)
					}
				}()
				out, _ := p.RunAll(input)
				for _, o := range out {
					// "secret" only exists past len(input), so seeing any of it
					// means the scanner ran off the end.
					if strings.Contains(string(o), "secret") || strings.Contains(string(o), "hunter2") {
						t.Errorf("query=%q input=%q: output %q contains bytes from beyond len(input)", q, input, o)
					}
				}
			}()
		}
	}

	// Same overshoot, no spare capacity: the byte past the end is off the end of
	// the allocation, so this is where it used to panic.
	for _, q := range queries {
		p, err := Compile(q)
		if err != nil {
			t.Fatalf("Compile(%q): %v", q, err)
		}
		for _, in := range []string{`"ab\`, `"000000\`, `{"ab\`, `{"a":"b\`, `"ab\u00`} {
			exact := make([]byte, len(in))
			copy(exact, in)
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("PANIC: query=%q input=%q: %v", q, in, r)
					}
				}()
				p.RunAll(exact)
			}()
		}
	}
}

// TestTruncatedEscapeTerminates guards the other half of the same defect: the
// ascii_downcase / ascii_upcase loop used to see a trailing backslash, decline to
// copy a second byte that wasn't there, and `continue` without advancing — an
// infinite loop on malformed input, reachable from any caller running untrusted
// documents.
func TestTruncatedEscapeTerminates(t *testing.T) {
	for _, q := range []string{"ascii_downcase", "ascii_upcase", ".[0:3]", "@base64", "explode"} {
		for _, in := range []string{`"ab\`, `"abcdefghijklmnopqrstuvwxyz\`, `"ab\u`} {
			t.Run(q+in, func(t *testing.T) {
				p, err := Compile(q)
				if err != nil {
					t.Fatal(err)
				}
				done := make(chan struct{})
				go func() {
					defer func() {
						recover() // a panic is TestNoReadPastInputLength's business
						close(done)
					}()
					p.RunAll([]byte(in))
				}()
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					t.Fatalf("query=%q input=%q did not terminate", q, in)
				}
			})
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
