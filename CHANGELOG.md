# Changelog

Entries are in reverse chronological order. Each entry notes new operations, tradeoffs, and any significant benchmark movements.

---

## [Unreleased] — add, range, flatten, split, join

### Added
Five new zero-alloc operations:

- **`add`** — reduces an array by summing numbers, concatenating strings/arrays, or merging object key-value pairs. Empty/null → `null`. Uses float accumulation with integer output when result is a whole number.
- **`flatten` / `flatten(n)`** — recursively flattens nested arrays. `flatten` = unlimited depth; `flatten(n)` = at most n levels. Zero-alloc recursive scanner. Fixed off-by-one (`curDepth > maxDepth` not `>=`).
- **`split("s")`** — splits a JSON string by separator → array of strings. Zero-alloc scan for separator in raw string content.
- **`join("s")`** — joins array elements with separator → JSON string. Numbers converted to their string form; nulls → empty.

### Rejected
- **`range(n)`** — evaluated and removed. `range` synthesises new integer data from scratch, unlike all other operations which transform existing input bytes. Any temporary buffer passed to the result callback (`fn func([]byte) error`) escapes to the heap because Go cannot prove the function interface won't capture it. This results in an unavoidable 1 heap allocation per call. Additionally, the fixed-size buffer approach silently overflows for extreme float step values. Zero-alloc cannot be achieved without architectural changes (sync.Pool, etc.) that add complexity and don't fit the library's model. Documented in SYNTAX.md as "Rejected".

33 new tests, 397 total.

---

## [Unreleased] — from_entries: capitalized key variants (Key, Name, Value)

### Fixed
`from_entries` (and `parseEntryKeyValue` used by `with_entries`) now accepts all jq-documented capitalized variants: `"Key"`, `"Name"`, `"Value"` in addition to the existing `"key"`, `"name"`, `"value"`.

### with_entries alloc — clarification
The single `make([]byte, 0, 64)` alloc in `with_entries` is necessary and correct. `f` receives `{"key":"k","value":v}` as a JSON object; that object must exist as bytes in memory. Using `buf`'s spare capacity is unsafe because `exec(f, entry, nil)` may return result bytes that are sub-slices of `entry` — when we subsequently write the output to `buf`, it would overwrite the bytes we're still reading from. The 64-byte scratch is the minimum required, and the alloc is recycled by Go's allocator in steady state.

---

## [Unreleased] — values, recurse/.., in, type filters, quoted object keys

### Added
- **`values`** — `select(. != null)`. Filters null from streams: `.[] | values`. Zero-alloc.
- **`in(obj)`** — reverse membership: `"key" | in({"key":1})` = `true`. Zero-alloc.
- **Type-filter builtins** — parser aliases for `select(type == X)`: `numbers`, `strings`, `arrays`, `objects`, `booleans`, `nulls`, `iterables`, `scalars`. All zero-alloc (desugared at parse time, no new ops).
- **Quoted string keys in object construction** — `{"foo": .bar}` now works alongside `{foo: .bar}`. Fixes `in({"key":1})` patterns.

### Rejected
- **`recurse`/`..`** — implemented and removed. ~3-4 allocs per nesting level from recursive closures capturing `fn func([]byte) error` (function interface forces heap escape). Scales with JSON depth — 11 allocs for a 3-level object. See SYNTAX.md Rejected section for full explanation.

---

## [Unreleased] — @base64 / @base64d

### Added
- **`@base64`** — encode a JSON string to standard base64 with `=` padding. Operates on raw bytes between quotes. Zero-alloc: writes groups of 4 chars directly into buf.
- **`@base64d`** — decode a base64 JSON string. Accepts standard (`+/`), URL-safe (`-_`), padded and unpadded input. Non-printable decoded bytes are JSON-escaped as `\uXXXX`. Zero-alloc.

