# fastjq Design Document

## Overview

fastjq is a fast, minimal JQ engine for Go that avoids the expensive
marshal/unmarshal cycle of gojq. It operates directly on raw `[]byte` using
slice offsets — no `interface{}`, no `map[string]interface{}`.

## Architecture

### Core Idea

1. **Scanner** scans input bytes tracking positions, never copies data
2. **Query parser** compiles jq expressions into a small AST (allocates once at compile time, reused across runs)
3. **Executor** walks input with the scanner, copies only needed byte ranges into an output buffer

### Supported Operations

**Access & modification**
- `.` (identity)
- `.foo`, `.foo.bar` (field access, including nested)
- `del(.foo)`, `del(.foo, .bar)`, `del(.foo.bar)` (field deletion, including nested)
- `.[0]`, `.[-1]` (array indexing, negative = from end)
- `del(.[0])`, `del(.[1], .[3])` (array element deletion)
- `del(.[n:m])`, `del(.[-n:])` (array slice deletion)
- `.[]` (iterator — all elements of array or values of object)
- `.items[]`, `.items[0]`, `.data[0].name` (chained access)
- `.[n:m]`, `.[:m]`, `.[n:]` (array/string slicing, negative indices from end)
- `{name, age}` (object construction, shorthand)
- `{a: .foo, b: .bar}` (object construction, rename)
- `[.foo, .bar]` (array construction)

**Control flow**
- `expr | expr` (pipe, supports multi-output on left side)
- `null`, `true`, `false`, `"string"`, `123` (literal values)
- `.field == "value"`, `!=`, `<`, `<=`, `>`, `>=` (comparison — numbers and strings)
- `expr and expr`, `expr or expr` (boolean operators, short-circuit)
- `not` (boolean negation)
- `has("key")` (true if object has the field, even if its value is null)
- `if cond then expr [elif cond then expr]* [else expr] end` (conditional; elif and else optional)
- `empty` (produce zero outputs)
- `(expr)` (parenthesized grouping)
- `select(cond)` (filter — emit input if condition truthy, nothing if falsy)
- `.foo // "default"` (alternative — use right if left is null/false)
- `.foo?`, `.[0]?`, `.[]?` (optional — suppress errors, produce nothing)
- `try expr` / `try expr catch handler` (error suppression; handler receives JSON value from `error`, or string for built-in errors)
- `error` (throw input as a `jsonError`; `catch` receives original JSON value)
- `limit(n; expr)` (first N outputs; body can be a comma-separated generator: `limit(1; a, b)`)
- `first(expr)`, `last(expr)` (first/last output of expr)

**Arithmetic**
- `expr + expr` (sum, string/array concat, object merge — right wins, left key order preserved)
- `expr - expr` (number subtraction; array difference)
- `expr * expr` (number multiplication; string × n = repeat; object * object = recursive merge)
- `expr / expr` (number division; string / string = split)
- `expr % expr` (number modulo)

**Array/collection operations**
- `map(expr)` (apply expr to every array element; desugars to `[.[] | expr]` at parse time)
- `add` (sum numbers, concat strings/arrays, merge objects)
- `flatten`, `flatten(n)` (flatten nested arrays)
- `split("s")`, `join("s")` (split/join by separator)
- `min`, `max` (array extrema)
- `min_by(f)`, `max_by(f)` (array extrema by key function)
- `any`, `any(expr)`, `any(gen; cond)` (any truthy)
- `all`, `all(expr)`, `all(gen; cond)` (all truthy)
- `first`, `last` (first/last element; one-arg forms take an expr)
- `values` (filter nulls from stream)
- `numbers`, `strings`, `arrays`, `objects`, `booleans`, `nulls`, `iterables`, `scalars` (type filters)
- `index(s)`, `rindex(s)`, `indices(s)` (first/last/all occurrences; overlapping; Unicode codepoint positions; array subsequence search)
- `has("key")`, `has(n)` (field/index existence)
- `in(obj)` (reverse membership)
- `contains(val)`, `inside(val)` (recursive containment)

