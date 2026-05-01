package fastjq

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/itchyny/gojq"
)

// --- Input generators ---

// generateJSON creates a JSON object with n keys, each with a string value.
func generateJSON(n int, valueSize int) []byte {
	var b strings.Builder
	b.WriteString("{")
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `"field_%d":"%s"`, i, strings.Repeat("x", valueSize))
	}
	b.WriteString("}")
	return []byte(b.String())
}

// generateNestedJSON creates a JSON object with top-level and nested fields.
func generateNestedJSON(topKeys, nestedKeys, valueSize int) []byte {
	var b strings.Builder
	b.WriteString("{")
	for i := 0; i < topKeys; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		if i%3 == 0 && nestedKeys > 0 {
			// nested object
			fmt.Fprintf(&b, `"field_%d":{`, i)
			for j := 0; j < nestedKeys; j++ {
				if j > 0 {
					b.WriteString(",")
				}
				fmt.Fprintf(&b, `"sub_%d":"%s"`, j, strings.Repeat("y", valueSize))
			}
			b.WriteString("}")
		} else {
			fmt.Fprintf(&b, `"field_%d":"%s"`, i, strings.Repeat("x", valueSize))
		}
	}
	b.WriteString("}")
	return []byte(b.String())
}

// generateObjectArray creates a JSON array of n objects, each with name/value/active fields.
func generateObjectArray(n int) []byte {
	var b strings.Builder
	b.WriteString("[")
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		active := "true"
		if i%2 != 0 {
			active = "false"
		}
		fmt.Fprintf(&b, `{"name":"item_%d","value":%d,"active":%s}`, i, i*10, active)
	}
	b.WriteString("]")
	return []byte(b.String())
}

// generateIntArray creates a JSON array of n sequential integers [0, 1, ..., n-1].
func generateIntArray(n int) []byte {
	var b strings.Builder
	b.WriteString("[")
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, "%d", i)
	}
	b.WriteString("]")
	return []byte(b.String())
}

// --- Shared inputs ---

var (
	smallJSON   = generateJSON(5, 10)              // ~100B
	mediumJSON  = generateNestedJSON(20, 5, 30)    // ~2KB
	largeJSON   = generateNestedJSON(200, 10, 200) // ~100KB+
	smallArray  = generateObjectArray(20)          // ~600B, 20 objects
	mediumArray = generateObjectArray(100)         // ~3KB, 100 objects
	largeArray  = generateObjectArray(200)         // ~6KB, 200 objects
	largeIntArr = generateIntArray(200)            // [0..199], for numeric array benchmarks
	// largeBoolArr: 199 nulls + 1 truthy value — worst-case scan for any
	largeBoolArr = func() []byte {
		b := make([]byte, 0, 512)
		b = append(b, '[')
		for i := 0; i < 199; i++ {
			b = append(b, "null,"...)
		}
		return append(b, "1]"...)
	}()
)

// benchSink prevents the compiler from eliminating json.Marshal calls via dead-code
// elimination, which would make large-input gojq benchmarks appear identical to small ones.
var benchSink []byte

// --- Benchmark helpers ---

// benchFastjqObj is the standard fastjq benchmark pattern: compile once, run RunWithBuffer
// in a loop reusing buf. Safe for all operations where output ≠ input (del, field, construct…).
func benchFastjqObj(b *testing.B, query string, input []byte) {
	p, _ := Compile(query)
	buf := make([]byte, 0, len(input))
	b.ReportAllocs()
	for b.Loop() {
		buf, _ = p.RunWithBuffer(input, buf)
	}
}

// benchFastjqLargeSelect benchmarks a select-style query on large JSON with input rotation.
// Rotates 8 distinct inputs and passes scratch[:0] (discarding return) to prevent
// buf being reassigned to an input sub-slice, which would corrupt rotation inputs.
func benchFastjqLargeSelect(b *testing.B, query string) {
	p, _ := Compile(query)
	const n = 8
	inputs := make([][]byte, n)
	for i := range inputs {
		inputs[i] = generateNestedJSON(200, 10, 200)
	}
	scratch := make([]byte, 0, len(largeJSON))
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		_, _ = p.RunWithBuffer(inputs[i%n], scratch[:0])
		i++
	}
}

// benchFastjqFunc benchmarks a query using RunFunc with a no-op callback.
// Use for iterator / limit / multi-output operations.
func benchFastjqFunc(b *testing.B, query string, input []byte) {
	p, _ := Compile(query)
	b.ReportAllocs()
	for b.Loop() {
		p.RunFunc(input, func(_ []byte) error { return nil })
	}
}

// benchGojqObj is the standard gojq benchmark pattern: unmarshal → run → marshal.
func benchGojqObj(b *testing.B, query string, input []byte) {
	q, _ := gojq.Parse(query)
	code, _ := gojq.Compile(q)
	b.ReportAllocs()
	for b.Loop() {
		var v any
		json.Unmarshal(input, &v)
		iter := code.Run(v)
		result, _ := iter.Next()
		benchSink, _ = json.Marshal(result)
	}
}

// benchGojqLargeRot benchmarks a gojq query on large JSON with input rotation.
// All gojq large-JSON benchmarks need rotation to prevent calibration artifacts.
func benchGojqLargeRot(b *testing.B, query string) {
	q, _ := gojq.Parse(query)
	code, _ := gojq.Compile(q)
	const n = 8
	inputs := make([][]byte, n)
	for i := range inputs {
		inputs[i] = generateNestedJSON(200, 10, 200)
	}
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		var v any
		json.Unmarshal(inputs[i%n], &v)
		iter := code.Run(v)
		result, _ := iter.Next()
		benchSink, _ = json.Marshal(result)
		i++
	}
}

// benchGojqIter benchmarks a gojq query that produces multiple outputs (consumes all).
func benchGojqIter(b *testing.B, query string, input []byte) {
	q, _ := gojq.Parse(query)
	code, _ := gojq.Compile(q)
	b.ReportAllocs()
	for b.Loop() {
		var v any
		json.Unmarshal(input, &v)
		iter := code.Run(v)
		for {
			_, ok := iter.Next()
			if !ok {
				break
			}
		}
	}
}

// --- fastjq benchmarks ---