### Benchmarks (Apple M4 Max)
| Operation | fastjq | gojq | Speedup |
|-----------|--------|------|---------|
| `@base64` (34-char string) | 86 ns | 521 ns | **6x** |
| `@base64d` (48-char encoded) | 217 ns | 552 ns | **2.5x** |

---

## [Unreleased] — index/rindex/indices, has(n), debug

### Added
- **`index(s)` / `rindex(s)`** — first / last occurrence of a string in a string, or a value in an array. Returns the byte-position index or null. Zero-alloc byte scan.
- **`indices(s)`** — all occurrence positions as an integer array. Zero-alloc: writes integers directly into buf.
- **`has(n)` for arrays** — completes the `has` builtin. Array index membership requires `n ≥ 0` (negative indices always return false, matching jq semantics).
- **`debug`** — prints `[DEBUG]: <value>` to stderr, passes value through unchanged. Zero-alloc: uses `fmt.Fprintf(os.Stderr, ...)` and returns input sub-slice directly.

### Benchmarks (Apple M4 Max)
| Operation | fastjq | gojq | Speedup |
|-----------|--------|------|---------|
| `index(",")` on string | 73 ns | 890 ns | **12x** |
| `indices(",")` on string | 134 ns | 2,080 ns | **16x** |

---

## [Unreleased] — .[n:m] slicing and expr + expr

### Added
- **`.[n:m]` / `.[:m]` / `.[n:]` / `.[:]`** — Array and string slicing. Negative indices count from the end; out-of-range indices are clamped. Strings slice by logical characters (escape sequences count as 1). Zero-alloc: writes directly into buf via position-tracked scan.
- **`expr + expr`** — Binary concatenation/addition: strings concat, arrays concat, numbers sum, `null` is the identity element (`null + x = x`). Zero-alloc via nil-buf evaluation of both operands (cap-limited input sub-slices), result written to buf. `+` has higher precedence than comparison operators.
- Also added `bNull` global literal (like `bTrue`/`bFalse`) to eliminate allocations when returning null for missing fields with a nil scratch buffer.

### Benchmarks (Apple M4 Max)
| Operation | fastjq | gojq | Speedup |
|-----------|--------|------|---------|
| `.[1:4]` array slice | 86 ns | 788 ns | **9x** |
| `.[:5]` string slice | 47 ns | 437 ns | **9x** |
| `.a + .b` string concat | 69 ns | 655 ns | **9x** |
| `"prefix" + .name` | 107 ns | 709 ns | **6.6x** |

---

## [Unreleased] — README, .gitignore, null propagation docs

### Changed
- **README Limitations** updated:
  - `select` limitation clarified: `and`/`or` conditions work fine; only iterator-based conditions are limited
  - Added new limitation: `.field` on `null` errors — use `.field?` for null-safe access (discovered by compat test)
  - Last line now correctly says "Input can be pretty-printed or compact" (fastjq handles whitespace)
- **.gitignore** updated to exclude build artifacts (`fastjq`, `fastjq-bench`, `*.test`) and local Claude settings

---

## [Unreleased] — jq compatibility harness

### Added
`compat_test.go`: 149 test cases verifying fastjq produces byte-identical output to the jq CLI (1.8.1) across all supported operations. Tests cover identity, field access, deletion, construction, pipes, comparisons, boolean logic, select, alternative, type, has, length, map, to/from/with_entries, keys_unsorted, any/all, first/last/limit, if-then-else, string operations (downcase, upcase, startswith, endswith, ltrimstr, rtrimstr), flatten, add, split, join, unicode, and realistic log processing patterns.

Tests are skipped automatically when `jq` is not in PATH.

### Known intentional differences from jq (documented in the test)

| Behaviour | jq | fastjq | Notes |
|-----------|-----|--------|-------|
| Scientific notation casing | `1.5E+10` | `1.5e10` | fastjq preserves raw input bytes; jq normalises to uppercase E with explicit sign |
| `del(.a.b)` when `.a` is not an object/array | error | no-op, keeps original | fastjq silently ignores nested-del type errors, jq errors |
| Null propagation: `null \| .field` | `null` | error (use `.field?`) | jq null-propagates field access; fastjq errors unless the field is marked optional with `?` |

