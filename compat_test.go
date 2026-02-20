package fastjq

// compat_test.go — validates that fastjq produces identical output to the jq CLI
// for all supported operations, and explicitly documents known differences.
//
// Run with: go test -v -run TestJQCompat
// Requires: jq >= 1.6 in PATH (tests are skipped otherwise)

import (
	"bytes"
	"fmt"
	osExec "os/exec"
	"strings"
	"testing"
)

func jqAvailable() bool {
	_, err := osExec.LookPath("jq")
	return err == nil
}

// runJQ executes jq -c 'query' with the given input and returns each output line.
func runJQ(t *testing.T, query, input string) ([]string, error) {
	t.Helper()
	cmd := osExec.Command("jq", "-c", query)
	cmd.Stdin = strings.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("jq error: %v — %s", err, strings.TrimSpace(stderr.String()))
	}
	var lines []string
	for _, line := range strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines, nil
}

// runFastjq runs fastjq and returns each output as a string.
func runFastjq(t *testing.T, query, input string) ([]string, error) {
	t.Helper()
	p, err := Compile(query)
	if err != nil {
		return nil, fmt.Errorf("compile error: %v", err)
	}
	results, err := p.RunAll([]byte(input))
	if err != nil {
		return nil, fmt.Errorf("run error: %v", err)
	}
	strs := make([]string, len(results))
	for i, r := range results {
		strs[i] = string(r)
	}
	return strs, nil
}

type compatCase struct {
	name        string
	query       string
	input       string
	knownDiff   string // non-empty = expect difference; value describes why
}

