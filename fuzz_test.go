package fastjq

import "testing"

// FuzzCompile ensures no query string causes Compile to panic.
func FuzzCompile(f *testing.F) {
	seeds := []string{
		".", ".foo", ".foo.bar", ".[0]", ".[-1]", ".[]",
		"del(.foo)", "del(.foo, .bar)", "del(.[0])",
		`select(.x == "y")`, `select(.x != null)`,
		`select(.x > 0 and .y < 10)`,
		"{a, b}", "{a: .b}", "[.a, .b]",
		"map(.x)", "to_entries", "from_entries",
		`with_entries(select(.value != null))`,
		"length", "keys_unsorted", "type",
		"ascii_downcase", `startswith("foo")`,
		`ltrimstr("foo")`, `has("key")`,
		"any", "all", `any(. > 0)`, `all(. > 0)`,
		"first", "last", `first(.[] | select(. > 0))`,
		`limit(3; .[])`,
		`if .x == "y" then .a else .b end`,
		`.x // "default"`, "empty", "not",
		`true`, `false`, `null`, `42`, `"hello"`,
		// Edge cases in parsing
		`select(.a and .b or .c)`,
		`.a.b.c.d.e`,
		`with_entries(select(.key != "x" and .value != null))`,
		`if has("x") then .x else empty end`,
		`[limit(3; .[])]`,
		`any(ascii_downcase == "error")`,
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
		`"\u0000"`, `"\n\r\t\b\f\\\""`,
		`{"key with spaces": 1}`,
		`{"a":{"b":{"c":{"d":{"e":1}}}}}`,
		`[[[[[1]]]]]`,
	}
	for _, s := range seeds {
		f.Add(s)
	}

	queries := []string{
		".", ".a", ".a.b", ".[0]", ".[-1]", ".[]",
		"del(.a)", "del(.a, .b)", "del(.[0])",
		`select(.a == "x")`, `select(.a != null)`,
		`select(.a > 0)`, `select(has("a"))`,
		`select(.a | not)`, `select(any)`, `select(all)`,
		"{a}", "{a: .b}", "[.a, .b]",
		"map(.a)", "to_entries", "from_entries",
		`with_entries(select(.value != null))`,
		"length", "keys_unsorted", "type",
		"ascii_downcase", "ascii_upcase",
		`startswith("x")`, `endswith("x")`,
		`ltrimstr("x")`, `rtrimstr("x")`,
		"any", `any(. > 0)`, "all", `all(. > 0)`,
		"first", "last",
		`first(.[] | select(. > 0))`,
		`last(.[] | select(. > 0))`,
		`limit(3; .[])`,
		`if .a then .b else .c end`,
		`.a // "default"`,
		"empty", "not",
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
	f.Fuzz(func(t *testing.T, query, input string) {
		p, err := Compile(query)
		if err != nil {
			return // invalid query — compile errors are expected, panics are not
		}
		p.Run([]byte(input)) // must not panic
	})
}