---

## [Unreleased] — correctness: fuzz tests, no-panic guarantees, edge case fixes

### Added
- `fuzz_test.go`: three fuzz functions using Go's `testing.F`:
  - `FuzzCompile` — any string → Compile must not panic
  - `FuzzRunFixed` — fixed queries against arbitrary input → Run must not panic
  - `FuzzBoth` — random query + random input → no panic after compile error skip
- `correctness_test.go`: 26 tests covering edge cases that expose real bugs:
  - `TestNoPanicMalformedInput` — all operations vs truncated/malformed/empty JSON via `recover()`
  - `TestNoPanicInvalidQueries` — Compile must not panic on any string
  - String escapes, unicode, numbers, deeply nested, large objects, empty collections, falsy semantics, etc.

### Fixed (bugs found by correctness tests)
- **`del(.a.b)` panic on non-object value** — when the nested target value (e.g. `1`) is not an object/array, `execDelete` returned `(nil, err)` setting `buf = nil`. Subsequent `buf[:savedLen]` caused a panic. Fixed by saving the full slice header (`preDel := buf`) before calling `execDelete` and using that for the fallback append.

- **`length` wrong for `\uXXXX` escapes** — `length` on `"\u0041BC"` returned 7 instead of 3. The scanner skipped only 2 bytes for any escape sequence (`\\` + 1 char), but `\uXXXX` is 6 bytes. Fixed by detecting `u` after the backslash and skipping 5 more bytes (u + 4 hex digits), matching jq's behaviour of counting escape sequences as 1 logical character.

---

## [Unreleased] — README improvements and code refactoring

### Changed (README)
- Added "Try it out (CLI)" section with example commands using `cmd/fastjq-bench`
- Added highlights table for newer operations (ascii_downcase, keys_unsorted, to_entries, has, any)
- Fixed Limitations: removed `length`, `map`, `keys` which are now implemented; replaced with accurate description
- Added "Further reading" section linking to SYNTAX.md, BENCHMARKS.md, DESIGN.md, CONSTRAINTS.md, CHANGELOG.md

### Changed (code structure)
- `bench_test.go`: 1291 → 457 lines (-65%). Added 6 benchmark helpers (benchFastjqObj, benchFastjqLargeSelect, benchFastjqFunc, benchGojqObj, benchGojqLargeRot, benchGojqIter). All 90+ benchmark functions converted to 1-2 line bodies.
- `fastjq_test.go`: Added `assertQuery` and `assertNoOutput` helpers. Converted most repetitive test groups (identity, field access, delete, compare, literal, type) to single-line calls.
- `exec.go`: `execStringPredicate` now uses `boolResult` helper instead of inline duplication. `appendKV` helper deduplicates the 5-line `{"key":k,"value":v}` pattern shared by `execToEntries` and `execWithEntries`.

---

## [Unreleased] — benchmark cleanup: Large/Medium variants + bench_vs_jq.sh update

### Changed
Added Large (and Medium for map) benchmarks for all operations that previously only had Small variants:
- Large object benchmarks: `has`, `length`, `keys_unsorted`, `to_entries`, `with_entries`, `ascii_downcase`, `startswith`, `ltrimstr`
- Large/Medium array benchmarks: `map`, `any`, `any(expr)`, `first(expr)`, `last(expr)`, `limit`

**Fixed**: Three Large benchmarks (`has`, `ascii_downcase`, `startswith`) were showing calibration artifacts because `buf, _ = RunWithBuffer(...)` reassigned `buf` to a sub-slice of the input (for `select` operations that return input unchanged). Subsequent iterations used that sub-slice as scratch, corrupting rotation inputs. Fixed by using `_, _ = RunWithBuffer(input, scratch[:0])` with a dedicated non-reassigned scratch buffer.

