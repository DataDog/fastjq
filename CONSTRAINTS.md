# fastjq Project Constraints

## Performance Constraints

- **Zero allocations** on the hot path when using `RunWithBuffer` with a reused buffer — all operations including select, compare, and alternative achieve 0 allocs
- **Static error sentinels** on hot-path functions: `fmt.Errorf` with dynamic args poisons escape analysis, so hot-path error returns use pre-allocated `errors.New` values
- **No marshal/unmarshal**: never converts to `interface{}`, `map[string]interface{}`, or any Go type — operates entirely on raw `[]byte`
- **No data copying** except into the output buffer: scanner returns sub-slices of input, field values are copied verbatim
- **Output <= input**: deletion output is always smaller than or equal to input, so output buffer can be pre-allocated at `cap = len(input)`

## Scope Constraints

- **Supported operations**: identity (`.`), field access (`.foo`, `.foo.bar`), array indexing (`.[0]`, `.[-1]`), slicing (`.[n:m]`, `.[:m]`, `.[n:]`), deletion (`del(.foo)`, `del(.[0])`), iteration (`.[]`), object construction (`{name}`, `{a: .foo}`), array construction (`[.foo, .bar]`), `map(expr)`, `add`, `expr + expr`, `flatten`/`flatten(n)`, `split("s")`, `join("s")`, `to_entries`, `from_entries`, `with_entries(expr)`, `keys_unsorted`, `any`/`any(expr)`, `all`/`all(expr)`, `first`/`first(expr)`, `last`/`last(expr)`, `limit(n; expr)`, pipe (`expr | expr`), grouping (`(expr)`), literals (`null`, `true`, `false`, `"string"`, `123`), comparison (`==`, `!=`, `<`, `<=`, `>`, `>=`), boolean (`and`, `or`, `not`), `has("key")`, `length`, `ascii_downcase`, `ascii_upcase`, `startswith("s")`, `endswith("s")`, `ltrimstr("s")`, `rtrimstr("s")`, `if-then-else-end`, `empty`, select (`select(cond)`), alternative (`//`), optional (`.foo?`), type (`type`)
- **Input format**: valid JSON objects or arrays — no streaming, no JSONL
- **No validation**: assumes well-formed JSON input; behavior on malformed input is undefined
- **No pretty-printing**: output is compact JSON only
- **`select` condition must be single-output**: `execSelect` evaluates the condition via `execSingle`, which captures only the first result. Conditions that produce multiple values (e.g. `select(.items[] == "x")`) silently test only the first element. Use simple field comparisons: `select(.field == "value")`.
- **`del` paths must be literal field or index expressions**: `del(.foo)`, `del(.foo.bar)`, `del(.[0])`, and `del(.foo, .bar)` are supported. Dynamic paths are not: `del(.items[])` and `del(.items[] | select(...))` both return an error. There is no way to delete multiple elements matched by a runtime condition.

## Design Constraints

- **Scanner is stateless between runs**: `struct { data []byte; pos int }` reset per call
- **AST allocates once at compile time**: `Compile()` allocates, `Run()` does not (with buffer reuse, for basic ops)
- **Comma reconstruction**: deletion never copies commas from input; reconstructs containers with own commas to avoid trailing-comma bugs
- **String comparison without allocation**: `bytesEqualStr` compares `[]byte` keys to `string` field names without converting
- **Multi-output via callback**: `execMulti` uses `func([]byte) error` callback to avoid allocating result slices internally
- **Negative indexing is two-pass**: `arrayLen()` counts first (no alloc), then iterates to resolved index
- **Precedence via function chain**: `parseExpr` → `parseAlt` → `parseCmp` → `parseAtom` — no precedence table
- **Literals store raw JSON bytes**: compiled at parse time, zero-alloc at runtime
- **isFalsy by first-byte check**: `n` = null, `f` = false — one branch, zero alloc
- **Number comparison**: byte-identical fast path (zero-alloc), `parseFloat` slow path using `unsafe.String` to avoid string allocation
- **Optional is a flag, not an op type**: `node.optional = true` keeps AST simple

## Testing Constraints

- All operations must be covered by unit tests
- Benchmarks must compare against gojq on small (~100B), medium (~2KB), and large (~100KB+) JSON
- Benchmarks report ns/op, B/op, and allocs/op
- CLI throughput benchmarks compare against jq CLI on JSONL streams via `bench_vs_jq.sh`
