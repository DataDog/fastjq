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
| `.[4,2]` | Multi-index selection; emits one result per index | `["a","b","c","d","e"]` | `"e"`, `"c"` |
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
| `del(.[1,2])` | Delete multiple indices from one bracket expression | `["foo","bar","baz"]` | `["foo"]` |
| `del(.[-1])` | Delete last element | `[10,20,30]` | `[10,20]` |
| `del(.[2:4])` | Delete slice range | `[0,1,2,3,4,5]` | `[0,1,4,5]` |
| `del(.[-2:])` | Delete last two elements | `[0,1,2,3,4]` | `[0,1,2]` |
| `del(.[2:4],.[0])` | Mixed index and slice | `[0,1,2,3,4,5,6,7]` | `[1,4,5,6,7]` |

### Iterator

| Syntax | Description | Example Input | Example Output |
|--------|-------------|---------------|----------------|
| `.[]` | All array elements | `[1,2,3]` | `1`, `2`, `3` |
| `.[]` | All object values | `{"a":1,"b":2}` | `1`, `2` |

Use `RunAll` or `RunFunc` to consume multiple outputs. `Run`/`RunWithBuffer` return the first result only.

### Recursive Descent

| Syntax | Description | Example Input | Example Output |
|--------|-------------|---------------|----------------|
| `..` | Depth-first structural traversal, emitting the current value before descendants | `[1,[[2]],{"a":[1]}]` | `[1,[[2]],{"a":[1]}]`, `1`, `[[2]]`, `[2]`, `2`, `{"a":[1]}`, `[1]`, `1` |
| `[..]` | Collect the full recursive-descent stream into one array | `[1,[[2]],{"a":[1]}]` | `[[1,[[2]],{"a":[1]}],1,[[2]],[2],2,{"a":[1]},[1],1]` |
| `.. \| scalars` | Filter the descent stream like any other generator | `{"a":[1,true,{"b":"x"}],"c":null}` | `1`, `true`, `"x"`, `null` |

`..` is implemented as a dedicated structural generator. It composes with the existing pipeline and path-update machinery, so forms like `(.. | select(type=="boolean")) |= ...` behave like jq.

### Recursive Helpers

| Syntax | Description | Example Input | Example Output |
|--------|-------------|---------------|----------------|
| `recurse` | Recurse structurally through child values | `{"a":0,"b":[1]}` | `{"a":0,"b":[1]}`, `0`, `[1]`, `1` |
| `recurse(.foo[])` | Recurse by applying an explicit generator at each step | `{"foo":[{"foo":[]},{"foo":[{"foo":[]}]}]}` | `{"foo":[{"foo":[]},{"foo":[{"foo":[]}]}]}`, `{"foo":[]}`, `{"foo":[{"foo":[]}]}`, `{"foo":[]}` |
| `recurse(. * .; . < 20)` | Recurse while new outputs satisfy a condition | `2` | `2`, `4`, `16` |
| `walk(f)` | Rebuild arrays/objects bottom-up, then apply `f` at each node | `[[4,1,7],[8,5,2],[3,6,9]]` with `walk(if type == "array" then sort else . end)` | `[[1,4,7],[2,5,8],[3,6,9]]` |

`recurse` and `walk` are implemented on this branch for jq-suite parity. They are not part of the zero-allocation hot path.

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

Array construction follows jq generator precedence: `[a, b | f]` is parsed as
`[(a, b) | f]`, not `[a, (b | f)]`.

**Allocation note:** `[.[] | f]` / `map(f)` — when `f` returns an input sub-slice (field access, identity, comparison), the array is built 0-alloc. When `f` constructs new data (object `{…}`, arithmetic, string concat), ~1 alloc per element is needed to prevent result aliasing. `map(.name)` = 0 allocs; `map({name, value})` = ~1 alloc/element.

`map_values(f)` behaves like `map(f)` on arrays. On objects it rewrites through entry objects, applies `f` to each value, and drops fields where `f` produces no outputs.

### Pipe

| Syntax | Description | Example Input | Example Output |
|--------|-------------|---------------|----------------|
| `.address \| del(.zip)` | Chain operations | `{"address":{"city":"NYC","zip":"10001"}}` | `{"city":"NYC"}` |
| `.items \| .[0]` | Pipe to index | `{"items":[10,20]}` | `10` |
| `.[] \| {name}` | Iterate then construct | `[{"name":"alice","age":30}]` | `{"name":"alice"}` |
| `.items[] \| .name` | Multi-output pipe | `{"items":[{"name":"a"},{"name":"b"}]}` | `"a"`, `"b"` |

