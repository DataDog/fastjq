# fastjq Project Constraints

## Safety Constraints

- **Never panics.** fastjq must never panic regardless of input — valid JSON, malformed JSON, empty input, or arbitrary bytes. A panic in a log processing pipeline is worse than wrong output. This guarantee is enforced by `TestNoPanicMalformedInput` (deterministic) and three fuzz test functions (`FuzzCompile`, `FuzzRunFixed`, `FuzzBoth`). Any code path that could panic on bad input is a bug.
- **Correct results only guaranteed for valid JSON.** With malformed input the output is undefined, but the process remains safe.

## Performance Model

The governing principle: **allocations are proportional to what you ask for, never to what the engine scans.**

fastjq distinguishes four tiers of allocation behaviour:

| Tier | Rule | Examples |
|------|------|---------|
| **Tier 0** | 0 allocs — core processing hot path | access, filter, compare, arithmetic, construction, math |
| **Tier 1** | Allocs ∝ output size — unavoidable API constraints | `@base64`, `@uri`, `match`, `capture`, `scan`, `gsub` |
| **Tier 2** | Allocs ∝ output count — bounded by what was requested | `range(n)` (1 alloc/value); `sort`, `group_by`, `unique` *(planned)* |
| **Tier 3** | Deferred — broader runtime integration outside the pure library core | full decimal semantics / module-import helpers / host-boundary helpers |

When deciding whether to implement a new operation, the question is always: *does the allocation scale with the result, or with the input?* If the caller can control the allocation by choosing what to request, it is acceptable. If the allocation scales with the shape of the data being processed regardless of the query, it is rejected.

### Tier 0 — Zero-alloc (core processing)

Field access, filtering, comparison, arithmetic, construction, `map(.field)`, math builtins, `test(re)`, string operations, and most other operations achieve **0 allocs/op** at steady state when using `RunWithBuffer` or `RunFunc` with a reused buffer.

This is the hot path for log processing. Any regression to non-zero allocs on these operations is a bug.

Implementation rules:
- `RunWithBuffer` / `RunFunc` must achieve 0 allocs for all Tier 0 operations
- Static error sentinels (`errors.New`) on hot-path functions — `fmt.Errorf` with dynamic args poisons escape analysis
- No `interface{}`, `map[string]interface{}`, or any Go type conversion — operate entirely on raw `[]byte`
- No data copying except into the output buffer

### Tier 1 — Alloc ∝ output size (unavoidable)

Some operations must allocate to produce their result. The allocation is proportional to the *output*, not the input being scanned. Even gojq allocates thousands of times just to begin; these ops allocate a handful of times for specific results.

| Operation | Allocs | Reason |
|-----------|--------|--------|
| `@base64`, `@uri`, `@format "...\(...)"` | ~4 | String-escape decoding before encoding |
| `match(re)`, `capture(re)` | 1 on hit, 0 on miss | Go's `FindSubmatchIndex` `[]int` (unavoidable API) |
| `scan(re)`, `gsub(re)` | ∝ match count | Multi-result output |
| `sub(re)` | 1 on hit | `FindIndex []int` |
| `map(f)` when `f` constructs | ~1 per element | `execArrayConstruct` aliasing constraint |
| `error(expr)` | 1 on throw | `jsonError` payload copy (exceptional path only) |

**Exception — array construction aliasing:** `[.[] | f]` where `f` constructs new data (object construction, arithmetic, string concatenation) allocates ~1 buffer per element. This is a structural limitation: `execArrayConstruct` must pass `nil` scratch to `execMulti` to prevent aliasing when an element's iterator emits multiple results across callback invocations. Elements that return input sub-slices (field access, identity, comparisons) remain 0 allocs.

### Tier 2 — Alloc ∝ output count (bounded, acceptable)

Operations where allocation is proportional to *what the caller asked to produce*, not what was scanned. The caller controls the allocation by deciding what to request.

