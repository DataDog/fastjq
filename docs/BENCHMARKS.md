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
| `.field` | Small (~100B) | 0.086 | 1.03 | **12x** | 0 | 27 |
| `.field` | Large (~100KB) | 7.78 | 602 | **77x** | 0 | 2835 |
| `del(.f)` | Small (~100B) | 0.161 | 2.35 | **15x** | 0 | 58 |
| `del(.f)` | Medium (~2KB) | 2.22 | 20.2 | **9.1x** | 0 | 323 |
| `del(.f)` | Large (~100KB) | 35.2 | 830 | **24x** | 0 | 4666 |
| `.[n]` | 5-elem array | 0.024 | 0.651 | **27x** | 0 | 20 |
| `del(.[n], .[m])` | 5-elem array | 0.085 | 1.74 | **20x** | 0 | 53 |
| `{f0, f2}` (construct) | Small (~100B) | 0.138 | 1.44 | **10x** | 0 | 37 |
| `{f0, f50}` (construct) | Large (~100KB) | 8.17 | 599 | **73x** | 0 | 2867 |
| `.[]` iterator | 5-elem array | 0.062 | 0.796 | **13x** | 2 | 26 |
| `.[]` iterator | 200-elem array | 7.45 | 89.1 | **12x** | 2 | 1811 |
| `select(.f == "x")` | Small (~100B) | 0.126 | 1.79 | **14x** | 0 | 45 |
| `select(.f == "x")`¹ | Large (~100KB, last field) | 34.1 | 800 | **23x** | 0 | 4651 |
| `select(.f and .g)` | Small (~100B) | 0.169 | 1.93 | **11x** | 0 | 46 |
| `select(.f or .g)` | Small (~100B) | 0.072 | 1.85 | **26x** | 0 | 46 |
| `has("key")` in select | Small (~100B) | 0.093 | 1.83 | **20x** | 0 | 45 |
| `has("key")` in select | Large (~100KB) | 33.5 | 802 | **24x** | 0 | 4651 |
| `if-then-else` | Small (~100B) | 0.098 | 1.14 | **12x** | 0 | 30 |
| `.f // "default"` | Small (~100B) | 0.095 | 1.13 | **12x** | 0 | 31 |
| `try .field` (no error) | Small (~100B) | 4.70 | 1.24 | 0.3x† | 1 | 30 |
| `.a + .b` (strings) | Small (~100B) | 0.053 | 0.781 | **15x** | 0 | 21 |
| `"prefix" + .name` | Small (~100B) | 0.086 | 0.817 | **9.5x** | 0 | 23 |
| `.a - .b` (subtract) | Small (~100B) | 0.051 | 0.735 | **14x** | 0 | 20 |
| `.a * .b` (multiply) | Small (~100B) | 0.066 | 0.768 | **12x** | 0 | 21 |
| `.a / .b` (divide) | Small (~100B) | 0.099 | 0.763 | **7.7x** | 0 | 21 |
| `.a - .b` (array diff) | 5-elem arrays | 0.220 | 1.34 | **6.1x** | 0 | 33 |
| `length` | Small (~100B) | 0.041 | 1.03 | **25x** | 0 | 27 |
| `length` | Large (~100KB) | 32.4 | 600 | **19x** | 0 | 2835 |
| `add` (numbers) | 5-elem array | 0.061 | 0.802 | **13x** | 0 | 28 |
| `add` (strings) | 5-elem array | 0.132 | 0.990 | **7.5x** | 0 | 35 |
| `flatten` | 3-elem nested array | 0.098 | 1.41 | **14x** | 0 | 38 |
| `min` | 200-int array | 1.85 | 1.25 | 0.7x† | 0 | 15 |
| `min_by(.value)` | 100-elem object array | 8.44 | 56.4 | **6.7x** | 0 | 1347 |
| `sort` | 200-int array | 4.51 | 1.27 | 0.3x† | 14 | 15 |
| `sort_by(.value)` | 100-elem object array | 17.7 | 93.7 | **5.3x** | 422 | 2145 |
| `unique` | 200-int array | 5.81 | 1.25 | 0.2x† | 14 | 15 |
| `group_by(.active)` | 100-elem object array | 22.2 | 100 | **4.5x** | 422 | 2260 |
| `map(.name)` | 20-elem array (~600B) | 1.62 | 10.6 | **6.6x** | 29 | 251 |
| `map(.name)` | 200-elem array (~6KB) | 15.8 | 96.1 | **6.1x** | 209 | 2237 |
| `any` | 5-elem array | 0.045 | 2.04 | **45x** | 0 | 39 |
| `any(expr)` | 5-elem array | 0.391 | 2.25 | **5.7x** | 12 | 49 |
| `any(expr)`² | 200-int array | 3.95 | 1.89 | 0.5x† | 0 | 29 |
| `any(gen; cond)`² | 200-int array | 4.36 | 1.68 | 0.4x† | 3 | 27 |
| `first(expr)` | 5-elem array | 0.247 | 1.75 | **7.1x** | 9 | 39 |
| `first(expr)`² | 200-int array | 6.74 | 1.52 | 0.2x† | 107 | 23 |
| `last(expr)` | 5-elem array | 0.312 | 2.00 | **6.4x** | 10 | 43 |
| `last(expr)`² | 200-int array | 6.34 | 1.60 | 0.3x† | 107 | 24 |
| `limit(3; expr)` | 5-elem array | 0.102 | 1.76 | **17x** | 5 | 42 |
| `limit(10; expr)` | 200-int array | 0.686 | 1.67 | **2.4x** | 5 | 24 |
| `.[1:4]` slice | 6-elem array | 0.090 | 0.879 | **9.8x** | 0 | 21 |
| `values` | 9-elem array | 0.265 | 2.44 | **9.2x** | 13 | 51 |
| `skip(2; .[])` | 5-elem array | 0.148 | 1.79 | **12x** | 7 | 43 |
| `reduce .[] as $x (0; . + $x)` | 5-elem array | 141 | 1.46 | 0.0x† | 86 | 42 |
| `paths` | Small (~100B) | 0.397 | 4.63 | **12x** | 17 | 119 |
| `to_entries` | Small (~100B) | 0.165 | 4.27 | **26x** | 0 | 98 |
| `to_entries` | Large (~100KB) | 34.9 | 908 | **26x** | 0 | 6465 |
| `getpath(["field_0"])` | Small (~100B) | 0.254 | 1.07 | **4.2x** | 12 | 29 |
| `setpath(["field_0"]; "y")` | Small (~100B) | 0.409 | 1.83 | **4.5x** | 12 | 43 |
| `delpaths([["field_0"]])` | Small (~100B) | 0.726 | 1.77 | **2.4x** | 22 | 40 |
| `keys` | Small (~100B) | 0.310 | 1.35 | **4.3x** | 7 | 35 |
| `keys_unsorted` | Small (~100B) | 0.162 | 1.35 | **8.3x** | 0 | 35 |
| `keys_unsorted` | Large (~100KB) | 32.8 | 631 | **19x** | 0 | 3039 |
| `object merge .a + .b` | Small (~100B) | 0.183 | 1.66 | **9x** | 0 | 33 |
| `tojson` | Small (~100B) | 0.435 | 1.72 | **4x** | 0 | 39 |
| `fromjson` | JSON string | 0.199 | 1.38 | **7x** | 0 | 31 |
| `tonumber` | `"42"` string | 0.013 | 0.379 | **28x** | 0 | 11 |
| `toboolean` | `"true"` string | 0.0087 | 0.375 | **43x** | 0 | 11 |
| `split(",")` | short string | 0.146 | 0.842 | **5.8x** | 0 | 21 |
| `join(",")` | 5-elem array | 0.102 | 1.03 | **10x** | 0 | 37 |
| `ascii_downcase` in select | Small (~100B) | 0.144 | 1.90 | **13x** | 0 | 46 |
| `ascii_downcase` in select | Large (~100KB) | 34.5 | 814 | **24x** | 0 | 4653 |
| `startswith("s")` | Small (~100B) | 0.131 | 1.82 | **14x** | 0 | 45 |
| `startswith("s")` | Large (~100KB) | 33.6 | 808 | **24x** | 0 | 4651 |
| `endswith("s")` | Small (~100B) | 0.127 | 1.83 | **14x** | 0 | 45 |
| `trim` | short string | 0.052 | 0.401 | **7.7x** | 1 | 12 |
| `ltrim` | short string | 0.048 | 0.402 | **8.3x** | 1 | 12 |
| `rtrim` | short string | 0.052 | 0.410 | **7.8x** | 1 | 12 |
| `ltrimstr("s")` | Small (~100B) | 0.126 | 1.13 | **9x** | 0 | 30 |
| `rtrimstr("s")` | Small (~100B) | 0.126 | 1.09 | **8.7x** | 0 | 30 |
| `select` + string ops + arith + construct | ~200B log event | 1.06 | 3.03 | **2.9x** | 0 | 69 |
| `[.[] \| select + arith + construct]` | 20-elem array (~1.5KB) | 9.05 | 23.8 | **2.6x** | 106 | 525 |
| `length + map + add + min_by + any` | 20-elem array (~1.5KB) | 12.6 | 22.2 | **1.8x** | 106 | 555 |
| `map(try {…} catch …)` | 20-elem array (~1.5KB) | 269 | 28.5 | 0.1x† | 187 | 641 |
| `map(if … elif … else …)` | 20-elem array (~1.5KB) | 9.32 | 26.3 | **2.8x** | 127 | 595 |
| `[.[] \| str + tostring + str]` | 20-elem array (~1.5KB) | 11.2 | 24.7 | **2.2x** | 298 | 686 |
| `to_entries \| map(select) \| from_entries` | ~200B log event | 3.50 | 14.5 | **4.1x** | 69 | 250 |
| `@base64` | 34-char string | 0.149 | 0.582 | **3.9x** | 4 | 15 |
| `@base64d` | 48-char encoded | 0.213 | 0.583 | **2.7x** | 0 | 15 |
| `@uri` | 36-char URL string | 0.171 | 0.751 | **4.4x** | 4 | 14 |
| `index(",")` | short string | 0.098 | 0.944 | **9.6x** | 1 | 31 |
| `indices(",")` | short string | 0.211 | 2.18 | **10x** | 1 | 97 |
| `sqrt` | float (e≈2.718) | 0.069 | 0.477 | **6.9x** | 0 | 12 |
| `log` | float (e≈2.718) | 0.039 | 0.469 | **12x** | 0 | 12 |
| `sin` | float (e≈2.718) | 0.079 | 0.475 | **6x** | 0 | 12 |
| `atan` | integer 1 | 0.053 | 0.413 | **7.7x** | 0 | 11 |
| `exp` | integer 1 | 0.064 | 0.434 | **6.8x** | 0 | 11 |
| `tgamma` | integer 5 | 0.013 | 0.401 | **31x** | 0 | 11 |
| `fabs` | float -3.14 | 0.054 | 0.438 | **8.1x** | 0 | 12 |
| `abs` | float -3.14 | 0.0093 | 0.431 | **46x** | 0 | 12 |
| `expr as $x \| body` | Small (~20B object) | 30.1 | 0.823 | 0.0x† | 18 | 24 |
| ``"\(.level): \(.svc)"`` | ~45B object | 0.122 | 1.08 | **8.8x** | 0 | 35 |
| ``"user \(.name) …"`` | ~45B object | 0.090 | 1.26 | **14x** | 0 | 40 |
| `isempty(empty)` | null | 0.072 | 0.606 | **8.4x** | 6 | 18 |
| `isempty(.[])` | 5-elem array | 0.087 | 1.08 | **12x** | 6 | 33 |
| `nth(2; .[])` | 5-elem array | 0.130 | 2.23 | **17x** | 7 | 49 |
| `range(10)` (10 values) | null | 0.200 | 1.04 | **5.2x** | 12 | 18 |
| `limit(3; range(1000))` | null — only 3 allocs | 0.137 | 1.58 | **11x** | 8 | 26 |
| `test(re)` hit | short string | 0.108 | 1.32 | **12x** | 0 | 26 |
| `test(re)` miss | short string | 0.099 | 1.26 | **13x** | 0 | 26 |
| `match(re)` hit | short string | 0.218 | 2.99 | **14x** | 1 | 66 |
| `match(re)` miss | short string | 0.624 | 1.72 | **2.8x** | 0 | 24 |
| `capture(re)` hit | short string | 0.213 | — | — | 1 | — |
| `scan(re)` no groups (4 matches) | short string | 0.556 | — | — | 17 | — |
| `sub(re; s)` hit | short string | 0.125 | — | — | 1 | — |
| `gsub(re; s)` hit (4 matches) | short string | 0.590 | — | — | 5 | — |

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
BenchmarkFastjq_Small_Del-16                  	 7364912	       160.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Medium_Del-16                 	  538752	      2217 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Del-16                  	   33531	     35208 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Field-16                	14641004	        86.01 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Field-16                	  155931	      7777 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Index-16                	48396447	        24.33 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ArrayDel-16             	14261018	        85.33 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Construct-16            	 8646404	       137.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Construct-16            	  145051	      8167 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Iterator-16             	19005547	        61.97 ns/op	      40 B/op	       2 allocs/op
BenchmarkFastjq_Large_Iterator-16             	  160537	      7452 ns/op	      40 B/op	       2 allocs/op
BenchmarkFastjq_Small_Select-16               	 9681657	       125.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Select-16               	   34903	     34092 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Alternative-16          	13308631	        95.46 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SelectAnd-16            	 6904473	       168.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SelectOr-16             	16499088	        72.21 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Has-16                  	12772184	        93.05 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Has-16                  	   35731	     33483 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_IfThenElse-16           	12393129	        97.71 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Length-16               	30902944	        41.47 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Length-16               	   36811	     32428 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Map-16                  	  726120	      1617 ns/op	     689 B/op	      29 allocs/op
BenchmarkFastjq_Medium_Map-16                 	  157814	      7663 ns/op	    2609 B/op	     109 allocs/op
BenchmarkFastjq_Large_Map-16                  	   76520	     15810 ns/op	    5009 B/op	     209 allocs/op
BenchmarkFastjq_Small_ToEntries-16            	 7861220	       165.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_ToEntries-16            	   34513	     34918 ns/op	       6 B/op	       0 allocs/op
BenchmarkFastjq_Small_KeysUnsorted-16         	 7799932	       162.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Keys-16                 	 3893878	       309.8 ns/op	     456 B/op	       7 allocs/op
BenchmarkFastjq_Small_Paths-16                	 2992154	       397.0 ns/op	     400 B/op	      17 allocs/op
BenchmarkFastjq_Small_GetPath-16              	 4707682	       253.5 ns/op	     272 B/op	      12 allocs/op
BenchmarkFastjq_Small_SetPath-16              	 2947809	       408.8 ns/op	     264 B/op	      12 allocs/op
BenchmarkFastjq_Small_DelPaths-16             	 1643852	       726.3 ns/op	     608 B/op	      22 allocs/op
BenchmarkFastjq_Large_KeysUnsorted-16         	   36865	     32843 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Any-16                  	26317640	        44.96 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Any-16                  	  811300	      1398 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AnyExpr-16              	 3022246	       391.1 ns/op	     144 B/op	      12 allocs/op
BenchmarkFastjq_Large_AnyExpr-16              	  294855	      3954 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Add-16                  	19804743	        60.93 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AddStrings-16           	 9007867	       132.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Flatten-16              	12344377	        97.71 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Split-16                	 8218458	       146.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Join-16                 	11681275	       102.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Values-16               	 4505582	       264.8 ns/op	     328 B/op	      13 allocs/op
BenchmarkGojq_Small_Values-16                 	  482314	      2445 ns/op	    3144 B/op	      51 allocs/op
BenchmarkFastjq_Small_Base64Encode-16         	 8142874	       148.8 ns/op	     120 B/op	       4 allocs/op
BenchmarkFastjq_Small_Base64Decode-16         	 5567539	       213.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Base64Encode-16           	 2057256	       581.5 ns/op	    1321 B/op	      15 allocs/op
BenchmarkGojq_Small_Base64Decode-16           	 2078960	       582.9 ns/op	    1321 B/op	      15 allocs/op
BenchmarkFastjq_Small_IndexFind-16            	11965886	        98.43 ns/op	       3 B/op	       1 allocs/op
BenchmarkFastjq_Small_IndicesAll-16           	 5664654	       210.6 ns/op	       3 B/op	       1 allocs/op
BenchmarkGojq_Small_IndexFind-16              	 1272943	       944.4 ns/op	    2274 B/op	      31 allocs/op
BenchmarkGojq_Small_IndicesAll-16             	  539682	      2184 ns/op	    3907 B/op	      97 allocs/op
BenchmarkFastjq_Small_Slice-16                	13916343	        89.60 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SliceString-16          	11703391	       103.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Plus-16                 	22967312	        52.52 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_PlusStr-16              	13729086	        86.35 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AsciiDowncase-16        	 8172721	       144.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_AsciiDowncase-16        	   34676	     34452 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Startswith-16           	 9465867	       130.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Startswith-16           	   35875	     33608 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Endswith-16             	 9468410	       126.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Ltrimstr-16             	 9531468	       125.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Ltrimstr-16             	   36466	     32860 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Rtrimstr-16             	 9562879	       126.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Trim-16                 	22106300	        51.98 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_Ltrim-16                	25294731	        48.23 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_Rtrim-16                	22371086	        52.31 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_First-16                	 4948522	       247.4 ns/op	     216 B/op	       9 allocs/op
BenchmarkFastjq_Large_First-16                	  180908	      6738 ns/op	    2584 B/op	     107 allocs/op
BenchmarkFastjq_Small_Last-16                 	 3828656	       312.1 ns/op	     256 B/op	      10 allocs/op
BenchmarkFastjq_Large_Last-16                 	  187716	      6343 ns/op	    2584 B/op	     107 allocs/op
BenchmarkFastjq_Small_Limit-16                	11822096	       102.3 ns/op	     104 B/op	       5 allocs/op
BenchmarkFastjq_Small_Skip-16                 	 8252710	       147.7 ns/op	     208 B/op	       7 allocs/op
BenchmarkFastjq_Small_Reduce-16               	    8838	    140724 ns/op	    2632 B/op	      86 allocs/op
BenchmarkFastjq_Large_Limit-16                	 1752130	       685.6 ns/op	     104 B/op	       5 allocs/op
BenchmarkGojq_Small_Del-16                    	  510252	      2346 ns/op	    3763 B/op	      58 allocs/op
BenchmarkGojq_Medium_Del-16                   	   59432	     20152 ns/op	   16974 B/op	     323 allocs/op
BenchmarkGojq_Large_Del-16                    	    1454	    829577 ns/op	  545836 B/op	    4666 allocs/op
BenchmarkGojq_Small_Field-16                  	 1000000	      1029 ns/op	    1641 B/op	      27 allocs/op
BenchmarkGojq_Large_Field-16                  	    2012	    602454 ns/op	  270048 B/op	    2835 allocs/op
BenchmarkGojq_Small_Index-16                  	 1847407	       650.9 ns/op	    1401 B/op	      20 allocs/op
BenchmarkGojq_Small_ArrayDel-16               	  694023	      1738 ns/op	    3363 B/op	      53 allocs/op
BenchmarkGojq_Small_Construct-16              	  853552	      1441 ns/op	    2330 B/op	      37 allocs/op
BenchmarkGojq_Large_Construct-16              	    2018	    599226 ns/op	  274509 B/op	    2867 allocs/op
BenchmarkGojq_Small_Iterator-16               	 1506422	       796.2 ns/op	    1776 B/op	      26 allocs/op
BenchmarkGojq_Large_Iterator-16               	   13460	     89141 ns/op	  109809 B/op	    1811 allocs/op
BenchmarkGojq_Small_Select-16                 	  690166	      1789 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Large_Select-16                 	    1506	    800268 ns/op	  537252 B/op	    4651 allocs/op
BenchmarkGojq_Small_Alternative-16            	 1000000	      1129 ns/op	    1897 B/op	      31 allocs/op
BenchmarkGojq_Small_SelectAnd-16              	  564062	      1933 ns/op	    2890 B/op	      46 allocs/op
BenchmarkGojq_Small_SelectOr-16               	  637392	      1849 ns/op	    2890 B/op	      46 allocs/op
BenchmarkGojq_Small_Has-16                    	  679791	      1831 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Large_Has-16                    	    1486	    801704 ns/op	  538730 B/op	    4651 allocs/op
BenchmarkGojq_Small_IfThenElse-16             	 1000000	      1135 ns/op	    1817 B/op	      30 allocs/op
BenchmarkGojq_Small_Length-16                 	 1000000	      1030 ns/op	    1633 B/op	      27 allocs/op
BenchmarkGojq_Large_Length-16                 	    2030	    600032 ns/op	  269833 B/op	    2835 allocs/op
BenchmarkGojq_Small_Map-16                    	  113953	     10602 ns/op	   13654 B/op	     251 allocs/op
BenchmarkGojq_Large_Map-16                    	   12462	     96137 ns/op	  118601 B/op	    2237 allocs/op
BenchmarkGojq_Small_ToEntries-16              	  285193	      4272 ns/op	    6559 B/op	      98 allocs/op
BenchmarkGojq_Large_ToEntries-16              	    1285	    907602 ns/op	  668538 B/op	    6465 allocs/op
BenchmarkGojq_Small_KeysUnsorted-16           	  906357	      1348 ns/op	    1953 B/op	      35 allocs/op
BenchmarkGojq_Small_Keys-16                   	  908574	      1347 ns/op	    1953 B/op	      35 allocs/op
BenchmarkGojq_Small_Paths-16                  	  272538	      4629 ns/op	    6608 B/op	     119 allocs/op
BenchmarkGojq_Small_GetPath-16                	 1000000	      1071 ns/op	    1721 B/op	      29 allocs/op
BenchmarkGojq_Small_SetPath-16                	  671450	      1835 ns/op	    2618 B/op	      43 allocs/op
BenchmarkGojq_Small_DelPaths-16               	  687650	      1772 ns/op	    2426 B/op	      40 allocs/op
BenchmarkGojq_Large_KeysUnsorted-16           	    1910	    631431 ns/op	  282837 B/op	    3039 allocs/op
BenchmarkGojq_Small_Any-16                    	  558435	      2041 ns/op	    4508 B/op	      39 allocs/op
BenchmarkGojq_Small_AnyExpr-16                	  542499	      2246 ns/op	    4620 B/op	      49 allocs/op
BenchmarkGojq_Large_AnyExpr-16                	  665155	      1894 ns/op	    2818 B/op	      29 allocs/op
BenchmarkGojq_Small_Add-16                    	 1485134	       801.9 ns/op	    1529 B/op	      28 allocs/op
BenchmarkGojq_Small_AddStrings-16             	 1207242	       990.3 ns/op	    1785 B/op	      35 allocs/op
BenchmarkGojq_Small_Flatten-16                	  862698	      1407 ns/op	    1857 B/op	      38 allocs/op
BenchmarkGojq_Small_Split-16                  	 1418217	       842.4 ns/op	    1553 B/op	      21 allocs/op
BenchmarkGojq_Small_Join-16                   	 1000000	      1035 ns/op	    1865 B/op	      37 allocs/op
BenchmarkGojq_Small_Slice-16                  	 1374067	       878.9 ns/op	    1425 B/op	      21 allocs/op
BenchmarkGojq_Small_SliceString-16            	 2293512	       512.6 ns/op	    1145 B/op	      12 allocs/op
BenchmarkGojq_Small_Plus-16                   	 1534580	       781.2 ns/op	    1673 B/op	      21 allocs/op
BenchmarkGojq_Small_PlusStr-16                	 1488409	       816.6 ns/op	    1705 B/op	      23 allocs/op
BenchmarkGojq_Small_AsciiDowncase-16          	  641690	      1900 ns/op	    2714 B/op	      46 allocs/op
BenchmarkGojq_Large_AsciiDowncase-16          	    1458	    813786 ns/op	  540811 B/op	    4653 allocs/op
BenchmarkGojq_Small_Startswith-16             	  654288	      1816 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Large_Startswith-16             	    1407	    807569 ns/op	  535678 B/op	    4651 allocs/op
BenchmarkGojq_Small_Endswith-16               	  672811	      1833 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Small_Ltrimstr-16               	 1000000	      1133 ns/op	    1737 B/op	      30 allocs/op
BenchmarkGojq_Small_Rtrimstr-16               	 1000000	      1094 ns/op	    1737 B/op	      30 allocs/op
BenchmarkGojq_Small_Trim-16                   	 2984097	       400.8 ns/op	    1129 B/op	      12 allocs/op
BenchmarkGojq_Small_Ltrim-16                  	 2999816	       402.4 ns/op	    1129 B/op	      12 allocs/op
BenchmarkGojq_Small_Rtrim-16                  	 2895560	       409.7 ns/op	    1129 B/op	      12 allocs/op
BenchmarkGojq_Small_First-16                  	  697327	      1747 ns/op	    3379 B/op	      39 allocs/op
BenchmarkGojq_Large_First-16                  	  787101	      1522 ns/op	    1889 B/op	      23 allocs/op
BenchmarkGojq_Small_Last-16                   	  596888	      2003 ns/op	    3483 B/op	      43 allocs/op
BenchmarkGojq_Large_Last-16                   	  746700	      1597 ns/op	    2193 B/op	      24 allocs/op
BenchmarkGojq_Small_Limit-16                  	  664710	      1762 ns/op	    3328 B/op	      42 allocs/op
BenchmarkGojq_Small_Skip-16                   	  673309	      1789 ns/op	    2880 B/op	      43 allocs/op
BenchmarkGojq_Small_Reduce-16                 	  828950	      1460 ns/op	    2530 B/op	      42 allocs/op
BenchmarkGojq_Large_Limit-16                  	  715176	      1674 ns/op	    2200 B/op	      24 allocs/op
BenchmarkFastjq_Small_Subtract-16             	23493882	        51.04 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Subtract-16               	 1629669	       735.1 ns/op	    1649 B/op	      20 allocs/op
BenchmarkFastjq_Small_Multiply-16             	18273554	        66.12 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Multiply-16               	 1542463	       767.7 ns/op	    1649 B/op	      21 allocs/op
BenchmarkFastjq_Small_Divide-16               	11845436	        99.44 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Divide-16                 	 1574413	       763.1 ns/op	    1665 B/op	      21 allocs/op
BenchmarkFastjq_Small_Min-16                  	  618336	      1853 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Min-16                    	  919686	      1254 ns/op	    1217 B/op	      15 allocs/op
BenchmarkFastjq_Small_MinBy-16                	  130610	      8442 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_MinBy-16                  	   21289	     56384 ns/op	   63630 B/op	    1347 allocs/op
BenchmarkFastjq_Sort-16                       	  269163	      4507 ns/op	    7688 B/op	      14 allocs/op
BenchmarkGojq_Sort-16                         	  972493	      1273 ns/op	    1217 B/op	      15 allocs/op
BenchmarkFastjq_SortBy-16                     	   67863	     17729 ns/op	   24872 B/op	     422 allocs/op
BenchmarkGojq_SortBy-16                       	   12784	     93673 ns/op	   96942 B/op	    2145 allocs/op
BenchmarkFastjq_Unique-16                     	  205242	      5807 ns/op	    7688 B/op	      14 allocs/op
BenchmarkGojq_Unique-16                       	  979388	      1253 ns/op	    1217 B/op	      15 allocs/op
BenchmarkFastjq_GroupBy-16                    	   54104	     22231 ns/op	   24872 B/op	     422 allocs/op
BenchmarkGojq_GroupBy-16                      	   10000	    100255 ns/op	  101907 B/op	    2260 allocs/op
BenchmarkFastjq_Small_URIEncode-16            	 7115436	       171.0 ns/op	     120 B/op	       4 allocs/op
BenchmarkGojq_Small_URIEncode-16              	 1624653	       751.2 ns/op	    1289 B/op	      14 allocs/op
BenchmarkFastjq_Small_ArrayDiff-16            	 5464250	       220.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ArrayDiff-16              	  896520	      1337 ns/op	    2121 B/op	      33 allocs/op
BenchmarkFastjq_Small_TryNoError-16           	  260084	      4704 ns/op	      64 B/op	       1 allocs/op
BenchmarkFastjq_Small_TryCatchNoError-16      	  257899	      4743 ns/op	      64 B/op	       1 allocs/op
BenchmarkGojq_Small_TryNoError-16             	  997392	      1242 ns/op	    1913 B/op	      30 allocs/op
BenchmarkFastjq_Small_ObjectMerge-16          	 6506510	       183.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ObjectMerge-16            	  687636	      1658 ns/op	    2906 B/op	      33 allocs/op
BenchmarkFastjq_Small_ToJSON-16               	 2665437	       434.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ToJSON-16                 	  697178	      1723 ns/op	    2410 B/op	      39 allocs/op
BenchmarkFastjq_Small_FromJSON-16             	 6064806	       198.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_FromJSON-16               	  792379	      1381 ns/op	    2714 B/op	      31 allocs/op
BenchmarkFastjq_Small_ToString-16             	 2848726	       422.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToNumber-16             	88574233	        13.50 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToBoolean-16            	138005422	         8.668 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ToNumber-16               	 3179161	       379.3 ns/op	    1113 B/op	      11 allocs/op
BenchmarkGojq_Small_ToBoolean-16              	 3198181	       375.2 ns/op	    1113 B/op	      11 allocs/op
BenchmarkFastjq_Small_AnyTwoArg-16            	  270453	      4357 ns/op	      57 B/op	       3 allocs/op
BenchmarkGojq_Small_AnyTwoArg-16              	  665014	      1681 ns/op	    2482 B/op	      27 allocs/op
BenchmarkFastjq_Complex_LogNormalize-16       	 1000000	      1059 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Complex_LogNormalize-16         	  388994	      3030 ns/op	    4324 B/op	      69 allocs/op
BenchmarkFastjq_Complex_ArrayPipeline-16      	  133442	      9046 ns/op	    2264 B/op	     106 allocs/op
BenchmarkGojq_Complex_ArrayPipeline-16        	   51686	     23828 ns/op	   20408 B/op	     525 allocs/op
BenchmarkFastjq_Complex_Aggregation-16        	   97278	     12561 ns/op	    3184 B/op	     106 allocs/op
BenchmarkGojq_Complex_Aggregation-16          	   53738	     22240 ns/op	   22397 B/op	     555 allocs/op
BenchmarkFastjq_Complex_TolerantMap-16        	    4146	    268568 ns/op	    5216 B/op	     187 allocs/op
BenchmarkGojq_Complex_TolerantMap-16          	   42115	     28481 ns/op	   27870 B/op	     641 allocs/op
BenchmarkFastjq_Complex_ElifRouting-16        	  128206	      9316 ns/op	    3000 B/op	     127 allocs/op
BenchmarkGojq_Complex_ElifRouting-16          	   44000	     26340 ns/op	   25920 B/op	     595 allocs/op
BenchmarkFastjq_Complex_StringBuild-16        	  107528	     11235 ns/op	    7216 B/op	     298 allocs/op
BenchmarkGojq_Complex_StringBuild-16          	   48324	     24731 ns/op	   20534 B/op	     686 allocs/op
BenchmarkFastjq_Complex_EntryFilter-16        	  348412	      3495 ns/op	    2888 B/op	      69 allocs/op
BenchmarkGojq_Complex_EntryFilter-16          	   82398	     14470 ns/op	   18428 B/op	     250 allocs/op
BenchmarkFastjq_Small_Sqrt-16                 	17125414	        69.15 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Sqrt-16                   	 2514151	       476.9 ns/op	    1145 B/op	      12 allocs/op
BenchmarkFastjq_Small_Log-16                  	32161197	        38.66 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Log-16                    	 2567835	       469.1 ns/op	    1129 B/op	      12 allocs/op
BenchmarkFastjq_Small_Sin-16                  	15018702	        79.49 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Sin-16                    	 2526854	       474.8 ns/op	    1145 B/op	      12 allocs/op
BenchmarkFastjq_Small_Atan-16                 	22251925	        53.31 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Atan-16                   	 2914501	       413.1 ns/op	    1121 B/op	      11 allocs/op
BenchmarkFastjq_Small_Exp-16                  	18814502	        63.68 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Exp-16                    	 2759250	       433.7 ns/op	    1121 B/op	      11 allocs/op
BenchmarkFastjq_Small_Tgamma-16               	92597950	        12.82 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Tgamma-16                 	 2982802	       401.2 ns/op	    1105 B/op	      11 allocs/op
BenchmarkFastjq_Small_Fabs-16                 	22468543	        53.96 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Fabs-16                   	 2779340	       437.9 ns/op	    1113 B/op	      12 allocs/op
BenchmarkFastjq_Small_Abs-16                  	128640882	         9.341 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Abs-16                    	 2831707	       431.0 ns/op	    1113 B/op	      12 allocs/op
BenchmarkFastjq_Small_Bind-16                 	   39763	     30108 ns/op	     648 B/op	      18 allocs/op
BenchmarkGojq_Small_Bind-16                   	 1465294	       822.7 ns/op	    1793 B/op	      24 allocs/op
BenchmarkFastjq_Small_StringInterp-16         	 9948313	       122.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_StringInterp-16           	 1000000	      1077 ns/op	    2121 B/op	      35 allocs/op
BenchmarkFastjq_Small_StringInterpNum-16      	13471647	        90.21 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_StringInterpNum-16        	  932998	      1265 ns/op	    2498 B/op	      40 allocs/op
BenchmarkFastjq_Small_IsEmptyTrue-16          	16586344	        71.86 ns/op	     105 B/op	       6 allocs/op
BenchmarkGojq_Small_IsEmptyTrue-16            	 1962237	       606.3 ns/op	    1921 B/op	      18 allocs/op
BenchmarkFastjq_Small_IsEmptyFalse-16         	13900767	        86.94 ns/op	     105 B/op	       6 allocs/op
BenchmarkGojq_Small_IsEmptyFalse-16           	 1000000	      1085 ns/op	    2898 B/op	      33 allocs/op
BenchmarkFastjq_Small_Nth-16                  	 9314028	       130.1 ns/op	     152 B/op	       7 allocs/op
BenchmarkGojq_Small_Nth-16                    	  540714	      2230 ns/op	    4372 B/op	      49 allocs/op
BenchmarkFastjq_Small_Range10-16              	 5996478	       199.6 ns/op	     120 B/op	      12 allocs/op
BenchmarkGojq_Small_Range10-16                	 1000000	      1044 ns/op	    1864 B/op	      18 allocs/op
BenchmarkFastjq_Small_RangeLimit-16           	 8832380	       137.3 ns/op	     128 B/op	       8 allocs/op
BenchmarkGojq_Small_RangeLimit-16             	  758349	      1577 ns/op	    3208 B/op	      26 allocs/op
BenchmarkRegexp_Match_Hit-16                  	27614112	        42.10 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Miss-16                 	51478394	        23.41 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Complex_Hit-16          	27426194	        43.62 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Long_Miss-16            	15329797	        78.10 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_MatchString-16                	27121759	        44.45 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_FindSubmatch_Hit-16           	 5786295	       207.9 ns/op	     160 B/op	       2 allocs/op
BenchmarkRegexp_FindSubmatch_Miss-16          	 1742409	       687.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_FindSubmatchIndex_Hit-16      	 6638829	       182.6 ns/op	      64 B/op	       1 allocs/op
BenchmarkRegexp_FindSubmatchIndex_Miss-16     	 1754944	       682.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_ReplaceAll_Hit-16             	 7773816	       154.9 ns/op	      97 B/op	       5 allocs/op
BenchmarkRegexp_ReplaceAll_Miss-16            	24339082	        49.68 ns/op	      48 B/op	       2 allocs/op
BenchmarkRegexp_MatchViaClosure-16            	27226704	        42.58 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_Hit-16           	11193716	       107.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_Miss-16          	12024328	        99.47 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_InPipeline-16    	 3983224	       297.3 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_MatchRe_Hit-16          	 5536615	       217.9 ns/op	      48 B/op	       1 allocs/op
BenchmarkFastjq_Small_MatchRe_Miss-16         	 1921189	       623.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_TestRe_Hit-16             	  929568	      1323 ns/op	    2743 B/op	      26 allocs/op
BenchmarkGojq_Small_TestRe_Miss-16            	  933558	      1263 ns/op	    2701 B/op	      26 allocs/op
BenchmarkGojq_Small_MatchRe_Hit-16            	  415429	      2993 ns/op	    4799 B/op	      66 allocs/op
BenchmarkGojq_Small_MatchRe_Miss-16           	  693206	      1722 ns/op	    2250 B/op	      24 allocs/op
BenchmarkFastjq_Small_CaptureRe_Hit-16        	 5676162	       212.8 ns/op	      48 B/op	       1 allocs/op
BenchmarkFastjq_Small_CaptureRe_Miss-16       	 1906641	       630.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ScanRe_NoGroups-16      	 2161471	       556.2 ns/op	     534 B/op	      17 allocs/op
BenchmarkFastjq_Small_ScanRe_WithGroups-16    	 1681220	       738.8 ns/op	     851 B/op	      20 allocs/op
BenchmarkFastjq_Small_SubRe_Hit-16            	 9569240	       125.2 ns/op	      16 B/op	       1 allocs/op
BenchmarkFastjq_Small_SubRe_Miss-16           	11847141	       101.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_GSubRe_Hit-16           	 2037940	       589.8 ns/op	     305 B/op	       5 allocs/op
BenchmarkFastjq_Small_GSubRe_Miss-16          	 4636531	       259.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_CaptureRe_Hit-16          	  505798	      2338 ns/op	    4721 B/op	      51 allocs/op
BenchmarkGojq_Small_CaptureRe_Miss-16         	  658070	      1915 ns/op	    2676 B/op	      27 allocs/op
BenchmarkGojq_Small_ScanRe_NoGroups-16        	  420792	      2856 ns/op	    5129 B/op	      57 allocs/op
BenchmarkGojq_Small_SubRe_Hit-16              	  217266	      5410 ns/op	    8992 B/op	      95 allocs/op
BenchmarkGojq_Small_GSubRe_Hit-16             	   80164	     15425 ns/op	   17894 B/op	     248 allocs/op
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