Pipes propagate multi-output: if the left side produces N results, the right side runs N times.

### Variables

| Syntax | Description | Example Input | Example Output |
|--------|-------------|---------------|----------------|
| `expr as $x \| body` | Bind the result of `expr` to `$x`, then run `body` against the original input | `null` with `1 as $x \| [$x,$x]` | `[1,1]` |
| `.bar as $x \| .foo \| . + $x` | Bound values remain visible across later pipeline stages | `{"foo":10,"bar":200}` | `210` |
| `1 as $x \| [$x,$x,$x as $x \| $x]` | Nested binds shadow outer ones lexically | `null` | `[1,1,1]` |
| `. as [$a, $b] \| [$b, $a]` | Array destructuring bind; missing elements bind to `null` | `[1,2]` | `[2,1]` |
| `. as {$a, b: [$c, {$d}]} \| [$a, $c, $d]` | Object destructuring with nested patterns | `{"a":1,"b":[2,{"d":3}]}` | `[1,2,3]` |

Destructuring patterns follow jq's soft-missing semantics: absent array slots/object fields bind to `null`, while indexing a non-null value with the wrong container type still raises jq-style errors.

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
| `if C then A elif C2 then B else D end` | elif chain — desugars to nested if-then-else | `{"x":2}` | `"two"` |
| `label $out \| .[] \| if . > 1 then break $out else . end` | Exit a labeled stream early | `[0,1,2]` | `0`, `1` |

`elif` is syntactic sugar: `elif C then X` rewrites to `else (if C then X end)` at parse time. Chains of any length are supported.

### Error Handling

| Syntax | Description | Example Input | Example Output |
|--------|-------------|---------------|----------------|
| `try expr` | Suppress errors from expr — produce no output on failure | `[1,2] \| try .foo` | *(no output)* |
| `try expr catch handler` | Run handler with caught value on failure | `[1,2] \| try .foo catch "err"` | `"err"` |
| `error` | Throw the input as an error; `catch` receives the original JSON value | `try ("boom" \| error) catch .` | `"boom"` |

`try` binds tightly: `try .a \| .b` = `(try .a) \| .b`. Wrap in parens to catch a full pipeline: `try (.a \| .b)`. The `errBreak` control signal (used by `first`/`limit`) propagates through `try` unchanged.

When `error` is thrown, the `catch` handler receives the **actual JSON value** (not a string), matching jq semantics. Errors from built-in operations (wrong type, division by zero, etc.) are wrapped as strings.

### Format Strings

| Syntax | Allocs | Description | Example Input | Example Output |
|--------|--------|-------------|---------------|----------------|
| `@base64` | ~4 | Base64-encode a JSON string | `"hello"` | `"aGVsbG8="` |
| `@base64d` | 0 | Base64-decode a JSON string | `"aGVsbG8="` | `"hello"` |
| `@uri` | ~4 | URL percent-encode (RFC 3986 unreserved chars pass through) | `"hello world"` | `"hello%20world"` |
| `@urid` | ~2 | URL percent-decode | `"%CE%BC"` | `"\u03bc"` |
| `@json` / `tojson` | 0 | Serialize any value as a JSON string | `{"a":1}` | `"{\"a\":1}"` |
| `@text` / `tostring` | 0 | Identity for strings; `tojson` for other types | `42` | `"42"` |
| `@html` | ~1 | HTML-escape `&`, `<`, `>`, `'`, `"` | `"<b>&</b>"` | `"&lt;b&gt;&amp;&lt;/b&gt;"` |
| `@csv` | ~1 | Format array as CSV (strings double-quoted, internal quotes doubled) | `[1,"a,b"]` | `"1,\"a,b\""` |
| `@tsv` | ~1 | Format array as TSV (tab/newline/backslash escaped) | `[1,"a\tb"]` | `"1\ta\\tb"` |
| `@sh` | ~1 | POSIX shell-quote a string (single-quote wrapping) | `"O'Hara"` | `"'O'\\''Hara'"` |
| ``@html "<b>\(.)</b>"`` | ~1 | Apply formatter to each interpolation, leave literal template bytes untouched | `"<tag>&"` | `"<b>&lt;tag&gt;&amp;</b>"` |
| ``@sh "echo \(.)"`` | ~1 | Format only interpolated payloads inside a template string | `"O'Hara"` | `"echo 'O'\\''Hara'"` |

