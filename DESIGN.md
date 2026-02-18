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

## File Structure

```
fastjq.go       — Public API: Compile(), Run(), RunWithBuffer(), RunAll(), RunFunc()
scanner.go      — Zero-alloc JSON scanner (skipValue, readString, object/array iteration)
query.go        — Query parser + AST types (8 op types)
exec.go         — Executor: field access, indexing, deletion, iteration, construction, pipe
fastjq_test.go  — Unit + integration tests (56 tests)
bench_test.go   — Benchmarks: fastjq vs gojq (22 benchmarks)
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
