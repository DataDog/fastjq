package fastjq

import (
	"testing"
)

// assertQuery compiles query, runs it against input, and checks the result equals want.
// Reduces 8-line boilerplate to a single call for simple single-output tests.
func assertQuery(t *testing.T, query, input, want string) {
	t.Helper()
	p, err := Compile(query)
	if err != nil {
		t.Fatalf("Compile(%q): %v", query, err)
	}
	got, err := p.Run([]byte(input))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if string(got) != want {
		t.Errorf("query %q on %s: got %s, want %s", query, input, got, want)
	}
}

// assertNoOutput compiles query, runs it against input, and checks it produces zero outputs.
func assertNoOutput(t *testing.T, query, input string) {
	t.Helper()
	p, err := Compile(query)
	if err != nil {
		t.Fatalf("Compile(%q): %v", query, err)
	}
	results, err := p.RunAll([]byte(input))
	if err != nil {
		t.Fatalf("RunAll: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("query %q on %s: expected 0 results, got %d: %v", query, input, len(results), results)
	}
}

func TestIdentity(t *testing.T) {
	assertQuery(t, ".", `{"name":"alice","age":30}`, `{"name":"alice","age":30}`)
}
func TestFieldAccess(t *testing.T) {
	assertQuery(t, ".name", `{"name":"alice","age":30}`, `"alice"`)
}
func TestFieldAccessNumber(t *testing.T) {
	assertQuery(t, ".age", `{"name":"alice","age":30}`, "30")
}
func TestFieldAccessNested(t *testing.T) {
	assertQuery(t, ".address.city", `{"name":"alice","address":{"city":"NYC","zip":"10001"}}`, `"NYC"`)
}
func TestFieldAccessMissing(t *testing.T) {
	assertQuery(t, ".missing", `{"name":"alice"}`, "null")
}

func TestDeleteSingle(t *testing.T) {
	assertQuery(t, "del(.age)", `{"name":"alice","age":30,"city":"NYC"}`, `{"name":"alice","city":"NYC"}`)
}
func TestDeleteMultiple(t *testing.T) {
	assertQuery(t, "del(.age, .city)", `{"name":"alice","age":30,"city":"NYC"}`, `{"name":"alice"}`)
}
func TestDeleteFirst(t *testing.T) {
	assertQuery(t, "del(.name)", `{"name":"alice","age":30}`, `{"age":30}`)
}
func TestDeleteLast(t *testing.T) {
	assertQuery(t, "del(.age)", `{"name":"alice","age":30}`, `{"name":"alice"}`)
}
func TestDeleteAll(t *testing.T) {
	assertQuery(t, "del(.name, .age)", `{"name":"alice","age":30}`, `{}`)
}
func TestDeleteNested(t *testing.T) {
	assertQuery(t, "del(.address.zip)",
		`{"name":"alice","address":{"city":"NYC","zip":"10001"}}`,
		`{"name":"alice","address":{"city":"NYC"}}`)
}
func TestDeleteNonexistent(t *testing.T) {
	assertQuery(t, "del(.missing)", `{"name":"alice","age":30}`, `{"name":"alice","age":30}`)
}

func TestPipe(t *testing.T) {
	p, err := Compile(".address | del(.zip)")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{"name":"alice","address":{"city":"NYC","zip":"10001"}}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	expected := `{"city":"NYC"}`
	if string(got) != expected {
		t.Errorf("got %s, want %s", got, expected)
	}
}

func TestPipeIdentityOptimization(t *testing.T) {
	// `. | del(.foo)` should be simplified to `del(.foo)`
	p, err := Compile(". | del(.age)")
	if err != nil {
		t.Fatal(err)
	}
	if p.root.typ != opDelete {
		t.Errorf("expected opDelete after simplification, got %d", p.root.typ)
	}
	input := []byte(`{"name":"alice","age":30}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	expected := `{"name":"alice"}`
	if string(got) != expected {
		t.Errorf("got %s, want %s", got, expected)
	}
}

func TestRunWithBuffer(t *testing.T) {
	p, err := Compile("del(.age)")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{"name":"alice","age":30}`)
	buf := make([]byte, 0, 256)

	// First call
	got, err := p.RunWithBuffer(input, buf)
	if err != nil {
		t.Fatal(err)
	}
	expected := `{"name":"alice"}`
	if string(got) != expected {
		t.Errorf("got %s, want %s", got, expected)
	}

	// Second call reuses buffer
	got2, err := p.RunWithBuffer(input, got[:0])
	if err != nil {
		t.Fatal(err)
	}
	if string(got2) != expected {
		t.Errorf("got %s, want %s", got2, expected)
	}
}

func TestWhitespace(t *testing.T) {
	p, err := Compile("del(.age)")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`  {  "name" : "alice" , "age" : 30  }  `)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	expected := `{"name":"alice"}`
	if string(got) != expected {
		t.Errorf("got %s, want %s", got, expected)
	}
}

func TestEmptyObject(t *testing.T) {
	p, err := Compile("del(.foo)")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{}` {
		t.Errorf("got %s, want {}", got)
	}
}

func TestNestedObjects(t *testing.T) {
	p, err := Compile(".data")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{"data":{"users":[1,2,3]},"meta":{"page":1}}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"users":[1,2,3]}` {
		t.Errorf("got %s, want %s", got, `{"users":[1,2,3]}`)
	}
}

func TestFieldAccessBool(t *testing.T) {
	p, err := Compile(".active")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{"active":true,"name":"alice"}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "true" {
		t.Errorf("got %s, want true", got)
	}
}

func TestFieldAccessNull(t *testing.T) {
	p, err := Compile(".value")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{"value":null,"other":"x"}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "null" {
		t.Errorf("got %s, want null", got)
	}
}

func TestFieldAccessArray(t *testing.T) {
	p, err := Compile(".items")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{"items":[1,"two",true,null],"count":4}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `[1,"two",true,null]` {
		t.Errorf("got %s, want %s", got, `[1,"two",true,null]`)
	}
}

func TestDeleteWithNestedArray(t *testing.T) {
	p, err := Compile("del(.meta)")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{"data":[1,2,3],"meta":{"page":1}}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"data":[1,2,3]}` {
		t.Errorf("got %s, want %s", got, `{"data":[1,2,3]}`)
	}
}

func TestCompileError(t *testing.T) {
	_, err := Compile("")
	if err == nil {
		t.Error("expected error for empty query")
	}

	_, err = Compile("foo")
	if err == nil {
		t.Error("expected error for invalid query")
	}

	_, err = Compile("del()")
	if err == nil {
		t.Error("expected error for del() with no args")
	}
}

func TestDeleteWithEscapedStrings(t *testing.T) {
	p, err := Compile("del(.remove)")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{"keep":"hello \"world\"","remove":"bye"}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	expected := `{"keep":"hello \"world\""}`
	if string(got) != expected {
		t.Errorf("got %s, want %s", got, expected)
	}
}

// --- Array Indexing ---

func TestArrayIndexFirst(t *testing.T) {
	p, err := Compile(".[0]")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`[10,20,30]`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "10" {
		t.Errorf("got %s, want 10", got)
	}
}

func TestArrayIndexMiddle(t *testing.T) {
	p, err := Compile(".[2]")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`["a","b","c","d"]`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `"c"` {
		t.Errorf("got %s, want %q", got, `"c"`)
	}
}

func TestArrayIndexNegative(t *testing.T) {
	p, err := Compile(".[-1]")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`[10,20,30]`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "30" {
		t.Errorf("got %s, want 30", got)
	}
}

func TestArrayIndexNegativeSecond(t *testing.T) {
	p, err := Compile(".[-2]")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`[10,20,30]`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "20" {
		t.Errorf("got %s, want 20", got)
	}
}

func TestArrayIndexOutOfBounds(t *testing.T) {
	p, err := Compile(".[5]")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`[10,20,30]`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "null" {
		t.Errorf("got %s, want null", got)
	}
}

func TestArrayIndexNegativeOutOfBounds(t *testing.T) {
	p, err := Compile(".[-10]")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`[10,20,30]`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "null" {
		t.Errorf("got %s, want null", got)
	}
}

// --- Chained Array Access ---

func TestChainedFieldIndex(t *testing.T) {
	p, err := Compile(".items[0]")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{"items":["a","b","c"]}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `"a"` {
		t.Errorf("got %s, want %q", got, `"a"`)
	}
}

func TestChainedFieldIndexField(t *testing.T) {
	p, err := Compile(".data[0].name")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{"data":[{"name":"alice"},{"name":"bob"}]}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `"alice"` {
		t.Errorf("got %s, want %q", got, `"alice"`)
	}
}

func TestPipeArrayIndex(t *testing.T) {
	p, err := Compile(".items | .[0]")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{"items":[10,20,30]}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "10" {
		t.Errorf("got %s, want 10", got)
	}
}

// --- Array Deletion ---

func TestArrayDeleteFirst(t *testing.T) {
	p, err := Compile("del(.[0])")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`[10,20,30]`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "[20,30]" {
		t.Errorf("got %s, want [20,30]", got)
	}
}

func TestArrayDeleteMultiple(t *testing.T) {
	p, err := Compile("del(.[1], .[3])")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`[10,20,30,40,50]`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "[10,30,50]" {
		t.Errorf("got %s, want [10,30,50]", got)
	}
}

func TestArrayDeleteLast(t *testing.T) {
	p, err := Compile("del(.[-1])")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`[10,20,30]`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "[10,20]" {
		t.Errorf("got %s, want [10,20]", got)
	}
}

func TestArrayDeleteAll(t *testing.T) {
	p, err := Compile("del(.[0], .[1])")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`[10,20]`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "[]" {
		t.Errorf("got %s, want []", got)
	}
}

func TestNestedArrayDelete(t *testing.T) {
	p, err := Compile(".items | del(.[0])")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{"items":[10,20,30]}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "[20,30]" {
		t.Errorf("got %s, want [20,30]", got)
	}
}

// --- Iterator ---

func TestIteratorArray(t *testing.T) {
	p, err := Compile(".[]")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`[10,20,30]`)
	results, err := p.RunAll(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	expected := []string{"10", "20", "30"}
	for i, r := range results {
		if string(r) != expected[i] {
			t.Errorf("result[%d] = %s, want %s", i, r, expected[i])
		}
	}
}

func TestIteratorObject(t *testing.T) {
	p, err := Compile(".[]")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{"a":1,"b":2,"c":3}`)
	results, err := p.RunAll(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	expected := []string{"1", "2", "3"}
	for i, r := range results {
		if string(r) != expected[i] {
			t.Errorf("result[%d] = %s, want %s", i, r, expected[i])
		}
	}
}

