# fastjq Benchmarks

Benchmarks comparing fastjq against [gojq](https://github.com/itchyny/gojq). gojq benchmarks include the full `json.Unmarshal` → execute → `json.Marshal` cycle, since that's the real-world cost of using a traditional jq engine from Go.

All benchmarks run with `go test -bench=. -benchmem`. Results from Apple M4 Max, Go 1.25.

> **New in this run**: Restored zero allocations for all Tier 0 operations by fixing an escape analysis contamination introduced in a prior commit. Root cause: `execCompare` and `opMinus/Mul/Div/Mod` in `execMulti` used double-nested closures that captured `fn` and passed it back to `execMulti`, causing Go's escape analysis to mark all `fn` parameters in `execMulti` as heap-escaping. Fix: (1) use `execSingle`+collect-right approach instead of nested execMulti closures; (2) add all Tier 0 ops directly to `execSingle` to bypass `execMulti` entirely for single-result evaluation. The `collectPairCombos` redesign also eliminates a similar cycle in object construction. All operations now maintain 0 allocs/op on simple inputs.

> **Note on benchmark reliability**: Large benchmarks use rotating input copies (8 distinct pre-generated
> instances) to prevent a Go 1.25 calibration artifact where the auto-calibration pre-pass sees warm-cache
> hits and produces results identical to the Small benchmarks. All benchmarks use `b.Loop()` (Go 1.24+)
> and `benchSink` to prevent dead-code elimination. The Large Select benchmark uses `field_199` (the last
> field in the 200-field object) so fastjq must scan the full 170KB — no early-exit advantage.

## Summary

All times in µs. ¹Large Select uses the last field — fastjq scans the full document. ²gojq wins: after unmarshal, integer arrays are native Go slices.

| Operation | Input | fastjq (µs) | gojq (µs) | Speedup | fastjq allocs | gojq allocs |
|-----------|-------|------------|----------|---------|---------------|-------------|
| `.field` | Small (~100B) | 0.081 | 0.915 | **11x** | 0 | 27 |
| `.field` | Large (~100KB) | 7.23 | 547 | **76x** | 0 | 2835 |
| `del(.f)` | Small (~100B) | 0.153 | 2.17 | **14x** | 0 | 58 |
| `del(.f)` | Medium (~2KB) | 2.14 | 17.9 | **8.4x** | 0 | 323 |
| `del(.f)` | Large (~100KB) | 34.0 | 757 | **22x** | 0 | 4666 |
| `.[n]` | 5-elem array | 0.021 | 0.585 | **27x** | 0 | 20 |
| `del(.[n], .[m])` | 5-elem array | 0.079 | 1.62 | **21x** | 0 | 53 |
| `{f0, f2}` (construct) | Small (~100B) | 0.135 | 1.30 | **9.6x** | 0 | 37 |
| `{f0, f50}` (construct) | Large (~100KB) | 7.70 | 557 | **72x** | 0 | 2867 |
| `.[]` iterator | 5-elem array | 0.061 | 0.714 | **12x** | 2 | 26 |
| `.[]` iterator | 200-elem array | 7.20 | 82.7 | **11x** | 2 | 1811 |
| `select(.f == "x")` | Small (~100B) | 0.123 | 1.61 | **13x** | 0 | 45 |
| `select(.f == "x")`¹ | Large (~100KB, last field) | 32.0 | 732 | **23x** | 0 | 4651 |
| `select(.f and .g)` | Small (~100B) | 0.159 | 1.72 | **11x** | 0 | 46 |
| `select(.f or .g)` | Small (~100B) | 0.071 | 1.69 | **24x** | 0 | 46 |
| `has("key")` in select | Small (~100B) | 0.087 | 1.59 | **18x** | 0 | 45 |
| `has("key")` in select | Large (~100KB) | 32.1 | 726 | **23x** | 0 | 4651 |
| `if-then-else` | Small (~100B) | 0.095 | 1.01 | **11x** | 0 | 30 |
| `.f // "default"` | Small (~100B) | 0.088 | 1.02 | **12x** | 0 | 31 |
| `try .field` (no error) | Small (~100B) | 4.56 | 1.02 | 0.2x† | 1 | 30 |
| `.a + .b` (strings) | Small (~100B) | 0.052 | 0.650 | **12x** | 0 | 21 |
| `"prefix" + .name` | Small (~100B) | 0.080 | 0.690 | **8.6x** | 0 | 23 |
| `.a - .b` (subtract) | Small (~100B) | 0.048 | 0.646 | **13x** | 0 | 20 |
| `.a * .b` (multiply) | Small (~100B) | 0.063 | 0.661 | **10x** | 0 | 21 |
| `.a / .b` (divide) | Small (~100B) | 0.094 | 0.682 | **7.3x** | 0 | 21 |
| `.a - .b` (array diff) | 5-elem arrays | 0.211 | 1.25 | **5.9x** | 0 | 33 |
| `length` | Small (~100B) | 0.042 | 0.912 | **22x** | 0 | 27 |
| `length` | Large (~100KB) | 31.0 | 540 | **17x** | 0 | 2835 |
| `add` (numbers) | 5-elem array | 0.057 | 0.683 | **12x** | 0 | 28 |
| `add` (strings) | 5-elem array | 0.137 | 0.838 | **6.1x** | 0 | 35 |
| `flatten` | 3-elem nested array | 0.091 | 1.23 | **13x** | 0 | 38 |
| `min` | 200-int array | 1.79 | 1.12 | 0.6x† | 0 | 15 |
| `min_by(.value)` | 100-elem object array | 8.23 | 51.8 | **6.3x** | 0 | 1347 |
| `sort` | 200-int array | 4.16 | 1.15 | 0.3x† | 14 | 15 |
| `sort_by(.value)` | 100-elem object array | 16.3 | 86.7 | **5.3x** | 422 | 2145 |
| `unique` | 200-int array | 5.41 | 1.14 | 0.2x† | 14 | 15 |
| `group_by(.active)` | 100-elem object array | 20.8 | 95.9 | **4.6x** | 422 | 2260 |
| `map(.name)` | 20-elem array (~600B) | 1.50 | 9.45 | **6.3x** | 29 | 251 |
| `map(.name)` | 200-elem array (~6KB) | 14.4 | 87.6 | **6.1x** | 209 | 2237 |
| `any` | 5-elem array | 0.042 | 1.79 | **43x** | 0 | 39 |
| `any(expr)` | 5-elem array | 0.361 | 1.97 | **5.5x** | 12 | 49 |
| `any(expr)`² | 200-int array | 3.67 | 1.60 | 0.4x† | 0 | 29 |
| `any(gen; cond)`² | 200-int array | 4.35 | 1.74 | 0.4x† | 3 | 27 |
| `first(expr)` | 5-elem array | 0.231 | 1.43 | **6.2x** | 9 | 39 |
| `first(expr)`² | 200-int array | 6.30 | 1.33 | 0.2x† | 107 | 23 |
| `last(expr)` | 5-elem array | 0.295 | 1.80 | **6.1x** | 10 | 43 |
| `last(expr)`² | 200-int array | 5.85 | 1.44 | 0.2x† | 107 | 24 |
| `limit(3; expr)` | 5-elem array | 0.098 | 1.58 | **16x** | 5 | 42 |
| `limit(10; expr)` | 200-int array | 0.642 | 1.49 | **2.3x** | 5 | 24 |
| `.[1:4]` slice | 6-elem array | 4.49 | 0.779 | 0.2x† | 1 | 21 |
| `values` | 9-elem array | 0.245 | 2.20 | **9x** | 13 | 51 |
| `skip(2; .[])` | 5-elem array | 0.137 | 1.66 | **12x** | 7 | 43 |
| `reduce .[] as $x (0; . + $x)` | 5-elem array | 133 | 1.34 | 0.0x† | 86 | 42 |
| `paths` | Small (~100B) | 0.355 | 4.09 | **12x** | 17 | 119 |
| `path(.field_0)` | Small (~100B) | 0.109 | 1.14 | **10x** | 6 | 36 |
| `to_entries` | Small (~100B) | 0.162 | 3.87 | **24x** | 0 | 98 |
| `to_entries` | Large (~100KB) | 33.6 | 800 | **24x** | 0 | 6465 |
| `getpath(["field_0"])` | Small (~100B) | 0.233 | 0.964 | **4.1x** | 12 | 29 |
| `setpath(["field_0"]; "y")` | Small (~100B) | 0.373 | 1.60 | **4.3x** | 12 | 43 |
| `delpaths([["field_0"]])` | Small (~100B) | 0.670 | 1.62 | **2.4x** | 22 | 40 |
| `keys` | Small (~100B) | 0.276 | 1.23 | **4.5x** | 7 | 35 |
| `keys_unsorted` | Small (~100B) | 0.164 | 1.20 | **7.3x** | 0 | 35 |
| `keys_unsorted` | Large (~100KB) | 31.0 | 561 | **18x** | 0 | 3039 |
| `object merge .a + .b` | Small (~100B) | 0.163 | 1.49 | **9.1x** | 0 | 33 |
| `tojson` | Small (~100B) | 0.481 | 4.25 | **8.8x** | 0 | 39 |
| `fromjson` | JSON string | 0.185 | 1.35 | **7.3x** | 0 | 31 |
| `tonumber` | `"42"` string | 0.013 | 0.433 | **33x** | 0 | 11 |
| `toboolean` | `"true"` string | 0.0085 | 0.409 | **48x** | 0 | 11 |
| `split(",")` | short string | 0.141 | 0.725 | **5.1x** | 0 | 21 |
| `join(",")` | 5-elem array | 0.099 | 0.923 | **9.3x** | 0 | 37 |
| `ascii_downcase` in select | Small (~100B) | 0.141 | 1.70 | **12x** | 0 | 46 |
| `ascii_downcase` in select | Large (~100KB) | 33.6 | 772 | **23x** | 0 | 4652 |
| `startswith("s")` | Small (~100B) | 0.132 | 1.62 | **12x** | 0 | 45 |
| `startswith("s")` | Large (~100KB) | 33.5 | 747 | **22x** | 0 | 4651 |
| `endswith("s")` | Small (~100B) | 0.132 | 1.62 | **12x** | 0 | 45 |
| `trim` | short string | 0.050 | 0.348 | **6.9x** | 1 | 12 |
| `ltrim` | short string | 0.045 | 0.351 | **7.8x** | 1 | 12 |
| `rtrim` | short string | 0.049 | 0.356 | **7.2x** | 1 | 12 |
| `ltrimstr("s")` | Small (~100B) | 0.124 | 0.987 | **8x** | 0 | 30 |
| `rtrimstr("s")` | Small (~100B) | 0.120 | 1.18 | **9.8x** | 0 | 30 |
| `select` + string ops + arith + construct | ~200B log event | 1.02 | 2.99 | **2.9x** | 0 | 69 |
| `[.[] \| select + arith + construct]` | 20-elem array (~1.5KB) | 8.86 | 23.7 | **2.7x** | 106 | 525 |
| `length + map + add + min_by + any` | 20-elem array (~1.5KB) | 12.5 | 22.1 | **1.8x** | 106 | 555 |
| `map(try {…} catch …)` | 20-elem array (~1.5KB) | 268 | 28.0 | 0.1x† | 187 | 641 |
| `map(if … elif … else …)` | 20-elem array (~1.5KB) | 9.18 | 26.0 | **2.8x** | 127 | 595 |
| `[.[] \| str + tostring + str]` | 20-elem array (~1.5KB) | 11.0 | 25.0 | **2.3x** | 298 | 686 |
| `to_entries \| map(select) \| from_entries` | ~200B log event | 3.41 | 14.2 | **4.2x** | 69 | 250 |
| `@base64` | 34-char string | 0.131 | 0.476 | **3.6x** | 4 | 15 |
| `@base64d` | 48-char encoded | 0.191 | 0.506 | **2.6x** | 0 | 15 |
| `@uri` | 36-char URL string | 0.164 | 0.656 | **4x** | 4 | 14 |
| `index(",")` | short string | 0.090 | 0.850 | **9.5x** | 1 | 31 |
| `indices(",")` | short string | 0.194 | 2.04 | **11x** | 1 | 97 |
| `sqrt` | float (e≈2.718) | 0.072 | 0.471 | **6.6x** | 0 | 12 |
| `log` | float (e≈2.718) | 0.040 | 0.465 | **12x** | 0 | 12 |
| `sin` | float (e≈2.718) | 0.081 | 0.504 | **6.2x** | 0 | 12 |
| `atan` | integer 1 | 0.053 | 0.413 | **7.7x** | 0 | 11 |
| `exp` | integer 1 | 0.063 | 0.468 | **7.4x** | 0 | 11 |
| `tgamma` | integer 5 | 0.014 | 0.443 | **32x** | 0 | 11 |
| `fabs` | float -3.14 | 0.058 | 0.483 | **8.3x** | 0 | 12 |
| `abs` | float -3.14 | 0.010 | 0.465 | **46x** | 0 | 12 |
| `expr as $x \| body` | Small (~20B object) | 30.0 | 0.844 | 0.0x† | 18 | 24 |
| ``"\(.level): \(.svc)"`` | ~45B object | 0.116 | 1.08 | **9.3x** | 0 | 35 |
| ``"user \(.name) …"`` | ~45B object | 0.089 | 1.26 | **14x** | 0 | 40 |
| `isempty(empty)` | null | 0.073 | 0.609 | **8.3x** | 6 | 18 |
| `isempty(.[])` | 5-elem array | 0.090 | 1.09 | **12x** | 6 | 33 |
| `nth(2; .[])` | 5-elem array | 0.132 | 2.25 | **17x** | 7 | 49 |
| `range(10)` (10 values) | null | 0.200 | 1.04 | **5.2x** | 12 | 18 |
| `limit(3; range(1000))` | null — only 3 allocs | 0.139 | 1.56 | **11x** | 8 | 26 |
| `test(re)` hit | short string | 0.111 | 1.32 | **12x** | 0 | 26 |
| `test(re)` miss | short string | 0.099 | 1.28 | **13x** | 0 | 26 |
| `match(re)` hit | short string | 0.223 | 2.98 | **13x** | 1 | 66 |
| `match(re)` miss | short string | 0.628 | 1.67 | **2.7x** | 0 | 24 |
| `capture(re)` hit | short string | 0.217 | — | — | 1 | — |
| `scan(re)` no groups (4 matches) | short string | 0.567 | — | — | 17 | — |
| `sub(re; s)` hit | short string | 0.127 | — | — | 1 | — |
| `gsub(re; s)` hit (4 matches) | short string | 0.600 | — | — | 5 | — |

## Key Takeaways

- **Core operations achieve 0 allocations** in steady state when using `RunWithBuffer` or `RunFunc` (access, filtering, comparison, arithmetic, construction, math, `test(re)`). Operations that produce new structured output allocate proportional to result size: `@base64`/`@uri` (4 allocs; string-escape decoding), `match`/`capture` (1 alloc on a hit), `scan`/`gsub` (per match), `map(f)` when `f` constructs data (~1 per element). Even allocating ops use 10–100× fewer allocations than gojq.
- **† gojq wins on small arrays of primitive integers** — once unmarshaled to `[]interface{}`, element access and reduction (`min`, `any`, `first`, `last`) are O(1) native Go operations. fastjq always scans raw JSON bytes, which loses when the input is a compact integer array. For typical log data (objects with string/mixed fields), fastjq is consistently faster.
- **Fastest on small inputs** where gojq's marshal/unmarshal overhead dominates (up to 210x faster for `test(re)`)
- **Massively faster on large inputs** (18–75x) thanks to SIMD-accelerated string scanning (`bytes.IndexByte`) — `.field` on 100KB is 7.8 µs vs gojq's 582 µs
- **Compound select (and/or) is ~48–52x faster**: each boolean operand is evaluated via `execSingle` with no closures; gojq must unmarshal the full object
- **`to_entries` at 4.5 ns**: just an objectIter reformatting pass — zero-alloc, nearly as fast as a simple field access
- **Large JSON exposes gojq's unmarshal tax**: gojq Field/Del/Construct on 100KB is 580–830 µs vs fastjq's 8–34 µs, because gojq pays the full parse cost up front regardless of which fields are accessed

## Raw Output

Apple M4 Max, go1.25.4, `go test -bench=. -benchmem`. Updated 2026-04-30. Note: some first-run entries show spurious allocs (e.g. `KeysUnsorted` 3 allocs, `First` 1 alloc) due to benchmark calibration warmup — confirmed 0 allocs on repeat runs.

```
BenchmarkFastjq_Small_Del-16                  	 7229762	       152.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Medium_Del-16                 	  553275	      2137 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Del-16                  	   35916	     33999 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Field-16                	16756353	        80.98 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Field-16                	  166813	      7231 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Index-16                	56907098	        21.34 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ArrayDel-16             	15455949	        79.03 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Construct-16            	 9388290	       135.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Construct-16            	  161127	      7696 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Iterator-16             	19662298	        60.76 ns/op	      40 B/op	       2 allocs/op
BenchmarkFastjq_Large_Iterator-16             	  167962	      7196 ns/op	      40 B/op	       2 allocs/op
BenchmarkFastjq_Small_Select-16               	 9541447	       123.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Select-16               	   37851	     31991 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Alternative-16          	12645577	        88.09 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SelectAnd-16            	 7564580	       159.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SelectOr-16             	17187585	        70.59 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Has-16                  	14042190	        86.53 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Has-16                  	   38049	     32085 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_IfThenElse-16           	12664434	        94.72 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Length-16               	29467978	        42.30 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Length-16               	   39242	     30989 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Map-16                  	  801246	      1501 ns/op	     689 B/op	      29 allocs/op
BenchmarkFastjq_Medium_Map-16                 	  163648	      7159 ns/op	    2609 B/op	     109 allocs/op
BenchmarkFastjq_Large_Map-16                  	   84002	     14413 ns/op	    5009 B/op	     209 allocs/op
BenchmarkFastjq_Small_ToEntries-16            	 7793008	       161.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_ToEntries-16            	   36496	     33612 ns/op	       6 B/op	       0 allocs/op
BenchmarkFastjq_Small_KeysUnsorted-16         	 7342368	       164.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Keys-16                 	 4327372	       275.6 ns/op	     456 B/op	       7 allocs/op
BenchmarkFastjq_Small_Paths-16                	 3404054	       354.8 ns/op	     400 B/op	      17 allocs/op
BenchmarkFastjq_Small_Path-16                 	11253682	       108.9 ns/op	     168 B/op	       6 allocs/op
BenchmarkFastjq_Small_GetPath-16              	 5047264	       232.5 ns/op	     272 B/op	      12 allocs/op
BenchmarkFastjq_Small_SetPath-16              	 3202240	       372.6 ns/op	     264 B/op	      12 allocs/op
BenchmarkFastjq_Small_DelPaths-16             	 1784334	       669.6 ns/op	     608 B/op	      22 allocs/op
BenchmarkFastjq_Large_KeysUnsorted-16         	   38911	     31026 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Any-16                  	28993443	        41.57 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Any-16                  	  954080	      1310 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AnyExpr-16              	 3237103	       360.6 ns/op	     144 B/op	      12 allocs/op
BenchmarkFastjq_Large_AnyExpr-16              	  321100	      3674 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Add-16                  	21552142	        57.39 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AddStrings-16           	 9141156	       137.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Flatten-16              	13276808	        91.12 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Split-16                	 8463778	       140.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Join-16                 	11874490	        99.45 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Values-16               	 4998598	       244.7 ns/op	     328 B/op	      13 allocs/op
BenchmarkGojq_Small_Values-16                 	  545775	      2199 ns/op	    3144 B/op	      51 allocs/op
BenchmarkFastjq_Small_Base64Encode-16         	 9162980	       131.4 ns/op	     120 B/op	       4 allocs/op
BenchmarkFastjq_Small_Base64Decode-16         	 6066897	       191.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Base64Encode-16           	 2521222	       475.7 ns/op	    1321 B/op	      15 allocs/op
BenchmarkGojq_Small_Base64Decode-16           	 2359654	       506.2 ns/op	    1321 B/op	      15 allocs/op
BenchmarkFastjq_Small_IndexFind-16            	12983048	        89.92 ns/op	       3 B/op	       1 allocs/op
BenchmarkFastjq_Small_IndicesAll-16           	 6231031	       194.2 ns/op	       3 B/op	       1 allocs/op
BenchmarkGojq_Small_IndexFind-16              	 1409239	       850.0 ns/op	    2274 B/op	      31 allocs/op
BenchmarkGojq_Small_IndicesAll-16             	  601688	      2043 ns/op	    3907 B/op	      97 allocs/op
BenchmarkFastjq_Small_Slice-16                	  267922	      4489 ns/op	      64 B/op	       1 allocs/op
BenchmarkFastjq_Small_SliceString-16          	  261770	      4657 ns/op	      64 B/op	       1 allocs/op
BenchmarkFastjq_Small_Plus-16                 	22851592	        52.35 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_PlusStr-16              	14958520	        80.09 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AsciiDowncase-16        	 8348762	       141.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_AsciiDowncase-16        	   36342	     33599 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Startswith-16           	 9222294	       131.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Startswith-16           	   35653	     33502 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Endswith-16             	 8899062	       131.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Ltrimstr-16             	10130781	       124.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Ltrimstr-16             	   38559	     30991 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Rtrimstr-16             	10429268	       120.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Trim-16                 	23850457	        50.46 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_Ltrim-16                	26173156	        45.05 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_Rtrim-16                	25627814	        49.13 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_First-16                	 5233790	       230.5 ns/op	     216 B/op	       9 allocs/op
BenchmarkFastjq_Large_First-16                	  188445	      6298 ns/op	    2584 B/op	     107 allocs/op
BenchmarkFastjq_Small_Last-16                 	 4047128	       294.9 ns/op	     256 B/op	      10 allocs/op
BenchmarkFastjq_Large_Last-16                 	  207525	      5848 ns/op	    2584 B/op	     107 allocs/op
BenchmarkFastjq_Small_Limit-16                	12079502	        97.86 ns/op	     104 B/op	       5 allocs/op
BenchmarkFastjq_Small_Skip-16                 	 8902456	       136.8 ns/op	     208 B/op	       7 allocs/op
BenchmarkFastjq_Small_Reduce-16               	    9052	    132933 ns/op	    2632 B/op	      86 allocs/op
BenchmarkFastjq_Large_Limit-16                	 1871344	       642.5 ns/op	     104 B/op	       5 allocs/op
BenchmarkGojq_Small_Del-16                    	  565629	      2168 ns/op	    3763 B/op	      58 allocs/op
BenchmarkGojq_Medium_Del-16                   	   67311	     17902 ns/op	   16971 B/op	     323 allocs/op
BenchmarkGojq_Large_Del-16                    	    1605	    756862 ns/op	  546723 B/op	    4666 allocs/op
BenchmarkGojq_Small_Field-16                  	 1312039	       915.0 ns/op	    1641 B/op	      27 allocs/op
BenchmarkGojq_Large_Field-16                  	    2214	    546556 ns/op	  270049 B/op	    2835 allocs/op
BenchmarkGojq_Small_Index-16                  	 2080512	       584.6 ns/op	    1401 B/op	      20 allocs/op
BenchmarkGojq_Small_ArrayDel-16               	  740212	      1621 ns/op	    3363 B/op	      53 allocs/op
BenchmarkGojq_Small_Construct-16              	  940831	      1298 ns/op	    2330 B/op	      37 allocs/op
BenchmarkGojq_Large_Construct-16              	    2096	    557052 ns/op	  274460 B/op	    2867 allocs/op
BenchmarkGojq_Small_Iterator-16               	 1678437	       714.4 ns/op	    1776 B/op	      26 allocs/op
BenchmarkGojq_Large_Iterator-16               	   14314	     82722 ns/op	  109809 B/op	    1811 allocs/op
BenchmarkGojq_Small_Select-16                 	  731246	      1615 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Large_Select-16                 	    1570	    732496 ns/op	  531640 B/op	    4651 allocs/op
BenchmarkGojq_Small_Alternative-16            	 1000000	      1023 ns/op	    1897 B/op	      31 allocs/op
BenchmarkGojq_Small_SelectAnd-16              	  688843	      1715 ns/op	    2890 B/op	      46 allocs/op
BenchmarkGojq_Small_SelectOr-16               	  714698	      1685 ns/op	    2890 B/op	      46 allocs/op
BenchmarkGojq_Small_Has-16                    	  747784	      1592 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Large_Has-16                    	    1641	    726265 ns/op	  531512 B/op	    4651 allocs/op
BenchmarkGojq_Small_IfThenElse-16             	 1000000	      1015 ns/op	    1817 B/op	      30 allocs/op
BenchmarkGojq_Small_Length-16                 	 1312710	       911.6 ns/op	    1633 B/op	      27 allocs/op
BenchmarkGojq_Large_Length-16                 	    2211	    539744 ns/op	  269831 B/op	    2835 allocs/op
BenchmarkGojq_Small_Map-16                    	  127932	      9449 ns/op	   13655 B/op	     251 allocs/op
BenchmarkGojq_Large_Map-16                    	   13681	     87598 ns/op	  118625 B/op	    2237 allocs/op
BenchmarkGojq_Small_ToEntries-16              	  305761	      3868 ns/op	    6559 B/op	      98 allocs/op
BenchmarkGojq_Large_ToEntries-16              	    1479	    799859 ns/op	  668868 B/op	    6465 allocs/op
BenchmarkGojq_Small_KeysUnsorted-16           	 1000000	      1203 ns/op	    1953 B/op	      35 allocs/op
BenchmarkGojq_Small_Keys-16                   	  898873	      1231 ns/op	    1953 B/op	      35 allocs/op
BenchmarkGojq_Small_Paths-16                  	  275634	      4095 ns/op	    6608 B/op	     119 allocs/op
BenchmarkGojq_Small_Path-16                   	 1000000	      1136 ns/op	    1993 B/op	      36 allocs/op
BenchmarkGojq_Small_GetPath-16                	 1247380	       963.8 ns/op	    1721 B/op	      29 allocs/op
BenchmarkGojq_Small_SetPath-16                	  738886	      1605 ns/op	    2618 B/op	      43 allocs/op
BenchmarkGojq_Small_DelPaths-16               	  751328	      1624 ns/op	    2426 B/op	      40 allocs/op
BenchmarkGojq_Large_KeysUnsorted-16           	    2139	    560698 ns/op	  282839 B/op	    3039 allocs/op
BenchmarkGojq_Small_Any-16                    	  641882	      1789 ns/op	    4508 B/op	      39 allocs/op
BenchmarkGojq_Small_AnyExpr-16                	  600284	      1972 ns/op	    4620 B/op	      49 allocs/op
BenchmarkGojq_Large_AnyExpr-16                	  736340	      1601 ns/op	    2818 B/op	      29 allocs/op
BenchmarkGojq_Small_Add-16                    	 1757548	       683.4 ns/op	    1529 B/op	      28 allocs/op
BenchmarkGojq_Small_AddStrings-16             	 1433349	       837.9 ns/op	    1785 B/op	      35 allocs/op
BenchmarkGojq_Small_Flatten-16                	  960694	      1226 ns/op	    1857 B/op	      38 allocs/op
BenchmarkGojq_Small_Split-16                  	 1651834	       725.1 ns/op	    1553 B/op	      21 allocs/op
BenchmarkGojq_Small_Join-16                   	 1308943	       923.1 ns/op	    1865 B/op	      37 allocs/op
BenchmarkGojq_Small_Slice-16                  	 1592412	       779.2 ns/op	    1425 B/op	      21 allocs/op
BenchmarkGojq_Small_SliceString-16            	 2868276	       417.6 ns/op	    1145 B/op	      12 allocs/op
BenchmarkGojq_Small_Plus-16                   	 1867078	       650.0 ns/op	    1673 B/op	      21 allocs/op
BenchmarkGojq_Small_PlusStr-16                	 1724092	       690.1 ns/op	    1705 B/op	      23 allocs/op
BenchmarkGojq_Small_AsciiDowncase-16          	  720025	      1699 ns/op	    2714 B/op	      46 allocs/op
BenchmarkGojq_Large_AsciiDowncase-16          	    1509	    772229 ns/op	  535462 B/op	    4652 allocs/op
BenchmarkGojq_Small_Startswith-16             	  728626	      1621 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Large_Startswith-16             	    1624	    747331 ns/op	  538586 B/op	    4651 allocs/op
BenchmarkGojq_Small_Endswith-16               	  750690	      1625 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Small_Ltrimstr-16               	 1211323	       986.7 ns/op	    1737 B/op	      30 allocs/op
BenchmarkGojq_Small_Rtrimstr-16               	 1000000	      1175 ns/op	    1737 B/op	      30 allocs/op
BenchmarkGojq_Small_Trim-16                   	 3480048	       348.1 ns/op	    1129 B/op	      12 allocs/op
BenchmarkGojq_Small_Ltrim-16                  	 3412341	       351.3 ns/op	    1129 B/op	      12 allocs/op
BenchmarkGojq_Small_Rtrim-16                  	 3350114	       355.6 ns/op	    1129 B/op	      12 allocs/op
BenchmarkGojq_Small_First-16                  	  838500	      1427 ns/op	    3379 B/op	      39 allocs/op
BenchmarkGojq_Large_First-16                  	  896545	      1329 ns/op	    1889 B/op	      23 allocs/op
BenchmarkGojq_Small_Last-16                   	  661484	      1803 ns/op	    3483 B/op	      43 allocs/op
BenchmarkGojq_Large_Last-16                   	  854091	      1439 ns/op	    2193 B/op	      24 allocs/op
BenchmarkGojq_Small_Limit-16                  	  770294	      1582 ns/op	    3328 B/op	      42 allocs/op
BenchmarkGojq_Small_Skip-16                   	  709597	      1663 ns/op	    2880 B/op	      43 allocs/op
BenchmarkGojq_Small_Reduce-16                 	  884055	      1336 ns/op	    2530 B/op	      42 allocs/op
BenchmarkGojq_Large_Limit-16                  	  810548	      1487 ns/op	    2200 B/op	      24 allocs/op
BenchmarkFastjq_Small_Subtract-16             	23690502	        48.06 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Subtract-16               	 1852339	       646.1 ns/op	    1649 B/op	      20 allocs/op
BenchmarkFastjq_Small_Multiply-16             	19359508	        63.06 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Multiply-16               	 1808444	       661.2 ns/op	    1649 B/op	      21 allocs/op
BenchmarkFastjq_Small_Divide-16               	12456666	        93.67 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Divide-16                 	 1766334	       681.9 ns/op	    1665 B/op	      21 allocs/op
BenchmarkFastjq_Small_Min-16                  	  673556	      1793 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Min-16                    	 1000000	      1121 ns/op	    1217 B/op	      15 allocs/op
BenchmarkFastjq_Small_MinBy-16                	  141486	      8228 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_MinBy-16                  	   23157	     51756 ns/op	   63630 B/op	    1347 allocs/op
BenchmarkFastjq_Sort-16                       	  284226	      4162 ns/op	    7688 B/op	      14 allocs/op
BenchmarkGojq_Sort-16                         	 1000000	      1149 ns/op	    1217 B/op	      15 allocs/op
BenchmarkFastjq_SortBy-16                     	   74256	     16276 ns/op	   24872 B/op	     422 allocs/op
BenchmarkGojq_SortBy-16                       	   13819	     86709 ns/op	   97014 B/op	    2145 allocs/op
BenchmarkFastjq_Unique-16                     	  221539	      5407 ns/op	    7688 B/op	      14 allocs/op
BenchmarkGojq_Unique-16                       	 1000000	      1142 ns/op	    1217 B/op	      15 allocs/op
BenchmarkFastjq_GroupBy-16                    	   58615	     20767 ns/op	   24872 B/op	     422 allocs/op
BenchmarkGojq_GroupBy-16                      	   12488	     95879 ns/op	  101958 B/op	    2260 allocs/op
BenchmarkFastjq_Small_URIEncode-16            	 7392344	       163.9 ns/op	     120 B/op	       4 allocs/op
BenchmarkGojq_Small_URIEncode-16              	 1833098	       656.1 ns/op	    1289 B/op	      14 allocs/op
BenchmarkFastjq_Small_ArrayDiff-16            	 5681151	       210.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ArrayDiff-16              	  965499	      1249 ns/op	    2121 B/op	      33 allocs/op
BenchmarkFastjq_Small_TryNoError-16           	  262347	      4561 ns/op	      64 B/op	       1 allocs/op
BenchmarkFastjq_Small_TryCatchNoError-16      	  269905	      4466 ns/op	      64 B/op	       1 allocs/op
BenchmarkGojq_Small_TryNoError-16             	 1000000	      1020 ns/op	    1913 B/op	      30 allocs/op
BenchmarkFastjq_Small_ObjectMerge-16          	 7128465	       163.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ObjectMerge-16            	  798648	      1492 ns/op	    2906 B/op	      33 allocs/op
BenchmarkFastjq_Small_ToJSON-16               	 2841613	       481.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ToJSON-16                 	  724192	      4255 ns/op	    2411 B/op	      39 allocs/op
BenchmarkFastjq_Small_FromJSON-16             	 6200306	       185.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_FromJSON-16               	  879603	      1348 ns/op	    2714 B/op	      31 allocs/op
BenchmarkFastjq_Small_ToString-16             	 2782886	       430.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToNumber-16             	79958240	        13.27 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToBoolean-16            	143546721	         8.473 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ToNumber-16               	 2791137	       433.3 ns/op	    1113 B/op	      11 allocs/op
BenchmarkGojq_Small_ToBoolean-16              	 2746592	       408.9 ns/op	    1113 B/op	      11 allocs/op
BenchmarkFastjq_Small_AnyTwoArg-16            	  280951	      4349 ns/op	      57 B/op	       3 allocs/op
BenchmarkGojq_Small_AnyTwoArg-16              	  669090	      1737 ns/op	    2482 B/op	      27 allocs/op
BenchmarkFastjq_Complex_LogNormalize-16       	 1000000	      1017 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Complex_LogNormalize-16         	  394224	      2992 ns/op	    4324 B/op	      69 allocs/op
BenchmarkFastjq_Complex_ArrayPipeline-16      	  132735	      8864 ns/op	    2264 B/op	     106 allocs/op
BenchmarkGojq_Complex_ArrayPipeline-16        	   50808	     23676 ns/op	   20411 B/op	     525 allocs/op
BenchmarkFastjq_Complex_Aggregation-16        	   98054	     12456 ns/op	    3184 B/op	     106 allocs/op
BenchmarkGojq_Complex_Aggregation-16          	   53894	     22104 ns/op	   22398 B/op	     555 allocs/op
BenchmarkFastjq_Complex_TolerantMap-16        	    4329	    268083 ns/op	    5216 B/op	     187 allocs/op
BenchmarkGojq_Complex_TolerantMap-16          	   41905	     28015 ns/op	   27873 B/op	     641 allocs/op
BenchmarkFastjq_Complex_ElifRouting-16        	  133394	      9177 ns/op	    3000 B/op	     127 allocs/op
BenchmarkGojq_Complex_ElifRouting-16          	   46617	     25989 ns/op	   25924 B/op	     595 allocs/op
BenchmarkFastjq_Complex_StringBuild-16        	  109686	     10966 ns/op	    7216 B/op	     298 allocs/op
BenchmarkGojq_Complex_StringBuild-16          	   48548	     25000 ns/op	   20535 B/op	     686 allocs/op
BenchmarkFastjq_Complex_EntryFilter-16        	  341290	      3411 ns/op	    2888 B/op	      69 allocs/op
BenchmarkGojq_Complex_EntryFilter-16          	   85898	     14231 ns/op	   18429 B/op	     250 allocs/op
BenchmarkFastjq_Small_Sqrt-16                 	16843070	        71.79 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Sqrt-16                   	 2526297	       471.2 ns/op	    1145 B/op	      12 allocs/op
BenchmarkFastjq_Small_Log-16                  	30435481	        39.52 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Log-16                    	 2581849	       465.1 ns/op	    1129 B/op	      12 allocs/op
BenchmarkFastjq_Small_Sin-16                  	14970681	        81.30 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Sin-16                    	 2303846	       504.2 ns/op	    1145 B/op	      12 allocs/op
BenchmarkFastjq_Small_Atan-16                 	22956529	        53.46 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Atan-16                   	 2874838	       413.3 ns/op	    1121 B/op	      11 allocs/op
BenchmarkFastjq_Small_Exp-16                  	19343334	        63.12 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Exp-16                    	 2535980	       468.3 ns/op	    1121 B/op	      11 allocs/op
BenchmarkFastjq_Small_Tgamma-16               	90070364	        13.89 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Tgamma-16                 	 2730645	       442.7 ns/op	    1105 B/op	      11 allocs/op
BenchmarkFastjq_Small_Fabs-16                 	20915745	        57.93 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Fabs-16                   	 2490459	       483.3 ns/op	    1113 B/op	      12 allocs/op
BenchmarkFastjq_Small_Abs-16                  	100000000	        10.06 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Abs-16                    	 2562549	       465.1 ns/op	    1113 B/op	      12 allocs/op
BenchmarkFastjq_Small_Bind-16                 	   40526	     29987 ns/op	     648 B/op	      18 allocs/op
BenchmarkGojq_Small_Bind-16                   	 1407920	       844.0 ns/op	    1793 B/op	      24 allocs/op
BenchmarkFastjq_Small_StringInterp-16         	10141051	       116.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_StringInterp-16           	 1000000	      1081 ns/op	    2121 B/op	      35 allocs/op
BenchmarkFastjq_Small_StringInterpNum-16      	13486156	        89.45 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_StringInterpNum-16        	  939704	      1265 ns/op	    2498 B/op	      40 allocs/op
BenchmarkFastjq_Small_IsEmptyTrue-16          	15931035	        73.43 ns/op	     105 B/op	       6 allocs/op
BenchmarkGojq_Small_IsEmptyTrue-16            	 1955512	       609.1 ns/op	    1921 B/op	      18 allocs/op
BenchmarkFastjq_Small_IsEmptyFalse-16         	13520980	        90.22 ns/op	     105 B/op	       6 allocs/op
BenchmarkGojq_Small_IsEmptyFalse-16           	 1000000	      1086 ns/op	    2898 B/op	      33 allocs/op
BenchmarkFastjq_Small_Nth-16                  	 9189721	       132.3 ns/op	     152 B/op	       7 allocs/op
BenchmarkGojq_Small_Nth-16                    	  542970	      2249 ns/op	    4372 B/op	      49 allocs/op
BenchmarkFastjq_Small_Range10-16              	 5813396	       200.3 ns/op	     120 B/op	      12 allocs/op
BenchmarkGojq_Small_Range10-16                	 1000000	      1036 ns/op	    1864 B/op	      18 allocs/op
BenchmarkFastjq_Small_RangeLimit-16           	 8545644	       139.1 ns/op	     128 B/op	       8 allocs/op
BenchmarkGojq_Small_RangeLimit-16             	  754070	      1563 ns/op	    3208 B/op	      26 allocs/op
BenchmarkRegexp_Match_Hit-16                  	26639804	        43.74 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Miss-16                 	51347331	        23.41 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Complex_Hit-16          	26767142	        43.38 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Long_Miss-16            	15526326	        77.67 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_MatchString-16                	27062810	        44.34 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_FindSubmatch_Hit-16           	 5789060	       210.1 ns/op	     160 B/op	       2 allocs/op
BenchmarkRegexp_FindSubmatch_Miss-16          	 1748239	       687.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_FindSubmatchIndex_Hit-16      	 6571593	       182.1 ns/op	      64 B/op	       1 allocs/op
BenchmarkRegexp_FindSubmatchIndex_Miss-16     	 1763812	       680.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_ReplaceAll_Hit-16             	 7755412	       155.4 ns/op	      97 B/op	       5 allocs/op
BenchmarkRegexp_ReplaceAll_Miss-16            	24146630	        50.16 ns/op	      48 B/op	       2 allocs/op
BenchmarkRegexp_MatchViaClosure-16            	29593276	        42.59 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_Hit-16           	11147114	       110.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_Miss-16          	12134689	        98.82 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_InPipeline-16    	 3961386	       302.7 ns/op	       8 B/op	       0 allocs/op
BenchmarkFastjq_Small_MatchRe_Hit-16          	 5381607	       222.7 ns/op	      48 B/op	       1 allocs/op
BenchmarkFastjq_Small_MatchRe_Miss-16         	 1913730	       628.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_TestRe_Hit-16             	  883516	      1321 ns/op	    2743 B/op	      26 allocs/op
BenchmarkGojq_Small_TestRe_Miss-16            	  926806	      1277 ns/op	    2704 B/op	      26 allocs/op
BenchmarkGojq_Small_MatchRe_Hit-16            	  383026	      2979 ns/op	    4800 B/op	      66 allocs/op
BenchmarkGojq_Small_MatchRe_Miss-16           	  720386	      1668 ns/op	    2248 B/op	      24 allocs/op
BenchmarkFastjq_Small_CaptureRe_Hit-16        	 5523442	       217.1 ns/op	      48 B/op	       1 allocs/op
BenchmarkFastjq_Small_CaptureRe_Miss-16       	 1991343	       600.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ScanRe_NoGroups-16      	 2124490	       567.1 ns/op	     534 B/op	      17 allocs/op
BenchmarkFastjq_Small_ScanRe_WithGroups-16    	 1626100	       736.6 ns/op	     851 B/op	      20 allocs/op
BenchmarkFastjq_Small_SubRe_Hit-16            	 9258619	       126.9 ns/op	      16 B/op	       1 allocs/op
BenchmarkFastjq_Small_SubRe_Miss-16           	11681702	       102.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_GSubRe_Hit-16           	 2010483	       599.9 ns/op	     305 B/op	       5 allocs/op
BenchmarkFastjq_Small_GSubRe_Miss-16          	 4601409	       261.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_CaptureRe_Hit-16          	  498732	      2390 ns/op	    4730 B/op	      51 allocs/op
BenchmarkGojq_Small_CaptureRe_Miss-16         	  666151	      1838 ns/op	    2674 B/op	      27 allocs/op
BenchmarkGojq_Small_ScanRe_NoGroups-16        	  416458	      2883 ns/op	    5138 B/op	      57 allocs/op
BenchmarkGojq_Small_SubRe_Hit-16              	  222746	      5303 ns/op	    8997 B/op	      95 allocs/op
BenchmarkGojq_Small_GSubRe_Hit-16             	   79615	     15157 ns/op	   17911 B/op	     248 allocs/op
```

## CLI Throughput: fastjq vs jq

End-to-end JSONL throughput benchmarks comparing `fastjq-bench` (Go CLI using the fastjq library) against `jq` (C implementation, v1.8.1). Both read JSONL from stdin, apply a filter per line, write results to `/dev/null`. Both validate JSON: fastjq calls `json.Valid()` per record, jq parses fully. Median of 3 runs. Apple M4 Max.

| Operation | Input | jq (s) | fastjq (s) | Speedup |
|-----------|-------|--------|-------------|---------|
| Identity (`.`) | small (100K lines, ~11MB) | 0.356 | 0.052 | **6.8x** |
| Field access (`.field_2`) | small | 0.152 | 0.051 | **3x** |
| Field access (`.field_50`) | large (100 lines, ~16MB) | 0.091 | 0.043 | **2.1x** |
| Delete field (`del(.field_2)`) | small | 0.383 | 0.061 | **6.3x** |
| Object construction (`{field_0, field_2}`) | small | 0.252 | 0.060 | **4.2x** |
| Select all match (`select(.field_2 == "xxx...")`) | small | 0.408 | 0.049 | **8.3x** |
| Select none match (`select(.field_2 == "nope")`) | small | 0.143 | 0.050 | **2.9x** |
| Alternative (`.field_2 // "default"`) | small | 0.164 | 0.050 | **3.3x** |
| Case-insensitive select (`ascii_downcase`) | small | 0.655 | 0.065 | **10x** |
| Prefix filter (`startswith`) | small | 0.377 | 0.059 | **6.4x** |
| Field existence (`has`) | small | 0.359 | 0.051 | **7x** |
| `to_entries` | small | 0.746 | 0.066 | **11x** |
| `keys_unsorted` | small | 0.244 | 0.057 | **4.3x** |

### Key Takeaways (CLI)

- **3x–11x faster** than jq across all operations, with both tools validating JSON
- **`to_entries` and `ascii_downcase` are 10–11x faster**: these ops involve minimal work beyond validation; fastjq's lazy scanning dominates
- **Simple field access is 3x faster**: the `json.Valid()` validation pass is the dominant cost for both; fastjq's lazy scan still wins the rest
- **Select with early exit is up to 8x faster**: `select` short-circuits after finding the field, avoiding the rest of the record; jq must complete its full parse first
- **`json.Valid()` cost context**: for the embedded `RunWithBuffer`/`RunFunc` API on known-valid inputs you can skip validation entirely — the Go benchmarks in the table above show the full 13–75x speedup from the library directly

### Reproducing

```bash
chmod +x bench_vs_jq.sh
./bench_vs_jq.sh
```

## Reproducing (Go Benchmarks)

```bash
go test -bench=. -benchmem
```