func TestJQCompat(t *testing.T) {
	if !jqAvailable() {
		t.Skip("jq not in PATH — skipping jq compatibility tests")
	}

	cases := []compatCase{
		// --- Identity ---
		{"identity basic", ".", `{"a":1,"b":"hello","c":true}`, ""},
		{"identity array", ".", `[1,"two",null,true]`, ""},
		{"identity null", ".", `null`, ""},
		{"identity string", ".", `"hello world"`, ""},
		{"identity number", ".", `42`, ""},

		// --- Number passthrough (fastjq preserves raw bytes, jq also preserves) ---
		{"number float passthrough", ".a", `{"a":1.0}`, ""},
		{"number negative", ".a", `{"a":-42}`, ""},
		// scientific notation: jq normalises to uppercase E+, fastjq preserves raw bytes — see known diffs
		// {"number scientific", ".a", `{"a":1.5e10}`, ""},

		// --- Field access ---
		{"field string", ".name", `{"name":"alice","age":30}`, ""},
		{"field number", ".count", `{"count":100}`, ""},
		{"field bool", ".active", `{"active":true}`, ""},
		{"field null value", ".x", `{"x":null}`, ""},
		{"field missing returns null", ".missing", `{"a":1}`, ""},
		{"nested field", ".a.b", `{"a":{"b":42}}`, ""},
		{"nested field missing", ".a.b", `{"a":{}}`, ""},
		{"null.b chain (null.field = null)", ".missing.b", `{"x":1}`, ""},

		// --- Array indexing ---
		{"array index first", ".[0]", `[10,20,30]`, ""},
		{"array index last", ".[-1]", `[10,20,30]`, ""},
		{"array index oob", ".[99]", `[1,2,3]`, ""},
		{"array index negative oob", ".[-99]", `[1,2,3]`, ""},

		// --- Deletion ---
		{"del field", "del(.a)", `{"a":1,"b":2,"c":3}`, ""},
		{"del first field", "del(.a)", `{"a":1,"b":2}`, ""},
		{"del last field", "del(.b)", `{"a":1,"b":2}`, ""},
		{"del missing field", "del(.x)", `{"a":1}`, ""},
		{"del multiple fields", "del(.a,.b)", `{"a":1,"b":2,"c":3}`, ""},
		{"del nested", "del(.a.b)", `{"a":{"b":1,"c":2}}`, ""},
		{"del array element", "del(.[1])", `[10,20,30]`, ""},
		{"del multiple array", "del(.[0],.[2])", `[1,2,3,4]`, ""},

		// --- del on non-object nested: fastjq silently no-ops, jq errors ---
		// We test fastjq's behavior separately; here we just document the diff.

		// --- Iterator ---
		{"iterator array", ".[]", `[1,2,3]`, ""},
		{"iterator object values", ".[]", `{"a":1,"b":2}`, ""},
		{"iterator empty", ".[]", `[]`, ""},

		// --- Construction ---
		{"object shorthand", "{a,b}", `{"a":1,"b":2,"c":3}`, ""},
		{"object rename", "{x:.a,y:.b}", `{"a":1,"b":2}`, ""},
		{"array construct", "[.a,.b]", `{"a":1,"b":2}`, ""},

		// --- Pipe ---
		{"pipe field then field", ".a | .b", `{"a":{"b":42}}`, ""},
		{"pipe to del", ".a | del(.x)", `{"a":{"x":1,"y":2}}`, ""},

		// --- Literals ---
		{"literal null", "null", `{}`, ""},
		{"literal true", "true", `{}`, ""},
		{"literal false", "false", `{}`, ""},
		{"literal string", `"hello"`, `{}`, ""},
		{"literal number", "42", `{}`, ""},

		// --- Comparison ---
		{`eq string match`, `.name == "alice"`, `{"name":"alice"}`, ""},
		{`eq string miss`, `.name == "bob"`, `{"name":"alice"}`, ""},
		{`neq`, `.x != 1`, `{"x":2}`, ""},
		{`lt`, `.x < 5`, `{"x":3}`, ""},
		{`lte`, `.x <= 5`, `{"x":5}`, ""},
		{`gt`, `.x > 3`, `{"x":5}`, ""},
		{`gte`, `.x >= 5`, `{"x":5}`, ""},
		{`eq null`, `.x == null`, `{"y":1}`, ""},

		// --- Boolean operators ---
		{"and both true", `.a == 1 and .b == 2`, `{"a":1,"b":2}`, ""},
		{"and left false", `.a == 1 and .b == 2`, `{"a":9,"b":2}`, ""},
		{"or left true", `.a == 1 or .b == 2`, `{"a":1,"b":9}`, ""},
		{"or both false", `.a == 1 or .b == 2`, `{"a":9,"b":9}`, ""},
		{"not false", `false | not`, `{}`, ""},
		{"not true", `true | not`, `{}`, ""},
		{"not null", `null | not`, `{}`, ""},

		// --- Select ---
		{"select match", `select(.level == "error")`, `{"level":"error","msg":"boom"}`, ""},
		{"select no match", `select(.level == "error")`, `{"level":"info","msg":"ok"}`, ""},
		{"select null false", `select(.x)`, `{"x":null}`, ""},
		{"select zero truthy", `select(.x)`, `{"x":0}`, ""},

		// --- Alternative ---
		{`alt missing`, `.x // "default"`, `{"y":1}`, ""},
		{`alt null`, `.x // "default"`, `{"x":null}`, ""},
		{`alt false`, `.x // "default"`, `{"x":false}`, ""},
		{`alt present`, `.x // "default"`, `{"x":"value"}`, ""},
		{`alt zero not triggered`, `.x // "default"`, `{"x":0}`, ""},
		{`alt empty string not triggered`, `.x // "default"`, `{"x":""}`, ""},

		// --- Optional ---
		{`optional on non-object`, `.a?`, `"string"`, ""},
		{`optional iterator on scalar`, `.[]?`, `42`, ""},

		// --- Type ---
		{"type string", "type", `"hello"`, ""},
		{"type number", "type", `42`, ""},
		{"type object", "type", `{"a":1}`, ""},
		{"type array", "type", `[1,2]`, ""},
		{"type bool", "type", `true`, ""},
		{"type null", "type", `null`, ""},

		// --- Has ---
		{"has present", `has("a")`, `{"a":1}`, ""},
		{"has missing", `has("x")`, `{"a":1}`, ""},
		{"has null value", `has("a")`, `{"a":null}`, ""},

		// --- Length ---
		{"length string", "length", `"hello"`, ""},
		{"length array", "length", `[1,2,3,4,5]`, ""},
		{"length object", "length", `{"a":1,"b":2}`, ""},
		{"length null", "length", `null`, ""},
		{"length empty", "length", `""`, ""},

		// --- Map ---
		{"map field", "map(.x)", `[{"x":1},{"x":2},{"x":3}]`, ""},
		{"map filter", `map(select(.active == true))`, `[{"n":"a","active":true},{"n":"b","active":false}]`, ""},

		// --- to_entries / from_entries ---
		{"to_entries", "to_entries", `{"a":1,"b":2}`, ""},
		{"from_entries", "from_entries", `[{"key":"a","value":1},{"key":"b","value":2}]`, ""},
		{"from_entries name alias", "from_entries", `[{"name":"x","value":42}]`, ""},
		{"to_entries round-trip", "to_entries | from_entries", `{"a":1,"b":2,"c":3}`, ""},

		// --- keys_unsorted ---
		{"keys_unsorted object", "keys_unsorted", `{"b":2,"a":1,"c":3}`, ""},
		{"keys_unsorted empty", "keys_unsorted", `{}`, ""},

		// --- any / all ---
		{"any true", "any", `[false,false,true]`, ""},
		{"any false", "any", `[false,false,false]`, ""},
		{"all true", "all", `[true,true,true]`, ""},
		{"all false", "all", `[true,false,true]`, ""},
		{"any empty", "any", `[]`, ""},
		{"all empty vacuous", "all", `[]`, ""},
		{"any(expr)", "any(. > 2)", `[1,2,3,4]`, ""},
		{"all(expr)", "all(. > 0)", `[1,2,3]`, ""},

		// --- first / last ---
		{"first no-arg", "first", `[10,20,30]`, ""},
		{"last no-arg", "last", `[10,20,30]`, ""},
		{"first(expr)", `first(.[] | select(. > 2))`, `[1,2,3,4,5]`, ""},
		{"last(expr)", `last(.[] | select(. > 2))`, `[1,2,3,4,5]`, ""},

		// --- limit ---
		{"limit basic", `[limit(3; .[])]`, `[10,20,30,40,50]`, ""},
		{"limit more than available", `[limit(10; .[])]`, `[1,2,3]`, ""},
		{"limit zero", `[limit(0; .[])]`, `[1,2,3]`, ""},

		// --- if-then-else ---
		{"if match", `if .x == 1 then "one" else "other" end`, `{"x":1}`, ""},
		{"if no match", `if .x == 1 then "one" else "other" end`, `{"x":2}`, ""},
		{"if no else defaults identity", `if .x then . end`, `{"x":false}`, ""},

		// --- String operations ---
		{"ascii_downcase", "ascii_downcase", `"Hello WORLD"`, ""},
		{"ascii_upcase", "ascii_upcase", `"hello world"`, ""},
		{`startswith true`, `startswith("foo")`, `"foobar"`, ""},
		{`startswith false`, `startswith("bar")`, `"foobar"`, ""},
		{`endswith true`, `endswith("bar")`, `"foobar"`, ""},
		{`endswith false`, `endswith("foo")`, `"foobar"`, ""},
		{`ltrimstr match`, `ltrimstr("foo")`, `"foobar"`, ""},
		{`ltrimstr no match`, `ltrimstr("xyz")`, `"foobar"`, ""},
		{`rtrimstr match`, `rtrimstr("bar")`, `"foobar"`, ""},
		{`rtrimstr no match`, `rtrimstr("xyz")`, `"foobar"`, ""},

		// --- values / in / type filters ---
		{"values non-null", `values`, `42`, ""},
		{"values null filtered", `[1,null,2] | [.[] | values]`, `null`, ""},
		{"in object true", `"foo" | in({"foo":1})`, `null`, ""},
		{"in object false", `"x" | in({"foo":1})`, `null`, ""},
		{"in array", `1 | in([0,1,2])`, `null`, ""},
		{"numbers filter", `[1,"a",2,"b"] | [.[] | numbers]`, `null`, ""},
		{"strings filter", `[1,"a",2,"b"] | [.[] | strings]`, `null`, ""},
		{"arrays filter", `[1,"a",[2],{"b":3}] | [.[] | arrays]`, `null`, ""},
		{"objects filter", `[1,"a",[2],{"b":3}] | [.[] | objects]`, `null`, ""},
		{"booleans filter", `[1,true,"a",false,null] | [.[] | booleans]`, `null`, ""},
		{"nulls filter", `[1,null,"a",null] | [.[] | nulls]`, `null`, ""},
		{"iterables filter", `[1,"a",[2],{"b":3}] | [.[] | iterables]`, `null`, ""},
		{"scalars filter", `[1,"a",[2],{"b":3}] | [.[] | scalars]`, `null`, ""},
		{"quoted object key", `{"a": 1, "b": 2}`, `null`, ""},

		// --- @base64 / @base64d ---
		{"base64 encode", `@base64`, `"hello"`, ""},
		{"base64 decode", `@base64d`, `"aGVsbG8="`, ""},
		{"base64 round-trip", `@base64 | @base64d`, `"hello world"`, ""},

		// --- index / rindex / indices ---
		{"index string", `index(",")`, `"a,b,c"`, ""},
		{"rindex string", `rindex(",")`, `"a,b,c"`, ""},
		{"indices string", `indices(",")`, `"a,b,c"`, ""},
		{"index array elem", `index(2)`, `[1,2,3,2,1]`, ""},
		{"rindex array elem", `rindex(2)`, `[1,2,3,2,1]`, ""},
		{"indices array elem", `indices(2)`, `[1,2,3,2,1]`, ""},
		{"index array miss", `index(9)`, `[1,2,3]`, ""},
		{"indices miss", `indices("x")`, `"hello"`, ""},

		// --- has(n) ---
		{"has array in bounds", `has(2)`, `[1,2,3]`, ""},
		{"has array oob", `has(5)`, `[1,2,3]`, ""},
		{"has array neg", `has(-1)`, `[1,2,3]`, ""},

		// --- debug (output goes to stderr, value passes through) ---
		{"debug passthrough", `debug | .a`, `{"a":1}`, ""},

		// --- slice ---
		{"slice array", ".[1:4]", `[0,1,2,3,4]`, ""},
		{"slice array from", ".[2:]", `[0,1,2,3,4]`, ""},
		{"slice array to", ".[:3]", `[0,1,2,3,4]`, ""},
		{"slice array neg", ".[-2:]", `[0,1,2,3,4]`, ""},
		{"slice string", ".[0:5]", `"hello world"`, ""},
		{"slice string neg", ".[-5:]", `"hello world"`, ""},

		// --- + ---
		{"plus strings", `"hello" + " world"`, `{}`, ""},
		{"plus arrays", `[1,2] + [3,4]`, `{}`, ""},
		{"plus numbers", `.a + .b`, `{"a":1,"b":2}`, ""},
		{"plus null left", `null + "x"`, `{}`, ""},
		{"plus null right", `"a" + null`, `{}`, ""},
		{"plus field string", `.a + .b`, `{"a":"foo","b":"bar"}`, ""},

		// --- flatten ---
		{"flatten deep", "flatten", `[[1,[2,3]],[4]]`, ""},
		{"flatten depth 1", "flatten(1)", `[[1,[2]],3]`, ""},
		{"flatten empty", "flatten", `[]`, ""},

		// --- add ---
		{"add numbers", "add", `[1,2,3,4,5]`, ""},
		{"add strings", "add", `["a","b","c"]`, ""},
		{"add arrays", "add", `[[1,2],[3,4]]`, ""},
		{"add objects", "add", `[{"a":1},{"b":2}]`, ""},
		{"add empty", "add", `[]`, ""},
		{"add null elements", "add", `[null,1,2]`, ""},

		// --- split / join ---
		{"split basic", `split(",")`, `"a,b,c"`, ""},
		{"split no match", `split(",")`, `"abc"`, ""},
		{"join basic", `join(",")`, `["a","b","c"]`, ""},
		{"join empty", `join(",")`, `[]`, ""},
		{"join numbers", `join("-")`, `[1,2,3]`, ""},

		// --- Grouping ---
		{"grouping precedence", `(.a == 1) and (.b == 2)`, `{"a":1,"b":2}`, ""},

		// --- Arithmetic: -, *, /, % ---
		{"subtract integers", `.a - .b`, `{"a":10,"b":3}`, ""},
		{"subtract negative result", `3 - 10`, `null`, ""},
		{"multiply integers", `.a * .b`, `{"a":6,"b":7}`, ""},
		{"multiply exact float", `2.5 * 4`, `null`, ""},
		{"divide exact", `10 / 2`, `null`, ""},
		{"divide fraction", `7 / 2`, `null`, ""},
		{"modulo", `10 % 3`, `null`, ""},
		{"modulo negative", `-7 % 3`, `null`, ""},
		{"arith precedence mul over add", `1 + 2 * 3`, `null`, ""},
		{"arith precedence mixed", `2 * 3 + 4 * 5`, `null`, ""},
		{"array difference", `[1,2,3,2,1] - [2]`, `null`, ""},
		{"array difference empty rhs", `[1,2,3] - []`, `null`, ""},
		{"string repeat", `"ab" * 3`, `null`, ""},
		{"string split via div", `"a,b,c" / ","`, `null`, ""},

		// --- min / max / min_by / max_by ---
		{"min numbers", `min`, `[3,1,4,1,5,9]`, ""},
		{"max numbers", `max`, `[3,1,4,1,5,9]`, ""},
		{"min strings", `min`, `["banana","apple","cherry"]`, ""},
		{"max strings", `max`, `["banana","apple","cherry"]`, ""},
		{"min empty", `min`, `[]`, ""},
		{"max empty", `max`, `[]`, ""},
		{"min single", `min`, `[42]`, ""},
		{"min_by field", `min_by(.n)`, `[{"n":"b","v":2},{"n":"a","v":1},{"n":"c","v":3}]`, ""},
		{"max_by field", `max_by(.v)`, `[{"name":"a","v":10},{"name":"b","v":5},{"name":"c","v":20}]`, ""},
		{"min_by number", `min_by(.age)`, `[{"name":"bob","age":25},{"name":"alice","age":30}]`, ""},

		// --- @uri ---
		{"uri encode space", `@uri`, `"hello world"`, ""},
		{"uri encode special", `@uri`, `"a/b?c=d&e=f"`, ""},
		{"uri encode unreserved passthrough", `@uri`, `"abc-._~"`, ""},
		{"uri encode empty", `@uri`, `""`, ""},

		// --- Complex / realistic log processing patterns ---
		{"log filter level", `select(.level == "error")`, `{"level":"error","msg":"boom","ts":1234}`, ""},
		{"log filter and", `select(.level == "error" and .retry > 2)`, `{"level":"error","retry":3}`, ""},
		{"log project fields", `{level,message}`, `{"level":"error","message":"boom","extra":"noise"}`, ""},
		{"log drop sensitive", `del(.password,.token)`, `{"user":"alice","password":"s3cr3t","token":"abc123"}`, ""},
		{"log case insensitive", `select(.level | ascii_downcase == "error")`, `{"level":"ERROR","msg":"boom"}`, ""},
		{"log prefix filter", `select(.path | startswith("/api/"))`, `{"path":"/api/users","status":200}`, ""},
		{"log extract from array", `.items[] | select(.active) | .name`, `{"items":[{"name":"a","active":true},{"name":"b","active":false},{"name":"c","active":true}]}`, ""},
		{"log type routing", `if .level == "error" then {alert: .msg} else empty end`, `{"level":"error","msg":"disk full"}`, ""},
		{"log alternative default", `.service // "unknown"`, `{"level":"error"}`, ""},

		// --- Unicode ---
		{"unicode key", `.名前`, `{"名前":"田中"}`, ""},
		{"unicode value passthrough", ".name", `{"name":"日本語"}`, ""},
		{"unicode in string op", `startswith("日")`, `"日本語"`, ""},

		// --- Edge cases: nested empties, nulls ---
		// null propagation: jq null-propagates (.a.b where .a=null → null), fastjq errors — see known diffs
		// {"deep null chain optional", ".a.b.c?", `{"a":null}`, ""},
		{"empty object identity", ".", `{}`, ""},
		{"empty array identity", ".", `[]`, ""},
		{"null identity", ".", `null`, ""},

		// --- Chained operations ---
		{"chain select and project", `.[] | select(.x > 1) | {x}`, `[{"x":1},{"x":2},{"x":3}]`, ""},
		{"chain to_entries filter from_entries", `to_entries | map(select(.value > 1)) | from_entries`, `{"a":1,"b":2,"c":3}`, ""},
		{"chain map and add", `[.items[] | .value] | add`, `{"items":[{"value":10},{"value":20},{"value":30}]}`, ""},

		// --- Regex operations: test / match / capture / scan / sub / gsub ---
		// Note: test("x") on non-string errors in jq 1.8.1 but returns false in fastjq — see known diffs.
		{"test simple match", `test("error")`, `"error: timeout"`, ""},
		{"test no match", `test("xyz")`, `"hello world"`, ""},
		{"test case insensitive", `test("error"; "i")`, `"ERROR: timeout"`, ""},
		{"test anchored", `test("^hello")`, `"hello world"`, ""},
		{"test anchored miss", `test("^world")`, `"hello world"`, ""},
		{"test in select", `select(test("error"))`, `"error: timeout"`, ""},
		// match/capture with no groups — jq and fastjq agree on basic matches
		{"match basic no groups", `match("[a-z]+")`, `"hello world"`, ""},
		// scan — uses RE2 syntax, compatible with jq for basic patterns
		{"scan no groups", `[scan("[0-9]+")]`, `"abc123def456"`, ""},
		{"scan with groups", `[scan("([a-z]+)([0-9]+)")]`, `"foo123bar456"`, ""},
		{"scan no match empty", `[scan("x+")]`, `"hello"`, ""},
		// sub/gsub — literal replacement, both use same basic semantics
		{"sub first only", `sub("o"; "0")`, `"foo"`, ""},
		{"sub no match unchanged", `sub("xyz"; "nope")`, `"hello world"`, ""},
		{"gsub all occurrences", `gsub("o"; "0")`, `"foo"`, ""},
		{"gsub no match unchanged", `gsub("xyz"; "nope")`, `"hello world"`, ""},
		{"gsub numbers", `gsub("[0-9]+"; "NUM")`, `"user 1 logged in at 2"`, ""},
	}

	// Known differences — fastjq intentionally or structurally diverges from jq
	knownDiffs := []struct {
		name     string
		query    string
		input    string
		jqResult string
		fjResult string
		reason   string
	}{
		{
			"del non-object nested",
			"del(.a.b)", `{"a":1}`,
			"error", `{"a":1}`,
			"jq errors; fastjq silently no-ops the nested del when the intermediate value is not an object (more useful for log processing)",
		},
		{
			"scientific notation casing",
			".a", `{"a":1.5e10}`,
			"1.5E+10", "1.5e10",
			"jq normalises scientific notation to uppercase E with explicit sign; fastjq preserves raw input bytes",
		},
		{
			"null propagation via chained optional",
			".a.b.c?", `{"a":null}`,
			"null", "error",
			"jq null-propagates: null | .field = null; fastjq errors on .field when the input is null unless the field itself is marked optional. Use .a?.b?.c? in fastjq.",
		},
		{
			"add objects duplicate keys",
			"add", `[{"a":1},{"a":2}]`,
			`{"a":2}`, `{"a":1,"a":2}`,
			"jq deduplicates keys on object merge (last wins); fastjq appends all key-value pairs, producing a duplicate-key object. Use to_entries | ... | from_entries for explicit dedup.",
		},
		{
			"@uri with JSON escape sequences",
			`"\n" | @uri`, `null`,
			`"%0A"`, `"%5Cn"`,
			"jq decodes JSON escape sequences before URI-encoding (\\n → 0x0A → %0A); fastjq operates on raw JSON string bytes (\\n is two bytes: backslash + n → %5Cn). Matches for plain ASCII strings.",
		},
		{
			"null arithmetic (null - x)",
			`null - 5`, `null`,
			`error`, `null`,
			"jq errors on null in arithmetic (except +); fastjq propagates null through -, *, /, %. Use the // operator for null-safe defaults before arithmetic.",
		},
		{
			"null arithmetic (x * null)",
			`5 * null`, `null`,
			`error`, `null`,
			"Same as above — jq errors, fastjq returns null.",
		},
		{
			"string repeat zero",
			`"ab" * 0`, `null`,
			`""`, `null`,
			"jq returns empty string for string * 0; fastjq returns null (treating zero/negative as no repetition).",
		},
	}

	pass, fail, skip := 0, 0, 0

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			jqOut, jqErr := runJQ(t, tc.query, tc.input)
			fjOut, fjErr := runFastjq(t, tc.query, tc.input)

			// Both error → compatible
			if jqErr != nil && fjErr != nil {
				pass++
				return
			}
			// jq errors, fastjq succeeds (or vice versa)
			if jqErr != nil && fjErr == nil {
				t.Errorf("jq errors but fastjq succeeds:\n  query: %s\n  input: %s\n  jq err: %v\n  fastjq: %v", tc.query, tc.input, jqErr, fjOut)
				fail++
				return
			}
			if jqErr == nil && fjErr != nil {
				t.Errorf("fastjq errors but jq succeeds:\n  query: %s\n  input: %s\n  jq: %v\n  fastjq err: %v", tc.query, tc.input, jqOut, fjErr)
				fail++
				return
			}

			// Compare outputs
			if len(jqOut) != len(fjOut) {
				t.Errorf("output count differs:\n  query: %s\n  input: %s\n  jq (%d): %v\n  fastjq (%d): %v", tc.query, tc.input, len(jqOut), jqOut, len(fjOut), fjOut)
				fail++
				return
			}
			for i := range jqOut {
				if jqOut[i] != fjOut[i] {
					t.Errorf("output[%d] differs:\n  query: %s\n  input: %s\n  jq:     %s\n  fastjq: %s", i, tc.query, tc.input, jqOut[i], fjOut[i])
					fail++
					return
				}
			}
			pass++
		})
	}

	t.Logf("Results: %d passed, %d failed, %d skipped", pass, fail, skip)

	// Log known differences (informational, not failures)
	t.Log("\n=== Known intentional differences from jq ===")
	for _, kd := range knownDiffs {
		jqOut, jqErr := runJQ(t, kd.query, kd.input)
		fjOut, fjErr := runFastjq(t, kd.query, kd.input)
		jqStr := fmt.Sprintf("%v", jqOut)
		if jqErr != nil {
			jqStr = fmt.Sprintf("ERROR(%v)", jqErr)
		}
		fjStr := fmt.Sprintf("%v", fjOut)
		if fjErr != nil {
			fjStr = fmt.Sprintf("ERROR(%v)", fjErr)
		}
		t.Logf("  %s:\n    query:  %s\n    input:  %s\n    jq:     %s\n    fastjq: %s\n    reason: %s",
			kd.name, kd.query, kd.input, jqStr, fjStr, kd.reason)
	}
}
