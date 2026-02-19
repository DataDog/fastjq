package fastjq

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/itchyny/gojq"
)

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

var (
	smallJSON  = generateJSON(5, 10)              // ~100B
	mediumJSON = generateNestedJSON(20, 5, 30)    // ~2KB
	largeJSON  = generateNestedJSON(200, 10, 200) // ~100KB+
	smallArray = generateObjectArray(20)          // ~600B, 20 objects
)

// benchSink prevents the compiler from eliminating json.Marshal calls via dead-code
// elimination, which would make large-input gojq benchmarks appear identical to small ones.
var benchSink []byte

// --- fastjq benchmarks ---

func BenchmarkFastjq_Small_Del(b *testing.B) {
	p, _ := Compile("del(.field_2)")
	buf := make([]byte, 0, len(smallJSON))
	b.ReportAllocs()
	for b.Loop() {
		buf, _ = p.RunWithBuffer(smallJSON, buf)
	}
}

func BenchmarkFastjq_Medium_Del(b *testing.B) {
	p, _ := Compile("del(.field_5)")
	buf := make([]byte, 0, len(mediumJSON))
	b.ReportAllocs()
	for b.Loop() {
		buf, _ = p.RunWithBuffer(mediumJSON, buf)
	}
}

func BenchmarkFastjq_Large_Del(b *testing.B) {
	p, _ := Compile("del(.field_50)")
	buf := make([]byte, 0, len(largeJSON))
	b.ReportAllocs()
	for b.Loop() {
		buf, _ = p.RunWithBuffer(largeJSON, buf)
	}
}

func BenchmarkFastjq_Small_Field(b *testing.B) {
	p, _ := Compile(".field_2")
	buf := make([]byte, 0, 64)
	b.ReportAllocs()
	for b.Loop() {
		buf, _ = p.RunWithBuffer(smallJSON, buf)
	}
}

func BenchmarkFastjq_Large_Field(b *testing.B) {
	p, _ := Compile(".field_50")
	buf := make([]byte, 0, 256)
	b.ReportAllocs()
	for b.Loop() {
		buf, _ = p.RunWithBuffer(largeJSON, buf)
	}
}

func BenchmarkFastjq_Small_Index(b *testing.B) {
	input := []byte(`[1,2,3,4,5]`)
	p, _ := Compile(".[2]")
	buf := make([]byte, 0, 64)
	b.ReportAllocs()
	for b.Loop() {
		buf, _ = p.RunWithBuffer(input, buf)
	}
}

func BenchmarkFastjq_Small_ArrayDel(b *testing.B) {
	input := []byte(`[1,2,3,4,5]`)
	p, _ := Compile("del(.[1], .[3])")
	buf := make([]byte, 0, 64)
	b.ReportAllocs()
	for b.Loop() {
		buf, _ = p.RunWithBuffer(input, buf)
	}
}

func BenchmarkFastjq_Small_Construct(b *testing.B) {
	p, _ := Compile("{field_0, field_2}")
	buf := make([]byte, 0, 128)
	b.ReportAllocs()
	for b.Loop() {
		buf, _ = p.RunWithBuffer(smallJSON, buf)
	}
}

func BenchmarkFastjq_Large_Construct(b *testing.B) {
	p, _ := Compile("{field_0, field_50}")
	buf := make([]byte, 0, 1024)
	b.ReportAllocs()
	for b.Loop() {
		buf, _ = p.RunWithBuffer(largeJSON, buf)
	}
}

func BenchmarkFastjq_Small_Iterator(b *testing.B) {
	input := []byte(`[1,2,3,4,5]`)
	p, _ := Compile(".[]")
	b.ReportAllocs()
	for b.Loop() {
		p.RunFunc(input, func(result []byte) error {
			return nil
		})
	}
}

func BenchmarkFastjq_Large_Iterator(b *testing.B) {
	// Build a large array
	var items strings.Builder
	items.WriteString("[")
	for i := 0; i < 200; i++ {
		if i > 0 {
			items.WriteString(",")
		}
		fmt.Fprintf(&items, `{"id":%d,"value":"%s"}`, i, strings.Repeat("x", 50))
	}
	items.WriteString("]")
	input := []byte(items.String())

	p, _ := Compile(".[]")
	b.ReportAllocs()
	for b.Loop() {
		p.RunFunc(input, func(result []byte) error {
			return nil
		})
	}
}

