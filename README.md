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
| `select(.f == "x")` | Small (~100B) | 0.104 µs | 1.66 µs | **16x** | 0 |
| `select(.f == "x")`¹ | Large (~100KB) | 33 µs | 769 µs | **24x** | 0 |
| `.field` | Small (~100B) | 0.083 µs | 0.94 µs | **11x** | 0 |
| `.field` | Large (~100KB) | 7.7 µs | 573 µs | **75x** | 0 |
| `del(.f)` | Large (~100KB) | 33 µs | 790 µs | **24x** | 0 |
| `select(.f \| ascii_downcase == "x")` | Large (~100KB) | 33 µs | 774 µs | **24x** | 0 |
| `map(.name)` | 200-elem array (~6KB) | 13 µs | 91 µs | **7x** | 0 |
| `min_by(.value)` | 100-elem array (~3KB) | 8.2 µs | 54 µs | **6.6x** | 0 |
| `.a * .b` (multiply) | Small (~100B) | 0.064 µs | 0.70 µs | **11x** | 0 |
| `first(.[] \| select(. > 100))` | 200-int array | 3.5 µs | 1.4 µs | **0.4x**² | 0 |

¹ Large select uses the last field in a 200-field object — fastjq scans the full document, no early-exit advantage.
² gojq wins on small arrays of raw integers: after unmarshal, element access is native Go slice operations.

The speedup is largest on small inputs where gojq's marshal/unmarshal overhead dominates. On large inputs fastjq is 18–75x faster thanks to SIMD-accelerated string scanning. The exception is small primitive integer arrays, where gojq's in-memory representation wins.

### vs jq CLI (JSONL throughput, 100K lines, ~11MB, Apple M4 Max, jq 1.8.1)

Both tools validate JSON. fastjq calls `json.Valid()` before processing each record.

| Operation | Input | jq (s) | fastjq (s) | Speedup |
|-----------|-------|--------|------------|---------|
| `.` (identity) | small | 0.356 | 0.052 | **6.8x** |
| `.field` | small | 0.152 | 0.051 | **3x** |
| `.field` | large (~16MB, 100 lines) | 0.091 | 0.043 | **2.1x** |
| `del(.field)` | small | 0.383 | 0.061 | **6.3x** |
| `{field_0, field_2}` (construct) | small | 0.252 | 0.060 | **4.2x** |
| `select(.f == "x")` (all match) | small | 0.408 | 0.049 | **8.3x** |
| `select(.f == "x")` (none match) | small | 0.143 | 0.050 | **2.9x** |
| `.field // "default"` | small | 0.164 | 0.050 | **3.3x** |
| `select(.f \| ascii_downcase == "x")` | small | 0.655 | 0.065 | **10x** |
| `select(.f \| startswith("x"))` | small | 0.377 | 0.059 | **6.4x** |
| `select(has("field"))` | small | 0.359 | 0.051 | **7x** |
| `to_entries` | small | 0.746 | 0.066 | **11x** |
| `keys_unsorted` | small | 0.244 | 0.057 | **4.3x** |

**Without validation** (using `RunWithBuffer`/`RunFunc` directly on known-valid inputs), the Go library benchmarks show **13–75x** speedups — see the table at the top of this section and [BENCHMARKS.md](docs/BENCHMARKS.md) for the full comparison.

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

- **Access:** `.`, `.foo`, `.[0]`, `.[i,j]`, `.[]`, `.[n:m]`, `.foo?`, `paths`, `path(expr)`, chained access
- **Modification:** `del(.foo)`, `del(.[i,j])`, `del(.[n:m])`, `setpath(path; value)`, `delpaths(paths)`, `{name, a: .b}`, `[.a, .b]`
- **Control flow:** `\|`, `select`, `if-elif-else`, `try-catch`, `//`, `empty`, `expr as $x | body`, `reduce`, `foreach`, `"\(expr)"`
- **Arithmetic:** `+`, `-`, `*`, `/`, `%`, `add`, `floor`, `ceil`, `round`, `nearbyint`
- **Math:** `abs`, `sqrt`, `log`, `exp`, `sin`, `cos`, `atan`, `tgamma`, `j0`, `pow(x;y)`, and 15 more — all zero-alloc
- **Special values:** `nan`, `infinite`, `-nan`, `-infinite`; `isnan`, `isinfinite`, `isfinite`, `isnormal`; `nan`/`infinite` output as `null` (JSON-safe)
- **Arrays:** `map`, `flatten`, `sort`, `sort_by(f)`, `unique`, `unique_by(f)`, `group_by(f)`, `transpose`, `min/max`, `min_by/max_by`, `any/all`, `first/last`, `limit`, `skip`, `nth`, `isempty`, `values`, type filters, `index/indices`, `paths`, `path(expr)`, `getpath`, `setpath`, `delpaths`
- **Strings:** `split/join`, `ascii_downcase/upcase`, `startswith/endswith`, `trim/ltrim/rtrim`, `ltrimstr/rtrimstr`, `"\(expr)"` interpolation, `explode`, `implode`
- **Format strings:** `@base64/d`, `@uri/d`, `@html`, `@csv`, `@tsv`, `@sh`, `@text`, `@json`
- **Regex (Go RE2):** `test(re)` *(0 allocs)*, `match(re)`, `capture(re)`, `scan(re)`, `sub(re; s)`, `gsub(re; s)`
- **Objects:** `to_entries`, `from_entries`, `keys`, `keys_unsorted`, `paths`, `path(expr)`, `getpath`, `setpath`, `delpaths`, `length`, `has`, `in`, `contains`, `inside`
- **Type:** `type`, `tojson/fromjson`, `tostring/tonumber/toboolean`, `debug`, `error`

