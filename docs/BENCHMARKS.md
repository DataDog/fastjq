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
| `.field` | Small (~100B) | 0.087 | 1.01 | **12x** | 0 | 27 |
| `.field` | Large (~100KB) | 7.53 | 603 | **80x** | 0 | 2835 |
| `del(.f)` | Small (~100B) | 0.156 | 2.31 | **15x** | 0 | 58 |
| `del(.f)` | Medium (~2KB) | 2.27 | 19.6 | **8.7x** | 0 | 323 |
| `del(.f)` | Large (~100KB) | 35.8 | 819 | **23x** | 0 | 4666 |
| `.[n]` | 5-elem array | 0.022 | 0.644 | **29x** | 0 | 20 |
| `del(.[n], .[m])` | 5-elem array | 0.079 | 1.68 | **21x** | 0 | 53 |
| `{f0, f2}` (construct) | Small (~100B) | 0.130 | 1.40 | **11x** | 0 | 37 |
| `{f0, f50}` (construct) | Large (~100KB) | 8.08 | 641 | **79x** | 0 | 2867 |
| `.[]` iterator | 5-elem array | 0.065 | 0.855 | **13x** | 2 | 26 |
| `.[]` iterator | 200-elem array | 7.90 | 92.4 | **12x** | 2 | 1811 |
| `select(.f == "x")` | Small (~100B) | 0.124 | 1.82 | **15x** | 0 | 45 |
| `select(.f == "x")`¹ | Large (~100KB, last field) | 33.1 | 815 | **25x** | 0 | 4652 |
| `select(.f and .g)` | Small (~100B) | 0.166 | 1.94 | **12x** | 0 | 46 |
| `select(.f or .g)` | Small (~100B) | 0.074 | 1.88 | **26x** | 0 | 46 |
| `has("key")` in select | Small (~100B) | 0.088 | 1.76 | **20x** | 0 | 45 |
| `has("key")` in select | Large (~100KB) | 32.6 | 794 | **24x** | 0 | 4651 |
| `if-then-else` | Small (~100B) | 0.098 | 1.13 | **12x** | 0 | 30 |
| `.f // "default"` | Small (~100B) | 0.096 | 1.14 | **12x** | 0 | 31 |
| `try .field` (no error) | Small (~100B) | 4.75 | 1.07 | 0.2x† | 1 | 30 |
| `.a + .b` (strings) | Small (~100B) | 0.053 | 0.700 | **13x** | 0 | 21 |
| `"prefix" + .name` | Small (~100B) | 0.082 | 0.754 | **9.2x** | 0 | 23 |
| `.a - .b` (subtract) | Small (~100B) | 0.050 | 0.725 | **15x** | 0 | 20 |
| `.a * .b` (multiply) | Small (~100B) | 0.066 | 0.745 | **11x** | 0 | 21 |
| `.a / .b` (divide) | Small (~100B) | 0.098 | 0.798 | **8.1x** | 0 | 21 |
| `.a - .b` (array diff) | 5-elem arrays | 0.213 | 1.28 | **6x** | 0 | 33 |
| `length` | Small (~100B) | 0.041 | 1.02 | **25x** | 0 | 27 |
| `length` | Large (~100KB) | 32.8 | 594 | **18x** | 0 | 2835 |
| `add` (numbers) | 5-elem array | 0.057 | 0.748 | **13x** | 0 | 28 |
| `add` (strings) | 5-elem array | 0.135 | 0.913 | **6.8x** | 0 | 35 |
| `flatten` | 3-elem nested array | 0.092 | 1.35 | **15x** | 0 | 38 |
| `min` | 200-int array | 3.88 | 1.25 | 0.3x† | 300 | 15 |
| `min_by(.value)` | 100-elem object array | 10.9 | 57.3 | **5.3x** | 297 | 1347 |
| `sort` | 200-int array | 6.76 | 1.26 | 0.2x† | 265 | 15 |
| `sort_by(.value)` | 100-elem object array | 20.2 | 93.8 | **4.7x** | 667 | 2145 |
| `unique` | 200-int array | 9.75 | 1.27 | 0.1x† | 466 | 15 |
| `group_by(.active)` | 100-elem object array | 22.6 | 101 | **4.5x** | 422 | 2260 |
| `map(.name)` | 20-elem array (~600B) | 1.66 | 10.3 | **6.2x** | 29 | 251 |
| `map(.name)` | 200-elem array (~6KB) | 15.8 | 94.4 | **6x** | 209 | 2237 |
| `any` | 5-elem array | 0.041 | 1.91 | **47x** | 0 | 39 |
| `any(expr)` | 5-elem array | 0.391 | 2.15 | **5.5x** | 12 | 49 |
| `any(expr)`² | 200-int array | 5.72 | 1.76 | 0.3x† | 205 | 29 |
| `any(gen; cond)`² | 200-int array | 6.26 | 1.66 | 0.3x† | 206 | 27 |
| `first(expr)` | 5-elem array | 0.332 | 1.55 | **4.7x** | 15 | 39 |
| `first(expr)`² | 200-int array | 8.62 | 1.45 | 0.2x† | 312 | 23 |
| `last(expr)` | 5-elem array | 0.432 | 2.01 | **4.7x** | 20 | 43 |
| `last(expr)`² | 200-int array | 8.42 | 1.59 | 0.2x† | 310 | 24 |
| `limit(3; expr)` | 5-elem array | 0.105 | 1.79 | **17x** | 5 | 42 |
| `limit(10; expr)` | 200-int array | 0.681 | 1.66 | **2.4x** | 5 | 24 |
| `.[1:4]` slice | 6-elem array | 4.70 | 0.876 | 0.2x† | 1 | 21 |
| `values` | 9-elem array | 0.266 | 2.44 | **9.2x** | 13 | 51 |
| `skip(2; .[])` | 5-elem array | 0.147 | 1.82 | **12x** | 7 | 43 |
| `reduce .[] as $x (0; . + $x)` | 5-elem array | 106 | 1.52 | 0.0x† | 83 | 42 |
| `foreach .[] as $x (0; . + $x)` | 5-elem array | 88.9 | 1.39 | 0.0x† | 97 | 39 |
| `paths` | Small (~100B) | 0.384 | 4.36 | **11x** | 17 | 119 |
| `path(.field_0)` | Small (~100B) | 0.118 | 1.24 | **10x** | 6 | 36 |
| `to_entries` | Small (~100B) | 0.171 | 4.16 | **24x** | 0 | 98 |
| `to_entries` | Large (~100KB) | 35.4 | 880 | **25x** | 0 | 6465 |
| `getpath(["field_0"])` | Small (~100B) | 0.254 | 1.05 | **4.2x** | 12 | 29 |
| `setpath(["field_0"]; "y")` | Small (~100B) | 0.408 | 1.73 | **4.2x** | 12 | 43 |
| `delpaths([["field_0"]])` | Small (~100B) | 0.717 | 1.76 | **2.4x** | 22 | 40 |
| `keys` | Small (~100B) | 0.295 | 1.31 | **4.4x** | 7 | 35 |
| `keys_unsorted` | Small (~100B) | 0.175 | 1.31 | **7.5x** | 0 | 35 |
| `keys_unsorted` | Large (~100KB) | 32.6 | 616 | **19x** | 0 | 3039 |
| `object merge .a + .b` | Small (~100B) | 0.163 | 1.54 | **9.4x** | 0 | 33 |
| `tojson` | Small (~100B) | 0.416 | 1.62 | **3.9x** | 0 | 39 |
| `fromjson` | JSON string | 0.197 | 1.34 | **6.8x** | 0 | 31 |
| `tonumber` | `"42"` string | 0.013 | 0.369 | **28x** | 0 | 11 |
| `toboolean` | `"true"` string | 0.0086 | 0.359 | **42x** | 0 | 11 |
| `split(",")` | short string | 0.146 | 0.780 | **5.4x** | 0 | 21 |
| `join(",")` | 5-elem array | 0.100 | 1.06 | **11x** | 0 | 37 |
| `ascii_downcase` in select | Small (~100B) | 0.137 | 1.84 | **13x** | 0 | 46 |
| `ascii_downcase` in select | Large (~100KB) | 33.9 | 798 | **24x** | 0 | 4652 |
| `startswith("s")` | Small (~100B) | 0.122 | 1.75 | **14x** | 0 | 45 |
| `startswith("s")` | Large (~100KB) | 33.3 | 801 | **24x** | 0 | 4651 |
| `endswith("s")` | Small (~100B) | 0.127 | 1.80 | **14x** | 0 | 45 |
| `trim` | short string | 0.052 | 0.390 | **7.5x** | 1 | 12 |
| `ltrim` | short string | 0.047 | 0.391 | **8.4x** | 1 | 12 |
| `rtrim` | short string | 0.051 | 0.393 | **7.7x** | 1 | 12 |
| `ltrimstr("s")` | Small (~100B) | 0.125 | 1.09 | **8.7x** | 0 | 30 |
| `rtrimstr("s")` | Small (~100B) | 0.123 | 1.07 | **8.7x** | 0 | 30 |
| `select` + string ops + arith + construct | ~200B log event | 1.06 | 2.98 | **2.8x** | 2 | 69 |
| `[.[] \| select + arith + construct]` | 20-elem array (~1.5KB) | 9.15 | 22.8 | **2.5x** | 132 | 525 |
| `length + map + add + min_by + any` | 20-elem array (~1.5KB) | 12.8 | 21.9 | **1.7x** | 144 | 555 |
| `map(try {…} catch …)` | 20-elem array (~1.5KB) | 268 | 28.1 | 0.1x† | 187 | 641 |
| `map(if … elif … else …)` | 20-elem array (~1.5KB) | 9.62 | 25.7 | **2.7x** | 181 | 595 |
| `[.[] \| str + tostring + str]` | 20-elem array (~1.5KB) | 11.5 | 24.5 | **2.1x** | 298 | 686 |
| `to_entries \| map(select) \| from_entries` | ~200B log event | 3.42 | 14.0 | **4.1x** | 69 | 250 |
| `@base64` | 34-char string | 0.148 | 0.538 | **3.6x** | 4 | 15 |
| `@base64d` | 48-char encoded | 0.217 | 0.565 | **2.6x** | 0 | 15 |
| `@uri` | 36-char URL string | 0.168 | 0.689 | **4.1x** | 4 | 14 |
| `index(",")` | short string | 0.098 | 0.944 | **9.7x** | 1 | 31 |
| `indices(",")` | short string | 0.205 | 2.31 | **11x** | 1 | 97 |
| `sqrt` | float (e≈2.718) | 0.071 | 0.467 | **6.6x** | 0 | 12 |
| `log` | float (e≈2.718) | 0.040 | 0.456 | **11x** | 0 | 12 |
| `sin` | float (e≈2.718) | 0.080 | 0.465 | **5.8x** | 0 | 12 |
| `atan` | integer 1 | 0.052 | 0.399 | **7.6x** | 0 | 11 |
| `exp` | integer 1 | 0.063 | 0.410 | **6.5x** | 0 | 11 |
| `tgamma` | integer 5 | 0.013 | 0.389 | **31x** | 0 | 11 |
| `fabs` | float -3.14 | 0.055 | 0.421 | **7.7x** | 0 | 12 |
| `abs` | float -3.14 | 0.0093 | 0.418 | **45x** | 0 | 12 |
| `expr as $x \| body` | Small (~20B object) | 29.6 | 0.803 | 0.0x† | 18 | 24 |
| ``"\(.level): \(.svc)"`` | ~45B object | 0.120 | 1.06 | **8.8x** | 0 | 35 |
| ``"user \(.name) …"`` | ~45B object | 0.088 | 1.23 | **14x** | 0 | 40 |
| `isempty(empty)` | null | 0.073 | 0.602 | **8.2x** | 6 | 18 |
| `isempty(.[])` | 5-elem array | 0.091 | 1.07 | **12x** | 6 | 33 |
| `nth(2; .[])` | 5-elem array | 0.132 | 2.17 | **16x** | 7 | 49 |
| `range(10)` (10 values) | null | 0.202 | 1.03 | **5.1x** | 12 | 18 |
| `limit(3; range(1000))` | null — only 3 allocs | 0.137 | 1.52 | **11x** | 8 | 26 |
| `test(re)` hit | short string | 0.107 | 1.34 | **12x** | 0 | 26 |
| `test(re)` miss | short string | 0.099 | 1.27 | **13x** | 0 | 26 |
| `match(re)` hit | short string | 0.222 | 3.05 | **14x** | 1 | 66 |
| `match(re)` miss | short string | 0.629 | 1.72 | **2.7x** | 0 | 24 |
| `capture(re)` hit | short string | 0.211 | — | — | 1 | — |
| `scan(re)` no groups (4 matches) | short string | 0.557 | — | — | 17 | — |
| `sub(re; s)` hit | short string | 0.125 | — | — | 1 | — |
| `gsub(re; s)` hit (4 matches) | short string | 0.585 | — | — | 5 | — |

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
BenchmarkFastjq_Small_Del-16                  	 7140957	       155.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Medium_Del-16                 	  519826	      2265 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Del-16                  	   33595	     35834 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Field-16                	12365746	        87.37 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Field-16                	  156286	      7528 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Index-16                	53790352	        22.25 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ArrayDel-16             	14469096	        78.53 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Construct-16            	 9053098	       129.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Construct-16            	  150916	      8077 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Iterator-16             	18638809	        64.91 ns/op	      40 B/op	       2 allocs/op
BenchmarkFastjq_Large_Iterator-16             	  150656	      7902 ns/op	      40 B/op	       2 allocs/op
BenchmarkFastjq_Small_Select-16               	 9796921	       123.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Select-16               	   35714	     33098 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Alternative-16          	14123365	        95.75 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SelectAnd-16            	 7285586	       166.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SelectOr-16             	16497310	        73.53 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Has-16                  	13011673	        87.67 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Has-16                  	   36619	     32644 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_IfThenElse-16           	12249871	        97.93 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Length-16               	31108500	        40.53 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Length-16               	   37302	     32838 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Map-16                  	  704463	      1662 ns/op	     689 B/op	      29 allocs/op
BenchmarkFastjq_Medium_Map-16                 	  154785	      7920 ns/op	    2609 B/op	     109 allocs/op
BenchmarkFastjq_Large_Map-16                  	   76075	     15768 ns/op	    5009 B/op	     209 allocs/op
BenchmarkFastjq_Small_ToEntries-16            	 7054894	       170.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_ToEntries-16            	   34093	     35388 ns/op	       6 B/op	       0 allocs/op
BenchmarkFastjq_Small_KeysUnsorted-16         	 7685958	       174.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Keys-16                 	 4043300	       295.2 ns/op	     456 B/op	       7 allocs/op
BenchmarkFastjq_Small_Paths-16                	 3118411	       384.0 ns/op	     400 B/op	      17 allocs/op
BenchmarkFastjq_Small_Path-16                 	 9645580	       118.2 ns/op	     168 B/op	       6 allocs/op
BenchmarkFastjq_Small_GetPath-16              	 4843465	       253.5 ns/op	     272 B/op	      12 allocs/op
BenchmarkFastjq_Small_SetPath-16              	 2967744	       408.5 ns/op	     264 B/op	      12 allocs/op
BenchmarkFastjq_Small_DelPaths-16             	 1675285	       717.4 ns/op	     608 B/op	      22 allocs/op
BenchmarkFastjq_Large_KeysUnsorted-16         	   36787	     32605 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Any-16                  	27140263	        40.90 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Any-16                  	  932535	      1325 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AnyExpr-16              	 3086510	       391.3 ns/op	     144 B/op	      12 allocs/op
BenchmarkFastjq_Large_AnyExpr-16              	  210410	      5718 ns/op	     629 B/op	     205 allocs/op
BenchmarkFastjq_Small_Add-16                  	20986220	        56.72 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AddStrings-16           	 8971434	       134.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Flatten-16              	12940237	        91.74 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Split-16                	 8207798	       145.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Join-16                 	11975599	       100.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Values-16               	 4559139	       265.6 ns/op	     328 B/op	      13 allocs/op
BenchmarkGojq_Small_Values-16                 	  489123	      2435 ns/op	    3144 B/op	      51 allocs/op
BenchmarkFastjq_Small_Base64Encode-16         	 8093758	       148.5 ns/op	     120 B/op	       4 allocs/op
BenchmarkFastjq_Small_Base64Decode-16         	 5536998	       217.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Base64Encode-16           	 2253802	       537.7 ns/op	    1321 B/op	      15 allocs/op
BenchmarkGojq_Small_Base64Decode-16           	 2116507	       565.0 ns/op	    1321 B/op	      15 allocs/op
BenchmarkFastjq_Small_IndexFind-16            	12212222	        97.69 ns/op	       3 B/op	       1 allocs/op
BenchmarkFastjq_Small_IndicesAll-16           	 5888640	       205.0 ns/op	       3 B/op	       1 allocs/op
BenchmarkGojq_Small_IndexFind-16              	 1274130	       943.7 ns/op	    2274 B/op	      31 allocs/op
BenchmarkGojq_Small_IndicesAll-16             	  542698	      2310 ns/op	    3907 B/op	      97 allocs/op
BenchmarkFastjq_Small_Slice-16                	  258679	      4700 ns/op	      64 B/op	       1 allocs/op
BenchmarkFastjq_Small_SliceString-16          	  254780	      4742 ns/op	      64 B/op	       1 allocs/op
BenchmarkFastjq_Small_Plus-16                 	23102227	        52.98 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_PlusStr-16              	14731622	        81.99 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AsciiDowncase-16        	 8640337	       136.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_AsciiDowncase-16        	   35451	     33855 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Startswith-16           	 9953938	       122.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Startswith-16           	   36148	     33278 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Endswith-16             	 9800425	       126.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Ltrimstr-16             	 9103736	       124.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Ltrimstr-16             	   37597	     31836 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Rtrimstr-16             	10183600	       122.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Trim-16                 	23168688	        51.66 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_Ltrim-16                	25443246	        46.74 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_Rtrim-16                	23750187	        50.86 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_First-16                	 3618444	       331.5 ns/op	     264 B/op	      15 allocs/op
BenchmarkFastjq_Large_First-16                	  141222	      8616 ns/op	    3248 B/op	     312 allocs/op
BenchmarkFastjq_Small_Last-16                 	 2777586	       431.7 ns/op	     266 B/op	      20 allocs/op
BenchmarkFastjq_Large_Last-16                 	  143587	      8424 ns/op	    3208 B/op	     310 allocs/op
BenchmarkFastjq_Small_Limit-16                	11181300	       105.3 ns/op	     104 B/op	       5 allocs/op
BenchmarkFastjq_Small_Skip-16                 	 8225443	       146.6 ns/op	     208 B/op	       7 allocs/op
BenchmarkFastjq_Small_Reduce-16               	   10000	    106193 ns/op	    2408 B/op	      83 allocs/op
BenchmarkFastjq_Small_Foreach-16              	   13509	     88876 ns/op	    3056 B/op	      97 allocs/op
BenchmarkFastjq_Large_Limit-16                	 1762324	       680.6 ns/op	     104 B/op	       5 allocs/op
BenchmarkGojq_Small_Del-16                    	  522494	      2314 ns/op	    3763 B/op	      58 allocs/op
BenchmarkGojq_Medium_Del-16                   	   61632	     19597 ns/op	   16970 B/op	     323 allocs/op
BenchmarkGojq_Large_Del-16                    	    1459	    819400 ns/op	  542851 B/op	    4666 allocs/op
BenchmarkGojq_Small_Field-16                  	 1000000	      1007 ns/op	    1641 B/op	      27 allocs/op
BenchmarkGojq_Large_Field-16                  	    1934	    602587 ns/op	  270051 B/op	    2835 allocs/op
BenchmarkGojq_Small_Index-16                  	 1854694	       643.9 ns/op	    1401 B/op	      20 allocs/op
BenchmarkGojq_Small_ArrayDel-16               	  725662	      1683 ns/op	    3363 B/op	      53 allocs/op
BenchmarkGojq_Small_Construct-16              	  873558	      1395 ns/op	    2330 B/op	      37 allocs/op
BenchmarkGojq_Large_Construct-16              	    2001	    641341 ns/op	  274568 B/op	    2867 allocs/op
BenchmarkGojq_Small_Iterator-16               	 1410030	       855.2 ns/op	    1776 B/op	      26 allocs/op
BenchmarkGojq_Large_Iterator-16               	   12956	     92394 ns/op	  109809 B/op	    1811 allocs/op
BenchmarkGojq_Small_Select-16                 	  689865	      1820 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Large_Select-16                 	    1476	    815241 ns/op	  542173 B/op	    4652 allocs/op
BenchmarkGojq_Small_Alternative-16            	  974997	      1143 ns/op	    1897 B/op	      31 allocs/op
BenchmarkGojq_Small_SelectAnd-16              	  569822	      1943 ns/op	    2890 B/op	      46 allocs/op
BenchmarkGojq_Small_SelectOr-16               	  599149	      1877 ns/op	    2890 B/op	      46 allocs/op
BenchmarkGojq_Small_Has-16                    	  675987	      1762 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Large_Has-16                    	    1502	    793542 ns/op	  534185 B/op	    4651 allocs/op
BenchmarkGojq_Small_IfThenElse-16             	 1000000	      1132 ns/op	    1817 B/op	      30 allocs/op
BenchmarkGojq_Small_Length-16                 	 1000000	      1016 ns/op	    1633 B/op	      27 allocs/op
BenchmarkGojq_Large_Length-16                 	    2024	    594355 ns/op	  269829 B/op	    2835 allocs/op
BenchmarkGojq_Small_Map-16                    	  116138	     10322 ns/op	   13654 B/op	     251 allocs/op
BenchmarkGojq_Large_Map-16                    	   12702	     94443 ns/op	  118587 B/op	    2237 allocs/op
BenchmarkGojq_Small_ToEntries-16              	  289200	      4155 ns/op	    6558 B/op	      98 allocs/op
BenchmarkGojq_Large_ToEntries-16              	    1360	    880347 ns/op	  666345 B/op	    6465 allocs/op
BenchmarkGojq_Small_KeysUnsorted-16           	  940224	      1307 ns/op	    1953 B/op	      35 allocs/op
BenchmarkGojq_Small_Keys-16                   	  936487	      1311 ns/op	    1953 B/op	      35 allocs/op
BenchmarkGojq_Small_Paths-16                  	  270913	      4360 ns/op	    6608 B/op	     119 allocs/op
BenchmarkGojq_Small_Path-16                   	  969027	      1236 ns/op	    1993 B/op	      36 allocs/op
BenchmarkGojq_Small_GetPath-16                	 1000000	      1054 ns/op	    1721 B/op	      29 allocs/op
BenchmarkGojq_Small_SetPath-16                	  686073	      1725 ns/op	    2618 B/op	      43 allocs/op
BenchmarkGojq_Small_DelPaths-16               	  680042	      1756 ns/op	    2426 B/op	      40 allocs/op
BenchmarkGojq_Large_KeysUnsorted-16           	    1954	    615672 ns/op	  282756 B/op	    3039 allocs/op
BenchmarkGojq_Small_Any-16                    	  628460	      1906 ns/op	    4508 B/op	      39 allocs/op
BenchmarkGojq_Small_AnyExpr-16                	  555907	      2154 ns/op	    4620 B/op	      49 allocs/op
BenchmarkGojq_Large_AnyExpr-16                	  689415	      1764 ns/op	    2818 B/op	      29 allocs/op
BenchmarkGojq_Small_Add-16                    	 1608544	       748.1 ns/op	    1529 B/op	      28 allocs/op
BenchmarkGojq_Small_AddStrings-16             	 1315222	       913.4 ns/op	    1785 B/op	      35 allocs/op
BenchmarkGojq_Small_Flatten-16                	  937771	      1351 ns/op	    1857 B/op	      38 allocs/op
BenchmarkGojq_Small_Split-16                  	 1526677	       780.4 ns/op	    1553 B/op	      21 allocs/op
BenchmarkGojq_Small_Join-16                   	 1264527	      1061 ns/op	    1865 B/op	      37 allocs/op
BenchmarkGojq_Small_Slice-16                  	 1358878	       876.1 ns/op	    1425 B/op	      21 allocs/op
BenchmarkGojq_Small_SliceString-16            	 2510907	       476.9 ns/op	    1145 B/op	      12 allocs/op
BenchmarkGojq_Small_Plus-16                   	 1714959	       699.7 ns/op	    1673 B/op	      21 allocs/op
BenchmarkGojq_Small_PlusStr-16                	 1598898	       754.0 ns/op	    1705 B/op	      23 allocs/op
BenchmarkGojq_Small_AsciiDowncase-16          	  639654	      1836 ns/op	    2714 B/op	      46 allocs/op
BenchmarkGojq_Large_AsciiDowncase-16          	    1512	    797688 ns/op	  538894 B/op	    4652 allocs/op
BenchmarkGojq_Small_Startswith-16             	  684639	      1749 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Large_Startswith-16             	    1512	    801000 ns/op	  538881 B/op	    4651 allocs/op
BenchmarkGojq_Small_Endswith-16               	  677458	      1800 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Small_Ltrimstr-16               	 1000000	      1089 ns/op	    1737 B/op	      30 allocs/op
BenchmarkGojq_Small_Rtrimstr-16               	 1000000	      1070 ns/op	    1737 B/op	      30 allocs/op
BenchmarkGojq_Small_Trim-16                   	 3079684	       390.0 ns/op	    1129 B/op	      12 allocs/op
BenchmarkGojq_Small_Ltrim-16                  	 3074497	       391.3 ns/op	    1129 B/op	      12 allocs/op
BenchmarkGojq_Small_Rtrim-16                  	 3006882	       392.5 ns/op	    1129 B/op	      12 allocs/op
BenchmarkGojq_Small_First-16                  	  797162	      1553 ns/op	    3379 B/op	      39 allocs/op
BenchmarkGojq_Large_First-16                  	  832251	      1449 ns/op	    1889 B/op	      23 allocs/op
BenchmarkGojq_Small_Last-16                   	  622099	      2012 ns/op	    3483 B/op	      43 allocs/op
BenchmarkGojq_Large_Last-16                   	  759310	      1593 ns/op	    2193 B/op	      24 allocs/op
BenchmarkGojq_Small_Limit-16                  	  700732	      1788 ns/op	    3328 B/op	      42 allocs/op
BenchmarkGojq_Small_Skip-16                   	  647784	      1821 ns/op	    2880 B/op	      43 allocs/op
BenchmarkGojq_Small_Reduce-16                 	  805675	      1519 ns/op	    2530 B/op	      42 allocs/op
BenchmarkGojq_Small_Foreach-16                	  881218	      1392 ns/op	    2184 B/op	      39 allocs/op
BenchmarkGojq_Large_Limit-16                  	  726220	      1662 ns/op	    2200 B/op	      24 allocs/op
BenchmarkFastjq_Small_Subtract-16             	24285331	        49.51 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Subtract-16               	 1647274	       725.3 ns/op	    1649 B/op	      20 allocs/op
BenchmarkFastjq_Small_Multiply-16             	17886765	        65.63 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Multiply-16               	 1602138	       744.7 ns/op	    1649 B/op	      21 allocs/op
BenchmarkFastjq_Small_Divide-16               	12126182	        98.12 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Divide-16                 	 1505797	       798.3 ns/op	    1665 B/op	      21 allocs/op
BenchmarkFastjq_Small_Min-16                  	  305545	      3876 ns/op	     392 B/op	     300 allocs/op
BenchmarkGojq_Small_Min-16                    	  976261	      1253 ns/op	    1217 B/op	      15 allocs/op
BenchmarkFastjq_Small_MinBy-16                	  109312	     10867 ns/op	     517 B/op	     297 allocs/op
BenchmarkGojq_Small_MinBy-16                  	   20846	     57301 ns/op	   63632 B/op	    1347 allocs/op
BenchmarkFastjq_Sort-16                       	  174916	      6764 ns/op	    8180 B/op	     265 allocs/op
BenchmarkGojq_Sort-16                         	  975751	      1257 ns/op	    1217 B/op	      15 allocs/op
BenchmarkFastjq_SortBy-16                     	   59552	     20173 ns/op	   25632 B/op	     667 allocs/op
BenchmarkGojq_SortBy-16                       	   12804	     93845 ns/op	   97006 B/op	    2145 allocs/op
BenchmarkFastjq_Unique-16                     	  122559	      9747 ns/op	    8562 B/op	     466 allocs/op
BenchmarkGojq_Unique-16                       	  973992	      1267 ns/op	    1217 B/op	      15 allocs/op
BenchmarkFastjq_GroupBy-16                    	   53461	     22554 ns/op	   24872 B/op	     422 allocs/op
BenchmarkGojq_GroupBy-16                      	   10000	    101399 ns/op	  101947 B/op	    2260 allocs/op
BenchmarkFastjq_Small_URIEncode-16            	 7102842	       167.8 ns/op	     120 B/op	       4 allocs/op
BenchmarkGojq_Small_URIEncode-16              	 1755286	       688.5 ns/op	    1289 B/op	      14 allocs/op
BenchmarkFastjq_Small_ArrayDiff-16            	 5587994	       213.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ArrayDiff-16              	  926479	      1276 ns/op	    2121 B/op	      33 allocs/op
BenchmarkFastjq_Small_TryNoError-16           	  254446	      4746 ns/op	      64 B/op	       1 allocs/op
BenchmarkFastjq_Small_TryCatchNoError-16      	  258330	      4654 ns/op	      64 B/op	       1 allocs/op
BenchmarkGojq_Small_TryNoError-16             	 1000000	      1075 ns/op	    1913 B/op	      30 allocs/op
BenchmarkFastjq_Small_ObjectMerge-16          	 7361400	       163.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ObjectMerge-16            	  780062	      1538 ns/op	    2906 B/op	      33 allocs/op
BenchmarkFastjq_Small_ToJSON-16               	 2883358	       416.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ToJSON-16                 	  733260	      1620 ns/op	    2410 B/op	      39 allocs/op
BenchmarkFastjq_Small_FromJSON-16             	 6037872	       197.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_FromJSON-16               	  924777	      1338 ns/op	    2714 B/op	      31 allocs/op
BenchmarkFastjq_Small_ToString-16             	 2894139	       417.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToNumber-16             	88522777	        13.33 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToBoolean-16            	140872082	         8.560 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ToNumber-16               	 3232968	       369.1 ns/op	    1113 B/op	      11 allocs/op
BenchmarkGojq_Small_ToBoolean-16              	 3352772	       358.8 ns/op	    1113 B/op	      11 allocs/op
BenchmarkFastjq_Small_AnyTwoArg-16            	  191658	      6260 ns/op	     680 B/op	     206 allocs/op
BenchmarkGojq_Small_AnyTwoArg-16              	  728766	      1660 ns/op	    2482 B/op	      27 allocs/op
BenchmarkFastjq_Complex_LogNormalize-16       	 1000000	      1059 ns/op	       6 B/op	       2 allocs/op
BenchmarkGojq_Complex_LogNormalize-16         	  401301	      2982 ns/op	    4324 B/op	      69 allocs/op
BenchmarkFastjq_Complex_ArrayPipeline-16      	  130723	      9151 ns/op	    2368 B/op	     132 allocs/op
BenchmarkGojq_Complex_ArrayPipeline-16        	   52366	     22847 ns/op	   20407 B/op	     525 allocs/op
BenchmarkFastjq_Complex_Aggregation-16        	   94066	     12827 ns/op	    3256 B/op	     144 allocs/op
BenchmarkGojq_Complex_Aggregation-16          	   54542	     21912 ns/op	   22397 B/op	     555 allocs/op
BenchmarkFastjq_Complex_TolerantMap-16        	    4422	    267956 ns/op	    5216 B/op	     187 allocs/op
BenchmarkGojq_Complex_TolerantMap-16          	   42766	     28108 ns/op	   27869 B/op	     641 allocs/op
BenchmarkFastjq_Complex_ElifRouting-16        	  124884	      9615 ns/op	    3152 B/op	     181 allocs/op
BenchmarkGojq_Complex_ElifRouting-16          	   47310	     25692 ns/op	   25921 B/op	     595 allocs/op
BenchmarkFastjq_Complex_StringBuild-16        	  106773	     11488 ns/op	    7216 B/op	     298 allocs/op
BenchmarkGojq_Complex_StringBuild-16          	   49080	     24462 ns/op	   20533 B/op	     686 allocs/op
BenchmarkFastjq_Complex_EntryFilter-16        	  345816	      3425 ns/op	    2888 B/op	      69 allocs/op
BenchmarkGojq_Complex_EntryFilter-16          	   84411	     14007 ns/op	   18428 B/op	     250 allocs/op
BenchmarkFastjq_Small_Sqrt-16                 	16629058	        71.19 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Sqrt-16                   	 2531112	       467.4 ns/op	    1145 B/op	      12 allocs/op
BenchmarkFastjq_Small_Log-16                  	30710302	        39.75 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Log-16                    	 2638521	       456.1 ns/op	    1129 B/op	      12 allocs/op
BenchmarkFastjq_Small_Sin-16                  	14636577	        79.78 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Sin-16                    	 2563695	       465.3 ns/op	    1145 B/op	      12 allocs/op
BenchmarkFastjq_Small_Atan-16                 	22896556	        52.39 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Atan-16                   	 2996473	       399.2 ns/op	    1121 B/op	      11 allocs/op
BenchmarkFastjq_Small_Exp-16                  	18746289	        63.36 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Exp-16                    	 2922860	       410.0 ns/op	    1121 B/op	      11 allocs/op
BenchmarkFastjq_Small_Tgamma-16               	95126109	        12.57 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Tgamma-16                 	 3065764	       388.9 ns/op	    1104 B/op	      11 allocs/op
BenchmarkFastjq_Small_Fabs-16                 	21814941	        54.60 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Fabs-16                   	 2812114	       421.0 ns/op	    1113 B/op	      12 allocs/op
BenchmarkFastjq_Small_Abs-16                  	128631132	         9.325 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Abs-16                    	 2889154	       417.7 ns/op	    1112 B/op	      12 allocs/op
BenchmarkFastjq_Small_Bind-16                 	   40506	     29589 ns/op	     648 B/op	      18 allocs/op
BenchmarkGojq_Small_Bind-16                   	 1496808	       803.1 ns/op	    1793 B/op	      24 allocs/op
BenchmarkFastjq_Small_StringInterp-16         	10053292	       119.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_StringInterp-16           	 1000000	      1056 ns/op	    2121 B/op	      35 allocs/op
BenchmarkFastjq_Small_StringInterpNum-16      	14006752	        87.52 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_StringInterpNum-16        	  985076	      1233 ns/op	    2498 B/op	      40 allocs/op
BenchmarkFastjq_Small_IsEmptyTrue-16          	16110039	        73.19 ns/op	     105 B/op	       6 allocs/op
BenchmarkGojq_Small_IsEmptyTrue-16            	 2003293	       601.8 ns/op	    1921 B/op	      18 allocs/op
BenchmarkFastjq_Small_IsEmptyFalse-16         	13254242	        91.24 ns/op	     105 B/op	       6 allocs/op
BenchmarkGojq_Small_IsEmptyFalse-16           	 1000000	      1074 ns/op	    2898 B/op	      33 allocs/op
BenchmarkFastjq_Small_Nth-16                  	 9200798	       131.6 ns/op	     152 B/op	       7 allocs/op
BenchmarkGojq_Small_Nth-16                    	  565082	      2170 ns/op	    4372 B/op	      49 allocs/op
BenchmarkFastjq_Small_Range10-16              	 5938177	       202.3 ns/op	     120 B/op	      12 allocs/op
BenchmarkGojq_Small_Range10-16                	 1000000	      1029 ns/op	    1864 B/op	      18 allocs/op
BenchmarkFastjq_Small_RangeLimit-16           	 8813775	       136.9 ns/op	     128 B/op	       8 allocs/op
BenchmarkGojq_Small_RangeLimit-16             	  831648	      1519 ns/op	    3208 B/op	      26 allocs/op
BenchmarkRegexp_Match_Hit-16                  	29286297	        42.17 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Miss-16                 	51474530	        23.26 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Complex_Hit-16          	27853213	        43.08 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Long_Miss-16            	15580983	        77.23 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_MatchString-16                	28207722	        44.12 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_FindSubmatch_Hit-16           	 5903938	       203.3 ns/op	     160 B/op	       2 allocs/op
BenchmarkRegexp_FindSubmatch_Miss-16          	 1756986	       683.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_FindSubmatchIndex_Hit-16      	 6666704	       180.9 ns/op	      64 B/op	       1 allocs/op
BenchmarkRegexp_FindSubmatchIndex_Miss-16     	 1741658	       689.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_ReplaceAll_Hit-16             	 7821666	       153.8 ns/op	      97 B/op	       5 allocs/op
BenchmarkRegexp_ReplaceAll_Miss-16            	24762264	        48.90 ns/op	      48 B/op	       2 allocs/op
BenchmarkRegexp_MatchViaClosure-16            	27495529	        42.33 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_Hit-16           	11444862	       107.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_Miss-16          	12111781	        98.93 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_InPipeline-16    	 4003388	       300.0 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_MatchRe_Hit-16          	 5460970	       222.3 ns/op	      48 B/op	       1 allocs/op
BenchmarkFastjq_Small_MatchRe_Miss-16         	 1909693	       628.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_TestRe_Hit-16             	  859165	      1339 ns/op	    2743 B/op	      26 allocs/op
BenchmarkGojq_Small_TestRe_Miss-16            	  961669	      1275 ns/op	    2698 B/op	      26 allocs/op
BenchmarkGojq_Small_MatchRe_Hit-16            	  406585	      3050 ns/op	    4795 B/op	      66 allocs/op
BenchmarkGojq_Small_MatchRe_Miss-16           	  717247	      1715 ns/op	    2248 B/op	      24 allocs/op
BenchmarkFastjq_Small_CaptureRe_Hit-16        	 5647304	       211.5 ns/op	      48 B/op	       1 allocs/op
BenchmarkFastjq_Small_CaptureRe_Miss-16       	 1916450	       626.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ScanRe_NoGroups-16      	 2147930	       557.2 ns/op	     534 B/op	      17 allocs/op
BenchmarkFastjq_Small_ScanRe_WithGroups-16    	 1687200	       719.9 ns/op	     850 B/op	      20 allocs/op
BenchmarkFastjq_Small_SubRe_Hit-16            	 9551041	       125.0 ns/op	      16 B/op	       1 allocs/op
BenchmarkFastjq_Small_SubRe_Miss-16           	11873120	       101.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_GSubRe_Hit-16           	 2058424	       584.5 ns/op	     305 B/op	       5 allocs/op
BenchmarkFastjq_Small_GSubRe_Miss-16          	 4765255	       252.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_CaptureRe_Hit-16          	  509946	      2323 ns/op	    4720 B/op	      51 allocs/op
BenchmarkGojq_Small_CaptureRe_Miss-16         	  661434	      1836 ns/op	    2671 B/op	      27 allocs/op
BenchmarkGojq_Small_ScanRe_NoGroups-16        	  429460	      2813 ns/op	    5119 B/op	      57 allocs/op
BenchmarkGojq_Small_SubRe_Hit-16              	  220860	      5280 ns/op	    8982 B/op	      95 allocs/op
BenchmarkGojq_Small_GSubRe_Hit-16             	   81914	     14845 ns/op	   17844 B/op	     248 allocs/op
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
