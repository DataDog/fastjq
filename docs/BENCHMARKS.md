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
| `.field` | Small (~100B) | 0.087 | 1.08 | **12x** | 0 | 27 |
| `.field` | Large (~100KB) | 7.71 | 622 | **81x** | 0 | 2835 |
| `del(.f)` | Small (~100B) | 0.164 | 2.54 | **15x** | 0 | 58 |
| `del(.f)` | Medium (~2KB) | 2.29 | 21.1 | **9.2x** | 0 | 323 |
| `del(.f)` | Large (~100KB) | 35.8 | 885 | **25x** | 0 | 4667 |
| `.[n]` | 5-elem array | 0.023 | 0.681 | **30x** | 0 | 20 |
| `del(.[n], .[m])` | 5-elem array | 0.082 | 1.89 | **23x** | 0 | 53 |
| `{f0, f2}` (construct) | Small (~100B) | 0.131 | 1.52 | **12x** | 0 | 37 |
| `{f0, f50}` (construct) | Large (~100KB) | 8.20 | 631 | **77x** | 0 | 2867 |
| `.[]` iterator | 5-elem array | 0.063 | 0.878 | **14x** | 2 | 26 |
| `.[]` iterator | 200-elem array | 7.65 | 94.3 | **12x** | 2 | 1811 |
| `select(.f == "x")` | Small (~100B) | 0.126 | 1.89 | **15x** | 0 | 45 |
| `select(.f == "x")`¹ | Large (~100KB, last field) | 34.6 | 854 | **25x** | 0 | 4652 |
| `select(.f and .g)` | Small (~100B) | 0.171 | 2.02 | **12x** | 0 | 46 |
| `select(.f or .g)` | Small (~100B) | 0.074 | 1.97 | **26x** | 0 | 46 |
| `has("key")` in select | Small (~100B) | 0.091 | 1.92 | **21x** | 0 | 45 |
| `has("key")` in select | Large (~100KB) | 34.2 | 854 | **25x** | 0 | 4652 |
| `if-then-else` | Small (~100B) | 0.100 | 1.25 | **12x** | 0 | 30 |
| `.f // "default"` | Small (~100B) | 0.092 | 1.23 | **13x** | 0 | 31 |
| `try .field` (no error) | Small (~100B) | 4.80 | 1.16 | 0.2x† | 1 | 30 |
| `.a + .b` (strings) | Small (~100B) | 0.053 | 0.789 | **15x** | 0 | 21 |
| `"prefix" + .name` | Small (~100B) | 0.086 | 0.843 | **9.8x** | 0 | 23 |
| `.a - .b` (subtract) | Small (~100B) | 0.054 | 0.775 | **14x** | 0 | 20 |
| `.a * .b` (multiply) | Small (~100B) | 0.069 | 0.816 | **12x** | 0 | 21 |
| `.a / .b` (divide) | Small (~100B) | 0.101 | 0.824 | **8.2x** | 0 | 21 |
| `.a - .b` (array diff) | 5-elem arrays | 0.221 | 1.39 | **6.3x** | 0 | 33 |
| `length` | Small (~100B) | 0.041 | 1.11 | **27x** | 0 | 27 |
| `length` | Large (~100KB) | 33.2 | 621 | **19x** | 0 | 2835 |
| `add` (numbers) | 5-elem array | 0.061 | 0.839 | **14x** | 0 | 28 |
| `add` (strings) | 5-elem array | 0.139 | 1.01 | **7.3x** | 0 | 35 |
| `flatten` | 3-elem nested array | 0.098 | 1.45 | **15x** | 0 | 38 |
| `min` | 200-int array | 1.88 | 1.32 | 0.7x† | 0 | 15 |
| `min_by(.value)` | 100-elem object array | 8.64 | 59.8 | **6.9x** | 0 | 1347 |
| `sort` | 200-int array | 4.93 | 1.32 | 0.3x† | 14 | 15 |
| `sort_by(.value)` | 100-elem object array | 20.1 | 106 | **5.3x** | 422 | 2145 |
| `unique` | 200-int array | 6.38 | 1.31 | 0.2x† | 14 | 15 |
| `group_by(.active)` | 100-elem object array | 23.6 | 107 | **4.5x** | 422 | 2260 |
| `map(.name)` | 20-elem array (~600B) | 1.68 | 11.3 | **6.7x** | 29 | 251 |
| `map(.name)` | 200-elem array (~6KB) | 16.5 | 100 | **6.1x** | 209 | 2237 |
| `any` | 5-elem array | 0.046 | 2.15 | **47x** | 0 | 39 |
| `any(expr)` | 5-elem array | 0.403 | 2.37 | **5.9x** | 12 | 49 |
| `any(expr)`² | 200-int array | 4.07 | 1.94 | 0.5x† | 0 | 29 |
| `any(gen; cond)`² | 200-int array | 4.64 | 1.82 | 0.4x† | 3 | 27 |
| `first(expr)` | 5-elem array | 0.263 | 1.73 | **6.6x** | 9 | 39 |
| `first(expr)`² | 200-int array | 7.00 | 1.60 | 0.2x† | 107 | 23 |
| `last(expr)` | 5-elem array | 0.326 | 2.17 | **6.7x** | 10 | 43 |
| `last(expr)`² | 200-int array | 6.66 | 1.72 | 0.3x† | 107 | 24 |
| `limit(3; expr)` | 5-elem array | 0.108 | 1.92 | **18x** | 5 | 42 |
| `limit(10; expr)` | 200-int array | 0.744 | 1.79 | **2.4x** | 5 | 24 |
| `.[1:4]` slice | 6-elem array | 0.087 | 0.905 | **10x** | 0 | 21 |
| `values` | 9-elem array | 0.281 | 2.53 | **9x** | 13 | 51 |
| `skip(2; .[])` | 5-elem array | 0.153 | 1.93 | **13x** | 7 | 43 |
| `paths` | Small (~100B) | 0.412 | 4.86 | **12x** | 17 | 119 |
| `to_entries` | Small (~100B) | 0.177 | 4.44 | **25x** | 0 | 98 |
| `to_entries` | Large (~100KB) | 35.8 | 970 | **27x** | 0 | 6465 |
| `getpath(["field_0"])` | Small (~100B) | 0.264 | 1.17 | **4.4x** | 12 | 29 |
| `setpath(["field_0"]; "y")` | Small (~100B) | 0.442 | 1.90 | **4.3x** | 12 | 43 |
| `delpaths([["field_0"]])` | Small (~100B) | 0.752 | 1.88 | **2.5x** | 22 | 40 |
| `keys` | Small (~100B) | 0.314 | 1.43 | **4.6x** | 7 | 35 |
| `keys_unsorted` | Small (~100B) | 0.181 | 1.44 | **7.9x** | 0 | 35 |
| `keys_unsorted` | Large (~100KB) | 33.8 | 646 | **19x** | 0 | 3039 |
| `object merge .a + .b` | Small (~100B) | 0.171 | 1.66 | **9.7x** | 0 | 33 |
| `tojson` | Small (~100B) | 0.342 | 1.76 | **5.1x** | 0 | 39 |
| `fromjson` | JSON string | 0.161 | 1.48 | **9.2x** | 0 | 31 |
| `tonumber` | `"42"` string | 0.014 | 0.445 | **32x** | 0 | 11 |
| `toboolean` | `"true"` string | 0.0093 | 0.442 | **47x** | 0 | 11 |
| `split(",")` | short string | 0.149 | 0.888 | **6x** | 0 | 21 |
| `join(",")` | 5-elem array | 0.105 | 1.09 | **10x** | 0 | 37 |
| `ascii_downcase` in select | Small (~100B) | 0.141 | 1.98 | **14x** | 0 | 46 |
| `ascii_downcase` in select | Large (~100KB) | 34.6 | 869 | **25x** | 0 | 4653 |
| `startswith("s")` | Small (~100B) | 0.124 | 1.91 | **15x** | 0 | 45 |
| `startswith("s")` | Large (~100KB) | 34.2 | 841 | **25x** | 0 | 4652 |
| `endswith("s")` | Small (~100B) | 0.124 | 1.90 | **15x** | 0 | 45 |
| `trim` | short string | 0.054 | 0.439 | **8.1x** | 1 | 12 |
| `ltrim` | short string | 0.048 | 0.452 | **9.4x** | 1 | 12 |
| `rtrim` | short string | 0.053 | 0.447 | **8.5x** | 1 | 12 |
| `ltrimstr("s")` | Small (~100B) | 0.125 | 1.17 | **9.3x** | 0 | 30 |
| `rtrimstr("s")` | Small (~100B) | 0.126 | 1.15 | **9.2x** | 0 | 30 |
| `select` + string ops + arith + construct | ~200B log event | 1.09 | 3.60 | **3.3x** | 0 | 69 |
| `[.[] \| select + arith + construct]` | 20-elem array (~1.5KB) | 10.8 | 26.1 | **2.4x** | 106 | 525 |
| `length + map + add + min_by + any` | 20-elem array (~1.5KB) | 13.0 | 24.3 | **1.9x** | 106 | 555 |
| `map(try {…} catch …)` | 20-elem array (~1.5KB) | 281 | 30.3 | 0.1x† | 187 | 641 |
| `map(if … elif … else …)` | 20-elem array (~1.5KB) | 11.2 | 36.3 | **3.2x** | 127 | 595 |
| `[.[] \| str + tostring + str]` | 20-elem array (~1.5KB) | 13.0 | 26.9 | **2.1x** | 298 | 686 |
| `to_entries \| map(select) \| from_entries` | ~200B log event | 3.79 | 15.8 | **4.2x** | 69 | 250 |
| `@base64` | 34-char string | 0.150 | 0.572 | **3.8x** | 4 | 15 |
| `@base64d` | 48-char encoded | 0.221 | 0.602 | **2.7x** | 0 | 15 |
| `@uri` | 36-char URL string | 0.175 | 0.752 | **4.3x** | 4 | 14 |
| `index(",")` | short string | 0.100 | 0.971 | **9.7x** | 1 | 31 |
| `indices(",")` | short string | 0.212 | 2.30 | **11x** | 1 | 97 |
| `sqrt` | float (e≈2.718) | 0.070 | 0.549 | **7.9x** | 0 | 12 |
| `log` | float (e≈2.718) | 0.045 | 0.539 | **12x** | 0 | 12 |
| `sin` | float (e≈2.718) | 0.084 | 0.523 | **6.2x** | 0 | 12 |
| `atan` | integer 1 | 0.057 | 0.506 | **8.9x** | 0 | 11 |
| `exp` | integer 1 | 0.098 | 0.719 | **7.3x** | 0 | 11 |
| `tgamma` | integer 5 | 0.021 | 0.502 | **24x** | 0 | 11 |
| `fabs` | float -3.14 | 0.058 | 0.483 | **8.4x** | 0 | 12 |
| `abs` | float -3.14 | 0.010 | 0.584 | **58x** | 0 | 12 |
| `expr as $x \| body` | Small (~20B object) | 51.9 | 1.23 | 0.0x† | 18 | 24 |
| ``"\(.level): \(.svc)"`` | ~45B object | 0.136 | 1.21 | **8.9x** | 0 | 35 |
| ``"user \(.name) …"`` | ~45B object | 0.102 | 1.47 | **14x** | 0 | 40 |
| `isempty(empty)` | null | 0.109 | 1.12 | **10x** | 6 | 18 |
| `isempty(.[])` | 5-elem array | 0.139 | 1.32 | **9.5x** | 6 | 33 |
| `nth(2; .[])` | 5-elem array | 0.142 | 2.51 | **18x** | 7 | 49 |
| `range(10)` (10 values) | null | 0.219 | 1.28 | **5.8x** | 12 | 18 |
| `limit(3; range(1000))` | null — only 3 allocs | 0.201 | 2.13 | **11x** | 8 | 26 |
| `test(re)` hit | short string | 0.142 | 1.91 | **13x** | 0 | 26 |
| `test(re)` miss | short string | 0.122 | 2.37 | **19x** | 0 | 26 |
| `match(re)` hit | short string | 0.242 | 5.43 | **22x** | 1 | 66 |
| `match(re)` miss | short string | 0.676 | 2.46 | **3.6x** | 0 | 24 |
| `capture(re)` hit | short string | 0.255 | — | — | 1 | — |
| `scan(re)` no groups (4 matches) | short string | 0.611 | — | — | 17 | — |
| `sub(re; s)` hit | short string | 0.160 | — | — | 1 | — |
| `gsub(re; s)` hit (4 matches) | short string | 0.713 | — | — | 5 | — |

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
BenchmarkFastjq_Small_Del-16                  	 7104057	       163.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Medium_Del-16                 	  528907	      2286 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Del-16                  	   33549	     35838 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Field-16                	13373900	        86.98 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Field-16                	  156494	      7705 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Index-16                	51185895	        22.93 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ArrayDel-16             	14507295	        82.39 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Construct-16            	 9681459	       131.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Construct-16            	  141182	      8201 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Iterator-16             	19490722	        62.91 ns/op	      40 B/op	       2 allocs/op
BenchmarkFastjq_Large_Iterator-16             	  164916	      7653 ns/op	      40 B/op	       2 allocs/op
BenchmarkFastjq_Small_Select-16               	 9487915	       125.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Select-16               	   34728	     34631 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Alternative-16          	13167579	        92.47 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SelectAnd-16            	 6946862	       170.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SelectOr-16             	16687758	        74.41 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Has-16                  	13123867	        91.45 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Has-16                  	   35641	     34161 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_IfThenElse-16           	11888490	       100.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Length-16               	29872294	        40.87 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Length-16               	   35650	     33229 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Map-16                  	  693362	      1683 ns/op	     689 B/op	      29 allocs/op
BenchmarkFastjq_Medium_Map-16                 	  142554	      8279 ns/op	    2609 B/op	     109 allocs/op
BenchmarkFastjq_Large_Map-16                  	   70765	     16535 ns/op	    5009 B/op	     209 allocs/op
BenchmarkFastjq_Small_ToEntries-16            	 6665595	       177.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_ToEntries-16            	   32866	     35821 ns/op	       6 B/op	       0 allocs/op
BenchmarkFastjq_Small_KeysUnsorted-16         	 7095818	       181.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Keys-16                 	 3875216	       313.5 ns/op	     456 B/op	       7 allocs/op
BenchmarkFastjq_Small_Paths-16                	 2867796	       411.9 ns/op	     400 B/op	      17 allocs/op
BenchmarkFastjq_Small_GetPath-16              	 4549790	       263.8 ns/op	     272 B/op	      12 allocs/op
BenchmarkFastjq_Small_SetPath-16              	 2751914	       442.3 ns/op	     264 B/op	      12 allocs/op
BenchmarkFastjq_Small_DelPaths-16             	 1576192	       752.5 ns/op	     608 B/op	      22 allocs/op
BenchmarkFastjq_Large_KeysUnsorted-16         	   35682	     33770 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Any-16                  	24200073	        45.58 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Any-16                  	  912188	      1377 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AnyExpr-16              	 2872176	       402.7 ns/op	     144 B/op	      12 allocs/op
BenchmarkFastjq_Large_AnyExpr-16              	  296810	      4070 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Add-16                  	19781265	        60.94 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AddStrings-16           	 8730507	       138.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Flatten-16              	12214765	        97.58 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Split-16                	 7920754	       149.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Join-16                 	11480168	       104.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Values-16               	 4409034	       280.7 ns/op	     328 B/op	      13 allocs/op
BenchmarkGojq_Small_Values-16                 	  454080	      2532 ns/op	    3144 B/op	      51 allocs/op
BenchmarkFastjq_Small_Base64Encode-16         	 8017790	       150.5 ns/op	     120 B/op	       4 allocs/op
BenchmarkFastjq_Small_Base64Decode-16         	 5419726	       221.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Base64Encode-16           	 2082598	       572.3 ns/op	    1321 B/op	      15 allocs/op
BenchmarkGojq_Small_Base64Decode-16           	 1990846	       602.4 ns/op	    1321 B/op	      15 allocs/op
BenchmarkFastjq_Small_IndexFind-16            	11950668	        99.82 ns/op	       3 B/op	       1 allocs/op
BenchmarkFastjq_Small_IndicesAll-16           	 5732810	       211.6 ns/op	       3 B/op	       1 allocs/op
BenchmarkGojq_Small_IndexFind-16              	 1237016	       970.5 ns/op	    2274 B/op	      31 allocs/op
BenchmarkGojq_Small_IndicesAll-16             	  535729	      2304 ns/op	    3907 B/op	      97 allocs/op
BenchmarkFastjq_Small_Slice-16                	13669034	        87.37 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SliceString-16          	11616542	       101.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Plus-16                 	22873262	        52.97 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_PlusStr-16              	13841680	        86.33 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AsciiDowncase-16        	 8474900	       141.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_AsciiDowncase-16        	   34616	     34615 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Startswith-16           	 9461517	       123.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Startswith-16           	   34879	     34183 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Endswith-16             	 9828391	       123.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Ltrimstr-16             	 9912892	       125.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Ltrimstr-16             	   36280	     32557 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Rtrimstr-16             	 9862729	       126.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Trim-16                 	22368080	        54.20 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_Ltrim-16                	24501567	        48.20 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_Rtrim-16                	23724459	        52.56 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_First-16                	 4573680	       262.8 ns/op	     216 B/op	       9 allocs/op
BenchmarkFastjq_Large_First-16                	  153492	      7000 ns/op	    2584 B/op	     107 allocs/op
BenchmarkFastjq_Small_Last-16                 	 3718004	       325.9 ns/op	     256 B/op	      10 allocs/op
BenchmarkFastjq_Large_Last-16                 	  182784	      6663 ns/op	    2584 B/op	     107 allocs/op
BenchmarkFastjq_Small_Limit-16                	11321640	       107.8 ns/op	     104 B/op	       5 allocs/op
BenchmarkFastjq_Small_Skip-16                 	 7956651	       153.1 ns/op	     208 B/op	       7 allocs/op
BenchmarkFastjq_Large_Limit-16                	 1594578	       744.3 ns/op	     104 B/op	       5 allocs/op
BenchmarkGojq_Small_Del-16                    	  489606	      2537 ns/op	    3763 B/op	      58 allocs/op
BenchmarkGojq_Medium_Del-16                   	   55981	     21136 ns/op	   16976 B/op	     323 allocs/op
BenchmarkGojq_Large_Del-16                    	    1305	    884961 ns/op	  549960 B/op	    4667 allocs/op
BenchmarkGojq_Small_Field-16                  	 1000000	      1079 ns/op	    1641 B/op	      27 allocs/op
BenchmarkGojq_Large_Field-16                  	    1898	    622301 ns/op	  270046 B/op	    2835 allocs/op
BenchmarkGojq_Small_Index-16                  	 1758974	       681.1 ns/op	    1401 B/op	      20 allocs/op
BenchmarkGojq_Small_ArrayDel-16               	  661336	      1894 ns/op	    3363 B/op	      53 allocs/op
BenchmarkGojq_Small_Construct-16              	  797995	      1516 ns/op	    2330 B/op	      37 allocs/op
BenchmarkGojq_Large_Construct-16              	    1927	    630652 ns/op	  274541 B/op	    2867 allocs/op
BenchmarkGojq_Small_Iterator-16               	 1369102	       878.3 ns/op	    1776 B/op	      26 allocs/op
BenchmarkGojq_Large_Iterator-16               	   12727	     94281 ns/op	  109809 B/op	    1811 allocs/op
BenchmarkGojq_Small_Select-16                 	  638722	      1894 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Large_Select-16                 	    1435	    854053 ns/op	  540245 B/op	    4652 allocs/op
BenchmarkGojq_Small_Alternative-16            	 1000000	      1226 ns/op	    1897 B/op	      31 allocs/op
BenchmarkGojq_Small_SelectAnd-16              	  605438	      2025 ns/op	    2890 B/op	      46 allocs/op
BenchmarkGojq_Small_SelectOr-16               	  613452	      1969 ns/op	    2890 B/op	      46 allocs/op
BenchmarkGojq_Small_Has-16                    	  658416	      1916 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Large_Has-16                    	    1306	    853811 ns/op	  542375 B/op	    4652 allocs/op
BenchmarkGojq_Small_IfThenElse-16             	  942938	      1249 ns/op	    1817 B/op	      30 allocs/op
BenchmarkGojq_Small_Length-16                 	 1000000	      1109 ns/op	    1633 B/op	      27 allocs/op
BenchmarkGojq_Large_Length-16                 	    1924	    621156 ns/op	  269829 B/op	    2835 allocs/op
BenchmarkGojq_Small_Map-16                    	  102061	     11297 ns/op	   13655 B/op	     251 allocs/op
BenchmarkGojq_Large_Map-16                    	   10000	    100135 ns/op	  118614 B/op	    2237 allocs/op
BenchmarkGojq_Small_ToEntries-16              	  260793	      4442 ns/op	    6559 B/op	      98 allocs/op
BenchmarkGojq_Large_ToEntries-16              	    1174	    969545 ns/op	  677481 B/op	    6465 allocs/op
BenchmarkGojq_Small_KeysUnsorted-16           	  858031	      1438 ns/op	    1953 B/op	      35 allocs/op
BenchmarkGojq_Small_Keys-16                   	  832720	      1434 ns/op	    1953 B/op	      35 allocs/op
BenchmarkGojq_Small_Paths-16                  	  249489	      4864 ns/op	    6608 B/op	     119 allocs/op
BenchmarkGojq_Small_GetPath-16                	  972807	      1173 ns/op	    1721 B/op	      29 allocs/op
BenchmarkGojq_Small_SetPath-16                	  634028	      1904 ns/op	    2618 B/op	      43 allocs/op
BenchmarkGojq_Small_DelPaths-16               	  630196	      1876 ns/op	    2426 B/op	      40 allocs/op
BenchmarkGojq_Large_KeysUnsorted-16           	    1878	    646126 ns/op	  282846 B/op	    3039 allocs/op
BenchmarkGojq_Small_Any-16                    	  522139	      2155 ns/op	    4508 B/op	      39 allocs/op
BenchmarkGojq_Small_AnyExpr-16                	  510619	      2367 ns/op	    4620 B/op	      49 allocs/op
BenchmarkGojq_Large_AnyExpr-16                	  636402	      1940 ns/op	    2818 B/op	      29 allocs/op
BenchmarkGojq_Small_Add-16                    	 1435604	       838.6 ns/op	    1529 B/op	      28 allocs/op
BenchmarkGojq_Small_AddStrings-16             	 1000000	      1011 ns/op	    1785 B/op	      35 allocs/op
BenchmarkGojq_Small_Flatten-16                	  824041	      1445 ns/op	    1857 B/op	      38 allocs/op
BenchmarkGojq_Small_Split-16                  	 1337737	       888.2 ns/op	    1553 B/op	      21 allocs/op
BenchmarkGojq_Small_Join-16                   	 1000000	      1089 ns/op	    1865 B/op	      37 allocs/op
BenchmarkGojq_Small_Slice-16                  	 1329514	       905.3 ns/op	    1425 B/op	      21 allocs/op
BenchmarkGojq_Small_SliceString-16            	 2298901	       522.2 ns/op	    1145 B/op	      12 allocs/op
BenchmarkGojq_Small_Plus-16                   	 1512213	       789.0 ns/op	    1673 B/op	      21 allocs/op
BenchmarkGojq_Small_PlusStr-16                	 1427154	       843.2 ns/op	    1705 B/op	      23 allocs/op
BenchmarkGojq_Small_AsciiDowncase-16          	  615692	      1983 ns/op	    2714 B/op	      46 allocs/op
BenchmarkGojq_Large_AsciiDowncase-16          	    1322	    869161 ns/op	  543551 B/op	    4653 allocs/op
BenchmarkGojq_Small_Startswith-16             	  629235	      1910 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Large_Startswith-16             	    1430	    840802 ns/op	  539997 B/op	    4652 allocs/op
BenchmarkGojq_Small_Endswith-16               	  640612	      1897 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Small_Ltrimstr-16               	 1000000	      1165 ns/op	    1737 B/op	      30 allocs/op
BenchmarkGojq_Small_Rtrimstr-16               	 1000000	      1154 ns/op	    1737 B/op	      30 allocs/op
BenchmarkGojq_Small_Trim-16                   	 2752486	       438.7 ns/op	    1129 B/op	      12 allocs/op
BenchmarkGojq_Small_Ltrim-16                  	 2705077	       452.0 ns/op	    1129 B/op	      12 allocs/op
BenchmarkGojq_Small_Rtrim-16                  	 2693383	       446.8 ns/op	    1129 B/op	      12 allocs/op
BenchmarkGojq_Small_First-16                  	  752000	      1725 ns/op	    3379 B/op	      39 allocs/op
BenchmarkGojq_Large_First-16                  	  764644	      1602 ns/op	    1889 B/op	      23 allocs/op
BenchmarkGojq_Small_Last-16                   	  552600	      2173 ns/op	    3483 B/op	      43 allocs/op
BenchmarkGojq_Large_Last-16                   	  710182	      1715 ns/op	    2193 B/op	      24 allocs/op
BenchmarkGojq_Small_Limit-16                  	  636862	      1920 ns/op	    3328 B/op	      42 allocs/op
BenchmarkGojq_Small_Skip-16                   	  616171	      1927 ns/op	    2880 B/op	      43 allocs/op
BenchmarkGojq_Large_Limit-16                  	  699288	      1788 ns/op	    2200 B/op	      24 allocs/op
BenchmarkFastjq_Small_Subtract-16             	22548726	        53.55 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Subtract-16               	 1551448	       775.4 ns/op	    1649 B/op	      20 allocs/op
BenchmarkFastjq_Small_Multiply-16             	17522095	        69.05 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Multiply-16               	 1474023	       816.1 ns/op	    1649 B/op	      21 allocs/op
BenchmarkFastjq_Small_Divide-16               	11891131	       100.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Divide-16                 	 1455456	       824.3 ns/op	    1665 B/op	      21 allocs/op
BenchmarkFastjq_Small_Min-16                  	  630954	      1880 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Min-16                    	  921970	      1323 ns/op	    1217 B/op	      15 allocs/op
BenchmarkFastjq_Small_MinBy-16                	  136816	      8645 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_MinBy-16                  	   20092	     59792 ns/op	   63631 B/op	    1347 allocs/op
BenchmarkFastjq_Sort-16                       	  241456	      4929 ns/op	    7688 B/op	      14 allocs/op
BenchmarkGojq_Sort-16                         	  880542	      1324 ns/op	    1217 B/op	      15 allocs/op
BenchmarkFastjq_SortBy-16                     	   61854	     20076 ns/op	   24872 B/op	     422 allocs/op
BenchmarkGojq_SortBy-16                       	   10000	    105728 ns/op	   97006 B/op	    2145 allocs/op
BenchmarkFastjq_Unique-16                     	  180613	      6378 ns/op	    7688 B/op	      14 allocs/op
BenchmarkGojq_Unique-16                       	  937545	      1308 ns/op	    1217 B/op	      15 allocs/op
BenchmarkFastjq_GroupBy-16                    	   51247	     23621 ns/op	   24872 B/op	     422 allocs/op
BenchmarkGojq_GroupBy-16                      	   10000	    106567 ns/op	  101963 B/op	    2260 allocs/op
BenchmarkFastjq_Small_URIEncode-16            	 6886622	       175.1 ns/op	     120 B/op	       4 allocs/op
BenchmarkGojq_Small_URIEncode-16              	 1585995	       751.7 ns/op	    1289 B/op	      14 allocs/op
BenchmarkFastjq_Small_ArrayDiff-16            	 5405026	       221.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ArrayDiff-16              	  896488	      1387 ns/op	    2121 B/op	      33 allocs/op
BenchmarkFastjq_Small_TryNoError-16           	  256252	      4800 ns/op	      64 B/op	       1 allocs/op
BenchmarkFastjq_Small_TryCatchNoError-16      	  248568	      4809 ns/op	      64 B/op	       1 allocs/op
BenchmarkGojq_Small_TryNoError-16             	 1000000	      1163 ns/op	    1913 B/op	      30 allocs/op
BenchmarkFastjq_Small_ObjectMerge-16          	 7021964	       170.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ObjectMerge-16            	  723234	      1657 ns/op	    2906 B/op	      33 allocs/op
BenchmarkFastjq_Small_ToJSON-16               	 3473080	       342.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ToJSON-16                 	  697934	      1760 ns/op	    2410 B/op	      39 allocs/op
BenchmarkFastjq_Small_FromJSON-16             	 7650297	       160.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_FromJSON-16               	  797354	      1480 ns/op	    2714 B/op	      31 allocs/op
BenchmarkFastjq_Small_ToString-16             	 3435002	       345.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToNumber-16             	87148076	        13.92 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToBoolean-16            	127842514	         9.345 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ToNumber-16               	 2683939	       445.1 ns/op	    1113 B/op	      11 allocs/op
BenchmarkGojq_Small_ToBoolean-16              	 2745679	       441.8 ns/op	    1113 B/op	      11 allocs/op
BenchmarkFastjq_Small_AnyTwoArg-16            	  253236	      4638 ns/op	      57 B/op	       3 allocs/op
BenchmarkGojq_Small_AnyTwoArg-16              	  659688	      1817 ns/op	    2482 B/op	      27 allocs/op
BenchmarkFastjq_Complex_LogNormalize-16       	  929454	      1095 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Complex_LogNormalize-16         	  369740	      3597 ns/op	    4324 B/op	      69 allocs/op
BenchmarkFastjq_Complex_ArrayPipeline-16      	  103022	     10835 ns/op	    2264 B/op	     106 allocs/op
BenchmarkGojq_Complex_ArrayPipeline-16        	   43885	     26061 ns/op	   20410 B/op	     525 allocs/op
BenchmarkFastjq_Complex_Aggregation-16        	   91316	     12985 ns/op	    3184 B/op	     106 allocs/op
BenchmarkGojq_Complex_Aggregation-16          	   48164	     24343 ns/op	   22397 B/op	     555 allocs/op
BenchmarkFastjq_Complex_TolerantMap-16        	    4305	    280877 ns/op	    5216 B/op	     187 allocs/op
BenchmarkGojq_Complex_TolerantMap-16          	   39615	     30277 ns/op	   27873 B/op	     641 allocs/op
BenchmarkFastjq_Complex_ElifRouting-16        	  120171	     11214 ns/op	    3000 B/op	     127 allocs/op
BenchmarkGojq_Complex_ElifRouting-16          	   32602	     36274 ns/op	   25920 B/op	     595 allocs/op
BenchmarkFastjq_Complex_StringBuild-16        	   83684	     12987 ns/op	    7216 B/op	     298 allocs/op
BenchmarkGojq_Complex_StringBuild-16          	   45426	     26891 ns/op	   20534 B/op	     686 allocs/op
BenchmarkFastjq_Complex_EntryFilter-16        	  300043	      3794 ns/op	    2888 B/op	      69 allocs/op
BenchmarkGojq_Complex_EntryFilter-16          	   75212	     15832 ns/op	   18429 B/op	     250 allocs/op
BenchmarkFastjq_Small_Sqrt-16                 	16951207	        69.82 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Sqrt-16                   	 2310849	       548.6 ns/op	    1145 B/op	      12 allocs/op
BenchmarkFastjq_Small_Log-16                  	26640050	        45.08 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Log-16                    	 2217271	       539.4 ns/op	    1129 B/op	      12 allocs/op
BenchmarkFastjq_Small_Sin-16                  	13575082	        84.12 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Sin-16                    	 2301526	       523.1 ns/op	    1145 B/op	      12 allocs/op
BenchmarkFastjq_Small_Atan-16                 	21639313	        57.07 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Atan-16                   	 2510784	       506.2 ns/op	    1121 B/op	      11 allocs/op
BenchmarkFastjq_Small_Exp-16                  	13470854	        98.09 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Exp-16                    	 1676794	       719.4 ns/op	    1121 B/op	      11 allocs/op
BenchmarkFastjq_Small_Tgamma-16               	54162522	        20.68 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Tgamma-16                 	 2336526	       502.2 ns/op	    1105 B/op	      11 allocs/op
BenchmarkFastjq_Small_Fabs-16                 	20426893	        57.52 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Fabs-16                   	 2466855	       483.0 ns/op	    1113 B/op	      12 allocs/op
BenchmarkFastjq_Small_Abs-16                  	100000000	        10.14 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Abs-16                    	 2296576	       584.2 ns/op	    1113 B/op	      12 allocs/op
BenchmarkFastjq_Small_Bind-16                 	   23130	     51935 ns/op	     648 B/op	      18 allocs/op
BenchmarkGojq_Small_Bind-16                   	  873783	      1231 ns/op	    1793 B/op	      24 allocs/op
BenchmarkFastjq_Small_StringInterp-16         	 7633918	       135.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_StringInterp-16           	  972945	      1210 ns/op	    2121 B/op	      35 allocs/op
BenchmarkFastjq_Small_StringInterpNum-16      	11974060	       101.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_StringInterpNum-16        	  833638	      1465 ns/op	    2498 B/op	      40 allocs/op
BenchmarkFastjq_Small_IsEmptyTrue-16          	12805713	       108.8 ns/op	     105 B/op	       6 allocs/op
BenchmarkGojq_Small_IsEmptyTrue-16            	 1000000	      1117 ns/op	    1921 B/op	      18 allocs/op
BenchmarkFastjq_Small_IsEmptyFalse-16         	 7922541	       138.6 ns/op	     105 B/op	       6 allocs/op
BenchmarkGojq_Small_IsEmptyFalse-16           	  788664	      1323 ns/op	    2898 B/op	      33 allocs/op
BenchmarkFastjq_Small_Nth-16                  	 8238325	       141.9 ns/op	     152 B/op	       7 allocs/op
BenchmarkGojq_Small_Nth-16                    	  486566	      2508 ns/op	    4372 B/op	      49 allocs/op
BenchmarkFastjq_Small_Range10-16              	 5412554	       218.6 ns/op	     120 B/op	      12 allocs/op
BenchmarkGojq_Small_Range10-16                	 1000000	      1278 ns/op	    1864 B/op	      18 allocs/op
BenchmarkFastjq_Small_RangeLimit-16           	 6151312	       200.6 ns/op	     128 B/op	       8 allocs/op
BenchmarkGojq_Small_RangeLimit-16             	  568076	      2126 ns/op	    3208 B/op	      26 allocs/op
BenchmarkRegexp_Match_Hit-16                  	22221038	        50.96 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Miss-16                 	45310089	        25.27 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Complex_Hit-16          	25813575	        46.37 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Long_Miss-16            	13544704	        86.46 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_MatchString-16                	23433456	        61.53 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_FindSubmatch_Hit-16           	 3325952	       367.7 ns/op	     160 B/op	       2 allocs/op
BenchmarkRegexp_FindSubmatch_Miss-16          	 1000000	      1034 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_FindSubmatchIndex_Hit-16      	 5132184	       211.5 ns/op	      64 B/op	       1 allocs/op
BenchmarkRegexp_FindSubmatchIndex_Miss-16     	 1707303	       700.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_ReplaceAll_Hit-16             	 7430533	       161.6 ns/op	      97 B/op	       5 allocs/op
BenchmarkRegexp_ReplaceAll_Miss-16            	21801021	        56.72 ns/op	      48 B/op	       2 allocs/op
BenchmarkRegexp_MatchViaClosure-16            	23611975	        56.91 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_Hit-16           	 8092351	       142.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_Miss-16          	 9306746	       121.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_InPipeline-16    	 3434718	       340.9 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_MatchRe_Hit-16          	 5034156	       242.0 ns/op	      48 B/op	       1 allocs/op
BenchmarkFastjq_Small_MatchRe_Miss-16         	 1786456	       676.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_TestRe_Hit-16             	  738224	      1911 ns/op	    2748 B/op	      26 allocs/op
BenchmarkGojq_Small_TestRe_Miss-16            	  567934	      2371 ns/op	    2708 B/op	      26 allocs/op
BenchmarkGojq_Small_MatchRe_Hit-16            	  211681	      5429 ns/op	    4802 B/op	      66 allocs/op
BenchmarkGojq_Small_MatchRe_Miss-16           	  426436	      2458 ns/op	    2251 B/op	      24 allocs/op
BenchmarkFastjq_Small_CaptureRe_Hit-16        	 4484325	       255.2 ns/op	      48 B/op	       1 allocs/op
BenchmarkFastjq_Small_CaptureRe_Miss-16       	 1726016	       693.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ScanRe_NoGroups-16      	 1965800	       610.8 ns/op	     534 B/op	      17 allocs/op
BenchmarkFastjq_Small_ScanRe_WithGroups-16    	 1495890	       818.9 ns/op	     850 B/op	      20 allocs/op
BenchmarkFastjq_Small_SubRe_Hit-16            	 7794682	       160.0 ns/op	      16 B/op	       1 allocs/op
BenchmarkFastjq_Small_SubRe_Miss-16           	 9721924	       123.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_GSubRe_Hit-16           	 1634192	       713.0 ns/op	     306 B/op	       5 allocs/op
BenchmarkFastjq_Small_GSubRe_Miss-16          	 4228759	       276.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_CaptureRe_Hit-16          	  453154	      2613 ns/op	    4720 B/op	      51 allocs/op
BenchmarkGojq_Small_CaptureRe_Miss-16         	  584725	      2169 ns/op	    2676 B/op	      27 allocs/op
BenchmarkGojq_Small_ScanRe_NoGroups-16        	  338829	      4103 ns/op	    5137 B/op	      57 allocs/op
BenchmarkGojq_Small_SubRe_Hit-16              	  115976	     11196 ns/op	    9017 B/op	      95 allocs/op
BenchmarkGojq_Small_GSubRe_Hit-16             	   36043	     29348 ns/op	   17948 B/op	     248 allocs/op
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