| Operation | Allocs | Reason |
|-----------|--------|--------|
| `range(n)` | 1 per value | Each generated integer is a fresh byte slice. For `range(10)`: 10 allocs. |
| `range(from;to;step)` | 1 per value | Same model with explicit bounds and step. |
| `sort` | ~n+O(log n) | Collect element sub-slices into index, sort, re-emit. No element copies. |
| `sort_by(f)` | ~3n | Collect elements + compute keys + sort index. Keys with nil buf are sub-slices (no copy); constructed keys allocate their own buffers. |
| `unique`, `unique_by(f)` | ~n | Same as sort, then deduplicate consecutive equal keys. |
| `group_by(f)` | ~3n | Same as sort_by; groups consecutive elements with equal keys. |
| `transpose` | ~n×m | Build transposed matrix: n rows × m columns, each element copied once. |
| `.[]` == `.[]` multi-output compare | 0 | true/false are static literals; operands use nil-buf path. |
| `{a: .x[]}` object construction | ~n per multi-output pair | Cartesian product: one prefix copy per value per level. |

Rule: Tier 2 operations must document their alloc model in CHANGELOG and SYNTAX.md. They must not appear in hot-path benchmarks that assert 0 allocs.

### Tier 3 — Deferred (requires executor redesign)

These operations are not rejected on principle — they're deferred because implementing them correctly requires a different execution architecture, not just documented allocations.

| Operation | Why deferred |
|-----------|-------------|
| full `have_decnum` semantics | Requires an exact decimal runtime instead of the current float-based executor. This is a semantic/runtime redesign, not an isolated builtin. |
| module/import syntax, `modulemeta` | Requires filesystem-backed resolution, dependency loading, and module scoping beyond the current pure `Compile` + `Run` library contract. |
| `input`, `inputs`, `env`, `$ENV`, `stderr` | These cross the host boundary by reading stdin, environment variables, or process stderr rather than operating only on the provided JSON bytes. |

## Scope Constraints