**Object operations**
- `to_entries` / `from_entries` / `with_entries` (reshape)
- `keys_unsorted` (keys in insertion order; array → indices)
- `length` (string → char count, array/object → count, null → 0, number → |n|)

**Type & conversion**
- `type` (type name string)
- `tojson` / `@json`, `fromjson` (JSON string serialize/parse)
- `tostring` / `@text`, `tonumber` (string/number coercion)
- `floor`, `ceil`, `round` (numeric rounding)
- `ascii_downcase`, `ascii_upcase` (case conversion)
- `startswith("s")`, `endswith("s")`, `ltrimstr("s")`, `rtrimstr("s")` (string prefix/suffix)

**Format strings**
- `@base64`, `@base64d` (base64 encode/decode; decode JSON string content first)
- `@uri`, `@urid` (URL percent-encode/decode; decode JSON string content first)
- `@html` (HTML-escape `&<>'"`)
- `@csv`, `@tsv` (CSV/TSV formatting from array)
- `@sh` (POSIX shell quoting)

**Debugging**
- `debug` (print to stderr, pass through)

## Key Design Decisions

### 1. Scanner

Uses a simple `struct { data []byte; pos int }`. Skips values via depth-counting
(no recursion for nested objects/arrays). `readString()` returns a sub-slice of
input — zero allocation. `objectIter` and `arrayIter` provide callback-based
iteration over containers.

### 2. Multi-Output via Callback

Internal execution uses `execMulti(node, input, buf, fn)` where `fn` is called
for each result. Single-output ops call `fn` once. Iterators call `fn` per
element. Pipes propagate multi-output: if the left side produces N outputs, the
right side is invoked N times.

### 3. Deletion Algorithm

Never copies commas from input. Reconstructs `{` + kept pairs with own commas
+ `}` for objects, and `[` + kept elements with own commas + `]` for arrays.
Key-value content is copied verbatim from input (zero-copy for kept fields).

### 4. Negative Indexing

`arrayLen()` does a counting pass first (no allocation), then `arrayIter` stops
at the resolved index. Two-pass but still zero-alloc.

### 5. Object Construction (Zero-Copy)

For `{name}` shorthand, the key is known at compile time and the value is
copied from input. For `{a: .foo}`, key `a` is from the AST, value is extracted
via field access.

### 6. Output Buffer

For deletion, pre-allocate `cap = len(input)` since output is always ≤ input.
`RunWithBuffer` lets callers reuse buffers across calls for zero steady-state
allocations.

### 7. Parser Precedence Chain

Operator precedence is implemented via a function call chain:
`parsePipeExpr` → `parseExpr` → `parseAlt` → `parseOr` → `parseAnd` →
`parseCmp` → `parseAtom`. Pipe (`|`) is loosest. Then alternative (`//`),
`or`, `and`, comparison, then atoms. No precedence table needed.

### 8. Literals Store Raw JSON Bytes

Literals like `"error"`, `42`, `null` store raw JSON bytes (`[]byte`) at
compile time. At runtime: `append(buf, literal...)` — zero-alloc on hot path.

### 9. Select Zero-Output Pattern

`select(cond)` evaluates the condition; if falsy (null or false), it simply
doesn't call the callback function — producing zero outputs that propagate
naturally through pipes.

### 10. execSingle Fast Path

`execSingle` handles common single-result op types (literal, identity, field,
index, compare, type) without creating closures, avoiding heap allocations in
hot paths like `select(.field == "value")`.

### 11. Multi-Output Arithmetic Operands

`execMulti` uses `execMulti` for the left side of arithmetic operators.
This supports `.[] + 1` and similar generators as operands. The right side is
evaluated once per left output. `execPlusValues` holds core arithmetic logic,
decoupled from operand evaluation.

### 12. Object Merge Key Order

`expr + expr` on two objects: all left keys are emitted first (with right's
value for duplicates), then new right-only keys. This preserves left-object
key order even when right overrides a value.

### 13. Object Equality (Key-Order Independent)

