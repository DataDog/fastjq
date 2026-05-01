# fastjq

[![License: Apache-2.0](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)

A fast jq engine for Go with a principled allocation model, designed for high-throughput structured log processing. Maintained by Datadog.

fastjq operates directly on raw `[]byte` — no `json.Unmarshal`, no `map[string]interface{}`, no marshal/unmarshal cycle. It compiles a query once and executes it against JSON bytes by scanning byte positions, copying only what it needs into an output buffer.

fastjq passes all attempted cases in the upstream five-file jq library harness. See [Limitations](#limitations) for the remaining exclusions.

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
The core `fastjq` module has zero external Go module dependencies. The `gojq` comparison benchmarks live in the separate `compare/` module so the library stays dependency-free.

**Allocation model — allocations are proportional to what you ask for, never to what the engine scans:**

| Tier | Operations | Allocs/op |
|------|-----------|-----------|
| **Core** | access, filter, compare, arithmetic, construction, math, `test(re)`, and most others | **0** |
| **Output-encoding** | `@base64`, `@uri` (string decoding), `match`/`capture` (result struct), `scan`/`gsub` (multi-match) | ∝ output size |
| **Construction** | `map(f)` when `f` builds new data (object/arithmetic), `[.[] \| f]` | ~1 per element |

The core tier covers the full hot path for log processing. For operations that do allocate, the allocation is bounded by the result they produce — not by the document size they scan. See `allocs` column in [BENCHMARKS.md](docs/BENCHMARKS.md) for per-operation detail.

| Operation | Input | fastjq | gojq | Speedup | allocs |
|-----------|-------|--------|------|---------|--------|
| `select(.f == "x")` | Small (~100B) | 0.132 µs | 1.75 µs | **13x** | 0 |
| `select(.f == "x")`¹ | Large (~100KB) | 32.4 µs | 800 µs | **25x** | 0 |
| `.field` | Small (~100B) | 0.088 µs | 1.00 µs | **11x** | 0 |
| `.field` | Large (~100KB) | 7.44 µs | 609 µs | **82x** | 0 |
| `del(.f)` | Large (~100KB) | 35.9 µs | 845 µs | **24x** | 0 |
| `select(.f \| ascii_downcase == "x")` | Large (~100KB) | 33.5 µs | 818 µs | **24x** | 0 |
| `map(.name)` | 200-elem array (~6KB) | 12.7 µs | 95.5 µs | **7.5x** | 6 |
| `min_by(.value)` | 100-elem array (~3KB) | 11.1 µs | 56.0 µs | **5x** | 297 |
| `.a * .b` (multiply) | Small (~100B) | 0.072 µs | 0.727 µs | **10x** | 0 |
| `first(.[] \| select(. > 100))` | 200-int array | 6.61 µs | 26.5 µs | **4x**² | 209 |

¹ Large select uses the last field in a 200-field object — fastjq scans the full document, no early-exit advantage.
² Iterator-heavy control-flow queries over primitive arrays allocate more than the core hot path, but they still stay ahead of gojq in the current benchmark set.

The speedup is largest on small inputs where gojq's marshal/unmarshal overhead dominates. On large inputs fastjq is roughly 19–82x faster thanks to SIMD-accelerated string scanning. Control-flow-heavy iterator queries allocate more, but the regenerated benchmark set still keeps a healthy lead over gojq.

### vs jq CLI (JSONL throughput, 100K lines, ~11MB, Apple M4 Max, jq 1.8.1)

Both tools validate JSON. fastjq calls `json.Valid()` before processing each record.

| Operation | Input | jq (s) | fastjq (s) | Speedup |
|-----------|-------|--------|------------|---------|
| `.` (identity) | small | 0.368 | 0.061 | **6.0x** |
| `.field` | small | 0.167 | 0.051 | **3.3x** |
| `.field` | large (~16MB, 100 lines) | 0.092 | 0.044 | **2.1x** |
| `del(.field)` | small | 0.380 | 0.068 | **5.6x** |
| `{field_0, field_2}` (construct) | small | 0.260 | 0.064 | **4.1x** |
| `select(.f == "x")` (all match) | small | 0.388 | 0.058 | **6.7x** |
| `select(.f == "x")` (none match) | small | 0.150 | 0.060 | **2.5x** |
| `.field // "default"` | small | 0.178 | 0.057 | **3.1x** |
| `select(.f \| ascii_downcase == "x")` | small | 0.702 | 0.062 | **11.3x** |
| `select(.f \| startswith("x"))` | small | 0.394 | 0.053 | **7.4x** |
| `select(has("field"))` | small | 0.380 | 0.052 | **7.3x** |
| `to_entries` | small | 0.761 | 0.068 | **11.2x** |
| `keys_unsorted` | small | 0.256 | 0.061 | **4.2x** |

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
go get github.com/DataDog/fastjq
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

fastjq supports most of jq's library-oriented surface. See [SYNTAX.md](docs/SYNTAX.md) for the complete operation list, examples, allocation notes, and the remaining unsupported features.

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

All 751 attempted cases pass. The remaining 32 skips are limited to full decimal semantics (`have_decnum`), module/import helpers, and host-boundary helpers like `input`, `env`, and `stderr`.

## Limitations

**Regex uses Go RE2, not PCRE/Oniguruma.**
Named captures require `(?P<name>...)` syntax. Backreferences and lookahead are unsupported. `test(re)` is 0-alloc; `match`/`capture` alloc one `[]int` on a hit; `scan`/`gsub` alloc per match. Replacement strings in `sub`/`gsub` are literals.

**`nan`/`infinite` are supported but serialize to `null` at output.** `nan | type` = `"number"`, `nan | isnan` = `true`, `infinite * -1 < 0` = `true`. `nan` and `infinite` values convert to JSON `null` at the API boundary. Values inside arrays/objects are also normalized to `null`.

**Not supported:** module/import syntax and `modulemeta` (they need filesystem-backed resolution and module-loading semantics that break the current pure library boundary); host-boundary helpers such as `input`, `inputs`, `env`, `$ENV`, and `stderr` (they depend on process state or stdin rather than pure input bytes); and full decimal-mode semantics behind `have_decnum` (they need a different numeric runtime than the current float-based executor).

**Compatibility aliases:** fastjq accepts historical jq names `leaf_paths` and `date` for parity convenience. Current jq 1.8.1 does not expose those names directly; `leaf_paths` maps to `paths(scalars)` semantics and `date` maps to `todate`.

See [SYNTAX.md](docs/SYNTAX.md) for the full allocation-tiered roadmap.

## Further reading

- [SYNTAX.md](docs/SYNTAX.md) — Full operation reference with examples, and a roadmap of unimplemented operations categorised by feasibility
- [BENCHMARKS.md](docs/BENCHMARKS.md) — Complete benchmark tables, raw output, and CLI throughput results
- [DESIGN.md](docs/DESIGN.md) — Architecture details and key design decisions
- [CONSTRAINTS.md](docs/CONSTRAINTS.md) — Performance and scope constraints
- [CHANGELOG.md](CHANGELOG.md) — Change history with tradeoffs and benchmark notes

## License

fastjq is licensed under the [Apache License 2.0](LICENSE). See [NOTICE](NOTICE)
for attribution and [LICENSE-3rdparty.csv](LICENSE-3rdparty.csv) for the
inventory of third-party components used by the benchmark comparison module.

## Contributing

Contributions are welcome — please read [CONTRIBUTING.md](CONTRIBUTING.md) and
[CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) before opening a pull request.
