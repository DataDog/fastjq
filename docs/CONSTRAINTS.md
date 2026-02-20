# fastjq Project Constraints

## Safety Constraints

- **Never panics.** fastjq must never panic regardless of input — valid JSON, malformed JSON, empty input, or arbitrary bytes. A panic in a log processing pipeline is worse than wrong output. This guarantee is enforced by `TestNoPanicMalformedInput` (deterministic) and three fuzz test functions (`FuzzCompile`, `FuzzRunFixed`, `FuzzBoth`). Any code path that could panic on bad input is a bug.
- **Correct results only guaranteed for valid JSON.** With malformed input the output is undefined, but the process remains safe.

## Performance Constraints

- **Zero allocations** on the hot path when using `RunWithBuffer` with a reused buffer — all operations including select, compare, and alternative achieve 0 allocs
- **Exception — array construction with data-building element expressions**: `[.[] | f]` where `f` constructs new data (object construction `{…}`, arithmetic, string concatenation) allocates ~1 buffer per element. This is a structural limitation: `execArrayConstruct` must pass `nil` scratch to `execMulti` to prevent aliasing when an element's iterator emits multiple results across callback invocations. Elements that return input sub-slices (field access, identity, comparisons) remain 0 allocs. fastjq still uses 5–8x fewer allocations than gojq on these queries.
- **Static error sentinels** on hot-path functions: `fmt.Errorf` with dynamic args poisons escape analysis, so hot-path error returns use pre-allocated `errors.New` values
- **No marshal/unmarshal**: never converts to `interface{}`, `map[string]interface{}`, or any Go type — operates entirely on raw `[]byte`
- **No data copying** except into the output buffer: scanner returns sub-slices of input, field values are copied verbatim
- **Output <= input**: deletion output is always smaller than or equal to input, so output buffer can be pre-allocated at `cap = len(input)`

## Scope Constraints

- **Supported operations**: identity (`.`), field access (`.foo`, `.foo.bar`), array indexing (`.[0]`, `.[-1]`), slicing (`.[n:m]`, `.[:m]`, `.[n:]`), deletion (`del(.foo)`, `del(.[0])`), iteration (`.[]`), object construction (`{name}`, `{a: .foo}`), array construction (`[.foo, .bar]`), `map(expr)`, `add`, `expr + expr` (including object merge), `expr - expr`, `expr * expr`, `expr / expr`, `expr % expr`, `flatten`/`flatten(n)`, `split("s")`, `join("s")`, `min`/`max`, `min_by(f)`/`max_by(f)`, `to_entries`, `from_entries`, `keys_unsorted`, `any`/`any(expr)`/`any(gen; cond)`, `all`/`all(expr)`/`all(gen; cond)`, `first`/`first(expr)`, `last`/`last(expr)`, `limit(n; expr)`, pipe (`expr | expr`), grouping (`(expr)`), literals (`null`, `true`, `false`, `"string"`, `123`), comparison (`==`, `!=`, `<`, `<=`, `>`, `>=`), boolean (`and`, `or`, `not`), `has("key")`, `length`, `ascii_downcase`, `ascii_upcase`, `startswith("s")`, `endswith("s")`, `ltrimstr("s")`, `rtrimstr("s")`, `if-then-elif-...-else-end`, `empty`, select (`select(cond)`), alternative (`//`), optional (`.foo?`), type (`type`), `try`/`try-catch`, `tojson`/`@json`, `fromjson`, `tostring`, `tonumber`, `@base64`, `@base64d`, `@uri`
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
- **`try` propagates `errBreak`**: `errBreak` is a control signal (for `first`/`limit`), not an error — `opTry` in `execMulti` propagates it unchanged
- **`objectContainsKey` uses manual scan loop**: no closure/callback to avoid heap allocation — same pattern as `arrayContainsElem`
- **`elif` desugars at parse time**: `elif C then X` rewrites to `else (if C then X end)` — no new op type needed
- **`try-catch` error message allocation**: only when an actual error occurs (exceptional path), so the `make([]byte, 0, 64)` is acceptable
- **`execSingle` for `opTry` falls back to `exec`**: `try` may suppress errors so the single-result path cannot guarantee a value; uses `exec` fallback

## Testing Constraints

- All operations must be covered by unit tests
- Benchmarks must compare against gojq on small (~100B), medium (~2KB), and large (~100KB+) JSON
- Benchmarks report ns/op, B/op, and allocs/op
- CLI throughput benchmarks compare against jq CLI on JSONL streams via `bench_vs_jq.sh`