`jsonEqual` for objects does a content comparison: for every key in A, look it
up in B and recursively compare values; also verify key counts match. This
ensures `{"a":1,"b":2} == {"b":2,"a":1}` returns `true`, matching jq semantics.

### 14. jsonError for error Propagation

The `error` builtin returns a `*jsonError{payload []byte}` instead of a Go
`fmt.Errorf`. `try-catch` handlers receive the payload directly as the input
JSON value, not a string representation. Regular built-in errors (wrong type,
division by zero, etc.) still produce string messages in catch.

### 15. Iterator Error Propagation

`execIterator` propagates `*jsonError` and `errBreak` from its callback, while
silently dropping other errors. This allows `try (.[] | error) catch .` to work
correctly while preserving the lenient multi-output behaviour (e.g.
`.[] | .foo` on a mixed array doesn't abort on non-objects).

### 16. opGenerator for Comma-Separated Bodies

`limit(n; a, b)` — where the body is a comma-separated generator — is parsed
into an `opGenerator` node with `elems []*op`. `execMulti` runs each element
in sequence, feeding all outputs to the callback. This gives `limit` the ability
to short-circuit across generator elements cleanly.

### 17. Format String JSON Decoding

All format strings that operate on string content (`@base64`, `@uri`, `@html`,
`@csv`, `@tsv`, `@sh`) first decode JSON string escape sequences
(`decodeJSONStringContent`) before encoding. This ensures `"\n"` becomes
byte `0x0a` in base64 output, not the two ASCII bytes `\` and `n`.

### 18. Containment (Zero-Alloc Parallel Scan)

`jsonContains(haystack, needle)` is implemented recursively using parallel
scanner instances for objects (no allocation for the comparison logic). For
arrays, it collects element sub-slices in a local `[][]byte` which may
allocate. For strings it's a pure byte-range scan.

### 19. add Object Deduplication

`add` on an array of objects uses last-wins deduplication with first-occurrence
key ordering. Keys appear in the order of their first occurrence across all
objects; the value used is the last one seen.

## File Structure

```
fastjq.go           — Public API: Compile, Run, RunWithBuffer, RunAll, RunFunc; BOM stripping
scanner.go          — Zero-alloc JSON scanner: skipValue, readString, objectIter, arrayIter,
                      jsonEqual (key-order independent for objects), jsonContains, isFalsy,
                      byteOffsetToCodepointOffset, compareJSONOrder
query.go            — Query parser + AST (~55 op types); parseGeneratorExpr for comma bodies
exec.go             — Executor: all op implementations; jsonError type; execPlusValues;
                      execArrayConstruct; execDeleteArray (slices); execFindIndex (overlapping,
                      Unicode codepoints, array subsequences); format string decoders
float.go            — Zero-alloc float parsing via unsafe.String
fastjq_test.go      — Unit tests (~430 tests)
correctness_test.go — Edge case, no-panic, and Unicode tests
complex_test.go     — Complex multi-step query tests
fuzz_test.go        — Fuzz tests (FuzzCompile, FuzzRunFixed, FuzzBoth)
bench_test.go       — Benchmarks: fastjq vs gojq (159 benchmarks)
jqtest/run_test.go  — Official jq test harness (jq.test + man.test, 751 total)
cmd/fastjq/main.go  — JSONL processor CLI
bench_vs_jq.sh      — CLI throughput benchmark script
scripts/update_benchmarks.go — Regenerates BENCHMARKS.md
```

## Public API

```go
// Compile once, reuse across goroutines
func Compile(query string) (*Program, error)

// Single-output (returns first result for multi-output queries)
func (p *Program) Run(input []byte) ([]byte, error)
func (p *Program) RunWithBuffer(input []byte, buf []byte) ([]byte, error)

// Multi-output
func (p *Program) RunAll(input []byte) ([][]byte, error)
func (p *Program) RunFunc(input []byte, fn func(result []byte) error) error
```

`Compile` parses once. `Run`/`RunWithBuffer` execute against raw bytes and
return the first result. `RunAll` collects all results. `RunFunc` streams
results via callback with zero steady-state allocations. All API methods strip
a UTF-8 BOM from input before parsing.
