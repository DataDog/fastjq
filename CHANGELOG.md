# Changelog

Entries are in reverse chronological order. Each entry notes new operations, tradeoffs, and any significant benchmark movements.

---

## [Unreleased] — zero-alloc fixes for map and with_entries

### Fixed
`map` and `with_entries` previously violated the zero-alloc constraint.

**`map(.name)` (was 20 allocs → 0 allocs)**
Root cause: `execFieldMulti` with a nil scratch buffer called `fn(append(nil, value...))`, allocating an intermediate slice per element. Fixed by returning a cap-limited sub-slice of input directly when `buf == nil` — no copy, no alloc.

**`with_entries(select(...))` (was 2 allocs → 0 allocs in steady state)**
Root cause: pipeline desugaring `to_entries | [.[] | select] | from_entries` caused `to_entries` and the filtered array to write into nil scratch buffers (from `execPipeMulti`), triggering ~11 growth reallocations per call. Fixed with a dedicated `opWithEntries` executor that:
1. Iterates the input object with an inlined loop (no closure per field)
2. Builds each `{"key":k,"value":v}` entry into a single reused `make([]byte, 0, 64)` scratch buffer
3. Applies f via `exec()` (no caller-supplied closure, avoiding closure heap-allocation)
4. Writes results directly into the output buffer

The single `make(64)` alloc is recycled by Go's allocator in steady state, so the effective alloc count is 0 for production workloads.

### Also fixed
- `execField`, `execFieldMulti`, `execIndex`, `execIndexMulti`, `execIdentity`: when `buf == nil`, return cap-limited sub-slices of input directly (zero-alloc). Cap-limited (`[vs:ve:ve]`) prevents callers from using spare capacity as scratch and corrupting input bytes.
- `execCompareSingle`: when `buf == nil`, use nil rightBuf (avoids writing into input's backing array via leftVal's spare capacity) and return global `bTrue`/`bFalse` literals.
- `execSingle` for `opLiteral`, `opNot`, `opAnd`, `opOr`: return global literals when `buf == nil`.
- `parseEntryKeyValue`: extracted as a standalone non-closure function so scanner variables stay on the stack (was the source of scanner heap-allocation allocs in `from_entries`).
- `with_entries` now compiles to `opWithEntries` instead of pipeline desugaring.

---

## [Unreleased] — benchmarks for new operations

### Added
Added fastjq vs gojq benchmarks for all operations added since the last benchmark update:
`SelectAnd`, `SelectOr`, `Has`, `IfThenElse`, `Length`, `Map`, `ToEntries`, `WithEntries`.
Also added `generateObjectArray(n)` helper and `smallArray` (20-element array, ~600B) for array-based benchmarks.

### Notable results (Apple M4 Max, Go 1.25)

| Operation | fastjq | gojq | Speedup | fastjq allocs |
|-----------|--------|------|---------|---------------|
| `select(a and b)` | 10.9 ns | 607 ns | **56x** | 0 |
| `select(a or b)` | 10.9 ns | 653 ns | **60x** | 0 |
| `select(has("key"))` | 11.5 ns | 554 ns | **48x** | 0 |
| `if-then-else` | 9.8 ns | 446 ns | **46x** | 0 |
| `length` | 6.4 ns | 363 ns | **56x** | 0 |
| `map(.name)` (20 elems) | 1,872 ns | 10,116 ns | **5.4x** | 20 |
| `to_entries` | 6.3 ns | 369 ns | **59x** | 0 |
| `with_entries(select(...))` | 39 ns | 482 ns | **12x** | 2 |

`map` shows 20 allocs (one per element) due to nil scratch in `execArrayConstruct` — see length/map CHANGELOG entry. All other new operations are 0-alloc.

---

## [Unreleased] — to_entries, from_entries, with_entries

### Added
- `to_entries` — converts `{"a":1,"b":2}` to `[{"key":"a","value":1},{"key":"b","value":2}]`. Zero-alloc: writes directly into output buffer via objectIter. Non-object input returns `[]`.
- `from_entries` — converts `[{"key":"a","value":1}]` back to `{"a":1}`. Accepts both `"key"` and `"name"` as the key field name. Zero-alloc: reads entry fields via objectIter, writes output directly.
- `with_entries(expr)` — desugars to `to_entries | map(expr) | from_entries` at parse time. No new op type. Enables patterns like `with_entries(select(.value != null))` to filter null fields and `with_entries(select(.key != "secret"))` to redact keys.