func BenchmarkFastjq_Small_Select(b *testing.B) {
	p, _ := Compile(`select(.field_2 == "xxxxxxxxxx")`)
	buf := make([]byte, 0, len(smallJSON))
	b.ReportAllocs()
	for b.Loop() {
		buf, _ = p.RunWithBuffer(smallJSON, buf)
	}
}

func BenchmarkFastjq_Large_Select(b *testing.B) {
	// field_199 is the last field in the object — fastjq must scan the full
	// 170KB to find it, preventing early-exit from skewing the comparison.
	p, _ := Compile(`select(.field_199 == "` + strings.Repeat("x", 200) + `")`)
	const n = 8
	inputs := make([][]byte, n)
	for i := range inputs {
		inputs[i] = generateNestedJSON(200, 10, 200)
	}
	buf := make([]byte, 0, len(largeJSON))
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		buf, _ = p.RunWithBuffer(inputs[i%n], buf)
		i++
	}
}

func BenchmarkFastjq_Small_Alternative(b *testing.B) {
	p, _ := Compile(`.field_2 // "default"`)
	buf := make([]byte, 0, 64)
	b.ReportAllocs()
	for b.Loop() {
		buf, _ = p.RunWithBuffer(smallJSON, buf)
	}
}

func BenchmarkFastjq_Small_SelectAnd(b *testing.B) {
	p, _ := Compile(`select(.field_0 == "xxxxxxxxxx" and .field_1 == "xxxxxxxxxx")`)
	buf := make([]byte, 0, len(smallJSON))
	b.ReportAllocs()
	for b.Loop() {
		buf, _ = p.RunWithBuffer(smallJSON, buf)
	}
}

func BenchmarkFastjq_Small_SelectOr(b *testing.B) {
	p, _ := Compile(`select(.field_0 == "xxxxxxxxxx" or .field_4 == "xxxxxxxxxx")`)
	buf := make([]byte, 0, len(smallJSON))
	b.ReportAllocs()
	for b.Loop() {
		buf, _ = p.RunWithBuffer(smallJSON, buf)
	}
}

func BenchmarkFastjq_Small_Has(b *testing.B) {
	p, _ := Compile(`select(has("field_2"))`)
	buf := make([]byte, 0, len(smallJSON))
	b.ReportAllocs()
	for b.Loop() {
		buf, _ = p.RunWithBuffer(smallJSON, buf)
	}
}

func BenchmarkFastjq_Small_IfThenElse(b *testing.B) {
	p, _ := Compile(`if .field_0 == "xxxxxxxxxx" then .field_0 else "default" end`)
	buf := make([]byte, 0, 64)
	b.ReportAllocs()
	for b.Loop() {
		buf, _ = p.RunWithBuffer(smallJSON, buf)
	}
}

func BenchmarkFastjq_Small_Length(b *testing.B) {
	p, _ := Compile(`.field_0 | length`)
	buf := make([]byte, 0, 8)
	b.ReportAllocs()
	for b.Loop() {
		buf, _ = p.RunWithBuffer(smallJSON, buf)
	}
}

func BenchmarkFastjq_Small_Map(b *testing.B) {
	p, _ := Compile(`map(.name)`)
	buf := make([]byte, 0, 256)
	b.ReportAllocs()
	for b.Loop() {
		buf, _ = p.RunWithBuffer(smallArray, buf)
	}
}

func BenchmarkFastjq_Small_ToEntries(b *testing.B) {
	p, _ := Compile(`to_entries`)
	buf := make([]byte, 0, 256)
	b.ReportAllocs()
	for b.Loop() {
		buf, _ = p.RunWithBuffer(smallJSON, buf)
	}
}

func BenchmarkFastjq_Small_WithEntries(b *testing.B) {
	p, _ := Compile(`with_entries(select(.value != null))`)
	buf := make([]byte, 0, len(smallJSON))
	b.ReportAllocs()
	for b.Loop() {
		buf, _ = p.RunWithBuffer(smallJSON, buf)
	}
}

