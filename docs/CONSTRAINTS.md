# fastjq Project Constraints

## Safety Constraints

- **Never panics.** fastjq must never panic regardless of input — valid JSON, malformed JSON, empty input, or arbitrary bytes. A panic in a log processing pipeline is worse than wrong output. This guarantee is enforced by `TestNoPanicMalformedInput` (deterministic) and three fuzz test functions (`FuzzCompile`, `FuzzRunFixed`, `FuzzBoth`). Any code path that could panic on bad input is a bug.
- **Progressive validation on malformed input.** The scanner detects structural errors (unterminated strings/containers, mismatched brackets, invalid value starts, missing object structure) during execution and returns errors. `Validate()` provides full RFC 8259 validation (escapes, control chars, number format, keywords). Both are zero-alloc.

## Performance Model

The governing principle: **allocations are proportional to what you ask for, never to what the engine scans.**

fastjq distinguishes four tiers of allocation behaviour:

| Tier | Rule | Examples |
|------|------|---------|
| **Tier 0** | 0 allocs — core processing hot path | access, filter, compare, arithmetic, construction, math |
| **Tier 1** | Allocs ∝ output size — unavoidable API constraints | `@base64`, `@uri`, `match`, `capture`, `scan`, `gsub` |
| **Tier 2** | Allocs ∝ output count — bounded by what was requested | `range(n)` (1 alloc/value); `sort`, `group_by`, `unique` *(planned)* |
| **Tier 3** | Deferred — requires executor redesign, not a policy decision | `recurse`/`..` |

When deciding whether to implement a new operation, the question is always: *does the allocation scale with the result, or with the input?* If the caller can control the allocation by choosing what to request, it is acceptable. If the allocation scales with the shape of the data being processed regardless of the query, it is rejected.

### Tier 0 — Zero-alloc (core processing)

Field access, filtering, comparison, arithmetic, construction, `map(.field)`, math builtins, `test(re)`, string operations, and most other operations achieve **0 allocs/op** at steady state when using `RunWithBuffer` or `RunFunc` with a reused buffer.

This is the hot path for log processing. Any regression to non-zero allocs on these operations is a bug.

Implementation rules:
- `RunWithBuffer` / `RunFunc` must achieve 0 allocs for all Tier 0 operations
- Static error sentinels (`errors.New`) on hot-path functions — `fmt.Errorf` with dynamic args poisons escape analysis
- No `interface{}`, `map[string]interface{}`, or any Go type conversion — operate entirely on raw `[]byte`
- No data copying except into the output buffer

### Tier 1 — Alloc ∝ output size (unavoidable)

Some operations must allocate to produce their result. The allocation is proportional to the *output*, not the input being scanned. Even gojq allocates thousands of times just to begin; these ops allocate a handful of times for specific results.

| Operation | Allocs | Reason |
|-----------|--------|--------|
| `@base64`, `@uri` | ~4 | String-escape decoding before encoding |
| `match(re)`, `capture(re)` | 1 on hit, 0 on miss | Go's `FindSubmatchIndex` `[]int` (unavoidable API) |
| `scan(re)`, `gsub(re)` | ∝ match count | Multi-result output |
| `sub(re)` | 1 on hit | `FindIndex []int` |
| `map(f)` when `f` constructs | ~1 per element | `execArrayConstruct` aliasing constraint |
| `error(expr)` | 1 on throw | `jsonError` payload copy (exceptional path only) |

**Exception — array construction aliasing:** `[.[] | f]` where `f` constructs new data (object construction, arithmetic, string concatenation) allocates ~1 buffer per element. This is a structural limitation: `execArrayConstruct` must pass `nil` scratch to `execMulti` to prevent aliasing when an element's iterator emits multiple results across callback invocations. Elements that return input sub-slices (field access, identity, comparisons) remain 0 allocs.

### Tier 2 — Alloc ∝ output count (bounded, acceptable)

Operations where allocation is proportional to *what the caller asked to produce*, not what was scanned. The caller controls the allocation by deciding what to request.

