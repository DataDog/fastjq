package fastjq

import (
	"testing"
)

func TestIdentity(t *testing.T) {
	p, err := Compile(".")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{"name":"alice","age":30}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(input) {
		t.Errorf("got %s, want %s", got, input)
	}
}

func TestFieldAccess(t *testing.T) {
	p, err := Compile(".name")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{"name":"alice","age":30}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `"alice"` {
		t.Errorf("got %s, want %q", got, `"alice"`)
	}
}

func TestFieldAccessNumber(t *testing.T) {
	p, err := Compile(".age")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{"name":"alice","age":30}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "30" {
		t.Errorf("got %s, want 30", got)
	}
}

func TestFieldAccessNested(t *testing.T) {
	p, err := Compile(".address.city")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{"name":"alice","address":{"city":"NYC","zip":"10001"}}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `"NYC"` {
		t.Errorf("got %s, want %q", got, `"NYC"`)
	}
}

func TestFieldAccessMissing(t *testing.T) {
	p, err := Compile(".missing")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{"name":"alice"}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "null" {
		t.Errorf("got %s, want null", got)
	}
}

func TestDeleteSingle(t *testing.T) {
	p, err := Compile("del(.age)")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{"name":"alice","age":30,"city":"NYC"}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	expected := `{"name":"alice","city":"NYC"}`
	if string(got) != expected {
		t.Errorf("got %s, want %s", got, expected)
	}
}

func TestDeleteMultiple(t *testing.T) {
	p, err := Compile("del(.age, .city)")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{"name":"alice","age":30,"city":"NYC"}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	expected := `{"name":"alice"}`
	if string(got) != expected {
		t.Errorf("got %s, want %s", got, expected)
	}
}

func TestDeleteFirst(t *testing.T) {
	p, err := Compile("del(.name)")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{"name":"alice","age":30}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	expected := `{"age":30}`
	if string(got) != expected {
		t.Errorf("got %s, want %s", got, expected)
	}
}

func TestDeleteLast(t *testing.T) {
	p, err := Compile("del(.age)")
	if err != nil {
		t.Fatal(err)
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

func TestDeleteAll(t *testing.T) {
	p, err := Compile("del(.name, .age)")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{"name":"alice","age":30}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	expected := `{}`
	if string(got) != expected {
		t.Errorf("got %s, want %s", got, expected)
	}
}

func TestDeleteNested(t *testing.T) {
	p, err := Compile("del(.address.zip)")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{"name":"alice","address":{"city":"NYC","zip":"10001"}}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	expected := `{"name":"alice","address":{"city":"NYC"}}`
	if string(got) != expected {
		t.Errorf("got %s, want %s", got, expected)
	}
}

func TestDeleteNonexistent(t *testing.T) {
	p, err := Compile("del(.missing)")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{"name":"alice","age":30}`)
	got, err := p.Run(input)
	if err != nil {
		t.Fatal(err)
	}
	expected := `{"name":"alice","age":30}`
	if string(got) != expected {
		t.Errorf("got %s, want %s", got, expected)
	}
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