`@base64`, `@uri`, `@html`, `@csv`, `@tsv`, `@sh`, and their `@format "...\(...)"` template forms allocate because they decode JSON string escape sequences before encoding — `\n` becomes byte `0x0a`, `\uXXXX` decoded to UTF-8. These are Tier 1 (output-encoding) allocations: proportional to the string being encoded, not the document being scanned. `@base64d`, `@json`, `@text` write directly into the output buffer and are 0-alloc.

### Numeric Rounding and Math

All math functions are zero-alloc. NaN/Infinity results are output as `null` to preserve valid JSON output.

**Rounding**

| Syntax | Description | Example Input | Example Output |
|--------|-------------|---------------|----------------|
| `floor` | Round toward −∞ | `-1.9` | `-2` |
| `ceil` | Round toward +∞ | `-1.1` | `-1` |
| `round` | Round to nearest (half away from zero) | `1.5` | `2` |
| `nearbyint` | Round to nearest integer (uses `math.Round`; differs from IEEE nearbyint only for exactly .5) | `3.5` | `4` |
| `trunc` | Truncate toward zero | `-1.9` | `-1` |

**1-arg floating-point functions** (all take the input number)

| Syntax | Description | Example | Output |
|--------|-------------|---------|--------|
| `sqrt` | Square root | `4 \| sqrt` | `2` |
| `fabs` | Absolute value | `-3.5 \| fabs` | `3.5` |
| `log` | Natural logarithm (ln) | `1 \| log` | `0` |
| `log2` | Base-2 logarithm | `8 \| log2` | `3` |
| `log10` | Base-10 logarithm | `1000 \| log10` | `3` |
| `exp` | e^x | `0 \| exp` | `1` |
| `exp2` | 2^x | `3 \| exp2` | `8` |
| `exp10` | 10^x | `3 \| exp10` | `1000` |
| `cbrt` | Cube root | `27 \| cbrt` | `3` |
| `logb` | Base-2 exponent (integer, as float) | `8 \| logb` | `3` |
| `sin`, `cos`, `tan` | Trigonometric functions (radians) | `0 \| sin` | `0` |
| `asin`, `acos` | Inverse trig | `0 \| asin` | `0` |
| `atan` | 1-arg arctangent | `1 \| atan` | `0.7853981633974483` |
| `tgamma` | Gamma function Γ(x) | `5 \| tgamma` | `24` |
| `lgamma` | ln\|Γ(x)\| | `1 \| lgamma` | `0` |
| `j0`, `j1` | Bessel functions of first kind, orders 0 and 1 | `0 \| j0` | `1` |

**Not supported (rejected)**

| Syntax | Reason |
|--------|--------|
| `nan`, `infinite` | Produce non-JSON output, violating the "output is always compact JSON" constraint |
| `isnan`, `isinfinite`, `isfinite`, `isnormal` | Depend on nan/infinite representation; meaningless without it |
| `pow(x; y)`, `hypot(x; y)`, `atan(y; x)`, `fma(x;y;z)` | 2/3-arg forms. Every test for these is blocked by `as $` binding (0 exclusive tests). Parser would need 2-arg semicolon-separated forms. |
| `frexp`, `modf` | Return array pairs `[mantissa, exponent]`; 0 exclusive tests |
| `ldexp`, `scalb`, `scalbln` | Take a float + integer exponent; 0 exclusive tests |
| `significand` | Complex semantics (mantissa in [1,2)); 0 exclusive tests |

### Containment

| Syntax | Description | Example Input | Example Output |
|--------|-------------|---------------|----------------|
| `contains(val)` | True if input recursively contains val | `"foobar" \| contains("foo")` | `true` |
| `contains(val)` | String: substring check | `"ab\u0000cd" \| contains("b\u0000c")` | `true` |
| `contains(val)` | Object: all key-value pairs of val present (recursively) | `{"a":1,"b":2} \| contains({"a":1})` | `true` |
| `contains(val)` | Array: all elements of val contained in some element | `[1,2,3] \| contains([2,3])` | `true` |
| `inside(val)` | Reverse of contains: `a \| inside(b)` ≡ `b \| contains(a)` | `{"a":1} \| inside({"a":1,"b":2})` | `true` |

### Type Filters

