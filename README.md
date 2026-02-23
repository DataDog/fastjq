# fastjq

A fast, allocation-conscious jq engine for Go, designed for high-throughput structured log processing.

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

fastjq's allocation model: **core operations are 0 allocs** (access, filtering, comparison, arithmetic, construction, math, `test(re)`). Operations that return new structured data allocate proportional to their *output* size — never to input size: `@base64`/`@uri` (4 allocs; string decoding), `match`/`capture` (1 alloc on a hit; `[]int` for subgroup indices), `scan`/`gsub` (allocs per match), `map(f)` when `f` builds new data (~1 per element). See the `allocs` column in [BENCHMARKS.md](docs/BENCHMARKS.md).

| Operation | Input | fastjq | gojq | Speedup | allocs |
|-----------|-------|--------|------|---------|--------|
| `select(.f == "x")` | Small (~100B) | 0.011 µs | 0.57 µs | **51x** | 0 |
| `select(.f == "x")`¹ | Large (~100KB) | 31 µs | 781 µs | **25x** | 0 |
| `.field` | Small (~100B) | 0.085 µs | 0.35 µs | **4.1x** | 0 |
| `.field` | Large (~100KB) | 7.8 µs | 582 µs | **75x** | 0 |
| `del(.f)` | Large (~100KB) | 34 µs | 813 µs | **24x** | 0 |
| `select(.f \| ascii_downcase == "x")` | Large (~100KB) | 31 µs | 789 µs | **25x** | 0 |
| `map(.name)` | 200-elem array (~6KB) | 13 µs | 92 µs | **7.1x** | 0 |
| `min_by(.value)` | 100-elem array (~3KB) | 8.3 µs | 55 µs | **6.6x** | 0 |
| `.a * .b` (multiply) | Small (~100B) | 0.065 µs | 0.74 µs | **11x** | 0 |
| `first(.[] \| select(. > 100))` | 200-int array | 3.9 µs | 1.4 µs | **0.4x**² | 0 |

¹ Large select uses the last field in a 200-field object — fastjq scans the full document, no early-exit advantage.
² gojq wins on small arrays of raw integers: after unmarshal, element access is native Go slice operations.

The speedup is largest on small inputs where gojq's marshal/unmarshal overhead dominates. On large inputs fastjq is 18–75x faster thanks to SIMD-accelerated string scanning. The exception is small primitive integer arrays, where gojq's in-memory representation wins.

### vs jq CLI (JSONL throughput, 100K lines, ~11MB, Apple M4 Max, jq 1.8.1)

| Operation | Input | jq (s) | fastjq (s) | Speedup |
|-----------|-------|--------|------------|---------|
| `.` (identity) | small | 0.343 | 0.026 | **13x** |
| `.field` | small | 0.151 | 0.021 | **7x** |
| `.field` | large (~16MB, 100 lines) | 0.093 | 0.013 | **7x** |
| `del(.field)` | small | 0.365 | 0.034 | **11x** |
| `{field_0, field_2}` (construct) | small | 0.247 | 0.032 | **8x** |
| `select(.f == "x")` (all match) | small | 0.372 | 0.023 | **16x** |
| `select(.f == "x")` (none match) | small | 0.141 | 0.023 | **6x** |
| `.field // "default"` | small | 0.168 | 0.022 | **8x** |
| `select(.f \| ascii_downcase == "x")` | small | 0.679 | 0.033 | **21x** |
| `select(.f \| startswith("x"))` | small | 0.366 | 0.026 | **14x** |
| `select(has("field"))` | small | 0.365 | 0.023 | **16x** |
| `to_entries` | small | 0.717 | 0.040 | **18x** |
| `keys_unsorted` | small | 0.243 | 0.031 | **8x** |

See [BENCHMARKS.md](docs/BENCHMARKS.md) for the complete table.

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

- **Access:** `.`, `.foo`, `.[0]`, `.[]`, `.[n:m]`, `.foo?`, chained access
- **Modification:** `del(.foo)`, `del(.[n:m])`, `{name, a: .b}`, `[.a, .b]`
- **Control flow:** `\|`, `select`, `if-elif-else`, `try-catch`, `//`, `empty`, `"\(expr)"`
- **Arithmetic:** `+`, `-`, `*`, `/`, `%`, `add`, `floor`, `ceil`, `round`, `nearbyint`
- **Math:** `sqrt`, `log`, `exp`, `sin`, `cos`, `atan`, `tgamma`, `j0`, and 15 more — all zero-alloc
- **Arrays:** `map`, `flatten`, `min/max`, `any/all`, `first/last`, `limit`, `nth`, `isempty`, `values`, type filters, `index/indices`
- **Strings:** `split/join`, `ascii_downcase/upcase`, `startswith/endswith`, `ltrimstr/rtrimstr`, `"\(expr)"` interpolation
- **Format strings:** `@base64/d`, `@uri/d`, `@html`, `@csv`, `@tsv`, `@sh`, `@text`, `@json`
- **Regex (Go RE2):** `test(re)` *(0 allocs)*, `match(re)`, `capture(re)`, `scan(re)`, `sub(re; s)`, `gsub(re; s)`
- **Objects:** `to_entries`, `from_entries`, `keys_unsorted`, `length`, `has`, `in`, `contains`, `inside`
- **Type:** `type`, `tojson/fromjson`, `tostring/tonumber`, `debug`, `error`

