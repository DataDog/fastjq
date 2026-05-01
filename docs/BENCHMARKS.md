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
| `.field` | Small (~100B) | 0.080 | 1.00 | **13x** | 0 | 27 |
| `.field` | Large (~100KB) | 7.44 | 597 | **80x** | 0 | 2835 |
| `del(.f)` | Small (~100B) | 0.174 | 2.36 | **14x** | 0 | 58 |
| `del(.f)` | Medium (~2KB) | 2.30 | 19.5 | **8.5x** | 0 | 323 |
| `del(.f)` | Large (~100KB) | 34.4 | 819 | **24x** | 0 | 4666 |
| `.[n]` | 5-elem array | 0.022 | 0.655 | **30x** | 0 | 20 |
| `del(.[n], .[m])` | 5-elem array | 0.081 | 1.70 | **21x** | 0 | 53 |
| `{f0, f2}` (construct) | Small (~100B) | 0.131 | 1.39 | **11x** | 0 | 37 |
| `{f0, f50}` (construct) | Large (~100KB) | 7.82 | 600 | **77x** | 0 | 2867 |
| `.[]` iterator | 5-elem array | 0.043 | 0.793 | **19x** | 1 | 26 |
| `.[]` iterator | 200-elem array | 7.24 | 90.7 | **13x** | 1 | 1811 |
| `select(.f == "x")` | Small (~100B) | 0.122 | 1.81 | **15x** | 0 | 45 |
| `select(.f == "x")`¹ | Large (~100KB, last field) | 32.7 | 834 | **26x** | 0 | 4651 |
| `select(.f and .g)` | Small (~100B) | 0.167 | 1.87 | **11x** | 0 | 46 |
| `select(.f or .g)` | Small (~100B) | 0.072 | 1.84 | **25x** | 0 | 46 |
| `has("key")` in select | Small (~100B) | 0.089 | 1.76 | **20x** | 0 | 45 |
| `has("key")` in select | Large (~100KB) | 31.9 | 798 | **25x** | 0 | 4651 |
| `if-then-else` | Small (~100B) | 0.099 | 1.12 | **11x** | 0 | 30 |
| `.f // "default"` | Small (~100B) | 0.095 | 1.12 | **12x** | 0 | 31 |
| `try .field` (no error) | Small (~100B) | 5.04 | 1.15 | 0.2x† | 1 | 30 |
| `.a + .b` (strings) | Small (~100B) | 0.051 | 0.760 | **15x** | 0 | 21 |
| `"prefix" + .name` | Small (~100B) | 0.081 | 0.762 | **9.4x** | 0 | 23 |
| `.a - .b` (subtract) | Small (~100B) | 0.047 | 0.712 | **15x** | 0 | 20 |
| `.a * .b` (multiply) | Small (~100B) | 0.064 | 0.731 | **11x** | 0 | 21 |
| `.a / .b` (divide) | Small (~100B) | 0.095 | 0.752 | **7.9x** | 0 | 21 |
| `.a - .b` (array diff) | 5-elem arrays | 0.202 | 1.39 | **6.9x** | 0 | 33 |
| `length` | Small (~100B) | 0.042 | 1.01 | **24x** | 0 | 27 |
| `length` | Large (~100KB) | 31.8 | 597 | **19x** | 0 | 2835 |
| `add` (numbers) | 5-elem array | 0.055 | 0.777 | **14x** | 0 | 28 |
| `add` (strings) | 5-elem array | 0.139 | 0.920 | **6.6x** | 0 | 35 |
| `flatten` | 3-elem nested array | 0.088 | 1.35 | **15x** | 0 | 38 |
| `min` | 200-int array | 7.09 | 11.7 | **1.6x** | 597 | 409 |
| `min_by(.value)` | 100-elem object array | 10.3 | 59.4 | **5.8x** | 297 | 1347 |
| `sort` | 200-int array | 10.5 | 25.7 | **2.4x** | 507 | 614 |
| `sort_by(.value)` | 100-elem object array | 16.9 | 94.5 | **5.6x** | 566 | 2145 |
| `unique` | 200-int array | 16.2 | 27.3 | **1.7x** | 906 | 622 |
| `group_by(.active)` | 100-elem object array | 19.4 | 105 | **5.4x** | 321 | 2260 |
| `map(.name)` | 20-elem array (~600B) | 1.28 | 10.5 | **8.2x** | 6 | 251 |
| `map(.name)` | 200-elem array (~6KB) | 12.6 | 96.5 | **7.7x** | 6 | 2237 |
| `any` | 5-elem array | 0.041 | 2.04 | **49x** | 0 | 39 |
| `any(expr)` | 5-elem array | 0.369 | 2.27 | **6.2x** | 12 | 49 |
| `any(expr)`² | 200-int array | 5.48 | 32.6 | **5.9x** | 205 | 631 |
| `any(gen; cond)`² | 200-int array | 5.68 | 30.4 | **5.4x** | 207 | 629 |
| `first(expr)` | 5-elem array | 0.232 | 1.66 | **7.1x** | 10 | 39 |
| `first(expr)`² | 200-int array | 5.97 | 25.9 | **4.3x** | 209 | 626 |
| `last(expr)` | 5-elem array | 0.329 | 1.98 | **6x** | 13 | 43 |
| `last(expr)`² | 200-int array | 11.4 | 41.2 | **3.6x** | 404 | 822 |
| `limit(3; expr)` | 5-elem array | 0.074 | 1.78 | **24x** | 3 | 42 |
| `limit(10; expr)` | 200-int array | 0.625 | 13.8 | **22x** | 3 | 433 |
| `.[1:4]` slice | 6-elem array | 4.80 | 0.884 | 0.2x† | 1 | 21 |
| `values` | 9-elem array | 0.112 | 2.63 | **23x** | 2 | 51 |
| `skip(2; .[])` | 5-elem array | 0.097 | 1.82 | **19x** | 4 | 43 |
| `reduce .[] as $x (0; . + $x)` | 5-elem array | 105 | 1.45 | 0.0x† | 70 | 42 |
| `foreach .[] as $x (0; . + $x)` | 5-elem array | 87.7 | 1.38 | 0.0x† | 79 | 39 |
| `while(.<100; .*2)` | integer 1 | 0.823 | 1.70 | **2.1x** | 39 | 25 |
| ``[.,1]\|until(...)\|.[1]`` | integer 5 | 1.75 | 2.50 | **1.4x** | 78 | 53 |
| `paths` | Small (~100B) | 0.363 | 4.45 | **12x** | 16 | 119 |
| `leaf_paths`³ | Small (~100B) | 1.19 | 6.48 | **5.4x** | 56 | 138 |
| `..` | Small (~100B) | 0.228 | 2.92 | **13x** | 7 | 90 |
| `recurse` | Small (~20B object) | 0.095 | 1.83 | **19x** | 5 | 49 |
| `walk(.)` | Small (~10B object) | 0.204 | 2.40 | **12x** | 12 | 52 |
| `path(.field_0)` | Small (~100B) | 0.096 | 1.25 | **13x** | 5 | 36 |
| `to_entries` | Small (~100B) | 0.170 | 4.21 | **25x** | 0 | 98 |
| `to_entries` | Large (~100KB) | 33.8 | 891 | **26x** | 0 | 6465 |
| `getpath(["field_0"])` | Small (~100B) | 0.207 | 1.06 | **5.1x** | 9 | 29 |
| `setpath(["field_0"]; "y")` | Small (~100B) | 0.355 | 1.83 | **5.1x** | 10 | 43 |
| `delpaths([["field_0"]])` | Small (~100B) | 0.661 | 1.84 | **2.8x** | 19 | 40 |
| `keys` | Small (~100B) | 0.278 | 1.34 | **4.8x** | 7 | 35 |
| `keys_unsorted` | Small (~100B) | 0.165 | 1.33 | **8.1x** | 0 | 35 |
| `keys_unsorted` | Large (~100KB) | 31.8 | 647 | **20x** | 0 | 3039 |
| `pick(.field_0, .field_2)` | Small (~100B) | 0.853 | 2.56 | **3x** | 27 | 64 |
| `INDEX(range(5)... )` | null | 1.67 | 7.23 | **4.3x** | 90 | 165 |
| `JOIN({...}; .[0]\|tostring)` | 3-pair array | 0.996 | 3.29 | **3.3x** | 49 | 81 |
| `have_decnum` | null | 0.0027 | — | — | 0 | — |
| `object merge .a + .b` | Small (~100B) | 0.162 | 1.67 | **10x** | 0 | 33 |
| `tojson` | Small (~100B) | 0.400 | 1.71 | **4.3x** | 0 | 39 |
| `fromjson` | JSON string | 0.188 | 1.40 | **7.4x** | 0 | 31 |
| `todate` | epoch float | 0.047 | 0.778 | **17x** | 0 | 21 |
| `date`³ | epoch float | 0.048 | 0.763 | **16x** | 0 | 21 |
| `now` | null | 0.061 | 0.416 | **6.9x** | 0 | 10 |
| `tonumber` | `"42"` string | 0.013 | 0.406 | **31x** | 0 | 11 |
| `toboolean` | `"true"` string | 0.0084 | 0.392 | **46x** | 0 | 11 |
| `utf8bytelength` | `"asdf\u03bc"` | 0.030 | 0.430 | **15x** | 1 | 12 |
| `split(",")` | short string | 0.146 | 0.852 | **5.9x** | 0 | 21 |
| `join(",")` | 5-elem array | 0.099 | 1.07 | **11x** | 0 | 37 |
| `ascii_downcase` in select | Small (~100B) | 0.132 | 1.80 | **14x** | 0 | 46 |
| `ascii_downcase` in select | Large (~100KB) | 33.1 | 810 | **24x** | 0 | 4653 |
| `startswith("s")` | Small (~100B) | 0.128 | 1.76 | **14x** | 0 | 45 |
| `startswith("s")` | Large (~100KB) | 32.4 | 807 | **25x** | 0 | 4651 |
| `endswith("s")` | Small (~100B) | 0.124 | 1.76 | **14x** | 0 | 45 |
| `trim` | short string | 0.050 | 0.392 | **7.9x** | 1 | 12 |
| `ltrim` | short string | 0.047 | 0.421 | **9x** | 1 | 12 |
| `rtrim` | short string | 0.051 | 0.420 | **8.2x** | 1 | 12 |
| `trimstr("s")` | short string | 0.087 | 0.444 | **5.1x** | 5 | 14 |
| `ltrimstr("s")` | Small (~100B) | 0.128 | 1.08 | **8.4x** | 1 | 30 |
| `rtrimstr("s")` | Small (~100B) | 0.129 | 1.08 | **8.4x** | 1 | 30 |
| `reverse` | 5-elem array | 0.136 | 0.968 | **7.1x** | 4 | 22 |
| `select` + string ops + arith + construct | ~200B log event | 1.04 | 3.10 | **3x** | 3 | 69 |
| `[.[] \| select + arith + construct]` | 20-elem array (~1.5KB) | 8.20 | 23.9 | **2.9x** | 97 | 525 |
| `length + map + add + min_by + any` | 20-elem array (~1.5KB) | 11.5 | 23.3 | **2x** | 95 | 555 |
| `map(try {…} catch …)` | 20-elem array (~1.5KB) | 299 | 28.9 | 0.1x† | 144 | 641 |
| `map(if … elif … else …)` | 20-elem array (~1.5KB) | 8.89 | 26.4 | **3x** | 158 | 595 |
| `[.[] \| str + tostring + str]` | 20-elem array (~1.5KB) | 9.18 | 25.1 | **2.7x** | 209 | 686 |
| `to_entries \| map(select) \| from_entries` | ~200B log event | 3.02 | 14.4 | **4.8x** | 52 | 250 |
| `@base64` | 34-char string | 0.140 | 0.565 | **4x** | 4 | 15 |
| `@base64d` | 48-char encoded | 0.269 | 0.571 | **2.1x** | 5 | 15 |
| `@uri` | 36-char URL string | 0.158 | 0.738 | **4.7x** | 4 | 14 |
| ``@html "<b>\(.field_0)</b>"`` | Small (~100B) | 0.129 | 1.48 | **11x** | 4 | 40 |
| `index(",")` | short string | 0.096 | 0.948 | **9.9x** | 1 | 31 |
| `indices(",")` | short string | 0.203 | 2.18 | **11x** | 1 | 97 |
| `bsearch(42)` | 7-elem sorted array | 0.313 | 0.823 | **2.6x** | 16 | 30 |
| `5 \| IN(range(10))` | null | 0.637 | 2.49 | **3.9x** | 36 | 36 |
| `sqrt` | float (e≈2.718) | 0.066 | 0.470 | **7.1x** | 0 | 12 |
| `log` | float (e≈2.718) | 0.035 | 0.484 | **14x** | 0 | 12 |
| `sin` | float (e≈2.718) | 0.076 | 0.490 | **6.4x** | 0 | 12 |
| `atan` | integer 1 | 0.051 | 0.449 | **8.8x** | 0 | 11 |
| `exp` | integer 1 | 0.060 | 0.460 | **7.6x** | 0 | 11 |
| `tgamma` | integer 5 | 0.012 | 0.417 | **34x** | 0 | 11 |
| `hypot(3;4)` | null | 0.021 | 0.468 | **22x** | 0 | 13 |
| `fma(2;3;4)` | null | 0.028 | 0.478 | **17x** | 0 | 13 |
| `fabs` | float -3.14 | 0.052 | 0.430 | **8.2x** | 0 | 12 |
| `abs` | float -3.14 | 0.0091 | 0.444 | **49x** | 0 | 12 |
| `expr as $x \| body` | Small (~20B object) | 30.6 | 0.868 | 0.0x† | 12 | 24 |
| `def inc: . + 1; inc` | integer 1 | 26.4 | 0.554 | 0.0x† | 22 | 15 |
| ``"\(.level): \(.svc)"`` | ~45B object | 0.117 | 1.15 | **9.9x** | 0 | 35 |
| ``"user \(.name) …"`` | ~45B object | 0.082 | 1.32 | **16x** | 0 | 40 |
| `isempty(empty)` | null | 0.043 | 0.630 | **15x** | 4 | 18 |
| `isempty(.[])` | 5-elem array | 0.063 | 1.15 | **18x** | 4 | 33 |
| `nth(2; .[])` | 5-elem array | 0.100 | 2.34 | **24x** | 5 | 49 |
| `range(10)` (10 values) | null | 0.282 | 1.10 | **3.9x** | 20 | 18 |
| `limit(3; range(1000))` | null — only 3 allocs | 0.217 | 1.66 | **7.6x** | 15 | 26 |
| `test(re)` hit | short string | 0.145 | 1.41 | **9.7x** | 2 | 26 |
| `test(re)` miss | short string | 0.123 | 1.39 | **11x** | 2 | 26 |
| `match(re)` hit | short string | 0.218 | 3.10 | **14x** | 1 | 66 |
| `match(re)` miss | short string | 0.630 | 1.69 | **2.7x** | 0 | 24 |
| `capture(re)` hit | short string | 0.215 | — | — | 1 | — |
| `scan(re)` no groups (4 matches) | short string | 0.530 | — | — | 15 | — |
| `sub(re; s)` hit | short string | 0.127 | — | — | 1 | — |
| `gsub(re; s)` hit (4 matches) | short string | 0.599 | — | — | 5 | — |