| Syntax | Description | Equivalent to |
|--------|-------------|---------------|
| `numbers` | Keep only numbers | `select(type == "number")` |
| `strings` | Keep only strings | `select(type == "string")` |
| `arrays` | Keep only arrays | `select(type == "array")` |
| `objects` | Keep only objects | `select(type == "object")` |
| `booleans` | Keep only booleans | `select(type == "boolean")` |
| `nulls` | Keep only null | `select(type == "null")` |
| `values` | Keep anything that is not null | `select(. != null)` |
| `iterables` | Keep arrays and objects | `select(type == "array" or type == "object")` |
| `scalars` | Keep non-containers | `select(type != "array" and type != "object")` |

These are all zero-alloc parser aliases — no new ops, just desugared to existing `select(type == ...)` forms.

### Reverse Membership

| Syntax | Description | Example | Output |
|--------|-------------|---------|--------|
| `"key" \| in(obj)` | True if string is a key in the object | `"foo" \| in({"foo":1})` | `true` |
| `n \| in(arr)` | True if integer n is a valid array index | `1 \| in([0,1,2])` | `true` |

### Search and Debug

| Syntax | Description | Example Input | Example Output |
|--------|-------------|---------------|----------------|
| `index("s")` | First occurrence of string/value in string or array | `"a,b,c"` | `1` |
| `rindex("s")` | Last occurrence | `"a,b,c"` | `3` |
| `indices("s")` | All occurrences as array | `"a,b,c"` | `[1,3]` |
| `index(n)` | First occurrence of value n in array | `[1,2,3,2]` with `index(2)` | `1` |
| `rindex(n)` | Last occurrence of value n in array | `[1,2,3,2]` with `rindex(2)` | `3` |
| `indices([a,b])` | All positions where subsequence `[a,b]` starts in array | `[0,1,2,3,1,2]` with `indices([1,2])` | `[1,4]` |
| `debug` | Print value to stderr as `[DEBUG]: value`, pass through | `{"x":1}` | `{"x":1}` |

For strings, positions are Unicode codepoint offsets (not byte offsets), matching jq behaviour for multi-byte characters. Substring matches are overlapping — `indices("aba")` on `"xababax"` returns `[1,3]`. Returns `null` if not found, `[]` for `indices` with no matches.

### Membership & Length

| Syntax | Description | Example Input | Example Output |
|--------|-------------|---------------|----------------|
| `has("key")` | True if object has field (even if null) | `{"x":null}` | `true` |
| `has(n)` | True if array index n exists (n ≥ 0) | `[1,2,3]` with `has(2)` | `true` |
| `length` | String → chars, array/object → count, null → 0 | `[1,2,3]` | `3` |
| `abs` | Numbers become positive; strings pass through unchanged | `-3.5` | `3.5` |
| `keys` | Object keys sorted lexicographically; array → indices | `{"b":1,"a":2}` | `["a","b"]` |
| `keys_unsorted` | Object keys in insertion order; array → indices | `{"b":1,"a":2}` | `["b","a"]` |
| `paths` / `paths(filter)` | Emit non-root structural paths, optionally filtered by value | `[1,[[],{"a":2}]]` with `[paths(type == "number")]` | `[[0],[1,1,"a"]]` |
| `path(expr)` | Emit the symbolic path taken by a direct field/index/iterator expression | `{"foo":{"bar":4},"baz":"bar"}` with `path(.foo[.baz])` | `["foo","bar"]` |
| `getpath(path)` | Follow a path array of strings/numbers; variadic args emit multiple outputs | `{"a":{"b":0,"c":1}}` with `[getpath(["a","b"], ["a","c"])]` | `[0,1]` |
| `setpath(path; value)` | Set or synthesize a nested path | `null` with `setpath(["a","b"]; 1)` | `{"a":{"b":1}}` |
| `delpaths(paths)` | Delete a list of nested paths | `{"a":{"b":1},"x":{"y":2}}` with `delpaths([["a","b"]])` | `{"a":{},"x":{"y":2}}` |

### Slicing and Concatenation

