# fastjq

A fast, zero-allocation jq engine for Go, designed for high-throughput structured log processing.

fastjq operates directly on raw `[]byte` — no `json.Unmarshal`, no `map[string]interface{}`, no marshal/unmarshal cycle. It compiles a query once and executes it against JSON bytes by scanning byte positions, copying only what it needs into an output buffer.

**This is not a full jq implementation.** It supports a targeted subset of jq operations chosen for log processing workloads. See [Limitations](#limitations) before using.

## Design

The standard approach to jq-style processing in Go is:

```
json.Unmarshal → manipulate map[string]interface{} → json.Marshal
```

That round-trip dominates. For a 100KB log object it costs ~870 µs and ~4,600 allocations per record — before you've done any actual work. fastjq eliminates it entirely.

**No parse tree.** fastjq never converts JSON to Go types. The input `[]byte` is the only representation. A scanner tracks a position integer and moves forward through the bytes, skipping values by depth-counting brackets rather than recursing.

**Compile once, run many times.** `Compile` parses the query string into a small AST and allocates the result. That AST is immutable and safe for concurrent use. `Run`/`RunWithBuffer`/`RunFunc` do not allocate — they walk the input bytes with the scanner, guided by the pre-compiled AST.

**Copy only what you need.** Field access returns a sub-slice of the input — no copy. Deletion reconstructs the object by copying the kept fields into an output buffer and inserting its own commas, so the result is always compact regardless of how the input was formatted. Nothing else is touched.

**Zero allocations via caller-owned buffer.** Output is written into a `[]byte` passed in by the caller. The caller reuses the same buffer across calls — it grows if an output exceeds its current capacity, then stabilises. At steady state, zero heap allocations occur per record.

**Early exit.** Operations like `select` and field access stop scanning as soon as they have what they need. `select(.level == "error")` on a 100KB object finds `level` early in the document and exits — it never touches the rest of the bytes.

## Benchmarks

### vs gojq (in-process, Apple M4 Max, Go 1.25)

gojq times include the full `json.Unmarshal` → execute → `json.Marshal` cycle.

| Operation | Input | fastjq | gojq | Speedup |
|-----------|-------|--------|------|---------|
| `select(.f == "x")` | Small (~100B) | 0.0075 µs | 0.558 µs | **74x** |
| `select(.f == "x")` | Large (~100KB, last field) | 21 µs | 788 µs | **38x** |
| `del(.foo)` | Small (~100B) | 0.158 µs | 0.892 µs | **5.6x** |
| `del(.foo)` | Large (~100KB) | 155 µs | 766 µs | **4.9x** |
| `.field` | Small (~100B) | 0.144 µs | 0.327 µs | **2.3x** |
| `.field` | Large (~100KB) | 109 µs | 543 µs | **5.0x** |
| `{f0, f2}` | Small (~100B) | 0.263 µs | 0.665 µs | **2.5x** |
| `.[]` | 200-elem array | 9.7 µs | 80 µs | **8.2x** |

fastjq achieves **0 allocations** on all operations above. The Large Select benchmark uses the last field in a 200-field object so fastjq must scan the full 170KB — even worst-case it's 38x faster than gojq, which must unmarshal all 170KB regardless of which field it needs.

### vs jq CLI (JSONL throughput, 100K lines, ~11MB)

| Operation | jq | fastjq | Speedup |
|-----------|-----|--------|---------|
| `.` (identity) | 0.346s | 0.025s | **14x** |
| `del(.field)` | 0.389s | 0.036s | **11x** |
| `select(.f == "x")` (all match) | 0.368s | 0.030s | **12x** |
| `select(.f \| ascii_downcase == "x")` | 0.650s | 0.036s | **18x** |
| `select(.f \| startswith("x"))` | 0.391s | 0.030s | **13x** |
| `select(has("field"))` | 0.364s | 0.031s | **12x** |
| `.field` | 0.146s | 0.027s | **5x** |
| `{f0, f2}` | 0.251s | 0.048s | **5x** |
| `to_entries` | 0.714s | 0.039s | **18x** |

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

`Compile` allocates. `Run`/`RunWithBuffer`/`RunFunc` achieve zero allocations at steady state across all supported operations.

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
| `.foo == "val"`, `.foo != "val"` | Equality |
| `.foo < val`, `.foo <= val`, `.foo > val`, `.foo >= val` | Ordering (numbers and strings) |
| `expr and expr`, `expr or expr` | Boolean — short-circuit, always return true/false |
| `.foo \| not` | Boolean negation |
| `has("key")` | True if object contains field (even if value is null) |
| `length` | Length: string → char count, array/object → element count, null → 0 |
| `map(expr)` | Apply expr to every array element, collect results |
| `to_entries` | `{"a":1}` → `[{"key":"a","value":1}]` |
| `from_entries` | `[{"key":"a","value":1}]` → `{"a":1}` (also accepts `"name"` for key) |
| `with_entries(expr)` | `to_entries \| map(expr) \| from_entries` — transform or filter object entries |
| `if cond then expr else expr end` | Conditional — else is optional (defaults to identity) |
| `empty` | Produce zero outputs — use as else branch to drop records |
| `select(.level == "error")` | Filter — pass through or drop |
| `.foo // "default"` | Alternative — use right if left is null/false |
| `first` / `last` | First/last element of array (no-arg); or first/last output of `first(expr)` / `last(expr)` |
| `limit(n; expr)` | First N outputs of expr |
| `keys_unsorted` | Object keys in insertion order; array → indices |
| `any` / `any(expr)` | True if any element/result is truthy (short-circuit) |
| `all` / `all(expr)` | True if all elements/results are truthy (short-circuit) |
| `ascii_downcase`, `ascii_upcase` | Convert string to lower/upper case |
| `startswith("s")`, `endswith("s")` | String prefix/suffix test |
| `ltrimstr("s")`, `rtrimstr("s")` | Strip prefix/suffix from string |
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

**Input must be valid JSON.** Behavior on malformed input is undefined. Output is always compact (no pretty-printing).