| Operation | Allocs | Reason |
|-----------|--------|--------|
| `range(n)` | 1 per value | Each generated integer is a fresh byte slice. For `range(10)`: 10 allocs. |
| `range(from;to;step)` | 1 per value | Same model with explicit bounds and step. |
| `sort`, `sort_by(f)` *(planned)* | O(n) index | Collect element offsets, sort, re-emit. |
| `group_by(f)` *(planned)* | O(n) index | Sort-based grouping. |
| `unique`, `unique_by(f)` *(planned)* | O(n) index | Sort-based deduplication. |

Rule: Tier 2 operations must document their alloc model in CHANGELOG and SYNTAX.md. They must not appear in hot-path benchmarks that assert 0 allocs.

### Tier 3 — Deferred (requires executor redesign)

These operations are not rejected on principle — they're deferred because implementing them correctly requires a different execution architecture, not just documented allocations.

| Operation | Why deferred |
|-----------|-------------|
| `recurse` / `..` | The callback-based executor (`fn func([]byte) error`) forces Go's escape analysis to heap-allocate a closure at every JSON nesting level (~3–4 allocs/level). On a 10-deep record that's ~40 allocs regardless of query complexity, and the caller cannot bound it. Fixing this requires replacing the callback architecture with an explicit stack — a significant redesign. |
| `range(n)` / `range(from;to;step)` | Synthesises new data not present in the input. Formatting each integer requires a buffer passed through the function interface — Go's escape analysis heap-allocates it, giving 1 alloc/value. This is the one case where the allocation comes from the operation itself, not its input or output. |

## Scope Constraints

- **Supported operations**: identity (`.`), field access (`.foo`, `.foo.bar`), array indexing (`.[0]`, `.[-1]`), slicing (`.[n:m]`, `.[:m]`, `.[n:]`), deletion (`del(.foo)`, `del(.[0])`, `del(.[n:m])`), iteration (`.[]`), object construction (`{name}`, `{a: .foo}`), array construction (`[.foo, .bar]`), `map(expr)`, `add`, `expr + expr` (including object merge), `expr - expr`, `expr * expr` (including `string * n`, `n * string`, `object * object` recursive merge), `expr / expr`, `expr % expr`, `flatten`/`flatten(n)`, `split("s")`, `join("s")`, `min`/`max`, `min_by(f)`/`max_by(f)`, `to_entries`, `from_entries`, `keys_unsorted`, `any`/`any(expr)`/`any(gen; cond)`, `all`/`all(expr)`/`all(gen; cond)`, `first`/`first(expr)`, `last`/`last(expr)`, `limit(n; expr)`, `nth(n; gen)`, `isempty(expr)`, string interpolation (`"\(expr)"`), pipe (`expr | expr`), grouping (`(expr)`), literals (`null`, `true`, `false`, `"string"`, `123`), comparison (`==`, `!=`, `<`, `<=`, `>`, `>=`), boolean (`and`, `or`, `not`), `has("key")`, `length`, `ascii_downcase`, `ascii_upcase`, `startswith("s")`, `endswith("s")`, `ltrimstr("s")`, `rtrimstr("s")`, `if-then-elif-...-else-end`, `empty`, select (`select(cond)`), alternative (`//`), optional (`.foo?`), type (`type`), `try`/`try-catch`, `error`/`error(expr)`, `tojson`/`@json`, `fromjson`, `tostring`/`@text`, `tonumber`, `@base64`, `@base64d`, `@uri`, `@urid`, `@html`, `@csv`, `@tsv`, `@sh`, `floor`, `ceil`, `round`, `nearbyint`, `contains(val)`, `inside(val)`, `index(s)`, `rindex(s)`, `indices(s)`, `sqrt`, `fabs`, `atan` (1-arg), `log`, `log2`, `log10`, `exp`, `exp2`, `exp10`, `cbrt`, `logb`, `sin`, `cos`, `tan`, `asin`, `acos`, `tgamma`, `lgamma`, `j0`, `j1`, `test(re)`/`test(re; flags)`, `match(re)`/`match(re; flags)`, `capture(re)`/`capture(re; flags)`, `scan(re)`/`scan(re; flags)`, `sub(re; s)`, `gsub(re; s)`
- **Input format**: valid JSON objects or arrays — no streaming, no JSONL
- **Progressive validation**: the scanner detects structural errors during execution; `Validate()` provides full RFC 8259 checking. Validation errors bypass `try-catch` (they indicate malformed input, not query-level errors).
- **No pretty-printing**: output is compact JSON only
- **`select` condition must be single-output**: `execSelect` evaluates the condition via `execSingle`, which captures only the first result. Conditions that produce multiple values silently test only the first element. Use simple field comparisons: `select(.field == "value")`.
- **`if-then-else` condition supports multi-output**: `execIf` uses `execMulti` for the condition, so generators like `if empty then x end` correctly produce zero outputs. Single-output conditions take a fast path to avoid closure allocation.
- **`del` paths must be literal field, index, or slice expressions**: `del(.foo)`, `del(.foo.bar)`, `del(.[0])`, `del(.foo, .bar)`, `del(.[n:m])`, and `del(.[-n:])` are supported. Dynamic paths are not: `del(.items[])` and `del(.items[] | select(...))` both return an error. There is no way to delete multiple elements matched by a runtime condition.

