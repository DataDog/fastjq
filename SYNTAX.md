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
| `{status: "ok"}` | Literal value | `{}` | `{"status":"ok"}` |
| `{name: .name // "anon"}` | With alternative | `{}` | `{"name":"anon"}` |

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

### Literals

| Syntax | Description | Example Output |
|--------|-------------|----------------|
| `null` | Null literal | `null` |
| `true` | Boolean true | `true` |
| `false` | Boolean false | `false` |
| `"hello"` | String literal | `"hello"` |
| `42` | Integer literal | `42` |
| `3.14` | Float literal | `3.14` |
| `-5` | Negative number | `-5` |

Literals are constant values independent of input.

### Comparison Operators

| Syntax | Description | Example Input | Example Output |
|--------|-------------|---------------|----------------|
| `.name == "alice"` | String equality | `{"name":"alice"}` | `true` |
| `.age != 30` | Not equal | `{"age":25}` | `true` |
| `.x == null` | Compare to null | `{"y":1}` | `true` |
| `.x == .y` | Compare two fields | `{"x":1,"y":1}` | `true` |
| `1.0 == 1` | Number comparison (float path) | any | `true` |

Non-associative: `a == b == c` is a parse error.

### Select (Filter)

| Syntax | Description | Example Input | Example Output |
|--------|-------------|---------------|----------------|
| `select(.level == "error")` | Keep if condition is truthy | `{"level":"error","msg":"boom"}` | `{"level":"error","msg":"boom"}` |
| `select(.level == "error")` | Filter out if condition is falsy | `{"level":"info"}` | *(no output)* |
| `select(true)` | Always pass through | `42` | `42` |
| `select(false)` | Always filter out | `42` | *(no output)* |
| `.[] \| select(.active == true)` | Filter array elements | `[{"active":true},{"active":false}]` | `{"active":true}` |

`select` produces zero outputs when the condition is falsy (null or false). This propagates naturally through pipes.

### Alternative Operator (`//`)

| Syntax | Description | Example Input | Example Output |
|--------|-------------|---------------|----------------|
| `.foo // "default"` | Use right if left is null/false | `{"bar":1}` | `"default"` |
| `null // "x"` | Null triggers alternative | any | `"x"` |
| `false // "x"` | False triggers alternative | any | `"x"` |
| `.foo // .bar` | Fallback to another field | `{"bar":"ok"}` | `"ok"` |
| `.a // .b // .c` | Chained alternatives | `{"c":"found"}` | `"found"` |
| `.foo // "default"` | Left exists — no fallback | `{"foo":"hi"}` | `"hi"` |

Left-associative. Only triggers on null or false (not on empty string, zero, etc.).

### Optional Operator (`?`)

| Syntax | Description | Example Input | Example Output |
|--------|-------------|---------------|----------------|
| `.foo?` | Suppress error on non-object | `"string"` | *(no output)* |
| `.[0]?` | Suppress error on non-array | `{"a":1}` | *(no output)* |
| `.[]?` | Suppress error on non-iterable | `42` | *(no output)* |
| `.foo?` | Normal case — works as usual | `{"foo":"bar"}` | `"bar"` |
| `.foo?.bar` | Optional field with chain | `"string"` | *(no output)* |

Produces zero outputs (instead of an error) when the input type doesn't match.

### Type Builtin

| Syntax | Description | Example Input | Example Output |
|--------|-------------|---------------|----------------|
| `type` | String type name | `"hello"` | `"string"` |
| `type` | Number type name | `42` | `"number"` |
| `type` | Object type name | `{"a":1}` | `"object"` |
| `type` | Array type name | `[1,2]` | `"array"` |
| `type` | Boolean type name | `true` | `"boolean"` |
| `type` | Null type name | `null` | `"null"` |
| `.value \| type` | Piped type check | `{"value":"hi"}` | `"string"` |
| `.[] \| select(type == "object")` | Filter by type | `[1,"a",{"x":1}]` | `{"x":1}` |

---

### Boolean Operators

| Syntax | Description | Example Input | Example Output |
|--------|-------------|---------------|----------------|
| `expr and expr` | True if both truthy (short-circuits) | `{"a":1,"b":2}` | `true` |
| `expr or expr` | True if either truthy (short-circuits) | `{"a":null,"b":1}` | `true` |
| `expr \| not` | Boolean negation | `false` | `true` |

Only `null` and `false` are falsy. `0`, `""`, `[]`, `{}` are all truthy.

### Comparison Operators

| Syntax | Description | Example Input | Example Output |
|--------|-------------|---------------|----------------|
| `.foo == "val"` | Equality | `{"foo":"val"}` | `true` |
| `.foo != "val"` | Not equal | `{"foo":"other"}` | `true` |
| `.n < 5` | Less than (numbers or strings) | `{"n":3}` | `true` |
| `.n <= 5` | Less than or equal | `{"n":5}` | `true` |
| `.n > 5` | Greater than | `{"n":7}` | `true` |
| `.n >= 5` | Greater than or equal | `{"n":5}` | `true` |

Ordering works on numbers (float comparison) and strings (lexicographic). Cross-type comparisons return `false`.

### Conditionals

| Syntax | Description | Example Input | Example Output |
|--------|-------------|---------------|----------------|
| `if .f == "x" then .a else .b end` | Conditional | `{"f":"x","a":1,"b":2}` | `1` |
| `if .f == "x" then .a end` | Without else — defaults to identity | `{"f":"y"}` | `{"f":"y"}` |
| `if C then A elif C2 then B else D end` | Not supported — nest manually | — | — |

### Membership & Length