| Syntax | Description | Example Input | Example Output |
|--------|-------------|---------------|----------------|
| `.[n:m]` | Array/string slice from index n to m (exclusive); bounds may be numeric expressions | `[0,1,2,3,4]` with `.[1.2:3.5]` | `[1,2,3]` |
| `.[:m]` | Slice from start to m | `"hello world"` with `.[:rindex(" ")]` | `"hello"` |
| `.[n:]` | Slice from n to end | `[0,1,2,3,4]` with `.[2:]` | `[2,3,4]` |
| `.[:]` | Full slice (identity for arrays/strings) | `[1,2,3]` | `[1,2,3]` |
| `.[-n:]` | Last n elements | `[0,1,2,3,4]` with `.[-2:]` | `[3,4]` |
| `.[n:m]?` | Optional slice: suppress type errors for non-sliceable values while preserving jq's `null` result case | `[1,null]` with `[.[] | .[1:3]?]` | `[null]` |
| `"a" + "b"` | String concatenation | any | `"ab"` |
| `[1] + [2,3]` | Array concatenation | any | `[1,2,3]` |
| `.a + .b` | Field concatenation | `{"a":"foo","b":"bar"}` | `"foobar"` |
| `null + x` / `x + null` | null is identity for `+` | any | `x` |

Slicing uses **logical characters** for strings: each escape sequence (`\n`, `\uXXXX`, etc.) and each UTF-8 codepoint counts as one character. Float bounds follow jq's rules (start floors, end ceils, `nan` becomes start `0` / end `length`). Negative indices count from the end. Indices are clamped to valid range.

`+` supports: strings (concat), arrays (concat), numbers (sum), objects (merge). Null is the identity element. For object merge, right-hand keys win on conflict: `{"a":1} + {"a":2}` = `{"a":2}`.

### Arithmetic Operators

| Syntax | Description | Example Input | Example Output |
|--------|-------------|---------------|----------------|
| `expr - expr` | Number subtraction; array difference | `.a - .b` on `{"a":10,"b":3}` | `7` |
| `expr - expr` | Array difference (elements of left not in right) | `[1,2,3] - [2]` | `[1,3]` |
| `expr * expr` | Number multiplication; string × n = repeat | `.price * .qty` on `{"price":2.5,"qty":4}` | `10` |
| `expr / expr` | Number division; string / string = split | `"a,b,c" / ","` | `["a","b","c"]` |
| `expr % expr` | Number modulo | `10 % 3` | `1` |

Precedence: `*`, `/`, `%` bind tighter than `+`, `-`. All left-associative. `null` propagates: `null op x = null`.

### Array Extrema

| Syntax | Description | Example Input | Example Output |
|--------|-------------|---------------|----------------|
| `min` | Minimum element of array | `[3,1,4,1,5]` | `1` |
| `max` | Maximum element of array | `[3,1,4,1,5]` | `5` |
| `min_by(f)` | Element where `f` is minimum | `[{"n":"a","v":3},{"n":"b","v":1}]` with `min_by(.v)` | `{"n":"b","v":1}` |
| `max_by(f)` | Element where `f` is maximum | same | `{"n":"a","v":3}` |
| `sort` | Sort using jq type ordering | `[3,null,"a",1]` | `[null,1,3,"a"]` |
| `sort_by(f)` | Sort by key function | `[{"k":3},{"k":1}]` with `sort_by(.k)` | `[{"k":1},{"k":3}]` |
| `sort_by(.a, .b)` | Sort by tuple key | `[{"a":1,"b":3},{"a":1,"b":1}]` | `[{"a":1,"b":1},{"a":1,"b":3}]` |
| `unique` | Sort and deduplicate | `[3,1,2,1,3]` | `[1,2,3]` |
| `unique_by(f)` | Keep first of each key group | `[{"v":1,"x":1},{"v":1,"x":2}]` with `unique_by(.v)` | `[{"v":1,"x":1}]` |
| `group_by(f)` | Group by key into sub-arrays | `[{"v":1},{"v":2},{"v":1}]` with `group_by(.v)` | `[[{"v":1},{"v":1}],[{"v":2}]]` |
| `transpose` | Matrix transpose (null-pad short rows) | `[[1],[2,3]]` | `[[1,2],[null,3]]` |

Numbers compared by value; strings lexicographically. Empty array → `null`. `sort` type ordering: null < false < true < numbers < strings < arrays < objects. **Tier 2: allocates O(n) proportional to array size.**

### Reduction and Generation

| Syntax | Description | Example Input | Example Output |
|--------|-------------|---------------|----------------|
| `add` | Sum numbers, concatenate strings/arrays, merge objects | `[1,2,3]` | `6` |
| `add` | String concatenation | `["a","b","c"]` | `"abc"` |
| `add` | Array concatenation | `[[1,2],[3,4]]` | `[1,2,3,4]` |
| `add` | Empty/null array → null | `[]` | `null` |
| `flatten` | Fully flatten nested arrays | `[[1,[2]],3]` | `[1,2,3]` |
| `flatten(n)` | Flatten at most n levels | `[[1,[2]],3]` with `flatten(1)` | `[1,[2],3]` |
| `split("s")` | Split string by separator | `"a,b,c"` with `split(",")` | `["a","b","c"]` |
| `join("s")` | Join array with separator | `["a","b","c"]` with `join(",")` | `"a,b,c"` |

