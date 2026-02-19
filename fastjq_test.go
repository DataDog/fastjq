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

// --- Literals ---

func TestLiteralNull(t *testing.T) {
	p, err := Compile("null")
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Run([]byte(`{"a":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "null" {
		t.Errorf("got %s, want null", got)
	}
}

func TestLiteralTrue(t *testing.T) {
	p, err := Compile("true")
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Run([]byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "true" {
		t.Errorf("got %s, want true", got)
	}
}

func TestLiteralFalse(t *testing.T) {
	p, err := Compile("false")
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Run([]byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "false" {
		t.Errorf("got %s, want false", got)
	}
}

func TestLiteralString(t *testing.T) {
	p, err := Compile(`"hello"`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Run([]byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `"hello"` {
		t.Errorf("got %s, want %q", got, `"hello"`)
	}
}

func TestLiteralInteger(t *testing.T) {
	p, err := Compile("42")
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Run([]byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "42" {
		t.Errorf("got %s, want 42", got)
	}
}

func TestLiteralFloat(t *testing.T) {
	p, err := Compile("3.14")
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Run([]byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "3.14" {
		t.Errorf("got %s, want 3.14", got)
	}
}

func TestLiteralNegative(t *testing.T) {
	p, err := Compile("-5")
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Run([]byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "-5" {
		t.Errorf("got %s, want -5", got)
	}
}

// --- Comparisons ---

func TestCompareStringEqual(t *testing.T) {
	p, err := Compile(`.name == "alice"`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Run([]byte(`{"name":"alice"}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "true" {
		t.Errorf("got %s, want true", got)
	}
}

func TestCompareStringNotEqual(t *testing.T) {
	p, err := Compile(`.name == "bob"`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Run([]byte(`{"name":"alice"}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "false" {
		t.Errorf("got %s, want false", got)
	}
}

func TestCompareNotEqualOp(t *testing.T) {
	p, err := Compile(`.age != 30`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Run([]byte(`{"age":25}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "true" {
		t.Errorf("got %s, want true", got)
	}
}

func TestCompareNull(t *testing.T) {
	p, err := Compile(`.x == null`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Run([]byte(`{"y":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "true" {
		t.Errorf("got %s, want true", got)
	}
}

func TestCompareLiteralStrings(t *testing.T) {
	p, err := Compile(`"a" == "a"`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Run([]byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "true" {
		t.Errorf("got %s, want true", got)
	}
}

func TestCompareLiteralStringsNotEqual(t *testing.T) {
	p, err := Compile(`"a" != "b"`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Run([]byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "true" {
		t.Errorf("got %s, want true", got)
	}
}

func TestCompareFields(t *testing.T) {
	p, err := Compile(`.x == .y`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Run([]byte(`{"x":1,"y":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "true" {
		t.Errorf("got %s, want true", got)
	}
}

func TestCompareNumberFloat(t *testing.T) {
	p, err := Compile(`1.0 == 1`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Run([]byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "true" {
		t.Errorf("got %s, want true", got)
	}
}

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

func TestTypeString(t *testing.T) {
	p, err := Compile("type")
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Run([]byte(`"hello"`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `"string"` {
		t.Errorf("got %s, want %q", got, `"string"`)
	}
}

func TestTypeNumber(t *testing.T) {
	p, err := Compile("type")
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Run([]byte(`42`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `"number"` {
		t.Errorf("got %s, want %q", got, `"number"`)
	}
}

func TestTypeObject(t *testing.T) {
	p, err := Compile("type")
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Run([]byte(`{"a":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `"object"` {
		t.Errorf("got %s, want %q", got, `"object"`)
	}
}

func TestTypeArray(t *testing.T) {
	p, err := Compile("type")
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Run([]byte(`[1,2]`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `"array"` {
		t.Errorf("got %s, want %q", got, `"array"`)
	}
}

func TestTypeBoolean(t *testing.T) {
	p, err := Compile("type")
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Run([]byte(`true`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `"boolean"` {
		t.Errorf("got %s, want %q", got, `"boolean"`)
	}
}

func TestTypeNull(t *testing.T) {
	p, err := Compile("type")
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Run([]byte(`null`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `"null"` {
		t.Errorf("got %s, want %q", got, `"null"`)
	}
}

func TestTypePiped(t *testing.T) {
	p, err := Compile(".value | type")
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Run([]byte(`{"value":"hello"}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `"string"` {
		t.Errorf("got %s, want %q", got, `"string"`)
	}
}

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
