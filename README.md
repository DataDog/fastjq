# fastjq

A fast jq engine for Go with a principled allocation model, designed for high-throughput structured log processing.

fastjq operates directly on raw `[]byte` — no `json.Unmarshal`, no `map[string]interface{}`, no marshal/unmarshal cycle. It compiles a query once and executes it against JSON bytes by scanning byte positions, copying only what it needs into an output buffer.

**This is not a full jq implementation.** It supports a targeted subset of jq operations chosen for log processing workloads. See [Limitations](#limitations) before using.

**Requires valid JSON input.** fastjq does not validate its input — behavior on malformed JSON is undefined. It never panics (enforced by fuzz tests), but may silently produce wrong results. Use `json.Valid` if you can't guarantee the source.

## Design

The standard approach costs ~800 µs and ~4,600 allocations just to parse a 100KB log object — before you've done any work. fastjq eliminates it:

- **No parse tree.** Input `[]byte` is the only representation. A scanner moves through bytes tracking a position integer, skipping values by depth-counting brackets. Nothing is ever converted to Go types.
- **Compile once, run many times.** `Compile` builds an immutable AST (allocates once). `Run`/`RunWithBuffer`/`RunFunc` walk the input guided by that AST — zero allocations at runtime for core operations with a reused buffer.
- **Copy only what you need.** Field access returns a sub-slice of input — no copy. Deletion reconstructs the object into an output buffer with its own commas. Everything else is left untouched.
- **Caller-owned buffer.** Output is written into a `[]byte` the caller passes in. It grows if needed, then stabilises. At steady state, zero heap allocations per record for typical log-processing queries.
- **Early exit.** `select(.level == "error")` on a 100KB object stops scanning the moment it finds `level` — the rest of the bytes are never read.

## Benchmarks

Compared against [gojq](https://github.com/itchyny/gojq), the standard jq library for Go. gojq times include the full `json.Unmarshal → execute → json.Marshal` cycle. Apple M4 Max, Go 1.25.

**Allocation model — allocations are proportional to what you ask for, never to what the engine scans:**

| Tier | Operations | Allocs/op |
|------|-----------|-----------|
| **Core** | access, filter, compare, arithmetic, construction, math, `test(re)`, and most others | **0** |
| **Output-encoding** | `@base64`, `@uri` (string decoding), `match`/`capture` (result struct), `scan`/`gsub` (multi-match) | ∝ output size |
| **Construction** | `map(f)` when `f` builds new data (object/arithmetic), `[.[] \| f]` | ~1 per element |

The core tier covers the full hot path for log processing. For operations that do allocate, the allocation is bounded by the result they produce — not by the document size they scan. See `allocs` column in [BENCHMARKS.md](docs/BENCHMARKS.md) for per-operation detail.

| Operation | Input | fastjq | gojq | Speedup | allocs |
|-----------|-------|--------|------|---------|--------|
| `select(.f == "x")` | Small (~100B) | 0.130 µs | 1.73 µs | **13x** | 0 |
| `select(.f == "x")`¹ | Large (~100KB) | 32.9 µs | 786 µs | **24x** | 0 |
| `.field` | Small (~100B) | 0.085 µs | 0.986 µs | **12x** | 0 |
| `.field` | Large (~100KB) | 7.50 µs | 587 µs | **78x** | 0 |
| `del(.f)` | Large (~100KB) | 33.7 µs | 817 µs | **24x** | 0 |
| `select(.f \| ascii_downcase == "x")` | Large (~100KB) | 33.6 µs | 799 µs | **24x** | 0 |
| `map(.name)` | 200-elem array (~6KB) | 12.9 µs | 93.7 µs | **7.3x** | 6 |
| `min_by(.value)` | 100-elem array (~3KB) | 8.2 µs | 54 µs | **6.6x** | 0 |
| `.a * .b` (multiply) | Small (~100B) | 0.064 µs | 0.70 µs | **11x** | 0 |
| `first(.[] \| select(. > 100))` | 200-int array | 3.5 µs | 1.4 µs | **0.4x**² | 0 |

¹ Large select uses the last field in a 200-field object — fastjq scans the full document, no early-exit advantage.
² gojq wins on small arrays of raw integers: after unmarshal, element access is native Go slice operations.

The speedup is largest on small inputs where gojq's marshal/unmarshal overhead dominates. On large inputs fastjq is roughly 19–78x faster thanks to SIMD-accelerated string scanning. The exception is small primitive integer arrays, where gojq's in-memory representation wins.

### vs jq CLI (JSONL throughput, 100K lines, ~11MB, Apple M4 Max, jq 1.8.1)

Both tools validate JSON. fastjq calls `json.Valid()` before processing each record.

| Operation | Input | jq (s) | fastjq (s) | Speedup |
|-----------|-------|--------|------------|---------|
| `.` (identity) | small | 0.368 | 0.059 | **6.2x** |
| `.field` | small | 0.165 | 0.049 | **3.4x** |
| `.field` | large (~16MB, 100 lines) | 0.092 | 0.044 | **2.1x** |
| `del(.field)` | small | 0.381 | 0.065 | **5.9x** |
| `{field_0, field_2}` (construct) | small | 0.263 | 0.060 | **4.4x** |
| `select(.f == "x")` (all match) | small | 0.388 | 0.057 | **6.8x** |
| `select(.f == "x")` (none match) | small | 0.152 | 0.059 | **2.6x** |
| `.field // "default"` | small | 0.177 | 0.052 | **3.4x** |
| `select(.f \| ascii_downcase == "x")` | small | 0.693 | 0.061 | **11.4x** |
| `select(.f \| startswith("x"))` | small | 0.383 | 0.053 | **7.2x** |
| `select(has("field"))` | small | 0.380 | 0.052 | **7.3x** |
| `to_entries` | small | 0.739 | 0.069 | **10.7x** |
| `keys_unsorted` | small | 0.259 | 0.060 | **4.3x** |

**Without validation** (using `RunWithBuffer`/`RunFunc` directly on known-valid inputs), the Go library benchmarks show roughly **12–78x** speedups on the common access/filter/delete slice — see the table at the top of this section and [BENCHMARKS.md](docs/BENCHMARKS.md) for the full comparison.

## Try it out (CLI)

A minimal JSONL processor CLI is included for benchmarking and quick experimentation:

```bash
go build -o fastjq ./cmd/fastjq

cat app.log | ./fastjq 'select(.level == "error")'
cat app.log | ./fastjq 'select(.level | ascii_downcase == "error")'
cat app.log | ./fastjq 'del(.password, .token)'
```

## Install

```bash
go get github.com/brianfloersch/fastjq
```

## Usage

```go
// Compile once — safe for concurrent use
p, err := fastjq.Compile(`select(.level == "error")`)

// Stream results with zero steady-state allocations
err = p.RunFunc(line, func(result []byte) error {
    _, err := w.Write(result)
    return err
})

// Or reuse a buffer across calls
buf := make([]byte, 0, 1024)
buf, err = p.RunWithBuffer(line, buf)

// Collect all outputs (allocates)
results, err := p.RunAll(line)
```

### API

```go
func Compile(query string) (*Program, error)

func (p *Program) Run(input []byte) ([]byte, error)
func (p *Program) RunWithBuffer(input []byte, buf []byte) ([]byte, error)
func (p *Program) RunAll(input []byte) ([][]byte, error)
func (p *Program) RunFunc(input []byte, fn func(result []byte) error) error
```

`Compile` allocates. Core operations via `RunWithBuffer`/`RunFunc` achieve zero allocations at steady state. Operations that produce new structured output (regex matches, base64/URI encoding, `map(f)` with construction) allocate proportional to result size.

## Supported Operations

fastjq supports a large targeted subset of jq. The complete reference with examples is in [SYNTAX.md](docs/SYNTAX.md).

**Quick summary by category:**

- **Access:** `.`, `.foo`, `.[0]`, `.[i,j]`, `.[]`, `..`, `recurse`, `.[n:m]`, `.foo?`, `paths`, `path(expr)`, chained access
- **Modification:** `del(.foo)`, `del(.[i,j])`, `del(.[n:m])`, `setpath(path; value)`, `delpaths(paths)`, `{name, a: .b}`, `[.a, .b]`
- **Control flow:** `\|`, `select`, `if-elif-else`, `try-catch`, `//`, `empty`, `expr as $x | body`, destructuring `as` binds, `?//` binding alternation, `def f: body; expr`, parameterized defs, `reduce`, `foreach`, `while`, `until`, `recurse`, `walk(f)`, `label` / `break`, `"\(expr)"`
- **Arithmetic:** `+`, `-`, `*`, `/`, `%`, `add`, `floor`, `ceil`, `round`, `nearbyint`
- **Math:** `abs`, `sqrt`, `log`, `exp`, `sin`, `cos`, `atan`, `tgamma`, `j0`, `pow(x;y)`, and 15 more — all zero-alloc
- **Special values:** `nan`, `infinite`, `-nan`, `-infinite`; `isnan`, `isinfinite`, `isfinite`, `isnormal`; `nan`/`infinite` output as `null` (JSON-safe)
- **Arrays:** `map`, `map_values`, `flatten`, `sort`, `sort_by(f)`, `unique`, `unique_by(f)`, `group_by(f)`, `transpose`, `reverse`, `combinations`, `min/max`, `min_by/max_by`, `any/all`, `first/last`, `limit`, `skip`, `repeat`, `range`, `nth`, `isempty`, `values`, `pick`, `bsearch`, `IN`, `INDEX`, `JOIN`, type filters, `index/indices`, `paths`, `path(expr)`, `getpath`, `setpath`, `delpaths`, `tostream`, `truncate_stream`, `fromstream`
- **Strings:** `split/join`, `ascii_downcase/upcase`, `startswith/endswith`, `trim/ltrim/rtrim`, `trimstr/ltrimstr/rtrimstr`, `utf8bytelength`, `"\(expr)"` interpolation, `explode`, `implode`
- **Format strings:** `@base64/d`, `@uri/d`, `@html`, `@csv`, `@tsv`, `@sh`, `@text`, `@json`, and `@format "...\(...)"` template forms
- **Regex (Go RE2):** `test(re)` *(0 allocs)*, `match(re)`, `capture(re)`, `scan(re)`, `sub(re; s)`, `gsub(re; s)`
- **Objects:** `to_entries`, `from_entries`, `with_entries`, `keys`, `keys_unsorted`, `paths`, `path(expr)`, `getpath`, `setpath`, `delpaths`, `length`, `has`, `in`, `contains`, `inside`, `map_values`
- **Date/time:** `strftime`, `strflocaltime`, `strptime`, `mktime`, `gmtime`, `fromdate`
- **Type:** `type`, `tojson/fromjson`, `tostring/tonumber/toboolean`, `have_decnum`, `debug`, `error`, `builtins`, `$__loc__`

## Official jq test suite coverage

fastjq is validated against five upstream jq library-focused test files (`go test ./jqtest/`).

| File | Total | Skipped | Attempted | Passed | Failed |
|------|-------|---------|-----------|--------|--------|
| [`tests/jq.test`](https://github.com/jqlang/jq/blob/master/tests/jq.test) (regression suite) | 521 | 25 | 496 | **496 (100.0%)** | 0 |
| [`tests/man.test`](https://github.com/jqlang/jq/blob/master/tests/man.test) (manual examples) | 230 | 7 | 223 | **223 (100.0%)** | 0 |
| [`tests/optional.test`](https://github.com/jqlang/jq/blob/master/tests/optional.test) | 2 | 0 | 2 | **2 (100.0%)** | 0 |
| [`tests/base64.test`](https://github.com/jqlang/jq/blob/master/tests/base64.test) | 10 | 0 | 10 | **10 (100.0%)** | 0 |
| [`tests/uri.test`](https://github.com/jqlang/jq/blob/master/tests/uri.test) | 20 | 0 | 20 | **20 (100.0%)** | 0 |
| **Combined** | **783** | **32** | **751** | **751 (100.0%)** | **0** |

All currently attempted official jq tests pass on this branch across the expanded five-file harness. Recent parity work removed compile skips around destructuring `as` binds, `?//` binding alternation, user-defined functions (`def`), `@format "template"` strings, `map_values`, `with_entries`, `label` / `break`, unary minus, postfix optional `expr?`, quoted field access (`."foo"`), `while` / `until`, `recurse`, `walk(f)`, dynamic slice bounds, grouped/dynamic `del(...)` paths, `paths`, `path(...)`, `getpath(...)`, `setpath(...)`, `delpaths(...)`, `reduce`, `foreach`, dedicated recursive descent (`..`), richer object construction, date/time builtins (`strftime`, `strflocaltime`, `strptime`, `mktime`, `gmtime`, `fromdate`), stricter `@base64d` / `@urid` decoding parity, `pick`, `bsearch`, `IN`, `INDEX`, `JOIN`, `combinations`, jq-style multi-index array access/deletion (`.[4,2]`, `del(.[1,2])`), `//=` updates, grouped assignment, root slice assignment, `repeat`, stream reconstruction helpers (`tostream`, `truncate_stream`, `fromstream`), `builtins`, `$__loc__`, and identity-before-operator forms like `.-.`. The remaining skipped tests are now concentrated in full decimal semantics (`have_decnum`), module/import helpers, and host-boundary helpers we are still intentionally deferring.

## Limitations

**Regex uses Go RE2, not PCRE/Oniguruma.**
Named captures require `(?P<name>...)` syntax. Backreferences and lookahead are unsupported. `test(re)` is 0-alloc; `match`/`capture` alloc one `[]int` on a hit; `scan`/`gsub` alloc per match. Replacement strings in `sub`/`gsub` are literals.

**`nan`/`infinite` are supported but serialize to `null` at output.** `nan | type` = `"number"`, `nan | isnan` = `true`, `infinite * -1 < 0` = `true`. `nan` and `infinite` values convert to JSON `null` at the API boundary. Values inside arrays/objects are also normalized to `null`.

**Still intentionally deferred:** module/import syntax and `modulemeta` (they need filesystem-backed resolution and module-loading semantics that break the current pure library boundary); host-boundary helpers such as `input`, `inputs`, `env`, `$ENV`, and `stderr` (they depend on process state or stdin rather than pure input bytes); full decimal-mode semantics behind `have_decnum` (they need a different numeric runtime than the current float-based executor); and a small builtin tail that has little impact on library-heavy jq coverage such as `leaf_paths`, `hypot(x;y)`, `fma(x;y;z)`, `date`, `now`, and `todate`.

**Known behavior gap:** `select(cond)` still evaluates `cond` via the single-result fast path, so generator conditions use only their first output. The official jq files we run do not depend on multi-output `select` conditions, but this is still not full jq semantics.

See [SYNTAX.md](docs/SYNTAX.md) for the full allocation-tiered roadmap.

## Further reading

- [SYNTAX.md](docs/SYNTAX.md) — Full operation reference with examples, and a roadmap of unimplemented operations categorised by feasibility
- [BENCHMARKS.md](docs/BENCHMARKS.md) — Complete benchmark tables, raw output, and CLI throughput results
- [DESIGN.md](docs/DESIGN.md) — Architecture details and key design decisions
- [CONSTRAINTS.md](docs/CONSTRAINTS.md) — Performance and scope constraints
- [CHANGELOG.md](CHANGELOG.md) — Change history with tradeoffs and benchmark notes