func BenchmarkFastjq_Small_First(b *testing.B) {
	p, _ := Compile(`first(.[] | select(. > 2))`)
	input := []byte(`[1,2,3,4,5]`)
	buf := make([]byte, 0, 8)
	b.ReportAllocs()
	for b.Loop() {
		buf, _ = p.RunWithBuffer(input, buf)
	}
}

func BenchmarkFastjq_Small_Last(b *testing.B) {
	p, _ := Compile(`last(.[] | select(. > 2))`)
	input := []byte(`[1,2,3,4,5]`)
	buf := make([]byte, 0, 8)
	b.ReportAllocs()
	for b.Loop() {
		buf, _ = p.RunWithBuffer(input, buf)
	}
}

func BenchmarkFastjq_Small_Limit(b *testing.B) {
	p, _ := Compile(`limit(3; .[])`)
	input := []byte(`[10,20,30,40,50]`)
	b.ReportAllocs()
	for b.Loop() {
		p.RunFunc(input, func([]byte) error { return nil })
	}
}

func BenchmarkFastjq_Small_KeysUnsorted(b *testing.B) {
	p, _ := Compile("keys_unsorted")
	buf := make([]byte, 0, 64)
	b.ReportAllocs()
	for b.Loop() {
		buf, _ = p.RunWithBuffer(smallJSON, buf)
	}
}

func BenchmarkFastjq_Small_Any(b *testing.B) {
	// any on array of 5 elements — short-circuits on first truthy
	input := []byte(`[false,false,true,false,false]`)
	p, _ := Compile("any")
	buf := make([]byte, 0, 8)
	b.ReportAllocs()
	for b.Loop() {
		buf, _ = p.RunWithBuffer(input, buf)
	}
}

func BenchmarkFastjq_Small_AnyExpr(b *testing.B) {
	p, _ := Compile(`any(. == "xxxxxxxxxx")`)
	input := []byte(`["aaaa","bbbb","xxxxxxxxxx","cccc","dddd"]`)
	buf := make([]byte, 0, 8)
	b.ReportAllocs()
	for b.Loop() {
		buf, _ = p.RunWithBuffer(input, buf)
	}
}

func BenchmarkFastjq_Small_AsciiDowncase(b *testing.B) {
	// select with case-insensitive match — most common use case
	p, _ := Compile(`select(.field_2 | ascii_downcase == "xxxxxxxxxx")`)
	buf := make([]byte, 0, len(smallJSON))
	b.ReportAllocs()
	for b.Loop() {
		buf, _ = p.RunWithBuffer(smallJSON, buf)
	}
}

func BenchmarkFastjq_Small_Startswith(b *testing.B) {
	p, _ := Compile(`select(.field_2 | startswith("xxx"))`)
	buf := make([]byte, 0, len(smallJSON))
	b.ReportAllocs()
	for b.Loop() {
		buf, _ = p.RunWithBuffer(smallJSON, buf)
	}
}

func BenchmarkFastjq_Small_Endswith(b *testing.B) {
	p, _ := Compile(`select(.field_2 | endswith("xxx"))`)
	buf := make([]byte, 0, len(smallJSON))
	b.ReportAllocs()
	for b.Loop() {
		buf, _ = p.RunWithBuffer(smallJSON, buf)
	}
}

func BenchmarkFastjq_Small_Ltrimstr(b *testing.B) {
	p, _ := Compile(`.field_2 | ltrimstr("xxx")`)
	buf := make([]byte, 0, 32)
	b.ReportAllocs()
	for b.Loop() {
		buf, _ = p.RunWithBuffer(smallJSON, buf)
	}
}

func BenchmarkFastjq_Small_Rtrimstr(b *testing.B) {
	p, _ := Compile(`.field_2 | rtrimstr("xxx")`)
	buf := make([]byte, 0, 32)
	b.ReportAllocs()
	for b.Loop() {
		buf, _ = p.RunWithBuffer(smallJSON, buf)
	}
}

// --- gojq benchmarks ---

func BenchmarkGojq_Small_Del(b *testing.B) {
	query, _ := gojq.Parse("del(.field_2)")
	code, _ := gojq.Compile(query)
	b.ReportAllocs()
	for b.Loop() {
		var v any
		json.Unmarshal(smallJSON, &v)
		iter := code.Run(v)
		result, _ := iter.Next()
		benchSink, _ = json.Marshal(result)
	}
}