`join` converts numbers to their string representation; nulls become empty strings.

### Stream Control

| Syntax | Allocs | Description | Example Input | Example Output |
|--------|--------|-------------|---------------|----------------|
| `first` | 0 | First element (no-arg: `.[0]`) | `[10,20,30]` | `10` |
| `last` | 0 | Last element (no-arg: `.[-1]`) | `[10,20,30]` | `30` |
| `first(expr)` | 0 | First output of expr | `[1,2,3,4,5]` (with `first(.[] \| select(. > 2))`) | `3` |
| `last(expr)` | 0 | Last output of expr | `[1,2,3,4,5]` (with `last(.[] \| select(. > 2))`) | `5` |
| `limit(n; expr)` | 0 | First N outputs of expr as a stream | `[1,2,3,4,5]` (with `limit(3; .[])`) | `1`, `2`, `3` |
| `skip(n; expr)` | 0 | Drop the first N outputs of expr, emit the rest | `[1,2,3,4,5]` (with `skip(2; .[])`) | `3`, `4`, `5` |
| `reduce gen as $x (init; update)` | output-shaped | Fold generator outputs into one or more accumulator states | `[1,2,4]` (with `reduce .[] as $x (0; . + $x)`) | `7` |
| `foreach gen as $x (init; update; extract?)` | output-shaped | Emit intermediate accumulator-derived values while folding | `[1,2,4]` (with `foreach .[] as $x (0; . + $x; [., $x])`) | `[1,1]`, `[3,2]`, `[7,4]` |
| `def f: body; expr` | 0 | Define a zero-arg jq filter with lexical scope and call it later in `expr` | `3` (with `def f: . + 1; f`) | `4` |
| `def f(a; $b): body; f(arg1; arg2)` | output-shaped | Define jq functions with filter params (`a`) and value params (`$b`) | `[1,2,3]` (with `def y($a;$b): $a + $b; y(.[]; .[]*2)`) | `3`, `5`, `4`, `6`, `5`, `7` |
| `range(n)` | 1/value | Generate integers 0, 1, …, n−1 | — | `0`, `1`, `2` |
| `range(from; to)` | 1/value | Generate integers from `from` to `to−1` | — | `2`, `3`, `4` |
| `range(from; to; step)` | 1/value | Generate with explicit step (float ok, negative ok) | — | `0`, `2`, `4` |

`limit` emits a stream, not an array. Wrap in `[...]` if you need an array: `[limit(3; .[])]`. The body can be a comma-separated generator: `limit(1; a, b)`.

`reduce` evaluates `init` once, then runs `update` against the accumulator for every value produced by `gen` while binding `$x` to the current item. This branch supports the simple jq form `reduce gen as $x (init; update)`.

`foreach` shares the same accumulator model as `reduce`, but emits after each update. When the extract clause is omitted, it defaults to identity on the updated accumulator. If `update` produces multiple outputs, `foreach` emits extract results for each one but only carries the last update result forward to the next iteration, matching jq.

`def` uses jq-style lexical scoping. Bare params such as `x` are filter params that re-run against the current input each time they are referenced inside the function body; `$x` params are value params and expand over all outputs of their argument expression. Nested defs shadow outer defs by name/arity, and self-recursion is supported.

`range` is a **Tier 2** operation: 1 alloc per generated value (the output byte slice), proportional to what you asked to generate. Compose with `limit` for lazy evaluation: `limit(3; range(1000))` produces only 3 values and 3 allocs.

`any(generator; cond)` two-arg form is supported.

### Boolean Reduction

| Syntax | Description | Example Input | Example Output |
|--------|-------------|---------------|----------------|
| `any` | True if any element is truthy | `[false,1,false]` | `true` |
| `all` | True if all elements are truthy (vacuously true for `[]`) | `[1,"x",true]` | `true` |
| `any(expr)` | True if expr is truthy for any element of input array | `[1,2,3]` with `any(. > 2)` | `true` |
| `all(expr)` | True if expr is truthy for all elements of input array | `[1,2,3]` with `all(. > 0)` | `true` |
| `any(gen; cond)` | True if `gen \| cond` is truthy for any output of gen | `any(.[]; .active)` | — |
| `all(gen; cond)` | True if `gen \| cond` is truthy for all outputs of gen | `all(.[]; .n > 0)` | — |