func BenchmarkFastjq_Small_Del(b *testing.B)   { benchFastjqObj(b, "del(.field_2)", smallJSON) }
func BenchmarkFastjq_Medium_Del(b *testing.B)  { benchFastjqObj(b, "del(.field_5)", mediumJSON) }
func BenchmarkFastjq_Large_Del(b *testing.B)   { benchFastjqObj(b, "del(.field_50)", largeJSON) }
func BenchmarkFastjq_Small_Field(b *testing.B) { benchFastjqObj(b, ".field_2", smallJSON) }
func BenchmarkFastjq_Large_Field(b *testing.B) { benchFastjqObj(b, ".field_50", largeJSON) }
func BenchmarkFastjq_Small_Index(b *testing.B) { benchFastjqObj(b, ".[2]", []byte(`[1,2,3,4,5]`)) }
func BenchmarkFastjq_Small_ArrayDel(b *testing.B) {
	benchFastjqObj(b, "del(.[1], .[3])", []byte(`[1,2,3,4,5]`))
}
func BenchmarkFastjq_Small_Construct(b *testing.B) {
	benchFastjqObj(b, "{field_0, field_2}", smallJSON)
}
func BenchmarkFastjq_Large_Construct(b *testing.B) {
	benchFastjqObj(b, "{field_0, field_50}", largeJSON)
}
func BenchmarkFastjq_Small_Iterator(b *testing.B) {
	benchFastjqFunc(b, ".[]", []byte(`[1,2,3,4,5]`))
}
func BenchmarkFastjq_Large_Iterator(b *testing.B) {
	// 200-element array of objects
	var items strings.Builder
	items.WriteString("[")
	for i := 0; i < 200; i++ {
		if i > 0 {
			items.WriteString(",")
		}
		fmt.Fprintf(&items, `{"id":%d,"value":"%s"}`, i, strings.Repeat("x", 50))
	}
	items.WriteString("]")
	benchFastjqFunc(b, ".[]", []byte(items.String()))
}
func BenchmarkFastjq_Small_Select(b *testing.B) {
	benchFastjqObj(b, `select(.field_2 == "xxxxxxxxxx")`, smallJSON)
}
func BenchmarkFastjq_Large_Select(b *testing.B) {
	// field_199 = last field; rotation prevents buf-reassignment corruption
	benchFastjqLargeSelect(b, `select(.field_199 == "`+strings.Repeat("x", 200)+`")`)
}
func BenchmarkFastjq_Small_Alternative(b *testing.B) {
	benchFastjqObj(b, `.field_2 // "default"`, smallJSON)
}
func BenchmarkFastjq_Small_SelectAnd(b *testing.B) {
	benchFastjqObj(b, `select(.field_0 == "xxxxxxxxxx" and .field_1 == "xxxxxxxxxx")`, smallJSON)
}
func BenchmarkFastjq_Small_SelectOr(b *testing.B) {
	benchFastjqObj(b, `select(.field_0 == "xxxxxxxxxx" or .field_4 == "xxxxxxxxxx")`, smallJSON)
}
func BenchmarkFastjq_Small_Has(b *testing.B) {
	benchFastjqObj(b, `select(has("field_2"))`, smallJSON)
}
func BenchmarkFastjq_Large_Has(b *testing.B) {
	benchFastjqLargeSelect(b, `select(has("field_199"))`)
}
func BenchmarkFastjq_Small_IfThenElse(b *testing.B) {
	benchFastjqObj(b, `if .field_0 == "xxxxxxxxxx" then .field_0 else "default" end`, smallJSON)
}
func BenchmarkFastjq_Small_Length(b *testing.B) { benchFastjqObj(b, `.field_0 | length`, smallJSON) }
func BenchmarkFastjq_Large_Length(b *testing.B) { benchFastjqObj(b, `length`, largeJSON) }
func BenchmarkFastjq_Small_Map(b *testing.B)    { benchFastjqObj(b, `map(.name)`, smallArray) }
func BenchmarkFastjq_Medium_Map(b *testing.B)   { benchFastjqObj(b, `map(.name)`, mediumArray) }
func BenchmarkFastjq_Large_Map(b *testing.B)    { benchFastjqObj(b, `map(.name)`, largeArray) }
func BenchmarkFastjq_Small_ToEntries(b *testing.B) {
	benchFastjqObj(b, `to_entries`, smallJSON)
}
func BenchmarkFastjq_Large_ToEntries(b *testing.B) {
	benchFastjqObj(b, `to_entries`, largeJSON)
}
func BenchmarkFastjq_Small_KeysUnsorted(b *testing.B) {
	benchFastjqObj(b, `keys_unsorted`, smallJSON)
}
func BenchmarkFastjq_Small_Keys(b *testing.B) {
	benchFastjqObj(b, `keys`, smallJSON)
}
func BenchmarkFastjq_Small_Paths(b *testing.B) {
	benchFastjqFunc(b, `paths`, smallJSON)
}
func BenchmarkFastjq_Small_RecursiveDescent(b *testing.B) {
	benchFastjqFunc(b, `..`, smallJSON)
}
func BenchmarkFastjq_Small_Path(b *testing.B) {
	benchFastjqObj(b, `path(.field_0)`, smallJSON)
}
func BenchmarkFastjq_Small_GetPath(b *testing.B) {
	benchFastjqObj(b, `getpath(["field_0"])`, smallJSON)
}
func BenchmarkFastjq_Small_SetPath(b *testing.B) {
	benchFastjqObj(b, `setpath(["field_0"]; "y")`, smallJSON)
}
func BenchmarkFastjq_Small_DelPaths(b *testing.B) {
	benchFastjqObj(b, `delpaths([["field_0"]])`, smallJSON)
}
func BenchmarkFastjq_Large_KeysUnsorted(b *testing.B) {
	benchFastjqObj(b, `keys_unsorted`, largeJSON)
}
func BenchmarkFastjq_Small_Any(b *testing.B) {
	benchFastjqObj(b, `any`, []byte(`[false,false,true,false,false]`))
}
func BenchmarkFastjq_Large_Any(b *testing.B) { benchFastjqObj(b, `any`, largeBoolArr) }
func BenchmarkFastjq_Small_AnyExpr(b *testing.B) {
	benchFastjqObj(b, `any(. == "xxxxxxxxxx")`, []byte(`["aaaa","bbbb","xxxxxxxxxx","cccc","dddd"]`))
}
func BenchmarkFastjq_Large_AnyExpr(b *testing.B) {
	benchFastjqObj(b, `any(. > 100)`, largeIntArr)
}
func BenchmarkFastjq_Small_Add(b *testing.B) {
	benchFastjqObj(b, `add`, []byte(`[1,2,3,4,5]`))
}
func BenchmarkFastjq_Small_AddStrings(b *testing.B) {
	benchFastjqObj(b, `add`, []byte(`["field_0","field_1","field_2","field_3","field_4"]`))
}
func BenchmarkFastjq_Small_Flatten(b *testing.B) {
	benchFastjqObj(b, `flatten`, []byte(`[[1,2],[3,[4,5]],[6]]`))
}
func BenchmarkFastjq_Small_Split(b *testing.B) {
	benchFastjqObj(b, `split(",")`, []byte(`"field_0,field_1,field_2,field_3,field_4"`))
}
func BenchmarkFastjq_Small_Join(b *testing.B) {
	benchFastjqObj(b, `join(",")`, []byte(`["field_0","field_1","field_2","field_3","field_4"]`))
}

