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

func BenchmarkFastjq_Small_Del(b *testing.B)    { benchFastjqObj(b, "del(.field_2)", smallJSON) }
func BenchmarkFastjq_Medium_Del(b *testing.B)   { benchFastjqObj(b, "del(.field_5)", mediumJSON) }
func BenchmarkFastjq_Large_Del(b *testing.B)    { benchFastjqObj(b, "del(.field_50)", largeJSON) }
func BenchmarkFastjq_Small_Field(b *testing.B)  { benchFastjqObj(b, ".field_2", smallJSON) }
func BenchmarkFastjq_Large_Field(b *testing.B)  { benchFastjqObj(b, ".field_50", largeJSON) }
func BenchmarkFastjq_Small_Index(b *testing.B)  { benchFastjqObj(b, ".[2]", []byte(`[1,2,3,4,5]`)) }
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
func BenchmarkFastjq_Small_WithEntries(b *testing.B) {
	benchFastjqObj(b, `with_entries(select(.value != null))`, smallJSON)
}
func BenchmarkFastjq_Large_WithEntries(b *testing.B) {
	benchFastjqObj(b, `with_entries(select(.value != null))`, largeJSON)
}
func BenchmarkFastjq_Small_KeysUnsorted(b *testing.B) {
	benchFastjqObj(b, `keys_unsorted`, smallJSON)
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
func BenchmarkGojq_Large_Field(b *testing.B)  { benchGojqLargeRot(b, ".field_50") }
func BenchmarkGojq_Small_Index(b *testing.B)  { benchGojqObj(b, ".[2]", []byte(`[1,2,3,4,5]`)) }
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
func BenchmarkGojq_Large_Has(b *testing.B)  { benchGojqLargeRot(b, `select(has("field_199"))`) }
func BenchmarkGojq_Small_IfThenElse(b *testing.B) {
	benchGojqObj(b, `if .field_0 == "xxxxxxxxxx" then .field_0 else "default" end`, smallJSON)
}
func BenchmarkGojq_Small_Length(b *testing.B) {
	benchGojqObj(b, `.field_0 | length`, smallJSON)
}
func BenchmarkGojq_Large_Length(b *testing.B)       { benchGojqLargeRot(b, `length`) }
func BenchmarkGojq_Small_Map(b *testing.B)          { benchGojqObj(b, `map(.name)`, smallArray) }
func BenchmarkGojq_Large_Map(b *testing.B)          { benchGojqObj(b, `map(.name)`, largeArray) }
func BenchmarkGojq_Small_ToEntries(b *testing.B)    { benchGojqObj(b, `to_entries`, smallJSON) }
func BenchmarkGojq_Large_ToEntries(b *testing.B)    { benchGojqLargeRot(b, `to_entries`) }
func BenchmarkGojq_Small_WithEntries(b *testing.B) {
	benchGojqObj(b, `with_entries(select(.value != null))`, smallJSON)
}
func BenchmarkGojq_Small_KeysUnsorted(b *testing.B) {
	benchGojqObj(b, `keys`, smallJSON) // gojq: keys_unsorted not supported
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
func BenchmarkGojq_Large_Limit(b *testing.B) {
	benchGojqIter(b, `limit(10; .[])`, largeIntArr)
}