## Key Takeaways

- **Tier 0 hot-path ops remain zero-alloc** under `RunWithBuffer` / `RunFunc` for direct access, filtering, arithmetic, construction, and most string/math work. Allocating features here are the deliberate parity exceptions or output-shaped helpers.
- **Large-object access stays roughly **80x** faster than gojq**: `.field` on the ~100KB benchmark is 7.44 µs for fastjq versus 597 µs for gojq, and large-object `select` remains about **26x** faster (32.7 µs versus 834 µs).
- **Recursive parity helpers are materially faster than gojq despite allocs**: `..` is **13x**, `recurse` is **19x**, and `walk(.)` is **12x** on the focused small cases.
- **gojq wins on tiny primitive-array reductions and some stateful/value-synthesizing helpers** such as `first`, `last`, `reduce`, `foreach`, and `range`, where its unmarshaled in-memory representation is cheaper than rescanning raw JSON bytes or emitting many fresh outputs.

## Raw Output

Apple M4 Max, go1.25.4. Updated 2026-05-01. fastjq benchmarks: `go test -bench=. -benchmem`. gojq comparison benchmarks: `(cd compare && go test -bench=. -benchmem)`. Note: some first-run entries show spurious allocs (e.g. `KeysUnsorted` 3 allocs, `First` 1 alloc) due to benchmark calibration warmup — confirmed 0 allocs on repeat runs.