func BenchmarkGojq_Medium_Del(b *testing.B) {
	query, _ := gojq.Parse("del(.field_5)")
	code, _ := gojq.Compile(query)
	b.ReportAllocs()
	for b.Loop() {
		var v any
		json.Unmarshal(mediumJSON, &v)
		iter := code.Run(v)
		result, _ := iter.Next()
		benchSink, _ = json.Marshal(result)
	}
}

func BenchmarkGojq_Large_Del(b *testing.B) {
	query, _ := gojq.Parse("del(.field_50)")
	code, _ := gojq.Compile(query)
	// Rotate 8 distinct input copies so the runtime cannot pool or cache
	// json.Unmarshal results across iterations (fixes calibration artifact).
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

func BenchmarkGojq_Small_Field(b *testing.B) {
	query, _ := gojq.Parse(".field_2")
	code, _ := gojq.Compile(query)
	b.ReportAllocs()
	for b.Loop() {
		var v any
		json.Unmarshal(smallJSON, &v)
		iter := code.Run(v)
		result, _ := iter.Next()
		benchSink, _ = json.Marshal(result)
	}
}

func BenchmarkGojq_Large_Field(b *testing.B) {
	query, _ := gojq.Parse(".field_50")
	code, _ := gojq.Compile(query)
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

func BenchmarkGojq_Small_Index(b *testing.B) {
	input := []byte(`[1,2,3,4,5]`)
	query, _ := gojq.Parse(".[2]")
	code, _ := gojq.Compile(query)
	b.ReportAllocs()
	for b.Loop() {
		var v any
		json.Unmarshal(input, &v)
		iter := code.Run(v)
		result, _ := iter.Next()
		benchSink, _ = json.Marshal(result)
	}
}

func BenchmarkGojq_Small_ArrayDel(b *testing.B) {
	input := []byte(`[1,2,3,4,5]`)
	query, _ := gojq.Parse("del(.[1], .[3])")
	code, _ := gojq.Compile(query)
	b.ReportAllocs()
	for b.Loop() {
		var v any
		json.Unmarshal(input, &v)
		iter := code.Run(v)
		result, _ := iter.Next()
		benchSink, _ = json.Marshal(result)
	}
}

func BenchmarkGojq_Small_Construct(b *testing.B) {
	query, _ := gojq.Parse("{field_0, field_2}")
	code, _ := gojq.Compile(query)
	b.ReportAllocs()
	for b.Loop() {
		var v any
		json.Unmarshal(smallJSON, &v)
		iter := code.Run(v)
		result, _ := iter.Next()
		benchSink, _ = json.Marshal(result)
	}
}

func BenchmarkGojq_Large_Construct(b *testing.B) {
	query, _ := gojq.Parse("{field_0, field_50}")
	code, _ := gojq.Compile(query)
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

func BenchmarkGojq_Small_Iterator(b *testing.B) {
	input := []byte(`[1,2,3,4,5]`)
	query, _ := gojq.Parse(".[]")
	code, _ := gojq.Compile(query)
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
	input := []byte(items.String())

	query, _ := gojq.Parse(".[]")
	code, _ := gojq.Compile(query)
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

func BenchmarkGojq_Small_Select(b *testing.B) {
	query, _ := gojq.Parse(`select(.field_2 == "xxxxxxxxxx")`)
	code, _ := gojq.Compile(query)
	b.ReportAllocs()
	for b.Loop() {
		var v any
		json.Unmarshal(smallJSON, &v)
		iter := code.Run(v)
		result, _ := iter.Next()
		benchSink, _ = json.Marshal(result)
	}
}

func BenchmarkGojq_Large_Select(b *testing.B) {
	query, _ := gojq.Parse(`select(.field_199 == "` + strings.Repeat("x", 200) + `")`)
	code, _ := gojq.Compile(query)
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

func BenchmarkGojq_Small_Alternative(b *testing.B) {
	query, _ := gojq.Parse(`.field_2 // "default"`)
	code, _ := gojq.Compile(query)
	b.ReportAllocs()
	for b.Loop() {
		var v any
		json.Unmarshal(smallJSON, &v)
		iter := code.Run(v)
		result, _ := iter.Next()
		benchSink, _ = json.Marshal(result)
	}
}

func BenchmarkGojq_Small_SelectAnd(b *testing.B) {
	query, _ := gojq.Parse(`select(.field_0 == "xxxxxxxxxx" and .field_1 == "xxxxxxxxxx")`)
	code, _ := gojq.Compile(query)
	b.ReportAllocs()
	for b.Loop() {
		var v any
		json.Unmarshal(smallJSON, &v)
		iter := code.Run(v)
		result, _ := iter.Next()
		benchSink, _ = json.Marshal(result)
	}
}

func BenchmarkGojq_Small_SelectOr(b *testing.B) {
	query, _ := gojq.Parse(`select(.field_0 == "xxxxxxxxxx" or .field_4 == "xxxxxxxxxx")`)
	code, _ := gojq.Compile(query)
	b.ReportAllocs()
	for b.Loop() {
		var v any
		json.Unmarshal(smallJSON, &v)
		iter := code.Run(v)
		result, _ := iter.Next()
		benchSink, _ = json.Marshal(result)
	}
}

func BenchmarkGojq_Small_Has(b *testing.B) {
	query, _ := gojq.Parse(`select(has("field_2"))`)
	code, _ := gojq.Compile(query)
	b.ReportAllocs()
	for b.Loop() {
		var v any
		json.Unmarshal(smallJSON, &v)
		iter := code.Run(v)
		result, _ := iter.Next()
		benchSink, _ = json.Marshal(result)
	}
}

func BenchmarkGojq_Small_IfThenElse(b *testing.B) {
	query, _ := gojq.Parse(`if .field_0 == "xxxxxxxxxx" then .field_0 else "default" end`)
	code, _ := gojq.Compile(query)
	b.ReportAllocs()
	for b.Loop() {
		var v any
		json.Unmarshal(smallJSON, &v)
		iter := code.Run(v)
		result, _ := iter.Next()
		benchSink, _ = json.Marshal(result)
	}
}

func BenchmarkGojq_Small_Length(b *testing.B) {
	query, _ := gojq.Parse(`.field_0 | length`)
	code, _ := gojq.Compile(query)
	b.ReportAllocs()
	for b.Loop() {
		var v any
		json.Unmarshal(smallJSON, &v)
		iter := code.Run(v)
		result, _ := iter.Next()
		benchSink, _ = json.Marshal(result)
	}
}

func BenchmarkGojq_Small_Map(b *testing.B) {
	query, _ := gojq.Parse(`map(.name)`)
	code, _ := gojq.Compile(query)
	b.ReportAllocs()
	for b.Loop() {
		var v any
		json.Unmarshal(smallArray, &v)
		iter := code.Run(v)
		result, _ := iter.Next()
		benchSink, _ = json.Marshal(result)
	}
}

func BenchmarkGojq_Small_ToEntries(b *testing.B) {
	query, _ := gojq.Parse(`to_entries`)
	code, _ := gojq.Compile(query)
	b.ReportAllocs()
	for b.Loop() {
		var v any
		json.Unmarshal(smallJSON, &v)
		iter := code.Run(v)
		result, _ := iter.Next()
		benchSink, _ = json.Marshal(result)
	}
}

func BenchmarkGojq_Small_WithEntries(b *testing.B) {
	query, _ := gojq.Parse(`with_entries(select(.value != null))`)
	code, _ := gojq.Compile(query)
	b.ReportAllocs()
	for b.Loop() {
		var v any
		json.Unmarshal(smallJSON, &v)
		iter := code.Run(v)
		result, _ := iter.Next()
		benchSink, _ = json.Marshal(result)
	}
}

func BenchmarkGojq_Small_First(b *testing.B) {
	query, _ := gojq.Parse(`first(.[] | select(. > 2))`)
	code, _ := gojq.Compile(query)
	input := []byte(`[1,2,3,4,5]`)
	b.ReportAllocs()
	for b.Loop() {
		var v any
		json.Unmarshal(input, &v)
		iter := code.Run(v)
		result, _ := iter.Next()
		benchSink, _ = json.Marshal(result)
	}
}

func BenchmarkGojq_Small_Last(b *testing.B) {
	query, _ := gojq.Parse(`last(.[] | select(. > 2))`)
	code, _ := gojq.Compile(query)
	input := []byte(`[1,2,3,4,5]`)
	b.ReportAllocs()
	for b.Loop() {
		var v any
		json.Unmarshal(input, &v)
		iter := code.Run(v)
		result, _ := iter.Next()
		benchSink, _ = json.Marshal(result)
	}
}

func BenchmarkGojq_Small_Limit(b *testing.B) {
	query, _ := gojq.Parse(`limit(3; .[])`)
	code, _ := gojq.Compile(query)
	input := []byte(`[10,20,30,40,50]`)
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

func BenchmarkGojq_Small_KeysUnsorted(b *testing.B) {
	query, _ := gojq.Parse("keys") // gojq uses keys; keys_unsorted not supported
	code, _ := gojq.Compile(query)
	b.ReportAllocs()
	for b.Loop() {
		var v any
		json.Unmarshal(smallJSON, &v)
		iter := code.Run(v)
		result, _ := iter.Next()
		benchSink, _ = json.Marshal(result)
	}
}

func BenchmarkGojq_Small_Any(b *testing.B) {
	input := []byte(`[false,false,true,false,false]`)
	query, _ := gojq.Parse("any")
	code, _ := gojq.Compile(query)
	b.ReportAllocs()
	for b.Loop() {
		var v any
		json.Unmarshal(input, &v)
		iter := code.Run(v)
		result, _ := iter.Next()
		benchSink, _ = json.Marshal(result)
	}
}

func BenchmarkGojq_Small_AnyExpr(b *testing.B) {
	input := []byte(`["aaaa","bbbb","xxxxxxxxxx","cccc","dddd"]`)
	query, _ := gojq.Parse(`any(. == "xxxxxxxxxx")`)
	code, _ := gojq.Compile(query)
	b.ReportAllocs()
	for b.Loop() {
		var v any
		json.Unmarshal(input, &v)
		iter := code.Run(v)
		result, _ := iter.Next()
		benchSink, _ = json.Marshal(result)
	}
}

func BenchmarkGojq_Small_AsciiDowncase(b *testing.B) {
	query, _ := gojq.Parse(`select(.field_2 | ascii_downcase == "xxxxxxxxxx")`)
	code, _ := gojq.Compile(query)
	b.ReportAllocs()
	for b.Loop() {
		var v any
		json.Unmarshal(smallJSON, &v)
		iter := code.Run(v)
		result, _ := iter.Next()
		benchSink, _ = json.Marshal(result)
	}
}

func BenchmarkGojq_Small_Startswith(b *testing.B) {
	query, _ := gojq.Parse(`select(.field_2 | startswith("xxx"))`)
	code, _ := gojq.Compile(query)
	b.ReportAllocs()
	for b.Loop() {
		var v any
		json.Unmarshal(smallJSON, &v)
		iter := code.Run(v)
		result, _ := iter.Next()
		benchSink, _ = json.Marshal(result)
	}
}

func BenchmarkGojq_Small_Endswith(b *testing.B) {
	query, _ := gojq.Parse(`select(.field_2 | endswith("xxx"))`)
	code, _ := gojq.Compile(query)
	b.ReportAllocs()
	for b.Loop() {
		var v any
		json.Unmarshal(smallJSON, &v)
		iter := code.Run(v)
		result, _ := iter.Next()
		benchSink, _ = json.Marshal(result)
	}
}

func BenchmarkGojq_Small_Ltrimstr(b *testing.B) {
	query, _ := gojq.Parse(`.field_2 | ltrimstr("xxx")`)
	code, _ := gojq.Compile(query)
	b.ReportAllocs()
	for b.Loop() {
		var v any
		json.Unmarshal(smallJSON, &v)
		iter := code.Run(v)
		result, _ := iter.Next()
		benchSink, _ = json.Marshal(result)
	}
}

func BenchmarkGojq_Small_Rtrimstr(b *testing.B) {
	query, _ := gojq.Parse(`.field_2 | rtrimstr("xxx")`)
	code, _ := gojq.Compile(query)
	b.ReportAllocs()
	for b.Loop() {
		var v any
		json.Unmarshal(smallJSON, &v)
		iter := code.Run(v)
		result, _ := iter.Next()
		benchSink, _ = json.Marshal(result)
	}
}