func BenchmarkFastjq_Small_Values(b *testing.B) {
	// filter nulls from stream
	benchFastjqFunc(b, `.[] | values`, []byte(`[1,null,2,null,3,null,4,null,5]`))
}
func BenchmarkGojq_Small_Values(b *testing.B) {
	benchGojqIter(b, `.[] | values`, []byte(`[1,null,2,null,3,null,4,null,5]`))
}

func BenchmarkFastjq_Small_Base64Encode(b *testing.B) {
	benchFastjqObj(b, `@base64`, []byte(`"hello world from fastjq benchmark"`))
}
func BenchmarkFastjq_Small_Base64Decode(b *testing.B) {
	benchFastjqObj(b, `@base64d`, []byte(`"aGVsbG8gd29ybGQgZnJvbSBmYXN0anEgYmVuY2htYXJr"`))
}
func BenchmarkGojq_Small_Base64Encode(b *testing.B) {
	benchGojqObj(b, `@base64`, []byte(`"hello world from fastjq benchmark"`))
}
func BenchmarkGojq_Small_Base64Decode(b *testing.B) {
	benchGojqObj(b, `@base64d`, []byte(`"aGVsbG8gd29ybGQgZnJvbSBmYXN0anEgYmVuY2htYXJr"`))
}

func BenchmarkFastjq_Small_IndexFind(b *testing.B) {
	benchFastjqObj(b, `index(",")`, []byte(`"field_0,field_1,field_2,field_3,field_4"`))
}
func BenchmarkFastjq_Small_IndicesAll(b *testing.B) {
	benchFastjqObj(b, `indices(",")`, []byte(`"field_0,field_1,field_2,field_3,field_4"`))
}

func BenchmarkGojq_Small_IndexFind(b *testing.B) {
	benchGojqObj(b, `index(",")`, []byte(`"field_0,field_1,field_2,field_3,field_4"`))
}
func BenchmarkGojq_Small_IndicesAll(b *testing.B) {
	benchGojqObj(b, `indices(",")`, []byte(`"field_0,field_1,field_2,field_3,field_4"`))
}

func BenchmarkFastjq_Small_Slice(b *testing.B) {
	benchFastjqObj(b, `.[1:4]`, []byte(`[0,1,2,3,4,5]`))
}
func BenchmarkFastjq_Small_SliceString(b *testing.B) {
	benchFastjqObj(b, `.[:5]`, []byte(`"hello world from fastjq"`))
}
func BenchmarkFastjq_Small_Plus(b *testing.B) {
	benchFastjqObj(b, `.a + .b`, []byte(`{"a":"foo","b":"bar"}`))
}
func BenchmarkFastjq_Small_PlusStr(b *testing.B) {
	benchFastjqObj(b, `.prefix + .name`,
		[]byte(`{"prefix":"user_","name":"alice"}`))
}

func BenchmarkFastjq_Small_AsciiDowncase(b *testing.B) {
	benchFastjqObj(b, `select(.field_2 | ascii_downcase == "xxxxxxxxxx")`, smallJSON)
}
func BenchmarkFastjq_Large_AsciiDowncase(b *testing.B) {
	benchFastjqLargeSelect(b, `select(.field_199 | ascii_downcase == "`+strings.Repeat("x", 200)+`")`)
}
func BenchmarkFastjq_Small_Startswith(b *testing.B) {
	benchFastjqObj(b, `select(.field_2 | startswith("xxx"))`, smallJSON)
}
func BenchmarkFastjq_Large_Startswith(b *testing.B) {
	benchFastjqLargeSelect(b, `select(.field_199 | startswith("xxx"))`)
}
func BenchmarkFastjq_Small_Endswith(b *testing.B) {
	benchFastjqObj(b, `select(.field_2 | endswith("xxx"))`, smallJSON)
}
func BenchmarkFastjq_Small_Ltrimstr(b *testing.B) {
	benchFastjqObj(b, `.field_2 | ltrimstr("xxx")`, smallJSON)
}
func BenchmarkFastjq_Large_Ltrimstr(b *testing.B) {
	benchFastjqObj(b, `.field_199 | ltrimstr("xxx")`, largeJSON)
}
func BenchmarkFastjq_Small_Rtrimstr(b *testing.B) {
	benchFastjqObj(b, `.field_2 | rtrimstr("xxx")`, smallJSON)
}
func BenchmarkFastjq_Small_Trimstr(b *testing.B) {
	benchFastjqObj(b, `trimstr("xxx")`, []byte(`"xxhelloxxx"`))
}
func BenchmarkFastjq_Small_Trim(b *testing.B) {
	benchFastjqObj(b, `trim`, []byte(`"  abc  "`))
}
func BenchmarkFastjq_Small_Ltrim(b *testing.B) {
	benchFastjqObj(b, `ltrim`, []byte(`"  abc  "`))
}
func BenchmarkFastjq_Small_Rtrim(b *testing.B) {
	benchFastjqObj(b, `rtrim`, []byte(`"  abc  "`))
}
func BenchmarkFastjq_Small_UTF8ByteLength(b *testing.B) {
	benchFastjqObj(b, `utf8bytelength`, []byte(`"asdf\u03bc"`))
}
func BenchmarkFastjq_Small_Reverse(b *testing.B) {
	benchFastjqObj(b, `reverse`, []byte(`[1,2,3,4,5]`))
}
func BenchmarkFastjq_Small_First(b *testing.B) {
	benchFastjqObj(b, `first(.[] | select(. > 2))`, []byte(`[1,2,3,4,5]`))
}
func BenchmarkFastjq_Large_First(b *testing.B) {
	benchFastjqObj(b, `first(.[] | select(. > 100))`, largeIntArr)
}
func BenchmarkFastjq_Small_Last(b *testing.B) {
	benchFastjqObj(b, `last(.[] | select(. > 2))`, []byte(`[1,2,3,4,5]`))
}
func BenchmarkFastjq_Large_Last(b *testing.B) {
	benchFastjqObj(b, `last(.[] | select(. > 100))`, largeIntArr)
}
func BenchmarkFastjq_Small_Limit(b *testing.B) {
	benchFastjqFunc(b, `limit(3; .[])`, []byte(`[10,20,30,40,50]`))
}
func BenchmarkFastjq_Small_Skip(b *testing.B) {
	benchFastjqFunc(b, `skip(2; .[])`, []byte(`[10,20,30,40,50]`))
}
func BenchmarkFastjq_Small_Reduce(b *testing.B) {
	benchFastjqObj(b, `reduce .[] as $x (0; . + $x)`, []byte(`[1,2,3,4,5]`))
}
func BenchmarkFastjq_Small_Foreach(b *testing.B) {
	benchFastjqFunc(b, `foreach .[] as $x (0; . + $x)`, []byte(`[1,2,3,4,5]`))
}
func BenchmarkFastjq_Small_While(b *testing.B) {
	benchFastjqFunc(b, `while(.<100; .*2)`, []byte(`1`))
}
func BenchmarkFastjq_Small_Until(b *testing.B) {
	benchFastjqObj(b, `[.,1]|until(.[0] < 1; [.[0] - 1, .[1] * .[0]])|.[1]`, []byte(`5`))
}
func BenchmarkFastjq_Small_Bsearch(b *testing.B) {
	benchFastjqObj(b, `bsearch(42)`, []byte(`[1,10,20,30,40,42,50]`))
}
func BenchmarkFastjq_Small_Pick(b *testing.B) {
	benchFastjqObj(b, `pick(.field_0, .field_2)`, smallJSON)
}
func BenchmarkFastjq_Small_IN(b *testing.B) {
	benchFastjqObj(b, `5 | IN(range(10))`, []byte(`null`))
}
func BenchmarkFastjq_Small_INDEX(b *testing.B) {
	benchFastjqObj(b, `INDEX(range(5)|[., "foo\(.)"]; .[0])`, []byte(`null`))
}
func BenchmarkFastjq_Small_JOIN(b *testing.B) {
	benchFastjqObj(b, `JOIN({"0":[0,"abc"],"1":[1,"bcd"],"2":[2,"def"]}; .[0]|tostring)`, []byte(`[[2,"x"],[1,"y"],[5,"z"]]`))
}
func BenchmarkFastjq_Small_HaveDecnum(b *testing.B) {
	benchFastjqObj(b, `have_decnum`, []byte(`null`))
}
func BenchmarkFastjq_Large_Limit(b *testing.B) {
	benchFastjqFunc(b, `limit(10; .[])`, largeIntArr)
}