func TestIteratorEmptyArray(t *testing.T) {
	p, err := Compile(".[]")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`[]`)
	results, err := p.RunAll(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestIteratorEmptyObject(t *testing.T) {
	p, err := Compile(".[]")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{}`)
	results, err := p.RunAll(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestFieldIterator(t *testing.T) {
	p, err := Compile(".items[]")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{"items":[1,2,3]}`)
	results, err := p.RunAll(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	expected := []string{"1", "2", "3"}
	for i, r := range results {
		if string(r) != expected[i] {
			t.Errorf("result[%d] = %s, want %s", i, r, expected[i])
		}
	}
}

func TestIteratorRunReturnsFirst(t *testing.T) {
	p, err := Compile(".[]")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`[10,20,30]`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "10" {
		t.Errorf("got %s, want 10", got)
	}
}

func TestIteratorPipeConstruct(t *testing.T) {
	p, err := Compile(".[] | {name}")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`[{"name":"alice","age":30},{"name":"bob","age":25}]`)
	results, err := p.RunAll(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if string(results[0]) != `{"name":"alice"}` {
		t.Errorf("result[0] = %s, want %s", results[0], `{"name":"alice"}`)
	}
	if string(results[1]) != `{"name":"bob"}` {
		t.Errorf("result[1] = %s, want %s", results[1], `{"name":"bob"}`)
	}
}

// --- Object Construction ---

func TestConstructShorthand(t *testing.T) {
	p, err := Compile("{name}")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{"name":"alice","age":30,"city":"NYC"}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"name":"alice"}` {
		t.Errorf("got %s, want %s", got, `{"name":"alice"}`)
	}
}

func TestConstructShorthandMultiple(t *testing.T) {
	p, err := Compile("{name, age}")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{"name":"alice","age":30,"city":"NYC"}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"name":"alice","age":30}` {
		t.Errorf("got %s, want %s", got, `{"name":"alice","age":30}`)
	}
}

func TestConstructRename(t *testing.T) {
	p, err := Compile("{a: .foo, b: .bar}")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{"foo":1,"bar":2,"baz":3}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"a":1,"b":2}` {
		t.Errorf("got %s, want %s", got, `{"a":1,"b":2}`)
	}
}

func TestConstructNested(t *testing.T) {
	p, err := Compile("{city: .address.city}")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{"name":"alice","address":{"city":"NYC","zip":"10001"}}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"city":"NYC"}` {
		t.Errorf("got %s, want %s", got, `{"city":"NYC"}`)
	}
}

func TestConstructMissingField(t *testing.T) {
	p, err := Compile("{name, missing}")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{"name":"alice"}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"name":"alice","missing":null}` {
		t.Errorf("got %s, want %s", got, `{"name":"alice","missing":null}`)
	}
}

// --- Array Construction ---

func TestArrayConstruct(t *testing.T) {
	p, err := Compile("[.name, .age]")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{"name":"alice","age":30}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `["alice",30]` {
		t.Errorf("got %s, want %s", got, `["alice",30]`)
	}
}

func TestArrayConstructNested(t *testing.T) {
	p, err := Compile("[.name, .address.city]")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{"name":"alice","address":{"city":"NYC"}}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `["alice","NYC"]` {
		t.Errorf("got %s, want %s", got, `["alice","NYC"]`)
	}
}

func TestArrayConstructSingle(t *testing.T) {
	p, err := Compile("[.name]")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{"name":"alice"}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `["alice"]` {
		t.Errorf("got %s, want %s", got, `["alice"]`)
	}
}

// --- Multi-output API ---

func TestRunAllSingleOutput(t *testing.T) {
	p, err := Compile(".name")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{"name":"alice"}`)
	results, err := p.RunAll(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if string(results[0]) != `"alice"` {
		t.Errorf("got %s, want %q", results[0], `"alice"`)
	}
}

