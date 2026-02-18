# fastjq Syntax Reference

## Supported Operations

### Identity & Field Access

| Syntax | Description | Example Input | Example Output |
|--------|-------------|---------------|----------------|
| `.` | Identity — return input unchanged | `{"a":1}` | `{"a":1}` |
| `.foo` | Field access | `{"foo":1,"bar":2}` | `1` |
| `.foo.bar` | Nested field access | `{"foo":{"bar":3}}` | `3` |
| `.missing` | Missing field returns null | `{"a":1}` | `null` |

### Array Indexing

| Syntax | Description | Example Input | Example Output |
|--------|-------------|---------------|----------------|
| `.[0]` | First element | `[10,20,30]` | `10` |
| `.[2]` | Nth element | `[10,20,30]` | `30` |
| `.[-1]` | Last element (negative = from end) | `[10,20,30]` | `30` |
| `.[-2]` | Second-to-last | `[10,20,30]` | `20` |
| `.[99]` | Out-of-bounds returns null | `[10,20,30]` | `null` |

### Chained Access

| Syntax | Description | Example Input | Example Output |
|--------|-------------|---------------|----------------|
| `.items[0]` | Field then index | `{"items":["a","b"]}` | `"a"` |
| `.data[0].name` | Field, index, field | `{"data":[{"name":"alice"}]}` | `"alice"` |
| `.items[]` | Field then iterate | `{"items":[1,2,3]}` | `1`, `2`, `3` |

### Deletion

| Syntax | Description | Example Input | Example Output |
|--------|-------------|---------------|----------------|
| `del(.foo)` | Delete field | `{"foo":1,"bar":2}` | `{"bar":2}` |
| `del(.foo, .bar)` | Delete multiple fields | `{"foo":1,"bar":2,"baz":3}` | `{"baz":3}` |
| `del(.foo.bar)` | Delete nested field | `{"foo":{"bar":1,"baz":2}}` | `{"foo":{"baz":2}}` |
| `del(.[0])` | Delete array element | `[10,20,30]` | `[20,30]` |
| `del(.[1], .[3])` | Delete multiple elements | `[10,20,30,40,50]` | `[10,30,50]` |
| `del(.[-1])` | Delete last element | `[10,20,30]` | `[10,20]` |

### Iterator

| Syntax | Description | Example Input | Example Output |
|--------|-------------|---------------|----------------|
| `.[]` | All array elements | `[1,2,3]` | `1`, `2`, `3` |
| `.[]` | All object values | `{"a":1,"b":2}` | `1`, `2` |

Use `RunAll` or `RunFunc` to consume multiple outputs. `Run`/`RunWithBuffer` return the first result only.

### Object Construction

| Syntax | Description | Example Input | Example Output |
|--------|-------------|---------------|----------------|
| `{name}` | Shorthand (pick field) | `{"name":"alice","age":30}` | `{"name":"alice"}` |
| `{name, age}` | Pick multiple fields | `{"name":"alice","age":30,"x":1}` | `{"name":"alice","age":30}` |
| `{a: .foo}` | Rename field | `{"foo":1}` | `{"a":1}` |
| `{a: .foo, b: .bar}` | Rename multiple | `{"foo":1,"bar":2}` | `{"a":1,"b":2}` |
| `{city: .address.city}` | Rename with nested access | `{"address":{"city":"NYC"}}` | `{"city":"NYC"}` |

### Array Construction

| Syntax | Description | Example Input | Example Output |
|--------|-------------|---------------|----------------|
| `[.name, .age]` | Build array from fields | `{"name":"alice","age":30}` | `["alice",30]` |
| `[.name]` | Single-element array | `{"name":"alice"}` | `["alice"]` |

### Pipe

| Syntax | Description | Example Input | Example Output |
|--------|-------------|---------------|----------------|
| `.address \| del(.zip)` | Chain operations | `{"address":{"city":"NYC","zip":"10001"}}` | `{"city":"NYC"}` |
| `.items \| .[0]` | Pipe to index | `{"items":[10,20]}` | `10` |
| `.[] \| {name}` | Iterate then construct | `[{"name":"alice","age":30}]` | `{"name":"alice"}` |
| `.items[] \| .name` | Multi-output pipe | `{"items":[{"name":"a"},{"name":"b"}]}` | `"a"`, `"b"` |

Pipes propagate multi-output: if the left side produces N results, the right side runs N times.

---

## Not Yet Supported

### Feasible at zero allocation

These operations can be implemented using the existing scanner + byte-copy approach with no allocations.

