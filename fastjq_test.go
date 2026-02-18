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