func TestRunFuncCount(t *testing.T) {
	p, err := Compile(".[]")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`[1,2,3,4,5]`)
	count := 0
	err = p.RunFunc(input, func(result []byte) error {
		count++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count != 5 {
		t.Errorf("expected 5 calls, got %d", count)
	}
}

func TestRunFuncValues(t *testing.T) {
	p, err := Compile(".[]")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`["a","b","c"]`)
	var results []string
	err = p.RunFunc(input, func(result []byte) error {
		results = append(results, string(result))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{`"a"`, `"b"`, `"c"`}
	if len(results) != len(expected) {
		t.Fatalf("expected %d results, got %d", len(expected), len(results))
	}
	for i, r := range results {
		if r != expected[i] {
			t.Errorf("result[%d] = %s, want %s", i, r, expected[i])
		}
	}
}

// --- Pipe with multi-output ---

func TestPipeIteratorField(t *testing.T) {
	p, err := Compile(".items[] | .name")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{"items":[{"name":"alice"},{"name":"bob"}]}`)
	results, err := p.RunAll(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if string(results[0]) != `"alice"` {
		t.Errorf("result[0] = %s, want %q", results[0], `"alice"`)
	}
	if string(results[1]) != `"bob"` {
		t.Errorf("result[1] = %s, want %q", results[1], `"bob"`)
	}
}

func TestPipeIteratorConstruct(t *testing.T) {
	p, err := Compile(".items[] | {n: .name}")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{"items":[{"name":"alice","age":30},{"name":"bob","age":25}]}`)
	results, err := p.RunAll(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if string(results[0]) != `{"n":"alice"}` {
		t.Errorf("result[0] = %s, want %s", results[0], `{"n":"alice"}`)
	}
	if string(results[1]) != `{"n":"bob"}` {
		t.Errorf("result[1] = %s, want %s", results[1], `{"n":"bob"}`)
	}
}

// --- Literals ---

func TestLiteralNull(t *testing.T)     { assertQuery(t, "null", `{"a":1}`, "null") }
func TestLiteralTrue(t *testing.T)     { assertQuery(t, "true", `{}`, "true") }
func TestLiteralFalse(t *testing.T)    { assertQuery(t, "false", `{}`, "false") }
func TestLiteralString(t *testing.T)   { assertQuery(t, `"hello"`, `{}`, `"hello"`) }
func TestLiteralInteger(t *testing.T)  { assertQuery(t, "42", `{}`, "42") }
func TestLiteralFloat(t *testing.T)    { assertQuery(t, "3.14", `{}`, "3.14") }
func TestLiteralNegative(t *testing.T) { assertQuery(t, "-5", `{}`, "-5") }

// --- Comparisons ---

func TestCompareStringEqual(t *testing.T)      { assertQuery(t, `.name == "alice"`, `{"name":"alice"}`, "true") }
func TestCompareStringNotEqual(t *testing.T)   { assertQuery(t, `.name == "bob"`, `{"name":"alice"}`, "false") }
func TestCompareNotEqualOp(t *testing.T)       { assertQuery(t, `.age != 30`, `{"age":25}`, "true") }
func TestCompareNull(t *testing.T)             { assertQuery(t, `.x == null`, `{"y":1}`, "true") }
func TestCompareLiteralStrings(t *testing.T)   { assertQuery(t, `"a" == "a"`, `{}`, "true") }
func TestCompareLiteralStringsNotEqual(t *testing.T) { assertQuery(t, `"a" != "b"`, `{}`, "true") }
func TestCompareFields(t *testing.T)           { assertQuery(t, `.x == .y`, `{"x":1,"y":1}`, "true") }
func TestCompareNumberFloat(t *testing.T)      { assertQuery(t, `1.0 == 1`, `{}`, "true") }

// --- Select ---

func TestSelectMatch(t *testing.T) {
	p, err := Compile(`select(.level == "error")`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Run([]byte(`{"level":"error","msg":"boom"}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"level":"error","msg":"boom"}` {
		t.Errorf("got %s, want original input", got)
	}
}

func TestSelectNoMatch(t *testing.T) {
	p, err := Compile(`select(.level == "error")`)
	if err != nil {
		t.Fatal(err)
	}
	results, err := p.RunAll([]byte(`{"level":"info","msg":"ok"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d: %s", len(results), results)
	}
}

func TestSelectTrue(t *testing.T) {
	p, err := Compile("select(true)")
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Run([]byte(`42`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "42" {
		t.Errorf("got %s, want 42", got)
	}
}

func TestSelectFalse(t *testing.T) {
	p, err := Compile("select(false)")
	if err != nil {
		t.Fatal(err)
	}
	results, err := p.RunAll([]byte(`42`))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestSelectMissing(t *testing.T) {
	p, err := Compile("select(.missing)")
	if err != nil {
		t.Fatal(err)
	}
	results, err := p.RunAll([]byte(`{"a":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results (null is falsy), got %d", len(results))
	}
}

func TestSelectIteratorFilter(t *testing.T) {
	p, err := Compile(`.[] | select(.active == true)`)
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`[{"name":"alice","active":true},{"name":"bob","active":false},{"name":"carol","active":true}]`)
	results, err := p.RunAll(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if string(results[0]) != `{"name":"alice","active":true}` {
		t.Errorf("result[0] = %s", results[0])
	}
	if string(results[1]) != `{"name":"carol","active":true}` {
		t.Errorf("result[1] = %s", results[1])
	}
}

func TestSelectIteratorConstruct(t *testing.T) {
	p, err := Compile(`.[] | select(.level == "error") | {message}`)
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`[{"level":"error","message":"boom"},{"level":"info","message":"ok"},{"level":"error","message":"crash"}]`)
	results, err := p.RunAll(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if string(results[0]) != `{"message":"boom"}` {
		t.Errorf("result[0] = %s", results[0])
	}
	if string(results[1]) != `{"message":"crash"}` {
		t.Errorf("result[1] = %s", results[1])
	}
}

// --- Alternative ---

func TestAlternativeDefault(t *testing.T) {
	p, err := Compile(`.foo // "default"`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Run([]byte(`{"bar":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `"default"` {
		t.Errorf("got %s, want %q", got, `"default"`)
	}
}

func TestAlternativeNull(t *testing.T) {
	p, err := Compile(`null // "x"`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Run([]byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `"x"` {
		t.Errorf("got %s, want %q", got, `"x"`)
	}
}

func TestAlternativeFalse(t *testing.T) {
	p, err := Compile(`false // "x"`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Run([]byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `"x"` {
		t.Errorf("got %s, want %q", got, `"x"`)
	}
}

func TestAlternativeFieldFallback(t *testing.T) {
	p, err := Compile(`.foo // .bar`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Run([]byte(`{"bar":"fallback"}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `"fallback"` {
		t.Errorf("got %s, want %q", got, `"fallback"`)
	}
}

func TestAlternativeChained(t *testing.T) {
	p, err := Compile(`.a // .b // .c`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Run([]byte(`{"c":"found"}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `"found"` {
		t.Errorf("got %s, want %q", got, `"found"`)
	}
}

func TestAlternativeNotNeeded(t *testing.T) {
	p, err := Compile(`.foo // "default"`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Run([]byte(`{"foo":"exists"}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `"exists"` {
		t.Errorf("got %s, want %q", got, `"exists"`)
	}
}

// --- Optional ---

func TestOptionalFieldOnString(t *testing.T) {
	p, err := Compile(".foo?")
	if err != nil {
		t.Fatal(err)
	}
	results, err := p.RunAll([]byte(`"not an object"`))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d: %s", len(results), results)
	}
}

func TestOptionalIndexOnObject(t *testing.T) {
	p, err := Compile(".[0]?")
	if err != nil {
		t.Fatal(err)
	}
	results, err := p.RunAll([]byte(`{"a":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d: %s", len(results), results)
	}
}

func TestOptionalIteratorOnScalar(t *testing.T) {
	p, err := Compile(".[]?")
	if err != nil {
		t.Fatal(err)
	}
	results, err := p.RunAll([]byte(`42`))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d: %s", len(results), results)
	}
}

func TestOptionalFieldNormal(t *testing.T) {
	p, err := Compile(".foo?")
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Run([]byte(`{"foo":"bar"}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `"bar"` {
		t.Errorf("got %s, want %q", got, `"bar"`)
	}
}

func TestOptionalChainedField(t *testing.T) {
	// .foo?.bar — foo optional, bar should still work if foo exists
	p, err := Compile(".foo?.bar")
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Run([]byte(`{"foo":{"bar":"baz"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `"baz"` {
		t.Errorf("got %s, want %q", got, `"baz"`)
	}
}

func TestOptionalChainedFieldMissing(t *testing.T) {
	// .foo?.bar — foo is optional and input is not an object
	p, err := Compile(".foo?.bar")
	if err != nil {
		t.Fatal(err)
	}
	results, err := p.RunAll([]byte(`"string"`))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d: %s", len(results), results)
	}
}

// --- Type ---

func TestTypeString(t *testing.T)  { assertQuery(t, "type", `"hello"`, `"string"`) }
func TestTypeNumber(t *testing.T)  { assertQuery(t, "type", `42`, `"number"`) }
func TestTypeObject(t *testing.T)  { assertQuery(t, "type", `{"a":1}`, `"object"`) }
func TestTypeArray(t *testing.T)   { assertQuery(t, "type", `[1,2]`, `"array"`) }
func TestTypeBoolean(t *testing.T) { assertQuery(t, "type", `true`, `"boolean"`) }
func TestTypeNull(t *testing.T)    { assertQuery(t, "type", `null`, `"null"`) }
func TestTypePiped(t *testing.T)   { assertQuery(t, ".value | type", `{"value":"hello"}`, `"string"`) }

func TestSelectWithType(t *testing.T) {
	p, err := Compile(`.[] | select(type == "object")`)
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`[1, "hello", {"a":1}, [2], {"b":2}]`)
	results, err := p.RunAll(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if string(results[0]) != `{"a":1}` {
		t.Errorf("result[0] = %s", results[0])
	}
	if string(results[1]) != `{"b":2}` {
		t.Errorf("result[1] = %s", results[1])
	}
}

// --- Combined ---

func TestCombinedLogFiltering(t *testing.T) {
	p, err := Compile(`.[] | select(.level == "error") | {msg: .message}`)
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`[{"level":"error","message":"boom"},{"level":"info","message":"ok"}]`)
	results, err := p.RunAll(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if string(results[0]) != `{"msg":"boom"}` {
		t.Errorf("got %s, want %s", results[0], `{"msg":"boom"}`)
	}
}

func TestCombinedServiceDefault(t *testing.T) {
	p, err := Compile(`.service // "unknown"`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Run([]byte(`{"level":"error"}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `"unknown"` {
		t.Errorf("got %s, want %q", got, `"unknown"`)
	}
}

func TestCombinedOptionalPipeIterator(t *testing.T) {
	p, err := Compile(`.data? | .items[]`)
	if err != nil {
		t.Fatal(err)
	}
	// Normal case — data exists
	input := []byte(`{"data":{"items":[1,2,3]}}`)
	results, err := p.RunAll(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
}

// --- Precedence ---

func TestPrecedenceCompareAlternativePipe(t *testing.T) {
	// .level == "error" // false | select(.)
	// Parses as ((.level == "error") // false) | select(.)
	// Left side: .level == "error" → true, true // false → true
	// Pipe sends true to select(.) → select passes through true
	p, err := Compile(`.level == "error" // false | select(.)`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Run([]byte(`{"level":"error"}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "true" {
		t.Errorf("got %s, want true", got)
	}

	// Verify: when level is not "error", comparison is false,
	// false // false → false (falsy), select(.) filters it out
	results, err := p.RunAll([]byte(`{"level":"info"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for non-matching, got %d: %s", len(results), results)
	}
}

func TestConstructWithLiteralValue(t *testing.T) {
	p, err := Compile(`{status: "ok", name: .name}`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Run([]byte(`{"name":"alice"}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"status":"ok","name":"alice"}` {
		t.Errorf("got %s", got)
	}
}

func TestConstructWithAlternative(t *testing.T) {
	p, err := Compile(`{name: .name // "anonymous"}`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Run([]byte(`{"age":30}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"name":"anonymous"}` {
		t.Errorf("got %s", got)
	}
}

// --- first / last / limit ---

func TestFirstNoArg(t *testing.T) {
	// first with no arg → .[0]
	p, _ := Compile("first")
	got, err := p.Run([]byte(`[10,20,30]`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "10" {
		t.Errorf("got %s, want 10", got)
	}
}

func TestLastNoArg(t *testing.T) {
	// last with no arg → .[-1]
	p, _ := Compile("last")
	got, err := p.Run([]byte(`[10,20,30]`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "30" {
		t.Errorf("got %s, want 30", got)
	}
}

func TestFirstExpr(t *testing.T) {
	p, _ := Compile(`first(.[] | select(. > 2))`)
	got, err := p.Run([]byte(`[1,2,3,4,5]`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "3" {
		t.Errorf("got %s, want 3", got)
	}
}

func TestLastExpr(t *testing.T) {
	p, _ := Compile(`last(.[] | select(. > 2))`)
	got, err := p.Run([]byte(`[1,2,3,4,5]`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "5" {
		t.Errorf("got %s, want 5", got)
	}
}

func TestFirstExprNoMatch(t *testing.T) {
	p, _ := Compile(`first(.[] | select(. > 10))`)
	results, err := p.RunAll([]byte(`[1,2,3]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results when nothing matches")
	}
}

func TestLastExprNoMatch(t *testing.T) {
	p, _ := Compile(`last(.[] | select(. > 10))`)
	results, err := p.RunAll([]byte(`[1,2,3]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results when nothing matches")
	}
}

func TestLimitBasic(t *testing.T) {
	p, _ := Compile(`limit(3; .[]  )`)
	results, err := p.RunAll([]byte(`[10,20,30,40,50]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	expected := []string{"10", "20", "30"}
	for i, r := range results {
		if string(r) != expected[i] {
			t.Errorf("result[%d] = %s, want %s", i, r, expected[i])
		}
	}
}

func TestLimitExact(t *testing.T) {
	// limit equals available results
	p, _ := Compile(`limit(3; .[])`)
	results, err := p.RunAll([]byte(`[1,2,3]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
}

func TestLimitMoreThanAvailable(t *testing.T) {
	// limit larger than available — returns all
	p, _ := Compile(`limit(10; .[])`)
	results, err := p.RunAll([]byte(`[1,2,3]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
}

func TestLimitZero(t *testing.T) {
	p, _ := Compile(`limit(0; .[])`)
	results, err := p.RunAll([]byte(`[1,2,3]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for limit(0)")
	}
}

func TestLimitWithFilter(t *testing.T) {
	p, _ := Compile(`limit(2; .[] | select(. > 2))`)
	results, err := p.RunAll([]byte(`[1,2,3,4,5,6]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if string(results[0]) != "3" || string(results[1]) != "4" {
		t.Errorf("got %v", results)
	}
}

// --- keys_unsorted ---

func TestKeysUnsortedObject(t *testing.T) {
	p, _ := Compile("keys_unsorted")
	got, err := p.Run([]byte(`{"b":2,"a":1,"c":3}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `["b","a","c"]` {
		t.Errorf("got %s, want [\"b\",\"a\",\"c\"] (insertion order)", got)
	}
}

func TestKeysUnsortedArray(t *testing.T) {
	p, _ := Compile("keys_unsorted")
	got, err := p.Run([]byte(`["x","y","z"]`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `[0,1,2]` {
		t.Errorf("got %s, want [0,1,2]", got)
	}
}

func TestKeysUnsortedEmpty(t *testing.T) {
	p, _ := Compile("keys_unsorted")
	got, err := p.Run([]byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `[]` {
		t.Errorf("got %s, want []", got)
	}
}

// --- any / all ---

func TestAnyNoArgTrueCase(t *testing.T) {
	p, _ := Compile("any")
	got, err := p.Run([]byte(`[false, null, 1]`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "true" {
		t.Errorf("got %s, want true", got)
	}
}

func TestAnyNoArgFalseCase(t *testing.T) {
	p, _ := Compile("any")
	got, err := p.Run([]byte(`[false, null, false]`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "false" {
		t.Errorf("got %s, want false", got)
	}
}

func TestAnyNoArgEmpty(t *testing.T) {
	p, _ := Compile("any")
	got, err := p.Run([]byte(`[]`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "false" {
		t.Errorf("got %s, want false (empty → vacuously false)", got)
	}
}

func TestAllNoArgTrue(t *testing.T) {
	p, _ := Compile("all")
	got, err := p.Run([]byte(`[1, "x", true]`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "true" {
		t.Errorf("got %s, want true", got)
	}
}

func TestAllNoArgFalse(t *testing.T) {
	p, _ := Compile("all")
	got, err := p.Run([]byte(`[1, false, true]`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "false" {
		t.Errorf("got %s, want false", got)
	}
}

func TestAllNoArgEmpty(t *testing.T) {
	p, _ := Compile("all")
	got, err := p.Run([]byte(`[]`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "true" {
		t.Errorf("got %s, want true (empty → vacuously true)", got)
	}
}

func TestAnyWithExpr(t *testing.T) {
	p, _ := Compile(`any(. > 2)`)
	got, err := p.Run([]byte(`[1, 2, 3]`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "true" {
		t.Errorf("got %s, want true", got)
	}
}

func TestAnyWithExprFalse(t *testing.T) {
	p, _ := Compile(`any(. > 10)`)
	got, err := p.Run([]byte(`[1, 2, 3]`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "false" {
		t.Errorf("got %s, want false", got)
	}
}

func TestAllWithExpr(t *testing.T) {
	p, _ := Compile(`all(. > 0)`)
	got, err := p.Run([]byte(`[1, 2, 3]`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "true" {
		t.Errorf("got %s, want true", got)
	}
}

func TestAllWithExprFalse(t *testing.T) {
	p, _ := Compile(`all(. > 1)`)
	got, err := p.Run([]byte(`[1, 2, 3]`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "false" {
		t.Errorf("got %s, want false", got)
	}
}

func TestAnyWithFieldExpr(t *testing.T) {
	p, _ := Compile(`any(.active == true)`)
	got, err := p.Run([]byte(`[{"active":false},{"active":true},{"active":false}]`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "true" {
		t.Errorf("got %s, want true", got)
	}
}

func TestAllWithFieldExpr(t *testing.T) {
	p, _ := Compile(`all(.level == "error")`)
	got, err := p.Run([]byte(`[{"level":"error"},{"level":"error"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "true" {
		t.Errorf("got %s, want true", got)
	}
}

func TestAnyInSelect(t *testing.T) {
	p, _ := Compile(`select(.tags | any(. == "critical"))`)
	input := []byte(`{"msg":"boom","tags":["warn","critical"]}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(input) {
		t.Errorf("got %s", got)
	}
	results, err := p.RunAll([]byte(`{"msg":"ok","tags":["info","warn"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results")
	}
}

// --- ascii_downcase / ascii_upcase ---

func TestAsciiDowncase(t *testing.T) {
	p, _ := Compile("ascii_downcase")
	got, err := p.Run([]byte(`"Hello World"`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `"hello world"` {
		t.Errorf("got %s", got)
	}
}

func TestAsciiUpcase(t *testing.T) {
	p, _ := Compile("ascii_upcase")
	got, err := p.Run([]byte(`"hello world"`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `"HELLO WORLD"` {
		t.Errorf("got %s", got)
	}
}

func TestAsciiDowncaseAlreadyLower(t *testing.T) {
	p, _ := Compile("ascii_downcase")
	got, err := p.Run([]byte(`"already lower"`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `"already lower"` {
		t.Errorf("got %s", got)
	}
}

func TestAsciiDowncasePreservesEscapes(t *testing.T) {
	p, _ := Compile("ascii_downcase")
	got, err := p.Run([]byte(`"Hello\nWorld"`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `"hello\nworld"` {
		t.Errorf("got %s", got)
	}
}

func TestAsciiDowncaseInSelect(t *testing.T) {
	p, _ := Compile(`select(.level | ascii_downcase == "error")`)
	input := []byte(`{"level":"ERROR","msg":"boom"}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(input) {
		t.Errorf("got %s", got)
	}
	results, err := p.RunAll([]byte(`{"level":"INFO","msg":"ok"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results")
	}
}

func TestAsciiDowncasePiped(t *testing.T) {
	p, _ := Compile(`.level | ascii_downcase`)
	got, err := p.Run([]byte(`{"level":"WARNING"}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `"warning"` {
		t.Errorf("got %s", got)
	}
}

// --- startswith / endswith ---

func TestStartsWithTrue(t *testing.T) {
	p, _ := Compile(`startswith("foo")`)
	got, err := p.Run([]byte(`"foobar"`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "true" {
		t.Errorf("got %s, want true", got)
	}
}

func TestStartsWithFalse(t *testing.T) {
	p, _ := Compile(`startswith("foo")`)
	got, err := p.Run([]byte(`"barfoo"`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "false" {
		t.Errorf("got %s, want false", got)
	}
}

func TestStartsWithExactMatch(t *testing.T) {
	p, _ := Compile(`startswith("foo")`)
	got, err := p.Run([]byte(`"foo"`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "true" {
		t.Errorf("got %s, want true", got)
	}
}

func TestStartsWithTooShort(t *testing.T) {
	p, _ := Compile(`startswith("foobar")`)
	got, err := p.Run([]byte(`"foo"`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "false" {
		t.Errorf("got %s, want false", got)
	}
}

func TestEndsWithTrue(t *testing.T) {
	p, _ := Compile(`endswith("bar")`)
	got, err := p.Run([]byte(`"foobar"`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "true" {
		t.Errorf("got %s, want true", got)
	}
}

func TestEndsWithFalse(t *testing.T) {
	p, _ := Compile(`endswith("bar")`)
	got, err := p.Run([]byte(`"barfoo"`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "false" {
		t.Errorf("got %s, want false", got)
	}
}

func TestStartsWithInSelect(t *testing.T) {
	p, _ := Compile(`select(.path | startswith("/api/"))`)
	input := []byte(`{"path":"/api/users","status":200}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(input) {
		t.Errorf("got %s", got)
	}
	results, err := p.RunAll([]byte(`{"path":"/health","status":200}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for non-/api path")
	}
}

// --- ltrimstr / rtrimstr ---

func TestLtrimStrMatch(t *testing.T) {
	p, _ := Compile(`ltrimstr("prod-")`)
	got, err := p.Run([]byte(`"prod-auth"`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `"auth"` {
		t.Errorf("got %s, want \"auth\"", got)
	}
}

func TestLtrimStrNoMatch(t *testing.T) {
	p, _ := Compile(`ltrimstr("prod-")`)
	got, err := p.Run([]byte(`"staging-auth"`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `"staging-auth"` {
		t.Errorf("got %s, want unchanged", got)
	}
}

func TestLtrimStrFullMatch(t *testing.T) {
	p, _ := Compile(`ltrimstr("foo")`)
	got, err := p.Run([]byte(`"foo"`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `""` {
		t.Errorf("got %s, want empty string", got)
	}
}

func TestRtrimStrMatch(t *testing.T) {
	p, _ := Compile(`rtrimstr(".log")`)
	got, err := p.Run([]byte(`"app.log"`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `"app"` {
		t.Errorf("got %s, want \"app\"", got)
	}
}

func TestRtrimStrNoMatch(t *testing.T) {
	p, _ := Compile(`rtrimstr(".log")`)
	got, err := p.Run([]byte(`"app.txt"`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `"app.txt"` {
		t.Errorf("got %s, want unchanged", got)
	}
}

func TestLtrimStrInSelect(t *testing.T) {
	p, _ := Compile(`select(.service | ltrimstr("prod-") == "auth")`)
	input := []byte(`{"service":"prod-auth","level":"error"}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(input) {
		t.Errorf("got %s", got)
	}
}

func TestStringOpsComposed(t *testing.T) {
	// Normalize, trim prefix, check suffix
	p, _ := Compile(`select(.path | ascii_downcase | ltrimstr("/api") | startswith("/users"))`)
	input := []byte(`{"path":"/API/users/123"}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(input) {
		t.Errorf("got %s", got)
	}
}

// --- values ---

func TestValuesPassesNonNull(t *testing.T)  { assertQuery(t, `values`, `{"a":1}`, `{"a":1}`) }
func TestValuesPassesZero(t *testing.T)     { assertQuery(t, `values`, `0`, `0`) }
func TestValuesPassesFalse(t *testing.T)    { assertQuery(t, `values`, `false`, `false`) }
func TestValuesFiltersNull(t *testing.T)    { assertNoOutput(t, `values`, `null`) }
func TestValuesInStream(t *testing.T) {
	// .[] | values filters nulls from a stream
	p, _ := Compile(`.[] | values`)
	results, _ := p.RunAll([]byte(`[1,null,2,null,3]`))
	if len(results) != 3 || string(results[0]) != "1" || string(results[2]) != "3" {
		t.Errorf("got %v", results)
	}
}
func TestValuesObjectStream(t *testing.T) {
	p, _ := Compile(`.[] | values`)
	results, _ := p.RunAll([]byte(`{"a":1,"b":null,"c":3}`))
	if len(results) != 2 || string(results[0]) != "1" || string(results[1]) != "3" {
		t.Errorf("got %v", results)
	}
}

// --- in ---

func TestInObjectTrue(t *testing.T)   { assertQuery(t, `"foo" | in({"foo":1,"bar":2})`, `null`, `true`) }
func TestInObjectFalse(t *testing.T)  { assertQuery(t, `"baz" | in({"foo":1})`, `null`, `false`) }
func TestInArrayTrue(t *testing.T)    { assertQuery(t, `1 | in([0,1,2])`, `null`, `true`) }
func TestInArrayFalse(t *testing.T)   { assertQuery(t, `5 | in([0,1,2])`, `null`, `false`) }
func TestInArrayNeg(t *testing.T)     { assertQuery(t, `-1 | in([0,1,2])`, `null`, `false`) }
func TestInFieldAccess(t *testing.T) {
	assertQuery(t, `.key | in({"foo":1,"bar":2})`, `{"key":"foo"}`, `true`)
}

// --- @base64 / @base64d ---

func TestBase64Encode(t *testing.T)        { assertQuery(t, `@base64`, `"hello"`, `"aGVsbG8="`) }
func TestBase64EncodeSpace(t *testing.T)   { assertQuery(t, `@base64`, `"hello world"`, `"aGVsbG8gd29ybGQ="`) }
func TestBase64EncodeEmpty(t *testing.T)   { assertQuery(t, `@base64`, `""`, `""`) }
func TestBase64Decode(t *testing.T)        { assertQuery(t, `@base64d`, `"aGVsbG8="`, `"hello"`) }
func TestBase64DecodeSpace(t *testing.T)   { assertQuery(t, `@base64d`, `"aGVsbG8gd29ybGQ="`, `"hello world"`) }
func TestBase64RoundTrip(t *testing.T)     { assertQuery(t, `@base64 | @base64d`, `"hello"`, `"hello"`) }
func TestBase64DecodeURLSafe(t *testing.T) {
	// fastjq extension: accept URL-safe base64 chars (- and _ as alternatives to + and /)
	// "aGVsbG8=" == "aGVsbG8=" in URL-safe (no difference for "hello")
	// Test with data that's identical in both variants:
	assertQuery(t, `@base64d`, `"aGVsbG8="`, `"hello"`)
}
func TestBase64EncodeInPipe(t *testing.T) {
	assertQuery(t, `.token | @base64d`, `{"token":"aGVsbG8="}`, `"hello"`)
}
func TestBase64NoPadding(t *testing.T) {
	// base64 without padding — many real-world APIs omit =
	assertQuery(t, `@base64d`, `"aGVsbG8"`, `"hello"`)
}

// --- index / rindex / indices ---

func TestIndexString(t *testing.T)         { assertQuery(t, `index(",")`, `"a,b,c"`, `1`) }
func TestIndexStringMiss(t *testing.T)     { assertQuery(t, `index("x")`, `"hello"`, `null`) }
func TestRIndexString(t *testing.T)        { assertQuery(t, `rindex(",")`, `"a,b,c"`, `3`) }
func TestRIndexStringMiss(t *testing.T)    { assertQuery(t, `rindex("x")`, `"hello"`, `null`) }
func TestIndicesString(t *testing.T)       { assertQuery(t, `indices(",")`, `"a,b,c"`, `[1,3]`) }
func TestIndicesStringNone(t *testing.T)   { assertQuery(t, `indices("x")`, `"hello"`, `[]`) }
func TestIndexArray(t *testing.T)          { assertQuery(t, `index(2)`, `[1,2,3,2,1]`, `1`) }
func TestRIndexArray(t *testing.T)         { assertQuery(t, `rindex(2)`, `[1,2,3,2,1]`, `3`) }
func TestIndicesArray(t *testing.T)        { assertQuery(t, `indices(2)`, `[1,2,3,2,1]`, `[1,3]`) }
func TestIndexArrayMiss(t *testing.T)      { assertQuery(t, `index(9)`, `[1,2,3]`, `null`) }
func TestIndexInPipe(t *testing.T) {
	assertQuery(t, `.path | index("/")`, `{"path":"/api/users"}`, `0`)
}

// --- has(n) for arrays ---

func TestHasArrayInBounds(t *testing.T)    { assertQuery(t, `has(2)`, `[1,2,3]`, `true`) }
func TestHasArrayOutOfBounds(t *testing.T) { assertQuery(t, `has(5)`, `[1,2,3]`, `false`) }
func TestHasArrayNegative(t *testing.T)    { assertQuery(t, `has(-1)`, `[1,2,3]`, `false`) }
func TestHasArrayZero(t *testing.T)        { assertQuery(t, `has(0)`, `[1,2,3]`, `true`) }
func TestHasArrayEmpty(t *testing.T)       { assertQuery(t, `has(0)`, `[]`, `false`) }

// --- debug ---

func TestDebugPassthrough(t *testing.T) {
	assertQuery(t, `debug`, `{"a":1}`, `{"a":1}`)
}
func TestDebugInPipe(t *testing.T) {
	assertQuery(t, `debug | .a`, `{"a":42}`, `42`)
}

// --- slice .[n:m] ---

func TestSliceArray(t *testing.T)         { assertQuery(t, ".[2:4]", `[0,1,2,3,4]`, `[2,3]`) }
func TestSliceArrayFrom(t *testing.T)     { assertQuery(t, ".[2:]", `[0,1,2,3,4]`, `[2,3,4]`) }
func TestSliceArrayTo(t *testing.T)       { assertQuery(t, ".[:3]", `[0,1,2,3,4]`, `[0,1,2]`) }
func TestSliceArrayAll(t *testing.T)      { assertQuery(t, ".[:]", `[0,1,2,3,4]`, `[0,1,2,3,4]`) }
func TestSliceArrayNegFrom(t *testing.T)  { assertQuery(t, ".[-2:]", `[0,1,2,3,4]`, `[3,4]`) }
func TestSliceArrayNegTo(t *testing.T)    { assertQuery(t, ".[:-1]", `[0,1,2,3,4]`, `[0,1,2,3]`) }
func TestSliceArrayBothNeg(t *testing.T)  { assertQuery(t, ".[-3:-1]", `[0,1,2,3,4]`, `[2,3]`) }
func TestSliceArrayEmpty(t *testing.T)    { assertQuery(t, ".[3:3]", `[0,1,2,3,4]`, `[]`) }
func TestSliceString(t *testing.T)        { assertQuery(t, ".[0:5]", `"hello world"`, `"hello"`) }
func TestSliceStringFrom(t *testing.T)    { assertQuery(t, ".[6:]", `"hello world"`, `"world"`) }
func TestSliceStringNeg(t *testing.T)     { assertQuery(t, ".[-5:]", `"hello world"`, `"world"`) }
func TestSliceStringEscape(t *testing.T)  { assertQuery(t, ".[0:3]", `"a\nb\nc"`, `"a\nb"`) } // escape = 1 char
func TestSliceInPipe(t *testing.T) {
	assertQuery(t, `.items[1:3]`, `{"items":[10,20,30,40]}`, `[20,30]`)
}

// --- + (plus) ---

func TestPlusStrings(t *testing.T)     { assertQuery(t, `"hello" + " world"`, `{}`, `"hello world"`) }
func TestPlusStringField(t *testing.T) { assertQuery(t, `.a + .b`, `{"a":"foo","b":"bar"}`, `"foobar"`) }
func TestPlusArrays(t *testing.T)      { assertQuery(t, `[1,2] + [3,4]`, `{}`, `[1,2,3,4]`) }
func TestPlusArrayField(t *testing.T)  { assertQuery(t, `.a + .b`, `{"a":[1,2],"b":[3,4]}`, `[1,2,3,4]`) }
func TestPlusNumbers(t *testing.T)     { assertQuery(t, `.a + .b`, `{"a":1,"b":2}`, `3`) }
func TestPlusNullLeft(t *testing.T)    { assertQuery(t, `null + "x"`, `{}`, `"x"`) }
func TestPlusNullRight(t *testing.T)   { assertQuery(t, `"a" + null`, `{}`, `"a"`) }
func TestPlusNullMissing(t *testing.T) { assertQuery(t, `.missing + "default"`, `{}`, `"default"`) }
func TestPlusChained(t *testing.T)     { assertQuery(t, `"a" + "b" + "c"`, `{}`, `"abc"`) }
func TestPlusInPipe(t *testing.T) {
	assertQuery(t, `.prefix + .name`, `{"prefix":"user_","name":"alice"}`, `"user_alice"`)
}
func TestPlusPrecedence(t *testing.T) {
	// + binds tighter than ==: (.a + .b) == 3
	assertQuery(t, `.a + .b == 3`, `{"a":1,"b":2}`, `true`)
}

// --- add ---

func TestAddNumbers(t *testing.T)            { assertQuery(t, "add", `[1,2,3,4,5]`, `15`) }
func TestAddStrings(t *testing.T)            { assertQuery(t, "add", `["a","b","c"]`, `"abc"`) }
func TestAddArrays(t *testing.T)             { assertQuery(t, "add", `[[1,2],[3,4]]`, `[1,2,3,4]`) }
func TestAddEmpty(t *testing.T)              { assertQuery(t, "add", `[]`, `null`) }
func TestAddNull(t *testing.T)               { assertQuery(t, "add", `null`, `null`) }
func TestAddNullElements(t *testing.T)       { assertQuery(t, "add", `[null,null]`, `null`) }
func TestAddFloats(t *testing.T)             { assertQuery(t, "add", `[1.5,2.5]`, `4`) }
func TestAddMixedWithNull(t *testing.T)      { assertQuery(t, "add", `[null,1,2]`, `3`) }
func TestAddSingleNumber(t *testing.T)       { assertQuery(t, "add", `[42]`, `42`) }
func TestAddSingleString(t *testing.T)       { assertQuery(t, "add", `["hello"]`, `"hello"`) }
func TestAddInPipe(t *testing.T) {
	assertQuery(t, `[.[] | .x] | add`, `[{"x":1},{"x":2},{"x":3}]`, `6`)
}

// --- flatten ---

func TestFlattenOnce(t *testing.T) {
	assertQuery(t, `flatten`, `[[1,[2]],3]`, `[1,2,3]`)
}
func TestFlattenDeep(t *testing.T) {
	assertQuery(t, `flatten`, `[[[1]],[[2,[3]]]]`, `[1,2,3]`)
}
func TestFlattenDepth1(t *testing.T) {
	assertQuery(t, `flatten(1)`, `[[1,[2]],3]`, `[1,[2],3]`)
}
func TestFlattenDepth0(t *testing.T) {
	// flatten(0) = no flattening
	assertQuery(t, `flatten(0)`, `[[1,2],[3]]`, `[[1,2],[3]]`)
}
func TestFlattenEmpty(t *testing.T) {
	assertQuery(t, `flatten`, `[]`, `[]`)
}
func TestFlattenAlreadyFlat(t *testing.T) {
	assertQuery(t, `flatten`, `[1,2,3]`, `[1,2,3]`)
}

// --- split / join ---

func TestSplit(t *testing.T) {
	assertQuery(t, `split(",")`, `"a,b,c"`, `["a","b","c"]`)
}
func TestSplitMultiChar(t *testing.T) {
	assertQuery(t, `split(", ")`, `"a, b, c"`, `["a","b","c"]`)
}
func TestSplitNoMatch(t *testing.T) {
	assertQuery(t, `split(",")`, `"abc"`, `["abc"]`)
}
func TestSplitEmptyResult(t *testing.T) {
	assertQuery(t, `split(",")`, `","`, `["",""]`)
}
func TestJoin(t *testing.T) {
	assertQuery(t, `join(",")`, `["a","b","c"]`, `"a,b,c"`)
}
func TestJoinMultiChar(t *testing.T) {
	assertQuery(t, `join(", ")`, `["a","b","c"]`, `"a, b, c"`)
}
func TestJoinEmpty(t *testing.T) {
	assertQuery(t, `join(",")`, `[]`, `""`)
}
func TestJoinWithNull(t *testing.T) {
	assertQuery(t, `join(",")`, `["a",null,"b"]`, `"a,,b"`) // null → empty
}
func TestJoinNumbers(t *testing.T) {
	assertQuery(t, `join("-")`, `[1,2,3]`, `"1-2-3"`)
}
func TestSplitJoinRoundTrip(t *testing.T) {
	// split then join should reproduce the original
	assertQuery(t, `split(",") | join(",")`, `"a,b,c"`, `"a,b,c"`)
}

// --- to_entries / from_entries ---

func TestToEntries(t *testing.T) {
	p, _ := Compile("to_entries")
	got, err := p.Run([]byte(`{"a":1,"b":"hello"}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `[{"key":"a","value":1},{"key":"b","value":"hello"}]` {
		t.Errorf("got %s", got)
	}
}

func TestToEntriesEmpty(t *testing.T) {
	p, _ := Compile("to_entries")
	got, err := p.Run([]byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `[]` {
		t.Errorf("got %s, want []", got)
	}
}

func TestToEntriesNonObject(t *testing.T) {
	p, _ := Compile("to_entries")
	got, err := p.Run([]byte(`[1,2,3]`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `[]` {
		t.Errorf("got %s, want [] for non-object", got)
	}
}

func TestFromEntries(t *testing.T) {
	p, _ := Compile("from_entries")
	got, err := p.Run([]byte(`[{"key":"a","value":1},{"key":"b","value":"hello"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"a":1,"b":"hello"}` {
		t.Errorf("got %s", got)
	}
}

func TestFromEntriesNameField(t *testing.T) {
	assertQuery(t, "from_entries", `[{"name":"x","value":42}]`, `{"x":42}`)
}
func TestFromEntriesCapitalizedKey(t *testing.T) {
	// jq accepts Key, Name, Value (capitalized variants)
	assertQuery(t, "from_entries", `[{"Key":"a","Value":1}]`, `{"a":1}`)
}
func TestFromEntriesCapitalizedName(t *testing.T) {
	assertQuery(t, "from_entries", `[{"Name":"b","Value":2}]`, `{"b":2}`)
}

func TestFromEntriesEmpty(t *testing.T) {
	p, _ := Compile("from_entries")
	got, err := p.Run([]byte(`[]`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{}` {
		t.Errorf("got %s, want {}", got)
	}
}

func TestToFromEntriesRoundTrip(t *testing.T) {
	p, _ := Compile("to_entries | from_entries")
	input := []byte(`{"a":1,"b":2,"c":3}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(input) {
		t.Errorf("round-trip: got %s, want %s", got, input)
	}
}

func TestToEntriesNestedValue(t *testing.T) {
	p, _ := Compile("to_entries")
	got, err := p.Run([]byte(`{"data":{"nested":true}}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `[{"key":"data","value":{"nested":true}}]` {
		t.Errorf("got %s", got)
	}
}

// --- length ---

func TestLengthString(t *testing.T) {
	p, _ := Compile("length")
	got, err := p.Run([]byte(`"hello"`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "5" {
		t.Errorf("got %s, want 5", got)
	}
}

func TestLengthEmptyString(t *testing.T) {
	p, _ := Compile("length")
	got, err := p.Run([]byte(`""`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "0" {
		t.Errorf("got %s, want 0", got)
	}
}

func TestLengthArray(t *testing.T) {
	p, _ := Compile("length")
	got, err := p.Run([]byte(`[1,2,3,4,5]`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "5" {
		t.Errorf("got %s, want 5", got)
	}
}

func TestLengthEmptyArray(t *testing.T) {
	p, _ := Compile("length")
	got, err := p.Run([]byte(`[]`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "0" {
		t.Errorf("got %s, want 0", got)
	}
}

func TestLengthObject(t *testing.T) {
	p, _ := Compile("length")
	got, err := p.Run([]byte(`{"a":1,"b":2,"c":3}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "3" {
		t.Errorf("got %s, want 3", got)
	}
}

func TestLengthNull(t *testing.T) {
	p, _ := Compile("length")
	got, err := p.Run([]byte(`null`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "0" {
		t.Errorf("got %s, want 0", got)
	}
}

func TestLengthInSelect(t *testing.T) {
	p, _ := Compile(`select(.message | length > 0)`)
	input := []byte(`{"message":"hello"}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(input) {
		t.Errorf("got %s", got)
	}
	results, err := p.RunAll([]byte(`{"message":""}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty message")
	}
}

func TestLengthPiped(t *testing.T) {
	p, _ := Compile(`.tags | length`)
	got, err := p.Run([]byte(`{"tags":["a","b","c"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "3" {
		t.Errorf("got %s, want 3", got)
	}
}

func TestLengthWithEscapes(t *testing.T) {
	// "he\"llo" — 6 chars but 7 bytes in source; length counts chars
	p, _ := Compile("length")
	got, err := p.Run([]byte(`"he\"llo"`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "6" {
		t.Errorf("got %s, want 6", got)
	}
}

// --- map ---

func TestMapFieldExtract(t *testing.T) {
	p, _ := Compile("map(.name)")
	input := []byte(`[{"name":"alice","age":30},{"name":"bob","age":25}]`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `["alice","bob"]` {
		t.Errorf("got %s, want [\"alice\",\"bob\"]", got)
	}
}

func TestMapConstruct(t *testing.T) {
	p, _ := Compile("map({name, age})")
	input := []byte(`[{"name":"alice","age":30,"x":1},{"name":"bob","age":25,"x":2}]`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `[{"name":"alice","age":30},{"name":"bob","age":25}]` {
		t.Errorf("got %s", got)
	}
}

func TestMapSelect(t *testing.T) {
	// map(select(...)) filters elements — those that don't match produce empty
	p, _ := Compile(`map(select(.active == true))`)
	input := []byte(`[{"name":"alice","active":true},{"name":"bob","active":false},{"name":"carol","active":true}]`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `[{"name":"alice","active":true},{"name":"carol","active":true}]` {
		t.Errorf("got %s", got)
	}
}

func TestMapEmptyArray(t *testing.T) {
	p, _ := Compile("map(.x)")
	got, err := p.Run([]byte(`[]`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `[]` {
		t.Errorf("got %s, want []", got)
	}
}

func TestArrayConstructMultiOutput(t *testing.T) {
	// [.items[]] should collect all elements
	p, _ := Compile("[.items[]]")
	got, err := p.Run([]byte(`{"items":[1,2,3]}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `[1,2,3]` {
		t.Errorf("got %s, want [1,2,3]", got)
	}
}

// --- empty ---

func TestEmptyProducesNoOutput(t *testing.T) {
	p, _ := Compile("empty")
	results, err := p.RunAll([]byte(`{"a":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestEmptyInPipe(t *testing.T) {
	p, _ := Compile(`. | empty`)
	results, err := p.RunAll([]byte(`{"a":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

// --- has ---

func TestHasFieldPresent(t *testing.T) {
	p, _ := Compile(`has("name")`)
	got, err := p.Run([]byte(`{"name":"alice","age":30}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "true" {
		t.Errorf("got %s, want true", got)
	}
}

func TestHasFieldMissing(t *testing.T) {
	p, _ := Compile(`has("missing")`)
	got, err := p.Run([]byte(`{"name":"alice"}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "false" {
		t.Errorf("got %s, want false", got)
	}
}

func TestHasFieldNullValue(t *testing.T) {
	// has returns true even when the field value is null
	p, _ := Compile(`has("x")`)
	got, err := p.Run([]byte(`{"x":null}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "true" {
		t.Errorf("got %s, want true (field exists even if null)", got)
	}
}

func TestHasEmptyObject(t *testing.T) {
	p, _ := Compile(`has("x")`)
	got, err := p.Run([]byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "false" {
		t.Errorf("got %s, want false", got)
	}
}

func TestHasInSelect(t *testing.T) {
	p, _ := Compile(`select(has("error"))`)
	input := []byte(`{"error":"something went wrong"}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(input) {
		t.Errorf("got %s", got)
	}
	results, err := p.RunAll([]byte(`{"level":"info"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results when field absent")
	}
}

func TestHasDistinguishesNullFromMissing(t *testing.T) {
	// has("x") vs .x != null — they differ when value is null
	pHas, _ := Compile(`has("x")`)
	pNotNull, _ := Compile(`.x != null`)
	input := []byte(`{"x":null}`)

	gotHas, _ := pHas.Run(input)
	gotNotNull, _ := pNotNull.Run(input)

	if string(gotHas) != "true" {
		t.Errorf("has: got %s, want true", gotHas)
	}
	if string(gotNotNull) != "false" {
		t.Errorf("!= null: got %s, want false", gotNotNull)
	}
}

// --- if-then-else ---

func TestIfThenElseTrue(t *testing.T) {
	p, _ := Compile(`if .level == "error" then "ALERT" else "ok" end`)
	got, err := p.Run([]byte(`{"level":"error"}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `"ALERT"` {
		t.Errorf("got %s, want \"ALERT\"", got)
	}
}

func TestIfThenElseFalse(t *testing.T) {
	p, _ := Compile(`if .level == "error" then "ALERT" else "ok" end`)
	got, err := p.Run([]byte(`{"level":"info"}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `"ok"` {
		t.Errorf("got %s, want \"ok\"", got)
	}
}

func TestIfThenNoElse(t *testing.T) {
	// No else branch — defaults to identity (pass input through)
	p, _ := Compile(`if .debug then . end`)
	input := []byte(`{"debug":false,"msg":"hi"}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(input) {
		t.Errorf("got %s, want input (identity)", got)
	}
}

func TestIfThenElseWithDel(t *testing.T) {
	p, _ := Compile(`if has("secret") then del(.secret) else . end`)
	withSecret := []byte(`{"name":"alice","secret":"s3cr3t"}`)
	got, err := p.Run(withSecret)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"name":"alice"}` {
		t.Errorf("got %s", got)
	}
	noSecret := []byte(`{"name":"bob"}`)
	got2, err := p.Run(noSecret)
	if err != nil {
		t.Fatal(err)
	}
	if string(got2) != string(noSecret) {
		t.Errorf("got %s, want input unchanged", got2)
	}
}

func TestIfThenElseEmpty(t *testing.T) {
	// Use empty as else to filter records
	p, _ := Compile(`if .level == "error" then . else empty end`)
	results, err := p.RunAll([]byte(`{"level":"info"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results (empty else), got %d", len(results))
	}
	input := []byte(`{"level":"error","msg":"boom"}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(input) {
		t.Errorf("got %s", got)
	}
}

func TestIfThenElseWithConstruct(t *testing.T) {
	p, _ := Compile(`if .status >= 400 then {alert: .path, code: .status} else empty end`)
	input := []byte(`{"status":500,"path":"/api/data"}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"alert":"/api/data","code":500}` {
		t.Errorf("got %s", got)
	}
}

func TestIfThenElseNested(t *testing.T) {
	p, _ := Compile(`if .level == "error" then "high" else if .level == "warn" then "med" else "low" end end`)
	cases := []struct {
		input string
		want  string
	}{
		{`{"level":"error"}`, `"high"`},
		{`{"level":"warn"}`, `"med"`},
		{`{"level":"info"}`, `"low"`},
	}
	for _, tc := range cases {
		got, err := p.Run([]byte(tc.input))
		if err != nil {
			t.Fatalf("input %s: %v", tc.input, err)
		}
		if string(got) != tc.want {
			t.Errorf("input %s: got %s, want %s", tc.input, got, tc.want)
		}
	}
}

func TestIfConditionWithPipe(t *testing.T) {
	// if (.level | not) then . else empty end — pass through non-debug records
	p, _ := Compile(`if (.debug | not) then . else empty end`)
	keep := []byte(`{"debug":false,"msg":"hi"}`)
	got, err := p.Run(keep)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(keep) {
		t.Errorf("got %s", got)
	}
	results, err := p.RunAll([]byte(`{"debug":true,"msg":"noise"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for debug=true")
	}
}

// --- and / or / not ---

func TestAndBothTrue(t *testing.T) {
	p, _ := Compile(`.a == 1 and .b == 2`)
	got, err := p.Run([]byte(`{"a":1,"b":2}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "true" {
		t.Errorf("got %s, want true", got)
	}
}

func TestAndLeftFalse(t *testing.T) {
	p, _ := Compile(`.a == 1 and .b == 2`)
	got, err := p.Run([]byte(`{"a":9,"b":2}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "false" {
		t.Errorf("got %s, want false", got)
	}
}

func TestAndRightFalse(t *testing.T) {
	p, _ := Compile(`.a == 1 and .b == 2`)
	got, err := p.Run([]byte(`{"a":1,"b":9}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "false" {
		t.Errorf("got %s, want false", got)
	}
}

func TestOrLeftTrue(t *testing.T) {
	p, _ := Compile(`.a == 1 or .b == 2`)
	got, err := p.Run([]byte(`{"a":1,"b":9}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "true" {
		t.Errorf("got %s, want true", got)
	}
}

func TestOrRightTrue(t *testing.T) {
	p, _ := Compile(`.a == 1 or .b == 2`)
	got, err := p.Run([]byte(`{"a":9,"b":2}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "true" {
		t.Errorf("got %s, want true", got)
	}
}

func TestOrBothFalse(t *testing.T) {
	p, _ := Compile(`.a == 1 or .b == 2`)
	got, err := p.Run([]byte(`{"a":9,"b":9}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "false" {
		t.Errorf("got %s, want false", got)
	}
}

func TestNotFalse(t *testing.T) {
	p, _ := Compile(`false | not`)
	got, err := p.Run([]byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "true" {
		t.Errorf("got %s, want true", got)
	}
}

func TestNotTrue(t *testing.T) {
	p, _ := Compile(`true | not`)
	got, err := p.Run([]byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "false" {
		t.Errorf("got %s, want false", got)
	}
}

func TestNotNull(t *testing.T) {
	p, _ := Compile(`null | not`)
	got, err := p.Run([]byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "true" {
		t.Errorf("got %s, want true", got)
	}
}

func TestNotFieldPipe(t *testing.T) {
	p, _ := Compile(`.debug | not`)
	// debug=false → not → true
	got, err := p.Run([]byte(`{"debug":false}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "true" {
		t.Errorf("got %s, want true", got)
	}
}

func TestSelectWithAnd(t *testing.T) {
	p, _ := Compile(`select(.level == "error" and .retries > 2)`)
	input := []byte(`{"level":"error","retries":3}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(input) {
		t.Errorf("got %s", got)
	}
	results, err := p.RunAll([]byte(`{"level":"error","retries":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results when retries <= 2")
	}
}

func TestSelectWithOr(t *testing.T) {
	p, _ := Compile(`select(.level == "error" or .level == "fatal")`)
	input := []byte(`{"level":"fatal","msg":"down"}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(input) {
		t.Errorf("got %s", got)
	}
}

func TestSelectWithNot(t *testing.T) {
	p, _ := Compile(`select(.debug | not)`)
	input := []byte(`{"level":"info","debug":false}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(input) {
		t.Errorf("got %s", got)
	}
	results, err := p.RunAll([]byte(`{"debug":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results when debug=true")
	}
}

func TestAndOrPrecedence(t *testing.T) {
	// a or b and c  parses as  a or (b and c)  — and binds tighter
	// .a=false, .b=true, .c=true → false or (true and true) → true
	p, _ := Compile(`.a or .b and .c`)
	got, err := p.Run([]byte(`{"a":false,"b":true,"c":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "true" {
		t.Errorf("got %s, want true (and binds tighter than or)", got)
	}
	// .a=false, .b=true, .c=false → false or (true and false) → false
	got2, err := p.Run([]byte(`{"a":false,"b":true,"c":false}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got2) != "false" {
		t.Errorf("got %s, want false", got2)
	}
}

// --- Ordering operators ---

func TestLessThanNumbers(t *testing.T) {
	p, _ := Compile(`.x < .y`)
	got, err := p.Run([]byte(`{"x":1,"y":2}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "true" {
		t.Errorf("got %s, want true", got)
	}
}

func TestLessThanFalse(t *testing.T) {
	p, _ := Compile(`.x < .y`)
	got, err := p.Run([]byte(`{"x":2,"y":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "false" {
		t.Errorf("got %s, want false", got)
	}
}

func TestLessThanEqual(t *testing.T) {
	p, _ := Compile(`.x < .y`)
	got, err := p.Run([]byte(`{"x":1,"y":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "false" {
		t.Errorf("got %s, want false (equal is not less than)", got)
	}
}

func TestLessThanOrEqualTrue(t *testing.T) {
	p, _ := Compile(`.x <= .y`)
	got, err := p.Run([]byte(`{"x":1,"y":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "true" {
		t.Errorf("got %s, want true", got)
	}
}

func TestGreaterThan(t *testing.T) {
	p, _ := Compile(`.latency > 100`)
	got, err := p.Run([]byte(`{"latency":200}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "true" {
		t.Errorf("got %s, want true", got)
	}
}

func TestGreaterThanOrEqual(t *testing.T) {
	p, _ := Compile(`.status >= 400`)
	got, err := p.Run([]byte(`{"status":400}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "true" {
		t.Errorf("got %s, want true", got)
	}
}

func TestLessThanStrings(t *testing.T) {
	p, _ := Compile(`.a < .b`)
	got, err := p.Run([]byte(`{"a":"abc","b":"abd"}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "true" {
		t.Errorf("got %s, want true", got)
	}
}

func TestGreaterThanStrings(t *testing.T) {
	p, _ := Compile(`.a > .b`)
	got, err := p.Run([]byte(`{"a":"z","b":"a"}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "true" {
		t.Errorf("got %s, want true", got)
	}
}

func TestOrderingCrossTypeFalse(t *testing.T) {
	// string < number — incompatible types return false
	p, _ := Compile(`.a < .b`)
	got, err := p.Run([]byte(`{"a":"hello","b":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "false" {
		t.Errorf("got %s, want false (cross-type)", got)
	}
}

func TestSelectWithOrderingOperator(t *testing.T) {
	p, _ := Compile(`select(.latency > 500)`)
	input := []byte(`{"latency":750,"path":"/api"}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(input) {
		t.Errorf("got %s", got)
	}
	results, err := p.RunAll([]byte(`{"latency":100}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for low latency")
	}
}

func TestComplexFilter(t *testing.T) {
	// Realistic log filter: errors with high latency from a specific service
	p, _ := Compile(`select(.level == "error" and .latency > 500 and .service == "api")`)
	match := []byte(`{"level":"error","latency":750,"service":"api"}`)
	got, err := p.Run(match)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(match) {
		t.Errorf("got %s", got)
	}
	noMatch := []byte(`{"level":"error","latency":100,"service":"api"}`)
	results, err := p.RunAll(noMatch)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results")
	}
}

// --- Output larger than input ---

// type on a minimal object produces an 8-byte string from a 2-byte input.
func TestOutputLargerType(t *testing.T) {
	p, _ := Compile("type")
	input := []byte(`{}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `"object"` {
		t.Errorf("got %s, want \"object\"", got)
	}
	if len(got) <= len(input) {
		t.Errorf("expected output (%d bytes) to be larger than input (%d bytes)", len(got), len(input))
	}
}

// Alternative fallback to a literal larger than the input.
func TestOutputLargerAlternative(t *testing.T) {
	p, _ := Compile(`.x // "this fallback is much longer than the input"`)
	input := []byte(`{}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	want := `"this fallback is much longer than the input"`
	if string(got) != want {
		t.Errorf("got %s, want %s", got, want)
	}
	if len(got) <= len(input) {
		t.Errorf("expected output (%d bytes) to be larger than input (%d bytes)", len(got), len(input))
	}
}

// Construction with literal values larger than the whole input.
func TestOutputLargerConstruct(t *testing.T) {
	p, _ := Compile(`{status: "operational", region: "us-east-1", env: "production"}`)
	input := []byte(`{}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"status":"operational","region":"us-east-1","env":"production"}`
	if string(got) != want {
		t.Errorf("got %s, want %s", got, want)
	}
	if len(got) <= len(input) {
		t.Errorf("expected output (%d bytes) to be larger than input (%d bytes)", len(got), len(input))
	}
}

// RunWithBuffer grows correctly when output > initial buffer capacity,
// and stabilises (no realloc) on subsequent calls with the grown buffer.
func TestOutputLargerRunWithBuffer(t *testing.T) {
	p, _ := Compile(`{status: "operational", region: "us-east-1", env: "production"}`)
	input := []byte(`{}`)
	want := `{"status":"operational","region":"us-east-1","env":"production"}`

	// Start with a 1-byte buffer — well under the output size.
	buf := make([]byte, 0, 1)

	got, err := p.RunWithBuffer(input, buf)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Errorf("first call: got %s, want %s", got, want)
	}

	// Reuse the (now-grown) buffer for a second call.
	got2, err := p.RunWithBuffer(input, got[:0])
	if err != nil {
		t.Fatal(err)
	}
	if string(got2) != want {
		t.Errorf("second call: got %s, want %s", got2, want)
	}
}

// --- Pretty-printed input ---

func TestPrettyPrintedFieldAccess(t *testing.T) {
	p, _ := Compile(".name")
	input := []byte("{\n  \"name\": \"alice\",\n  \"age\": 30\n}")
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `"alice"` {
		t.Errorf("got %s, want \"alice\"", got)
	}
}

func TestPrettyPrintedIterator(t *testing.T) {
	p, _ := Compile(".[]")
	input := []byte("[\n  1,\n  2,\n  3\n]")
	results, err := p.RunAll(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
}

func TestPrettyPrintedSelect(t *testing.T) {
	p, _ := Compile(`select(.level == "error")`)
	input := []byte("{\n  \"level\": \"error\",\n  \"msg\": \"boom\"\n}")
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(input) {
		t.Errorf("select on pretty-printed input should return input verbatim")
	}
}

func TestPrettyPrintedDelete(t *testing.T) {
	p, _ := Compile("del(.age)")
	input := []byte("{\n  \"name\": \"alice\",\n  \"age\": 30\n}")
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"name":"alice"}` {
		t.Errorf("got %s, want {\"name\":\"alice\"}", got)
	}
}

// --- Del edge cases ---

func TestDeleteNestedFieldMissingParent(t *testing.T) {
	// del(.foo.bar) when .foo doesn't exist — should be a no-op
	p, _ := Compile("del(.foo.bar)")
	input := []byte(`{"a":1,"b":2}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"a":1,"b":2}` {
		t.Errorf("got %s, want no-op result", got)
	}
}

func TestDeleteOnlyField(t *testing.T) {
	p, _ := Compile("del(.x)")
	input := []byte(`{"x":1}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{}` {
		t.Errorf("got %s, want {}", got)
	}
}

func TestDeleteArrayOutOfBounds(t *testing.T) {
	// del(.[5]) on a 3-element array — should be a no-op
	p, _ := Compile("del(.[5])")
	input := []byte(`[1,2,3]`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `[1,2,3]` {
		t.Errorf("got %s, want [1,2,3]", got)
	}
}

func TestDeleteFieldWithSpecialChars(t *testing.T) {
	// Field value contains chars that look like JSON structure
	p, _ := Compile("del(.noise)")
	input := []byte(`{"keep":"{\"nested\":true}","noise":"drop"}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"keep":"{\"nested\":true}"}` {
		t.Errorf("got %s", got)
	}
}

// --- Alternative edge cases ---

func TestAlternativeBothFalsy(t *testing.T) {
	// false // null — left is falsy, evaluates right (null)
	p, _ := Compile(`false // null`)
	got, err := p.Run([]byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "null" {
		t.Errorf("got %s, want null", got)
	}
}

func TestAlternativeNullFallsToFalse(t *testing.T) {
	// null // false — left is falsy, evaluates right (false)
	p, _ := Compile(`null // false`)
	got, err := p.Run([]byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "false" {
		t.Errorf("got %s, want false", got)
	}
}

func TestAlternativeZeroIsNotFalsy(t *testing.T) {
	// 0 is truthy in jq (only null and false are falsy)
	p, _ := Compile(`.count // 99`)
	got, err := p.Run([]byte(`{"count":0}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "0" {
		t.Errorf("got %s, want 0 (zero is truthy)", got)
	}
}

func TestAlternativeEmptyStringIsNotFalsy(t *testing.T) {
	p, _ := Compile(`.name // "default"`)
	got, err := p.Run([]byte(`{"name":""}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `""` {
		t.Errorf("got %s, want empty string (truthy)", got)
	}
}

// --- Select edge cases ---

func TestSelectNull(t *testing.T) {
	p, _ := Compile("select(null)")
	results, err := p.RunAll([]byte(`{"a":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, null is falsy")
	}
}

func TestSelectZeroIsTruthy(t *testing.T) {
	// 0 is truthy — select(0) should pass through
	p, _ := Compile("select(.count)")
	got, err := p.Run([]byte(`{"count":0}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"count":0}` {
		t.Errorf("got %s, want input (0 is truthy)", got)
	}
}

func TestSelectNotEqual(t *testing.T) {
	p, _ := Compile(`select(.level != "debug")`)
	input := []byte(`{"level":"error","msg":"boom"}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(input) {
		t.Errorf("got %s", got)
	}
	results, err := p.RunAll([]byte(`{"level":"debug","msg":"noise"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for debug level")
	}
}

// --- Comparison edge cases ---

func TestCompareBoolTrue(t *testing.T) {
	p, _ := Compile(`.active == true`)
	got, err := p.Run([]byte(`{"active":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "true" {
		t.Errorf("got %s, want true", got)
	}
}

func TestCompareBoolFalse(t *testing.T) {
	p, _ := Compile(`.active == false`)
	got, err := p.Run([]byte(`{"active":false}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "true" {
		t.Errorf("got %s, want true", got)
	}
}

func TestCompareNullField(t *testing.T) {
	p, _ := Compile(`.v == null`)
	// explicit null value
	got, err := p.Run([]byte(`{"v":null}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "true" {
		t.Errorf("got %s, want true for explicit null", got)
	}
	// missing field also returns null
	got2, err := p.Run([]byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got2) != "true" {
		t.Errorf("got %s, want true for missing field", got2)
	}
}

func TestCompareCrossTypeFalse(t *testing.T) {
	// string vs number — never equal
	p, _ := Compile(`.x == 1`)
	got, err := p.Run([]byte(`{"x":"1"}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "false" {
		t.Errorf("got %s, want false (string != number)", got)
	}
}

func TestCompareNegativeNumber(t *testing.T) {
	p, _ := Compile(`.x == -1`)
	got, err := p.Run([]byte(`{"x":-1}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "true" {
		t.Errorf("got %s, want true", got)
	}
}

// --- Constructor edge cases ---

func TestConstructWithLiteralLargerThanInput(t *testing.T) {
	// Explicit test that construction can exceed input size
	p, _ := Compile(`{tag: "this-is-a-long-tag-value", src: .id}`)
	input := []byte(`{"id":1}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"tag":"this-is-a-long-tag-value","src":1}`
	if string(got) != want {
		t.Errorf("got %s, want %s", got, want)
	}
	if len(got) <= len(input) {
		t.Errorf("expected output (%d) larger than input (%d)", len(got), len(input))
	}
}

func TestConstructMissingExprField(t *testing.T) {
	// {a: .missing} — missing field expression yields null
	p, _ := Compile(`{a: .missing, b: .present}`)
	got, err := p.Run([]byte(`{"present":42}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"a":null,"b":42}` {
		t.Errorf("got %s", got)
	}
}

func TestArrayConstructWithLiterals(t *testing.T) {
	p, _ := Compile(`[.name, "active", true]`)
	input := []byte(`{"name":"alice"}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `["alice","active",true]` {
		t.Errorf("got %s", got)
	}
	if len(got) <= len(input) {
		t.Errorf("expected output larger than input")
	}
}

// --- Arithmetic: -, *, /, % ---

func TestArithSubtract(t *testing.T) {
	assertQuery(t, `5 - 3`, `null`, `2`)
	assertQuery(t, `.a - .b`, `{"a":10,"b":3}`, `7`)
	assertQuery(t, `1.5 - 0.5`, `null`, `1`)
	assertQuery(t, `3 - 5`, `null`, `-2`)
}
func TestArithSubtractArray(t *testing.T) {
	// array difference
	assertQuery(t, `. - ["b","c"]`, `["a","b","c","d"]`, `["a","d"]`)
	assertQuery(t, `[1,2,3,2,1] - [2]`, `null`, `[1,3,1]`)
	assertQuery(t, `. - []`, `[1,2,3]`, `[1,2,3]`)
	assertQuery(t, `[] - [1]`, `null`, `[]`)
}
func TestArithMultiply(t *testing.T) {
	assertQuery(t, `3 * 4`, `null`, `12`)
	assertQuery(t, `2.5 * 4`, `null`, `10`)
	assertQuery(t, `.price * .qty`, `{"price":2.5,"qty":4}`, `10`)
}
func TestArithMultiplyStringRepeat(t *testing.T) {
	assertQuery(t, `"ab" * 3`, `null`, `"ababab"`)
	assertQuery(t, `"x" * 1`, `null`, `"x"`)
	assertQuery(t, `"x" * 0`, `null`, `""`)
}
func TestArithDivide(t *testing.T) {
	assertQuery(t, `10 / 4`, `null`, `2.5`)
	assertQuery(t, `10 / 2`, `null`, `5`)
	assertQuery(t, `7 / 2`, `null`, `3.5`)
}
func TestArithDivideStringSplit(t *testing.T) {
	// string / string = split
	assertQuery(t, `"a,b,c" / ","`, `null`, `["a","b","c"]`)
	assertQuery(t, `"hello" / "l"`, `null`, `["he","","o"]`)
}
func TestArithModulo(t *testing.T) {
	assertQuery(t, `10 % 3`, `null`, `1`)
	assertQuery(t, `7 % 7`, `null`, `0`)
	assertQuery(t, `-7 % 3`, `null`, `-1`)
}
func TestArithNullPropagation(t *testing.T) {
	assertQuery(t, `null - 5`, `null`, `null`)
	assertQuery(t, `5 * null`, `null`, `null`)
}
func TestArithPrecedence(t *testing.T) {
	// * binds tighter than +
	assertQuery(t, `1 + 2 * 3`, `null`, `7`)
	assertQuery(t, `2 * 3 + 4 * 5`, `null`, `26`)
	// - and + left-associative
	assertQuery(t, `10 - 3 - 2`, `null`, `5`)
}
func TestArithNegativeResult(t *testing.T) {
	// appendInt must handle negatives
	assertQuery(t, `1 - 10`, `null`, `-9`)
	assertQuery(t, `.a - .b`, `{"a":0,"b":5}`, `-5`)
}

// --- min / max / min_by / max_by ---

func TestMinNumbers(t *testing.T) {
	assertQuery(t, `min`, `[3,1,4,1,5,9,2,6]`, `1`)
	assertQuery(t, `min`, `[42]`, `42`)
	assertQuery(t, `min`, `[]`, `null`)
}
func TestMaxNumbers(t *testing.T) {
	assertQuery(t, `max`, `[3,1,4,1,5,9,2,6]`, `9`)
	assertQuery(t, `max`, `[42]`, `42`)
	assertQuery(t, `max`, `[]`, `null`)
}
func TestMinStrings(t *testing.T) {
	assertQuery(t, `min`, `["banana","apple","cherry"]`, `"apple"`)
	assertQuery(t, `max`, `["banana","apple","cherry"]`, `"cherry"`)
}
func TestMinBy(t *testing.T) {
	assertQuery(t, `min_by(.age)`,
		`[{"name":"alice","age":30},{"name":"bob","age":25},{"name":"carol","age":35}]`,
		`{"name":"bob","age":25}`)
}
func TestMaxBy(t *testing.T) {
	assertQuery(t, `max_by(.value)`,
		`[{"id":1,"value":10},{"id":2,"value":5},{"id":3,"value":20}]`,
		`{"id":3,"value":20}`)
}
func TestMinByNested(t *testing.T) {
	assertQuery(t, `min_by(.x) | .name`,
		`[{"name":"a","x":3},{"name":"b","x":1},{"name":"c","x":2}]`,
		`"b"`)
}

// --- @uri ---

func TestURIEncodeSimple(t *testing.T) {
	assertQuery(t, `@uri`, `"hello world"`, `"hello%20world"`)
}
func TestURIEncodeUnreserved(t *testing.T) {
	// unreserved chars pass through unchanged
	assertQuery(t, `@uri`, `"abc-._~"`, `"abc-._~"`)
}
func TestURIEncodeSpecial(t *testing.T) {
	assertQuery(t, `@uri`, `"a/b?c=d&e=f"`, `"a%2Fb%3Fc%3Dd%26e%3Df"`)
}
func TestURIEncodePath(t *testing.T) {
	assertQuery(t, `.path | @uri`, `{"path":"/api/v1/users"}`, `"%2Fapi%2Fv1%2Fusers"`)
}

// --- try / try-catch ---

func TestTrySuppressesError(t *testing.T) {
	assertNoOutput(t, `try .foo`, `[1,2,3]`) // field access on array → no output
}
func TestTrySucceeds(t *testing.T) {
	assertQuery(t, `try .foo`, `{"foo":42}`, `42`)
}
func TestTryInPipe(t *testing.T) {
	assertQuery(t, `try .a | .b`, `{"a":{"b":99}}`, `99`)
	assertNoOutput(t, `try .a | .b`, `[1,2,3]`)
}
func TestTryCatchLiteral(t *testing.T) {
	assertQuery(t, `try .foo catch "caught"`, `[1,2,3]`, `"caught"`)
}
func TestTryCatchPassthrough(t *testing.T) {
	p, _ := Compile(`try .foo catch .`)
	got, err := p.Run([]byte(`[1,2,3]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 || got[0] != '"' {
		t.Errorf("expected JSON string error message, got %s", got)
	}
}
func TestTryMapTolerant(t *testing.T) {
	p, _ := Compile(`[.[] | try .x]`)
	got, err := p.Run([]byte(`[{"x":1},42,{"x":3}]`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `[1,3]` {
		t.Errorf("got %s", got)
	}
}

// --- elif ---

func TestElif(t *testing.T) {
	q := `if .x == 1 then "one" elif .x == 2 then "two" else "other" end`
	assertQuery(t, q, `{"x":1}`, `"one"`)
	assertQuery(t, q, `{"x":2}`, `"two"`)
	assertQuery(t, q, `{"x":3}`, `"other"`)
}
func TestElifChain(t *testing.T) {
	q := `if .x == 1 then "one" elif .x == 2 then "two" elif .x == 3 then "three" else "other" end`
	assertQuery(t, q, `{"x":1}`, `"one"`)
	assertQuery(t, q, `{"x":2}`, `"two"`)
	assertQuery(t, q, `{"x":3}`, `"three"`)
	assertQuery(t, q, `{"x":9}`, `"other"`)
}
func TestElifNoElse(t *testing.T) {
	q := `if .x == 1 then "one" elif .x == 2 then "two" end`
	assertQuery(t, q, `{"x":1}`, `"one"`)
	assertQuery(t, q, `{"x":2}`, `"two"`)
	assertQuery(t, q, `{"x":3}`, `{"x":3}`) // no else → identity
}

// --- object merge ---

func TestPlusObjectMerge(t *testing.T) {
	assertQuery(t, `.a + .b`, `{"a":{"x":1},"b":{"y":2}}`, `{"x":1,"y":2}`)
}
func TestPlusObjectRightWins(t *testing.T) {
	assertQuery(t, `.a + .b`, `{"a":{"k":1},"b":{"k":2}}`, `{"k":2}`)
}
func TestPlusObjectLiteral(t *testing.T) {
	assertQuery(t, `{"a":1} + {"b":2}`, `null`, `{"a":1,"b":2}`)
}
func TestPlusObjectEmptyLeft(t *testing.T) {
	assertQuery(t, `{} + {"a":1}`, `null`, `{"a":1}`)
}
func TestPlusObjectEmptyRight(t *testing.T) {
	assertQuery(t, `{"a":1} + {}`, `null`, `{"a":1}`)
}
func TestPlusObjectNullLeft(t *testing.T) {
	assertQuery(t, `null + {"a":1}`, `null`, `{"a":1}`)
}

// --- tojson / fromjson / @json ---

func TestToJSON(t *testing.T) {
	assertQuery(t, `tojson`, `{"a":1}`, `"{\"a\":1}"`)
	assertQuery(t, `tojson`, `42`, `"42"`)
	assertQuery(t, `tojson`, `"hello"`, `"\"hello\""`)
	assertQuery(t, `tojson`, `null`, `"null"`)
	assertQuery(t, `tojson`, `true`, `"true"`)
	assertQuery(t, `tojson`, `[1,2,3]`, `"[1,2,3]"`)
}
func TestAtJSON(t *testing.T) {
	assertQuery(t, `@json`, `{"a":1}`, `"{\"a\":1}"`)
}
func TestFromJSON(t *testing.T) {
	assertQuery(t, `fromjson`, `"{\"a\":1}"`, `{"a":1}`)
	assertQuery(t, `fromjson`, `"42"`, `42`)
	assertQuery(t, `fromjson`, `"true"`, `true`)
	assertQuery(t, `fromjson`, `"[1,2,3]"`, `[1,2,3]`)
}
func TestToFromJSONRoundTrip(t *testing.T) {
	assertQuery(t, `tojson | fromjson`, `{"a":1,"b":"hello"}`, `{"a":1,"b":"hello"}`)
	assertQuery(t, `tojson | fromjson`, `[1,2,3]`, `[1,2,3]`)
}

// --- tostring / tonumber ---

func TestToString(t *testing.T) {
	assertQuery(t, `tostring`, `"hello"`, `"hello"`)
	assertQuery(t, `tostring`, `42`, `"42"`)
	assertQuery(t, `tostring`, `true`, `"true"`)
	assertQuery(t, `tostring`, `null`, `"null"`)
	assertQuery(t, `tostring`, `{"a":1}`, `"{\"a\":1}"`)
}
func TestToNumber(t *testing.T) {
	assertQuery(t, `tonumber`, `42`, `42`)
	assertQuery(t, `tonumber`, `3.14`, `3.14`)
	assertQuery(t, `tonumber`, `"42"`, `42`)
	assertQuery(t, `tonumber`, `"3.14"`, `3.14`)
	assertQuery(t, `tonumber`, `"-5"`, `-5`)
}
func TestToNumberError(t *testing.T) {
	p, _ := Compile(`tonumber`)
	_, err := p.Run([]byte(`"hello"`))
	if err == nil {
		t.Error("expected error for tonumber on non-numeric string")
	}
}
func TestToStringTonumberRoundTrip(t *testing.T) {
	assertQuery(t, `tostring | tonumber`, `42`, `42`)
}

// --- any(gen; cond) / all(gen; cond) ---

func TestAnyTwoArg(t *testing.T) {
	assertQuery(t, `any(.[]; . > 3)`, `[1,2,3,4,5]`, `true`)
	assertQuery(t, `any(.[]; . > 10)`, `[1,2,3,4,5]`, `false`)
}
func TestAllTwoArg(t *testing.T) {
	assertQuery(t, `all(.[]; . > 0)`, `[1,2,3]`, `true`)
	assertQuery(t, `all(.[]; . > 2)`, `[1,2,3]`, `false`)
}
func TestAnyTwoArgEmpty(t *testing.T) {
	assertQuery(t, `any(.[]; . > 0)`, `[]`, `false`)
}
func TestAllTwoArgEmpty(t *testing.T) {
	assertQuery(t, `all(.[]; . > 0)`, `[]`, `true`)
}
func TestAnyTwoArgGenerator(t *testing.T) {
	assertQuery(t, `any(.items[]; .active)`,
		`{"items":[{"active":false},{"active":true},{"active":false}]}`, `true`)
}
