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

var (
	smallJSON  = generateJSON(5, 10)                   // ~100B
	mediumJSON = generateNestedJSON(20, 5, 30)         // ~2KB
	largeJSON  = generateNestedJSON(200, 10, 200)      // ~100KB+
)

// --- fastjq benchmarks ---

func BenchmarkFastjq_Small_Del(b *testing.B) {
	p, _ := Compile("del(.field_2)")
	buf := make([]byte, 0, len(smallJSON))
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf, _ = p.RunWithBuffer(smallJSON, buf)
	}
}

func BenchmarkFastjq_Medium_Del(b *testing.B) {
	p, _ := Compile("del(.field_5)")
	buf := make([]byte, 0, len(mediumJSON))
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf, _ = p.RunWithBuffer(mediumJSON, buf)
	}
}

func BenchmarkFastjq_Large_Del(b *testing.B) {
	p, _ := Compile("del(.field_50)")
	buf := make([]byte, 0, len(largeJSON))
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf, _ = p.RunWithBuffer(largeJSON, buf)
	}
}

func BenchmarkFastjq_Small_Field(b *testing.B) {
	p, _ := Compile(".field_2")
	buf := make([]byte, 0, 64)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf, _ = p.RunWithBuffer(smallJSON, buf)
	}
}

func BenchmarkFastjq_Large_Field(b *testing.B) {
	p, _ := Compile(".field_50")
	buf := make([]byte, 0, 256)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf, _ = p.RunWithBuffer(largeJSON, buf)
	}
}

// --- gojq benchmarks ---

func BenchmarkGojq_Small_Del(b *testing.B) {
	query, _ := gojq.Parse("del(.field_2)")
	code, _ := gojq.Compile(query)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var v interface{}
		json.Unmarshal(smallJSON, &v)
		iter := code.Run(v)
		result, _ := iter.Next()
		json.Marshal(result)
	}
}

func BenchmarkGojq_Medium_Del(b *testing.B) {
	query, _ := gojq.Parse("del(.field_5)")
	code, _ := gojq.Compile(query)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var v interface{}
		json.Unmarshal(mediumJSON, &v)
		iter := code.Run(v)
		result, _ := iter.Next()
		json.Marshal(result)
	}
}

func BenchmarkGojq_Large_Del(b *testing.B) {
	query, _ := gojq.Parse("del(.field_50)")
	code, _ := gojq.Compile(query)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var v interface{}
		json.Unmarshal(largeJSON, &v)
		iter := code.Run(v)
		result, _ := iter.Next()
		json.Marshal(result)
	}
}

func BenchmarkGojq_Small_Field(b *testing.B) {
	query, _ := gojq.Parse(".field_2")
	code, _ := gojq.Compile(query)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var v interface{}
		json.Unmarshal(smallJSON, &v)
		iter := code.Run(v)
		result, _ := iter.Next()
		json.Marshal(result)
	}
}

func BenchmarkGojq_Large_Field(b *testing.B) {
	query, _ := gojq.Parse(".field_50")
	code, _ := gojq.Compile(query)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var v interface{}
		json.Unmarshal(largeJSON, &v)
		iter := code.Run(v)
		result, _ := iter.Next()
		json.Marshal(result)
	}
}
