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

### 4. Run benchmarks and update BENCHMARKS.md
```bash
go run scripts/update_benchmarks.go
```
This runs the full benchmark suite (~3 minutes) and **automatically regenerates both the summary comparison table and raw output section** of `BENCHMARKS.md`. Never edit `BENCHMARKS.md` by hand — always use this script.

**Every new operation must have a fastjq benchmark AND a matching gojq benchmark.** Add them to `bench_test.go` following the existing helper pattern (`benchFastjqObj`, `benchGojqObj`, etc.) before committing. Also add a corresponding `row{}` entry to `tableRows` in `scripts/update_benchmarks.go` so it appears in the summary table.

**Any non-zero `allocs/op` in a fastjq benchmark is a failure**, with one documented structural exception (see below). Stop and check in with the user before proceeding. The options are:
- Fix the allocation (preferred)
- Reject the feature as incompatible with the zero-alloc constraint
- Document it explicitly as a known edge case. Only accept if the alloc is a single fixed-size buffer recycled by the allocator. Reject if allocs scale with input structure (see `range` and `recurse` in SYNTAX.md Rejected section).

**Known structural exception — array construction with data-building elements:**
`[.[] | f]` / `map(f)` where `f` constructs new data (object `{…}`, arithmetic, string concat) allocates ~1 buffer per element. Root cause: `execArrayConstruct` must pass `nil` scratch to `execMulti` to prevent aliasing — an iterator calls the callback multiple times, and if all invocations share the same scratch position they overwrite each other. Elements that return input sub-slices (field access, identity) are still 0 allocs. See `CONSTRAINTS.md` and the `execArrayConstruct` comment for the full explanation. fastjq uses 5–8x fewer allocs than gojq on these queries even with this limitation.

### 5. Update ALL the docs
Every code change that affects the public surface, supported operations, or performance characteristics must update:

| File | What to update |
|------|----------------|
| `README.md` | Supported operations table, benchmark tables if numbers changed |
| `DESIGN.md` | Supported operations list, file structure, new design decisions |
| `CONSTRAINTS.md` | Scope constraints (supported ops), any new design constraints |
| `SYNTAX.md` | Add new operations with examples; move from "Not Yet Supported" to supported |
| `BENCHMARKS.md` | Run `go run scripts/update_benchmarks.go` — regenerates summary table + raw output automatically |
| `bench_test.go` | Add fastjq + gojq benchmarks for every new operation (required, not optional) |
| `CHANGELOG.md` | Add an entry for every meaningful change (see format below) |

#### CHANGELOG format
Add a new `## [Unreleased] — short description` section at the top. Include:
- **Added** — new operations or APIs
- **Fixed** — bug fixes or correctness improvements
- **Tradeoffs** — any design decisions with non-obvious consequences
- **Benchmark results** — if numbers changed significantly (>10%), note the before/after. Always note if a benchmark regressed.

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
fastjq.go           — Public API
scanner.go          — Zero-alloc JSON scanner
query.go            — Parser + AST (39 op types)
exec.go             — Executor
float.go            — Zero-alloc float parsing
fastjq_test.go      — Unit tests (397 tests)
correctness_test.go — Edge case, no-panic, and unicode tests
fuzz_test.go        — Fuzz tests (FuzzCompile, FuzzRunFixed, FuzzBoth)
bench_test.go       — Benchmarks (fastjq vs gojq) — ADD NEW OPS HERE
cmd/fastjq/main.go  — JSONL CLI (`fastjq 'query' < input.jsonl`)
bench_vs_jq.sh      — CLI throughput benchmark script
```

## Benchmark reliability notes

Large-input gojq benchmarks use rotating input copies (8 distinct instances) to prevent a Go 1.25 calibration artifact where auto-calibration produces incorrect results. The Large Select benchmark uses `field_199` (last field) to prevent early-exit skewing the comparison. If you add new Large benchmarks, apply the same pattern — see existing `BenchmarkGojq_Large_Del` for reference.
