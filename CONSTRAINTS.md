# fastjq Project Constraints

## Performance Constraints

- **Zero allocations** on the hot path when using `RunWithBuffer` with a reused buffer
- **No marshal/unmarshal**: never converts to `interface{}`, `map[string]interface{}`, or any Go type — operates entirely on raw `[]byte`
- **No data copying** except into the output buffer: scanner returns sub-slices of input, field values are copied verbatim
- **Output <= input**: deletion output is always smaller than or equal to input, so output buffer can be pre-allocated at `cap = len(input)`

## Scope Constraints

- **Supported operations**: identity (`.`), field access (`.foo`, `.foo.bar`), deletion (`del(.foo)`, `del(.foo, .bar)`, `del(.foo.bar)`), pipe (`expr | expr`)
- **Input format**: valid JSON objects only — no streaming, no JSONL, no arrays at top level for deletion
- **No validation**: assumes well-formed JSON input; behavior on malformed input is undefined
- **No pretty-printing**: output is compact JSON only

## Design Constraints

- **Scanner is stateless between runs**: `struct { data []byte; pos int }` reset per call
- **AST allocates once at compile time**: `Compile()` allocates, `Run()` does not (with buffer reuse)
- **Comma reconstruction**: deletion never copies commas from input; reconstructs `{` + kept pairs with own commas + `}` to avoid trailing-comma bugs
- **String comparison without allocation**: `bytesEqualStr` compares `[]byte` keys to `string` field names without converting

## Testing Constraints

- All operations must be covered by unit tests
- Benchmarks must compare against gojq on small (~100B), medium (~2KB), and large (~100KB+) JSON
- Benchmarks report ns/op, B/op, and allocs/op