// --- gojq benchmarks ---

func BenchmarkGojq_Small_Del(b *testing.B)  { benchGojqObj(b, "del(.field_2)", smallJSON) }
func BenchmarkGojq_Medium_Del(b *testing.B) { benchGojqObj(b, "del(.field_5)", mediumJSON) }
func BenchmarkGojq_Large_Del(b *testing.B)  { benchGojqLargeRot(b, "del(.field_50)") }
func BenchmarkGojq_Small_Field(b *testing.B) {
	benchGojqObj(b, ".field_2", smallJSON)
}
func BenchmarkGojq_Large_Field(b *testing.B) { benchGojqLargeRot(b, ".field_50") }
func BenchmarkGojq_Small_Index(b *testing.B) { benchGojqObj(b, ".[2]", []byte(`[1,2,3,4,5]`)) }
func BenchmarkGojq_Small_ArrayDel(b *testing.B) {
	benchGojqObj(b, "del(.[1], .[3])", []byte(`[1,2,3,4,5]`))
}
func BenchmarkGojq_Small_Construct(b *testing.B) {
	benchGojqObj(b, "{field_0, field_2}", smallJSON)
}
func BenchmarkGojq_Large_Construct(b *testing.B) { benchGojqLargeRot(b, "{field_0, field_50}") }
func BenchmarkGojq_Small_Iterator(b *testing.B) {
	benchGojqIter(b, ".[]", []byte(`[1,2,3,4,5]`))
}
func BenchmarkGojq_Large_Iterator(b *testing.B) {
	var items strings.Builder
	items.WriteString("[")
	for i := 0; i < 200; i++ {
		if i > 0 {
			items.WriteString(",")
		}
		fmt.Fprintf(&items, `{"id":%d,"value":"%s"}`, i, strings.Repeat("x", 50))
	}
	items.WriteString("]")
	benchGojqIter(b, ".[]", []byte(items.String()))
}
func BenchmarkGojq_Small_Select(b *testing.B) {
	benchGojqObj(b, `select(.field_2 == "xxxxxxxxxx")`, smallJSON)
}
func BenchmarkGojq_Large_Select(b *testing.B) {
	benchGojqLargeRot(b, `select(.field_199 == "`+strings.Repeat("x", 200)+`")`)
}
func BenchmarkGojq_Small_Alternative(b *testing.B) {
	benchGojqObj(b, `.field_2 // "default"`, smallJSON)
}
func BenchmarkGojq_Small_SelectAnd(b *testing.B) {
	benchGojqObj(b, `select(.field_0 == "xxxxxxxxxx" and .field_1 == "xxxxxxxxxx")`, smallJSON)
}
func BenchmarkGojq_Small_SelectOr(b *testing.B) {
	benchGojqObj(b, `select(.field_0 == "xxxxxxxxxx" or .field_4 == "xxxxxxxxxx")`, smallJSON)
}
func BenchmarkGojq_Small_Has(b *testing.B) {
	benchGojqObj(b, `select(has("field_2"))`, smallJSON)
}
func BenchmarkGojq_Large_Has(b *testing.B) { benchGojqLargeRot(b, `select(has("field_199"))`) }
func BenchmarkGojq_Small_IfThenElse(b *testing.B) {
	benchGojqObj(b, `if .field_0 == "xxxxxxxxxx" then .field_0 else "default" end`, smallJSON)
}
func BenchmarkGojq_Small_Length(b *testing.B) {
	benchGojqObj(b, `.field_0 | length`, smallJSON)
}
func BenchmarkGojq_Large_Length(b *testing.B)    { benchGojqLargeRot(b, `length`) }
func BenchmarkGojq_Small_Map(b *testing.B)       { benchGojqObj(b, `map(.name)`, smallArray) }
func BenchmarkGojq_Large_Map(b *testing.B)       { benchGojqObj(b, `map(.name)`, largeArray) }
func BenchmarkGojq_Small_ToEntries(b *testing.B) { benchGojqObj(b, `to_entries`, smallJSON) }
func BenchmarkGojq_Large_ToEntries(b *testing.B) { benchGojqLargeRot(b, `to_entries`) }
func BenchmarkGojq_Small_KeysUnsorted(b *testing.B) {
	benchGojqObj(b, `keys`, smallJSON) // gojq: keys_unsorted not supported
}
func BenchmarkGojq_Small_Keys(b *testing.B) {
	benchGojqObj(b, `keys`, smallJSON)
}
func BenchmarkGojq_Small_Paths(b *testing.B) {
	benchGojqIter(b, `paths`, smallJSON)
}
func BenchmarkGojq_Small_RecursiveDescent(b *testing.B) {
	benchGojqIter(b, `..`, smallJSON)
}
func BenchmarkGojq_Small_Path(b *testing.B) {
	benchGojqObj(b, `path(.field_0)`, smallJSON)
}
func BenchmarkGojq_Small_GetPath(b *testing.B) {
	benchGojqObj(b, `getpath(["field_0"])`, smallJSON)
}
func BenchmarkGojq_Small_SetPath(b *testing.B) {
	benchGojqObj(b, `setpath(["field_0"]; "y")`, smallJSON)
}
func BenchmarkGojq_Small_DelPaths(b *testing.B) {
	benchGojqObj(b, `delpaths([["field_0"]])`, smallJSON)
}
func BenchmarkGojq_Large_KeysUnsorted(b *testing.B) {
	benchGojqLargeRot(b, `keys`) // gojq: keys_unsorted not supported
}
func BenchmarkGojq_Small_Any(b *testing.B) {
	benchGojqObj(b, `any`, []byte(`[false,false,true,false,false]`))
}
func BenchmarkGojq_Small_AnyExpr(b *testing.B) {
	benchGojqObj(b, `any(. == "xxxxxxxxxx")`, []byte(`["aaaa","bbbb","xxxxxxxxxx","cccc","dddd"]`))
}
func BenchmarkGojq_Large_AnyExpr(b *testing.B) { benchGojqObj(b, `any(. > 100)`, largeIntArr) }
func BenchmarkGojq_Small_Add(b *testing.B) {
	benchGojqObj(b, `add`, []byte(`[1,2,3,4,5]`))
}
func BenchmarkGojq_Small_AddStrings(b *testing.B) {
	benchGojqObj(b, `add`, []byte(`["field_0","field_1","field_2","field_3","field_4"]`))
}
func BenchmarkGojq_Small_Flatten(b *testing.B) {
	benchGojqObj(b, `flatten`, []byte(`[[1,2],[3,[4,5]],[6]]`))
}
func BenchmarkGojq_Small_Split(b *testing.B) {
	benchGojqObj(b, `split(",")`, []byte(`"field_0,field_1,field_2,field_3,field_4"`))
}
func BenchmarkGojq_Small_Join(b *testing.B) {
	benchGojqObj(b, `join(",")`, []byte(`["field_0","field_1","field_2","field_3","field_4"]`))
}