## Official jq test suite coverage

fastjq is validated against two official jq test files (`go test ./jqtest/`).

| File | Total | Skipped | Attempted | Passed | Failed |
|------|-------|---------|-----------|--------|--------|
| [`tests/jq.test`](https://github.com/jqlang/jq/blob/master/tests/jq.test) (regression suite) | 521 | 321 | 200 | **194 (97.0%)** | 6 |
| [`tests/man.test`](https://github.com/jqlang/jq/blob/master/tests/man.test) (manual examples) | 230 | 112 | 118 | **115 (97.5%)** | 3 |
| **Combined** | **751** | **433** | **318** | **309 (97.2%)** | **9** |

The 9 failures are all known, intentional differences — not bugs:

- **3 (jq.test — string normalisation)**: jq normalises escape sequences (`\r` → `\u000d`, `\u0020` → literal space); fastjq passes bytes through unchanged (zero-copy constraint).
- **3 (jq.test — architectural)**: Error message format (`"expected object for field access"` vs jq's verbose form); `try body catch h` catches errors from the entire callback chain not just the body (see CHANGELOG for details).
- **3 (man.test)**: Object construction with multi-output iterator value; `==` uses `execSingle` on both operands; `[a, b | f]` parses as `[a, (b|f)]`.

The skipped tests cover operations outside fastjq's scope: recursive descent (`..`), `sort`/`group_by`/`unique`, path operations, `reduce`/`foreach`, string interpolation (jq's `onig.test` regex tests use PCRE/Oniguruma syntax), math builtins, date functions, `env`, and others listed in the [Limitations](#limitations) section.

## Limitations

fastjq is intentionally scope-limited. It will not grow into a full jq implementation.

**`select` conditions must be single-valued.**
`select(.items[] == "x")` silently tests only the first element. Use `any(.[]; . == "x")` instead.

**`.field` on `null` errors — use `.field?` for null-safe access.**
`null | .field` errors in fastjq (jq returns null). Use `.a?.b?` for full null-safety.

**`map(f)` / `[.[] | f]` allocates when `f` constructs new data.**
`map(.name)` is 0 allocs. `map({name, price})` allocates ~1 buffer per element. fastjq still uses 5–8x fewer allocations than gojq on these queries.

**String escape sequences pass through unchanged.**
fastjq does not normalise string escapes on output (`\r` stays `\r`, not `\u000d`). Re-encoding every string would violate the zero-copy constraint.

**Regex uses Go RE2, not PCRE/Oniguruma.**
Named captures require `(?P<name>...)` syntax. Backreferences and lookahead are unsupported. `test(re)` is 0-alloc; `match`/`capture` alloc one `[]int` on a hit; `scan`/`gsub` alloc per match. Replacement strings in `sub`/`gsub` are literals (no `\(...)` references).

**`@format "template"` combined syntax not supported.**
`@html "<b>\(.)</b>"` applies the format to each interpolated value — not yet implemented. Plain string interpolation `"\(.field)"` and standalone format strings (`@html`, `@csv`, `@sh`, etc.) both work fine.

**try-catch catches errors from the full callback chain, not just the body.**
`(try . catch h) | right` — if `right` errors, the try catches it. jq scopes the catch to the body only. Complex nested try patterns may behave differently.

**`[a, b | f]` parses as `[a, (b|f)]`.**
jq treats `[a, b | f]` as `[(a,b) | f]`. fastjq parses array elements independently, so `[true, false | not]` gives `[true, true]` not `[false, true]`.

**No recursive descent** (`..|..` / `recurse`).

**No sorting, grouping, or deduplication** — `sort`, `sort_by`, `group_by`, `unique`, `unique_by` require O(n) auxiliary storage.

**Not yet implemented:** `path`, `getpath`, `setpath`, `delpaths`, `reduce`, `foreach`, `label-break`, variable binding (`as $x`), user-defined functions (`def`), `explode`/`implode`, `nan`/`infinite` constants, 2-arg math forms (`pow(x;y)`, `hypot(x;y)`, `atan(y;x)`).

**Output is always compact JSON.** fastjq never panics — malformed input may produce wrong results but the process is always safe.

See [SYNTAX.md](docs/SYNTAX.md) for the full categorised roadmap of unimplemented operations.

## Further reading

- [SYNTAX.md](docs/SYNTAX.md) — Full operation reference with examples, and a roadmap of unimplemented operations categorised by feasibility
- [BENCHMARKS.md](docs/BENCHMARKS.md) — Complete benchmark tables, raw output, and CLI throughput results
- [DESIGN.md](docs/DESIGN.md) — Architecture details and key design decisions
- [CONSTRAINTS.md](docs/CONSTRAINTS.md) — Performance and scope constraints
- [CHANGELOG.md](CHANGELOG.md) — Change history with tradeoffs and benchmark notes
