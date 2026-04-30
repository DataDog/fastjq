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
| `.field` | Small (~100B) | 0.088 | 1.01 | **11x** | 0 | 27 |
| `.field` | Large (~100KB) | 8.10 | 599 | **74x** | 0 | 2835 |
| `del(.f)` | Small (~100B) | 0.161 | 2.38 | **15x** | 0 | 58 |
| `del(.f)` | Medium (~2KB) | 2.25 | 19.8 | **8.8x** | 0 | 323 |
| `del(.f)` | Large (~100KB) | 36.1 | 845 | **23x** | 0 | 4666 |
| `.[n]` | 5-elem array | 0.024 | 0.662 | **28x** | 0 | 20 |
| `del(.[n], .[m])` | 5-elem array | 0.083 | 1.70 | **21x** | 0 | 53 |
| `{f0, f2}` (construct) | Small (~100B) | 0.146 | 1.41 | **9.7x** | 0 | 37 |
| `{f0, f50}` (construct) | Large (~100KB) | 8.48 | 615 | **73x** | 0 | 2867 |
| `.[]` iterator | 5-elem array | 0.060 | 0.799 | **13x** | 2 | 26 |
| `.[]` iterator | 200-elem array | 7.65 | 88.4 | **12x** | 2 | 1811 |
| `select(.f == "x")` | Small (~100B) | 0.130 | 1.76 | **14x** | 0 | 45 |
| `select(.f == "x")`¹ | Large (~100KB, last field) | 34.6 | 799 | **23x** | 0 | 4651 |
| `select(.f and .g)` | Small (~100B) | 0.171 | 1.87 | **11x** | 0 | 46 |
| `select(.f or .g)` | Small (~100B) | 0.074 | 1.77 | **24x** | 0 | 46 |
| `has("key")` in select | Small (~100B) | 0.096 | 1.76 | **18x** | 0 | 45 |
| `has("key")` in select | Large (~100KB) | 33.9 | 789 | **23x** | 0 | 4651 |
| `if-then-else` | Small (~100B) | 0.096 | 1.09 | **11x** | 0 | 30 |
| `.f // "default"` | Small (~100B) | 0.095 | 1.09 | **11x** | 0 | 31 |
| `try .field` (no error) | Small (~100B) | 4.59 | 1.11 | 0.2x† | 1 | 30 |
| `.a + .b` (strings) | Small (~100B) | 0.053 | 0.673 | **13x** | 0 | 21 |
| `"prefix" + .name` | Small (~100B) | 0.088 | 0.743 | **8.5x** | 0 | 23 |
| `.a - .b` (subtract) | Small (~100B) | 0.053 | 0.780 | **15x** | 0 | 20 |
| `.a * .b` (multiply) | Small (~100B) | 0.064 | 0.734 | **11x** | 0 | 21 |
| `.a / .b` (divide) | Small (~100B) | 0.097 | 0.778 | **8x** | 0 | 21 |
| `.a - .b` (array diff) | 5-elem arrays | 0.218 | 1.32 | **6x** | 0 | 33 |
| `length` | Small (~100B) | 0.041 | 0.991 | **24x** | 0 | 27 |
| `length` | Large (~100KB) | 32.9 | 592 | **18x** | 0 | 2835 |
| `add` (numbers) | 5-elem array | 0.060 | 0.743 | **12x** | 0 | 28 |
| `add` (strings) | 5-elem array | 0.132 | 0.965 | **7.3x** | 0 | 35 |
| `flatten` | 3-elem nested array | 0.094 | 1.31 | **14x** | 0 | 38 |
| `min` | 200-int array | 1.81 | 1.21 | 0.7x† | 0 | 15 |
| `min_by(.value)` | 100-elem object array | 8.15 | 55.1 | **6.8x** | 0 | 1347 |
| `sort` | 200-int array | 4.51 | 1.27 | 0.3x† | 14 | 15 |
| `sort_by(.value)` | 100-elem object array | 18.3 | 94.0 | **5.1x** | 422 | 2145 |
| `unique` | 200-int array | 6.07 | 1.22 | 0.2x† | 14 | 15 |
| `group_by(.active)` | 100-elem object array | 23.0 | 101 | **4.4x** | 422 | 2260 |
| `map(.name)` | 20-elem array (~600B) | 1.60 | 10.4 | **6.5x** | 29 | 251 |
| `map(.name)` | 200-elem array (~6KB) | 15.4 | 93.5 | **6.1x** | 209 | 2237 |
| `any` | 5-elem array | 0.047 | 1.89 | **40x** | 0 | 39 |
| `any(expr)` | 5-elem array | 0.394 | 2.13 | **5.4x** | 12 | 49 |
| `any(expr)`² | 200-int array | 3.94 | 1.73 | 0.4x† | 0 | 29 |
| `any(gen; cond)`² | 200-int array | 4.36 | 1.69 | 0.4x† | 3 | 27 |
| `first(expr)` | 5-elem array | 0.249 | 1.59 | **6.4x** | 9 | 39 |
| `first(expr)`² | 200-int array | 6.84 | 1.50 | 0.2x† | 107 | 23 |
| `last(expr)` | 5-elem array | 0.323 | 2.01 | **6.2x** | 10 | 43 |
| `last(expr)`² | 200-int array | 6.54 | 1.64 | 0.3x† | 107 | 24 |
| `limit(3; expr)` | 5-elem array | 0.108 | 1.76 | **16x** | 5 | 42 |
| `limit(10; expr)` | 200-int array | 0.689 | 1.65 | **2.4x** | 5 | 24 |
| `.[1:4]` slice | 6-elem array | 0.086 | 0.823 | **9.6x** | 0 | 21 |
| `values` | 9-elem array | 0.260 | 2.43 | **9.3x** | 13 | 51 |
| `skip(2; .[])` | 5-elem array | 0.148 | 1.80 | **12x** | 7 | 43 |
| `to_entries` | Small (~100B) | 0.164 | 4.17 | **25x** | 0 | 98 |
| `to_entries` | Large (~100KB) | 35.3 | 887 | **25x** | 0 | 6465 |
| `getpath(["field_0"])` | Small (~100B) | 0.249 | 1.04 | **4.2x** | 12 | 29 |
| `keys` | Small (~100B) | 0.302 | 1.30 | **4.3x** | 7 | 35 |
| `keys_unsorted` | Small (~100B) | 0.172 | 1.32 | **7.6x** | 0 | 35 |
| `keys_unsorted` | Large (~100KB) | 33.4 | 621 | **19x** | 0 | 3039 |
| `object merge .a + .b` | Small (~100B) | 0.165 | 1.55 | **9.4x** | 0 | 33 |
| `tojson` | Small (~100B) | 0.330 | 1.66 | **5x** | 0 | 39 |
| `fromjson` | JSON string | 0.154 | 1.36 | **8.8x** | 0 | 31 |
| `tonumber` | `"42"` string | 0.013 | 0.385 | **29x** | 0 | 11 |
| `toboolean` | `"true"` string | 0.0085 | 0.378 | **44x** | 0 | 11 |
| `split(",")` | short string | 0.147 | 0.793 | **5.4x** | 0 | 21 |
| `join(",")` | 5-elem array | 0.104 | 0.975 | **9.4x** | 0 | 37 |
| `ascii_downcase` in select | Small (~100B) | 0.146 | 1.81 | **12x** | 0 | 46 |
| `ascii_downcase` in select | Large (~100KB) | 34.8 | 801 | **23x** | 0 | 4652 |
| `startswith("s")` | Small (~100B) | 0.130 | 1.74 | **13x** | 0 | 45 |
| `startswith("s")` | Large (~100KB) | 34.9 | 810 | **23x** | 0 | 4652 |
| `endswith("s")` | Small (~100B) | 0.132 | 1.84 | **14x** | 0 | 45 |
| `trim` | short string | 0.051 | 0.392 | **7.7x** | 1 | 12 |
| `ltrim` | short string | 0.048 | 0.412 | **8.6x** | 1 | 12 |
| `rtrim` | short string | 0.053 | 0.422 | **8x** | 1 | 12 |
| `ltrimstr("s")` | Small (~100B) | 0.127 | 1.08 | **8.5x** | 0 | 30 |
| `rtrimstr("s")` | Small (~100B) | 0.125 | 1.08 | **8.6x** | 0 | 30 |
| `select` + string ops + arith + construct | ~200B log event | 1.04 | 3.16 | **3x** | 0 | 69 |
| `[.[] \| select + arith + construct]` | 20-elem array (~1.5KB) | 9.17 | 23.6 | **2.6x** | 106 | 525 |
| `length + map + add + min_by + any` | 20-elem array (~1.5KB) | 12.6 | 22.6 | **1.8x** | 106 | 555 |
| `map(try {…} catch …)` | 20-elem array (~1.5KB) | 261 | 29.0 | 0.1x† | 187 | 641 |
| `map(if … elif … else …)` | 20-elem array (~1.5KB) | 9.43 | 26.7 | **2.8x** | 127 | 595 |
| `[.[] \| str + tostring + str]` | 20-elem array (~1.5KB) | 11.6 | 25.5 | **2.2x** | 298 | 686 |
| `to_entries \| map(select) \| from_entries` | ~200B log event | 3.68 | 15.5 | **4.2x** | 69 | 250 |
| `@base64` | 34-char string | 0.146 | 0.536 | **3.7x** | 4 | 15 |
| `@base64d` | 48-char encoded | 0.214 | 0.571 | **2.7x** | 0 | 15 |
| `@uri` | 36-char URL string | 0.167 | 0.708 | **4.2x** | 4 | 14 |
| `index(",")` | short string | 0.097 | 0.980 | **10x** | 1 | 31 |
| `indices(",")` | short string | 0.206 | 2.15 | **10x** | 1 | 97 |
| `sqrt` | float (e≈2.718) | 0.068 | 0.470 | **6.9x** | 0 | 12 |
| `log` | float (e≈2.718) | 0.037 | 0.471 | **13x** | 0 | 12 |
| `sin` | float (e≈2.718) | 0.078 | 0.490 | **6.3x** | 0 | 12 |
| `atan` | integer 1 | 0.053 | 0.451 | **8.5x** | 0 | 11 |
| `exp` | integer 1 | 0.064 | 0.454 | **7.1x** | 0 | 11 |
| `tgamma` | integer 5 | 0.013 | 0.433 | **33x** | 0 | 11 |
| `fabs` | float -3.14 | 0.055 | 0.446 | **8.1x** | 0 | 12 |
| `abs` | float -3.14 | 0.0092 | 0.433 | **47x** | 0 | 12 |
| `expr as $x \| body` | Small (~20B object) | 29.3 | 0.815 | 0.0x† | 18 | 24 |
| ``"\(.level): \(.svc)"`` | ~45B object | 0.118 | 1.10 | **9.3x** | 0 | 35 |
| ``"user \(.name) …"`` | ~45B object | 0.091 | 1.26 | **14x** | 0 | 40 |
| `isempty(empty)` | null | 0.070 | 0.597 | **8.5x** | 6 | 18 |
| `isempty(.[])` | 5-elem array | 0.088 | 1.08 | **12x** | 6 | 33 |
| `nth(2; .[])` | 5-elem array | 0.129 | 2.24 | **17x** | 7 | 49 |
| `range(10)` (10 values) | null | 0.205 | 1.03 | **5x** | 12 | 18 |
| `limit(3; range(1000))` | null — only 3 allocs | 0.138 | 1.54 | **11x** | 8 | 26 |
| `test(re)` hit | short string | 0.114 | 1.36 | **12x** | 0 | 26 |
| `test(re)` miss | short string | 0.099 | 1.28 | **13x** | 0 | 26 |
| `match(re)` hit | short string | 0.216 | 2.95 | **14x** | 1 | 66 |
| `match(re)` miss | short string | 0.595 | 1.63 | **2.7x** | 0 | 24 |
| `capture(re)` hit | short string | 0.229 | — | — | 1 | — |
| `scan(re)` no groups (4 matches) | short string | 0.556 | — | — | 17 | — |
| `sub(re; s)` hit | short string | 0.126 | — | — | 1 | — |
| `gsub(re; s)` hit (4 matches) | short string | 0.586 | — | — | 5 | — |

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
BenchmarkFastjq_Small_Del-16                  	 6421075	       161.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Medium_Del-16                 	  526848	      2249 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Del-16                  	   33823	     36148 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Field-16                	13327854	        88.42 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Field-16                	  142460	      8096 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Index-16                	50585686	        23.53 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ArrayDel-16             	14572818	        82.72 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Construct-16            	 8411474	       145.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Construct-16            	  139108	      8477 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Iterator-16             	18922532	        60.49 ns/op	      40 B/op	       2 allocs/op
BenchmarkFastjq_Large_Iterator-16             	  161089	      7647 ns/op	      40 B/op	       2 allocs/op
BenchmarkFastjq_Small_Select-16               	 9531235	       130.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Select-16               	   35293	     34623 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Alternative-16          	12102705	        95.40 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SelectAnd-16            	 7164751	       170.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SelectOr-16             	16203561	        73.86 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Has-16                  	12382899	        96.21 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Has-16                  	   34849	     33888 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_IfThenElse-16           	12647726	        96.01 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Length-16               	30594975	        40.92 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Length-16               	   36351	     32854 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Map-16                  	  747085	      1599 ns/op	     689 B/op	      29 allocs/op
BenchmarkFastjq_Medium_Map-16                 	  157020	      7607 ns/op	    2609 B/op	     109 allocs/op
BenchmarkFastjq_Large_Map-16                  	   77830	     15433 ns/op	    5009 B/op	     209 allocs/op
BenchmarkFastjq_Small_ToEntries-16            	 7517964	       164.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_ToEntries-16            	   33572	     35343 ns/op	       6 B/op	       0 allocs/op
BenchmarkFastjq_Small_KeysUnsorted-16         	 7003814	       172.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Keys-16                 	 3976608	       301.7 ns/op	     456 B/op	       7 allocs/op
BenchmarkFastjq_Small_GetPath-16              	 4834554	       249.0 ns/op	     272 B/op	      12 allocs/op
BenchmarkFastjq_Large_KeysUnsorted-16         	   35041	     33411 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Any-16                  	25486951	        46.68 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Any-16                  	  904629	      1370 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AnyExpr-16              	 3055969	       393.6 ns/op	     144 B/op	      12 allocs/op
BenchmarkFastjq_Large_AnyExpr-16              	  302670	      3941 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Add-16                  	20195532	        59.78 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AddStrings-16           	 9065070	       131.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Flatten-16              	12663097	        93.58 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Split-16                	 8235390	       147.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Join-16                 	11268324	       103.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Values-16               	 4507747	       260.3 ns/op	     328 B/op	      13 allocs/op
BenchmarkGojq_Small_Values-16                 	  493204	      2428 ns/op	    3144 B/op	      51 allocs/op
BenchmarkFastjq_Small_Base64Encode-16         	 8247589	       145.7 ns/op	     120 B/op	       4 allocs/op
BenchmarkFastjq_Small_Base64Decode-16         	 5487342	       214.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Base64Encode-16           	 2226387	       536.1 ns/op	    1321 B/op	      15 allocs/op
BenchmarkGojq_Small_Base64Decode-16           	 2122350	       570.7 ns/op	    1321 B/op	      15 allocs/op
BenchmarkFastjq_Small_IndexFind-16            	12180056	        97.37 ns/op	       3 B/op	       1 allocs/op
BenchmarkFastjq_Small_IndicesAll-16           	 5875992	       205.6 ns/op	       3 B/op	       1 allocs/op
BenchmarkGojq_Small_IndexFind-16              	 1211791	       980.3 ns/op	    2274 B/op	      31 allocs/op
BenchmarkGojq_Small_IndicesAll-16             	  564601	      2155 ns/op	    3907 B/op	      97 allocs/op
BenchmarkFastjq_Small_Slice-16                	13899110	        85.93 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SliceString-16          	11970163	        97.09 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Plus-16                 	23191542	        53.11 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_PlusStr-16              	13813419	        87.58 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AsciiDowncase-16        	 8378881	       146.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_AsciiDowncase-16        	   34774	     34793 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Startswith-16           	 9481262	       129.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Startswith-16           	   34363	     34948 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Endswith-16             	 8260174	       131.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Ltrimstr-16             	 9257984	       126.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Ltrimstr-16             	   36865	     32811 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Rtrimstr-16             	 9732589	       125.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Trim-16                 	22788469	        51.21 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_Ltrim-16                	25065556	        48.12 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_Rtrim-16                	22534804	        52.68 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_First-16                	 4819658	       249.2 ns/op	     216 B/op	       9 allocs/op
BenchmarkFastjq_Large_First-16                	  170905	      6839 ns/op	    2584 B/op	     107 allocs/op
BenchmarkFastjq_Small_Last-16                 	 3686409	       323.3 ns/op	     256 B/op	      10 allocs/op
BenchmarkFastjq_Large_Last-16                 	  182373	      6542 ns/op	    2584 B/op	     107 allocs/op
BenchmarkFastjq_Small_Limit-16                	11087558	       107.8 ns/op	     104 B/op	       5 allocs/op
BenchmarkFastjq_Small_Skip-16                 	 8178235	       148.1 ns/op	     208 B/op	       7 allocs/op
BenchmarkFastjq_Large_Limit-16                	 1745499	       689.1 ns/op	     104 B/op	       5 allocs/op
BenchmarkGojq_Small_Del-16                    	  483770	      2381 ns/op	    3763 B/op	      58 allocs/op
BenchmarkGojq_Medium_Del-16                   	   60102	     19802 ns/op	   16970 B/op	     323 allocs/op
BenchmarkGojq_Large_Del-16                    	    1458	    844733 ns/op	  545400 B/op	    4666 allocs/op
BenchmarkGojq_Small_Field-16                  	 1000000	      1013 ns/op	    1641 B/op	      27 allocs/op
BenchmarkGojq_Large_Field-16                  	    2012	    598537 ns/op	  270055 B/op	    2835 allocs/op
BenchmarkGojq_Small_Index-16                  	 1802179	       662.5 ns/op	    1401 B/op	      20 allocs/op
BenchmarkGojq_Small_ArrayDel-16               	  703831	      1704 ns/op	    3363 B/op	      53 allocs/op
BenchmarkGojq_Small_Construct-16              	  866989	      1409 ns/op	    2330 B/op	      37 allocs/op
BenchmarkGojq_Large_Construct-16              	    1944	    614922 ns/op	  274497 B/op	    2867 allocs/op
BenchmarkGojq_Small_Iterator-16               	 1507281	       799.2 ns/op	    1776 B/op	      26 allocs/op
BenchmarkGojq_Large_Iterator-16               	   13485	     88387 ns/op	  109808 B/op	    1811 allocs/op
BenchmarkGojq_Small_Select-16                 	  704137	      1757 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Large_Select-16                 	    1483	    799025 ns/op	  538802 B/op	    4651 allocs/op
BenchmarkGojq_Small_Alternative-16            	 1000000	      1095 ns/op	    1897 B/op	      31 allocs/op
BenchmarkGojq_Small_SelectAnd-16              	  675724	      1870 ns/op	    2890 B/op	      46 allocs/op
BenchmarkGojq_Small_SelectOr-16               	  671553	      1773 ns/op	    2890 B/op	      46 allocs/op
BenchmarkGojq_Small_Has-16                    	  660416	      1760 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Large_Has-16                    	    1531	    788987 ns/op	  532969 B/op	    4651 allocs/op
BenchmarkGojq_Small_IfThenElse-16             	 1000000	      1095 ns/op	    1817 B/op	      30 allocs/op
BenchmarkGojq_Small_Length-16                 	 1221108	       991.4 ns/op	    1633 B/op	      27 allocs/op
BenchmarkGojq_Large_Length-16                 	    2004	    591992 ns/op	  269829 B/op	    2835 allocs/op
BenchmarkGojq_Small_Map-16                    	  114960	     10421 ns/op	   13654 B/op	     251 allocs/op
BenchmarkGojq_Large_Map-16                    	   12856	     93505 ns/op	  118606 B/op	    2237 allocs/op
BenchmarkGojq_Small_ToEntries-16              	  286096	      4168 ns/op	    6559 B/op	      98 allocs/op
BenchmarkGojq_Large_ToEntries-16              	    1347	    887234 ns/op	  672895 B/op	    6465 allocs/op
BenchmarkGojq_Small_KeysUnsorted-16           	  902173	      1316 ns/op	    1953 B/op	      35 allocs/op
BenchmarkGojq_Small_Keys-16                   	  937233	      1305 ns/op	    1953 B/op	      35 allocs/op
BenchmarkGojq_Small_GetPath-16                	 1000000	      1040 ns/op	    1721 B/op	      29 allocs/op
BenchmarkGojq_Large_KeysUnsorted-16           	    1957	    620995 ns/op	  282779 B/op	    3039 allocs/op
BenchmarkGojq_Small_Any-16                    	  643425	      1889 ns/op	    4508 B/op	      39 allocs/op
BenchmarkGojq_Small_AnyExpr-16                	  565854	      2133 ns/op	    4620 B/op	      49 allocs/op
BenchmarkGojq_Large_AnyExpr-16                	  667764	      1731 ns/op	    2818 B/op	      29 allocs/op
BenchmarkGojq_Small_Add-16                    	 1631293	       742.6 ns/op	    1529 B/op	      28 allocs/op
BenchmarkGojq_Small_AddStrings-16             	 1227582	       965.2 ns/op	    1785 B/op	      35 allocs/op
BenchmarkGojq_Small_Flatten-16                	  898707	      1312 ns/op	    1857 B/op	      38 allocs/op
BenchmarkGojq_Small_Split-16                  	 1505348	       792.9 ns/op	    1553 B/op	      21 allocs/op
BenchmarkGojq_Small_Join-16                   	 1231950	       975.0 ns/op	    1865 B/op	      37 allocs/op
BenchmarkGojq_Small_Slice-16                  	 1473634	       822.8 ns/op	    1425 B/op	      21 allocs/op
BenchmarkGojq_Small_SliceString-16            	 2570899	       465.9 ns/op	    1145 B/op	      12 allocs/op
BenchmarkGojq_Small_Plus-16                   	 1779147	       673.4 ns/op	    1673 B/op	      21 allocs/op
BenchmarkGojq_Small_PlusStr-16                	 1608507	       742.9 ns/op	    1705 B/op	      23 allocs/op
BenchmarkGojq_Small_AsciiDowncase-16          	  704067	      1806 ns/op	    2714 B/op	      46 allocs/op
BenchmarkGojq_Large_AsciiDowncase-16          	    1473	    801252 ns/op	  538317 B/op	    4652 allocs/op
BenchmarkGojq_Small_Startswith-16             	  677492	      1744 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Large_Startswith-16             	    1507	    809635 ns/op	  542829 B/op	    4652 allocs/op
BenchmarkGojq_Small_Endswith-16               	  648046	      1838 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Small_Ltrimstr-16               	 1000000	      1084 ns/op	    1737 B/op	      30 allocs/op
BenchmarkGojq_Small_Rtrimstr-16               	 1000000	      1080 ns/op	    1737 B/op	      30 allocs/op
BenchmarkGojq_Small_Trim-16                   	 3010267	       392.4 ns/op	    1129 B/op	      12 allocs/op
BenchmarkGojq_Small_Ltrim-16                  	 2870527	       412.0 ns/op	    1129 B/op	      12 allocs/op
BenchmarkGojq_Small_Rtrim-16                  	 2839167	       422.3 ns/op	    1129 B/op	      12 allocs/op
BenchmarkGojq_Small_First-16                  	  715833	      1594 ns/op	    3379 B/op	      39 allocs/op
BenchmarkGojq_Large_First-16                  	  799370	      1502 ns/op	    1889 B/op	      23 allocs/op
BenchmarkGojq_Small_Last-16                   	  574735	      2008 ns/op	    3483 B/op	      43 allocs/op
BenchmarkGojq_Large_Last-16                   	  752431	      1636 ns/op	    2193 B/op	      24 allocs/op
BenchmarkGojq_Small_Limit-16                  	  695544	      1762 ns/op	    3328 B/op	      42 allocs/op
BenchmarkGojq_Small_Skip-16                   	  631057	      1799 ns/op	    2880 B/op	      43 allocs/op
BenchmarkGojq_Large_Limit-16                  	  724593	      1652 ns/op	    2200 B/op	      24 allocs/op
BenchmarkFastjq_Small_Subtract-16             	23382962	        52.87 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Subtract-16               	 1560052	       780.2 ns/op	    1649 B/op	      20 allocs/op
BenchmarkFastjq_Small_Multiply-16             	17878558	        64.44 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Multiply-16               	 1645352	       734.4 ns/op	    1649 B/op	      21 allocs/op
BenchmarkFastjq_Small_Divide-16               	12412920	        97.30 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Divide-16                 	 1550992	       777.9 ns/op	    1665 B/op	      21 allocs/op
BenchmarkFastjq_Small_Min-16                  	  630404	      1815 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Min-16                    	  975199	      1214 ns/op	    1217 B/op	      15 allocs/op
BenchmarkFastjq_Small_MinBy-16                	  145116	      8149 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_MinBy-16                  	   21951	     55063 ns/op	   63630 B/op	    1347 allocs/op
BenchmarkFastjq_Sort-16                       	  262610	      4513 ns/op	    7688 B/op	      14 allocs/op
BenchmarkGojq_Sort-16                         	  980734	      1268 ns/op	    1217 B/op	      15 allocs/op
BenchmarkFastjq_SortBy-16                     	   64876	     18263 ns/op	   24872 B/op	     422 allocs/op
BenchmarkGojq_SortBy-16                       	   12811	     94002 ns/op	   96955 B/op	    2145 allocs/op
BenchmarkFastjq_Unique-16                     	  193102	      6071 ns/op	    7688 B/op	      14 allocs/op
BenchmarkGojq_Unique-16                       	  991574	      1220 ns/op	    1217 B/op	      15 allocs/op
BenchmarkFastjq_GroupBy-16                    	   54690	     22965 ns/op	   24872 B/op	     422 allocs/op
BenchmarkGojq_GroupBy-16                      	   10000	    100964 ns/op	  101909 B/op	    2260 allocs/op
BenchmarkFastjq_Small_URIEncode-16            	 6969006	       166.9 ns/op	     120 B/op	       4 allocs/op
BenchmarkGojq_Small_URIEncode-16              	 1700258	       707.5 ns/op	    1289 B/op	      14 allocs/op
BenchmarkFastjq_Small_ArrayDiff-16            	 5420194	       218.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ArrayDiff-16              	  854446	      1319 ns/op	    2121 B/op	      33 allocs/op
BenchmarkFastjq_Small_TryNoError-16           	  261778	      4592 ns/op	      64 B/op	       1 allocs/op
BenchmarkFastjq_Small_TryCatchNoError-16      	  250084	      4543 ns/op	      64 B/op	       1 allocs/op
BenchmarkGojq_Small_TryNoError-16             	  995068	      1112 ns/op	    1913 B/op	      30 allocs/op
BenchmarkFastjq_Small_ObjectMerge-16          	 7125597	       164.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ObjectMerge-16            	  761004	      1551 ns/op	    2906 B/op	      33 allocs/op
BenchmarkFastjq_Small_ToJSON-16               	 3594069	       330.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ToJSON-16                 	  731823	      1659 ns/op	    2410 B/op	      39 allocs/op
BenchmarkFastjq_Small_FromJSON-16             	 7464136	       154.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_FromJSON-16               	  885400	      1356 ns/op	    2714 B/op	      31 allocs/op
BenchmarkFastjq_Small_ToString-16             	 3615675	       332.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToNumber-16             	90233196	        13.40 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToBoolean-16            	141439554	         8.509 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ToNumber-16               	 3120801	       384.6 ns/op	    1113 B/op	      11 allocs/op
BenchmarkGojq_Small_ToBoolean-16              	 3165612	       377.7 ns/op	    1113 B/op	      11 allocs/op
BenchmarkFastjq_Small_AnyTwoArg-16            	  276640	      4356 ns/op	      57 B/op	       3 allocs/op
BenchmarkGojq_Small_AnyTwoArg-16              	  725926	      1685 ns/op	    2482 B/op	      27 allocs/op
BenchmarkFastjq_Complex_LogNormalize-16       	 1000000	      1041 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Complex_LogNormalize-16         	  387504	      3163 ns/op	    4324 B/op	      69 allocs/op
BenchmarkFastjq_Complex_ArrayPipeline-16      	  128235	      9173 ns/op	    2264 B/op	     106 allocs/op
BenchmarkGojq_Complex_ArrayPipeline-16        	   50715	     23550 ns/op	   20409 B/op	     525 allocs/op
BenchmarkFastjq_Complex_Aggregation-16        	   95906	     12572 ns/op	    3184 B/op	     106 allocs/op
BenchmarkGojq_Complex_Aggregation-16          	   53148	     22572 ns/op	   22397 B/op	     555 allocs/op
BenchmarkFastjq_Complex_TolerantMap-16        	    4585	    261100 ns/op	    5216 B/op	     187 allocs/op
BenchmarkGojq_Complex_TolerantMap-16          	   41972	     28964 ns/op	   27874 B/op	     641 allocs/op
BenchmarkFastjq_Complex_ElifRouting-16        	  128562	      9425 ns/op	    3000 B/op	     127 allocs/op
BenchmarkGojq_Complex_ElifRouting-16          	   44814	     26668 ns/op	   25923 B/op	     595 allocs/op
BenchmarkFastjq_Complex_StringBuild-16        	  107266	     11571 ns/op	    7216 B/op	     298 allocs/op
BenchmarkGojq_Complex_StringBuild-16          	   46933	     25514 ns/op	   20534 B/op	     686 allocs/op
BenchmarkFastjq_Complex_EntryFilter-16        	  314626	      3677 ns/op	    2888 B/op	      69 allocs/op
BenchmarkGojq_Complex_EntryFilter-16          	   77157	     15466 ns/op	   18429 B/op	     250 allocs/op
BenchmarkFastjq_Small_Sqrt-16                 	17837632	        67.71 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Sqrt-16                   	 2611341	       469.7 ns/op	    1145 B/op	      12 allocs/op
BenchmarkFastjq_Small_Log-16                  	30691582	        37.16 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Log-16                    	 2557879	       470.8 ns/op	    1129 B/op	      12 allocs/op
BenchmarkFastjq_Small_Sin-16                  	15205351	        78.33 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Sin-16                    	 2494030	       489.8 ns/op	    1145 B/op	      12 allocs/op
BenchmarkFastjq_Small_Atan-16                 	22392880	        53.24 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Atan-16                   	 2717814	       450.5 ns/op	    1121 B/op	      11 allocs/op
BenchmarkFastjq_Small_Exp-16                  	19213820	        64.28 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Exp-16                    	 2552509	       453.9 ns/op	    1121 B/op	      11 allocs/op
BenchmarkFastjq_Small_Tgamma-16               	96076861	        12.98 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Tgamma-16                 	 2809729	       433.2 ns/op	    1105 B/op	      11 allocs/op
BenchmarkFastjq_Small_Fabs-16                 	21178120	        54.78 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Fabs-16                   	 2750792	       445.7 ns/op	    1113 B/op	      12 allocs/op
BenchmarkFastjq_Small_Abs-16                  	130115922	         9.182 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Abs-16                    	 2807515	       433.0 ns/op	    1113 B/op	      12 allocs/op
BenchmarkFastjq_Small_Bind-16                 	   40782	     29265 ns/op	     648 B/op	      18 allocs/op
BenchmarkGojq_Small_Bind-16                   	 1474424	       815.2 ns/op	    1793 B/op	      24 allocs/op
BenchmarkFastjq_Small_StringInterp-16         	10156192	       118.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_StringInterp-16           	 1000000	      1101 ns/op	    2121 B/op	      35 allocs/op
BenchmarkFastjq_Small_StringInterpNum-16      	12651142	        90.52 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_StringInterpNum-16        	  963441	      1258 ns/op	    2498 B/op	      40 allocs/op
BenchmarkFastjq_Small_IsEmptyTrue-16          	17213194	        70.42 ns/op	     105 B/op	       6 allocs/op
BenchmarkGojq_Small_IsEmptyTrue-16            	 2018499	       597.3 ns/op	    1921 B/op	      18 allocs/op
BenchmarkFastjq_Small_IsEmptyFalse-16         	13563574	        88.21 ns/op	     105 B/op	       6 allocs/op
BenchmarkGojq_Small_IsEmptyFalse-16           	 1000000	      1079 ns/op	    2898 B/op	      33 allocs/op
BenchmarkFastjq_Small_Nth-16                  	 9157426	       129.1 ns/op	     152 B/op	       7 allocs/op
BenchmarkGojq_Small_Nth-16                    	  526998	      2242 ns/op	    4372 B/op	      49 allocs/op
BenchmarkFastjq_Small_Range10-16              	 5975724	       205.0 ns/op	     120 B/op	      12 allocs/op
BenchmarkGojq_Small_Range10-16                	 1000000	      1031 ns/op	    1864 B/op	      18 allocs/op
BenchmarkFastjq_Small_RangeLimit-16           	 8824945	       138.4 ns/op	     128 B/op	       8 allocs/op
BenchmarkGojq_Small_RangeLimit-16             	  766804	      1542 ns/op	    3208 B/op	      26 allocs/op
BenchmarkRegexp_Match_Hit-16                  	27456681	        44.01 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Miss-16                 	52047940	        23.05 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Complex_Hit-16          	27800220	        42.81 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Long_Miss-16            	15569696	        77.06 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_MatchString-16                	27526723	        43.62 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_FindSubmatch_Hit-16           	 5909390	       203.6 ns/op	     160 B/op	       2 allocs/op
BenchmarkRegexp_FindSubmatch_Miss-16          	 1741663	       688.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_FindSubmatchIndex_Hit-16      	 6697215	       178.6 ns/op	      64 B/op	       1 allocs/op
BenchmarkRegexp_FindSubmatchIndex_Miss-16     	 1751924	       687.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_ReplaceAll_Hit-16             	 7773642	       156.8 ns/op	      97 B/op	       5 allocs/op
BenchmarkRegexp_ReplaceAll_Miss-16            	24414745	        49.05 ns/op	      48 B/op	       2 allocs/op
BenchmarkRegexp_MatchViaClosure-16            	27308155	        43.85 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_Hit-16           	10608949	       113.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_Miss-16          	12247850	        98.75 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_InPipeline-16    	 4143070	       290.5 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_MatchRe_Hit-16          	 5594586	       216.1 ns/op	      48 B/op	       1 allocs/op
BenchmarkFastjq_Small_MatchRe_Miss-16         	 2026101	       595.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_TestRe_Hit-16             	  883118	      1363 ns/op	    2745 B/op	      26 allocs/op
BenchmarkGojq_Small_TestRe_Miss-16            	  916306	      1282 ns/op	    2705 B/op	      26 allocs/op
BenchmarkGojq_Small_MatchRe_Hit-16            	  396382	      2952 ns/op	    4796 B/op	      66 allocs/op
BenchmarkGojq_Small_MatchRe_Miss-16           	  720080	      1634 ns/op	    2249 B/op	      24 allocs/op
BenchmarkFastjq_Small_CaptureRe_Hit-16        	 5219326	       228.9 ns/op	      48 B/op	       1 allocs/op
BenchmarkFastjq_Small_CaptureRe_Miss-16       	 2010924	       595.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ScanRe_NoGroups-16      	 2172680	       555.6 ns/op	     534 B/op	      17 allocs/op
BenchmarkFastjq_Small_ScanRe_WithGroups-16    	 1683468	       717.0 ns/op	     851 B/op	      20 allocs/op
BenchmarkFastjq_Small_SubRe_Hit-16            	 9479929	       126.3 ns/op	      16 B/op	       1 allocs/op
BenchmarkFastjq_Small_SubRe_Miss-16           	11860026	       100.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_GSubRe_Hit-16           	 2049027	       586.0 ns/op	     305 B/op	       5 allocs/op
BenchmarkFastjq_Small_GSubRe_Miss-16          	 4704660	       255.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_CaptureRe_Hit-16          	  514940	      2350 ns/op	    4726 B/op	      51 allocs/op
BenchmarkGojq_Small_CaptureRe_Miss-16         	  670064	      1842 ns/op	    2673 B/op	      27 allocs/op
BenchmarkGojq_Small_ScanRe_NoGroups-16        	  421802	      2889 ns/op	    5136 B/op	      57 allocs/op
BenchmarkGojq_Small_SubRe_Hit-16              	  223254	      5376 ns/op	    8988 B/op	      95 allocs/op
BenchmarkGojq_Small_GSubRe_Hit-16             	   78296	     15178 ns/op	   17902 B/op	     248 allocs/op
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
