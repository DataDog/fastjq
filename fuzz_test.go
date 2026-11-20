// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License 2.0.
//
// This product includes software developed at Datadog (https://www.datadoghq.com/)
// Copyright 2026-Present Datadog, Inc.

package fastjq

import (
	"errors"
	"testing"
)

// FuzzCompile ensures no query string causes Compile to panic.
func FuzzCompile(f *testing.F) {
	seeds := []string{
		".", ".foo", ".foo.bar", ".[0]", ".[-1]", ".[]",
		"del(.foo)", "del(.foo, .bar)", "del(.[0])",
		// del with slice ranges
		"del(.[1:3])", "del(.[:2])", "del(.[-1:])", "del(.[2:4],.[0])",
		`select(.x == "y")`, `select(.x != null)`,
		`select(.x > 0 and .y < 10)`,
		"{a, b}", "{a: .b}", "[.a, .b]",
		"map(.x)", "to_entries", "from_entries",
		"length", "keys_unsorted", "type",
		"ascii_downcase", "ascii_upcase",
		`startswith("foo")`, `endswith("foo")`,
		`ltrimstr("foo")`, `rtrimstr("foo")`,
		`has("key")`, `has(0)`,
		"any", "all", `any(. > 0)`, `all(. > 0)`,
		`any(.[]; . > 0)`, `all(.[]; . > 0)`,
		"first", "last", `first(.[] | select(. > 0))`,
		`limit(3; .[])`,
		`if .x == "y" then .a else .b end`,
		`if .x == 1 then "a" elif .x == 2 then "b" else "c" end`,
		`.x // "default"`, "empty", "not",
		`true`, `false`, `null`, `42`, `"hello"`,
		"try .foo", `try .foo catch "err"`,
		"tojson", "fromjson", "tostring", "tonumber",
		"@base64", "@base64d", "@uri", "@json",
		"min", "max", `min_by(.x)`, `max_by(.x)`,
		".a + .b", ".a - .b", ".a * .b", ".a / .b", ".a % .b",
		"add", "flatten", `flatten(1)`,
		`split(",")`, `join(",")`,
		"values", "numbers", "strings", "arrays", "objects", "booleans", "nulls",
		"iterables", "scalars",
		`index(",")`, `rindex(",")`, `indices(",")`,
		// indices with array needle (subsequence search)
		`indices([1,2])`, `index([1,2])`, `rindex([1,2])`,
		`in({"a":1})`,
		".[1:3]", ".[:2]", ".[1:]",
		"debug",
		// Edge cases in parsing
		`select(.a and .b or .c)`,
		`.a.b.c.d.e`,
		`if has("x") then .x else empty end`,
		`[limit(3; .[])]`,
		`any(ascii_downcase == "error")`,
		`try (.a / .b) catch 0`,
		`[.[] | try .x catch null]`,
		`{"a":1} + {"b":2}`,
		`.a // .b // .c // "default"`,
		`to_entries | map(select(.value != null)) | from_entries`,
		// Math builtins — 1-arg float ops
		"sqrt", "fabs", "atan", "log", "log2", "log10",
		"exp", "exp2", "exp10", "cbrt", "logb", "nearbyint",
		"sin", "cos", "tan", "asin", "acos",
		"tgamma", "lgamma", "j0", "j1",
		// String interpolation
		`"\(.name)"`, `"\(.a) and \(.b)"`, `"value: \(.)"`,
		`"\(.level): \(.svc) [\(.status)]"`,
		// isempty / nth / error(expr)
		`isempty(empty)`, `isempty(.[])`, `isempty(1,2,3)`,
		`nth(0; .[])`, `nth(2; .[])`, `[nth(1; .[])]`,
		`try error("msg") catch .`,
		`error("x")`, // compile fine, runtime error
		// try / // precedence
		`try error(0) // 1`,
		`try .a // "default"`,
		// Variable binding / destructuring / defs / control flow
		`. as $x | [$x,$x]`,
		`. as [$a,$b] | [$b,$a]`,
		`. as {$a, b: [$c]} | [$a,$c]`,
		`reduce .[] as $x (0; . + $x)`,
		`foreach .[] as $x (0; . + $x; .)`,
		`label $out | .[] | if . > 2 then break $out else . end`,
		`def inc: . + 1; inc`,
		`def addn($x): . + $x; addn(2)`,
		`def twice(f): f | f; twice(. + 1)`,
		// Path / update / recursion helpers
		`paths`, `paths(type == "number")`,
		`path(.foo[0])`, `getpath(["foo",0])`,
		`setpath(["foo",0]; 1)`, `delpaths([["foo",0]])`,
		`pick(.foo, .bar[0])`,
		`..`, `[..]`, `.. | scalars`,
		`recurse(.foo[])`, `recurse(. * 2; . < 20)`,
		`walk(if type == "array" then sort else . end)`,
		// Newer builtin / formatter parity
		`map_values(. + 1)`, `with_entries(.key |= "x_" + .)`,
		`INDEX(.[]; .id)`, `IN(.[])`, `bsearch(3)`,
		`combinations`, `combinations(2)`,
		`@html "<b>\(.)</b>"`, `@sh "echo \(.)"`,
		`todate`, `date`, `now`, `fromdate`,
		`strftime("%Y-%m-%d")`, `strflocaltime("%Y")`,
		`strptime("%Y-%m-%d")`, `mktime`, `gmtime`,
		`builtins`, `$__loc__`,
		`leaf_paths`, `hypot(3;4)`, `fma(2;3;4)`,
		// Truncated object construction — one per key form. These panicked
		// until parseConstruct rechecked for end of input after the comma.
		"{a,", "{a:1,", "{a,b,", "{0,", `{"a",`, `{"a":1,`, "{$a,", "{(1):2,",
		"{a", "{a,}", "{",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, query string) {
		// Must not panic regardless of input
		Compile(query)
	})
}

