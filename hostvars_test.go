// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License 2.0.
//
// This product includes software developed at Datadog (https://www.datadoghq.com/)
// Copyright 2026-Present Datadog, Inc.

package fastjq

import (
	"errors"
	"strings"
	"testing"
)

func vals(vs ...string) [][]byte {
	out := make([][]byte, len(vs))
	for i, v := range vs {
		out[i] = []byte(v)
	}
	return out
}

// Expected outputs are what `jq -c --argjson name value query` prints for the
// same query, input and values. Values are embedded as written, so whitespace
// inside a value is kept where jq would compact it.
func TestWithVariablesParity(t *testing.T) {
	tests := []struct {
		name  string
		query string
		vars  []string
		input string
		vals  []string
		want  string
	}{
		{"scalar", `$x`, []string{"$x"}, `null`, []string{`42`}, `42`},
		{"string", `$s`, []string{"$s"}, `null`, []string{`"hi"`}, `"hi"`},
		{"object field", `$o.a`, []string{"$o"}, `null`, []string{`{"a":[1,2]}`}, `[1,2]`},
		{"object index", `$o["a"]`, []string{"$o"}, `null`, []string{`{"a":7}`}, `7`},
		{"array index", `$a[1]`, []string{"$a"}, `null`, []string{`[10,20,30]`}, `20`},
		{"two values in order", `[$a, $b]`, []string{"$a", "$b"}, `null`, []string{`1`, `"two"`}, `[1,"two"]`},
		{"mixed with input", `.name + $suffix`, []string{"$suffix"}, `{"name":"bob"}`, []string{`"!"`}, `"bob!"`},
		{"in select", `select(.k == $want) | .v`, []string{"$want"}, `{"k":"a","v":1}`, []string{`"a"`}, `1`},
		{"in object construction", `{id: .id, key: $key}`, []string{"$key"}, `{"id":3}`, []string{`"k"`}, `{"id":3,"key":"k"}`},
		{"shorthand construction", `{$a, b: .b}`, []string{"$a"}, `{"b":2}`, []string{`1`}, `{"a":1,"b":2}`},
		{"in comparison", `. > $limit`, []string{"$limit"}, `5`, []string{`3`}, `true`},
		{"in arithmetic", `. * $factor`, []string{"$factor"}, `4`, []string{`2.5`}, `10`},
		{"inside map", `map(. + $n)`, []string{"$n"}, `[1,2,3]`, []string{`10`}, `[11,12,13]`},
		{"inside reduce", `reduce .[] as $i (0; . + $i * $k)`, []string{"$k"}, `[1,2,3]`, []string{`2`}, `12`},
		{"inside user function", `def add_n: . + $n; add_n`, []string{"$n"}, `1`, []string{`5`}, `6`},
		{"inside function closure", `def f(g): g; f(. + $n)`, []string{"$n"}, `1`, []string{`5`}, `6`},
		{"in string interpolation", `"hello \($who)"`, []string{"$who"}, `null`, []string{`"world"`}, `"hello world"`},
		{"as destructuring source", `$o as {a: $a} | $a`, []string{"$o"}, `null`, []string{`{"a":9}`}, `9`},
		{"surrounding whitespace", `$x`, []string{"$x"}, `null`, []string{" \n[1, 2]\t"}, `[1, 2]`},
		{"underscore and digits", `$api_key2`, []string{"$api_key2"}, `null`, []string{`"s3cret"`}, `"s3cret"`},
		{"declared but unused", `.a`, []string{"$unused"}, `{"a":1}`, []string{`1`}, `1`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := Compile(tt.query, WithVariables(tt.vars...))
			if err != nil {
				t.Fatalf("Compile: %v", err)
			}
			got, err := p.Run([]byte(tt.input), vals(tt.vals...)...)
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("Run = %s, want %s", got, tt.want)
			}
			buf := make([]byte, 0, 64)
			got, err = p.RunWithBuffer([]byte(tt.input), buf, vals(tt.vals...)...)
			if err != nil {
				t.Fatalf("RunWithBuffer: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("RunWithBuffer = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestWithVariablesMultiOutput(t *testing.T) {
	p, err := Compile(`.[] | . + $n`, WithVariables("$n"))
	if err != nil {
		t.Fatal(err)
	}
	in := []byte(`[1,2,3]`)
	n := vals(`100`)

	all, err := p.RunAll(in, n...)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range all {
		got = append(got, string(r))
	}
	if strings.Join(got, ",") != "101,102,103" {
		t.Errorf("RunAll = %v", got)
	}

	got = nil
	err = p.RunFunc(in, func(r []byte) error {
		got = append(got, string(r))
		return nil
	}, n...)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "101,102,103" {
		t.Errorf("RunFunc = %v", got)
	}
}

func TestWithVariablesShadowing(t *testing.T) {
	// A binding inside the query wins over the host value, like a nested
	// scope in jq, and the host value is visible again outside it.
	p, err := Compile(`[(1 as $x | $x), $x]`, WithVariables("$x"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Run([]byte(`null`), []byte(`"host"`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `[1,"host"]` {
		t.Errorf("got %s", got)
	}
}

func TestWithVariablesValueCount(t *testing.T) {
	p, err := Compile(`[$a, $b]`, WithVariables("$a", "$b"))
	if err != nil {
		t.Fatal(err)
	}
	in := []byte(`null`)
	for _, v := range [][][]byte{nil, vals(`1`), vals(`1`, `2`, `3`)} {
		if _, err := p.Run(in, v...); err == nil {
			t.Errorf("Run with %d values: want an error", len(v))
		}
		if _, err := p.RunWithBuffer(in, nil, v...); err == nil {
			t.Errorf("RunWithBuffer with %d values: want an error", len(v))
		}
		if _, err := p.RunAll(in, v...); err == nil {
			t.Errorf("RunAll with %d values: want an error", len(v))
		}
		called := false
		err := p.RunFunc(in, func([]byte) error { called = true; return nil }, v...)
		if err == nil || called {
			t.Errorf("RunFunc with %d values: err=%v called=%v", len(v), err, called)
		}
	}
	// A program with no declared variables rejects stray values too, so a
	// forgotten WithVariables does not go unnoticed.
	plain, err := Compile(`.`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plain.Run(in, []byte(`1`)); err == nil {
		t.Error("Run with a value but no declared variables: want an error")
	}
}

func TestWithVariablesEmptyValue(t *testing.T) {
	p, err := Compile(`$x`, WithVariables("$x"))
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"", "  \n"} {
		_, err := p.Run([]byte(`null`), []byte(v))
		if err == nil || !strings.Contains(err.Error(), "$x") {
			t.Errorf("value %q: err = %v, want an error naming $x", v, err)
		}
	}
}

func TestWithVariablesInvalidNames(t *testing.T) {
	bad := []string{
		"x",           // missing '$'
		"$",           // empty
		"$1x",         // starts with a digit
		"$a-b",        // not an identifier
		"$a b",        // whitespace
		"$a.b",        // field access
		"$ x",         // space after sigil
		"$__loc__",    // built-in
		"$ENV",        // built-in
		"$\"quoted\"", // quoting is not part of the name
	}
	for _, name := range bad {
		if _, err := Compile(`.`, WithVariables(name)); err == nil {
			t.Errorf("WithVariables(%q): want an error", name)
		}
	}
	if _, err := Compile(`.`, WithVariables("$a", "$b", "$a")); err == nil {
		t.Error("duplicate name: want an error")
	}
	if _, err := Compile(`.`, WithVariables("$a"), WithVariables("$a")); err == nil {
		t.Error("duplicate name across options: want an error")
	}
	// Valid names that look close to the reserved ones.
	for _, name := range []string{"$_", "$_x", "$ENVIRON", "$env", "$loc"} {
		if _, err := Compile(`.`, WithVariables(name)); err != nil {
			t.Errorf("WithVariables(%q): %v", name, err)
		}
	}
}

func TestWithVariablesUndeclaredStillFailsAtCompile(t *testing.T) {
	_, err := Compile(`$a + $b`, WithVariables("$a"))
	if err == nil || !strings.Contains(err.Error(), "$b is not defined") {
		t.Errorf("err = %v, want $b is not defined", err)
	}
	// Without the option the same reference is an error, as before.
	if _, err := Compile(`$a`); err == nil {
		t.Error("undeclared $a: want a compile error")
	}
}

func TestWithVariablesValuesStayOutOfCompileErrors(t *testing.T) {
	// The point of host variables: the secret never touches the query text, so
	// no parse error can echo it.
	const secret = "hunter2-token"
	p, err := Compile(`$k`, WithVariables("$k"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Run([]byte(`null`), []byte(`"`+secret+`"`)); err != nil {
		t.Fatal(err)
	}
	_, err = Compile(`$k | )`, WithVariables("$k"))
	if err == nil {
		t.Fatal("want a parse error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("compile error leaks the value: %v", err)
	}
	// Option errors name the variable, never a value.
	_, err = Compile(`.`, WithVariables("$bad name"))
	if err == nil || !strings.Contains(err.Error(), "$bad name") {
		t.Errorf("err = %v", err)
	}
}

func TestWithVariablesQueryTextIsNotAffectedByValues(t *testing.T) {
	// A value that looks like jq syntax is data, never parsed.
	p, err := Compile(`$x`, WithVariables("$x"))
	if err != nil {
		t.Fatal(err)
	}
	payload := `"\"} | error(\"pwned\") | {\"a\":\"\\(1+1)"`
	got, err := p.Run([]byte(`null`), []byte(payload))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != payload {
		t.Errorf("got %s, want it back unchanged", got)
	}
}

func TestWithVariablesRuntimeErrorsStillWork(t *testing.T) {
	p, err := Compile(`$x | error("boom")`, WithVariables("$x"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Run([]byte(`null`), []byte(`1`)); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err = %v", err)
	}
	stop := errors.New("stop")
	err = p.RunFunc([]byte(`null`), func([]byte) error { return stop }, []byte(`1`))
	if err == nil {
		t.Error("want the query error")
	}
}

func TestWithVariablesConcurrent(t *testing.T) {
	// Values are per call, never stored on the Program.
	p, err := Compile(`$x`, WithVariables("$x"))
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan string, 8)
	for i := 0; i < 8; i++ {
		go func(i int) {
			v := []byte{byte('0' + i)}
			var bad string
			for n := 0; n < 500; n++ {
				got, err := p.Run([]byte(`null`), v)
				if err != nil || string(got) != string(v) {
					bad = "mismatch"
					break
				}
			}
			done <- bad
		}(i)
	}
	for i := 0; i < 8; i++ {
		if bad := <-done; bad != "" {
			t.Error(bad)
		}
	}
}

func TestWithVariablesZeroAllocs(t *testing.T) {
	p, err := Compile(`{id: .id, key: $key, limit: $limit, next: (.id + $limit)}`, WithVariables("$key", "$limit"))
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{"id":1}`)
	// Built once and reused, the way a caller on a hot path would. Spelling the
	// values out at each call site makes the compiler allocate the variadic
	// slice.
	vs := vals(`"abc"`, `3`)
	buf := make([]byte, 0, 128)
	got := testing.AllocsPerRun(200, func() {
		buf, _ = p.RunWithBuffer(input, buf, vs...)
	})
	if got != 0 {
		t.Errorf("RunWithBuffer with host variables allocated %.0f times per run, want 0", got)
	}
	if want := `{"id":1,"key":"abc","limit":3,"next":4}`; string(buf) != want {
		t.Errorf("output = %s, want %s", buf, want)
	}

	// Programs without variables must not pay for the feature.
	plain, err := Compile(`.id`)
	if err != nil {
		t.Fatal(err)
	}
	got = testing.AllocsPerRun(200, func() {
		buf, _ = plain.RunWithBuffer(input, buf)
	})
	if got != 0 {
		t.Errorf("RunWithBuffer without variables allocated %.0f times per run, want 0", got)
	}

	multi, err := Compile(`.id, $key, $limit`, WithVariables("$key", "$limit"))
	if err != nil {
		t.Fatal(err)
	}
	sink := 0
	fn := func(r []byte) error { sink += len(r); return nil }
	got = testing.AllocsPerRun(200, func() {
		_ = multi.RunFunc(input, fn, vs...)
	})
	// 1 is RunFunc's own wrapper closure, same as without variables.
	if got > 1 {
		t.Errorf("RunFunc with host variables allocated %.0f times per run, want at most 1", got)
	}
}