func BenchmarkGojq_Small_Slice(b *testing.B) {
	benchGojqObj(b, `.[1:4]`, []byte(`[0,1,2,3,4,5]`))
}
func BenchmarkGojq_Small_SliceString(b *testing.B) {
	benchGojqObj(b, `.[:5]`, []byte(`"hello world from fastjq"`))
}
func BenchmarkGojq_Small_Plus(b *testing.B) {
	benchGojqObj(b, `.a + .b`, []byte(`{"a":"foo","b":"bar"}`))
}
func BenchmarkGojq_Small_PlusStr(b *testing.B) {
	benchGojqObj(b, `.prefix + .name`,
		[]byte(`{"prefix":"user_","name":"alice"}`))
}

func BenchmarkGojq_Small_AsciiDowncase(b *testing.B) {
	benchGojqObj(b, `select(.field_2 | ascii_downcase == "xxxxxxxxxx")`, smallJSON)
}
func BenchmarkGojq_Large_AsciiDowncase(b *testing.B) {
	benchGojqLargeRot(b, `select(.field_199 | ascii_downcase == "`+strings.Repeat("x", 200)+`")`)
}
func BenchmarkGojq_Small_Startswith(b *testing.B) {
	benchGojqObj(b, `select(.field_2 | startswith("xxx"))`, smallJSON)
}
func BenchmarkGojq_Large_Startswith(b *testing.B) {
	benchGojqLargeRot(b, `select(.field_199 | startswith("xxx"))`)
}
func BenchmarkGojq_Small_Endswith(b *testing.B) {
	benchGojqObj(b, `select(.field_2 | endswith("xxx"))`, smallJSON)
}
func BenchmarkGojq_Small_Ltrimstr(b *testing.B) {
	benchGojqObj(b, `.field_2 | ltrimstr("xxx")`, smallJSON)
}
func BenchmarkGojq_Small_Rtrimstr(b *testing.B) {
	benchGojqObj(b, `.field_2 | rtrimstr("xxx")`, smallJSON)
}
func BenchmarkGojq_Small_Trimstr(b *testing.B) {
	benchGojqObj(b, `trimstr("xxx")`, []byte(`"xxhelloxxx"`))
}
func BenchmarkGojq_Small_Trim(b *testing.B) {
	benchGojqObj(b, `trim`, []byte(`"  abc  "`))
}
func BenchmarkGojq_Small_Ltrim(b *testing.B) {
	benchGojqObj(b, `ltrim`, []byte(`"  abc  "`))
}
func BenchmarkGojq_Small_Rtrim(b *testing.B) {
	benchGojqObj(b, `rtrim`, []byte(`"  abc  "`))
}
func BenchmarkGojq_Small_UTF8ByteLength(b *testing.B) {
	benchGojqObj(b, `utf8bytelength`, []byte(`"asdf\u03bc"`))
}
func BenchmarkGojq_Small_Reverse(b *testing.B) {
	benchGojqObj(b, `reverse`, []byte(`[1,2,3,4,5]`))
}
func BenchmarkGojq_Small_First(b *testing.B) {
	benchGojqObj(b, `first(.[] | select(. > 2))`, []byte(`[1,2,3,4,5]`))
}
func BenchmarkGojq_Large_First(b *testing.B) {
	benchGojqObj(b, `first(.[] | select(. > 100))`, largeIntArr)
}
func BenchmarkGojq_Small_Last(b *testing.B) {
	benchGojqObj(b, `last(.[] | select(. > 2))`, []byte(`[1,2,3,4,5]`))
}
func BenchmarkGojq_Large_Last(b *testing.B) {
	benchGojqObj(b, `last(.[] | select(. > 100))`, largeIntArr)
}
func BenchmarkGojq_Small_Limit(b *testing.B) {
	benchGojqIter(b, `limit(3; .[])`, []byte(`[10,20,30,40,50]`))
}
func BenchmarkGojq_Small_Skip(b *testing.B) {
	benchGojqIter(b, `skip(2; .[])`, []byte(`[10,20,30,40,50]`))
}
func BenchmarkGojq_Small_Reduce(b *testing.B) {
	benchGojqObj(b, `reduce .[] as $x (0; . + $x)`, []byte(`[1,2,3,4,5]`))
}
func BenchmarkGojq_Small_Foreach(b *testing.B) {
	benchGojqIter(b, `foreach .[] as $x (0; . + $x)`, []byte(`[1,2,3,4,5]`))
}
func BenchmarkGojq_Small_While(b *testing.B) {
	benchGojqIter(b, `while(.<100; .*2)`, []byte(`1`))
}
func BenchmarkGojq_Small_Until(b *testing.B) {
	benchGojqObj(b, `[.,1]|until(.[0] < 1; [.[0] - 1, .[1] * .[0]])|.[1]`, []byte(`5`))
}
func BenchmarkGojq_Small_Bsearch(b *testing.B) {
	benchGojqObj(b, `bsearch(42)`, []byte(`[1,10,20,30,40,42,50]`))
}
func BenchmarkGojq_Small_Pick(b *testing.B) {
	benchGojqObj(b, `pick(.field_0, .field_2)`, smallJSON)
}
func BenchmarkGojq_Small_IN(b *testing.B) {
	benchGojqObj(b, `5 | IN(range(10))`, []byte(`null`))
}
func BenchmarkGojq_Small_INDEX(b *testing.B) {
	benchGojqObj(b, `INDEX(range(5)|[., "foo\(.)"]; .[0])`, []byte(`null`))
}
func BenchmarkGojq_Small_JOIN(b *testing.B) {
	benchGojqObj(b, `JOIN({"0":[0,"abc"],"1":[1,"bcd"],"2":[2,"def"]}; .[0]|tostring)`, []byte(`[[2,"x"],[1,"y"],[5,"z"]]`))
}
func BenchmarkGojq_Small_HaveDecnum(b *testing.B) {
	benchGojqObj(b, `have_decnum`, []byte(`null`))
}
func BenchmarkGojq_Large_Limit(b *testing.B) {
	benchGojqIter(b, `limit(10; .[])`, largeIntArr)
}

