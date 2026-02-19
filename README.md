# fastjq

A fast, zero-allocation jq engine for Go, designed for high-throughput structured log processing.

fastjq operates directly on raw `[]byte` — no `json.Unmarshal`, no `map[string]interface{}`, no marshal/unmarshal cycle. It compiles a query once and executes it against JSON bytes by scanning byte positions, copying only what it needs into an output buffer.

**This is not a full jq implementation.** It supports a targeted subset of jq operations chosen for log processing workloads. See [Limitations](#limitations) before using.

## Why

If you're processing JSONL log streams in Go — filtering by level, extracting fields, dropping sensitive keys — the standard approach is:

```
json.Unmarshal → manipulate map[string]interface{} → json.Marshal
```

That round-trip dominates. For a 100KB log object, it costs ~870 µs and ~4,600 allocations per record, before you've done any actual work.

fastjq eliminates the round-trip. A `select(.level == "error")` on a 100KB object takes **7 ns** and **0 allocations** — it scans to the `level` field and exits immediately.

## Install

```bash
go get github.com/brianfloersch/fastjq
```

## Usage

```go
// Compile once — safe for concurrent use
p, err := fastjq.Compile(`select(.level == "error")`)

// Run against each log line
err = p.RunFunc(line, func(result []byte) error {
    // called for each output (0 outputs if select doesn't match)
    _, err := w.Write(result)
    return err
})

// Or use RunWithBuffer to reuse a buffer across calls (zero steady-state allocs)
buf := make([]byte, 0, 1024)
buf, err = p.RunWithBuffer(line, buf)

// RunAll collects all outputs (allocates)
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

`Compile` allocates. `Run`/`RunWithBuffer`/`RunFunc` do not (for basic ops with a reused buffer).

## Supported Operations

| Syntax | Description |
|--------|-------------|
| `.` | Identity — pass input through |
| `.foo`, `.foo.bar` | Field access, nested |
| `.[0]`, `.[-1]` | Array index, negative from end |
| `.[]` | Iterator — emit each element |
| `.items[]`, `.items[0]`, `.data[0].name` | Chained access |
| `del(.foo)`, `del(.foo, .bar)` | Delete fields |
| `del(.foo.bar)` | Delete nested field |
| `del(.[0])`, `del(.[1], .[3])` | Delete array elements |
| `{name, age}` | Object construction (shorthand) |
| `{a: .foo, b: .bar}` | Object construction (rename) |
| `[.foo, .bar]` | Array construction |
| `expr \| expr` | Pipe |
| `null`, `true`, `false`, `"str"`, `123` | Literals |
| `.foo == "val"`, `.foo != "val"` | Comparison |
| `select(.level == "error")` | Filter — pass through or drop |
| `.foo // "default"` | Alternative — use right if left is null/false |
| `.foo?`, `.[0]?`, `.[]?` | Optional — suppress errors |
| `type` | Type name: `"string"`, `"number"`, `"object"`, etc. |

## Limitations

fastjq is intentionally scope-limited. It will not grow into a full jq implementation.

**`select` conditions must produce a single value.**
The condition is evaluated via a single-result path. If the condition uses an iterator (e.g. `select(.items[] == "x")`), only the first element is tested and the rest are silently ignored. Use simple field comparisons: `select(.field == "value")`.

**`del` paths must be literal field or index expressions.**
`del(.foo)`, `del(.foo.bar)`, `del(.[0])`, and `del(.foo, .bar)` work. Dynamic deletion does not: `del(.items[])` and `del(.items[] | select(...))` both return an error.

**No arithmetic, string interpolation, or functions.**
`+`, `-`, `*`, `/`, `length`, `keys`, `map`, `reduce`, `@base64`, `env`, `path`, etc. are not supported.

**No recursive descent** (`..|..`).

**No try-catch** (beyond `?` optional suppression).

**Input must be valid, compact JSON.** Behavior on malformed input is undefined. Output is always compact (no pretty-printing).

## Benchmarks

All benchmarks on Apple M4 Max, Go 1.25. gojq times include the full `json.Unmarshal` → execute → `json.Marshal` cycle.

### vs gojq (in-process)

| Operation | Input | fastjq | gojq | Speedup |
|-----------|-------|--------|------|---------|
| `select(.f == "x")` | Small (~100B) | 0.0074 µs | 0.527 µs | **71x** |
| `select(.f == "x")` | Large (~100KB) | 0.0074 µs | 710 µs | **96,000x** |
| `del(.foo)` | Small (~100B) | 0.163 µs | 0.864 µs | **5.3x** |
| `del(.foo)` | Large (~100KB) | 130 µs | 730 µs | **5.6x** |
| `.field` | Small (~100B) | 0.141 µs | 0.318 µs | **2.3x** |
| `.field` | Large (~100KB) | 103 µs | 527 µs | **5.1x** |
| `{f0, f2}` | Small (~100B) | 0.287 µs | 0.653 µs | **2.3x** |
| `.[]` | 200-elem array | 9.2 µs | 77 µs | **8.4x** |

fastjq achieves **0 allocations** on all operations above when using `RunWithBuffer` or `RunFunc`.

The `select` speedup on large JSON (96,000x) is not a typo: fastjq scans forward to the compared field and exits immediately, while gojq must unmarshal the entire 170KB object before it can evaluate the condition.

### vs jq CLI (JSONL throughput)

End-to-end throughput on JSONL streams (100K lines, ~11MB). Both tools read from stdin, write to `/dev/null`.

| Operation | jq | fastjq | Speedup |
|-----------|-----|--------|---------|
| `.` (identity) | 0.323s | 0.031s | **10x** |
| `del(.field)` | 0.336s | 0.033s | **10x** |
| `select(.f == "x")` (all match) | 0.368s | 0.031s | **12x** |
| `.field` | 0.149s | 0.024s | **6x** |
| `{f0, f2}` | 0.246s | 0.048s | **5x** |

Run `./bench_vs_jq.sh` to reproduce.

## How It Works

1. **Compile**: parse the query string into a small AST. Allocates once.
2. **Execute**: walk the input `[]byte` with a zero-copy scanner. Field access finds the key by scanning byte-by-byte; deletion reconstructs the object with its own commas (never copies commas from input). Nothing is converted to a Go type.
3. **Output**: results are written into a caller-supplied `[]byte` buffer, returned as a sub-slice.

See [DESIGN.md](DESIGN.md) for architecture details.

## License

MIT
