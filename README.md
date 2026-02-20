# fastjq

A fast, zero-allocation jq engine for Go, designed for high-throughput structured log processing.

fastjq operates directly on raw `[]byte` — no `json.Unmarshal`, no `map[string]interface{}`, no marshal/unmarshal cycle. It compiles a query once and executes it against JSON bytes by scanning byte positions, copying only what it needs into an output buffer.

**This is not a full jq implementation.** It supports a targeted subset of jq operations chosen for log processing workloads. See [Limitations](#limitations) before using.

> **Input must be valid JSON.** fastjq does not validate its input — it assumes well-formed JSON and skips values using bracket depth-counting rather than full parsing. Malformed input produces undefined behaviour: wrong results, panics, or silent corruption. Validate input with `json.Valid` or `json.Unmarshal` before passing it to fastjq if you cannot guarantee its source.

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

Compared against [gojq](https://github.com/itchyny/gojq), the standard jq library for Go. gojq times include the full `json.Unmarshal → execute → json.Marshal` cycle. Apple M4 Max, Go 1.25. fastjq achieves **0 allocations** in steady state on all operations when using `RunWithBuffer` or `RunFunc`.

| Operation | Input | fastjq | gojq | Speedup | allocs |
|-----------|-------|--------|------|---------|--------|
| `select(.level == "error")` | Small (~100B) | 0.009 µs | 0.57 µs | **64x** | 0 |
| `select(.level == "error")` | Large (~100KB)¹ | 16 µs | 770 µs | **48x** | 0 |
| `.field` | Large (~100KB) | 109 µs | 543 µs | **5x** | 0 |
| `del(.sensitive)` | Large (~100KB) | 155 µs | 766 µs | **5x** | 0 |
| `select(.f \| ascii_downcase == "x")` | Large (~100KB) | 178 µs | 794 µs | **4.5x** | 0 |
| `map(.name)` | 200-elem array (~6KB) | 20 µs | 91 µs | **4.5x** | 0 |
| `min_by(.value)` | 100-elem array (~3KB) | 12 µs | 56 µs | **4.5x** | 0 |
| `.a * .b` (multiply) | Small (~100B) | 0.08 µs | 0.68 µs | **8x** | 0 |
| `first(.[] \| select(. > 100))` | 200-int array | 3.6 µs | 1.4 µs | **0.4x**² | 0 |

¹ The large select benchmark uses the **last** field in a 200-field object — fastjq scans the full document, no early-exit advantage.
² gojq is faster on small arrays of raw integers: after unmarshal it accesses a native Go slice; fastjq always scans bytes.

The advantage is largest on small inputs, where gojq's marshal/unmarshal overhead dominates regardless of how simple the query is. On large inputs both engines are doing real work scanning bytes, and fastjq is still consistently 4–5x faster. The exception is small primitive integer arrays, where gojq's in-memory representation wins.

### vs jq CLI (JSONL throughput, 100K lines, ~11MB, Apple M4 Max)

| Operation | jq | fastjq | Speedup |
|-----------|-----|--------|---------|
| `.` (identity) | 0.346s | 0.025s | **14x** |
| `del(.field)` | 0.389s | 0.036s | **11x** |
| `select(.f == "x")` | 0.368s | 0.030s | **12x** |
| `select(.f \| ascii_downcase == "x")` | 0.650s | 0.036s | **18x** |
| `select(has("field"))` | 0.364s | 0.031s | **12x** |
| `.field` | 0.146s | 0.027s | **5x** |
| `to_entries` | 0.714s | 0.039s | **18x** |

See [BENCHMARKS.md](BENCHMARKS.md) for the complete table.

## Try it out (CLI)

A minimal JSONL processor CLI is included for benchmarking and quick experimentation:

```bash
# Build
go build -o fastjq ./cmd/fastjq

# Filter error logs from a JSONL stream
cat app.log | ./fastjq 'select(.level == "error")'

# Case-insensitive filter
cat app.log | ./fastjq 'select(.level | ascii_downcase == "error")'

# Drop sensitive fields
cat app.log | ./fastjq 'del(.password, .token)'

# Benchmark against jq CLI (requires jq in PATH)
chmod +x bench_vs_jq.sh
./bench_vs_jq.sh
```

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

**Access**

| Syntax | Description |
|--------|-------------|
| `.` | Identity — pass input through |
| `.foo`, `.foo.bar` | Field access, nested |
| `.[0]`, `.[-1]` | Array index (negative counts from end) |
| `.[]` | Iterator — emit each element |
| `.items[]`, `.items[0]`, `.data[0].name` | Chained access |
| `.[n:m]`, `.[:m]`, `.[n:]` | Slice array or string (negative indices from end) |
| `.foo?`, `.[0]?`, `.[]?` | Optional — suppress errors, produce no output |

**Modification**

| Syntax | Description |
|--------|-------------|
| `del(.foo)`, `del(.foo, .bar)` | Delete fields |
| `del(.foo.bar)` | Delete nested field |
| `del(.[0])`, `del(.[1], .[3])` | Delete array elements |
| `{name, age}` | Object construction (shorthand) |
| `{a: .foo, b: .bar}` | Object construction (rename / literal values) |
| `[.foo, .bar]` | Array construction |

**Logic & Control Flow**

| Syntax | Description |
|--------|-------------|
| `expr \| expr` | Pipe |
| `.foo == "val"`, `.foo != "val"`, `<`, `<=`, `>`, `>=` | Comparison |
| `expr and expr`, `expr or expr`, `expr \| not` | Boolean operators — always return true/false |
| `select(cond)` | Filter — emit input if truthy, nothing if falsy |
| `if cond then expr [elif cond then expr]* [else expr] end` | Conditional — elif and else optional |
| `try expr` / `try expr catch handler` | Suppress errors; catch receives error message as string |
| `.foo // "default"` | Alternative — right if left is null/false |
| `empty` | Produce zero outputs |

**Arithmetic**

| Syntax | Description |
|--------|-------------|
| `expr + expr` | Numbers: sum; strings/arrays: concat; objects: merge (right wins); null: identity |
| `expr - expr` | Numbers: subtract; arrays: difference (remove right's elements from left) |
| `expr * expr` | Numbers: multiply; `"str" * n`: repeat string n times |
| `expr / expr` | Numbers: divide; `"str" / "sep"`: split by separator |
| `expr % expr` | Number modulo |
| `add` | Reduce array: sum numbers, concat strings/arrays, merge objects |

**Arrays**

| Syntax | Description |
|--------|-------------|
| `map(expr)` | Apply expr to every element, collect results |
| `flatten`, `flatten(n)` | Flatten nested arrays (n = max depth) |
| `min` / `max` | Minimum / maximum element (numbers or strings) |
| `min_by(f)` / `max_by(f)` | Element with min/max value of `f` |
| `any` / `any(expr)` / `any(gen; cond)` | True if any element is truthy / matches expr / gen\|cond |
| `all` / `all(expr)` / `all(gen; cond)` | True if all elements truthy / match expr / gen\|cond |
| `first` / `last` | First/last element (or `first(expr)` / `last(expr)`) |
| `limit(n; expr)` | First N outputs of expr |
| `values` | Filter nulls — emit only non-null values |
| `numbers`, `strings`, `arrays`, `objects`, `booleans`, `nulls`, `iterables`, `scalars` | Type filters |
| `index(s)`, `rindex(s)`, `indices(s)` | First/last/all occurrences of value in string or array |
| `has(key)`, `has(n)` | True if object has field / array has index |
| `in(obj)` | Reverse membership: `"key" \| in({"key":1})` |

**Strings**

| Syntax | Description |
|--------|-------------|
| `split("s")` / `join("s")` | Split string by separator / join array with separator |
| `ascii_downcase`, `ascii_upcase` | Case conversion |
| `startswith("s")`, `endswith("s")` | Prefix/suffix test |
| `ltrimstr("s")`, `rtrimstr("s")` | Strip prefix/suffix |
| `@base64` / `@base64d` | Base64 encode/decode (handles standard and URL-safe variants) |
| `@uri` | URL percent-encode (RFC 3986 unreserved chars pass through) |

**Objects**

| Syntax | Description |
|--------|-------------|
| `to_entries` | `{"a":1}` → `[{"key":"a","value":1}]` |
| `from_entries` | `[{"key":"a","value":1}]` → `{"a":1}` (also accepts `"name"`) |
| `keys_unsorted` | Object keys in insertion order; array → indices |
| `length` | String → char count; array/object → count; null → 0 |

**Type & Inspection**

| Syntax | Description |
|--------|-------------|
| `type` | Type name: `"string"`, `"number"`, `"object"`, `"array"`, `"boolean"`, `"null"` |
| `tojson` / `@json` | Serialize any value as a JSON string |
| `fromjson` | Parse a JSON string to its contained value |
| `tostring` | Strings pass through; non-strings serialized via `tojson` |
| `tonumber` | Numbers pass through; strings parsed as floats |
| `null`, `true`, `false`, `"str"`, `123` | Literals |
| `debug` | Print value to stderr, pass through unchanged |

## Limitations

fastjq is intentionally scope-limited. It will not grow into a full jq implementation.

**`select` conditions must be single-valued.**
The condition is evaluated via a single-result path. Conditions using `and`/`or` work fine. Conditions using an iterator (e.g. `select(.items[] == "x")`) silently test only the first element. Use `any(.[]; . == "x")` instead.

**`del` paths must be literal field or index expressions.**
`del(.foo)`, `del(.foo.bar)`, `del(.[0])`, and `del(.foo, .bar)` work. Dynamic deletion does not: `del(.items[])` and `del(.items[] | select(...))` both return an error.

**`.field` on `null` errors — use `.field?` for null-safe access.**
In jq, `null | .field` returns `null`. In fastjq it errors. This affects chained access when an intermediate field is absent: `.a.b` where `.a` is a missing field returns `null` (absent field → null → child chain skipped), but `.a.b` where `.a` is explicitly `null` errors. Use `.a?.b?` for full null-safety.

**No string interpolation.**
`"\(.field)"` template syntax is not supported.

**No higher-order functions or builtins beyond those listed.**
`reduce`, `foreach`, `@csv`, `@html`, `env`, `path`, `sort`, `group_by`, `unique`, `test` (regex), etc. are not supported. See [SYNTAX.md](SYNTAX.md) for the full roadmap.

**No recursive descent** (`..|..`).

**`map(f)` / `[.[] | f]` allocates when `f` constructs new data.**
These queries achieve 0 allocs when `f` returns a field or sub-slice of the input (e.g. `map(.name)`). When `f` builds new bytes — object construction `{…}`, arithmetic, string concatenation — fastjq allocates ~1 buffer per element. This is a structural constraint: the array builder must keep element scratch separate from the output buffer because an iterator invokes the callback multiple times, and sharing a scratch position would cause each invocation to overwrite the previous result. fastjq still allocates 5–8x less than gojq on these queries.

**Output is always compact JSON.** Input can be pretty-printed or compact.

## Further reading

- [SYNTAX.md](SYNTAX.md) — Full operation reference with examples, and a roadmap of unimplemented operations categorised by feasibility
- [BENCHMARKS.md](BENCHMARKS.md) — Complete benchmark tables (Small/Medium/Large), raw output, and CLI throughput results
- [DESIGN.md](DESIGN.md) — Architecture details, key design decisions, supported operations list
- [CONSTRAINTS.md](CONSTRAINTS.md) — Performance and scope constraints; what the library will and won't do
- [CHANGELOG.md](CHANGELOG.md) — Change history with tradeoffs and benchmark notes