```text
# fastjq root module
BenchmarkFastjq_Small_Del-16                  	 6316941	       174.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Medium_Del-16                 	  534930	      2304 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Del-16                  	   34738	     34399 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Field-16                	13714141	        79.94 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Field-16                	  162648	      7437 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Index-16                	55132173	        21.72 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ArrayDel-16             	14522122	        80.90 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Construct-16            	 9504348	       130.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Construct-16            	  152490	      7824 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Iterator-16             	27752803	        42.83 ns/op	      16 B/op	       1 allocs/op
BenchmarkFastjq_Large_Iterator-16             	  172578	      7237 ns/op	      16 B/op	       1 allocs/op
BenchmarkFastjq_Small_Select-16               	 9252661	       121.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Select-16               	   36913	     32709 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Alternative-16          	12345583	        94.78 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SelectAnd-16            	 7331016	       167.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SelectOr-16             	16460472	        72.39 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Has-16                  	13950601	        89.26 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Has-16                  	   37783	     31929 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_IfThenElse-16           	12235164	        99.30 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Length-16               	27470719	        41.53 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Length-16               	   37250	     31833 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Map-16                  	  934748	      1278 ns/op	     137 B/op	       6 allocs/op
BenchmarkFastjq_Medium_Map-16                 	  203197	      6095 ns/op	     137 B/op	       6 allocs/op
BenchmarkFastjq_Large_Map-16                  	   95944	     12618 ns/op	     137 B/op	       6 allocs/op
BenchmarkFastjq_Small_ToEntries-16            	 7593422	       170.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_ToEntries-16            	   35797	     33766 ns/op	       6 B/op	       0 allocs/op
BenchmarkFastjq_Small_KeysUnsorted-16         	 6920858	       165.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Keys-16                 	 4319040	       277.5 ns/op	     456 B/op	       7 allocs/op
BenchmarkFastjq_Small_Paths-16                	 3301048	       362.9 ns/op	     376 B/op	      16 allocs/op
BenchmarkFastjq_Small_LeafPaths-16            	 1000000	      1193 ns/op	     696 B/op	      56 allocs/op
BenchmarkFastjq_Small_RecursiveDescent-16     	 5295555	       228.3 ns/op	     224 B/op	       7 allocs/op
BenchmarkFastjq_Small_Recurse-16              	12529779	        95.24 ns/op	      56 B/op	       5 allocs/op
BenchmarkFastjq_Small_Walk-16                 	 5895772	       203.6 ns/op	     200 B/op	      12 allocs/op
BenchmarkFastjq_Small_Path-16                 	12352573	        96.25 ns/op	     144 B/op	       5 allocs/op
BenchmarkFastjq_Small_GetPath-16              	 5756845	       207.4 ns/op	     200 B/op	       9 allocs/op
BenchmarkFastjq_Small_SetPath-16              	 3339468	       355.4 ns/op	     216 B/op	      10 allocs/op
BenchmarkFastjq_Small_DelPaths-16             	 1812650	       661.3 ns/op	     536 B/op	      19 allocs/op
BenchmarkFastjq_Small_Todate-16               	23689411	        46.87 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Date-16                 	25394271	        47.89 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Now-16                  	19721758	        60.60 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_KeysUnsorted-16         	   37766	     31770 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Any-16                  	26434993	        41.29 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Any-16                  	  941385	      1307 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AnyExpr-16              	 3234531	       368.9 ns/op	     144 B/op	      12 allocs/op
BenchmarkFastjq_Large_AnyExpr-16              	  214449	      5482 ns/op	     629 B/op	     205 allocs/op
BenchmarkFastjq_Small_Add-16                  	22135081	        55.19 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AddStrings-16           	 8672493	       139.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Flatten-16              	13049499	        88.18 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Split-16                	 8274974	       145.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Join-16                 	12137784	        98.91 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Values-16               	10666966	       112.2 ns/op	      64 B/op	       2 allocs/op
BenchmarkFastjq_Small_Base64Encode-16         	 8427520	       140.1 ns/op	     120 B/op	       4 allocs/op
BenchmarkFastjq_Small_Base64Decode-16         	 4413843	       269.3 ns/op	     168 B/op	       5 allocs/op
BenchmarkFastjq_Small_IndexFind-16            	12637027	        95.88 ns/op	       3 B/op	       1 allocs/op
BenchmarkFastjq_Small_IndicesAll-16           	 5897206	       203.3 ns/op	       3 B/op	       1 allocs/op
BenchmarkFastjq_Small_Slice-16                	  246228	      4797 ns/op	      64 B/op	       1 allocs/op
BenchmarkFastjq_Small_SliceString-16          	  252822	      4766 ns/op	      64 B/op	       1 allocs/op
BenchmarkFastjq_Small_Plus-16                 	23190702	        50.97 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_PlusStr-16              	14913871	        81.29 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AsciiDowncase-16        	 8780857	       132.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_AsciiDowncase-16        	   36255	     33087 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Startswith-16           	 9424918	       128.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Startswith-16           	   37060	     32438 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Endswith-16             	 9569793	       124.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Ltrimstr-16             	 9530896	       127.7 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Large_Ltrimstr-16             	   39457	     31060 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_Rtrimstr-16             	 9441634	       129.0 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_Trimstr-16              	13805983	        87.15 ns/op	      40 B/op	       5 allocs/op
BenchmarkFastjq_Small_Trim-16                 	23413624	        49.85 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_Ltrim-16                	25432348	        46.94 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_Rtrim-16                	23621523	        51.48 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_UTF8ByteLength-16       	40672106	        29.59 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_Reverse-16              	 8789311	       135.9 ns/op	     360 B/op	       4 allocs/op
BenchmarkFastjq_Small_First-16                	 5141618	       231.9 ns/op	     110 B/op	      10 allocs/op
BenchmarkFastjq_Large_First-16                	  201123	      5975 ns/op	     733 B/op	     209 allocs/op
BenchmarkFastjq_Small_Last-16                 	 3634839	       329.1 ns/op	      98 B/op	      13 allocs/op
BenchmarkFastjq_Large_Last-16                 	  105226	     11401 ns/op	    1344 B/op	     404 allocs/op
BenchmarkFastjq_Small_Limit-16                	16144474	        73.82 ns/op	      56 B/op	       3 allocs/op
BenchmarkFastjq_Small_Skip-16                 	12420628	        97.36 ns/op	     136 B/op	       4 allocs/op
BenchmarkFastjq_Small_Reduce-16               	   10000	    105157 ns/op	    2176 B/op	      70 allocs/op
BenchmarkFastjq_Small_Foreach-16              	   13660	     87701 ns/op	    2704 B/op	      79 allocs/op
BenchmarkFastjq_Small_While-16                	 1459459	       822.6 ns/op	     256 B/op	      39 allocs/op
BenchmarkFastjq_Small_Until-16                	  690301	      1745 ns/op	    1704 B/op	      78 allocs/op
BenchmarkFastjq_Small_Bsearch-16              	 3784590	       313.1 ns/op	     488 B/op	      16 allocs/op
BenchmarkFastjq_Small_Pick-16                 	 1410979	       852.6 ns/op	     752 B/op	      27 allocs/op
BenchmarkFastjq_Small_IN-16                   	 1893542	       637.0 ns/op	    1096 B/op	      36 allocs/op
BenchmarkFastjq_Small_INDEX-16                	  725440	      1671 ns/op	    2360 B/op	      90 allocs/op
BenchmarkFastjq_Small_JOIN-16                 	 1200486	       996.4 ns/op	    1216 B/op	      49 allocs/op
BenchmarkFastjq_Small_HaveDecnum-16           	438050194	         2.737 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Limit-16                	 1915419	       625.5 ns/op	      56 B/op	       3 allocs/op
BenchmarkFastjq_Small_Subtract-16             	24894673	        47.39 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Multiply-16             	19061397	        63.95 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Divide-16               	12616665	        95.22 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Min-16                  	  165123	      7088 ns/op	     920 B/op	     597 allocs/op
BenchmarkFastjq_Small_MinBy-16                	  112852	     10311 ns/op	     517 B/op	     297 allocs/op
BenchmarkFastjq_Sort-16                       	  115050	     10520 ns/op	   17141 B/op	     507 allocs/op
BenchmarkFastjq_SortBy-16                     	   71382	     16887 ns/op	   23208 B/op	     566 allocs/op
BenchmarkFastjq_Unique-16                     	   73393	     16209 ns/op	   18160 B/op	     906 allocs/op
BenchmarkFastjq_GroupBy-16                    	   61863	     19385 ns/op	   22448 B/op	     321 allocs/op
BenchmarkFastjq_Small_URIEncode-16            	 7461664	       157.8 ns/op	     120 B/op	       4 allocs/op
BenchmarkFastjq_Small_HTMLTemplate-16         	 9127111	       129.2 ns/op	      48 B/op	       4 allocs/op
BenchmarkFastjq_Small_ArrayDiff-16            	 5793340	       202.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TryNoError-16           	  237258	      5040 ns/op	      64 B/op	       1 allocs/op
BenchmarkFastjq_Small_TryCatchNoError-16      	  245838	      5016 ns/op	      64 B/op	       1 allocs/op
BenchmarkFastjq_Small_ObjectMerge-16          	 7290374	       161.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToJSON-16               	 3005952	       400.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_FromJSON-16             	 6501015	       187.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToString-16             	 3022022	       398.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToNumber-16             	92909818	        13.09 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToBoolean-16            	141461383	         8.448 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AnyTwoArg-16            	  212478	      5679 ns/op	     664 B/op	     207 allocs/op
BenchmarkFastjq_Complex_LogNormalize-16       	 1000000	      1037 ns/op	      16 B/op	       3 allocs/op
BenchmarkFastjq_Complex_ArrayPipeline-16      	  147516	      8198 ns/op	    1528 B/op	      97 allocs/op
BenchmarkFastjq_Complex_Aggregation-16        	  104564	     11497 ns/op	    2400 B/op	      95 allocs/op
BenchmarkFastjq_Complex_TolerantMap-16        	    4147	    298838 ns/op	    4184 B/op	     144 allocs/op
BenchmarkFastjq_Complex_ElifRouting-16        	  134427	      8893 ns/op	    2600 B/op	     158 allocs/op
BenchmarkFastjq_Complex_StringBuild-16        	  130140	      9183 ns/op	    5912 B/op	     209 allocs/op
BenchmarkFastjq_Complex_EntryFilter-16        	  397934	      3017 ns/op	    2480 B/op	      52 allocs/op
BenchmarkFastjq_Small_Sqrt-16                 	17487994	        66.08 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Log-16                  	34300536	        35.50 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Sin-16                  	15986703	        76.10 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Atan-16                 	23550961	        50.85 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Exp-16                  	19292617	        60.19 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Tgamma-16               	99433773	        12.28 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Hypot-16                	55539484	        21.46 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_FMA-16                  	41024295	        28.17 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Fabs-16                 	22186579	        52.30 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Abs-16                  	131512818	         9.148 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Bind-16                 	   39080	     30567 ns/op	     536 B/op	      12 allocs/op
BenchmarkFastjq_Small_Def-16                  	   45151	     26403 ns/op	     928 B/op	      22 allocs/op
BenchmarkFastjq_Small_StringInterp-16         	10195696	       116.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_StringInterpNum-16      	14567106	        82.48 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_IsEmptyTrue-16          	28168792	        42.61 ns/op	      57 B/op	       4 allocs/op
BenchmarkFastjq_Small_IsEmptyFalse-16         	19106010	        62.93 ns/op	      57 B/op	       4 allocs/op
BenchmarkFastjq_Small_Nth-16                  	12030838	        99.58 ns/op	     104 B/op	       5 allocs/op
BenchmarkFastjq_Small_Range10-16              	 4243723	       282.0 ns/op	     248 B/op	      20 allocs/op
BenchmarkFastjq_Small_RangeLimit-16           	 5470764	       217.4 ns/op	     232 B/op	      15 allocs/op
BenchmarkRegexp_Match_Hit-16                  	28029087	        43.22 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Miss-16                 	53215862	        22.64 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Complex_Hit-16          	27609002	        42.62 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Long_Miss-16            	16031277	        75.19 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_MatchString-16                	27812488	        43.37 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_FindSubmatch_Hit-16           	 6093235	       194.4 ns/op	     160 B/op	       2 allocs/op
BenchmarkRegexp_FindSubmatch_Miss-16          	 1771700	       676.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_FindSubmatchIndex_Hit-16      	 6967318	       171.8 ns/op	      64 B/op	       1 allocs/op
BenchmarkRegexp_FindSubmatchIndex_Miss-16     	 1780972	       676.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_ReplaceAll_Hit-16             	 8120038	       146.7 ns/op	      97 B/op	       5 allocs/op
BenchmarkRegexp_ReplaceAll_Miss-16            	24885960	        46.76 ns/op	      48 B/op	       2 allocs/op
BenchmarkRegexp_MatchViaClosure-16            	27909762	        43.14 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_Hit-16           	 9288278	       145.3 ns/op	      17 B/op	       2 allocs/op
BenchmarkFastjq_Small_TestRe_Miss-16          	 9782554	       123.3 ns/op	      17 B/op	       2 allocs/op
BenchmarkFastjq_Small_TestRe_InPipeline-16    	 3825657	       311.1 ns/op	       8 B/op	       0 allocs/op
BenchmarkFastjq_Small_MatchRe_Hit-16          	 5476879	       218.0 ns/op	      48 B/op	       1 allocs/op
BenchmarkFastjq_Small_MatchRe_Miss-16         	 1914709	       629.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_CaptureRe_Hit-16        	 5504097	       215.1 ns/op	      48 B/op	       1 allocs/op
BenchmarkFastjq_Small_CaptureRe_Miss-16       	 1997053	       593.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ScanRe_NoGroups-16      	 2261014	       530.2 ns/op	     484 B/op	      15 allocs/op
BenchmarkFastjq_Small_ScanRe_WithGroups-16    	 1739995	       693.4 ns/op	     799 B/op	      18 allocs/op
BenchmarkFastjq_Small_SubRe_Hit-16            	 9403058	       126.7 ns/op	      16 B/op	       1 allocs/op
BenchmarkFastjq_Small_SubRe_Miss-16           	11721830	       103.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_GSubRe_Hit-16           	 2011519	       598.8 ns/op	     305 B/op	       5 allocs/op
BenchmarkFastjq_Small_GSubRe_Miss-16          	 4660675	       260.8 ns/op	       0 B/op	       0 allocs/op

# gojq comparison module
BenchmarkGojq_Small_Values-16              	  394112	      2627 ns/op	    3144 B/op	      51 allocs/op
BenchmarkGojq_Small_Base64Encode-16        	 2056749	       565.3 ns/op	    1321 B/op	      15 allocs/op
BenchmarkGojq_Small_Base64Decode-16        	 2095224	       570.5 ns/op	    1321 B/op	      15 allocs/op
BenchmarkGojq_Small_IndexFind-16           	 1273604	       947.6 ns/op	    2274 B/op	      31 allocs/op
BenchmarkGojq_Small_IndicesAll-16          	  566617	      2182 ns/op	    3907 B/op	      97 allocs/op
BenchmarkGojq_Small_Del-16                 	  490124	      2362 ns/op	    3763 B/op	      58 allocs/op
BenchmarkGojq_Medium_Del-16                	   60166	     19536 ns/op	   16969 B/op	     323 allocs/op
BenchmarkGojq_Large_Del-16                 	    1452	    819129 ns/op	  543704 B/op	    4666 allocs/op
BenchmarkGojq_Small_Field-16               	 1000000	      1004 ns/op	    1641 B/op	      27 allocs/op
BenchmarkGojq_Large_Field-16               	    1999	    596914 ns/op	  270052 B/op	    2835 allocs/op
BenchmarkGojq_Small_Index-16               	 1862128	       655.4 ns/op	    1401 B/op	      20 allocs/op
BenchmarkGojq_Small_ArrayDel-16            	  686224	      1702 ns/op	    3362 B/op	      53 allocs/op
BenchmarkGojq_Small_Construct-16           	  849888	      1387 ns/op	    2329 B/op	      37 allocs/op
BenchmarkGojq_Large_Construct-16           	    2013	    600222 ns/op	  274480 B/op	    2867 allocs/op
BenchmarkGojq_Small_Iterator-16            	 1514434	       793.2 ns/op	    1776 B/op	      26 allocs/op
BenchmarkGojq_Large_Iterator-16            	   13375	     90682 ns/op	  109808 B/op	    1811 allocs/op
BenchmarkGojq_Small_Select-16              	  694425	      1814 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Large_Select-16              	    1491	    834269 ns/op	  536513 B/op	    4651 allocs/op
BenchmarkGojq_Small_Alternative-16         	 1000000	      1119 ns/op	    1897 B/op	      31 allocs/op
BenchmarkGojq_Small_SelectAnd-16           	  643726	      1873 ns/op	    2890 B/op	      46 allocs/op
BenchmarkGojq_Small_SelectOr-16            	  662680	      1844 ns/op	    2890 B/op	      46 allocs/op
BenchmarkGojq_Small_Has-16                 	  692641	      1758 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Large_Has-16                 	    1498	    797962 ns/op	  539182 B/op	    4651 allocs/op
BenchmarkGojq_Small_IfThenElse-16          	 1000000	      1125 ns/op	    1817 B/op	      30 allocs/op
BenchmarkGojq_Small_Length-16              	 1000000	      1012 ns/op	    1633 B/op	      27 allocs/op
BenchmarkGojq_Large_Length-16              	    2020	    596664 ns/op	  269833 B/op	    2835 allocs/op
BenchmarkGojq_Small_Map-16                 	  113947	     10539 ns/op	   13654 B/op	     251 allocs/op
BenchmarkGojq_Large_Map-16                 	   12372	     96542 ns/op	  118618 B/op	    2237 allocs/op
BenchmarkGojq_Small_ToEntries-16           	  290095	      4212 ns/op	    6558 B/op	      98 allocs/op
BenchmarkGojq_Large_ToEntries-16           	    1332	    891069 ns/op	  677668 B/op	    6465 allocs/op
BenchmarkGojq_Small_KeysUnsorted-16        	  894548	      1335 ns/op	    1953 B/op	      35 allocs/op
BenchmarkGojq_Small_Keys-16                	  935878	      1337 ns/op	    1953 B/op	      35 allocs/op
BenchmarkGojq_Small_Paths-16               	  253573	      4451 ns/op	    6608 B/op	     119 allocs/op
BenchmarkGojq_Small_LeafPaths-16           	  186106	      6476 ns/op	    7576 B/op	     138 allocs/op
BenchmarkGojq_Small_RecursiveDescent-16    	  411072	      2917 ns/op	    4856 B/op	      90 allocs/op
BenchmarkGojq_Small_Recurse-16             	  686396	      1828 ns/op	    4240 B/op	      49 allocs/op
BenchmarkGojq_Small_Walk-16                	  508258	      2396 ns/op	    5701 B/op	      52 allocs/op
BenchmarkGojq_Small_Path-16                	  951091	      1247 ns/op	    1993 B/op	      36 allocs/op
BenchmarkGojq_Small_GetPath-16             	 1000000	      1063 ns/op	    1721 B/op	      29 allocs/op
BenchmarkGojq_Small_SetPath-16             	  693987	      1828 ns/op	    2618 B/op	      43 allocs/op
BenchmarkGojq_Small_DelPaths-16            	  678010	      1838 ns/op	    2426 B/op	      40 allocs/op
BenchmarkGojq_Small_Todate-16              	 1523608	       778.2 ns/op	    1713 B/op	      21 allocs/op
BenchmarkGojq_Small_Date-16                	 1572074	       762.7 ns/op	    1713 B/op	      21 allocs/op
BenchmarkGojq_Small_Now-16                 	 2891966	       416.5 ns/op	    1112 B/op	      10 allocs/op
BenchmarkGojq_Large_KeysUnsorted-16        	    1924	    647332 ns/op	  282879 B/op	    3039 allocs/op
BenchmarkGojq_Small_Any-16                 	  612316	      2043 ns/op	    4508 B/op	      39 allocs/op
BenchmarkGojq_Small_AnyExpr-16             	  543823	      2274 ns/op	    4620 B/op	      49 allocs/op
BenchmarkGojq_Large_AnyExpr-16             	   36729	     32560 ns/op	   27055 B/op	     631 allocs/op
BenchmarkGojq_Small_Add-16                 	 1534798	       777.3 ns/op	    1529 B/op	      28 allocs/op
BenchmarkGojq_Small_AddStrings-16          	 1303410	       919.6 ns/op	    1785 B/op	      35 allocs/op
BenchmarkGojq_Small_Flatten-16             	  929154	      1350 ns/op	    1857 B/op	      38 allocs/op
BenchmarkGojq_Small_Split-16               	 1413562	       851.9 ns/op	    1553 B/op	      21 allocs/op
BenchmarkGojq_Small_Join-16                	 1000000	      1075 ns/op	    1865 B/op	      37 allocs/op
BenchmarkGojq_Small_Slice-16               	 1358260	       883.6 ns/op	    1425 B/op	      21 allocs/op
BenchmarkGojq_Small_SliceString-16         	 2438578	       497.6 ns/op	    1145 B/op	      12 allocs/op
BenchmarkGojq_Small_Plus-16                	 1599392	       760.3 ns/op	    1673 B/op	      21 allocs/op
BenchmarkGojq_Small_PlusStr-16             	 1572123	       761.7 ns/op	    1705 B/op	      23 allocs/op
BenchmarkGojq_Small_AsciiDowncase-16       	  664765	      1802 ns/op	    2714 B/op	      46 allocs/op
BenchmarkGojq_Large_AsciiDowncase-16       	    1483	    809713 ns/op	  539883 B/op	    4653 allocs/op
BenchmarkGojq_Small_Startswith-16          	  693920	      1758 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Large_Startswith-16          	    1473	    807266 ns/op	  536877 B/op	    4651 allocs/op
BenchmarkGojq_Small_Endswith-16            	  690734	      1760 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Small_Ltrimstr-16            	 1000000	      1078 ns/op	    1737 B/op	      30 allocs/op
BenchmarkGojq_Small_Rtrimstr-16            	 1000000	      1085 ns/op	    1737 B/op	      30 allocs/op
BenchmarkGojq_Small_Trimstr-16             	 2690866	       444.2 ns/op	    1225 B/op	      14 allocs/op
BenchmarkGojq_Small_Trim-16                	 3078374	       391.7 ns/op	    1129 B/op	      12 allocs/op
BenchmarkGojq_Small_Ltrim-16               	 2814552	       421.5 ns/op	    1129 B/op	      12 allocs/op
BenchmarkGojq_Small_Rtrim-16               	 2870251	       420.1 ns/op	    1129 B/op	      12 allocs/op
BenchmarkGojq_Small_UTF8ByteLength-16      	 2807196	       429.7 ns/op	    1137 B/op	      12 allocs/op
BenchmarkGojq_Small_Reverse-16             	 1232394	       968.1 ns/op	    1513 B/op	      22 allocs/op
BenchmarkGojq_Small_First-16               	  739010	      1655 ns/op	    3379 B/op	      39 allocs/op
BenchmarkGojq_Large_First-16               	   45037	     25945 ns/op	   25886 B/op	     626 allocs/op
BenchmarkGojq_Small_Last-16                	  605418	      1984 ns/op	    3483 B/op	      43 allocs/op
BenchmarkGojq_Large_Last-16                	   29092	     41218 ns/op	   29832 B/op	     822 allocs/op
BenchmarkGojq_Small_Limit-16               	  721623	      1778 ns/op	    3328 B/op	      42 allocs/op
BenchmarkGojq_Small_Skip-16                	  636360	      1820 ns/op	    2880 B/op	      43 allocs/op
BenchmarkGojq_Small_Reduce-16              	  830334	      1446 ns/op	    2530 B/op	      42 allocs/op
BenchmarkGojq_Small_Foreach-16             	  875166	      1380 ns/op	    2184 B/op	      39 allocs/op
BenchmarkGojq_Small_While-16               	  712393	      1696 ns/op	    1848 B/op	      25 allocs/op
BenchmarkGojq_Small_Until-16               	  479812	      2501 ns/op	    2658 B/op	      53 allocs/op
BenchmarkGojq_Small_Bsearch-16             	 1457180	       822.6 ns/op	    1545 B/op	      30 allocs/op
BenchmarkGojq_Small_Pick-16                	  465765	      2564 ns/op	    4484 B/op	      64 allocs/op
BenchmarkGojq_Small_IN-16                  	  492948	      2491 ns/op	    4147 B/op	      36 allocs/op
BenchmarkGojq_Small_INDEX-16               	  162333	      7233 ns/op	    9657 B/op	     165 allocs/op
BenchmarkGojq_Small_JOIN-16                	  347320	      3286 ns/op	    4171 B/op	      81 allocs/op
BenchmarkGojq_Large_Limit-16               	   88141	     13782 ns/op	   21984 B/op	     433 allocs/op
BenchmarkGojq_Small_Subtract-16            	 1674447	       712.3 ns/op	    1649 B/op	      20 allocs/op
BenchmarkGojq_Small_Multiply-16            	 1646294	       731.4 ns/op	    1649 B/op	      21 allocs/op
BenchmarkGojq_Small_Divide-16              	 1609983	       752.2 ns/op	    1665 B/op	      21 allocs/op
BenchmarkGojq_Small_Min-16                 	  105104	     11695 ns/op	   13571 B/op	     409 allocs/op
BenchmarkGojq_Small_MinBy-16               	   20110	     59430 ns/op	   63632 B/op	    1347 allocs/op
BenchmarkGojq_Sort-16                      	   47224	     25723 ns/op	   26023 B/op	     614 allocs/op
BenchmarkGojq_SortBy-16                    	   12758	     94476 ns/op	   96976 B/op	    2145 allocs/op
BenchmarkGojq_Unique-16                    	   44490	     27342 ns/op	   31902 B/op	     622 allocs/op
BenchmarkGojq_GroupBy-16                   	   10000	    104597 ns/op	  101939 B/op	    2260 allocs/op
BenchmarkGojq_Small_URIEncode-16           	 1613935	       738.4 ns/op	    1289 B/op	      14 allocs/op
BenchmarkGojq_Small_HTMLTemplate-16        	  822304	      1475 ns/op	    2274 B/op	      40 allocs/op
BenchmarkGojq_Small_ArrayDiff-16           	  893743	      1389 ns/op	    2121 B/op	      33 allocs/op
BenchmarkGojq_Small_TryNoError-16          	 1000000	      1145 ns/op	    1913 B/op	      30 allocs/op
BenchmarkGojq_Small_ObjectMerge-16         	  763400	      1668 ns/op	    2906 B/op	      33 allocs/op
BenchmarkGojq_Small_ToJSON-16              	  707724	      1712 ns/op	    2410 B/op	      39 allocs/op
BenchmarkGojq_Small_FromJSON-16            	  837807	      1395 ns/op	    2714 B/op	      31 allocs/op
BenchmarkGojq_Small_ToNumber-16            	 2974976	       406.3 ns/op	    1113 B/op	      11 allocs/op
BenchmarkGojq_Small_ToBoolean-16           	 3042150	       392.0 ns/op	    1113 B/op	      11 allocs/op
BenchmarkGojq_Small_AnyTwoArg-16           	   39853	     30395 ns/op	   26304 B/op	     629 allocs/op
BenchmarkGojq_Complex_LogNormalize-16      	  387310	      3099 ns/op	    4324 B/op	      69 allocs/op
BenchmarkGojq_Complex_ArrayPipeline-16     	   50884	     23939 ns/op	   20408 B/op	     525 allocs/op
BenchmarkGojq_Complex_Aggregation-16       	   50322	     23334 ns/op	   22397 B/op	     555 allocs/op
BenchmarkGojq_Complex_TolerantMap-16       	   41682	     28916 ns/op	   27872 B/op	     641 allocs/op
BenchmarkGojq_Complex_ElifRouting-16       	   44874	     26379 ns/op	   25923 B/op	     595 allocs/op
BenchmarkGojq_Complex_StringBuild-16       	   47850	     25138 ns/op	   20533 B/op	     686 allocs/op
BenchmarkGojq_Complex_EntryFilter-16       	   84847	     14369 ns/op	   18428 B/op	     250 allocs/op
BenchmarkGojq_Small_Sqrt-16                	 2570532	       470.0 ns/op	    1145 B/op	      12 allocs/op
BenchmarkGojq_Small_Log-16                 	 2576654	       483.8 ns/op	    1129 B/op	      12 allocs/op
BenchmarkGojq_Small_Sin-16                 	 2419947	       490.5 ns/op	    1145 B/op	      12 allocs/op
BenchmarkGojq_Small_Atan-16                	 2669546	       449.1 ns/op	    1121 B/op	      11 allocs/op
BenchmarkGojq_Small_Exp-16                 	 2572312	       460.2 ns/op	    1121 B/op	      11 allocs/op
BenchmarkGojq_Small_Tgamma-16              	 2799980	       417.0 ns/op	    1105 B/op	      11 allocs/op
BenchmarkGojq_Small_Hypot-16               	 2585139	       468.3 ns/op	    1273 B/op	      13 allocs/op
BenchmarkGojq_Small_FMA-16                 	 2484651	       478.1 ns/op	    1273 B/op	      13 allocs/op
BenchmarkGojq_Small_Fabs-16                	 2753308	       430.4 ns/op	    1113 B/op	      12 allocs/op
BenchmarkGojq_Small_Abs-16                 	 2761197	       444.2 ns/op	    1113 B/op	      12 allocs/op
BenchmarkGojq_Small_Bind-16                	 1359681	       868.4 ns/op	    1793 B/op	      24 allocs/op
BenchmarkGojq_Small_Def-16                 	 2161828	       553.7 ns/op	    1377 B/op	      15 allocs/op
BenchmarkGojq_Small_StringInterp-16        	 1000000	      1148 ns/op	    2121 B/op	      35 allocs/op
BenchmarkGojq_Small_StringInterpNum-16     	  933864	      1317 ns/op	    2498 B/op	      40 allocs/op
BenchmarkGojq_Small_IsEmptyTrue-16         	 1915006	       629.8 ns/op	    1921 B/op	      18 allocs/op
BenchmarkGojq_Small_IsEmptyFalse-16        	 1000000	      1154 ns/op	    2898 B/op	      33 allocs/op
BenchmarkGojq_Small_Nth-16                 	  506181	      2342 ns/op	    4372 B/op	      49 allocs/op
BenchmarkGojq_Small_Range10-16             	 1000000	      1099 ns/op	    1864 B/op	      18 allocs/op
BenchmarkGojq_Small_RangeLimit-16          	  751320	      1661 ns/op	    3208 B/op	      26 allocs/op
BenchmarkGojq_Small_TestRe_Hit-16          	  827212	      1412 ns/op	    2746 B/op	      26 allocs/op
BenchmarkGojq_Small_TestRe_Miss-16         	  920130	      1388 ns/op	    2704 B/op	      26 allocs/op
BenchmarkGojq_Small_MatchRe_Hit-16         	  362785	      3095 ns/op	    4801 B/op	      66 allocs/op
BenchmarkGojq_Small_MatchRe_Miss-16        	  721593	      1693 ns/op	    2251 B/op	      24 allocs/op
BenchmarkGojq_Small_CaptureRe_Hit-16       	  501430	      2521 ns/op	    4729 B/op	      51 allocs/op
BenchmarkGojq_Small_CaptureRe_Miss-16      	  654295	      1903 ns/op	    2676 B/op	      27 allocs/op
BenchmarkGojq_Small_ScanRe_NoGroups-16     	  420636	      3143 ns/op	    5144 B/op	      57 allocs/op
BenchmarkGojq_Small_SubRe_Hit-16           	  180386	      5728 ns/op	    9016 B/op	      95 allocs/op
BenchmarkGojq_Small_GSubRe_Hit-16          	   78018	     15639 ns/op	   17905 B/op	     248 allocs/op
```

