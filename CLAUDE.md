# Working on fastjq

## Before you start

Read these files to understand the project before making any changes:
- `DESIGN.md` — architecture, supported operations, key design decisions
- `CONSTRAINTS.md` — performance and scope constraints; what the library will and won't do
- `SYNTAX.md` — full operation reference with examples, and the roadmap of unimplemented ops

## Workflow

### 1. Run tests first
Before touching anything, confirm the baseline is clean:
```bash
go test -v -count=1
```

### 2. Make your changes

### 3. Run tests again — all must pass
```bash
go test -v -count=1
```
No regressions. Every new feature needs tests. If something is hard to test, that's a signal the design needs reconsideration.

### 4. Run benchmarks
```bash
go test -bench=. -benchmem -count=1
```
Check that no existing benchmark has regressed meaningfully. For new features that are hot-path, add a benchmark.

### 5. Update ALL the docs
Every code change that affects the public surface, supported operations, or performance characteristics must update:

| File | What to update |
|------|----------------|
| `README.md` | Supported operations table, benchmark tables if numbers changed |
| `DESIGN.md` | Supported operations list, file structure, new design decisions |
| `CONSTRAINTS.md` | Scope constraints (supported ops), any new design constraints |
| `SYNTAX.md` | Add new operations with examples; move from "Not Yet Supported" to supported |
| `BENCHMARKS.md` | Re-run benchmarks and update the summary table and raw output if numbers changed |

### 6. Ask before committing
**Do not commit automatically.** Show the user what you've done and ask if they want to commit. Let them review first.

## Design constraints — don't regress these

These are non-negotiable. Any change that violates them needs explicit discussion first:

- **Zero allocations on the hot path.** `RunWithBuffer` and `RunFunc` must achieve 0 allocs/op at steady state for all supported operations. Never introduce allocations inside `execMulti`, `execSingle`, or any scanner function.
- **No marshal/unmarshal.** Never convert input to `interface{}`, `map[string]interface{}`, or any Go type. Operate on raw `[]byte` only.
- **No data copying except into the output buffer.** The scanner returns sub-slices of input. Field values are copied verbatim. Nothing else.
- **Compile once, run many times.** `Compile` may allocate. `Run`/`RunWithBuffer`/`RunFunc` must not (with reused buffer).
- **Scope is intentionally limited.** fastjq is not a full jq implementation. Before adding an operation, check `SYNTAX.md` — if it's in the "Not Yet Supported" section, check the notes there. If it's "Challenging — likely require allocation", discuss with the user first.
- **Output is always compact JSON.** No pretty-printing.

## Key files

```
fastjq.go                  — Public API
scanner.go                 — Zero-alloc JSON scanner
query.go                   — Parser + AST (16 op types)
exec.go                    — Executor
float.go                   — Zero-alloc float parsing
fastjq_test.go             — Unit tests (158 tests)
bench_test.go              — Benchmarks (fastjq vs gojq)
cmd/fastjq-bench/main.go  — JSONL CLI for benchmarking vs jq
bench_vs_jq.sh             — CLI throughput benchmark script
```

## Benchmark reliability notes

Large-input gojq benchmarks use rotating input copies (8 distinct instances) to prevent a Go 1.25 calibration artifact where auto-calibration produces incorrect results. The Large Select benchmark uses `field_199` (last field) to prevent early-exit skewing the comparison. If you add new Large benchmarks, apply the same pattern — see existing `BenchmarkGojq_Large_Del` for reference.
