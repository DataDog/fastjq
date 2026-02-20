# Changelog

Entries are in reverse chronological order. Each entry notes new operations, tradeoffs, and any significant benchmark movements.

---

## [Unreleased] — regex: test, match, capture, scan, sub, gsub (Go RE2)

### Added

Six regex operations using Go's RE2 engine (linear-time, immune to ReDoS).
All patterns are compiled once at `Compile()` time — `Run()` never allocates for the engine.

- **`test(re)`** / **`test(re; flags)`** — 0 allocs: `regexp.Match([]byte)` requires no heap for compiled patterns.
- **`match(re)`** / **`match(re; flags)`** — returns full match object with `offset`, `length`, `string`, `captures`; 1 alloc on match (`[]int` from `FindSubmatchIndex`; unavoidable given Go's API). 0 allocs on miss.
- **`capture(re)`** / **`capture(re; flags)`** — named captures as a flat JSON object; same alloc profile as `match`.
- **`scan(re)`** / **`scan(re; flags)`** — streams all non-overlapping matches; no groups → JSON strings, with groups → JSON arrays. Allocates per match.
- **`sub(re; "rep")`** — replaces the first match with a literal string. 1 alloc on match (`FindIndex []int`).
- **`gsub(re; "rep")`** — replaces all matches with a literal string. Allocates per match.

Flags supported: `i` (case-insensitive), `m` (multiline `^`/`$`), `s` (`.` matches `\n`). Named capture groups use `(?P<name>...)` RE2 syntax.

### Tradeoffs

- Go RE2 ≠ PCRE/Oniguruma: no backreferences, no lookahead/behind. The official `jq` uses Oniguruma; test patterns using PCRE features will fail to compile with a clear error at `Compile()` time.
- Replacement strings in `sub`/`gsub` are literals. Capture group references (`\(.name)`) in replacements are not supported.
- The jq official test suite (`jq.test`, `man.test`) does not have standalone regex tests for the Go port — those live in `onig.test` which uses PCRE syntax. Coverage is provided by `regex_alloc_test.go` and `compat_test.go`.

---

## [Unreleased] — string interpolation, isempty, nth (295 → 309 passing)

### Added

- **String interpolation `"\(expr)"`** — zero-alloc: literal segments stored at compile time, each expression evaluated via `execSingle` and its string content written directly into `buf`. Multi-output expressions in interpolation produce only the first result.
- **`isempty(expr)`** — true if expr produces no outputs, false if it produces any. Zero-alloc: uses early-exit via `errBreak`.
- **`nth(n; gen)`** — nth output of a generator (0-indexed). No output if the generator has fewer than n+1 results. Zero-alloc: counter + `errBreak`.
- **`error(expr)` 1-arg form** — `error("msg")` throws `expr` as the error value (the 0-arg form `error` already existed; this adds the 1-arg variant).

### Fixed

- **`try` / `//` precedence**: `try error(0) // 1` now correctly parses as `(try error(0)) // 1 = 1` rather than `try (error(0) // 1) = empty`. The `parseTry` body now uses `parseOr` (stopping before `//`) instead of `parseExpr` (`parseAlt`).

### Not supported (rejected / documented)

- **`@format "template"`** combined syntax (e.g. `@html "<b>\(.)</b>"`, `@sh "echo \(.)"`) applies the format to each interpolated value separately — requires parser and executor changes beyond basic interpolation. Compile-errors gracefully (auto-skip).
- **Dynamic string keys in objects** (`{"key\(expr)": val}`) requires runtime evaluation of the key name. Compile-errors gracefully.

### Known architectural limitations (surfaced by expanded coverage)

Two pre-existing limitations are now visible because `\(` is no longer in the skip list:
1. **try-catch scope**: `(try body catch h) | right` — fastjq's try catches errors from the entire callback chain including `right`, not just from `body`. In jq, the catch only covers the body. Tests jq.test:2320 and jq.test:2325 expose this.
2. **Error message format**: fastjq uses static error sentinels for zero-alloc behaviour; jq produces verbose messages like `"Cannot index number with string \"a\""`. Test jq.test:1431 exposes this.

---

## [Unreleased] — 21 math builtins (jq test suite: 291 → 295 passing)

### Added

21 zero-alloc 1-arg floating-point math builtins. All take the input number, apply
a Go `math` function, and write the result directly into the output buffer via
`strconv.AppendFloat`/`AppendInt` — no heap allocations at steady state.

- **Rounding**: `nearbyint` (round to nearest; uses round-half-away as approximation)
- **Powers/logs**: `sqrt`, `log` (natural), `log2`, `log10`, `exp` (e^x), `exp2` (2^x), `exp10` (10^x), `cbrt`, `logb`
- **Trig**: `sin`, `cos`, `tan`, `asin`, `acos`, `atan` (1-arg)
- **Special**: `fabs`, `tgamma` (Γ(x)), `lgamma` (ln|Γ(x)|), `j0`, `j1` (Bessel functions)

NaN/Infinity results are output as `null` to preserve valid JSON output.

### Rejected — documented in SYNTAX.md

- **`nan` / `infinite` constants**: produce non-JSON output; violates "output is always compact JSON"
- **`isnan`, `isinfinite`, `isfinite`, `isnormal`**: depend on nan/infinite representation; meaningless without it
- **`pow(x;y)`, `hypot(x;y)`, `atan(y;x)`, `fma(x;y;z)`**: 2/3-arg forms; every test blocked by `as $` or `range(`(0 exclusive tests); deferred
- **`frexp`, `modf`**: return array pairs; 0 exclusive tests
- **`ldexp`, `scalb`, `scalbln`, `significand`**: complex; 0 exclusive tests

---

## [Unreleased] — man.test coverage + 3 bug fixes

Added `tests/man.test` (jq manual examples, 230 tests) to the official test harness.
Combined coverage: 751 tests across both files, 291/297 attempted passing (98.0%).

### Fixed

Three bugs surfaced by man.test:

- **`length` on negative numbers**: previously returned 0; now returns the absolute value (`-5 | length` → `5`), matching jq semantics.
- **Object `+` key order**: left-object key order is now preserved when merging with `+`. Previously, keys duplicated by the right operand were dropped from the left pass and appended at the end (`{a:1} + {a:2}` produced `{"a":2}` correctly but `{a:1,b:2} + {a:99}` incorrectly produced `{"b":2,"a":99}` instead of `{"a":99,"b":2}`).
- **`jsonEqual` object comparison**: objects now compare equal regardless of key order. Previously `{"a":1,"b":2} == {"b":2,"a":1}` returned `false`; now correctly returns `true`.

### Known differences (man.test failures)

Three man.test failures are structural differences from jq, not bugs:
1. Object construction emits one output even when a value expression produces multiple (e.g. `{key: .items[]}`).
2. `==` uses `execSingle` on operands, so `.[] == 1` returns one result, not one per element.
3. `[a, b | f]` parses as `[a, (b|f)]` in fastjq; jq parses it as `[(a,b)|f]`.

---

## [Unreleased] — new operations & iterator error propagation (98.2% → 98.4%)

### Added

Twelve new operations selected by scanning the official jq test suite for high-ROI, zero-alloc-compatible features:

- **`contains(val)`** — recursive containment: string substring, object key-value subset, array element subset. Zero-alloc for read-only traversal.
- **`inside(val)`** — reverse of `contains`: `a | inside(b)` ≡ `b | contains(a)`.
- **`floor`** / **`ceil`** / **`round`** — numeric rounding (zero-alloc, outputs integers when result is whole).
- **`error`** — throw the input as an error; caught by `try-catch` with the **actual JSON value** (not a string representation), matching jq semantics.
- **`@html`** — HTML-escape `&`, `<`, `>`, `'`, `"` in a string.
- **`@csv`** — format a JSON array as a CSV line (strings double-quoted, internal quotes doubled).
- **`@tsv`** — format a JSON array as a TSV line (tab/newline/backslash escaped per jq convention).
- **`@sh`** — POSIX shell-quote a string (single-quote wrapping with `'\''` for embedded quotes).
- **`@text`** — alias for `tostring`.
- **`@urid`** — percent-decode a URI-encoded string (non-ASCII codepoints output as `\uXXXX`).

Also fixed parser to allow comma-separated generator bodies in `limit(n; a, b)`.

### Fixed

- **`error` value propagation through `try-catch`**: `catch` handlers now receive the actual JSON value thrown by `error`, not a string representation. Introduced `jsonError` type (allocated only on the exceptional error-throw path).
- **Iterator error propagation**: `execIterator` now propagates `jsonError` and `errBreak` from callbacks instead of silently dropping them. Regular errors (e.g. field access on wrong type) continue to be dropped, preserving lenient multi-output behaviour.
- **Array construction error propagation**: `execArrayConstruct` now propagates `jsonError` values unwrapped so `try-catch` can intercept them with the correct value.
- **`limit(-1; ...)` validation**: negative count now throws `"limit doesn't support negative count"` (matches jq).

### Tradeoffs

`error` and the format strings allocate on their call paths (the same category as `@base64`/`@uri`). `contains()` uses recursive closures that may allocate per nesting level. Neither is on the steady-state hot path for typical log processing.

The iterator change (`errBreak` propagation) enables correct early-exit for `limit`/`first` inside iterators, which was previously untested. No existing tests were broken.

---

## [Unreleased] — jq official test suite bug fixes (91.7% → 98.2%)

### Fixed

Seven correctness bugs found by the official jq test suite (`go test ./jqtest/`):

- **BOM stripping** — UTF-8 BOM (`\xEF\xBB\xBF`) is now silently stripped from input before parsing, matching jq behaviour.
- **`indices()` overlapping matches** — `indices("aba")` on `"xababababax"` now returns `[1,3,5,7]` instead of `[1,5]`. Previously the search advanced by `len(needle)` after each match, skipping overlapping occurrences.
- **`index`/`rindex`/`indices` Unicode codepoint positions** — String search functions now report Unicode codepoint offsets rather than raw byte offsets. Multi-byte UTF-8 sequences and JSON escape sequences each count as one codepoint, matching jq's behaviour.
- **`indices([1,2])` array subsequence search** — When the search value is an array, `indices`/`index`/`rindex` now find all positions where that sequence occurs as a contiguous subsequence. Previously only single-element searches were supported.
- **`del()` with slice ranges** — `del(.[2:4])`, `del(.[-2:])`, and mixed index/slice arguments are now supported. Previously any slice argument in `del()` returned an error.
- **Array comparison in `min`/`max`** — `compareJSONOrder` now compares arrays element-by-element (zero-alloc parallel scan). Previously arrays always compared equal, so `min` on `[[4,...],[1,...]]` returned the first element instead of the minimum.
- **`max_by` tie-breaking** — When multiple elements share the minimum key value, `max_by` now returns the last such element (stable-sort semantics). `min_by` continues to return the first.
- **`@base64` / `@uri` JSON string decoding** — Both format functions now decode JSON escape sequences (e.g. `\n` → `0x0a`, `\uXXXX` → UTF-8 bytes) before encoding. Previously they operated on raw JSON bytes, producing wrong output for any string with escape sequences.

### Tradeoffs

The 3 remaining failures after these fixes are intentional: jq normalises string escape sequences on output (`\r` → `\u000d`, `\u0020` → literal space). fastjq passes string bytes through unchanged — a fundamental consequence of the zero-copy constraint. These are documented as known incompatibilities.

---

## [Unreleased] — try/catch, elif, object merge, tojson/fromjson, tostring/tonumber, any/all two-arg

### Added

Eight new zero-alloc features:

- **`try expr`** — suppresses errors from the body expression, producing no output on failure. `errBreak` (for `first`/`limit`) propagates unchanged.
- **`try expr catch handler`** — on failure, runs `handler` with the error message as a JSON string. The catch handler message is allocated once per actual error (exceptional path).
- **`elif`** — `if C then A elif C2 then B else D end` desugars to nested `if` at parse time; no new op type. Chains of any length are supported.
- **`+` (object merge)** — `{"a":1} + {"b":2}` = `{"a":1,"b":2}`. Right wins on duplicate keys. Zero-alloc: `objectContainsKey` uses a manual scan loop (no closure/callback), same pattern as `arrayContainsElem`.
- **`tojson` / `@json`** — wrap any JSON value as a JSON string, escaping `"` and `\`. Zero-alloc.
- **`fromjson`** — strip outer quotes from a JSON string and unescape `\"` and `\\`. Zero-alloc.
- **`tostring`** — strings pass through unchanged; non-strings go through `tojson`. Zero-alloc (string path returns cap-limited sub-slice).
- **`tonumber`** — numbers pass through; strings are parsed as floats via `parseJSONFloat`. Zero-alloc for the number identity path.
- **`any(gen; cond)` / `all(gen; cond)`** — two-arg forms: generator produces multiple outputs, condition is applied to each. Short-circuits. Zero-alloc.

### Benchmarks (Apple M4 Max)
| Operation | fastjq | gojq | Speedup |
|-----------|--------|------|---------|
| `try .field` (no error) | 149 ns | 635 ns | **4.3x** |
| object merge `.a + .b` | 157 ns | 710 ns | **4.5x** |
| `tojson` (~100B object) | 204 ns | 510 ns | **2.5x** |
| `fromjson` (JSON string) | 47 ns | 430 ns | **9x** |
| `tonumber` (`"42"` string) | 15 ns | 370 ns | **25x** |
| `any(.[]; . > 100)` (200 ints) | 2.9 µs | 11.3 µs | **3.9x** |

---

## [Unreleased] — arithmetic, min/max, @uri

### Added
- **`expr - expr`** — Number subtraction and array difference (`[1,2,3] - [2]` = `[1,3]`). Array diff is O(n²) but zero-alloc via manual scan loop (avoids closure capture of booleans that would escape to heap).
- **`expr * expr`** — Number multiplication and string repetition (`"ab" * 3` = `"ababab"`; `"x" * 0` = `null`).
- **`expr / expr`** — Number division and string split (`"a,b" / ","` = `["a","b"]`; division by zero → error).
- **`expr % expr`** — Number modulo (integer fast path; float via `math.Mod`).
- **`min` / `max`** — Minimum/maximum element of an array. Numbers: float comparison; strings: lexicographic; cross-type ordering: number < string < array < object < boolean < null. Empty array → `null`.
- **`min_by(f)` / `max_by(f)`** — Element with min/max value of key function `f`. One scan, zero-alloc.
- **`@uri`** — URL percent-encode a JSON string. RFC 3986 unreserved characters (`A-Z a-z 0-9 - _ . ~`) pass through; all others encoded as `%XX`. Operates on raw JSON string bytes (consistent with `@base64`).

### Precedence
Parser extended to two-level arithmetic precedence: `*`/`/`/`%` bind tighter than `+`/`-`. New chain:
```
parsePipeExpr → ... → parseCmp → parseAddExpr → parseMulExpr → parseAtom
```

### Fix: `appendInt` negative number bug
`appendInt` used `for n > 0` which silently produced wrong output for negative integers. Fixed to handle negatives. The bug was latent (all previous callers only passed non-negative values), but arithmetic operations now produce negative results.

### Fix: `parseJSONFloat` on non-numeric inputs
`execArith` was calling `parseJSONFloat` unconditionally, causing `strconv.ParseFloat` to allocate error objects when inputs were arrays or objects. Fixed by checking the first byte before calling `parseJSONFloat`. All arithmetic benchmarks are now 0 allocs.

### Benchmarks (Apple M4 Max)
| Operation | fastjq | gojq | Speedup |
|-----------|--------|------|---------|
| `.a - .b` (numbers) | 66 ns | 653 ns | **10x** |
| `.a * .b` (numbers) | 83 ns | 679 ns | **8.2x** |
| `.a / .b` (numbers) | 115 ns | 736 ns | **6.4x** |
| `min` (200 ints) | 2.98 µs | 11.7 µs | **3.9x** |
| `min_by(.v)` (100 objects) | 12.4 µs | 55.7 µs | **4.5x** |
| `@uri` (36-char URL) | 102 ns | 669 ns | **6.6x** |
| `.a - .b` (array diff) | 214 ns | 1,277 ns | **6.0x** |

---

## [Unreleased] — reject with_entries

### Rejected
- **`with_entries(f)`** — evaluated and removed. `with_entries` must build each `{"key":k,"value":v}` entry as a temporary JSON object before passing it to `f`. The entry bytes cannot share the output buffer (they may be read by `f` while results are being written), so a dedicated scratch buffer (`make([]byte, 0, 64)`) is unavoidable — 1 alloc/call that does not recycle in steady state at 100x iterations.

  The use cases are covered without the alloc: `to_entries | map(f) | from_entries` composes the same result using the caller-supplied buffer. For filtering null values: `to_entries | map(select(.value != null)) | from_entries`. For redacting keys: `to_entries | map(select(.key != "secret")) | from_entries`.

  Documented in SYNTAX.md as "Rejected" alongside `range` and `recurse`.

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
`from_entries` now accepts all jq-documented capitalized variants: `"Key"`, `"Name"`, `"Value"` in addition to the existing `"key"`, `"name"`, `"value"`. (`parseEntryKeyValue` is also used internally by `execFromEntries`.)

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
`compat_test.go`: 147 test cases verifying fastjq produces byte-identical output to the jq CLI (1.8.1) across all supported operations. Tests cover identity, field access, deletion, construction, pipes, comparisons, boolean logic, select, alternative, type, has, length, map, to_entries/from_entries, keys_unsorted, any/all, first/last/limit, if-then-else, string operations (downcase, upcase, startswith, endswith, ltrimstr, rtrimstr), flatten, add, split, join, unicode, and realistic log processing patterns.

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
- Large object benchmarks: `has`, `length`, `keys_unsorted`, `to_entries`, `ascii_downcase`, `startswith`, `ltrimstr`
- Large/Medium array benchmarks: `map`, `any`, `any(expr)`, `first(expr)`, `last(expr)`, `limit`

**Fixed**: Three Large benchmarks (`has`, `ascii_downcase`, `startswith`) were showing calibration artifacts because `buf, _ = RunWithBuffer(...)` reassigned `buf` to a sub-slice of the input (for `select` operations that return input unchanged). Subsequent iterations used that sub-slice as scratch, corrupting rotation inputs. Fixed by using `_, _ = RunWithBuffer(input, scratch[:0])` with a dedicated non-reassigned scratch buffer.

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

## [Unreleased] — zero-alloc fix for map

### Fixed
**`map(.name)` (was 20 allocs → 0 allocs)**
Root cause: `execFieldMulti` with a nil scratch buffer called `fn(append(nil, value...))`, allocating an intermediate slice per element. Fixed by returning a cap-limited sub-slice of input directly when `buf == nil` — no copy, no alloc.

### Also fixed
- `execField`, `execFieldMulti`, `execIndex`, `execIndexMulti`, `execIdentity`: when `buf == nil`, return cap-limited sub-slices of input directly (zero-alloc). Cap-limited (`[vs:ve:ve]`) prevents callers from using spare capacity as scratch and corrupting input bytes.
- `execCompareSingle`: when `buf == nil`, use nil rightBuf (avoids writing into input's backing array via leftVal's spare capacity) and return global `bTrue`/`bFalse` literals.
- `execSingle` for `opLiteral`, `opNot`, `opAnd`, `opOr`: return global literals when `buf == nil`.
- `parseEntryKeyValue`: extracted as a standalone non-closure function so scanner variables stay on the stack (was the source of scanner heap-allocation allocs in `from_entries`).

---

## [Unreleased] — benchmarks for new operations

### Added
Added fastjq vs gojq benchmarks for all operations added since the last benchmark update:
`SelectAnd`, `SelectOr`, `Has`, `IfThenElse`, `Length`, `Map`, `ToEntries`.
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

`map` shows 20 allocs (one per element) due to nil scratch in `execArrayConstruct` — see length/map CHANGELOG entry. All other new operations are 0-alloc.

---

## [Unreleased] — to_entries, from_entries

### Added
- `to_entries` — converts `{"a":1,"b":2}` to `[{"key":"a","value":1},{"key":"b","value":2}]`. Zero-alloc: writes directly into output buffer via objectIter. Non-object input returns `[]`.
- `from_entries` — converts `[{"key":"a","value":1}]` back to `{"a":1}`. Accepts both `"key"` and `"name"` as the key field name. Zero-alloc: reads entry fields via objectIter, writes output directly.

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