| Syntax | Description | Implementation Notes |
|--------|-------------|---------------------|
| `.[2:5]`, `.[:3]`, `.[1:]` | Array/string slicing | Iterate to start, copy through end. Just position tracking. |
| `.foo?` | Optional operator (suppress errors) | Compile-time flag on the node. Return null instead of error. |
| `length` | Length of string/array/object | Count pass with scanner, write integer to buf. |
| `keys_unsorted` | Object keys (insertion order) | Iterate object, copy quoted keys into output array. |
| `values` | Object values | Equivalent to `.[]`, already supported semantically. |
| `has("foo")`, `has(0)` | Membership test | Scan for field/index, write `true`/`false`. |
| `in(expr)` | Reverse membership test | Same scan logic, reversed operands. |
| `to_entries` | Object to `[{key,value}]` array | Reformat existing bytes into output buf. |
| `from_entries` | `[{key,value}]` array to object | Scan array elements, extract key/value, reconstruct object. |
| `map(expr)` | Map over array | Equivalent to `[.[] \| expr]`. Uses existing execMulti. |
| `select(cond)` | Filter elements | Evaluate condition, emit or skip. |
| `empty` | Produce zero outputs | Don't call callback. Trivial. |
| `null`, `true`, `false` | Literal values | Write constant bytes to buf. |
| `not` | Boolean negation | Read value, write `true`/`false`. |
| `first`, `last` | First/last of iterator | `first` = take 1 from execMulti. `last` = keep final. |
| `limit(n; expr)` | Take first N results | Counter in callback. |
| `add` | Sum array elements | Parse numbers, accumulate, write result. Zero-alloc with integer math. |
| `any`, `all` | Boolean reduction | Short-circuit scan over elements. |
| `flatten` | Flatten nested arrays | Recursive scan, copy non-array elements. |
| `range(n)` | Generate 0..n-1 | Write integers via callback. |
| `type` | Type name string | Peek at first byte, write `"string"`, `"number"`, etc. |
| `ascii_downcase`, `ascii_upcase` | Case conversion | Byte-by-byte transform in output buf. |
| `ltrimstr(s)`, `rtrimstr(s)` | String trim prefix/suffix | Byte comparison, copy remainder. |
| `startswith(s)`, `endswith(s)` | String predicates | Byte comparison, write `true`/`false`. |
| `split(s)` | String split | Scan for separator, build array in output buf. |
| `join(s)` | Array join | Iterate array, interleave separator. |
| `recurse` | Recursive descent | Walk all nested values via scanner, stream via callback. |
| `path(expr)` | Output path as array | Emit path like `["foo","bar"]` or `["items",0]`. |
| `debug` | Debug print, pass through | Print to stderr, forward value unchanged. |

### Feasible but require careful handling

These operations are implementable at zero allocation but involve more complexity or edge cases.

| Syntax | Description | Challenge |
|--------|-------------|-----------|
| `if-then-else` | Conditionals | Simple comparisons (`.foo == "bar"`) are zero-alloc via byte comparison. Complex expressions may need intermediate results in the buffer. |
| `==`, `!=`, `<`, `>`, `<=`, `>=` | Comparison operators | Strings: byte compare, zero-alloc. Numbers: must parse both sides. Still zero-alloc if done in registers, but floating-point edge cases (scientific notation, large numbers) add complexity. |
| `and`, `or` | Boolean operators | Need jq truthiness rules (`null` and `false` are falsy, everything else truthy). Zero-alloc. |
| `+` (arrays) | Array concatenation | `[1] + [2]` = `[1,2]`. Strip brackets, join with comma. Simple. |
| `+` (strings) | String concatenation | Strip quotes, join, re-quote. Must handle escape sequences. |
| `+` (objects) | Object merge | `{a:1} + {b:2}` = `{a:1,b:2}`. Must handle key conflicts (last wins). Scan right for all keys, iterate left skipping overrides, then append right. Complex but zero-alloc. |
| `*` (objects) | Recursive merge | Deep merge two objects. Recursive descent and reconstruction. Zero-alloc possible but recursion depth can be problematic. |
| `try-catch` | Error handling | Capture errors from sub-expressions and redirect. The callback pattern makes this viable. |
| `as $x \| expr` | Variable binding | Store `(start, end)` offsets into original input. **Zero-alloc only if bound values reference input, not constructed output.** Binding a constructed value (e.g., `{a:1} as $x`) would need to store bytes somewhere. |
| `def f: body; expr` | Function definitions | AST-level feature, compile-time only. But closures and recursion add parser/AST complexity. |
| `reduce .[] as $x (init; update)` | Fold/accumulate | Needs mutable accumulator. If accumulator lives in the output buffer, works, but each step reads previous output. May require double-buffering (ping-pong between two buffer slices). |
| `indices(s)`, `index(s)`, `rindex(s)` | Substring search | Byte scanning is zero-alloc. Output is an array of integers. |
| `@base64`, `@base64d` | Base64 encode/decode | Can be done in-place into output buf. |
| `@uri`, `@html` | URL/HTML encoding | Character-by-character transform, write to buf. |
| `@csv`, `@tsv` | CSV/TSV formatting | Iterate array, write fields with delimiters and escaping. |
| `label-break` | Control flow | `label $out \| foreach ...` — requires unwinding callback stack. Achievable with a sentinel error value. |
| `//` (alternative operator) | Null coalescing | `.foo // "default"`. Evaluate left, if null/false evaluate right. Requires parser support for string/number literals as expressions. |
| `foreach` | Stateful iteration | `foreach .[] as $x (init; update; extract)`. Requires mutable state across iterations. Double-buffering approach keeps it zero-alloc. |
| Arithmetic (`-`, `*`, `/`, `%`) | Numeric operations | Parse JSON numbers to native types, compute, serialize back. **`strconv.AppendFloat` into buf avoids allocation.** Integer arithmetic is simpler. |
| String interpolation `\(expr)` | Embedded expressions in strings | Evaluate inner expression, embed in string. Non-string results need serialization. Adds parser complexity. |
| `getpath(path)` | Get value at path | Navigate nested structure following path array. Zero-alloc via scanner. |
| `setpath(path; val)` | Set value at path | Navigate to position, reconstruct tree with modified value. Multi-level reconstruction. Zero-alloc feasible but code complexity is high. |
| `delpaths(paths)` | Delete at multiple paths | Like `setpath` but removing. Same reconstruction complexity. |
| `walk(f)` | Recursive transform | Apply f to every value bottom-up. Reconstruct entire tree with transformed values. Intermediate results from inner expressions may need temp storage. |
| `min`, `max` | Array extrema (simple types) | Keep one "best" offset, compare each element. Zero-alloc for numbers/strings. |
| `min_by(f)`, `max_by(f)` | Array extrema by key | Must evaluate `f` per element and compare keys. Needs temp storage for "best key so far." |