- **Supported operations**: identity (`.`), quoted field access (`."foo"`), field access (`.foo`, `.foo.bar`), array indexing (`.[0]`, `.[-1]`, `.[i,j]`), slicing (`.[n:m]`, `.[:m]`, `.[n:]`, `.[n:m]?`), deletion (`del(.foo)`, `del(.[0])`, `del(.[i,j])`, `del(.[n:m])`, `del((paths) | ...)`, `delpaths(paths)`), iteration (`.[]`), recursive descent (`..`), `recurse`, `recurse(f)`, `recurse(f; cond)`, `walk(f)`, object construction (`{name}`, `{a: .foo}`, `{a: .items[]}`), array construction (`[.foo, .bar]`), variable binding (`expr as $x | body`, `. as [$a, $b] | ...`, `. as {$a, b: [$c]} | ...`, `?//` alternation between binding patterns), user-defined functions (`def f: body; expr`, `def f(args...): body; f(...)`), `reduce gen as $x (init; update)`, `foreach gen as $x (init; update; extract?)`, `while(cond; update)`, `until(cond; next)`, `label $name | body`, `break $name`, `map(expr)`, `map_values(expr)`, `with_entries(expr)`, `add`, `expr + expr` (including object merge), `expr - expr`, unary negation (`-expr`), `expr * expr` (including `string * n`, `n * string`, `object * object` recursive merge), `expr / expr`, `expr % expr`, `flatten`/`flatten(n)`, `split("s")`, `join("s")`, `repeat(expr)`, `range(n)`/`range(from;to;step)`, `min`/`max`, `min_by(f)`/`max_by(f)`, `sort`, `sort_by(f)`, `unique`, `unique_by(f)`, `group_by(f)`, `transpose`, `reverse`, `to_entries`, `from_entries`, `keys`, `keys_unsorted`, `paths`, `paths(filter)`, `path(expr)`, `pick(path, ...)`, `getpath(path)`, `setpath(path; value)`, `tostream`, `truncate_stream(stream)`, `fromstream(stream)`, `bsearch(x)`, `INDEX(gen; key)`, `JOIN(index; key)`, `IN(gen)`/`IN(lhs; rhs)`, `any`/`any(expr)`/`any(gen; cond)`, `all`/`all(expr)`/`all(gen; cond)`, `first`/`first(expr)`, `last`/`last(expr)`, `limit(n; expr)`, `skip(n; expr)`, `nth(n; gen)`, `isempty(expr)`, string interpolation (`"\(expr)"`), format templates (for example `@html "<b>\(.)</b>"`), pipe (`expr | expr`), grouping (`(expr)`), literals (`null`, `true`, `false`, `"string"`, `123`, `nan`, `infinite`, `-nan`, `-infinite`), comparison (`==`, `!=`, `<`, `<=`, `>`, `>=`) with multi-output support, boolean (`and`, `or`, `not`), `has("key")`, `length`, `abs`, `ascii_downcase`, `ascii_upcase`, `startswith("s")`, `endswith("s")`, `trim`, `ltrim`, `rtrim`, `trimstr("s")`, `ltrimstr("s")`, `rtrimstr("s")`, `utf8bytelength`, `if-then-elif-...-else-end`, `empty`, select (`select(cond)`), alternative (`//`), optional (`.foo?`, `expr?`), assignment/update (`=`, `|=`, `//=`, `+=`, `-=`, `*=`, `/=`, `%=`), type (`type`), `try`/`try-catch`, `error`/`error(expr)`, `tojson`/`@json`, `fromjson`, `tostring`/`@text`, `tonumber`, `toboolean`, `have_decnum`, `builtins`, `$__loc__`, `@base64`, `@base64d`, `@uri`, `@urid`, `@html`, `@csv`, `@tsv`, `@sh`, `floor`, `ceil`, `round`, `nearbyint`, `contains(val)`, `inside(val)`, `index(s)`, `rindex(s)`, `indices(s)`, `explode`, `implode`, `sqrt`, `fabs`, `atan` (1-arg), `pow(x; y)`, `log`, `log2`, `log10`, `exp`, `exp2`, `exp10`, `cbrt`, `logb`, `sin`, `cos`, `tan`, `asin`, `acos`, `tgamma`, `lgamma`, `j0`, `j1`, `isnan`, `isinfinite`, `isfinite`, `isnormal`, `test(re)`/`test(re; flags)`, `match(re)`/`match(re; flags)`, `capture(re)`/`capture(re; flags)`, `scan(re)`/`scan(re; flags)`, `sub(re; s)`, `gsub(re; s)`
- **Input format**: valid JSON objects or arrays — no streaming, no JSONL
- **No validation**: assumes well-formed JSON input; behavior on malformed input is undefined
- **No pretty-printing**: output is compact JSON only, with canonical string escaping on emitted strings
- **`paths`, `path(expr)`, `..`, `recurse`, and `walk` are parity-first exceptions on `codex/jq-parity`**: these implementations unlock official jq-suite coverage even though they are not held to the usual zero-alloc target on this branch. `paths` and `path(expr)` allocate on the path-building call path, `..` currently benchmarks at `7 allocs/op`, `recurse` at `5 allocs/op`, and `walk(.)` at `12 allocs/op` on the focused small benchmarks, all still well below gojq for the same queries.
- **`select` condition still uses the single-result fast path**: `execSelect` evaluates the condition via `execSingle`, so conditions that produce multiple values still test only the first output. This does not affect the upstream jq files we currently run, but it remains a real semantic gap versus full jq.
- **`if-then-else` condition supports multi-output**: `execIf` uses `execMulti` for the condition, so generators like `if empty then x end` correctly produce zero outputs. Single-output conditions take a fast path to avoid closure allocation.
- **`del` paths must be literal field, index, or slice expressions**: `del(.foo)`, `del(.foo.bar)`, `del(.[0])`, `del(.foo, .bar)`, `del(.[n:m])`, and `del(.[-n:])` are supported. Dynamic paths are not: `del(.items[])` and `del(.items[] | select(...))` both return an error. There is no way to delete multiple elements matched by a runtime condition.
- **Numeric slice bounds are expression-driven but still local**: `.[1.2:3.5]`, `.[:rindex("x")]`, and chained forms like `.[3:3][1:]` are supported because bounds are evaluated once and clamped. This does not widen indexing into general dynamic path semantics.

