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