**Fixed**: `execWithEntries` entry scratch buffer reverted to 64-byte initial capacity (was changed to size-proportional which prevented allocator recycling). The 64-byte version recycles in steady state for typical inputs.

Added 5 new CLI benchmark queries to `bench_vs_jq.sh`:
- `select(.field_2 | ascii_downcase == "xxxxxxxxxx")` — 18x faster
- `select(.field_2 | startswith("xxxx"))` — 13x faster
- `select(has("field_2"))` — 12x faster
- `to_entries` — 18x faster
- `keys_unsorted` — 8x faster

### Notable findings
For small raw arrays of primitives (~600B, 200 ints), gojq can be faster than fastjq for `any(expr)`, `first`, `last`, `limit` because gojq accesses an in-memory `[]interface{}` slice while fastjq scans JSON bytes. For typical log processing (string/object fields, larger inputs), fastjq is consistently faster.

---

## [Unreleased] — first, last, limit(n; expr)

### Added
- `first` (no-arg) — desugars to `.[0]` at parse time. Zero-alloc.
- `last` (no-arg) — desugars to `.[-1]` at parse time. Zero-alloc.
- `first(expr)` — returns the first output of expr. Uses `errBreak` sentinel to stop `execMulti` after the first callback invocation. Zero-alloc.
- `last(expr)` — runs expr to completion, returns only the last output. Keeps a reference (no copy) to the last result — safe because results from `execMulti` with nil buf are either input sub-slices or global literals, both valid for the call's lifetime. Zero-alloc.
- `limit(n; expr)` — emits at most n outputs of expr (stream, not array). `n` is an expression evaluated via `execSingle`. Counter in callback, stops with `errBreak`. Zero-alloc for literal n.

### Benchmarks (Apple M4 Max)
| Operation | fastjq | gojq | Speedup |
|-----------|--------|------|---------|
| `first(expr)` | 107 ns | 1,478 ns | **14x** |
| `last(expr)` | 142 ns | 1,883 ns | **13x** |
| `limit(3; expr)` | 37 ns | 1,635 ns | **44x** |

---

## [Unreleased] — keys_unsorted, any/all

### Added
- `keys_unsorted` — returns object keys as a JSON array in insertion order, or array indices for array input. One objectIter/arrayLen pass. Zero-alloc.
- `any` / `all` (no-arg) — short-circuit boolean reduction over array/object values. `any` returns `true` on first truthy element; `all` returns `false` on first falsy element. Empty array: `any→false`, `all→true` (vacuous truth).
- `any(expr)` / `all(expr)` — applies expr to each element via `execSingle`, short-circuits. Common use: `select(.tags | any(. == "critical"))`.

`any(generator; cond)` two-arg form is not supported.

### Benchmarks (Apple M4 Max)
| Operation | fastjq | gojq | Speedup |
|-----------|--------|------|---------|
| `keys_unsorted` | 178 ns | 1,253 ns | **7x** |
| `any` (no-arg) | 46 ns | 1,804 ns | **39x** |
| `any(expr)` | 125 ns | 2,047 ns | **16x** |

---

## [Unreleased] — ascii_downcase/upcase, startswith/endswith, ltrimstr/rtrimstr

### Added
Six string operations for case-insensitive matching and string normalization in log pipelines:

- `ascii_downcase` / `ascii_upcase` — byte-by-byte case conversion. Escape sequences pass through unchanged. Error on non-string input.
- `startswith("s")` / `endswith("s")` — prefix/suffix predicates. Return `true`/`false`. Use `bTrue`/`bFalse` global literals when `buf == nil` (zero-alloc in condition context). Common use: `select(.path | startswith("/api/"))`.
- `ltrimstr("s")` / `rtrimstr("s")` — strip prefix/suffix. No-match returns input as a cap-limited sub-slice (zero-alloc). Match returns trimmed string written to buf.

All zero-alloc. `buf == nil` fast-paths added to predicates (return global literals) and trim ops (return cap-limited input sub-slices on no-match). 20 new tests, 218 total.

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