## CLI Throughput: fastjq vs jq

End-to-end JSONL throughput benchmarks comparing `fastjq-bench` (Go CLI using the fastjq library) against `jq` (C implementation, jq-1.7.1-apple). Both read JSONL from stdin, apply a filter per line, write results to `/dev/null`. Both validate JSON: fastjq calls `json.Valid()` per record, jq parses fully. Median of 3 runs. Apple M4 Max.

| Operation | Input | jq (s) | fastjq (s) | Speedup |
|-----------|-------|--------|-------------|---------|
| Identity (`.`) | small (100K lines, ~11MB) | 0.381 | 0.057 | **6.7x** |
| Field access (`.field_2`) | small | 0.162 | 0.044 | **3.7x** |
| Field access (`.field_50`) | large (100 lines, ~16MB) | 0.094 | 0.042 | **2.2x** |
| Delete field (`del(.field_2)`) | small | 0.395 | 0.064 | **6.2x** |
| Object construction (`{field_0, field_2}`) | small | 0.270 | 0.059 | **4.6x** |
| Select all match (`select(.field_2 == "xxx...")`) | small | 0.395 | 0.051 | **7.7x** |
| Select none match (`select(.field_2 == "nope")`) | small | 0.154 | 0.056 | **2.8x** |
| Alternative (`.field_2 // "default"`) | small | 0.183 | 0.049 | **3.7x** |
| Case-insensitive select (`ascii_downcase`) | small | 0.791 | 0.057 | **13.9x** |
| Prefix filter (`startswith`) | small | 0.395 | 0.050 | **7.9x** |
| Field existence (`has`) | small | 0.400 | 0.047 | **8.5x** |
| `to_entries` | small | 0.776 | 0.064 | **12.1x** |
| `keys_unsorted` | small | 0.267 | 0.059 | **4.5x** |

### Key Takeaways (CLI)

- **2.2x–13.9x faster than jq** across this validation-on CLI slice, with the biggest wins on filter/projection work rather than raw field extraction.
- **`to_entries` and case-insensitive filtering are the largest CLI wins** in this benchmark set, because fastjq keeps the work to a streaming scan while jq still pays the full parse cost per line.
- **Large-object field extraction is a modest CLI win** because both tools validate/parse the whole record; the much larger speedups show up when you call the library directly on already-valid JSON and skip the extra validation pass.
- **The CLI numbers are intentionally conservative**: they include JSON validation and process startup overhead that do not apply to the hot-path library API.

### Reproducing

```bash
chmod +x bench_vs_jq.sh
./bench_vs_jq.sh
```