## Official jq test suite coverage

fastjq is validated against two official jq test files (`go test ./jqtest/`).

| File | Total | Skipped | Attempted | Passed | Failed |
|------|-------|---------|-----------|--------|--------|
| [`tests/jq.test`](https://github.com/jqlang/jq/blob/master/tests/jq.test) (regression suite) | 521 | 185 | 336 | **336 (100.0%)** | 0 |
| [`tests/man.test`](https://github.com/jqlang/jq/blob/master/tests/man.test) (manual examples) | 230 | 60 | 170 | **170 (100.0%)** | 0 |
| **Combined** | **751** | **245** | **506** | **506 (100.0%)** | **0** |

All currently attempted official jq tests pass on this branch, and the branch now clears a wider majority of the full official suite. Recent parity work also removed compile skips around unary minus, dynamic slice bounds, `paths`, `path(...)`, `getpath(...)`, `setpath(...)`, `delpaths(...)`, `reduce`, `foreach`, jq-style multi-index array access/deletion (`.[4,2]`, `del(.[1,2])`), and identity-before-operator forms like `.-.`. The remaining skipped tests are concentrated in still-unimplemented families such as recursive descent (`..`), user-defined functions (`def`), assignment/update syntax, date functions, `env`, and other items listed in the [Limitations](#limitations) section.

## Limitations

**`select` conditions must be single-valued.**
`select(.items[] == "x")` silently tests only the first element. Use `any(.[]; . == "x")` instead.

**`.field` on non-object types errors — use `.field?` for null-safe access.**
`null | .field` returns `null` (matching jq). Other non-object types (`1 | .field`, `"s" | .field`) return an error. Use `.a?.b?` to suppress errors on any type.

**`map(f)` allocates when `f` constructs new data** (Tier 1 — proportional to output).
`map(.name)` is 0 allocs (field access returns an input sub-slice). `map({name, price})` allocates ~1 buffer per element to prevent result aliasing. Still 5–8x fewer allocations than gojq.

**String output is compact, canonical JSON.**
Escaped control characters are emitted in jq-compatible compact JSON form when a string value is produced as output.

**Regex uses Go RE2, not PCRE/Oniguruma.**
Named captures require `(?P<name>...)` syntax. Backreferences and lookahead are unsupported. `test(re)` is 0-alloc; `match`/`capture` alloc one `[]int` on a hit; `scan`/`gsub` alloc per match. Replacement strings in `sub`/`gsub` are literals.

**`@format "template"` combined syntax not supported.**
`@html "<b>\(.)</b>"` is not yet implemented. Plain `"\(.field)"` and standalone `@html`, `@csv`, `@sh`, etc. all work.

**No recursive descent** (`..|..` / `recurse`) — allocations scale with input depth, not output. Permanently rejected.

**`nan`/`infinite` are supported but serialize to `null` at output.** `nan | type` = `"number"`, `nan | isnan` = `true`, `infinite * -1 < 0` = `true`. `nan` and `infinite` values convert to JSON `null` at the API boundary. Values inside arrays/objects are also normalized to `null`.

**Not yet implemented:** `leaf_paths`, `label-break`, user-defined functions (`def`), `@format "template"`, assignment/update syntax, `hypot(x;y)`, `fma(x;y;z)`.

**Output is always compact JSON.** fastjq never panics — malformed input may produce wrong results but the process is always safe.

See [SYNTAX.md](docs/SYNTAX.md) for the full allocation-tiered roadmap.

## Further reading

- [SYNTAX.md](docs/SYNTAX.md) — Full operation reference with examples, and a roadmap of unimplemented operations categorised by feasibility
- [BENCHMARKS.md](docs/BENCHMARKS.md) — Complete benchmark tables, raw output, and CLI throughput results
- [DESIGN.md](docs/DESIGN.md) — Architecture details and key design decisions
- [CONSTRAINTS.md](docs/CONSTRAINTS.md) — Performance and scope constraints
- [CHANGELOG.md](CHANGELOG.md) — Change history with tradeoffs and benchmark notes