// FuzzRunFixed runs all supported operations against arbitrary JSON input.
// Ensures no input (including malformed) can cause a panic.
func FuzzRunFixed(f *testing.F) {
	seeds := []string{
		`{"a":1}`,
		`{"level":"error","msg":"boom"}`,
		`[1,2,3]`,
		`null`, `""`, `true`, `false`, `42`, `{}`, `[]`,
		// Malformed / edge cases
		``, `{`, `}`, `[`, `]`,
		`{"a":`, `{"a":"b`, `[1,2,`,
		`{"a":1`, `[1,2,3`,
		`{null}`, `[,]`, `{:1}`,
		`[[},[},00`, // regression seed for #43
		`"\u0000"`, `"\n\r\t\b\f\\\""`,
		`{"key with spaces": 1}`,
		`{"a":{"b":{"c":{"d":{"e":1}}}}}`,
		`[[[[[1]]]]]`,
		// Strings for @uri, @base64, tojson, etc.
		`"hello world"`,
		`"aGVsbG8="`,
		`"{\"a\":1}"`,
		`"42"`,
		// Unicode strings (multi-byte UTF-8, for index/indices codepoint tests)
		`"здравствуй мир!"`,
		`"ƒoo"`,
		`"\u03bc"`,
		// Strings with JSON escape sequences (for @base64/@uri decode correctness)
		`"foo\nbar"`,
		`"tab\there"`,
		// Truncated on a backslash escape: the escape skip has no second byte to
		// consume. Overshot the end of input until skipEscape clamped it.
		`"ab\`, `"000000\`, `{"ab\`, `{"a":"b\`, `[1,"x\`, `"ab\u`, `"ab\u00`,
		// Arrays for subsequence search
		`[0,1,2,3,1,4,2,5,1,2,6,7]`,
		`[1,2,1,2,1,2]`,
		// Inputs with UTF-8 BOM
		"\xEF\xBB\xBF\"hello\"",
		"\xEF\xBB\xBF{\"a\":1}",
		"\xEF\xBB\xBF[1,2,3]",
		// Numeric inputs for math builtins
		`0`, `1`, `-1`, `3.14159`, `-2.718`, `0.5`, `1e10`, `-0`,
		// Domain-edge inputs: sqrt(-1)→null, log(-1)→null, asin(2)→null
		`-1`, `-100`, `2`,
		// Mixed objects for string interpolation
		`{"name":"alice","level":"error","svc":"api","status":500}`,
		`{"foo":[{"id":1},{"id":2}],"bar":[3,2,1],"flags":{"a":true,"b":false}}`,
		`{"foo":{"bar":[1,2,3],"baz":{"qux":4}},"id":2}`,
		`[{"id":1,"name":"a"},{"id":2,"name":"b"}]`,
		`{"nested":[[4,1,7],[8,5,2],[3,6,9]]}`,
		`[{"foo":[{"foo":[]},{"foo":[{"foo":[]}]}]}]`,
		`"2024-01-02T03:04:05Z"`,
		`"2024-01-02"`,
		// Boolean and null for isempty
		`true`, `false`,
	}
	for _, s := range seeds {
		f.Add(s)
	}

	queries := []string{
		".", ".a", ".a.b", ".[0]", ".[-1]", ".[]",
		"del(.a)", "del(.a, .b)", "del(.[0])",
		// del with slice ranges
		"del(.[1:3])", "del(.[:2])", "del(.[-1:])", "del(.[2:4],.[0],.[-2:])",
		`select(.a == "x")`, `select(.a != null)`,
		`select(.a > 0)`, `select(has("a"))`,
		`select(.a | not)`, `select(any)`, `select(all)`,
		"{a}", "{a: .b}", "[.a, .b]",
		"map(.a)", "to_entries", "from_entries",
		"length", "keys_unsorted", "type",
		"ascii_downcase", "ascii_upcase",
		`startswith("x")`, `endswith("x")`,
		`ltrimstr("x")`, `rtrimstr("x")`,
		"any", `any(. > 0)`, "all", `all(. > 0)`,
		`any(.[]; . > 0)`, `all(.[]; . > 0)`,
		"first", "last",
		`first(.[] | select(. > 0))`,
		`last(.[] | select(. > 0))`,
		`limit(3; .[])`,
		`if .a then .b else .c end`,
		`if .a == 1 then "one" elif .a == 2 then "two" else "other" end`,
		`.a // "default"`,
		"empty", "not",
		"try .a", `try .a catch "err"`,
		"tojson", "fromjson", "tostring", "tonumber",
		"@base64", "@base64d", "@uri", "@json",
		"min", "max", `min_by(.a)`, `max_by(.a)`,
		".a + .b", ".a - .b", ".a * .b", ".a / .b", ".a % .b",
		"add", "flatten", `flatten(1)`,
		`split(",")`, `join(",")`,
		"values", "numbers", "strings", "arrays", "objects",
		"booleans", "nulls", "iterables", "scalars",
		`index(",")`, `rindex(",")`, `indices(",")`,
		// indices with array needle
		`indices([1,2])`, `index([1,2])`, `rindex([0])`,
		".[1:3]", ".[:2]", ".[1:]",
		"debug",
		`try (.a / .b) catch 0`,
		`[.[] | try .x catch null]`,
		// Math builtins
		"sqrt", "fabs", "atan", "log", "log2", "log10",
		"exp", "exp2", "exp10", "cbrt", "logb", "nearbyint",
		"sin", "cos", "tan", "asin", "acos", "tgamma", "lgamma", "j0", "j1",
		// String interpolation
		`"\(.a)"`, `"\(.a) and \(.b)"`,
		// isempty / nth / error(expr)
		`isempty(empty)`, `isempty(.[])`,
		`nth(0; .[])`, `nth(2; .[])`,
		`try error("msg") catch .`,
		// try / // precedence
		`try error(0) // 1`,
		`try .a // "default"`,
		// Variable binding / defs / control flow
		`. as $x | [$x,$x]`,
		`. as [$a,$b] | [$b,$a]`,
		`. as {$a, b: [$c]} | [$a,$c]`,
		`reduce .[] as $x (0; . + $x)`,
		`foreach .[] as $x (0; . + $x; .)`,
		`label $out | .[] | if . > 2 then break $out else . end`,
		`def inc: . + 1; inc`,
		`def addn($x): . + $x; addn(2)`,
		`def twice(f): f | f; twice(. + 1)`,
		// Path / update / recursion helpers
		`paths`, `paths(type == "number")`,
		`path(.foo[0])`, `getpath(["foo",0])`,
		`setpath(["foo",0]; 1)`, `delpaths([["foo",0]])`,
		`pick(.foo, .bar[0])`,
		`..`, `.. | scalars`,
		`recurse(.foo[])`, `recurse(. * 2; . < 20)`,
		`walk(if type == "array" then sort else . end)`,
		// Newer builtin / formatter parity
		`map_values(. + 1)`, `with_entries(.key |= "x_" + .)`,
		`INDEX(.[]; .id)`, `IN(.[])`, `bsearch(3)`,
		`combinations`, `combinations(2)`,
		`@html "<b>\(.)</b>"`, `@sh "echo \(.)"`,
		`todate`, `date`, `now`, `fromdate`,
		`strftime("%Y-%m-%d")`, `strflocaltime("%Y")`,
		`strptime("%Y-%m-%d")`, `mktime`, `gmtime`,
		`builtins`, `$__loc__`,
		`leaf_paths`, `hypot(3;4)`, `fma(2;3;4)`,
	}

	programs := make([]*Program, 0, len(queries))
	for _, q := range queries {
		p, err := Compile(q)
		if err == nil {
			programs = append(programs, p)
		}
	}

	f.Fuzz(func(t *testing.T, input string) {
		for _, p := range programs {
			p.Run([]byte(input)) // must not panic
		}
	})
}

// FuzzBoth fuzzes both the query string and the input together.
func FuzzBoth(f *testing.F) {
	f.Add(".", `{"a":1}`)
	f.Add(".foo", `{"foo":"bar"}`)
	f.Add(`select(.x == "y")`, `{"x":"y"}`)
	f.Add("del(.a)", `{"a":1,"b":2}`)
	f.Add(".[]", `[1,2,3]`)
	f.Add("length", `{"a":1}`)
	f.Add("type", `"hello"`)
	f.Add(`has("x")`, `{"x":null}`)
	f.Add("to_entries", `{"a":1}`)
	f.Add(`ascii_downcase`, `"HELLO"`)
	f.Add(`startswith("x")`, `"xyz"`)
	f.Add(`any(. > 0)`, `[1,2,3]`)
	f.Add(`limit(2; .[])`, `[1,2,3,4,5]`)
	f.Add(`try .foo catch "err"`, `[1,2,3]`)
	f.Add(`tojson`, `{"a":1}`)
	f.Add(`fromjson`, `"{\"a\":1}"`)
	f.Add(`tonumber`, `"42"`)
	f.Add(`@uri`, `"hello world"`)
	f.Add(`min`, `[3,1,4,1,5]`)
	f.Add(`min_by(.a)`, `[{"a":3},{"a":1}]`)
	f.Add(`.a + .b`, `{"a":1,"b":2}`)
	f.Add(`.a - .b`, `{"a":[1,2,3],"b":[2]}`)
	f.Add(`if .a == 1 then "x" elif .a == 2 then "y" else "z" end`, `{"a":1}`)
	f.Add(`[.[] | try .x catch null]`, `[{"x":1},42,{"x":3}]`)
	f.Add(`any(.[]; . > 5)`, `[1,2,3,4,5,6]`)
	// del with slice ranges
	f.Add(`del(.[1:3])`, `[0,1,2,3,4,5]`)
	f.Add(`del(.[:2],.[-1:])`, `[10,20,30,40,50]`)
	f.Add(`del(.[2:4],.[0],.[-2:])`, `[0,1,2,3,4,5,6,7]`)
	// indices with array needle
	f.Add(`indices([1,2])`, `[0,1,2,3,1,4,2,5,1,2,6,7]`)
	f.Add(`index([1,2])`, `[1,2,3,4,1,2]`)
	f.Add(`rindex([1,2])`, `[1,2,3,4,1,2]`)
	// overlapping substring indices
	f.Add(`indices("aba")`, `"xababababax"`)
	// Unicode index/indices
	f.Add(`index("!")`, `"здравствуй мир!"`)
	f.Add(`indices("o")`, `"ƒoo"`)
	// @base64 / @uri with escape sequences
	f.Add(`@base64`, `"foo\nbar"`)
	f.Add(`@uri`, "\"\u03bc\"")
	// min/max on array of arrays
	f.Add(`min`, `[[4,2],[1,3],[2,4]]`)
	f.Add(`max_by(.[1])`, `[[4,2,"a"],[3,1,"a"],[2,4,"a"]]`)
	// BOM-prefixed input (via identity)
	f.Add(`.`, "\xEF\xBB\xBF{\"a\":1}")
	// Math builtins — verify no panic on any float input including domain errors
	f.Add(`sqrt`, `9`)
	f.Add(`sqrt`, `-1`) // NaN domain → null
	f.Add(`log`, `1`)
	f.Add(`log`, `-1`) // NaN domain → null
	f.Add(`sin`, `0`)
	f.Add(`cos`, `0`)
	f.Add(`atan`, `1`)
	f.Add(`exp`, `0`)
	f.Add(`exp10`, `3`)
	f.Add(`cbrt`, `27`)
	f.Add(`logb`, `8`)
	f.Add(`tgamma`, `5`)
	f.Add(`lgamma`, `1`)
	f.Add(`fabs`, `-3.14`)
	f.Add(`nearbyint`, `3.7`)
	f.Add(`j0`, `0`)
	f.Add(`j1`, `1`)
	f.Add(`asin`, `2`) // NaN domain → null
	f.Add(`acos`, `2`) // NaN domain → null
	// String interpolation — field access (0 allocs) and number embedding
	f.Add(`"\(.name)"`, `{"name":"alice","age":30}`)
	f.Add(`"\(.a) + \(.b)"`, `{"a":"foo","b":"bar"}`)
	f.Add(`"value: \(.)"`, `42`)
	f.Add(`"\(.level): \(.svc)"`, `{"level":"error","svc":"api"}`)
	// Nested interpolation with pipes
	f.Add(`"\(.name | ascii_upcase)"`, `{"name":"alice"}`)
	// isempty
	f.Add(`isempty(empty)`, `null`)
	f.Add(`isempty(.[])`, `[]`)
	f.Add(`isempty(.[])`, `[1,2,3]`)
	f.Add(`[.[] | isempty(select(. > 2))]`, `[1,2,3,4,5]`)
	// nth
	f.Add(`nth(0; .[])`, `[10,20,30]`)
	f.Add(`nth(2; .[])`, `[10,20,30]`)
	f.Add(`nth(99; .[])`, `[1,2,3]`) // past end → no output
	// error(expr) 1-arg
	f.Add(`try error("boom") catch .`, `null`)
	f.Add(`try error(.) catch .`, `{"a":1}`)
	f.Add(`try error("invalid: \(.)") catch .`, `42`)
	// try / // precedence
	f.Add(`try error(0) // 1`, `null`)
	f.Add(`try .missing // "default"`, `{}`)
	// Variable binding / defs / control flow
	f.Add(`. as $x | [$x,$x]`, `{"a":1}`)
	f.Add(`. as [$a,$b] | [$b,$a]`, `[1,2]`)
	f.Add(`. as {$a, b: [$c]} | [$a,$c]`, `{"a":1,"b":[2]}`)
	f.Add(`reduce .[] as $x (0; . + $x)`, `[1,2,3,4]`)
	f.Add(`foreach .[] as $x (0; . + $x; .)`, `[1,2,3]`)
	f.Add(`label $out | .[] | if . > 2 then break $out else . end`, `[0,1,2,3]`)
	f.Add(`def inc: . + 1; inc`, `41`)
	f.Add(`def addn($x): . + $x; addn(2)`, `40`)
	f.Add(`def twice(f): f | f; twice(. + 1)`, `1`)
	// Recursive defs must return an error, not overflow the goroutine stack.
	f.Add(`def f: f; f`, `{"a":1}`)
	f.Add(`def f: .|f; f`, `{"a":1}`)
	f.Add(`def f(x): f(x); f(.)`, `{"a":1}`)
	f.Add(`def f($x): f($x); f(.)`, `{"a":1}`)
	// Path / update / recursion helpers
	f.Add(`paths`, `{"a":[1,true,{"b":"x"}],"c":null}`)
	f.Add(`paths(type == "number")`, `{"a":[1,true,{"b":"x"}],"c":null}`)
	f.Add(`path(.foo[0])`, `{"foo":["x","y"]}`)
	f.Add(`getpath(["foo",0])`, `{"foo":["x","y"]}`)
	f.Add(`setpath(["foo",0]; 1)`, `{"foo":[0,2]}`)
	f.Add(`delpaths([["foo",0]])`, `{"foo":[0,2]}`)
	f.Add(`pick(.foo, .bar[0])`, `{"foo":1,"bar":[2,3],"baz":4}`)
	f.Add(`.. | scalars`, `{"a":[1,true,{"b":"x"}],"c":null}`)
	f.Add(`recurse(.foo[])`, `{"foo":[{"foo":[]},{"foo":[{"foo":[]}]}]}`)
	f.Add(`walk(if type == "array" then sort else . end)`, `[[4,1,7],[8,5,2],[3,6,9]]`)
	// Newer builtins / formatters
	f.Add(`map_values(. + 1)`, `[1,2,3]`)
	f.Add(`with_entries(.key |= "x_" + .)`, `{"a":1,"b":2}`)
	f.Add(`INDEX(.[]; .id)`, `[{"id":1,"name":"a"},{"id":2,"name":"b"}]`)
	f.Add(`IN(.[])`, `[1,2,3]`)
	f.Add(`bsearch(3)`, `[1,2,3,4,5]`)
	f.Add(`combinations`, `[[1,2],[3,4]]`)
	f.Add(`combinations(2)`, `[[1,2],[3,4]]`)
	f.Add(`@html "<b>\(.)</b>"`, `"x&y"`)
	f.Add(`@sh "echo \(.)"`, `"hello world"`)
	f.Add(`todate`, `1704164645`)
	f.Add(`date`, `1704164645`)
	f.Add(`fromdate`, `"2024-01-02T03:04:05Z"`)
	f.Add(`strftime("%Y-%m-%d")`, `[1704164645,0]`)
	f.Add(`strflocaltime("%Y")`, `[1704164645,0]`)
	f.Add(`strptime("%Y-%m-%d")`, `"2024-01-02"`)
	f.Add(`mktime`, `[2024,0,2,3,4,5,0,1]`)
	f.Add(`gmtime`, `1704164645`)
	f.Add(`builtins`, `null`)
	f.Add(`$__loc__`, `null`)
	f.Add(`leaf_paths`, `{"a":[1,true,{"b":"x"}],"c":null}`)
	f.Add(`hypot(3;4)`, `null`)
	f.Add(`fma(2;3;4)`, `null`)
	f.Fuzz(func(t *testing.T, query, input string) {
		p, err := Compile(query)
		if err != nil {
			return // invalid query — compile errors are expected, panics are not
		}
		p.Run([]byte(input)) // must not panic
	})
}

// FuzzRunFuncCallbackError checks the RunFunc contract that no other target
// exercises: an error returned by the callback reaches the caller unchanged, and
// the callback is never invoked again afterwards. The other targets pass a
// nil-returning callback (or none at all), so an unwind path that drops a
// caller's error looks correct to them.
//
// Queries are fixed and the input is fuzzed, like FuzzRunFixed — the query
// string itself is already covered by FuzzCompile and FuzzBoth.
//
// The seed corpus passes. Running with -fuzz currently reproduces #39 and #40,
// two pre-existing panics that predate this target and are unrelated to it.
func FuzzRunFuncCallbackError(f *testing.F) {
	// Multi-output queries only: a single-output query cannot expose a drop.
	// The try nestings matter — each one wraps the signal on its way out, so an
	// iterator two try bodies deep sees it wrapped twice.
	queries := []string{
		`.[]`, `.[]?`, `.[] | .`, `.[] | .a`, `.[] | .a?`, `.[] | .[]`,
		`.a[], .b[]`, `.a, .b`, `(.[])?`,
		`try .[] catch .`,
		`try (try .[] catch 1) catch 2`,
		`try (try (try .[] catch 1) catch 2) catch 3`,
		`try (.[] | try .a catch 1) catch 2`,
		`try (.[] | error("boom")) catch .`,
		`limit(3; .[])`, `first(.[])`, `nth(1; .[])`,
		`range(3)`, `range(3) | . + 1`,
		`(repeat(.))?`,
		`while(. < 5; . + 1)`,
		`foreach .[] as $x (0; . + $x)`,
		`label $out | .[] | if . > 2 then break $out else . end`,
		`..`, `.. | scalars`, `paths`, `tostream`,
		`.[] as [$a] | $a`, `.[] // 99`,
		`to_entries[]`, `keys[]`, `.[] | tostring`,
	}
	programs := make([]*Program, 0, len(queries))
	compiled := make([]string, 0, len(queries))
	for _, q := range queries {
		p, err := Compile(q)
		if err == nil {
			programs = append(programs, p)
			compiled = append(compiled, q)
		}
	}

	inputs := []string{
		`[1,2,3]`, `[1,2,3,4,5]`, `{"a":1,"b":2}`, `{"a":[1,2],"b":3}`,
		`[{"a":1},42,{"a":3}]`, `[[1,2],[3,4]]`, `[]`, `{}`, `null`, `0`,
		`[null,false,0,"",[],{}]`, `{"a":[1,2],"b":[3,4]}`,
		// Malformed, so the engine raises its own errors alongside ours.
		`[1,2,`, `{"a":`, `[1,{"a":2},`,
	}
	for _, in := range inputs {
		f.Add(in, 1)
		f.Add(in, 2)
	}

	errStop := errors.New("stop")
	f.Fuzz(func(t *testing.T, input string, stopAt int) {
		if stopAt < 1 || stopAt > 8 {
			return // keep the stream bounded; a large cap just burns time
		}
		for i, p := range programs {
			seen, after := 0, 0
			got := p.RunFunc([]byte(input), func([]byte) error {
				seen++
				if seen > stopAt {
					after++
				}
				if seen >= stopAt {
					return errStop
				}
				return nil
			})
			if seen < stopAt {
				continue // fewer outputs than the cap — the callback never failed
			}
			if after > 0 {
				t.Fatalf("callback ran %d more times after failing: %q over %q", after, compiled[i], input)
			}
			// Identity, not errors.Is: the caller must get its own error back.
			if got != errStop {
				t.Fatalf("RunFunc returned %v (%T), want the callback's error: %q over %q", got, got, compiled[i], input)
			}
		}
	})
}
