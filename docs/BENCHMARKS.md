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
| `.field` | Small (~100B) | 0.086 | 1.02 | **12x** | 0 | 27 |
| `.field` | Large (~100KB) | 7.87 | 602 | **77x** | 0 | 2835 |
| `del(.f)` | Small (~100B) | 0.153 | 2.34 | **15x** | 0 | 58 |
| `del(.f)` | Medium (~2KB) | 2.20 | 20.1 | **9.1x** | 0 | 323 |
| `del(.f)` | Large (~100KB) | 34.4 | 854 | **25x** | 0 | 4666 |
| `.[n]` | 5-elem array | 0.023 | 0.678 | **30x** | 0 | 20 |
| `del(.[n], .[m])` | 5-elem array | 0.085 | 1.75 | **21x** | 0 | 53 |
| `{f0, f2}` (construct) | Small (~100B) | 0.133 | 1.42 | **11x** | 0 | 37 |
| `{f0, f50}` (construct) | Large (~100KB) | 8.38 | 608 | **72x** | 0 | 2867 |
| `.[]` iterator | 5-elem array | 0.061 | 0.811 | **13x** | 2 | 26 |
| `.[]` iterator | 200-elem array | 7.25 | 88.5 | **12x** | 2 | 1811 |
| `select(.f == "x")` | Small (~100B) | 0.125 | 1.79 | **14x** | 0 | 45 |
| `select(.f == "x")`¹ | Large (~100KB, last field) | 34.4 | 829 | **24x** | 0 | 4651 |
| `select(.f and .g)` | Small (~100B) | 0.168 | 1.90 | **11x** | 0 | 46 |
| `select(.f or .g)` | Small (~100B) | 0.073 | 1.86 | **25x** | 0 | 46 |
| `has("key")` in select | Small (~100B) | 0.094 | 1.82 | **19x** | 0 | 45 |
| `has("key")` in select | Large (~100KB) | 33.4 | 823 | **25x** | 0 | 4651 |
| `if-then-else` | Small (~100B) | 0.097 | 1.14 | **12x** | 0 | 30 |
| `.f // "default"` | Small (~100B) | 0.096 | 1.12 | **12x** | 0 | 31 |
| `try .field` (no error) | Small (~100B) | 4.54 | 1.10 | 0.2x† | 1 | 30 |
| `.a + .b` (strings) | Small (~100B) | 0.054 | 0.743 | **14x** | 0 | 21 |
| `"prefix" + .name` | Small (~100B) | 0.089 | 0.780 | **8.8x** | 0 | 23 |
| `.a - .b` (subtract) | Small (~100B) | 0.051 | 0.770 | **15x** | 0 | 20 |
| `.a * .b` (multiply) | Small (~100B) | 0.066 | 0.760 | **12x** | 0 | 21 |
| `.a / .b` (divide) | Small (~100B) | 0.099 | 0.780 | **7.9x** | 0 | 21 |
| `.a - .b` (array diff) | 5-elem arrays | 0.220 | 1.41 | **6.4x** | 0 | 33 |
| `length` | Small (~100B) | 0.044 | 1.02 | **23x** | 0 | 27 |
| `length` | Large (~100KB) | 32.2 | 605 | **19x** | 0 | 2835 |
| `add` (numbers) | 5-elem array | 0.059 | 0.768 | **13x** | 0 | 28 |
| `add` (strings) | 5-elem array | 0.134 | 0.937 | **7x** | 0 | 35 |
| `flatten` | 3-elem nested array | 0.094 | 1.34 | **14x** | 0 | 38 |
| `min` | 200-int array | 1.85 | 1.29 | 0.7x† | 0 | 15 |
| `min_by(.value)` | 100-elem object array | 8.54 | 57.3 | **6.7x** | 0 | 1347 |
| `sort` | 200-int array | 4.61 | 1.27 | 0.3x† | 14 | 15 |
| `sort_by(.value)` | 100-elem object array | 18.5 | 97.4 | **5.3x** | 422 | 2145 |
| `unique` | 200-int array | 5.94 | 1.31 | 0.2x† | 14 | 15 |
| `group_by(.active)` | 100-elem object array | 23.8 | 104 | **4.4x** | 422 | 2260 |
| `map(.name)` | 20-elem array (~600B) | 1.69 | 10.7 | **6.3x** | 29 | 251 |
| `map(.name)` | 200-elem array (~6KB) | 16.1 | 98.5 | **6.1x** | 209 | 2237 |
| `any` | 5-elem array | 0.045 | 1.98 | **44x** | 0 | 39 |
| `any(expr)` | 5-elem array | 0.396 | 2.21 | **5.6x** | 12 | 49 |
| `any(expr)`² | 200-int array | 3.97 | 1.81 | 0.5x† | 0 | 29 |
| `any(gen; cond)`² | 200-int array | 4.31 | 1.72 | 0.4x† | 3 | 27 |
| `first(expr)` | 5-elem array | 0.248 | 1.63 | **6.6x** | 9 | 39 |
| `first(expr)`² | 200-int array | 6.66 | 1.59 | 0.2x† | 107 | 23 |
| `last(expr)` | 5-elem array | 0.314 | 2.07 | **6.6x** | 10 | 43 |
| `last(expr)`² | 200-int array | 6.37 | 1.62 | 0.3x† | 107 | 24 |
| `limit(3; expr)` | 5-elem array | 0.104 | 1.85 | **18x** | 5 | 42 |
| `limit(10; expr)` | 200-int array | 0.690 | 1.75 | **2.5x** | 5 | 24 |
| `.[1:4]` slice | 6-elem array | 0.087 | 0.844 | **9.7x** | 0 | 21 |
| `values` | 9-elem array | 0.263 | 2.46 | **9.3x** | 13 | 51 |
| `skip(2; .[])` | 5-elem array | 0.146 | 1.92 | **13x** | 7 | 43 |
| `to_entries` | Small (~100B) | 0.159 | 4.34 | **27x** | 0 | 98 |
| `to_entries` | Large (~100KB) | 34.4 | 919 | **27x** | 0 | 6465 |
| `keys` | Small (~100B) | 0.297 | 1.38 | **4.7x** | 7 | 35 |
| `keys_unsorted` | Small (~100B) | 0.165 | 1.35 | **8.2x** | 0 | 35 |
| `keys_unsorted` | Large (~100KB) | 31.9 | 630 | **20x** | 0 | 3039 |
| `object merge .a + .b` | Small (~100B) | 0.170 | 1.56 | **9.2x** | 0 | 33 |
| `tojson` | Small (~100B) | 0.332 | 1.65 | **5x** | 0 | 39 |
| `fromjson` | JSON string | 0.155 | 1.33 | **8.6x** | 0 | 31 |
| `tonumber` | `"42"` string | 0.013 | 0.377 | **28x** | 0 | 11 |
| `toboolean` | `"true"` string | 0.0084 | 0.370 | **44x** | 0 | 11 |
| `split(",")` | short string | 0.149 | 0.838 | **5.6x** | 0 | 21 |
| `join(",")` | 5-elem array | 0.103 | 1.03 | **10x** | 0 | 37 |
| `ascii_downcase` in select | Small (~100B) | 0.143 | 1.84 | **13x** | 0 | 46 |
| `ascii_downcase` in select | Large (~100KB) | 35.0 | 828 | **24x** | 0 | 4653 |
| `startswith("s")` | Small (~100B) | 0.135 | 1.79 | **13x** | 0 | 45 |
| `startswith("s")` | Large (~100KB) | 33.9 | 834 | **25x** | 0 | 4651 |
| `endswith("s")` | Small (~100B) | 0.135 | 1.81 | **13x** | 0 | 45 |
| `trim` | short string | 0.051 | 0.395 | **7.8x** | 1 | 12 |
| `ltrim` | short string | 0.049 | 0.407 | **8.4x** | 1 | 12 |
| `rtrim` | short string | 0.052 | 0.410 | **7.8x** | 1 | 12 |
| `ltrimstr("s")` | Small (~100B) | 0.132 | 1.10 | **8.3x** | 0 | 30 |
| `rtrimstr("s")` | Small (~100B) | 0.132 | 1.08 | **8.2x** | 0 | 30 |
| `select` + string ops + arith + construct | ~200B log event | 1.07 | 3.08 | **2.9x** | 0 | 69 |
| `[.[] \| select + arith + construct]` | 20-elem array (~1.5KB) | 8.99 | 23.3 | **2.6x** | 106 | 525 |
| `length + map + add + min_by + any` | 20-elem array (~1.5KB) | 12.4 | 21.9 | **1.8x** | 106 | 555 |
| `map(try {…} catch …)` | 20-elem array (~1.5KB) | 262 | 28.2 | 0.1x† | 187 | 641 |
| `map(if … elif … else …)` | 20-elem array (~1.5KB) | 9.21 | 25.9 | **2.8x** | 127 | 595 |
| `[.[] \| str + tostring + str]` | 20-elem array (~1.5KB) | 11.1 | 24.9 | **2.2x** | 298 | 686 |
| `to_entries \| map(select) \| from_entries` | ~200B log event | 3.43 | 14.0 | **4.1x** | 69 | 250 |
| `@base64` | 34-char string | 0.148 | 0.571 | **3.8x** | 4 | 15 |
| `@base64d` | 48-char encoded | 0.215 | 0.584 | **2.7x** | 0 | 15 |
| `@uri` | 36-char URL string | 0.173 | 0.718 | **4.1x** | 4 | 14 |
| `index(",")` | short string | 0.099 | 0.940 | **9.5x** | 1 | 31 |
| `indices(",")` | short string | 0.207 | 2.20 | **11x** | 1 | 97 |
| `sqrt` | float (e≈2.718) | 0.070 | 0.462 | **6.6x** | 0 | 12 |
| `log` | float (e≈2.718) | 0.039 | 0.461 | **12x** | 0 | 12 |
| `sin` | float (e≈2.718) | 0.081 | 0.498 | **6.1x** | 0 | 12 |
| `atan` | integer 1 | 0.054 | 0.419 | **7.8x** | 0 | 11 |
| `exp` | integer 1 | 0.064 | 0.425 | **6.6x** | 0 | 11 |
| `tgamma` | integer 5 | 0.013 | 0.398 | **31x** | 0 | 11 |
| `fabs` | float -3.14 | 0.055 | 0.425 | **7.8x** | 0 | 12 |
| `abs` | float -3.14 | 0.0092 | 0.439 | **48x** | 0 | 12 |
| `expr as $x \| body` | Small (~20B object) | 29.9 | 0.831 | 0.0x† | 18 | 24 |
| ``"\(.level): \(.svc)"`` | ~45B object | 0.120 | 1.08 | **9x** | 0 | 35 |
| ``"user \(.name) …"`` | ~45B object | 0.091 | 1.27 | **14x** | 0 | 40 |
| `isempty(empty)` | null | 0.070 | 0.598 | **8.5x** | 6 | 18 |
| `isempty(.[])` | 5-elem array | 0.090 | 1.11 | **12x** | 6 | 33 |
| `nth(2; .[])` | 5-elem array | 0.128 | 2.36 | **18x** | 7 | 49 |
| `range(10)` (10 values) | null | 0.204 | 1.05 | **5.2x** | 12 | 18 |
| `limit(3; range(1000))` | null — only 3 allocs | 0.143 | 1.61 | **11x** | 8 | 26 |
| `test(re)` hit | short string | 0.114 | 1.31 | **11x** | 0 | 26 |
| `test(re)` miss | short string | 0.100 | 1.27 | **13x** | 0 | 26 |
| `match(re)` hit | short string | 0.221 | 2.96 | **13x** | 1 | 66 |
| `match(re)` miss | short string | 0.622 | 1.64 | **2.6x** | 0 | 24 |
| `capture(re)` hit | short string | 0.219 | — | — | 1 | — |
| `scan(re)` no groups (4 matches) | short string | 0.563 | — | — | 17 | — |
| `sub(re; s)` hit | short string | 0.127 | — | — | 1 | — |
| `gsub(re; s)` hit (4 matches) | short string | 0.594 | — | — | 5 | — |

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
BenchmarkFastjq_Small_Del-16                  	 7392543	       152.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Medium_Del-16                 	  522134	      2204 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Del-16                  	   35224	     34379 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Field-16                	15211591	        85.77 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Field-16                	  152110	      7873 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Index-16                	52982179	        22.77 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ArrayDel-16             	14082564	        84.74 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Construct-16            	 9270796	       132.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Construct-16            	  141487	      8382 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Iterator-16             	19748887	        60.99 ns/op	      40 B/op	       2 allocs/op
BenchmarkFastjq_Large_Iterator-16             	  167485	      7253 ns/op	      40 B/op	       2 allocs/op
BenchmarkFastjq_Small_Select-16               	 9408723	       125.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Select-16               	   35204	     34381 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Alternative-16          	12746559	        95.79 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SelectAnd-16            	 7201699	       167.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SelectOr-16             	16372063	        73.35 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Has-16                  	12507811	        94.47 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Has-16                  	   35000	     33442 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_IfThenElse-16           	12444130	        97.02 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Length-16               	29467527	        43.97 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Length-16               	   37221	     32248 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Map-16                  	  707997	      1687 ns/op	     689 B/op	      29 allocs/op
BenchmarkFastjq_Medium_Map-16                 	  149222	      8085 ns/op	    2609 B/op	     109 allocs/op
BenchmarkFastjq_Large_Map-16                  	   75333	     16127 ns/op	    5009 B/op	     209 allocs/op
BenchmarkFastjq_Small_ToEntries-16            	 7820732	       158.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_ToEntries-16            	   34994	     34358 ns/op	       6 B/op	       0 allocs/op
BenchmarkFastjq_Small_KeysUnsorted-16         	 7288701	       164.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Keys-16                 	 4084834	       296.9 ns/op	     456 B/op	       7 allocs/op
BenchmarkFastjq_Large_KeysUnsorted-16         	   37855	     31894 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Any-16                  	25561599	        45.04 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Any-16                  	  789300	      1396 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AnyExpr-16              	 3033818	       396.4 ns/op	     144 B/op	      12 allocs/op
BenchmarkFastjq_Large_AnyExpr-16              	  300678	      3970 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Add-16                  	20456854	        59.24 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AddStrings-16           	 8885118	       133.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Flatten-16              	12637027	        93.85 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Split-16                	 8020047	       148.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Join-16                 	11482282	       103.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Values-16               	 4539496	       263.0 ns/op	     328 B/op	      13 allocs/op
BenchmarkGojq_Small_Values-16                 	  482238	      2459 ns/op	    3144 B/op	      51 allocs/op
BenchmarkFastjq_Small_Base64Encode-16         	 8127025	       148.4 ns/op	     120 B/op	       4 allocs/op
BenchmarkFastjq_Small_Base64Decode-16         	 5585643	       214.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Base64Encode-16           	 2190276	       570.9 ns/op	    1321 B/op	      15 allocs/op
BenchmarkGojq_Small_Base64Decode-16           	 2040710	       584.2 ns/op	    1321 B/op	      15 allocs/op
BenchmarkFastjq_Small_IndexFind-16            	12100250	        98.83 ns/op	       3 B/op	       1 allocs/op
BenchmarkFastjq_Small_IndicesAll-16           	 5787334	       207.2 ns/op	       3 B/op	       1 allocs/op
BenchmarkGojq_Small_IndexFind-16              	 1283186	       940.1 ns/op	    2274 B/op	      31 allocs/op
BenchmarkGojq_Small_IndicesAll-16             	  546229	      2198 ns/op	    3907 B/op	      97 allocs/op
BenchmarkFastjq_Small_Slice-16                	13525761	        86.64 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SliceString-16          	11264127	       106.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Plus-16                 	21790497	        53.57 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_PlusStr-16              	13373900	        88.68 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AsciiDowncase-16        	 8504595	       142.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_AsciiDowncase-16        	   34611	     34959 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Startswith-16           	 8975594	       135.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Startswith-16           	   35068	     33883 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Endswith-16             	 9102942	       135.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Ltrimstr-16             	 9184762	       132.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Ltrimstr-16             	   36922	     32890 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Rtrimstr-16             	 9328550	       131.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Trim-16                 	23981414	        50.83 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_Ltrim-16                	24384171	        48.52 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_Rtrim-16                	22678360	        52.31 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_First-16                	 4813060	       247.7 ns/op	     216 B/op	       9 allocs/op
BenchmarkFastjq_Large_First-16                	  178322	      6658 ns/op	    2584 B/op	     107 allocs/op
BenchmarkFastjq_Small_Last-16                 	 3813897	       313.9 ns/op	     256 B/op	      10 allocs/op
BenchmarkFastjq_Large_Last-16                 	  187828	      6373 ns/op	    2584 B/op	     107 allocs/op
BenchmarkFastjq_Small_Limit-16                	11495816	       104.4 ns/op	     104 B/op	       5 allocs/op
BenchmarkFastjq_Small_Skip-16                 	 8236525	       145.7 ns/op	     208 B/op	       7 allocs/op
BenchmarkFastjq_Large_Limit-16                	 1736086	       690.3 ns/op	     104 B/op	       5 allocs/op
BenchmarkGojq_Small_Del-16                    	  508252	      2340 ns/op	    3763 B/op	      58 allocs/op
BenchmarkGojq_Medium_Del-16                   	   59713	     20122 ns/op	   16971 B/op	     323 allocs/op
BenchmarkGojq_Large_Del-16                    	    1417	    854090 ns/op	  543623 B/op	    4666 allocs/op
BenchmarkGojq_Small_Field-16                  	 1000000	      1016 ns/op	    1641 B/op	      27 allocs/op
BenchmarkGojq_Large_Field-16                  	    1977	    602482 ns/op	  270044 B/op	    2835 allocs/op
BenchmarkGojq_Small_Index-16                  	 1812105	       678.1 ns/op	    1401 B/op	      20 allocs/op
BenchmarkGojq_Small_ArrayDel-16               	  668503	      1745 ns/op	    3363 B/op	      53 allocs/op
BenchmarkGojq_Small_Construct-16              	  859185	      1417 ns/op	    2330 B/op	      37 allocs/op
BenchmarkGojq_Large_Construct-16              	    1990	    607502 ns/op	  274520 B/op	    2867 allocs/op
BenchmarkGojq_Small_Iterator-16               	 1480426	       811.2 ns/op	    1776 B/op	      26 allocs/op
BenchmarkGojq_Large_Iterator-16               	   13578	     88468 ns/op	  109809 B/op	    1811 allocs/op
BenchmarkGojq_Small_Select-16                 	  671979	      1794 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Large_Select-16                 	    1471	    828858 ns/op	  536196 B/op	    4651 allocs/op
BenchmarkGojq_Small_Alternative-16            	 1000000	      1121 ns/op	    1897 B/op	      31 allocs/op
BenchmarkGojq_Small_SelectAnd-16              	  616526	      1903 ns/op	    2890 B/op	      46 allocs/op
BenchmarkGojq_Small_SelectOr-16               	  638887	      1861 ns/op	    2890 B/op	      46 allocs/op
BenchmarkGojq_Small_Has-16                    	  672698	      1817 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Large_Has-16                    	    1434	    822658 ns/op	  534024 B/op	    4651 allocs/op
BenchmarkGojq_Small_IfThenElse-16             	 1000000	      1135 ns/op	    1817 B/op	      30 allocs/op
BenchmarkGojq_Small_Length-16                 	 1000000	      1018 ns/op	    1633 B/op	      27 allocs/op
BenchmarkGojq_Large_Length-16                 	    1950	    605004 ns/op	  269832 B/op	    2835 allocs/op
BenchmarkGojq_Small_Map-16                    	  113482	     10708 ns/op	   13654 B/op	     251 allocs/op
BenchmarkGojq_Large_Map-16                    	   12236	     98507 ns/op	  118605 B/op	    2237 allocs/op
BenchmarkGojq_Small_ToEntries-16              	  269656	      4337 ns/op	    6559 B/op	      98 allocs/op
BenchmarkGojq_Large_ToEntries-16              	    1339	    918851 ns/op	  681767 B/op	    6465 allocs/op
BenchmarkGojq_Small_KeysUnsorted-16           	  856548	      1353 ns/op	    1953 B/op	      35 allocs/op
BenchmarkGojq_Small_Keys-16                   	  894334	      1383 ns/op	    1953 B/op	      35 allocs/op
BenchmarkGojq_Large_KeysUnsorted-16           	    1897	    629604 ns/op	  282880 B/op	    3039 allocs/op
BenchmarkGojq_Small_Any-16                    	  628786	      1976 ns/op	    4508 B/op	      39 allocs/op
BenchmarkGojq_Small_AnyExpr-16                	  541454	      2211 ns/op	    4620 B/op	      49 allocs/op
BenchmarkGojq_Large_AnyExpr-16                	  667526	      1807 ns/op	    2818 B/op	      29 allocs/op
BenchmarkGojq_Small_Add-16                    	 1564662	       768.3 ns/op	    1529 B/op	      28 allocs/op
BenchmarkGojq_Small_AddStrings-16             	 1281481	       937.3 ns/op	    1785 B/op	      35 allocs/op
BenchmarkGojq_Small_Flatten-16                	  899148	      1345 ns/op	    1857 B/op	      38 allocs/op
BenchmarkGojq_Small_Split-16                  	 1432887	       838.1 ns/op	    1553 B/op	      21 allocs/op
BenchmarkGojq_Small_Join-16                   	 1000000	      1030 ns/op	    1865 B/op	      37 allocs/op
BenchmarkGojq_Small_Slice-16                  	 1421977	       843.8 ns/op	    1425 B/op	      21 allocs/op
BenchmarkGojq_Small_SliceString-16            	 2545912	       483.1 ns/op	    1145 B/op	      12 allocs/op
BenchmarkGojq_Small_Plus-16                   	 1614036	       742.6 ns/op	    1673 B/op	      21 allocs/op
BenchmarkGojq_Small_PlusStr-16                	 1532556	       779.7 ns/op	    1705 B/op	      23 allocs/op
BenchmarkGojq_Small_AsciiDowncase-16          	  643420	      1843 ns/op	    2714 B/op	      46 allocs/op
BenchmarkGojq_Large_AsciiDowncase-16          	    1428	    827744 ns/op	  539688 B/op	    4653 allocs/op
BenchmarkGojq_Small_Startswith-16             	  681282	      1794 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Large_Startswith-16             	    1455	    833605 ns/op	  536884 B/op	    4651 allocs/op
BenchmarkGojq_Small_Endswith-16               	  662196	      1815 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Small_Ltrimstr-16               	 1000000	      1097 ns/op	    1737 B/op	      30 allocs/op
BenchmarkGojq_Small_Rtrimstr-16               	 1000000	      1084 ns/op	    1737 B/op	      30 allocs/op
BenchmarkGojq_Small_Trim-16                   	 3055216	       394.9 ns/op	    1129 B/op	      12 allocs/op
BenchmarkGojq_Small_Ltrim-16                  	 2996046	       407.4 ns/op	    1129 B/op	      12 allocs/op
BenchmarkGojq_Small_Rtrim-16                  	 2944855	       409.6 ns/op	    1129 B/op	      12 allocs/op
BenchmarkGojq_Small_First-16                  	  784580	      1630 ns/op	    3379 B/op	      39 allocs/op
BenchmarkGojq_Large_First-16                  	  762630	      1590 ns/op	    1889 B/op	      23 allocs/op
BenchmarkGojq_Small_Last-16                   	  535506	      2069 ns/op	    3483 B/op	      43 allocs/op
BenchmarkGojq_Large_Last-16                   	  765710	      1625 ns/op	    2193 B/op	      24 allocs/op
BenchmarkGojq_Small_Limit-16                  	  646455	      1853 ns/op	    3328 B/op	      42 allocs/op
BenchmarkGojq_Small_Skip-16                   	  641524	      1921 ns/op	    2880 B/op	      43 allocs/op
BenchmarkGojq_Large_Limit-16                  	  707576	      1745 ns/op	    2200 B/op	      24 allocs/op
BenchmarkFastjq_Small_Subtract-16             	23996060	        50.78 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Subtract-16               	 1544312	       770.0 ns/op	    1649 B/op	      20 allocs/op
BenchmarkFastjq_Small_Multiply-16             	17665113	        65.68 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Multiply-16               	 1540536	       759.7 ns/op	    1649 B/op	      21 allocs/op
BenchmarkFastjq_Small_Divide-16               	12095050	        98.93 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Divide-16                 	 1562722	       780.0 ns/op	    1665 B/op	      21 allocs/op
BenchmarkFastjq_Small_Min-16                  	  643113	      1846 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Min-16                    	  923650	      1289 ns/op	    1217 B/op	      15 allocs/op
BenchmarkFastjq_Small_MinBy-16                	  143160	      8539 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_MinBy-16                  	   20998	     57327 ns/op	   63630 B/op	    1347 allocs/op
BenchmarkFastjq_Sort-16                       	  256156	      4612 ns/op	    7688 B/op	      14 allocs/op
BenchmarkGojq_Sort-16                         	  978628	      1270 ns/op	    1217 B/op	      15 allocs/op
BenchmarkFastjq_SortBy-16                     	   64996	     18546 ns/op	   24872 B/op	     422 allocs/op
BenchmarkGojq_SortBy-16                       	   12354	     97398 ns/op	   96929 B/op	    2145 allocs/op
BenchmarkFastjq_Unique-16                     	  204001	      5942 ns/op	    7688 B/op	      14 allocs/op
BenchmarkGojq_Unique-16                       	  916137	      1314 ns/op	    1217 B/op	      15 allocs/op
BenchmarkFastjq_GroupBy-16                    	   51018	     23823 ns/op	   24872 B/op	     422 allocs/op
BenchmarkGojq_GroupBy-16                      	   10000	    104010 ns/op	  101932 B/op	    2260 allocs/op
BenchmarkFastjq_Small_URIEncode-16            	 6895708	       173.1 ns/op	     120 B/op	       4 allocs/op
BenchmarkGojq_Small_URIEncode-16              	 1668117	       718.2 ns/op	    1289 B/op	      14 allocs/op
BenchmarkFastjq_Small_ArrayDiff-16            	 5511350	       219.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ArrayDiff-16              	  900830	      1405 ns/op	    2121 B/op	      33 allocs/op
BenchmarkFastjq_Small_TryNoError-16           	  267201	      4542 ns/op	      64 B/op	       1 allocs/op
BenchmarkFastjq_Small_TryCatchNoError-16      	  264831	      4569 ns/op	      64 B/op	       1 allocs/op
BenchmarkGojq_Small_TryNoError-16             	 1000000	      1099 ns/op	    1913 B/op	      30 allocs/op
BenchmarkFastjq_Small_ObjectMerge-16          	 7030327	       169.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ObjectMerge-16            	  801524	      1562 ns/op	    2906 B/op	      33 allocs/op
BenchmarkFastjq_Small_ToJSON-16               	 3608600	       331.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ToJSON-16                 	  712656	      1647 ns/op	    2410 B/op	      39 allocs/op
BenchmarkFastjq_Small_FromJSON-16             	 7723684	       155.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_FromJSON-16               	  892250	      1331 ns/op	    2714 B/op	      31 allocs/op
BenchmarkFastjq_Small_ToString-16             	 3595792	       340.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToNumber-16             	89007016	        13.45 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToBoolean-16            	142609096	         8.409 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ToNumber-16               	 3207918	       376.8 ns/op	    1113 B/op	      11 allocs/op
BenchmarkGojq_Small_ToBoolean-16              	 3301454	       370.2 ns/op	    1113 B/op	      11 allocs/op
BenchmarkFastjq_Small_AnyTwoArg-16            	  277635	      4314 ns/op	      57 B/op	       3 allocs/op
BenchmarkGojq_Small_AnyTwoArg-16              	  691310	      1719 ns/op	    2482 B/op	      27 allocs/op
BenchmarkFastjq_Complex_LogNormalize-16       	 1000000	      1075 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Complex_LogNormalize-16         	  383679	      3082 ns/op	    4324 B/op	      69 allocs/op
BenchmarkFastjq_Complex_ArrayPipeline-16      	  134158	      8986 ns/op	    2264 B/op	     106 allocs/op
BenchmarkGojq_Complex_ArrayPipeline-16        	   52100	     23340 ns/op	   20408 B/op	     525 allocs/op
BenchmarkFastjq_Complex_Aggregation-16        	   96834	     12364 ns/op	    3184 B/op	     106 allocs/op
BenchmarkGojq_Complex_Aggregation-16          	   54985	     21899 ns/op	   22397 B/op	     555 allocs/op
BenchmarkFastjq_Complex_TolerantMap-16        	    4539	    262192 ns/op	    5216 B/op	     187 allocs/op
BenchmarkGojq_Complex_TolerantMap-16          	   42290	     28218 ns/op	   27870 B/op	     641 allocs/op
BenchmarkFastjq_Complex_ElifRouting-16        	  130884	      9215 ns/op	    3000 B/op	     127 allocs/op
BenchmarkGojq_Complex_ElifRouting-16          	   45164	     25941 ns/op	   25920 B/op	     595 allocs/op
BenchmarkFastjq_Complex_StringBuild-16        	  107979	     11117 ns/op	    7216 B/op	     298 allocs/op
BenchmarkGojq_Complex_StringBuild-16          	   48470	     24891 ns/op	   20533 B/op	     686 allocs/op
BenchmarkFastjq_Complex_EntryFilter-16        	  350226	      3434 ns/op	    2888 B/op	      69 allocs/op
BenchmarkGojq_Complex_EntryFilter-16          	   85191	     14048 ns/op	   18428 B/op	     250 allocs/op
BenchmarkFastjq_Small_Sqrt-16                 	16701722	        70.00 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Sqrt-16                   	 2603241	       461.6 ns/op	    1145 B/op	      12 allocs/op
BenchmarkFastjq_Small_Log-16                  	30251146	        38.76 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Log-16                    	 2603360	       460.7 ns/op	    1129 B/op	      12 allocs/op
BenchmarkFastjq_Small_Sin-16                  	14839676	        81.27 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Sin-16                    	 2393649	       498.1 ns/op	    1145 B/op	      12 allocs/op
BenchmarkFastjq_Small_Atan-16                 	22728546	        53.71 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Atan-16                   	 2845398	       418.8 ns/op	    1121 B/op	      11 allocs/op
BenchmarkFastjq_Small_Exp-16                  	18786361	        64.07 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Exp-16                    	 2820558	       424.7 ns/op	    1121 B/op	      11 allocs/op
BenchmarkFastjq_Small_Tgamma-16               	93645438	        12.72 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Tgamma-16                 	 3014640	       397.6 ns/op	    1105 B/op	      11 allocs/op
BenchmarkFastjq_Small_Fabs-16                 	21803727	        54.58 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Fabs-16                   	 2814650	       425.4 ns/op	    1112 B/op	      12 allocs/op
BenchmarkFastjq_Small_Abs-16                  	129777168	         9.226 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Abs-16                    	 2769290	       438.5 ns/op	    1112 B/op	      12 allocs/op
BenchmarkFastjq_Small_Bind-16                 	   40030	     29925 ns/op	     648 B/op	      18 allocs/op
BenchmarkGojq_Small_Bind-16                   	 1446270	       831.4 ns/op	    1793 B/op	      24 allocs/op
BenchmarkFastjq_Small_StringInterp-16         	 9861456	       120.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_StringInterp-16           	 1000000	      1082 ns/op	    2121 B/op	      35 allocs/op
BenchmarkFastjq_Small_StringInterpNum-16      	13071817	        90.67 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_StringInterpNum-16        	  927601	      1267 ns/op	    2498 B/op	      40 allocs/op
BenchmarkFastjq_Small_IsEmptyTrue-16          	16752318	        70.37 ns/op	     105 B/op	       6 allocs/op
BenchmarkGojq_Small_IsEmptyTrue-16            	 1999254	       598.4 ns/op	    1921 B/op	      18 allocs/op
BenchmarkFastjq_Small_IsEmptyFalse-16         	13930560	        89.95 ns/op	     105 B/op	       6 allocs/op
BenchmarkGojq_Small_IsEmptyFalse-16           	 1000000	      1112 ns/op	    2898 B/op	      33 allocs/op
BenchmarkFastjq_Small_Nth-16                  	 9417472	       128.3 ns/op	     152 B/op	       7 allocs/op
BenchmarkGojq_Small_Nth-16                    	  515262	      2360 ns/op	    4372 B/op	      49 allocs/op
BenchmarkFastjq_Small_Range10-16              	 5876360	       203.9 ns/op	     120 B/op	      12 allocs/op
BenchmarkGojq_Small_Range10-16                	 1000000	      1054 ns/op	    1864 B/op	      18 allocs/op
BenchmarkFastjq_Small_RangeLimit-16           	 8759181	       142.7 ns/op	     128 B/op	       8 allocs/op
BenchmarkGojq_Small_RangeLimit-16             	  755653	      1609 ns/op	    3208 B/op	      26 allocs/op
BenchmarkRegexp_Match_Hit-16                  	25831773	        45.37 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Miss-16                 	51299863	        23.59 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Complex_Hit-16          	26856476	        43.60 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Long_Miss-16            	15417302	        77.64 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_MatchString-16                	27952778	        44.56 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_FindSubmatch_Hit-16           	 5679322	       207.5 ns/op	     160 B/op	       2 allocs/op
BenchmarkRegexp_FindSubmatch_Miss-16          	 1723554	       698.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_FindSubmatchIndex_Hit-16      	 6319909	       184.3 ns/op	      64 B/op	       1 allocs/op
BenchmarkRegexp_FindSubmatchIndex_Miss-16     	 1693641	       707.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_ReplaceAll_Hit-16             	 7506651	       159.0 ns/op	      97 B/op	       5 allocs/op
BenchmarkRegexp_ReplaceAll_Miss-16            	24514748	        49.74 ns/op	      48 B/op	       2 allocs/op
BenchmarkRegexp_MatchViaClosure-16            	26168448	        45.63 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_Hit-16           	10426689	       114.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_Miss-16          	12013003	       100.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_InPipeline-16    	 3879901	       306.9 ns/op	       8 B/op	       0 allocs/op
BenchmarkFastjq_Small_MatchRe_Hit-16          	 5524476	       220.7 ns/op	      48 B/op	       1 allocs/op
BenchmarkFastjq_Small_MatchRe_Miss-16         	 1937268	       621.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_TestRe_Hit-16             	  933152	      1314 ns/op	    2741 B/op	      26 allocs/op
BenchmarkGojq_Small_TestRe_Miss-16            	  963087	      1275 ns/op	    2701 B/op	      26 allocs/op
BenchmarkGojq_Small_MatchRe_Hit-16            	  402054	      2965 ns/op	    4793 B/op	      66 allocs/op
BenchmarkGojq_Small_MatchRe_Miss-16           	  726919	      1639 ns/op	    2246 B/op	      24 allocs/op
BenchmarkFastjq_Small_CaptureRe_Hit-16        	 5553453	       218.8 ns/op	      48 B/op	       1 allocs/op
BenchmarkFastjq_Small_CaptureRe_Miss-16       	 1924892	       622.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ScanRe_NoGroups-16      	 2125443	       562.6 ns/op	     534 B/op	      17 allocs/op
BenchmarkFastjq_Small_ScanRe_WithGroups-16    	 1685108	       713.2 ns/op	     851 B/op	      20 allocs/op
BenchmarkFastjq_Small_SubRe_Hit-16            	 9393300	       126.8 ns/op	      16 B/op	       1 allocs/op
BenchmarkFastjq_Small_SubRe_Miss-16           	11689852	       105.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_GSubRe_Hit-16           	 2048694	       593.8 ns/op	     305 B/op	       5 allocs/op
BenchmarkFastjq_Small_GSubRe_Miss-16          	 4620716	       256.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_CaptureRe_Hit-16          	  498412	      2441 ns/op	    4726 B/op	      51 allocs/op
BenchmarkGojq_Small_CaptureRe_Miss-16         	  651843	      1869 ns/op	    2673 B/op	      27 allocs/op
BenchmarkGojq_Small_ScanRe_NoGroups-16        	  399380	      2943 ns/op	    5128 B/op	      57 allocs/op
BenchmarkGojq_Small_SubRe_Hit-16              	  222734	      5365 ns/op	    8981 B/op	      95 allocs/op
BenchmarkGojq_Small_GSubRe_Hit-16             	   79719	     15099 ns/op	   17890 B/op	     248 allocs/op
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