func BenchmarkFastjq_Small_Subtract(b *testing.B) {
	benchFastjqObj(b, `.a - .b`, []byte(`{"a":100,"b":37}`))
}
func BenchmarkGojq_Small_Subtract(b *testing.B) {
	benchGojqObj(b, `.a - .b`, []byte(`{"a":100,"b":37}`))
}

func BenchmarkFastjq_Small_Multiply(b *testing.B) {
	benchFastjqObj(b, `.price * .qty`, []byte(`{"price":2.5,"qty":4}`))
}
func BenchmarkGojq_Small_Multiply(b *testing.B) {
	benchGojqObj(b, `.price * .qty`, []byte(`{"price":2.5,"qty":4}`))
}

func BenchmarkFastjq_Small_Divide(b *testing.B) {
	benchFastjqObj(b, `.total / .count`, []byte(`{"total":100,"count":3}`))
}
func BenchmarkGojq_Small_Divide(b *testing.B) {
	benchGojqObj(b, `.total / .count`, []byte(`{"total":100,"count":3}`))
}

func BenchmarkFastjq_Small_Min(b *testing.B) {
	benchFastjqObj(b, `min`, largeIntArr)
}
func BenchmarkGojq_Small_Min(b *testing.B) {
	benchGojqObj(b, `min`, largeIntArr)
}

func BenchmarkFastjq_Small_MinBy(b *testing.B) {
	benchFastjqObj(b, `min_by(.value)`, mediumArray)
}
func BenchmarkGojq_Small_MinBy(b *testing.B) {
	benchGojqObj(b, `min_by(.value)`, mediumArray)
}

// sort / sort_by / unique / group_by — Tier 2: O(n) allocation proportional to array size.
// 100-element integer array for sort; 100-element object array for sort_by/group_by.
func BenchmarkFastjq_Sort(b *testing.B)    { benchFastjqObj(b, `sort`, largeIntArr) }
func BenchmarkGojq_Sort(b *testing.B)      { benchGojqObj(b, `sort`, largeIntArr) }
func BenchmarkFastjq_SortBy(b *testing.B)  { benchFastjqObj(b, `sort_by(.value)`, mediumArray) }
func BenchmarkGojq_SortBy(b *testing.B)    { benchGojqObj(b, `sort_by(.value)`, mediumArray) }
func BenchmarkFastjq_Unique(b *testing.B)  { benchFastjqObj(b, `unique`, largeIntArr) }
func BenchmarkGojq_Unique(b *testing.B)    { benchGojqObj(b, `unique`, largeIntArr) }
func BenchmarkFastjq_GroupBy(b *testing.B) { benchFastjqObj(b, `group_by(.active)`, mediumArray) }
func BenchmarkGojq_GroupBy(b *testing.B)   { benchGojqObj(b, `group_by(.active)`, mediumArray) }

func BenchmarkFastjq_Small_URIEncode(b *testing.B) {
	benchFastjqObj(b, `@uri`, []byte(`"/api/v1/users?filter=active&page=1"`))
}
func BenchmarkGojq_Small_URIEncode(b *testing.B) {
	benchGojqObj(b, `@uri`, []byte(`"/api/v1/users?filter=active&page=1"`))
}
func BenchmarkFastjq_Small_HTMLTemplate(b *testing.B) {
	benchFastjqObj(b, `@html "<b>\(.field_0)</b>"`, smallJSON)
}
func BenchmarkGojq_Small_HTMLTemplate(b *testing.B) {
	benchGojqObj(b, `@html "<b>\(.field_0)</b>"`, smallJSON)
}

func BenchmarkFastjq_Small_ArrayDiff(b *testing.B) {
	benchFastjqObj(b, `.a - .b`, []byte(`{"a":[1,2,3,4,5],"b":[2,4]}`))
}
func BenchmarkGojq_Small_ArrayDiff(b *testing.B) {
	benchGojqObj(b, `.a - .b`, []byte(`{"a":[1,2,3,4,5],"b":[2,4]}`))
}

func BenchmarkFastjq_Small_TryNoError(b *testing.B) {
	benchFastjqObj(b, `try .field_2`, smallJSON)
}
func BenchmarkFastjq_Small_TryCatchNoError(b *testing.B) {
	benchFastjqObj(b, `try .field_2 catch "err"`, smallJSON)
}
func BenchmarkGojq_Small_TryNoError(b *testing.B) {
	benchGojqObj(b, `try .field_2`, smallJSON)
}

func BenchmarkFastjq_Small_ObjectMerge(b *testing.B) {
	benchFastjqObj(b, `.a + .b`, []byte(`{"a":{"x":1,"y":2},"b":{"y":3,"z":4}}`))
}
func BenchmarkGojq_Small_ObjectMerge(b *testing.B) {
	benchGojqObj(b, `.a + .b`, []byte(`{"a":{"x":1,"y":2},"b":{"y":3,"z":4}}`))
}

func BenchmarkFastjq_Small_ToJSON(b *testing.B) {
	benchFastjqObj(b, `tojson`, smallJSON)
}
func BenchmarkGojq_Small_ToJSON(b *testing.B) {
	benchGojqObj(b, `tojson`, smallJSON)
}

