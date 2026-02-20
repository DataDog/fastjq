# fastjq Design Document

## Overview

fastjq is a fast, minimal JQ engine for Go that avoids the expensive marshal/unmarshal cycle of gojq. It operates directly on raw `[]byte` using slice offsets — no `interface{}`, no `map[string]interface{}`.

## Architecture

### Core Idea

1. **Scanner** scans input bytes tracking positions, never copies data
2. **Query parser** compiles jq expressions into a small AST (allocates once at compile time, reused across runs)
3. **Executor** walks input with the scanner, copies only needed byte ranges into an output buffer

### Supported Operations

- `.` (identity)
- `.foo`, `.foo.bar` (field access, including nested)
- `del(.foo)`, `del(.foo, .bar)`, `del(.foo.bar)` (field deletion, including nested)
- `.[0]`, `.[-1]` (array indexing, negative = from end)
- `del(.[0])`, `del(.[1], .[3])` (array element deletion)
- `.[]` (iterator — all elements of array or values of object)
- `.items[]`, `.items[0]`, `.data[0].name` (chained access)
- `{name, age}` (object construction, shorthand)
- `{a: .foo, b: .bar}` (object construction, rename)
- `[.foo, .bar]` (array construction)
- `expr | expr` (pipe, supports multi-output on left side)
- `null`, `true`, `false`, `"string"`, `123` (literal values)
- `.field == "value"`, `.field != "value"` (equality operators)
- `.field < value`, `.field <= value`, `.field > value`, `.field >= value` (ordering operators — numbers and strings)
- `expr and expr`, `expr or expr` (boolean operators, short-circuit, always return true/false)
- `not` (boolean negation — used as `.foo | not`)
- `has("key")` (true if object has the field, even if its value is null)
- `length` (string → char count, array/object → element count, null → 0)
- `map(expr)` (apply expr to every array element; desugars to `[.[] | expr]` at parse time)
- `expr - expr` (number subtraction; array difference)
- `expr * expr` (number multiplication; string × n = repeat)
- `expr / expr` (number division; string / string = split)
- `expr % expr` (number modulo)
- `min`, `max` (array extrema — numbers by value, strings lexicographically)
- `min_by(f)`, `max_by(f)` (array extrema by key function)
- `@uri` (URL percent-encode a JSON string)
- `to_entries` (object → `[{"key":k,"value":v}]` array)
- `from_entries` (`[{"key":k,"value":v}]` → object; also accepts `"name"` as key field)
- `if cond then expr else expr end` (conditional; else is optional, defaults to identity)
- `empty` (produce zero outputs — useful as else branch to drop records)
- `(expr)` (parenthesized grouping)
- `select(.level == "error")` (filter — emit input if condition truthy, nothing if falsy)
- `.foo // "default"` (alternative — use right if left is null/false)
- `.foo?`, `.[0]?`, `.[]?` (optional — suppress errors, produce nothing)
- `type` (type name builtin — returns `"string"`, `"number"`, etc.)
- `try expr` / `try expr catch handler` (error suppression; handler receives error message as JSON string)
- `elif` branches in `if-then-elif-...-else-end` (desugars to nested if-then-else)
- `. + .` object merge (`expr + expr` on two objects; right wins for duplicate keys)
- `tojson` / `@json` (wrap value as a JSON string, escaping `"` and `\`)
- `fromjson` (parse a JSON string to its contained value; unescapes `\"` and `\\`)
- `tostring` (pass strings through unchanged; wrap non-strings with `tojson`)
- `tonumber` (numbers pass through; strings are parsed as floats)
- `any(gen; cond)` / `all(gen; cond)` (two-arg forms: generator + condition)

## Key Design Decisions

### 1. Scanner

Uses a simple `struct { data []byte; pos int }`. Skips values via depth-counting (no recursion for nested objects/arrays). `readString()` returns a sub-slice of input — zero allocation. `objectIter` and `arrayIter` provide callback-based iteration over containers.

### 2. Multi-Output via Callback

Internal execution uses `execMulti(node, input, buf, fn)` where `fn` is called for each result. Single-output ops call `fn` once. Iterators call `fn` per element. Pipes propagate multi-output: if the left side produces N outputs, the right side is invoked N times.

### 3. Deletion Algorithm

Never copies commas from input. Reconstructs `{` + kept pairs with our own commas + `}` for objects, and `[` + kept elements with own commas + `]` for arrays. This avoids all trailing-comma issues. Key-value *content* is copied verbatim from input (zero-copy for kept fields).

### 4. Nested Deletion

`del(.foo.bar)`: When we hit key `foo`, we recursively apply deletion of `bar` to its value. Everything outside `foo` is copied verbatim.

### 5. Negative Indexing

`arrayLen()` does a counting pass first (no allocation), then `arrayIter` stops at the resolved index. Two-pass but still zero-alloc.

### 6. Object Construction (Zero-Copy)

For `{name}` shorthand, the key is known at compile time and the value is copied from input. For `{a: .foo}`, key `a` is from the AST, value is extracted via field access.

### 7. Output Buffer

For deletion, pre-allocate `cap = len(input)` since output is always <= input. `RunWithBuffer` lets callers reuse buffers across calls for zero steady-state allocations.

### 8. Pipe Optimization

`. | del(.foo)` is simplified to `del(.foo)` at compile time. General pipes materialize an intermediate buffer.

### 9. Parser Precedence Chain

Operator precedence is implemented via a function call chain: `parseExpr` → `parseAlt` → `parseCmp` → `parseAddExpr` → `parseMulExpr` → `parseAtom`. Pipe (`|`) is the loosest, handled in `parsePipeExpr`. Then alternative (`//`), then `or`, then `and`, then comparison (`==`, `!=`, `<`, etc.), then `+`/`-`, then `*`/`/`/`%`, then atoms. Clean, extensible, no precedence table needed.

### 10. Literals Store Raw JSON Bytes

Literals like `"error"`, `42`, `null` store raw JSON bytes (`[]byte`) at compile time. At runtime, `append(buf, literal...)` — zero-alloc on hot path.

### 11. Select Zero-Output Pattern

`select(cond)` evaluates the condition; if falsy (null or false), it simply doesn't call the callback function — producing zero outputs that propagate naturally through pipes.

### 12. execSingle Fast Path

`execSingle` handles common single-result op types (literal, identity, field, index, compare, type) without creating closures, avoiding heap allocations in hot paths like `select(.field == "value")`.

## File Structure

```
fastjq.go                  — Public API: Compile(), Run(), RunWithBuffer(), RunAll(), RunFunc()
scanner.go                 — Zero-alloc JSON scanner (skipValue, readString, object/array iteration, jsonEqual, isFalsy)
query.go                   — Query parser + AST types (18 op types: adds opTry, opToJSON, opFromJSON, opToString, opToNumber)
exec.go                    — Executor: field access, indexing, deletion, iteration, construction, pipe, compare, select, alternative, type
float.go                   — Zero-alloc float parsing via unsafe.String
fastjq_test.go             — Unit + integration tests (97 tests)
bench_test.go              — Benchmarks: fastjq vs gojq (28 benchmarks)
cmd/fastjq/main.go         — JSONL processor CLI (usage: `fastjq 'query' < input.jsonl`)
bench_vs_jq.sh             — Shell script: generates test data, builds CLI, runs JSONL throughput benchmarks vs jq
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

`Compile` parses once. `Run`/`RunWithBuffer` execute against raw bytes and return the first result. `RunAll` collects all results. `RunFunc` streams results via callback with zero steady-state allocations. `RunWithBuffer` accepts a caller-owned buffer for zero steady-state allocations.
