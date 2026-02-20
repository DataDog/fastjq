# fastjq

A fast, zero-allocation jq engine for Go, designed for high-throughput structured log processing.

fastjq operates directly on raw `[]byte` — no `json.Unmarshal`, no `map[string]interface{}`, no marshal/unmarshal cycle. It compiles a query once and executes it against JSON bytes by scanning byte positions, copying only what it needs into an output buffer.

**This is not a full jq implementation.** It supports a targeted subset of jq operations chosen for log processing workloads. See [Limitations](#limitations) before using.

**Requires valid JSON input.** fastjq does not validate its input — behavior on malformed JSON is undefined. It never panics (enforced by fuzz tests), but may silently produce wrong results. Use `json.Valid` if you can't guarantee the source.

## Design

The standard approach costs ~870 µs and ~4,600 allocations just to parse a 100KB log object — before you've done any work. fastjq eliminates it:

- **No parse tree.** Input `[]byte` is the only representation. A scanner moves through bytes tracking a position integer, skipping values by depth-counting brackets. Nothing is ever converted to Go types.
- **Compile once, run many times.** `Compile` builds an immutable AST (allocates once). `Run`/`RunWithBuffer`/`RunFunc` walk the input guided by that AST — no allocations at runtime with a reused buffer.
- **Copy only what you need.** Field access returns a sub-slice of input — no copy. Deletion reconstructs the object into an output buffer with its own commas. Everything else is left untouched.
- **Caller-owned buffer.** Output is written into a `[]byte` the caller passes in. It grows if needed, then stabilises. At steady state, zero heap allocations per record.
- **Early exit.** `select(.level == "error")` on a 100KB object stops scanning the moment it finds `level` — the rest of the bytes are never read.

## Benchmarks

Compared against [gojq](https://github.com/itchyny/gojq), the standard jq library for Go. gojq times include the full `json.Unmarshal → execute → json.Marshal` cycle. Apple M4 Max, Go 1.25. fastjq achieves **0 allocations** in steady state on all operations when using `RunWithBuffer` or `RunFunc`.

| Operation | Input | fastjq | gojq | Speedup | allocs |
|-----------|-------|--------|------|---------|--------|
| `select(.f == "x")` | Small (~100B) | 0.010 µs | 0.56 µs | **56x** | 0 |
| `select(.f == "x")`¹ | Large (~100KB) | 187 µs | 770 µs | **4.1x** | 0 |
| `.field` | Small (~100B) | 0.141 µs | 0.34 µs | **2.4x** | 0 |
| `.field` | Large (~100KB) | 112 µs | 570 µs | **5.1x** | 0 |
| `del(.f)` | Large (~100KB) | 192 µs | 794 µs | **4.1x** | 0 |
| `select(.f \| ascii_downcase == "x")` | Large (~100KB) | 141 µs | 799 µs | **5.7x** | 0 |
| `map(.name)` | 200-elem array (~6KB) | 20 µs | 92 µs | **4.6x** | 0 |
| `min_by(.value)` | 100-elem array (~3KB) | 12 µs | 56 µs | **4.7x** | 0 |
| `.a * .b` (multiply) | Small (~100B) | 0.087 µs | 0.71 µs | **8.1x** | 0 |
| `first(.[] \| select(. > 100))` | 200-int array | 3.6 µs | 1.4 µs | **0.4x**² | 0 |

¹ Large select uses the last field in a 200-field object — fastjq scans the full document, no early-exit advantage.
² gojq wins on small arrays of raw integers: after unmarshal, element access is native Go slice operations.

The speedup is largest on small inputs where gojq's marshal/unmarshal overhead dominates. On large inputs both engines scan bytes and fastjq is still 4–5x faster. The exception is small primitive integer arrays, where gojq's in-memory representation wins.

### vs jq CLI (JSONL throughput, 100K lines, ~11MB, Apple M4 Max, jq 1.8.1)

| Operation | Input | jq (s) | fastjq (s) | Speedup |
|-----------|-------|--------|------------|---------|
| `.` (identity) | small | 0.344 | 0.025 | **14x** |
| `.field` | small | 0.145 | 0.024 | **6x** |
| `.field` | large (~16MB, 100 lines) | 0.088 | 0.023 | **4x** |
| `del(.field)` | small | 0.369 | 0.036 | **10x** |
| `{field_0, field_2}` (construct) | small | 0.268 | 0.051 | **5x** |
| `select(.f == "x")` (all match) | small | 0.370 | 0.028 | **13x** |
| `select(.f == "x")` (none match) | small | 0.138 | 0.027 | **5x** |
| `.field // "default"` | small | 0.166 | 0.026 | **6x** |
| `select(.f \| ascii_downcase == "x")` | small | 0.651 | 0.038 | **17x** |
| `select(.f \| startswith("x"))` | small | 0.363 | 0.029 | **13x** |
| `select(has("field"))` | small | 0.366 | 0.027 | **14x** |
| `to_entries` | small | 0.717 | 0.040 | **18x** |
| `keys_unsorted` | small | 0.247 | 0.029 | **9x** |

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

`Compile` allocates. `Run`/`RunWithBuffer`/`RunFunc` achieve zero allocations at steady state with a reused buffer.

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
Conditions using `and`/`or` work fine. Conditions using an iterator (e.g. `select(.items[] == "x")`) silently test only the first element. Use `any(.[]; . == "x")` instead.

**`del` paths must be literal field, index, or slice expressions.**
Dynamic deletion (`del(.items[])`, `del(.items[] | select(...))`) returns an error.
Slice ranges (`del(.[2:4])`, `del(.[-2:])`) are supported.

**`.field` on `null` errors — use `.field?` for null-safe access.**
In jq, `null | .field` returns `null`. In fastjq it errors. Missing fields return `null` and skip their child chain, but an explicitly `null` value in the middle of a chain will error. Use `.a?.b?` for full null-safety.

**`map(f)` / `[.[] | f]` allocates when `f` constructs new data.**
`map(.name)` is 0 allocs (field access returns an input sub-slice). `map({name, price})` or `map(.a * .b)` allocate ~1 buffer per element — the array builder can't share scratch across multiple callback invocations without aliasing. fastjq still allocates 5–8x less than gojq on these queries.

**String escape sequences pass through unchanged.**
fastjq does not normalise string escapes on output (`\r` stays `\r`, not `\u000d`). This is intentional — re-encoding every string byte-for-byte would violate the zero-copy constraint.

**No string interpolation** (`"\(.field)"` is not supported).

**No recursive descent** (`..|..` / `recurse`).

**Regex uses Go RE2** (not PCRE/Oniguruma). Named captures require `(?P<name>...)` syntax. Backreferences and lookahead are unsupported. `test(re)` is 0-alloc; `match`/`capture` allocate one `[]int` on a hit; `scan`/`gsub` allocate proportional to match count. Replacement strings in `sub`/`gsub` are literals — `\(.field)` capture group references are not supported.

**No sorting, grouping, or deduplication** — `sort`, `sort_by`, `group_by`, `unique`, `unique_by` all require O(n) auxiliary index structures, which violates the zero-alloc constraint.

**No `path`, `getpath`, `setpath`, `delpaths`** — not yet implemented.

**No `reduce` / `foreach` / `label-break` / variable binding** (`as $x`) / **user-defined functions** (`def`).

**`nan` / `infinite` constants not supported** — they produce non-JSON output, violating the compact-JSON output constraint. All 1-arg math functions (`sqrt`, `sin`, `cos`, `log`, etc.) are supported; NaN/Inf *results* are output as `null`. 2-arg forms (`pow(x;y)`, `hypot(x;y)`, `atan(y;x)`) are not yet supported.

**No Unicode codepoint conversion** (`explode`, `implode`).

**Output is always compact JSON.** Input can be pretty-printed or compact. fastjq never panics — malformed input may produce wrong results but the process is always safe.

See [SYNTAX.md](docs/SYNTAX.md) for the full categorised roadmap of unimplemented operations.

## Further reading

- [SYNTAX.md](docs/SYNTAX.md) — Full operation reference with examples, and a roadmap of unimplemented operations categorised by feasibility
- [BENCHMARKS.md](docs/BENCHMARKS.md) — Complete benchmark tables, raw output, and CLI throughput results
- [DESIGN.md](docs/DESIGN.md) — Architecture details and key design decisions
- [CONSTRAINTS.md](docs/CONSTRAINTS.md) — Performance and scope constraints
- [CHANGELOG.md](CHANGELOG.md) — Change history with tradeoffs and benchmark notes