## Design Constraints

- **Scanner is stateless between runs**: `struct { data []byte; pos int; err error }` reset per call; `err` captures validation errors during scanning
- **AST allocates once at compile time**: `Compile()` allocates, `Run()` does not (with buffer reuse, for Tier 0 ops)
- **Comma reconstruction**: deletion never copies commas from input; reconstructs containers with own commas to avoid trailing-comma bugs
- **String comparison without allocation**: `bytesEqualStr` compares `[]byte` keys to `string` field names without converting; `findFieldStr` avoids `[]byte(string)` conversion at call sites
- **Multi-output via callback**: `execMulti` uses `func([]byte) error` callback to avoid allocating result slices internally
- **Negative indexing is two-pass**: `arrayLen()` counts first (no alloc), then iterates to resolved index
- **Precedence via function chain**: `parsePipeExpr` → `parseExpr` → `parseAlt` → `parseOr` → `parseAnd` → `parseCmp` → `parseAtom` — no precedence table
- **`//` operator collects all truthy left outputs**: `execAlternative` uses `execMulti` for left side; if any output is truthy, it passes through; only if ALL outputs are falsy does right side run
- **Literals store raw JSON bytes**: compiled at parse time, zero-alloc at runtime
- **isFalsy by first-byte check**: `n` = null, `f` = false — one branch, zero alloc
- **Number comparison**: byte-identical fast path (zero-alloc), `parseFloat` slow path using `unsafe.String` to avoid string allocation
- **Optional is a flag, not an op type**: `node.optional = true` keeps AST simple
- **`try` propagates `errBreak` and validation errors**: `errBreak` is a control signal (for `first`/`limit`), not an error — `opTry` in `execMulti` propagates it unchanged. Validation errors (sentinel errors from the scanner) also bypass `try-catch` — they indicate malformed input, not query-recoverable errors
- **`exec` routes through `execSingle`**: `execSingle` handles all Tier 0 ops with direct return paths (no closures). Multi-output and Tier 1+ ops fall back to `execFirstResult` which uses the closure-based `execMulti` machinery.
- **`elif` desugars at parse time**: `elif C then X` rewrites to `else (if C then X end)` — no new op type needed
- **Regex patterns compiled at parse time**: `node.re *regexp.Regexp` holds the compiled RE2 pattern; Go RE2 guarantees linear-time matching (immune to ReDoS)
- **`findFieldStr` stops on match**: field lookups stop scanning as soon as the target key is found — no wasted scan of remaining fields

## Testing Constraints

- All operations must be covered by unit tests
- Benchmarks must compare against gojq on small (~100B), medium (~2KB), and large (~100KB+) JSON
- Benchmarks report ns/op, B/op, and allocs/op — **any unexpected non-zero allocs/op on a Tier 0 operation is a bug**
- Tier 1+ operations must document their alloc profile in the benchmark comment
- CLI throughput benchmarks compare against jq CLI on JSONL streams via `bench_vs_jq.sh`