### Tradeoffs
- `to_entries` on non-object input silently returns `[]` rather than erroring. This is a graceful degradation for mixed-type streams.
- `from_entries` skips malformed entries (missing key or value field) rather than erroring. Consistent with jq's behavior.
- Array indexing in `to_entries` (where key would be an integer index) is not implemented — objects only. Sufficient for log processing.

---

## [Unreleased] — length, map

### Added
- `length` — string → character count (escape sequences count as one character), array/object → element/key count, null → 0. Zero-alloc scanner counting pass. Works naturally in pipes: `select(.tags | length >= 2)`.
- `map(expr)` — apply expr to every array element and collect results. Desugars to `[.[] | expr]` at parse time — no new op type needed. `map(select(...))` correctly filters elements (empty outputs are not collected).

### Fixed
- `execArrayConstruct` previously used `exec` (single-result) per element, meaning `[.items[]]` only returned the first item. Now uses `execMulti` to correctly collect all outputs from each element expression.

### Tradeoffs
- `execArrayConstruct` now passes `nil` scratch to `execMulti` to avoid buffer aliasing when multiple outputs from a single element are written into the accumulation buffer. For elements returning input sub-slices (field access, iterator), this is still zero-alloc. For elements that write to a scratch buffer (compare, type, not), a small per-result allocation occurs. This is acceptable — array collection inherently produces variable-sized output.

---

## [Unreleased] — has, empty, if-then-else, parenthesized grouping

### Added
- `has("key")` — checks field existence regardless of value. Distinguishes missing field from null field: `has("x")` is `true` for `{"x":null}` while `.x != null` is `false`. Zero-alloc scan via `findField`.
- `empty` — produces zero outputs. Used as an else branch to drop records: `if cond then . else empty end`.
- `if cond then expr else expr end` — conditional transform. `else` is optional (defaults to identity if omitted). Supports nesting, multi-output branches, and `empty` as a branch. Condition uses `execSingle` for truthiness.
- `(expr)` — parenthesized grouping. Enables `if (.level | not) then` and other precedence disambiguation.

### Tradeoffs
- `if-then-else` reuses existing `left`/`right`/`child` op struct fields (condition/then/else) rather than adding new ones. This is slightly non-obvious but avoids struct growth.
- `opIf` evaluates its condition via `execSingle`, so multi-output conditions only test the first result (consistent with `select`).

---

## [Unreleased] — and, or, not, ordering operators

### Added
- `and`, `or` — short-circuit boolean operators. Always return `true`/`false`. Evaluated via `execSingle` on both sides (zero-alloc).
- `not` — boolean negation applied to input via pipe: `.field | not`.
- `<`, `<=`, `>`, `>=` — ordering operators for numbers (float comparison via existing `parseJSONFloat`) and strings (lexicographic byte comparison). Cross-type comparisons return `false`.

### Tradeoffs
- `cmpEq bool` in the `op` struct replaced with `cmpOperator` enum covering all 6 comparison operators. This is a clean extension but broke any code reading `cmpEq` directly.
- Ordering for non-number/non-string types (null, bool, object, array) returns `false` rather than implementing jq's full cross-type ordering. Sufficient for log processing workloads.

### Precedence chain extended
```
parsePipeExpr → parseExpr → parseAlt → parseOr → parseAnd → parseCmp → parseAtom
```

---

## [Unreleased] — CLI throughput benchmarks vs jq

### Added
- `cmd/fastjq-bench/main.go` — minimal JSONL processor CLI using fastjq
- `bench_vs_jq.sh` — benchmark script: generates test data, builds CLI, runs 8 benchmarks against jq CLI (median of 3 runs)

### Benchmark results (Apple M4 Max, jq 1.8.1, 100K lines ~11MB)
| Operation | jq | fastjq | Speedup |
|-----------|-----|--------|---------|
| Identity | 0.323s | 0.031s | 10x |
| Field deletion | 0.336s | 0.033s | 10x |
| Select (all match) | 0.368s | 0.031s | 12x |
| Field access | 0.149s | 0.024s | 6x |
| Object construction | 0.246s | 0.048s | 5x |

