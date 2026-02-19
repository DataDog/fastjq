# Changelog

Entries are in reverse chronological order. Each entry notes new operations, tradeoffs, and any significant benchmark movements.

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