| Syntax | Description | Example Input | Example Output |
|--------|-------------|---------------|----------------|
| `has("key")` | True if object has field (even if null) | `{"x":null}` | `true` |
| `length` | String → chars, array/object → count, null → 0 | `[1,2,3]` | `3` |
| `keys_unsorted` | Object keys in insertion order; array → indices | `{"b":1,"a":2}` | `["b","a"]` |

### Reduction and Generation

| Syntax | Description | Example Input | Example Output |
|--------|-------------|---------------|----------------|
| `add` | Sum numbers, concatenate strings/arrays, merge objects | `[1,2,3]` | `6` |
| `add` | String concatenation | `["a","b","c"]` | `"abc"` |
| `add` | Array concatenation | `[[1,2],[3,4]]` | `[1,2,3,4]` |
| `add` | Empty/null array → null | `[]` | `null` |
| `range(n)` | Emit 0, 1, …, n-1 | any (with `[range(3)]`) | `[0,1,2]` |
| `range(from; to)` | Emit from, …, to-1 | any (with `[range(2;5)]`) | `[2,3,4]` |
| `range(from; to; step)` | Emit with step | any (with `[range(0;10;3)]`) | `[0,3,6,9]` |
| `flatten` | Fully flatten nested arrays | `[[1,[2]],3]` | `[1,2,3]` |
| `flatten(n)` | Flatten at most n levels | `[[1,[2]],3]` with `flatten(1)` | `[1,[2],3]` |
| `split("s")` | Split string by separator | `"a,b,c"` with `split(",")` | `["a","b","c"]` |
| `join("s")` | Join array with separator | `["a","b","c"]` with `join(",")` | `"a,b,c"` |

`range` is a generator — use `[range(n)]` to collect into an array.
`join` converts numbers to their string representation; nulls become empty strings.

### Stream Control

| Syntax | Description | Example Input | Example Output |
|--------|-------------|---------------|----------------|
| `first` | First element (no-arg: `.[0]`) | `[10,20,30]` | `10` |
| `last` | Last element (no-arg: `.[-1]`) | `[10,20,30]` | `30` |
| `first(expr)` | First output of expr | `[1,2,3,4,5]` (with `first(.[] \| select(. > 2))`) | `3` |
| `last(expr)` | Last output of expr | `[1,2,3,4,5]` (with `last(.[] \| select(. > 2))`) | `5` |
| `limit(n; expr)` | First N outputs of expr as a stream | `[1,2,3,4,5]` (with `limit(3; .[])`) | `1`, `2`, `3` |

`limit` emits a stream, not an array. Wrap in `[...]` if you need an array: `[limit(3; .[])]`.
`any(generator; cond)` two-arg form is not supported.

### Boolean Reduction

| Syntax | Description | Example Input | Example Output |
|--------|-------------|---------------|----------------|
| `any` | True if any element is truthy | `[false,1,false]` | `true` |
| `all` | True if all elements are truthy (vacuously true for `[]`) | `[1,"x",true]` | `true` |
| `any(expr)` | True if expr is truthy for any element | `[1,2,3]` | — |
| `all(expr)` | True if expr is truthy for all elements | `[1,2,3]` | — |

`any(generator; cond)` two-arg form is not supported.

### String Operations

| Syntax | Description | Example Input | Example Output |
|--------|-------------|---------------|----------------|
| `ascii_downcase` | Convert string to lowercase | `"Hello"` | `"hello"` |
| `ascii_upcase` | Convert string to uppercase | `"hello"` | `"HELLO"` |
| `startswith("s")` | True if string starts with s | `"foobar"` | `true` |
| `endswith("s")` | True if string ends with s | `"foobar"` | `false` (for `"foo"`) |
| `ltrimstr("s")` | Remove prefix if present | `"prod-auth"` | `"auth"` |
| `rtrimstr("s")` | Remove suffix if present | `"app.log"` | `"app"` |

Escape sequences in strings are preserved by case conversion. All string operations work on raw JSON string bytes — no Unicode-aware processing.

### Object Transforms

| Syntax | Description | Example Input | Example Output |
|--------|-------------|---------------|----------------|
| `to_entries` | Object → `[{"key":k,"value":v}]` | `{"a":1}` | `[{"key":"a","value":1}]` |
| `from_entries` | `[{key,value}]` → object | `[{"key":"a","value":1}]` | `{"a":1}` |
| `with_entries(f)` | `to_entries \| map(f) \| from_entries` | `{"a":1,"b":null}` | — |

`from_entries` accepts both `"key"` and `"name"` as the key field.

---

## Operator Precedence (loosest → tightest)

```
pipe (|)  →  alternative (//)  →  or  →  and  →  comparison (==, !=, <, <=, >, >=)  →  atom
```

So `a or b and c` parses as `a or (b and c)` — `and` binds tighter than `or`.

---

## Not Yet Supported

### Feasible at zero allocation

| Syntax | Description | Implementation Notes |
|--------|-------------|---------------------|
| `.[2:5]`, `.[:3]`, `.[1:]` | Array/string slicing | Iterate to start, copy through end. Just position tracking. |
| `values` | Object values | Equivalent to `.[]`, already supported semantically. |
| `has(0)` | Array index membership | Scan array length, check bounds. |
| `in(expr)` | Reverse membership test | Same scan logic, reversed operands. |
| `recurse` / `..` | Recursive descent | Walk all nested values via scanner, stream via callback. |
| `path(expr)` | Output path as array | Emit path like `["foo","bar"]` or `["items",0]`. |
| `debug` | Debug print, pass through | Print to stderr, forward value unchanged. |

### Feasible but require careful handling

These operations are implementable at zero allocation but involve more complexity or edge cases.

| Syntax | Description | Challenge |
|--------|-------------|-----------|
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