### Challenging — likely require allocation

These operations fundamentally conflict with zero-allocation execution.

| Syntax | Description | Why |
|--------|-------------|-----|
| `sort`, `sort_by(expr)` | **Sorting** | Requires O(n) auxiliary storage. Must collect all element positions before reordering. An `[]int` index of offsets is the minimum allocation. For `sort_by`, must also evaluate and store the sort key for each element. |
| `group_by(expr)` | **Grouping** | Requires sort (above), then grouping into sub-arrays. Needs at least an offset index. |
| `unique`, `unique_by(expr)` | **Deduplication** | Efficient dedup requires a hash set or sorted index. O(n^2) byte comparison is zero-alloc but impractical for large arrays. |
| `keys` (sorted) | **Sorted object keys** | Must collect all key positions, sort lexicographically, then emit. Same constraint as `sort`. (`keys_unsorted` is zero-alloc and trivial.) |
| `test(re)`, `match(re)`, `capture(re)` | **Regex matching** | Go's `regexp` package allocates internally. **Cannot be made zero-alloc.** Affects `test`, `match`, `capture`, `scan`. |
| `sub(re; rep)`, `gsub(re; rep)` | **Regex substitution** | Same as above — regex engine allocates. |
| `ascii`, `implode`, `explode` | **Unicode codepoint operations** | `explode` produces an array of codepoints requiring multi-byte UTF-8 decoding. The decoding itself is zero-alloc, but the interaction with variable-width encoding adds edge cases. |
| `$ENV` | **Full environment map** | Building a JSON object from all env vars requires allocation for the result. |
| `getpath`/`setpath` (deeply nested) | **Deep path operations** | While possible at zero-alloc, deep nesting (10+ levels) requires multi-level tree reconstruction. Each level reconstructs into the output buffer. The code complexity scales linearly with max supported depth. |
| `walk(f)` (with construction) | **Recursive transform with new values** | If `f` produces constructed output at each level, intermediate results accumulate. Buffer management becomes very complex. |

### Not applicable (streaming/CLI concerns)

These are part of jq's CLI or streaming interface, not relevant for an embedded library:

| Syntax | Description |
|--------|-------------|
| `input`, `inputs` | Read from stdin |
| `env` | Access environment |
| `@json` | Re-encode as JSON string |
| `@text` | Convert to text |
| `--raw-output`, `-r` | CLI output formatting |
| `--slurp`, `-s` | CLI input mode |
| `--arg`, `--argjson` | CLI variable injection |
| `--jsonargs`, `--args` | CLI argument passing |

---

## Summary

**Most of jq's core functionality is achievable at zero allocation.** The byte-scanning + output-buffer approach covers field access, filtering, construction, iteration, and simple transforms naturally.

**Operations that require reordering** (`sort`, `group_by`, `unique`, sorted `keys`) are the hardest category. They fundamentally need an auxiliary index structure — at minimum a `[]int` of offsets. The pragmatic approach is to allow a small `[]int` allocation for these while keeping everything else zero-alloc.

**Regex is the one feature that cannot be zero-alloc in Go**, since the `regexp` package allocates internally.

**Numeric arithmetic** is feasible at zero-alloc using `strconv.AppendFloat`/`strconv.AppendInt` directly into the output buffer.
