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
- `expr | expr` (pipe)

## Key Design Decisions

### 1. Scanner

Uses a simple `struct { data []byte; pos int }`. Skips values via depth-counting (no recursion for nested objects/arrays). `readString()` returns a sub-slice of input — zero allocation.

### 2. Deletion Algorithm

Never copies commas from input. Reconstructs `{` + kept pairs with our own commas + `}`. This avoids all trailing-comma issues. Key-value *content* is copied verbatim from input (zero-copy for kept fields).

### 3. Nested Deletion

`del(.foo.bar)`: When we hit key `foo`, we recursively apply deletion of `bar` to its value. Everything outside `foo` is copied verbatim.

### 4. Output Buffer

For deletion, pre-allocate `cap = len(input)` since output is always <= input. `RunWithBuffer` lets callers reuse buffers across calls for zero steady-state allocations.

### 5. Pipe Optimization

`. | del(.foo)` is simplified to `del(.foo)` at compile time. General pipes materialize an intermediate buffer.

## File Structure

```
fastjq.go       — Public API: Compile(), Program.Run(), Program.RunWithBuffer()
scanner.go      — Zero-alloc JSON scanner (skipValue, readString, object iteration)
query.go        — Query parser + AST types
exec.go         — Executor: field access, deletion, pipe
fastjq_test.go  — Unit + integration tests
bench_test.go   — Benchmarks: fastjq vs gojq
```

## Public API

```go
func Compile(query string) (*Program, error)
func (p *Program) Run(input []byte) ([]byte, error)
func (p *Program) RunWithBuffer(input []byte, buf []byte) ([]byte, error)
```

`Compile` parses once. `Run`/`RunWithBuffer` execute against raw bytes. `RunWithBuffer` accepts a caller-owned buffer for zero steady-state allocations.