func BenchmarkFastjq_Small_FromJSON(b *testing.B) {
	// pre-encoded small JSON as a string input
	input := []byte(`"{\"field_0\":\"xxxxxxxxxx\",\"field_1\":\"xxxxxxxxxx\"}"`)
	benchFastjqObj(b, `fromjson`, input)
}
func BenchmarkGojq_Small_FromJSON(b *testing.B) {
	input := []byte(`"{\"field_0\":\"xxxxxxxxxx\",\"field_1\":\"xxxxxxxxxx\"}"`)
	benchGojqObj(b, `fromjson`, input)
}

func BenchmarkFastjq_Small_ToString(b *testing.B) {
	benchFastjqObj(b, `tostring`, smallJSON)
}
func BenchmarkFastjq_Small_ToNumber(b *testing.B) {
	benchFastjqObj(b, `tonumber`, []byte(`"42"`))
}
func BenchmarkFastjq_Small_ToBoolean(b *testing.B) {
	benchFastjqObj(b, `toboolean`, []byte(`"true"`))
}
func BenchmarkGojq_Small_ToNumber(b *testing.B) {
	benchGojqObj(b, `tonumber`, []byte(`"42"`))
}
func BenchmarkGojq_Small_ToBoolean(b *testing.B) {
	benchGojqObj(b, `toboolean`, []byte(`"true"`))
}

func BenchmarkFastjq_Small_AnyTwoArg(b *testing.B) {
	benchFastjqObj(b, `any(.[]; . > 100)`, largeIntArr)
}
func BenchmarkGojq_Small_AnyTwoArg(b *testing.B) {
	benchGojqObj(b, `any(.[]; . > 100)`, largeIntArr)
}

// --- Complex multi-feature benchmarks ---
// These benchmark realistic query patterns that combine multiple operations.
// They stress the interaction between features and represent production workloads.
//
// Note on allocations: benchmarks that build arrays with non-trivial element
// expressions (object construction, string concatenation) will show >0 allocs/op.
// execArrayConstruct passes nil scratch to each element; any expression that must
// write output (not just return a sub-slice) allocates. This is expected and
// proportional to the number of elements — fastjq still uses 5–8x fewer allocs
// than gojq and is 1.5–2.6x faster on these complex workloads.

// complexLogEvent is a realistic structured log record (~200B).
var complexLogEvent = []byte(`{"timestamp":"2026-01-01T12:00:00Z","LEVEL":"ERROR","service":"api-gateway","path":"/api/v1/users/123","method":"POST","status":500,"duration_ms":"342","retry":3,"user_id":9001,"trace_id":"abc123"}`)

// complexTransactions is an array of 20 transaction records (~1.5KB).
var complexTransactions = func() []byte {
	var b strings.Builder
	b.WriteString("[")
	products := []string{"widget", "gadget", "tool", "device", "part"}
	for i := 0; i < 20; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		active := "true"
		if i%3 == 0 {
			active = "false"
		}
		fmt.Fprintf(&b, `{"id":%d,"product":"%s","price":%d,"qty":%d,"active":%s,"tag":"%s"}`,
			i+1, products[i%5], (i%10+1)*5, i%8+1, active,
			[]string{"sale", "clearance", "regular"}[i%3])
	}
	b.WriteString("]")
	return []byte(b.String())
}()

// Benchmark: multi-stage log normalization
// select + string ops + arithmetic + construction
func BenchmarkFastjq_Complex_LogNormalize(b *testing.B) {
	benchFastjqObj(b,
		`select(.status >= 500) | {level: (.LEVEL | ascii_downcase), svc: .service, path: (.path | ltrimstr("/api/v1")), dur: (.duration_ms | tonumber), retry: .retry}`,
		complexLogEvent)
}
func BenchmarkGojq_Complex_LogNormalize(b *testing.B) {
	benchGojqObj(b,
		`select(.status >= 500) | {level: (.LEVEL | ascii_downcase), svc: .service, path: (.path | ltrimstr("/api/v1")), dur: (.duration_ms | tonumber), retry: .retry}`,
		complexLogEvent)
}

// Benchmark: array filter + project + arithmetic
// iterate → select → construct with arithmetic → array wrap
func BenchmarkFastjq_Complex_ArrayPipeline(b *testing.B) {
	benchFastjqObj(b,
		`[.[] | select(.active and .price * .qty > 20) | {product, revenue: .price * .qty}]`,
		complexTransactions)
}
func BenchmarkGojq_Complex_ArrayPipeline(b *testing.B) {
	benchGojqObj(b,
		`[.[] | select(.active and .price * .qty > 20) | {product, revenue: .price * .qty}]`,
		complexTransactions)
}

// Benchmark: aggregation — multiple stats in one pass
// length + map + add + min_by + any
func BenchmarkFastjq_Complex_Aggregation(b *testing.B) {
	benchFastjqObj(b,
		`{count: length, revenue: ([.[] | .price * .qty] | add), cheapest: (min_by(.price) | .product), any_inactive: any(.[]; .active | not)}`,
		complexTransactions)
}
func BenchmarkGojq_Complex_Aggregation(b *testing.B) {
	benchGojqObj(b,
		`{count: length, revenue: ([.[] | .price * .qty] | add), cheapest: (min_by(.price) | .product), any_inactive: any(.[]; .active | not)}`,
		complexTransactions)
}

// Benchmark: error-tolerant map with try/catch
// map with try/catch on each element
func BenchmarkFastjq_Complex_TolerantMap(b *testing.B) {
	benchFastjqObj(b,
		`[.[] | try {id, rev: (.price / .qty), tag: (.tag | ascii_upcase)} catch {id, rev: null, tag: "error"}]`,
		complexTransactions)
}
func BenchmarkGojq_Complex_TolerantMap(b *testing.B) {
	benchGojqObj(b,
		`[.[] | try {id, rev: (.price / .qty), tag: (.tag | ascii_upcase)} catch {id, rev: null, tag: "error"}]`,
		complexTransactions)
}

// Benchmark: elif routing — classify each record
func BenchmarkFastjq_Complex_ElifRouting(b *testing.B) {
	benchFastjqObj(b,
		`[.[] | {product, tier: if .price * .qty > 50 then "high" elif .price * .qty > 20 then "mid" else "low" end}]`,
		complexTransactions)
}
func BenchmarkGojq_Complex_ElifRouting(b *testing.B) {
	benchGojqObj(b,
		`[.[] | {product, tier: if .price * .qty > 50 then "high" elif .price * .qty > 20 then "mid" else "low" end}]`,
		complexTransactions)
}

// Benchmark: string building — construct a formatted string per record
func BenchmarkFastjq_Complex_StringBuild(b *testing.B) {
	benchFastjqObj(b,
		`[.[] | select(.active) | .product + ":" + (.price | tostring) + "x" + (.qty | tostring)]`,
		complexTransactions)
}
func BenchmarkGojq_Complex_StringBuild(b *testing.B) {
	benchGojqObj(b,
		`[.[] | select(.active) | .product + ":" + (.price | tostring) + "x" + (.qty | tostring)]`,
		complexTransactions)
}