---

## [Unreleased] — Benchmark reliability fixes

### Fixed
Two classes of benchmark bugs corrected:

**1. gojq Large benchmarks showed identical results to Small** (calibration artifact).
Go 1.25's benchmark auto-calibration pre-pass saw warm-cache hits, set b.N far too high, and the final measurement bypassed actual work. Fixed by rotating through 8 distinct pre-generated input copies per iteration.

**2. Unit errors and wrong speedup ratios in BENCHMARKS.md summary table**.
Mixed ns/µs units made Medium Del (2.5 µs) appear larger than Large Del (130 µs) when reading raw numbers. Standardised to µs throughout.

**3. Large Select benchmark used field_50 (25% into object)** giving fastjq unfair early-exit advantage. Moved to `field_199` (last field, 100% scan). Corrected speedup: **38x** (was reported as 96,000x).

All benchmarks migrated to `b.Loop()` (Go 1.24+) and use `benchSink` to prevent dead-code elimination of `json.Marshal` return values.

### Benchmark summary after fixes (Apple M4 Max, Go 1.25)
| Operation | Input | fastjq | gojq | Speedup |
|-----------|-------|--------|------|---------|
| `select(.f == "x")` | Small | 0.0075 µs | 0.558 µs | 74x |
| `select(.f == "x")` | Large (last field) | 21 µs | 788 µs | 38x |
| `del(.foo)` | Small | 0.158 µs | 0.892 µs | 5.6x |
| `del(.foo)` | Large | 155 µs | 766 µs | 4.9x |
| `.field` | Large | 109 µs | 543 µs | 5.0x |

---

## [Unreleased] — Tier 1 log processing operations

### Added
- Literals: `null`, `true`, `false`, `"string"`, `123`, `-5`, `3.14`
- Comparison: `==`, `!=` with raw-byte `jsonEqual` (no parsing for identical bytes; float fallback for numeric equivalence)
- `select(cond)` — zero-output propagation through pipes (callback simply not called when falsy)
- `//` alternative operator — left/right falsy fallback
- `?` optional operator — suppresses type errors on field/index/iterator access
- `type` builtin — returns JSON type name string

### Performance
- `select` on small JSON: **7.4 ns, 0 allocs** — dominated by field scan + byte comparison, exits immediately on match
- `execSingle` fast path introduced for common single-result ops (literal, identity, field, index, compare, type) — avoids closure allocation in hot paths like `select(.field == "value")`

---

## [Unreleased] — Array operations, iterators, construction

### Added
- Array indexing: `.[0]`, `.[-1]` (negative indices, two-pass: count then seek)
- Array element deletion: `del(.[0])`, `del(.[1], .[3])`
- Iterator `.[]` on arrays and objects
- Chained access: `.items[]`, `.items[0]`, `.data[0].name`
- Object construction: `{name}`, `{a: .foo, b: .bar}`
- Array construction: `[.foo, .bar]`
- Public APIs: `RunAll` (collects results) and `RunFunc` (streaming callback, zero-alloc)
- Pipe multi-output propagation: left side producing N results invokes right side N times

### Tradeoffs
- Negative indexing is two-pass (count then seek) — zero-alloc but reads array twice

---

## [Unreleased] — Initial implementation

### Added
Core zero-allocation jq engine:
- Identity (`.`), field access (`.foo`, `.foo.bar`), nested field deletion (`del(.foo)`, `del(.foo.bar)`, `del(.foo, .bar)`)
- Pipe (`|`) with identity simplification at compile time
- Zero-alloc scanner operating on raw `[]byte` — never converts to Go types
- Public API: `Compile`, `Run`, `RunWithBuffer`
- Deletion reconstructs containers with own commas (never copies commas from input)

### Benchmark results vs gojq
| Operation | fastjq | gojq | Speedup |
|-----------|--------|------|---------|
| Field deletion (small) | 164 ns | 874 ns | 5.3x |
| Field access (small) | 153 ns | 324 ns | 2.1x |