All forms short-circuit. `any(gen; cond)` is equivalent to `first(gen \| select(cond)) \| true` but with a false default.

### String Operations

| Syntax | Description | Example Input | Example Output |
|--------|-------------|---------------|----------------|
| `ascii_downcase` | Convert string to lowercase | `"Hello"` | `"hello"` |
| `ascii_upcase` | Convert string to uppercase | `"hello"` | `"HELLO"` |
| `startswith("s")` | True if string starts with s | `"foobar"` | `true` |
| `endswith("s")` | True if string ends with s | `"foobar"` | `false` (for `"foo"`) |
| `trim` | Trim leading and trailing Unicode whitespace | `"  abc  "` | `"abc"` |
| `ltrim` | Trim leading Unicode whitespace | `"  abc  "` | `"abc  "` |
| `rtrim` | Trim trailing Unicode whitespace | `"  abc  "` | `"  abc"` |
| `ltrimstr("s")` | Remove prefix if present | `"prod-auth"` | `"auth"` |
| `rtrimstr("s")` | Remove suffix if present | `"app.log"` | `"app"` |

Escape sequences in strings are preserved by case conversion. All string operations work on raw JSON string bytes — no Unicode-aware processing.

### Regex (Go RE2)

All patterns are compiled once at `Compile()` time (stored in the AST). `Run()` never allocates for the regex engine. Go's RE2 engine guarantees linear-time matching — immune to ReDoS. Named groups use `(?P<name>...)` syntax. Backreferences and lookahead are not supported; invalid patterns fail at `Compile()` time.

| Syntax | Allocs | Description | Example | Output |
|--------|--------|-------------|---------|--------|
| `test("re")` | **0** | `true` if input string matches | `"foo123" \| test("[0-9]+")` | `true` |
| `test("re"; "flags")` | **0** | Case-insensitive (`i`), multiline (`m`), dot-all (`s`) | `"FOO" \| test("foo"; "i")` | `true` |
| `match("re")` | 1 on hit | Match object: `{offset, length, string, captures}` | `"foo bar" \| match("(\\w+)")` | `{"offset":0,"length":3,"string":"foo","captures":[...]}` |
| `capture("re")` | 1 on hit | Named captures only as flat object | `"alice@example" \| capture("(?P<user>\\w+)@(?P<host>\\w+)")` | `{"user":"alice","host":"example"}` |
| `scan("re")` | per match | Stream all non-overlapping matches; no groups → strings, with groups → arrays | `"a1b2" \| [scan("[0-9]+")]` | `["1","2"]` |
| `sub("re"; "rep")` | 1 on hit | Replace first match with literal `rep` | `"hello world" \| sub("o"; "0")` | `"hell0 world"` |
| `gsub("re"; "rep")` | per match | Replace all matches with literal `rep` | `"aababc" \| gsub("a"; "x")` | `"xxbxbc"` |

Replacement strings in `sub`/`gsub` are literals — `\(...)` capture group references are not supported. Non-string input returns `null` (for `match`/`capture`) or `false` (for `test`) rather than erroring, so regex ops compose cleanly with `select`.

### Type Conversion

| Syntax | Description | Example Input | Example Output |
|--------|-------------|---------------|----------------|
| `tojson` / `@json` | Serialize any value as a JSON string | `{"a":1}` | `"{\"a\":1}"` |
| `fromjson` | Parse a JSON string to its value | `"{\"a\":1}"` | `{"a":1}` |
| `tostring` | Strings pass through; non-strings serialized via `tojson` | `42` | `"42"` |
| `tonumber` | Numbers pass through; strings parsed as floats | `"3.14"` | `3.14` |
| `toboolean` | Booleans pass through; `"true"`/`"false"` strings are parsed | `"true"` | `true` |

`tojson \| fromjson` is an identity round-trip. `tostring \| tonumber` round-trips numbers. `toboolean` throws a jq-style error string for invalid input such as `0`, `null`, or `"tru"`.

### Object Transforms