// Benchmark: to_entries → filter → from_entries (object key filtering)
func BenchmarkFastjq_Complex_EntryFilter(b *testing.B) {
	benchFastjqObj(b,
		`to_entries | map(select(.value | type == "string" or type == "number")) | from_entries`,
		complexLogEvent)
}
func BenchmarkGojq_Complex_EntryFilter(b *testing.B) {
	benchGojqObj(b,
		`to_entries | map(select(.value | type == "string" or type == "number")) | from_entries`,
		complexLogEvent)
}

// --- Math builtins (1-arg float ops) ---
// All zero-alloc: parseJSONFloat → math.* → appendJSONFloat into buf.

var mathInput = []byte(`2.718281828459045`) // e

func BenchmarkFastjq_Small_Sqrt(b *testing.B) { benchFastjqObj(b, `sqrt`, mathInput) }
func BenchmarkGojq_Small_Sqrt(b *testing.B)   { benchGojqObj(b, `sqrt`, mathInput) }

func BenchmarkFastjq_Small_Log(b *testing.B) { benchFastjqObj(b, `log`, mathInput) }
func BenchmarkGojq_Small_Log(b *testing.B)   { benchGojqObj(b, `log`, mathInput) }

func BenchmarkFastjq_Small_Sin(b *testing.B) { benchFastjqObj(b, `sin`, mathInput) }
func BenchmarkGojq_Small_Sin(b *testing.B)   { benchGojqObj(b, `sin`, mathInput) }

func BenchmarkFastjq_Small_Atan(b *testing.B) { benchFastjqObj(b, `atan`, []byte(`1`)) }
func BenchmarkGojq_Small_Atan(b *testing.B)   { benchGojqObj(b, `atan`, []byte(`1`)) }

func BenchmarkFastjq_Small_Exp(b *testing.B) { benchFastjqObj(b, `exp`, []byte(`1`)) }
func BenchmarkGojq_Small_Exp(b *testing.B)   { benchGojqObj(b, `exp`, []byte(`1`)) }

func BenchmarkFastjq_Small_Tgamma(b *testing.B) { benchFastjqObj(b, `tgamma`, []byte(`5`)) }
func BenchmarkGojq_Small_Tgamma(b *testing.B)   { benchGojqObj(b, `tgamma`, []byte(`5`)) }

func BenchmarkFastjq_Small_Fabs(b *testing.B) { benchFastjqObj(b, `fabs`, []byte(`-3.14`)) }
func BenchmarkGojq_Small_Fabs(b *testing.B)   { benchGojqObj(b, `fabs`, []byte(`-3.14`)) }
func BenchmarkFastjq_Small_Abs(b *testing.B)  { benchFastjqObj(b, `abs`, []byte(`-3.14`)) }
func BenchmarkGojq_Small_Abs(b *testing.B)    { benchGojqObj(b, `abs`, []byte(`-3.14`)) }

func BenchmarkFastjq_Small_Bind(b *testing.B) {
	benchFastjqObj(b, `.bar as $x | .foo | . + $x`, []byte(`{"foo":10,"bar":200}`))
}
func BenchmarkGojq_Small_Bind(b *testing.B) {
	benchGojqObj(b, `.bar as $x | .foo | . + $x`, []byte(`{"foo":10,"bar":200}`))
}
func BenchmarkFastjq_Small_Def(b *testing.B) {
	benchFastjqObj(b, `def inc: . + 1; inc`, []byte(`1`))
}
func BenchmarkGojq_Small_Def(b *testing.B) {
	benchGojqObj(b, `def inc: . + 1; inc`, []byte(`1`))
}

// --- String interpolation ---
// Zero-alloc for field-access expressions (sub-slices of input).
// The literal segs are compiled into the AST at parse time.

var stringInterpInput = []byte(`{"name":"alice","level":"error","svc":"api"}`)

func BenchmarkFastjq_Small_StringInterp(b *testing.B) {
	benchFastjqObj(b, `"\(.level): \(.svc)"`, stringInterpInput)
}
func BenchmarkGojq_Small_StringInterp(b *testing.B) {
	benchGojqObj(b, `"\(.level): \(.svc)"`, stringInterpInput)
}

func BenchmarkFastjq_Small_StringInterpNum(b *testing.B) {
	benchFastjqObj(b, `"user \(.name) at level \(.level)"`, stringInterpInput)
}
func BenchmarkGojq_Small_StringInterpNum(b *testing.B) {
	benchGojqObj(b, `"user \(.name) at level \(.level)"`, stringInterpInput)
}

// --- isempty ---
// Zero-alloc: early-exit via errBreak, no heap closure.

func BenchmarkFastjq_Small_IsEmptyTrue(b *testing.B) {
	benchFastjqObj(b, `isempty(empty)`, []byte(`null`))
}
func BenchmarkGojq_Small_IsEmptyTrue(b *testing.B) {
	benchGojqObj(b, `isempty(empty)`, []byte(`null`))
}

func BenchmarkFastjq_Small_IsEmptyFalse(b *testing.B) {
	benchFastjqObj(b, `isempty(.[])`, []byte(`[1,2,3,4,5]`))
}
func BenchmarkGojq_Small_IsEmptyFalse(b *testing.B) {
	benchGojqObj(b, `isempty(.[])`, []byte(`[1,2,3,4,5]`))
}

// --- nth ---
// Zero-alloc: counter + errBreak, no heap closure.

func BenchmarkFastjq_Small_Nth(b *testing.B) {
	benchFastjqObj(b, `nth(2; .[])`, []byte(`[10,20,30,40,50]`))
}
func BenchmarkGojq_Small_Nth(b *testing.B) {
	benchGojqObj(b, `nth(2; .[])`, []byte(`[10,20,30,40,50]`))
}

// --- range (Tier 2: 1 alloc per generated value) ---

func BenchmarkFastjq_Small_Range10(b *testing.B) {
	// 10 values → 10 allocs/op (Tier 2: proportional to output count)
	benchFastjqFunc(b, `range(10)`, []byte(`null`))
}
func BenchmarkGojq_Small_Range10(b *testing.B) {
	benchGojqIter(b, `range(10)`, []byte(`null`))
}

func BenchmarkFastjq_Small_RangeLimit(b *testing.B) {
	// limit(3; range(1000)): only 3 values generated → 3 allocs, not 1000
	benchFastjqFunc(b, `limit(3; range(1000))`, []byte(`null`))
}
func BenchmarkGojq_Small_RangeLimit(b *testing.B) {
	benchGojqIter(b, `limit(3; range(1000))`, []byte(`null`))
}