## Design Constraints

- **Scanner is stateless between runs**: `struct { data []byte; pos int }` reset per call
- **AST allocates once at compile time**: `Compile()` allocates, `Run()` does not (with buffer reuse, for Tier 0 ops)
- **Comma reconstruction**: deletion never copies commas from input; reconstructs containers with own commas to avoid trailing-comma bugs
- **String comparison without allocation**: `bytesEqualStr` compares `[]byte` keys to `string` field names without converting; `findFieldStr` avoids `[]byte(string)` conversion at call sites
- **Multi-output via callback**: `execMulti` uses `func([]byte) error` callback to avoid allocating result slices internally
- **Negative indexing is two-pass**: `arrayLen()` counts first (no alloc), then iterates to resolved index
- **Precedence via function chain**: `parsePipeExpr` → `parseExpr` → `parseAlt` → `parseOr` → `parseAnd` → `parseCmp` → `parseAtom` — no precedence table
- **Generator contexts use jq-style array precedence**: inside array construction and generator bodies, commas bind tighter than pipes so `[a, b | f]` parses as `[(a, b) | f]`
- **`//` operator collects all truthy left outputs**: `execAlternative` uses `execMulti` for left side; if any output is truthy, it passes through; only if ALL outputs are falsy does right side run
- **Literals store raw JSON bytes**: compiled at parse time, zero-alloc at runtime
- **isFalsy by first-byte check**: `n` = null, `f` = false — one branch, zero alloc
- **Number comparison**: byte-identical fast path (zero-alloc), `parseFloat` slow path using `unsafe.String` to avoid string allocation
- **Optional uses two representations**: direct field/index/iterator forms keep `node.optional = true` for the hot path, while general postfix `expr?` compiles to a wrapper op so jq-style transactional error suppression works across whole subexpressions
- **`try` propagates `errBreak`**: `errBreak` is a control signal (for `first`/`limit`), not an error — `opTry` in `execMulti` propagates it unchanged
- **`try` is scoped to its body**: downstream callback-chain errors are propagated past the inner `try` instead of being caught as if they originated in the body
- **`exec` routes through `execSingle`**: `execSingle` handles all Tier 0 ops with direct return paths (no closures). Multi-output and Tier 1+ ops fall back to `execFirstResult` which uses the closure-based `execMulti` machinery.
- **`elif` desugars at parse time**: `elif C then X` rewrites to `else (if C then X end)` — no new op type needed
- **Regex patterns compiled at parse time**: `node.re *regexp.Regexp` holds the compiled RE2 pattern; Go RE2 guarantees linear-time matching (immune to ReDoS)
- **`findFieldStr` stops on match**: field lookups stop scanning as soon as the target key is found — no wasted scan of remaining fields
- **`compareJSONOrder` follows jq type ordering**: null < false < true < numbers < strings < arrays < objects. Object comparison collects and sorts key-value pairs (allocates; acceptable for Tier 2 sort paths). This ordering drives all sort/min/max operations.
- **`hasMultiOutput` compile-time detection**: object construction (`opConstruct`) checks all pair expressions at compile time. Single-output pairs use `execConstruct` (zero-alloc fast path); multi-output pairs use `execConstructMulti` (Cartesian product).
- **Sort uses index-based sorting**: `sort_by`/`group_by`/`unique_by` precompute keys then sort a `[]int` index to avoid mismatching precomputed keys with reordered elements.

## Testing Constraints

- All operations must be covered by unit tests
- Benchmarks must compare against gojq on small (~100B), medium (~2KB), and large (~100KB+) JSON
- Benchmarks report ns/op, B/op, and allocs/op — **any unexpected non-zero allocs/op on a Tier 0 operation is a bug**
- Tier 1+ operations must document their alloc profile in the benchmark comment
- CLI throughput benchmarks compare against jq CLI on JSONL streams via `bench_vs_jq.sh`