| Syntax | Description | Example Input | Example Output |
|--------|-------------|---------------|----------------|
| `to_entries` | Object → `[{"key":k,"value":v}]` | `{"a":1}` | `[{"key":"a","value":1}]` |
| `from_entries` | `[{key,value}]` → object | `[{"key":"a","value":1}]` | `{"a":1}` |
| `with_entries(.key |= "KEY_" + .)` | Convenience form for `to_entries \| map(f) \| from_entries` | `{"a":1,"b":2}` | `{"KEY_a":1,"KEY_b":2}` |
| `map_values(.+1)` | Transform object values; arrays use `map(.+1)` semantics | `{"a":1,"b":2}` | `{"a":2,"b":3}` |

`from_entries` accepts both `"key"` and `"name"` as the key field. `with_entries(f)` is parser sugar for `to_entries | map(f) | from_entries`, and `map_values(f)` uses the same entry-transform shape on objects while behaving like `map(f)` on arrays.

---

## Operator Precedence (loosest → tightest)

```
pipe (|)  →  alternative (//)  →  or  →  and  →  comparison (==, !=, <, <=, >, >=)  →  atom
```

So `a or b and c` parses as `a or (b and c)` — `and` binds tighter than `or`.

---

## Not Yet Supported

### Feasible but require careful handling

These operations are implementable at zero allocation but involve more complexity or edge cases.

| Syntax | Description | Challenge |
|--------|-------------|-----------|

### Implemented — bounded O(n) allocation (Tier 2)

These operations allocate an auxiliary index structure, but the allocation is **bounded by the collection the user explicitly provided** — not by the document being scanned.

| Syntax | Alloc model | Notes |
|--------|-------------|-------|
| `sort` | ~n+O(log n) | Collect element sub-slices (no copy), sort in-place, emit array. |
| `sort_by(f)`, `sort_by(.a, .b)` | ~3n | Collect elements + compute keys (nil-buf → sub-slices for field access) + sort `[]int` index. Preserves original order of equal elements (stable). |
| `unique` | ~n | Same as `sort`, then remove consecutive duplicates by value. |
| `unique_by(f)` | ~3n | Same as `sort_by`, keep first of each key group. |
| `group_by(f)` | ~3n | Same as `sort_by`, emit `[[group1], [group2], ...]`. |
| `transpose` | ~n×m | Collect all rows, find max length, emit transposed columns with null padding. |
| `range(n)`, `range(from;to;step)` | 1 per value | Each integer output is a fresh byte slice (synthesised, not in input). |
| `{a: .x[]}` multi-output construction | ~n per level | Cartesian product via `execConstructMulti`; single-output pairs use zero-alloc fast path. |

### Feasible — bounded O(n) allocation (Tier 2, planned)

| Syntax | Alloc model | Notes |
|--------|-------------|-------|
| `implode` | O(n) | Array of codepoints → UTF-8 string. |
| `explode` | O(n) | String → array of Unicode codepoints. |

### Rejected — allocations proportional to INPUT structure

The governing principle rejects operations where allocation scales with the *shape of the data being processed*, not with the result being produced. The caller cannot control these allocations by choosing what to ask for.

| Syntax | Why rejected |
|--------|-------------|
| *(range is now implemented as Tier 2)* | `range(n)`, `range(from;to)`, `range(from;to;step)` are supported. See Stream Control section above. |

### Not applicable (streaming/CLI concerns)

| Syntax | Description |
|--------|-------------|
| `input`, `inputs` | Read from stdin |
| `env` | Access environment |
| `--raw-output`, `-r` | CLI output formatting |
| `--slurp`, `-s` | CLI input mode |
| `--arg`, `--argjson` | CLI variable injection |

---

## Summary

**fastjq's allocation model:** allocations are proportional to what you ask for, never to what the engine scans.

- **Tier 0 (zero-alloc):** field access, filtering, comparison, arithmetic, construction, `map(.field)`, math, `test(re)` — the full hot path for log processing.
- **Tier 1 (alloc ∝ output):** `@base64`, `@uri`, `match`, `capture`, `scan`, `gsub`, `map(f)` with construction — allocate proportional to the data they produce, never to the input size.
- **Tier 2 (alloc ∝ collection, implemented):** `sort`, `sort_by(f)`, `unique`, `unique_by(f)`, `group_by(f)`, `transpose`, `range(n)`, multi-output object construction — O(n) bounded by the array/output the user explicitly requested.
- **Still intentionally deferred:** full decimal-mode semantics behind `have_decnum`, module/import loading, and CLI/environment helpers such as `input`, `inputs`, `env`, and `$ENV`.
