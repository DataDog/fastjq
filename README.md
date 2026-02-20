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
| `del(.[n:m])`, `del(.[-n:])` | Delete slice range |
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
| `try expr` / `try expr catch handler` | Suppress errors; `catch` receives the original JSON value when thrown via `error`, or a string for built-in errors |
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
| `@base64` / `@base64d` | Base64 encode/decode (standard and URL-safe, with JSON escape decoding) |
| `@uri` / `@urid` | URL percent-encode / percent-decode |
| `@html` | HTML-escape `&`, `<`, `>`, `'`, `"` |
| `@csv` | Format array as a CSV line (strings double-quoted, quotes doubled) |
| `@tsv` | Format array as a TSV line (tab/newline/backslash escaped) |
| `@sh` | POSIX shell-quote a string |
| `@text` / `tostring` | Strings pass through; non-strings serialized via `tojson` |

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
| `tonumber` | Numbers pass through; strings parsed as floats |
| `null`, `true`, `false`, `"str"`, `123` | Literals |
| `debug` | Print value to stderr, pass through unchanged |
| `floor` / `ceil` / `round` | Numeric rounding (toward −∞ / +∞ / nearest) |
| `contains(val)` | `true` if input recursively contains val (string substring, object key-value subset) |
| `inside(val)` | Reverse: `a \| inside(b)` ≡ `b \| contains(a)` |
| `error` | Throw input as error; `catch` receives the original JSON value |

## Official jq test suite coverage

fastjq is validated against two official jq test files (`go test ./jqtest/`).

| File | Total | Skipped | Attempted | Passed | Failed |
|------|-------|---------|-----------|--------|--------|
| [`tests/jq.test`](https://github.com/jqlang/jq/blob/master/tests/jq.test) (regression suite) | 521 | 335 | 186 | **183 (98.4%)** | 3 |
| [`tests/man.test`](https://github.com/jqlang/jq/blob/master/tests/man.test) (manual examples) | 230 | 119 | 111 | **108 (97.3%)** | 3 |
| **Combined** | **751** | **454** | **297** | **291 (98.0%)** | **6** |

The 6 failures are all known, intentional differences — not bugs:

- **3 (jq.test)**: jq normalises string escape sequences on output (`\r` → `\u000d`, `\u0020` → literal space). fastjq passes bytes through unchanged to preserve zero-copy.
- **3 (man.test)**: Structural differences — object construction emits one output even when a value expression is a multi-output iterator; `==` uses `execSingle` on operands; `[a, b | f]` parses as `[a, (b|f)]` rather than `[(a,b)|f]`.

The 454 skipped tests cover operations outside fastjq's scope: recursive descent (`..`), `sort`/`group_by`/`unique`, path operations, `reduce`/`foreach`, string interpolation, regex, math builtins, date functions, `env`, and others listed in the [Limitations](#limitations) section.

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

**No regex** (`test`, `match`, `capture`, `scan`, `sub`, `gsub`).

**No sorting, grouping, or deduplication** — `sort`, `sort_by`, `group_by`, `unique`, `unique_by` all require O(n) auxiliary index structures, which violates the zero-alloc constraint.

**No `path`, `getpath`, `setpath`, `delpaths`** — not yet implemented.

**No `reduce` / `foreach` / `label-break` / variable binding** (`as $x`) / **user-defined functions** (`def`).

**No math beyond rounding** — `sin`, `cos`, `pow`, `log`, `sqrt`, etc. are not supported. `floor`, `ceil`, `round` are available.

**No Unicode codepoint conversion** (`explode`, `implode`).

**Output is always compact JSON.** Input can be pretty-printed or compact. fastjq never panics — malformed input may produce wrong results but the process is always safe.

See [SYNTAX.md](docs/SYNTAX.md) for the full categorised roadmap of unimplemented operations.

## Further reading

- [SYNTAX.md](docs/SYNTAX.md) — Full operation reference with examples, and a roadmap of unimplemented operations categorised by feasibility
- [BENCHMARKS.md](docs/BENCHMARKS.md) — Complete benchmark tables, raw output, and CLI throughput results
- [DESIGN.md](docs/DESIGN.md) — Architecture details and key design decisions
- [CONSTRAINTS.md](docs/CONSTRAINTS.md) — Performance and scope constraints
- [CHANGELOG.md](CHANGELOG.md) — Change history with tradeoffs and benchmark notes
