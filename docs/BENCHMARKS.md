# fastjq Benchmarks

Benchmarks comparing fastjq against [gojq](https://github.com/itchyny/gojq). The root `fastjq` module has zero external Go module dependencies; the `gojq` comparison benchmarks run from the separate `compare/` module. gojq benchmarks include the full `json.Unmarshal` → execute → `json.Marshal` cycle, since that's the real-world cost of using a traditional jq engine from Go.

All benchmarks run with `go test -bench=. -benchmem`. Results from Apple M4 Max, Go 1.25.

> **Benchmark scope**: This snapshot uses the five-file upstream jq library harness. Tier 0 library operations benchmark at 0 allocs/op on the hot path; the recursive/path/stateful parity helpers do allocate, but they remain dramatically lighter than gojq for the same queries.


> **Note on benchmark reliability**: Large benchmarks use rotating input copies (8 distinct pre-generated
> instances) to prevent a Go 1.25 calibration artifact where the auto-calibration pre-pass sees warm-cache
> hits and produces results identical to the Small benchmarks. All benchmarks use `b.Loop()` (Go 1.24+)
> and `benchSink` to prevent dead-code elimination. The Large Select benchmark uses `field_199` (the last
> field in the 200-field object) so fastjq must scan the full 170KB — no early-exit advantage.

## Summary

All times in µs. ¹Large Select uses the last field — fastjq scans the full document. ²gojq wins: after unmarshal, integer arrays are native Go slices. ³Compatibility alias in fastjq (`leaf_paths` / `date`); gojq is benchmarked with the equivalent upstream form.

| Operation | Input | fastjq (µs) | gojq (µs) | Speedup | fastjq allocs | gojq allocs |
|-----------|-------|------------|----------|---------|---------------|-------------|
| `.field` | Small (~100B) | 0.088 | 1.00 | **11x** | 0 | 27 |
| `.field` | Large (~100KB) | 7.44 | 609 | **82x** | 0 | 2835 |
| `del(.f)` | Small (~100B) | 0.173 | 2.39 | **14x** | 0 | 58 |
| `del(.f)` | Medium (~2KB) | 2.27 | 19.8 | **8.7x** | 0 | 323 |
| `del(.f)` | Large (~100KB) | 35.9 | 845 | **24x** | 0 | 4666 |
| `.[n]` | 5-elem array | 0.022 | 0.657 | **29x** | 0 | 20 |
| `del(.[n], .[m])` | 5-elem array | 0.081 | 1.75 | **22x** | 0 | 53 |
| `{f0, f2}` (construct) | Small (~100B) | 0.140 | 1.43 | **10x** | 0 | 37 |
| `{f0, f50}` (construct) | Large (~100KB) | 7.90 | 629 | **80x** | 0 | 2867 |
| `.[]` iterator | 5-elem array | 0.043 | 0.797 | **18x** | 1 | 26 |
| `.[]` iterator | 200-elem array | 7.06 | 88.8 | **13x** | 1 | 1811 |
| `select(.f == "x")` | Small (~100B) | 0.132 | 1.75 | **13x** | 0 | 45 |
| `select(.f == "x")`¹ | Large (~100KB, last field) | 32.4 | 800 | **25x** | 0 | 4651 |
| `select(.f and .g)` | Small (~100B) | 0.176 | 1.85 | **10x** | 0 | 46 |
| `select(.f or .g)` | Small (~100B) | 0.077 | 1.83 | **24x** | 0 | 46 |
| `has("key")` in select | Small (~100B) | 0.102 | 1.76 | **17x** | 0 | 45 |
| `has("key")` in select | Large (~100KB) | 31.6 | 800 | **25x** | 0 | 4651 |
| `if-then-else` | Small (~100B) | 0.101 | 1.12 | **11x** | 0 | 30 |
| `.f // "default"` | Small (~100B) | 0.089 | 1.10 | **12x** | 0 | 31 |
| `try .field` (no error) | Small (~100B) | 0.106 | 1.06 | **10x** | 0 | 30 |
| `.a + .b` (strings) | Small (~100B) | 0.058 | 0.724 | **12x** | 0 | 21 |
| `"prefix" + .name` | Small (~100B) | 0.089 | 0.784 | **8.8x** | 0 | 23 |
| `.a - .b` (subtract) | Small (~100B) | 0.056 | 0.708 | **13x** | 0 | 20 |
| `.a * .b` (multiply) | Small (~100B) | 0.072 | 0.727 | **10x** | 0 | 21 |
| `.a / .b` (divide) | Small (~100B) | 0.110 | 0.744 | **6.8x** | 0 | 21 |
| `.a - .b` (array diff) | 5-elem arrays | 0.230 | 1.27 | **5.5x** | 0 | 33 |
| `length` | Small (~100B) | 0.045 | 1.00 | **23x** | 0 | 27 |
| `length` | Large (~100KB) | 31.8 | 595 | **19x** | 0 | 2835 |
| `add` (numbers) | 5-elem array | 0.059 | 0.758 | **13x** | 0 | 28 |
| `add` (strings) | 5-elem array | 0.137 | 0.913 | **6.7x** | 0 | 35 |
| `flatten` | 3-elem nested array | 0.088 | 1.31 | **15x** | 0 | 38 |
| `min` | 200-int array | 7.54 | 11.6 | **1.5x** | 597 | 409 |
| `min_by(.value)` | 100-elem object array | 11.1 | 56.0 | **5x** | 297 | 1347 |
| `sort` | 200-int array | 11.3 | 24.7 | **2.2x** | 507 | 614 |
| `sort_by(.value)` | 100-elem object array | 19.0 | 91.0 | **4.8x** | 566 | 2145 |
| `unique` | 200-int array | 17.5 | 26.5 | **1.5x** | 906 | 622 |
| `group_by(.active)` | 100-elem object array | 21.9 | 99.8 | **4.6x** | 321 | 2260 |
| `map(.name)` | 20-elem array (~600B) | 1.34 | 10.5 | **7.8x** | 6 | 251 |
| `map(.name)` | 200-elem array (~6KB) | 12.7 | 95.5 | **7.5x** | 6 | 2237 |
| `any` | 5-elem array | 0.042 | 2.00 | **48x** | 0 | 39 |
| `any(expr)` | 5-elem array | 0.396 | 2.20 | **5.6x** | 12 | 49 |
| `any(expr)`² | 200-int array | 5.82 | 32.9 | **5.7x** | 205 | 631 |
| `any(gen; cond)`² | 200-int array | 6.41 | 29.6 | **4.6x** | 207 | 629 |
| `first(expr)` | 5-elem array | 0.259 | 1.55 | **6x** | 10 | 39 |
| `first(expr)`² | 200-int array | 6.61 | 26.5 | **4x** | 209 | 626 |
| `last(expr)` | 5-elem array | 0.362 | 2.03 | **5.6x** | 13 | 43 |
| `last(expr)`² | 200-int array | 12.8 | 42.2 | **3.3x** | 404 | 822 |
| `limit(3; expr)` | 5-elem array | 0.078 | 1.73 | **22x** | 3 | 42 |
| `limit(10; expr)` | 200-int array | 0.656 | 14.0 | **21x** | 3 | 433 |
| `.[1:4]` slice | 6-elem array | 0.083 | 0.858 | **10x** | 0 | 21 |
| `values` | 9-elem array | 0.124 | 2.48 | **20x** | 2 | 51 |
| `skip(2; .[])` | 5-elem array | 0.106 | 1.78 | **17x** | 4 | 43 |
| `reduce .[] as $x (0; . + $x)` | 5-elem array | 0.911 | 1.47 | **1.6x** | 54 | 42 |
| `foreach .[] as $x (0; . + $x)` | 5-elem array | 1.13 | 1.37 | **1.2x** | 63 | 39 |
| `while(.<100; .*2)` | integer 1 | 0.896 | 1.70 | **1.9x** | 39 | 25 |
| ``[.,1]\|until(...)\|.[1]`` | integer 5 | 1.93 | 2.51 | **1.3x** | 78 | 53 |
| `paths` | Small (~100B) | 0.371 | 4.38 | **12x** | 16 | 119 |
| `leaf_paths`³ | Small (~100B) | 1.23 | 6.53 | **5.3x** | 56 | 138 |
| `..` | Small (~100B) | 0.220 | 2.91 | **13x** | 7 | 90 |
| `recurse` | Small (~20B object) | 0.098 | 1.82 | **19x** | 5 | 49 |
| `walk(.)` | Small (~10B object) | 0.210 | 2.40 | **11x** | 12 | 52 |
| `path(.field_0)` | Small (~100B) | 0.102 | 1.25 | **12x** | 5 | 36 |
| `to_entries` | Small (~100B) | 0.170 | 4.17 | **25x** | 0 | 98 |
| `to_entries` | Large (~100KB) | 33.9 | 893 | **26x** | 0 | 6465 |
| `getpath(["field_0"])` | Small (~100B) | 0.216 | 1.07 | **4.9x** | 9 | 29 |
| `setpath(["field_0"]; "y")` | Small (~100B) | 0.367 | 1.81 | **4.9x** | 10 | 43 |
| `delpaths([["field_0"]])` | Small (~100B) | 0.669 | 1.84 | **2.7x** | 19 | 40 |
| `keys` | Small (~100B) | 0.274 | 1.33 | **4.8x** | 7 | 35 |
| `keys_unsorted` | Small (~100B) | 0.164 | 1.32 | **8x** | 0 | 35 |
| `keys_unsorted` | Large (~100KB) | 31.9 | 651 | **20x** | 0 | 3039 |
| `pick(.field_0, .field_2)` | Small (~100B) | 0.872 | 2.57 | **2.9x** | 27 | 64 |
| `INDEX(range(5)... )` | null | 1.74 | 6.90 | **4x** | 90 | 165 |
| `JOIN({...}; .[0]\|tostring)` | 3-pair array | 1.13 | 3.29 | **2.9x** | 49 | 81 |
| `have_decnum` | null | 0.0038 | — | — | 0 | — |
| `object merge .a + .b` | Small (~100B) | 0.175 | 1.53 | **8.7x** | 0 | 33 |
| `tojson` | Small (~100B) | 0.419 | 1.62 | **3.9x** | 0 | 39 |
| `fromjson` | JSON string | 0.197 | 1.33 | **6.8x** | 0 | 31 |
| `todate` | epoch float | 0.049 | 0.754 | **15x** | 0 | 21 |
| `date`³ | epoch float | 0.047 | 0.751 | **16x** | 0 | 21 |
| `now` | null | 0.062 | 0.429 | **6.9x** | 0 | 10 |
| `tonumber` | `"42"` string | 0.014 | 0.367 | **26x** | 0 | 11 |
| `toboolean` | `"true"` string | 0.0095 | 0.357 | **38x** | 0 | 11 |
| `utf8bytelength` | `"asdf\u03bc"` | 0.028 | 0.415 | **15x** | 1 | 12 |
| `split(",")` | short string | 0.142 | 0.821 | **5.8x** | 0 | 21 |
| `join(",")` | 5-elem array | 0.106 | 1.03 | **9.7x** | 0 | 37 |
| `ascii_downcase` in select | Small (~100B) | 0.152 | 1.82 | **12x** | 0 | 46 |
| `ascii_downcase` in select | Large (~100KB) | 33.5 | 818 | **24x** | 0 | 4652 |
| `startswith("s")` | Small (~100B) | 0.129 | 1.77 | **14x** | 0 | 45 |
| `startswith("s")` | Large (~100KB) | 32.9 | 807 | **25x** | 0 | 4652 |
| `endswith("s")` | Small (~100B) | 0.119 | 1.76 | **15x** | 0 | 45 |
| `trim` | short string | 0.048 | 0.398 | **8.3x** | 1 | 12 |
| `ltrim` | short string | 0.044 | 0.396 | **9x** | 1 | 12 |
| `rtrim` | short string | 0.047 | 0.396 | **8.4x** | 1 | 12 |
| `trimstr("s")` | short string | 0.088 | 0.447 | **5.1x** | 5 | 14 |
| `ltrimstr("s")` | Small (~100B) | 0.134 | 1.08 | **8.1x** | 1 | 30 |
| `rtrimstr("s")` | Small (~100B) | 0.140 | 1.10 | **7.8x** | 1 | 30 |
| `reverse` | 5-elem array | 0.135 | 0.912 | **6.8x** | 4 | 22 |
| `select` + string ops + arith + construct | ~200B log event | 1.17 | 3.01 | **2.6x** | 3 | 69 |
| `[.[] \| select + arith + construct]` | 20-elem array (~1.5KB) | 9.17 | 23.2 | **2.5x** | 97 | 525 |
| `length + map + add + min_by + any` | 20-elem array (~1.5KB) | 12.6 | 22.4 | **1.8x** | 95 | 555 |
| `map(try {…} catch …)` | 20-elem array (~1.5KB) | 9.97 | 28.6 | **2.9x** | 124 | 641 |
| `map(if … elif … else …)` | 20-elem array (~1.5KB) | 10.1 | 26.0 | **2.6x** | 158 | 595 |
| `[.[] \| str + tostring + str]` | 20-elem array (~1.5KB) | 10.7 | 25.0 | **2.3x** | 209 | 686 |
| `to_entries \| map(select) \| from_entries` | ~200B log event | 3.33 | 14.0 | **4.2x** | 52 | 250 |
| `@base64` | 34-char string | 0.137 | 0.563 | **4.1x** | 4 | 15 |
| `@base64d` | 48-char encoded | 0.264 | 0.604 | **2.3x** | 5 | 15 |
| `@uri` | 36-char URL string | 0.167 | 0.707 | **4.2x** | 4 | 14 |
| ``@html "<b>\(.field_0)</b>"`` | Small (~100B) | 0.148 | 1.37 | **9.3x** | 4 | 40 |
| `index(",")` | short string | 0.097 | 0.990 | **10x** | 1 | 31 |
| `indices(",")` | short string | 0.201 | 2.28 | **11x** | 1 | 97 |
| `bsearch(42)` | 7-elem sorted array | 0.322 | 0.817 | **2.5x** | 16 | 30 |
| `5 \| IN(range(10))` | null | 0.649 | 2.42 | **3.7x** | 36 | 36 |
| `sqrt` | float (e≈2.718) | 0.069 | 0.462 | **6.7x** | 0 | 12 |
| `log` | float (e≈2.718) | 0.037 | 0.472 | **13x** | 0 | 12 |
| `sin` | float (e≈2.718) | 0.078 | 0.475 | **6.1x** | 0 | 12 |
| `atan` | integer 1 | 0.053 | 0.410 | **7.7x** | 0 | 11 |
| `exp` | integer 1 | 0.064 | 0.424 | **6.6x** | 0 | 11 |
| `tgamma` | integer 5 | 0.013 | 0.393 | **31x** | 0 | 11 |
| `hypot(3;4)` | null | 0.034 | 0.445 | **13x** | 0 | 13 |
| `fma(2;3;4)` | null | 0.033 | 0.455 | **14x** | 0 | 13 |
| `fabs` | float -3.14 | 0.058 | 0.427 | **7.4x** | 0 | 12 |
| `abs` | float -3.14 | 0.010 | 0.429 | **42x** | 0 | 12 |
| `expr as $x \| body` | Small (~20B object) | 0.256 | 0.819 | **3.2x** | 8 | 24 |
| `def inc: . + 1; inc` | integer 1 | 0.377 | 0.511 | **1.4x** | 16 | 15 |
| ``"\(.level): \(.svc)"`` | ~45B object | 0.145 | 1.09 | **7.5x** | 0 | 35 |
| ``"user \(.name) …"`` | ~45B object | 0.106 | 1.27 | **12x** | 0 | 40 |
| `isempty(empty)` | null | 0.049 | 0.605 | **12x** | 4 | 18 |
| `isempty(.[])` | 5-elem array | 0.071 | 1.10 | **15x** | 4 | 33 |
| `nth(2; .[])` | 5-elem array | 0.113 | 2.17 | **19x** | 5 | 49 |
| `range(10)` (10 values) | null | 0.307 | 1.04 | **3.4x** | 20 | 18 |
| `limit(3; range(1000))` | null — only 3 allocs | 0.238 | 1.57 | **6.6x** | 15 | 26 |
| `test(re)` hit | short string | 0.135 | 1.29 | **9.6x** | 2 | 26 |
| `test(re)` miss | short string | 0.122 | 1.25 | **10x** | 2 | 26 |
| `match(re)` hit | short string | 0.218 | 2.90 | **13x** | 1 | 66 |
| `match(re)` miss | short string | 0.603 | 1.65 | **2.7x** | 0 | 24 |
| `capture(re)` hit | short string | 0.215 | — | — | 1 | — |
| `scan(re)` no groups (4 matches) | short string | 0.532 | — | — | 15 | — |
| `sub(re; s)` hit | short string | 0.125 | — | — | 1 | — |
| `gsub(re; s)` hit (4 matches) | short string | 0.583 | — | — | 5 | — |

## Key Takeaways

- **Tier 0 hot-path ops remain zero-alloc** under `RunWithBuffer` / `RunFunc` for direct access, filtering, arithmetic, construction, and most string/math work. Allocating features here are the deliberate parity exceptions or output-shaped helpers.
- **Large-object access stays roughly **82x** faster than gojq**: `.field` on the ~100KB benchmark is 7.44 µs for fastjq versus 609 µs for gojq, and large-object `select` remains about **25x** faster (32.4 µs versus 800 µs).
- **Recursive parity helpers are still materially faster than gojq despite allocs**: `..` is **13x**, `recurse` is **19x**, and `walk(.)` is **11x** on the focused small cases.
- **gojq still wins on tiny primitive-array reductions and some stateful/value-synthesizing helpers** such as `first`, `last`, `reduce`, `foreach`, and `range`, where its unmarshaled in-memory representation is cheaper than rescanning raw JSON bytes or emitting many fresh outputs.

## Raw Output

Apple M4 Max, go1.25.4. Updated 2026-05-01. fastjq benchmarks: `go test -bench=. -benchmem`. gojq comparison benchmarks: `(cd compare && go test -bench=. -benchmem)`. Note: some first-run entries show spurious allocs (e.g. `KeysUnsorted` 3 allocs, `First` 1 alloc) due to benchmark calibration warmup — confirmed 0 allocs on repeat runs.

```text
# fastjq root module
BenchmarkFastjq_Small_Del-16                  	 6103684	       173.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Medium_Del-16                 	  526477	      2270 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Del-16                  	   32536	     35854 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Field-16                	13651928	        88.28 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Field-16                	  159242	      7441 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Index-16                	52750633	        22.26 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ArrayDel-16             	13746418	        80.76 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Construct-16            	 8293698	       140.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Construct-16            	  152256	      7899 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Iterator-16             	27683416	        43.22 ns/op	      16 B/op	       1 allocs/op
BenchmarkFastjq_Large_Iterator-16             	  172660	      7061 ns/op	      16 B/op	       1 allocs/op
BenchmarkFastjq_Small_Select-16               	 9009045	       131.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Select-16               	   37399	     32380 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Alternative-16          	12596689	        88.65 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SelectAnd-16            	 6881563	       176.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SelectOr-16             	15185515	        76.95 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Has-16                  	11854056	       102.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Has-16                  	   37700	     31616 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_IfThenElse-16           	11984973	       101.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Length-16               	28381569	        44.55 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Length-16               	   37870	     31780 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Map-16                  	  810888	      1345 ns/op	     169 B/op	       6 allocs/op
BenchmarkFastjq_Medium_Map-16                 	  190443	      6306 ns/op	     169 B/op	       6 allocs/op
BenchmarkFastjq_Large_Map-16                  	   94794	     12675 ns/op	     169 B/op	       6 allocs/op
BenchmarkFastjq_Small_ToEntries-16            	 7239990	       170.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_ToEntries-16            	   35358	     33859 ns/op	       6 B/op	       0 allocs/op
BenchmarkFastjq_Small_KeysUnsorted-16         	 7300530	       163.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Keys-16                 	 4326493	       274.0 ns/op	     456 B/op	       7 allocs/op
BenchmarkFastjq_Small_Paths-16                	 3226191	       370.8 ns/op	     376 B/op	      16 allocs/op
BenchmarkFastjq_Small_LeafPaths-16            	  970494	      1233 ns/op	     696 B/op	      56 allocs/op
BenchmarkFastjq_Small_RecursiveDescent-16     	 5453541	       219.5 ns/op	     224 B/op	       7 allocs/op
BenchmarkFastjq_Small_Recurse-16              	12260968	        97.86 ns/op	      56 B/op	       5 allocs/op
BenchmarkFastjq_Small_Walk-16                 	 5694778	       210.1 ns/op	     200 B/op	      12 allocs/op
BenchmarkFastjq_Small_Path-16                 	11567439	       102.3 ns/op	     144 B/op	       5 allocs/op
BenchmarkFastjq_Small_GetPath-16              	 5571453	       216.3 ns/op	     200 B/op	       9 allocs/op
BenchmarkFastjq_Small_SetPath-16              	 3259298	       366.7 ns/op	     216 B/op	      10 allocs/op
BenchmarkFastjq_Small_DelPaths-16             	 1778594	       669.4 ns/op	     536 B/op	      19 allocs/op
BenchmarkFastjq_Small_Todate-16               	23563678	        48.90 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Date-16                 	25690154	        46.83 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Now-16                  	19373782	        61.74 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_KeysUnsorted-16         	   37665	     31881 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Any-16                  	27057445	        41.94 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Any-16                  	  945874	      1332 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AnyExpr-16              	 3007818	       395.6 ns/op	     144 B/op	      12 allocs/op
BenchmarkFastjq_Large_AnyExpr-16              	  201261	      5823 ns/op	     629 B/op	     205 allocs/op
BenchmarkFastjq_Small_Add-16                  	20451741	        58.91 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AddStrings-16           	 8849804	       136.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Flatten-16              	13240684	        88.05 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Split-16                	 8316946	       142.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Join-16                 	11512138	       106.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Values-16               	 9650636	       123.7 ns/op	      96 B/op	       2 allocs/op
BenchmarkFastjq_Small_Base64Encode-16         	 8687968	       137.2 ns/op	     120 B/op	       4 allocs/op
BenchmarkFastjq_Small_Base64Decode-16         	 4520678	       263.6 ns/op	     168 B/op	       5 allocs/op
BenchmarkFastjq_Small_IndexFind-16            	12292383	        97.13 ns/op	       3 B/op	       1 allocs/op
BenchmarkFastjq_Small_IndicesAll-16           	 6038054	       200.9 ns/op	       3 B/op	       1 allocs/op
BenchmarkFastjq_Small_Slice-16                	13935568	        82.51 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SliceString-16          	11533586	        97.22 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Plus-16                 	20423547	        57.95 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_PlusStr-16              	13429321	        89.29 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AsciiDowncase-16        	 7804512	       151.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_AsciiDowncase-16        	   35607	     33488 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Startswith-16           	 9382782	       128.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Startswith-16           	   36403	     32867 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Endswith-16             	 9243318	       118.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Ltrimstr-16             	 9058186	       134.2 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Large_Ltrimstr-16             	   39160	     30976 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_Rtrimstr-16             	 8575922	       139.9 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_Trimstr-16              	13723231	        88.05 ns/op	      40 B/op	       5 allocs/op
BenchmarkFastjq_Small_Trim-16                 	23272802	        48.04 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_Ltrim-16                	27229689	        43.98 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_Rtrim-16                	25357760	        46.99 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_UTF8ByteLength-16       	42760740	        28.33 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_Reverse-16              	 8901099	       134.7 ns/op	     360 B/op	       4 allocs/op
BenchmarkFastjq_Small_First-16                	 4535748	       258.5 ns/op	     142 B/op	      10 allocs/op
BenchmarkFastjq_Large_First-16                	  180465	      6614 ns/op	     765 B/op	     209 allocs/op
BenchmarkFastjq_Small_Last-16                 	 3315574	       361.9 ns/op	     130 B/op	      13 allocs/op
BenchmarkFastjq_Large_Last-16                 	   92407	     12830 ns/op	    1376 B/op	     404 allocs/op
BenchmarkFastjq_Small_Limit-16                	15297967	        77.77 ns/op	      56 B/op	       3 allocs/op
BenchmarkFastjq_Small_Skip-16                 	11219911	       106.2 ns/op	     168 B/op	       4 allocs/op
BenchmarkFastjq_Small_Reduce-16               	 1315201	       911.4 ns/op	    1624 B/op	      54 allocs/op
BenchmarkFastjq_Small_Foreach-16              	 1000000	      1133 ns/op	    2312 B/op	      63 allocs/op
BenchmarkFastjq_Small_While-16                	 1335650	       896.0 ns/op	     256 B/op	      39 allocs/op
BenchmarkFastjq_Small_Until-16                	  611002	      1927 ns/op	    2088 B/op	      78 allocs/op
BenchmarkFastjq_Small_Bsearch-16              	 3727758	       321.7 ns/op	     488 B/op	      16 allocs/op
BenchmarkFastjq_Small_Pick-16                 	 1372675	       872.4 ns/op	     752 B/op	      27 allocs/op
BenchmarkFastjq_Small_IN-16                   	 1847990	       648.7 ns/op	    1096 B/op	      36 allocs/op
BenchmarkFastjq_Small_INDEX-16                	  683054	      1742 ns/op	    2392 B/op	      90 allocs/op
BenchmarkFastjq_Small_JOIN-16                 	 1000000	      1127 ns/op	    1344 B/op	      49 allocs/op
BenchmarkFastjq_Small_HaveDecnum-16           	303483291	         3.838 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Limit-16                	 1822536	       655.6 ns/op	      56 B/op	       3 allocs/op
BenchmarkFastjq_Small_Subtract-16             	21380719	        56.29 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Multiply-16             	16744000	        72.22 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Divide-16               	10997326	       109.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Min-16                  	  160662	      7543 ns/op	     920 B/op	     597 allocs/op
BenchmarkFastjq_Small_MinBy-16                	  106317	     11130 ns/op	     517 B/op	     297 allocs/op
BenchmarkFastjq_Sort-16                       	  101037	     11336 ns/op	   17141 B/op	     507 allocs/op
BenchmarkFastjq_SortBy-16                     	   62582	     18972 ns/op	   23208 B/op	     566 allocs/op
BenchmarkFastjq_Unique-16                     	   69104	     17460 ns/op	   18160 B/op	     906 allocs/op
BenchmarkFastjq_GroupBy-16                    	   55005	     21915 ns/op	   22448 B/op	     321 allocs/op
BenchmarkFastjq_Small_URIEncode-16            	 7172803	       166.8 ns/op	     120 B/op	       4 allocs/op
BenchmarkFastjq_Small_HTMLTemplate-16         	 8008575	       148.2 ns/op	      48 B/op	       4 allocs/op
BenchmarkFastjq_Small_ArrayDiff-16            	 5226180	       230.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TryNoError-16           	11752084	       106.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TryCatchNoError-16      	11599390	       101.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ObjectMerge-16          	 6855271	       175.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToJSON-16               	 2862747	       419.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_FromJSON-16             	 6128316	       196.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToString-16             	 2661914	       437.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToNumber-16             	87467802	        13.87 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToBoolean-16            	125830832	         9.506 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AnyTwoArg-16            	  190155	      6411 ns/op	     696 B/op	     207 allocs/op
BenchmarkFastjq_Complex_LogNormalize-16       	 1000000	      1168 ns/op	      16 B/op	       3 allocs/op
BenchmarkFastjq_Complex_ArrayPipeline-16      	  132427	      9168 ns/op	    1592 B/op	      97 allocs/op
BenchmarkFastjq_Complex_Aggregation-16        	   96807	     12553 ns/op	    3168 B/op	      95 allocs/op
BenchmarkFastjq_Complex_TolerantMap-16        	  121896	      9966 ns/op	    2936 B/op	     124 allocs/op
BenchmarkFastjq_Complex_ElifRouting-16        	  118582	     10102 ns/op	    2632 B/op	     158 allocs/op
BenchmarkFastjq_Complex_StringBuild-16        	  109874	     10659 ns/op	    7640 B/op	     209 allocs/op
BenchmarkFastjq_Complex_EntryFilter-16        	  358351	      3327 ns/op	    2576 B/op	      52 allocs/op
BenchmarkFastjq_Small_Sqrt-16                 	17123959	        68.97 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Log-16                  	32512981	        37.07 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Sin-16                  	15316989	        78.12 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Atan-16                 	22864473	        53.50 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Exp-16                  	18656546	        63.84 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Tgamma-16               	95117628	        12.60 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Hypot-16                	35198055	        34.19 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_FMA-16                  	36823887	        33.29 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Fabs-16                 	20211688	        57.77 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Abs-16                  	100000000	        10.21 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Bind-16                 	 4669357	       256.0 ns/op	     432 B/op	       8 allocs/op
BenchmarkFastjq_Small_Def-16                  	 3184822	       377.3 ns/op	     680 B/op	      16 allocs/op
BenchmarkFastjq_Small_StringInterp-16         	 8337222	       145.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_StringInterpNum-16      	11295654	       105.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_IsEmptyTrue-16          	24337788	        49.17 ns/op	      57 B/op	       4 allocs/op
BenchmarkFastjq_Small_IsEmptyFalse-16         	17148470	        71.31 ns/op	      57 B/op	       4 allocs/op
BenchmarkFastjq_Small_Nth-16                  	 9924816	       112.6 ns/op	     104 B/op	       5 allocs/op
BenchmarkFastjq_Small_Range10-16              	 3919941	       307.5 ns/op	     248 B/op	      20 allocs/op
BenchmarkFastjq_Small_RangeLimit-16           	 5093361	       238.4 ns/op	     232 B/op	      15 allocs/op
BenchmarkRegexp_Match_Hit-16                  	27041313	        44.86 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Miss-16                 	51288168	        23.45 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Complex_Hit-16          	26675062	        44.83 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Long_Miss-16            	15436118	        77.82 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_MatchString-16                	26382662	        44.59 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_FindSubmatch_Hit-16           	 5953285	       203.7 ns/op	     160 B/op	       2 allocs/op
BenchmarkRegexp_FindSubmatch_Miss-16          	 1785208	       673.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_FindSubmatchIndex_Hit-16      	 6691261	       180.6 ns/op	      64 B/op	       1 allocs/op
BenchmarkRegexp_FindSubmatchIndex_Miss-16     	 1697071	       704.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_ReplaceAll_Hit-16             	 7844502	       153.9 ns/op	      97 B/op	       5 allocs/op
BenchmarkRegexp_ReplaceAll_Miss-16            	24878479	        48.69 ns/op	      48 B/op	       2 allocs/op
BenchmarkRegexp_MatchViaClosure-16            	26778092	        44.68 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_Hit-16           	 8990335	       134.5 ns/op	      17 B/op	       2 allocs/op
BenchmarkFastjq_Small_TestRe_Miss-16          	 9842680	       122.5 ns/op	      17 B/op	       2 allocs/op
BenchmarkFastjq_Small_TestRe_InPipeline-16    	 3800083	       314.6 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_MatchRe_Hit-16          	 5505262	       218.1 ns/op	      48 B/op	       1 allocs/op
BenchmarkFastjq_Small_MatchRe_Miss-16         	 1979596	       602.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_CaptureRe_Hit-16        	 5498493	       215.3 ns/op	      48 B/op	       1 allocs/op
BenchmarkFastjq_Small_CaptureRe_Miss-16       	 1973922	       604.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ScanRe_NoGroups-16      	 2250321	       531.7 ns/op	     484 B/op	      15 allocs/op
BenchmarkFastjq_Small_ScanRe_WithGroups-16    	 1766617	       679.8 ns/op	     799 B/op	      18 allocs/op
BenchmarkFastjq_Small_SubRe_Hit-16            	 9505398	       125.4 ns/op	      16 B/op	       1 allocs/op
BenchmarkFastjq_Small_SubRe_Miss-16           	11278292	       106.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_GSubRe_Hit-16           	 2051859	       582.7 ns/op	     305 B/op	       5 allocs/op
BenchmarkFastjq_Small_GSubRe_Miss-16          	 4650304	       258.2 ns/op	       0 B/op	       0 allocs/op

# gojq comparison module
BenchmarkGojq_Small_Values-16              	  482704	      2482 ns/op	    3144 B/op	      51 allocs/op
BenchmarkGojq_Small_Base64Encode-16        	 2180724	       563.3 ns/op	    1321 B/op	      15 allocs/op
BenchmarkGojq_Small_Base64Decode-16        	 1996546	       603.9 ns/op	    1321 B/op	      15 allocs/op
BenchmarkGojq_Small_IndexFind-16           	 1205614	       990.3 ns/op	    2274 B/op	      31 allocs/op
BenchmarkGojq_Small_IndicesAll-16          	  550999	      2279 ns/op	    3907 B/op	      97 allocs/op
BenchmarkGojq_Small_Del-16                 	  498825	      2391 ns/op	    3763 B/op	      58 allocs/op
BenchmarkGojq_Medium_Del-16                	   61833	     19774 ns/op	   16970 B/op	     323 allocs/op
BenchmarkGojq_Large_Del-16                 	    1471	    844751 ns/op	  540494 B/op	    4666 allocs/op
BenchmarkGojq_Small_Field-16               	 1000000	      1000 ns/op	    1641 B/op	      27 allocs/op
BenchmarkGojq_Large_Field-16               	    2006	    609091 ns/op	  270050 B/op	    2835 allocs/op
BenchmarkGojq_Small_Index-16               	 1827500	       656.6 ns/op	    1401 B/op	      20 allocs/op
BenchmarkGojq_Small_ArrayDel-16            	  690554	      1746 ns/op	    3363 B/op	      53 allocs/op
BenchmarkGojq_Small_Construct-16           	  889278	      1434 ns/op	    2330 B/op	      37 allocs/op
BenchmarkGojq_Large_Construct-16           	    1936	    629133 ns/op	  274415 B/op	    2867 allocs/op
BenchmarkGojq_Small_Iterator-16            	 1498418	       796.6 ns/op	    1776 B/op	      26 allocs/op
BenchmarkGojq_Large_Iterator-16            	   13543	     88814 ns/op	  109809 B/op	    1811 allocs/op
BenchmarkGojq_Small_Select-16              	  691836	      1745 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Large_Select-16              	    1504	    799807 ns/op	  535547 B/op	    4651 allocs/op
BenchmarkGojq_Small_Alternative-16         	 1000000	      1099 ns/op	    1897 B/op	      31 allocs/op
BenchmarkGojq_Small_SelectAnd-16           	  651687	      1846 ns/op	    2890 B/op	      46 allocs/op
BenchmarkGojq_Small_SelectOr-16            	  660336	      1827 ns/op	    2890 B/op	      46 allocs/op
BenchmarkGojq_Small_Has-16                 	  684604	      1760 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Large_Has-16                 	    1482	    799704 ns/op	  538826 B/op	    4651 allocs/op
BenchmarkGojq_Small_IfThenElse-16          	 1000000	      1121 ns/op	    1817 B/op	      30 allocs/op
BenchmarkGojq_Small_Length-16              	 1000000	      1004 ns/op	    1633 B/op	      27 allocs/op
BenchmarkGojq_Large_Length-16              	    1995	    595459 ns/op	  269833 B/op	    2835 allocs/op
BenchmarkGojq_Small_Map-16                 	  113200	     10511 ns/op	   13654 B/op	     251 allocs/op
BenchmarkGojq_Large_Map-16                 	   12532	     95492 ns/op	  118580 B/op	    2237 allocs/op
BenchmarkGojq_Small_ToEntries-16           	  278184	      4168 ns/op	    6558 B/op	      98 allocs/op
BenchmarkGojq_Large_ToEntries-16           	    1346	    893136 ns/op	  671756 B/op	    6465 allocs/op
BenchmarkGojq_Small_KeysUnsorted-16        	  862341	      1317 ns/op	    1953 B/op	      35 allocs/op
BenchmarkGojq_Small_Keys-16                	  920674	      1326 ns/op	    1953 B/op	      35 allocs/op
BenchmarkGojq_Small_Paths-16               	  270508	      4378 ns/op	    6608 B/op	     119 allocs/op
BenchmarkGojq_Small_LeafPaths-16           	  185119	      6527 ns/op	    7576 B/op	     138 allocs/op
BenchmarkGojq_Small_RecursiveDescent-16    	  411904	      2913 ns/op	    4856 B/op	      90 allocs/op
BenchmarkGojq_Small_Recurse-16             	  671779	      1820 ns/op	    4240 B/op	      49 allocs/op
BenchmarkGojq_Small_Walk-16                	  501877	      2400 ns/op	    5701 B/op	      52 allocs/op
BenchmarkGojq_Small_Path-16                	  976432	      1252 ns/op	    1993 B/op	      36 allocs/op
BenchmarkGojq_Small_GetPath-16             	 1000000	      1066 ns/op	    1721 B/op	      29 allocs/op
BenchmarkGojq_Small_SetPath-16             	  681800	      1807 ns/op	    2618 B/op	      43 allocs/op
BenchmarkGojq_Small_DelPaths-16            	  659160	      1839 ns/op	    2426 B/op	      40 allocs/op
BenchmarkGojq_Small_Todate-16              	 1564474	       753.9 ns/op	    1713 B/op	      21 allocs/op
BenchmarkGojq_Small_Date-16                	 1608788	       751.0 ns/op	    1713 B/op	      21 allocs/op
BenchmarkGojq_Small_Now-16                 	 2846370	       429.0 ns/op	    1112 B/op	      10 allocs/op
BenchmarkGojq_Large_KeysUnsorted-16        	    1854	    650508 ns/op	  282888 B/op	    3039 allocs/op
BenchmarkGojq_Small_Any-16                 	  601353	      1997 ns/op	    4508 B/op	      39 allocs/op
BenchmarkGojq_Small_AnyExpr-16             	  530546	      2202 ns/op	    4620 B/op	      49 allocs/op
BenchmarkGojq_Large_AnyExpr-16             	   35602	     32926 ns/op	   27056 B/op	     631 allocs/op
BenchmarkGojq_Small_Add-16                 	 1584568	       757.9 ns/op	    1529 B/op	      28 allocs/op
BenchmarkGojq_Small_AddStrings-16          	 1312852	       912.7 ns/op	    1785 B/op	      35 allocs/op
BenchmarkGojq_Small_Flatten-16             	  905337	      1313 ns/op	    1857 B/op	      38 allocs/op
BenchmarkGojq_Small_Split-16               	 1475762	       820.9 ns/op	    1553 B/op	      21 allocs/op
BenchmarkGojq_Small_Join-16                	 1000000	      1031 ns/op	    1865 B/op	      37 allocs/op
BenchmarkGojq_Small_Slice-16               	 1398506	       858.3 ns/op	    1425 B/op	      21 allocs/op
BenchmarkGojq_Small_SliceString-16         	 2427495	       492.4 ns/op	    1145 B/op	      12 allocs/op
BenchmarkGojq_Small_Plus-16                	 1637008	       723.6 ns/op	    1673 B/op	      21 allocs/op
BenchmarkGojq_Small_PlusStr-16             	 1560511	       783.9 ns/op	    1705 B/op	      23 allocs/op
BenchmarkGojq_Small_AsciiDowncase-16       	  640183	      1820 ns/op	    2714 B/op	      46 allocs/op
BenchmarkGojq_Large_AsciiDowncase-16       	    1494	    817525 ns/op	  533996 B/op	    4652 allocs/op
BenchmarkGojq_Small_Startswith-16          	  677376	      1766 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Large_Startswith-16          	    1495	    807020 ns/op	  543121 B/op	    4652 allocs/op
BenchmarkGojq_Small_Endswith-16            	  650613	      1760 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Small_Ltrimstr-16            	 1000000	      1084 ns/op	    1737 B/op	      30 allocs/op
BenchmarkGojq_Small_Rtrimstr-16            	 1000000	      1096 ns/op	    1737 B/op	      30 allocs/op
BenchmarkGojq_Small_Trimstr-16             	 2660530	       446.8 ns/op	    1225 B/op	      14 allocs/op
BenchmarkGojq_Small_Trim-16                	 3036258	       397.7 ns/op	    1129 B/op	      12 allocs/op
BenchmarkGojq_Small_Ltrim-16               	 3046171	       395.8 ns/op	    1129 B/op	      12 allocs/op
BenchmarkGojq_Small_Rtrim-16               	 3016245	       396.4 ns/op	    1129 B/op	      12 allocs/op
BenchmarkGojq_Small_UTF8ByteLength-16      	 2897815	       414.6 ns/op	    1137 B/op	      12 allocs/op
BenchmarkGojq_Small_Reverse-16             	 1313451	       911.8 ns/op	    1513 B/op	      22 allocs/op
BenchmarkGojq_Small_First-16               	  788317	      1546 ns/op	    3379 B/op	      39 allocs/op
BenchmarkGojq_Large_First-16               	   46876	     26461 ns/op	   25887 B/op	     626 allocs/op
BenchmarkGojq_Small_Last-16                	  580166	      2032 ns/op	    3483 B/op	      43 allocs/op
BenchmarkGojq_Large_Last-16                	   27831	     42158 ns/op	   29833 B/op	     822 allocs/op
BenchmarkGojq_Small_Limit-16               	  701500	      1733 ns/op	    3328 B/op	      42 allocs/op
BenchmarkGojq_Small_Skip-16                	  684955	      1777 ns/op	    2880 B/op	      43 allocs/op
BenchmarkGojq_Small_Reduce-16              	  854700	      1469 ns/op	    2530 B/op	      42 allocs/op
BenchmarkGojq_Small_Foreach-16             	  885194	      1371 ns/op	    2184 B/op	      39 allocs/op
BenchmarkGojq_Small_While-16               	  710314	      1695 ns/op	    1848 B/op	      25 allocs/op
BenchmarkGojq_Small_Until-16               	  493138	      2512 ns/op	    2658 B/op	      53 allocs/op
BenchmarkGojq_Small_Bsearch-16             	 1464739	       816.8 ns/op	    1545 B/op	      30 allocs/op
BenchmarkGojq_Small_Pick-16                	  468427	      2566 ns/op	    4484 B/op	      64 allocs/op
BenchmarkGojq_Small_IN-16                  	  484744	      2418 ns/op	    4147 B/op	      36 allocs/op
BenchmarkGojq_Small_INDEX-16               	  173799	      6901 ns/op	    9657 B/op	     165 allocs/op
BenchmarkGojq_Small_JOIN-16                	  357172	      3292 ns/op	    4171 B/op	      81 allocs/op
BenchmarkGojq_Large_Limit-16               	   84652	     13995 ns/op	   21984 B/op	     433 allocs/op
BenchmarkGojq_Small_Subtract-16            	 1707060	       707.5 ns/op	    1649 B/op	      20 allocs/op
BenchmarkGojq_Small_Multiply-16            	 1649666	       726.8 ns/op	    1649 B/op	      21 allocs/op
BenchmarkGojq_Small_Divide-16              	 1604514	       744.1 ns/op	    1665 B/op	      21 allocs/op
BenchmarkGojq_Small_Min-16                 	  104113	     11628 ns/op	   13571 B/op	     409 allocs/op
BenchmarkGojq_Small_MinBy-16               	   21429	     55997 ns/op	   63630 B/op	    1347 allocs/op
BenchmarkGojq_Sort-16                      	   48648	     24691 ns/op	   26020 B/op	     614 allocs/op
BenchmarkGojq_SortBy-16                    	   13177	     90992 ns/op	   96922 B/op	    2145 allocs/op
BenchmarkGojq_Unique-16                    	   45500	     26507 ns/op	   31901 B/op	     622 allocs/op
BenchmarkGojq_GroupBy-16                   	   12093	     99821 ns/op	  101909 B/op	    2260 allocs/op
BenchmarkGojq_Small_URIEncode-16           	 1698092	       707.2 ns/op	    1289 B/op	      14 allocs/op
BenchmarkGojq_Small_HTMLTemplate-16        	  876157	      1373 ns/op	    2274 B/op	      40 allocs/op
BenchmarkGojq_Small_ArrayDiff-16           	  944342	      1269 ns/op	    2121 B/op	      33 allocs/op
BenchmarkGojq_Small_TryNoError-16          	 1000000	      1065 ns/op	    1913 B/op	      30 allocs/op
BenchmarkGojq_Small_ObjectMerge-16         	  790617	      1531 ns/op	    2906 B/op	      33 allocs/op
BenchmarkGojq_Small_ToJSON-16              	  740102	      1623 ns/op	    2410 B/op	      39 allocs/op
BenchmarkGojq_Small_FromJSON-16            	  913068	      1327 ns/op	    2714 B/op	      31 allocs/op
BenchmarkGojq_Small_ToNumber-16            	 3266686	       366.6 ns/op	    1113 B/op	      11 allocs/op
BenchmarkGojq_Small_ToBoolean-16           	 3348598	       357.2 ns/op	    1113 B/op	      11 allocs/op
BenchmarkGojq_Small_AnyTwoArg-16           	   40951	     29615 ns/op	   26303 B/op	     629 allocs/op
BenchmarkGojq_Complex_LogNormalize-16      	  412748	      3011 ns/op	    4324 B/op	      69 allocs/op
BenchmarkGojq_Complex_ArrayPipeline-16     	   52303	     23226 ns/op	   20408 B/op	     525 allocs/op
BenchmarkGojq_Complex_Aggregation-16       	   53709	     22368 ns/op	   22398 B/op	     555 allocs/op
BenchmarkGojq_Complex_TolerantMap-16       	   41902	     28599 ns/op	   27872 B/op	     641 allocs/op
BenchmarkGojq_Complex_ElifRouting-16       	   46870	     26018 ns/op	   25920 B/op	     595 allocs/op
BenchmarkGojq_Complex_StringBuild-16       	   47691	     25041 ns/op	   20533 B/op	     686 allocs/op
BenchmarkGojq_Complex_EntryFilter-16       	   84862	     14038 ns/op	   18428 B/op	     250 allocs/op
BenchmarkGojq_Small_Sqrt-16                	 2586285	       462.4 ns/op	    1145 B/op	      12 allocs/op
BenchmarkGojq_Small_Log-16                 	 2539698	       471.9 ns/op	    1129 B/op	      12 allocs/op
BenchmarkGojq_Small_Sin-16                 	 2521270	       475.5 ns/op	    1145 B/op	      12 allocs/op
BenchmarkGojq_Small_Atan-16                	 2914202	       410.0 ns/op	    1121 B/op	      11 allocs/op
BenchmarkGojq_Small_Exp-16                 	 2825283	       423.8 ns/op	    1121 B/op	      11 allocs/op
BenchmarkGojq_Small_Tgamma-16              	 3041149	       392.7 ns/op	    1104 B/op	      11 allocs/op
BenchmarkGojq_Small_Hypot-16               	 2653477	       445.4 ns/op	    1273 B/op	      13 allocs/op
BenchmarkGojq_Small_FMA-16                 	 2646904	       455.2 ns/op	    1273 B/op	      13 allocs/op
BenchmarkGojq_Small_Fabs-16                	 2770863	       427.4 ns/op	    1112 B/op	      12 allocs/op
BenchmarkGojq_Small_Abs-16                 	 2792313	       429.0 ns/op	    1112 B/op	      12 allocs/op
BenchmarkGojq_Small_Bind-16                	 1458832	       819.1 ns/op	    1793 B/op	      24 allocs/op
BenchmarkGojq_Small_Def-16                 	 2342005	       510.9 ns/op	    1377 B/op	      15 allocs/op
BenchmarkGojq_Small_StringInterp-16        	 1000000	      1091 ns/op	    2121 B/op	      35 allocs/op
BenchmarkGojq_Small_StringInterpNum-16     	  927894	      1267 ns/op	    2498 B/op	      40 allocs/op
BenchmarkGojq_Small_IsEmptyTrue-16         	 2027847	       604.7 ns/op	    1921 B/op	      18 allocs/op
BenchmarkGojq_Small_IsEmptyFalse-16        	  961544	      1096 ns/op	    2898 B/op	      33 allocs/op
BenchmarkGojq_Small_Nth-16                 	  552500	      2175 ns/op	    4372 B/op	      49 allocs/op
BenchmarkGojq_Small_Range10-16             	 1000000	      1038 ns/op	    1864 B/op	      18 allocs/op
BenchmarkGojq_Small_RangeLimit-16          	  758695	      1571 ns/op	    3208 B/op	      26 allocs/op
BenchmarkGojq_Small_TestRe_Hit-16          	  931317	      1293 ns/op	    2738 B/op	      26 allocs/op
BenchmarkGojq_Small_TestRe_Miss-16         	  957255	      1250 ns/op	    2697 B/op	      26 allocs/op
BenchmarkGojq_Small_MatchRe_Hit-16         	  404251	      2899 ns/op	    4789 B/op	      66 allocs/op
BenchmarkGojq_Small_MatchRe_Miss-16        	  726963	      1645 ns/op	    2245 B/op	      24 allocs/op
BenchmarkGojq_Small_CaptureRe_Hit-16       	  486044	      2392 ns/op	    4721 B/op	      51 allocs/op
BenchmarkGojq_Small_CaptureRe_Miss-16      	  665996	      1845 ns/op	    2670 B/op	      27 allocs/op
BenchmarkGojq_Small_ScanRe_NoGroups-16     	  416896	      2864 ns/op	    5128 B/op	      57 allocs/op
BenchmarkGojq_Small_SubRe_Hit-16           	  219316	      5305 ns/op	    8982 B/op	      95 allocs/op
BenchmarkGojq_Small_GSubRe_Hit-16          	   81489	     15179 ns/op	   17866 B/op	     248 allocs/op
```

## CLI Throughput: fastjq vs jq

End-to-end JSONL throughput benchmarks comparing `fastjq-bench` (Go CLI using the fastjq library) against `jq` (C implementation, jq-1.8.1). Both read JSONL from stdin, apply a filter per line, write results to `/dev/null`. Both validate JSON: fastjq calls `json.Valid()` per record, jq parses fully. Median of 3 runs. Apple M4 Max.

| Operation | Input | jq (s) | fastjq (s) | Speedup |
|-----------|-------|--------|-------------|---------|
| Identity (`.`) | small (100K lines, ~11MB) | 0.368 | 0.061 | **6.0x** |
| Field access (`.field_2`) | small | 0.167 | 0.051 | **3.3x** |
| Field access (`.field_50`) | large (100 lines, ~16MB) | 0.096 | 0.046 | **2.1x** |
| Delete field (`del(.field_2)`) | small | 0.380 | 0.068 | **5.6x** |
| Object construction (`{field_0, field_2}`) | small | 0.260 | 0.064 | **4.1x** |
| Select all match (`select(.field_2 == "xxx...")`) | small | 0.388 | 0.058 | **6.7x** |
| Select none match (`select(.field_2 == "nope")`) | small | 0.150 | 0.060 | **2.5x** |
| Alternative (`.field_2 // "default"`) | small | 0.178 | 0.057 | **3.1x** |
| Case-insensitive select (`ascii_downcase`) | small | 0.702 | 0.062 | **11.3x** |
| Prefix filter (`startswith`) | small | 0.394 | 0.053 | **7.4x** |
| Field existence (`has`) | small | 0.388 | 0.052 | **7.5x** |
| `to_entries` | small | 0.761 | 0.068 | **11.2x** |
| `keys_unsorted` | small | 0.256 | 0.061 | **4.2x** |

### Key Takeaways (CLI)

- **2.1x–11.3x faster than jq** across this validation-on CLI slice, with the biggest wins on filter/projection work rather than raw field extraction.
- **`to_entries` and case-insensitive filtering are the standout CLI wins** in this run, because fastjq keeps the work to a streaming scan while jq still pays the full parse cost per line.
- **Large-object field extraction is still only a modest CLI win** because both tools validate/parse the whole record; the much larger speedups show up when you call the library directly on already-valid JSON and skip the extra validation pass.
- **The CLI numbers are intentionally conservative**: they include JSON validation and process startup overhead that do not apply to the hot-path library API.

### Reproducing

```bash
chmod +x bench_vs_jq.sh
./bench_vs_jq.sh
```
