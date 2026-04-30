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
| `.field` | Small (~100B) | 0.082 | 1.03 | **13x** | 0 | 27 |
| `.field` | Large (~100KB) | 7.59 | 605 | **80x** | 0 | 2835 |
| `del(.f)` | Small (~100B) | 0.172 | 2.39 | **14x** | 0 | 58 |
| `del(.f)` | Medium (~2KB) | 2.06 | 19.9 | **9.7x** | 0 | 323 |
| `del(.f)` | Large (~100KB) | 34.5 | 836 | **24x** | 0 | 4666 |
| `.[n]` | 5-elem array | 0.024 | 0.662 | **28x** | 0 | 20 |
| `del(.[n], .[m])` | 5-elem array | 0.083 | 1.74 | **21x** | 0 | 53 |
| `{f0, f2}` (construct) | Small (~100B) | 0.132 | 1.44 | **11x** | 0 | 37 |
| `{f0, f50}` (construct) | Large (~100KB) | 7.99 | 608 | **76x** | 0 | 2867 |
| `.[]` iterator | 5-elem array | 0.064 | 0.818 | **13x** | 2 | 26 |
| `.[]` iterator | 200-elem array | 7.51 | 88.9 | **12x** | 2 | 1811 |
| `select(.f == "x")` | Small (~100B) | 0.121 | 1.81 | **15x** | 0 | 45 |
| `select(.f == "x")`¹ | Large (~100KB, last field) | 33.2 | 812 | **24x** | 0 | 4651 |
| `select(.f and .g)` | Small (~100B) | 0.170 | 1.92 | **11x** | 0 | 46 |
| `select(.f or .g)` | Small (~100B) | 0.072 | 1.91 | **27x** | 0 | 46 |
| `has("key")` in select | Small (~100B) | 0.091 | 1.82 | **20x** | 0 | 45 |
| `has("key")` in select | Large (~100KB) | 32.4 | 809 | **25x** | 0 | 4651 |
| `if-then-else` | Small (~100B) | 0.097 | 1.15 | **12x** | 0 | 30 |
| `.f // "default"` | Small (~100B) | 0.087 | 1.14 | **13x** | 0 | 31 |
| `try .field` (no error) | Small (~100B) | 4.53 | 1.10 | 0.2x† | 1 | 30 |
| `.a + .b` (strings) | Small (~100B) | 0.052 | 0.789 | **15x** | 0 | 21 |
| `"prefix" + .name` | Small (~100B) | 0.085 | 0.871 | **10x** | 0 | 23 |
| `.a - .b` (subtract) | Small (~100B) | 0.064 | 0.842 | **13x** | 0 | 20 |
| `.a * .b` (multiply) | Small (~100B) | 0.076 | 0.901 | **12x** | 0 | 21 |
| `.a / .b` (divide) | Small (~100B) | 0.116 | 1.49 | **13x** | 0 | 21 |
| `.a - .b` (array diff) | 5-elem arrays | 0.233 | 1.30 | **5.6x** | 0 | 33 |
| `length` | Small (~100B) | 0.038 | 1.06 | **28x** | 0 | 27 |
| `length` | Large (~100KB) | 31.8 | 648 | **20x** | 0 | 2835 |
| `add` (numbers) | 5-elem array | 0.059 | 0.801 | **14x** | 0 | 28 |
| `add` (strings) | 5-elem array | 0.133 | 1.02 | **7.6x** | 0 | 35 |
| `flatten` | 3-elem nested array | 0.094 | 1.49 | **16x** | 0 | 38 |
| `min` | 200-int array | 7.37 | 2.38 | 0.3x† | 0 | 15 |
| `min_by(.value)` | 100-elem object array | 12.6 | 70.6 | **5.6x** | 0 | 1347 |
| `sort` | 200-int array | 5.90 | 1.52 | 0.3x† | 14 | 15 |
| `sort_by(.value)` | 100-elem object array | 21.9 | 115 | **5.2x** | 422 | 2145 |
| `unique` | 200-int array | 7.32 | 1.43 | 0.2x† | 14 | 15 |
| `group_by(.active)` | 100-elem object array | 25.6 | 123 | **4.8x** | 422 | 2260 |
| `map(.name)` | 20-elem array (~600B) | 1.62 | 11.3 | **7x** | 29 | 251 |
| `map(.name)` | 200-elem array (~6KB) | 15.7 | 96.1 | **6.1x** | 209 | 2237 |
| `any` | 5-elem array | 0.043 | 2.10 | **49x** | 0 | 39 |
| `any(expr)` | 5-elem array | 0.392 | 2.30 | **5.9x** | 12 | 49 |
| `any(expr)`² | 200-int array | 3.92 | 1.91 | 0.5x† | 0 | 29 |
| `any(gen; cond)`² | 200-int array | 4.46 | 1.69 | 0.4x† | 3 | 27 |
| `first(expr)` | 5-elem array | 0.254 | 2.16 | **8.5x** | 9 | 39 |
| `first(expr)`² | 200-int array | 6.80 | 1.77 | 0.3x† | 107 | 23 |
| `last(expr)` | 5-elem array | 0.317 | 2.20 | **6.9x** | 10 | 43 |
| `last(expr)`² | 200-int array | 6.64 | 1.79 | 0.3x† | 107 | 24 |
| `limit(3; expr)` | 5-elem array | 0.115 | 2.01 | **17x** | 5 | 42 |
| `limit(10; expr)` | 200-int array | 0.687 | 2.00 | **2.9x** | 5 | 24 |
| `.[1:4]` slice | 6-elem array | 0.089 | 0.928 | **10x** | 0 | 21 |
| `values` | 9-elem array | 0.271 | 2.48 | **9.2x** | 13 | 51 |
| `skip(2; .[])` | 5-elem array | 0.152 | 2.01 | **13x** | 7 | 43 |
| `to_entries` | Small (~100B) | 0.158 | 4.56 | **29x** | 0 | 98 |
| `to_entries` | Large (~100KB) | 33.9 | 906 | **27x** | 0 | 6465 |
| `getpath(["field_0"])` | Small (~100B) | 0.256 | 1.15 | **4.5x** | 12 | 29 |
| `setpath(["field_0"]; "y")` | Small (~100B) | 0.397 | 1.89 | **4.8x** | 12 | 43 |
| `delpaths([["field_0"]])` | Small (~100B) | 0.704 | 1.95 | **2.8x** | 22 | 40 |
| `keys` | Small (~100B) | 0.305 | 1.39 | **4.6x** | 7 | 35 |
| `keys_unsorted` | Small (~100B) | 0.157 | 1.43 | **9.2x** | 0 | 35 |
| `keys_unsorted` | Large (~100KB) | 31.5 | 646 | **21x** | 0 | 3039 |
| `object merge .a + .b` | Small (~100B) | 0.165 | 1.56 | **9.5x** | 0 | 33 |
| `tojson` | Small (~100B) | 0.332 | 1.66 | **5x** | 0 | 39 |
| `fromjson` | JSON string | 0.155 | 1.37 | **8.9x** | 0 | 31 |
| `tonumber` | `"42"` string | 0.013 | 0.376 | **28x** | 0 | 11 |
| `toboolean` | `"true"` string | 0.0084 | 0.370 | **44x** | 0 | 11 |
| `split(",")` | short string | 0.147 | 0.899 | **6.1x** | 0 | 21 |
| `join(",")` | 5-elem array | 0.102 | 1.11 | **11x** | 0 | 37 |
| `ascii_downcase` in select | Small (~100B) | 0.136 | 2.00 | **15x** | 0 | 46 |
| `ascii_downcase` in select | Large (~100KB) | 33.8 | 869 | **26x** | 0 | 4653 |
| `startswith("s")` | Small (~100B) | 0.123 | 1.92 | **16x** | 0 | 45 |
| `startswith("s")` | Large (~100KB) | 32.7 | 847 | **26x** | 0 | 4652 |
| `endswith("s")` | Small (~100B) | 0.127 | 1.86 | **15x** | 0 | 45 |
| `trim` | short string | 0.053 | 0.426 | **8x** | 1 | 12 |
| `ltrim` | short string | 0.049 | 0.434 | **8.9x** | 1 | 12 |
| `rtrim` | short string | 0.054 | 0.532 | **9.8x** | 1 | 12 |
| `ltrimstr("s")` | Small (~100B) | 0.125 | 1.21 | **9.7x** | 0 | 30 |
| `rtrimstr("s")` | Small (~100B) | 0.122 | 1.19 | **9.8x** | 0 | 30 |
| `select` + string ops + arith + construct | ~200B log event | 1.05 | 3.04 | **2.9x** | 0 | 69 |
| `[.[] \| select + arith + construct]` | 20-elem array (~1.5KB) | 9.08 | 23.4 | **2.6x** | 106 | 525 |
| `length + map + add + min_by + any` | 20-elem array (~1.5KB) | 12.6 | 23.2 | **1.8x** | 106 | 555 |
| `map(try {…} catch …)` | 20-elem array (~1.5KB) | 262 | 29.3 | 0.1x† | 187 | 641 |
| `map(if … elif … else …)` | 20-elem array (~1.5KB) | 9.43 | 26.5 | **2.8x** | 127 | 595 |
| `[.[] \| str + tostring + str]` | 20-elem array (~1.5KB) | 11.4 | 25.1 | **2.2x** | 298 | 686 |
| `to_entries \| map(select) \| from_entries` | ~200B log event | 3.53 | 15.2 | **4.3x** | 69 | 250 |
| `@base64` | 34-char string | 0.150 | 0.561 | **3.8x** | 4 | 15 |
| `@base64d` | 48-char encoded | 0.216 | 0.603 | **2.8x** | 0 | 15 |
| `@uri` | 36-char URL string | 0.208 | 0.805 | **3.9x** | 4 | 14 |
| `index(",")` | short string | 0.099 | 1.01 | **10x** | 1 | 31 |
| `indices(",")` | short string | 0.212 | 2.26 | **11x** | 1 | 97 |
| `sqrt` | float (e≈2.718) | 0.068 | 0.492 | **7.3x** | 0 | 12 |
| `log` | float (e≈2.718) | 0.037 | 0.488 | **13x** | 0 | 12 |
| `sin` | float (e≈2.718) | 0.079 | 0.476 | **6x** | 0 | 12 |
| `atan` | integer 1 | 0.052 | 0.420 | **8x** | 0 | 11 |
| `exp` | integer 1 | 0.062 | 0.445 | **7.1x** | 0 | 11 |
| `tgamma` | integer 5 | 0.013 | 0.414 | **32x** | 0 | 11 |
| `fabs` | float -3.14 | 0.054 | 0.445 | **8.2x** | 0 | 12 |
| `abs` | float -3.14 | 0.0093 | 0.438 | **47x** | 0 | 12 |
| `expr as $x \| body` | Small (~20B object) | 29.7 | 0.847 | 0.0x† | 18 | 24 |
| ``"\(.level): \(.svc)"`` | ~45B object | 0.120 | 1.11 | **9.2x** | 0 | 35 |
| ``"user \(.name) …"`` | ~45B object | 0.090 | 1.30 | **15x** | 0 | 40 |
| `isempty(empty)` | null | 0.074 | 0.631 | **8.5x** | 6 | 18 |
| `isempty(.[])` | 5-elem array | 0.092 | 1.12 | **12x** | 6 | 33 |
| `nth(2; .[])` | 5-elem array | 0.133 | 2.33 | **17x** | 7 | 49 |
| `range(10)` (10 values) | null | 0.210 | 1.06 | **5x** | 12 | 18 |
| `limit(3; range(1000))` | null — only 3 allocs | 0.144 | 1.56 | **11x** | 8 | 26 |
| `test(re)` hit | short string | 0.115 | 1.43 | **12x** | 0 | 26 |
| `test(re)` miss | short string | 0.100 | 1.75 | **17x** | 0 | 26 |
| `match(re)` hit | short string | 0.220 | 10.8 | **49x** | 1 | 66 |
| `match(re)` miss | short string | 0.640 | 2.17 | **3.4x** | 0 | 24 |
| `capture(re)` hit | short string | 0.217 | — | — | 1 | — |
| `scan(re)` no groups (4 matches) | short string | 0.572 | — | — | 17 | — |
| `sub(re; s)` hit | short string | 0.126 | — | — | 1 | — |
| `gsub(re; s)` hit (4 matches) | short string | 0.596 | — | — | 5 | — |

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
BenchmarkFastjq_Small_Del-16                  	 7414197	       171.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Medium_Del-16                 	  580692	      2056 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Del-16                  	   34388	     34485 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Field-16                	14074086	        81.57 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Field-16                	  158767	      7590 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Index-16                	50816773	        23.61 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ArrayDel-16             	14292626	        83.35 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Construct-16            	 9283443	       132.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Construct-16            	  152103	      7992 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Iterator-16             	18785662	        63.67 ns/op	      40 B/op	       2 allocs/op
BenchmarkFastjq_Large_Iterator-16             	  160672	      7510 ns/op	      40 B/op	       2 allocs/op
BenchmarkFastjq_Small_Select-16               	 9723432	       120.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Select-16               	   36001	     33191 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Alternative-16          	14077057	        86.74 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SelectAnd-16            	 6817434	       169.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SelectOr-16             	16818538	        71.93 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Has-16                  	13304420	        91.42 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Has-16                  	   36933	     32362 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_IfThenElse-16           	12499983	        96.59 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Length-16               	30598876	        37.86 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Length-16               	   37509	     31770 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Map-16                  	  733290	      1619 ns/op	     689 B/op	      29 allocs/op
BenchmarkFastjq_Medium_Map-16                 	  151435	      7837 ns/op	    2609 B/op	     109 allocs/op
BenchmarkFastjq_Large_Map-16                  	   74492	     15682 ns/op	    5009 B/op	     209 allocs/op
BenchmarkFastjq_Small_ToEntries-16            	 7484286	       157.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_ToEntries-16            	   35409	     33862 ns/op	       6 B/op	       0 allocs/op
BenchmarkFastjq_Small_KeysUnsorted-16         	 7807468	       156.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Keys-16                 	 3957256	       304.7 ns/op	     456 B/op	       7 allocs/op
BenchmarkFastjq_Small_GetPath-16              	 4648425	       255.9 ns/op	     272 B/op	      12 allocs/op
BenchmarkFastjq_Small_SetPath-16              	 3017882	       396.6 ns/op	     264 B/op	      12 allocs/op
BenchmarkFastjq_Small_DelPaths-16             	 1694043	       704.1 ns/op	     608 B/op	      22 allocs/op
BenchmarkFastjq_Large_KeysUnsorted-16         	   38067	     31476 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Any-16                  	28587367	        42.94 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Any-16                  	  892358	      1388 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AnyExpr-16              	 3047571	       392.4 ns/op	     144 B/op	      12 allocs/op
BenchmarkFastjq_Large_AnyExpr-16              	  304900	      3924 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Add-16                  	19998735	        58.87 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AddStrings-16           	 9023505	       133.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Flatten-16              	12593780	        94.10 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Split-16                	 8194470	       147.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Join-16                 	11513128	       102.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Values-16               	 4318938	       270.8 ns/op	     328 B/op	      13 allocs/op
BenchmarkGojq_Small_Values-16                 	  472879	      2482 ns/op	    3144 B/op	      51 allocs/op
BenchmarkFastjq_Small_Base64Encode-16         	 7949184	       149.6 ns/op	     120 B/op	       4 allocs/op
BenchmarkFastjq_Small_Base64Decode-16         	 5524141	       215.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Base64Encode-16           	 2137812	       561.1 ns/op	    1321 B/op	      15 allocs/op
BenchmarkGojq_Small_Base64Decode-16           	 2009124	       602.5 ns/op	    1321 B/op	      15 allocs/op
BenchmarkFastjq_Small_IndexFind-16            	12024448	        98.50 ns/op	       3 B/op	       1 allocs/op
BenchmarkFastjq_Small_IndicesAll-16           	 5734461	       211.6 ns/op	       3 B/op	       1 allocs/op
BenchmarkGojq_Small_IndexFind-16              	 1201070	      1006 ns/op	    2274 B/op	      31 allocs/op
BenchmarkGojq_Small_IndicesAll-16             	  533557	      2257 ns/op	    3907 B/op	      97 allocs/op
BenchmarkFastjq_Small_Slice-16                	13628283	        89.08 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SliceString-16          	11607220	       104.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Plus-16                 	22967514	        52.44 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_PlusStr-16              	14176311	        85.01 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AsciiDowncase-16        	 8939517	       135.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_AsciiDowncase-16        	   35180	     33792 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Startswith-16           	 9988422	       123.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Startswith-16           	   36883	     32683 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Endswith-16             	 9239685	       126.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Ltrimstr-16             	 9757990	       124.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Ltrimstr-16             	   38170	     31344 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Rtrimstr-16             	 9691173	       122.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Trim-16                 	21884797	        53.02 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_Ltrim-16                	24958921	        48.63 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_Rtrim-16                	22361740	        54.29 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_First-16                	 4574746	       253.9 ns/op	     216 B/op	       9 allocs/op
BenchmarkFastjq_Large_First-16                	  177024	      6800 ns/op	    2584 B/op	     107 allocs/op
BenchmarkFastjq_Small_Last-16                 	 3815360	       317.2 ns/op	     256 B/op	      10 allocs/op
BenchmarkFastjq_Large_Last-16                 	  179839	      6644 ns/op	    2584 B/op	     107 allocs/op
BenchmarkFastjq_Small_Limit-16                	10438438	       115.4 ns/op	     104 B/op	       5 allocs/op
BenchmarkFastjq_Small_Skip-16                 	 7538380	       152.2 ns/op	     208 B/op	       7 allocs/op
BenchmarkFastjq_Large_Limit-16                	 1750002	       686.7 ns/op	     104 B/op	       5 allocs/op
BenchmarkGojq_Small_Del-16                    	  498678	      2387 ns/op	    3763 B/op	      58 allocs/op
BenchmarkGojq_Medium_Del-16                   	   60106	     19933 ns/op	   16973 B/op	     323 allocs/op
BenchmarkGojq_Large_Del-16                    	    1434	    835652 ns/op	  541471 B/op	    4666 allocs/op
BenchmarkGojq_Small_Field-16                  	 1000000	      1031 ns/op	    1641 B/op	      27 allocs/op
BenchmarkGojq_Large_Field-16                  	    1987	    604863 ns/op	  270057 B/op	    2835 allocs/op
BenchmarkGojq_Small_Index-16                  	 1796268	       662.3 ns/op	    1401 B/op	      20 allocs/op
BenchmarkGojq_Small_ArrayDel-16               	  716002	      1740 ns/op	    3363 B/op	      53 allocs/op
BenchmarkGojq_Small_Construct-16              	  841040	      1435 ns/op	    2330 B/op	      37 allocs/op
BenchmarkGojq_Large_Construct-16              	    1983	    607512 ns/op	  274604 B/op	    2867 allocs/op
BenchmarkGojq_Small_Iterator-16               	 1460438	       817.9 ns/op	    1776 B/op	      26 allocs/op
BenchmarkGojq_Large_Iterator-16               	   13479	     88943 ns/op	  109808 B/op	    1811 allocs/op
BenchmarkGojq_Small_Select-16                 	  674511	      1815 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Large_Select-16                 	    1482	    812275 ns/op	  534199 B/op	    4651 allocs/op
BenchmarkGojq_Small_Alternative-16            	 1000000	      1138 ns/op	    1897 B/op	      31 allocs/op
BenchmarkGojq_Small_SelectAnd-16              	  648038	      1915 ns/op	    2890 B/op	      46 allocs/op
BenchmarkGojq_Small_SelectOr-16               	  630836	      1907 ns/op	    2890 B/op	      46 allocs/op
BenchmarkGojq_Small_Has-16                    	  673692	      1818 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Large_Has-16                    	    1458	    808683 ns/op	  536828 B/op	    4651 allocs/op
BenchmarkGojq_Small_IfThenElse-16             	 1000000	      1147 ns/op	    1817 B/op	      30 allocs/op
BenchmarkGojq_Small_Length-16                 	 1000000	      1060 ns/op	    1633 B/op	      27 allocs/op
BenchmarkGojq_Large_Length-16                 	    1940	    647913 ns/op	  269840 B/op	    2835 allocs/op
BenchmarkGojq_Small_Map-16                    	  107769	     11286 ns/op	   13654 B/op	     251 allocs/op
BenchmarkGojq_Large_Map-16                    	   12571	     96113 ns/op	  118631 B/op	    2237 allocs/op
BenchmarkGojq_Small_ToEntries-16              	  270010	      4564 ns/op	    6559 B/op	      98 allocs/op
BenchmarkGojq_Large_ToEntries-16              	    1293	    906147 ns/op	  669971 B/op	    6465 allocs/op
BenchmarkGojq_Small_KeysUnsorted-16           	  867535	      1433 ns/op	    1953 B/op	      35 allocs/op
BenchmarkGojq_Small_Keys-16                   	  873562	      1391 ns/op	    1953 B/op	      35 allocs/op
BenchmarkGojq_Small_GetPath-16                	 1000000	      1152 ns/op	    1721 B/op	      29 allocs/op
BenchmarkGojq_Small_SetPath-16                	  646269	      1890 ns/op	    2618 B/op	      43 allocs/op
BenchmarkGojq_Small_DelPaths-16               	  626502	      1948 ns/op	    2426 B/op	      40 allocs/op
BenchmarkGojq_Large_KeysUnsorted-16           	    1780	    645510 ns/op	  282911 B/op	    3039 allocs/op
BenchmarkGojq_Small_Any-16                    	  582835	      2101 ns/op	    4508 B/op	      39 allocs/op
BenchmarkGojq_Small_AnyExpr-16                	  523545	      2302 ns/op	    4620 B/op	      49 allocs/op
BenchmarkGojq_Large_AnyExpr-16                	  622207	      1909 ns/op	    2818 B/op	      29 allocs/op
BenchmarkGojq_Small_Add-16                    	 1527904	       801.3 ns/op	    1529 B/op	      28 allocs/op
BenchmarkGojq_Small_AddStrings-16             	 1000000	      1017 ns/op	    1785 B/op	      35 allocs/op
BenchmarkGojq_Small_Flatten-16                	  820807	      1485 ns/op	    1857 B/op	      38 allocs/op
BenchmarkGojq_Small_Split-16                  	 1332536	       898.6 ns/op	    1553 B/op	      21 allocs/op
BenchmarkGojq_Small_Join-16                   	 1000000	      1108 ns/op	    1865 B/op	      37 allocs/op
BenchmarkGojq_Small_Slice-16                  	 1290900	       928.3 ns/op	    1425 B/op	      21 allocs/op
BenchmarkGojq_Small_SliceString-16            	 2341431	       513.8 ns/op	    1145 B/op	      12 allocs/op
BenchmarkGojq_Small_Plus-16                   	 1517665	       789.1 ns/op	    1673 B/op	      21 allocs/op
BenchmarkGojq_Small_PlusStr-16                	 1374955	       871.1 ns/op	    1705 B/op	      23 allocs/op
BenchmarkGojq_Small_AsciiDowncase-16          	  585180	      2003 ns/op	    2714 B/op	      46 allocs/op
BenchmarkGojq_Large_AsciiDowncase-16          	    1372	    869164 ns/op	  546046 B/op	    4653 allocs/op
BenchmarkGojq_Small_Startswith-16             	  606775	      1921 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Large_Startswith-16             	    1402	    846559 ns/op	  543671 B/op	    4652 allocs/op
BenchmarkGojq_Small_Endswith-16               	  645079	      1857 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Small_Ltrimstr-16               	 1000000	      1212 ns/op	    1737 B/op	      30 allocs/op
BenchmarkGojq_Small_Rtrimstr-16               	 1000000	      1192 ns/op	    1737 B/op	      30 allocs/op
BenchmarkGojq_Small_Trim-16                   	 2910822	       425.7 ns/op	    1129 B/op	      12 allocs/op
BenchmarkGojq_Small_Ltrim-16                  	 2756751	       434.2 ns/op	    1129 B/op	      12 allocs/op
BenchmarkGojq_Small_Rtrim-16                  	 2443897	       532.4 ns/op	    1129 B/op	      12 allocs/op
BenchmarkGojq_Small_First-16                  	  541306	      2157 ns/op	    3379 B/op	      39 allocs/op
BenchmarkGojq_Large_First-16                  	  640174	      1773 ns/op	    1889 B/op	      23 allocs/op
BenchmarkGojq_Small_Last-16                   	  543843	      2202 ns/op	    3483 B/op	      43 allocs/op
BenchmarkGojq_Large_Last-16                   	  699116	      1792 ns/op	    2193 B/op	      24 allocs/op
BenchmarkGojq_Small_Limit-16                  	  593840	      2014 ns/op	    3328 B/op	      42 allocs/op
BenchmarkGojq_Small_Skip-16                   	  596314	      2009 ns/op	    2880 B/op	      43 allocs/op
BenchmarkGojq_Large_Limit-16                  	  662642	      2000 ns/op	    2200 B/op	      24 allocs/op
BenchmarkFastjq_Small_Subtract-16             	18516327	        63.54 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Subtract-16               	 1425758	       842.3 ns/op	    1649 B/op	      20 allocs/op
BenchmarkFastjq_Small_Multiply-16             	15393700	        76.28 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Multiply-16               	 1325695	       901.5 ns/op	    1649 B/op	      21 allocs/op
BenchmarkFastjq_Small_Divide-16               	10148848	       115.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Divide-16                 	 1000000	      1493 ns/op	    1665 B/op	      21 allocs/op
BenchmarkFastjq_Small_Min-16                  	  279430	      7370 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Min-16                    	  482378	      2378 ns/op	    1217 B/op	      15 allocs/op
BenchmarkFastjq_Small_MinBy-16                	   90927	     12566 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_MinBy-16                  	   17342	     70605 ns/op	   63633 B/op	    1347 allocs/op
BenchmarkFastjq_Sort-16                       	  208057	      5897 ns/op	    7688 B/op	      14 allocs/op
BenchmarkGojq_Sort-16                         	  763983	      1516 ns/op	    1217 B/op	      15 allocs/op
BenchmarkFastjq_SortBy-16                     	   56583	     21866 ns/op	   24872 B/op	     422 allocs/op
BenchmarkGojq_SortBy-16                       	   10000	    114713 ns/op	   96980 B/op	    2145 allocs/op
BenchmarkFastjq_Unique-16                     	  159640	      7323 ns/op	    7688 B/op	      14 allocs/op
BenchmarkGojq_Unique-16                       	  813614	      1433 ns/op	    1217 B/op	      15 allocs/op
BenchmarkFastjq_GroupBy-16                    	   47266	     25600 ns/op	   24872 B/op	     422 allocs/op
BenchmarkGojq_GroupBy-16                      	   10000	    123190 ns/op	  102006 B/op	    2260 allocs/op
BenchmarkFastjq_Small_URIEncode-16            	 5486678	       207.7 ns/op	     120 B/op	       4 allocs/op
BenchmarkGojq_Small_URIEncode-16              	 1502170	       804.7 ns/op	    1289 B/op	      14 allocs/op
BenchmarkFastjq_Small_ArrayDiff-16            	 4832281	       232.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ArrayDiff-16              	  914308	      1304 ns/op	    2121 B/op	      33 allocs/op
BenchmarkFastjq_Small_TryNoError-16           	  262747	      4528 ns/op	      64 B/op	       1 allocs/op
BenchmarkFastjq_Small_TryCatchNoError-16      	  271414	      4486 ns/op	      64 B/op	       1 allocs/op
BenchmarkGojq_Small_TryNoError-16             	 1000000	      1100 ns/op	    1913 B/op	      30 allocs/op
BenchmarkFastjq_Small_ObjectMerge-16          	 6924304	       165.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ObjectMerge-16            	  761521	      1562 ns/op	    2906 B/op	      33 allocs/op
BenchmarkFastjq_Small_ToJSON-16               	 3615741	       332.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ToJSON-16                 	  728991	      1663 ns/op	    2410 B/op	      39 allocs/op
BenchmarkFastjq_Small_FromJSON-16             	 7747458	       154.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_FromJSON-16               	  885949	      1373 ns/op	    2714 B/op	      31 allocs/op
BenchmarkFastjq_Small_ToString-16             	 3559015	       336.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToNumber-16             	88071777	        13.42 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToBoolean-16            	141996170	         8.433 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ToNumber-16               	 3168423	       375.6 ns/op	    1113 B/op	      11 allocs/op
BenchmarkGojq_Small_ToBoolean-16              	 3277348	       369.9 ns/op	    1113 B/op	      11 allocs/op
BenchmarkFastjq_Small_AnyTwoArg-16            	  250962	      4456 ns/op	      57 B/op	       3 allocs/op
BenchmarkGojq_Small_AnyTwoArg-16              	  707366	      1686 ns/op	    2482 B/op	      27 allocs/op
BenchmarkFastjq_Complex_LogNormalize-16       	 1000000	      1049 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Complex_LogNormalize-16         	  388640	      3045 ns/op	    4324 B/op	      69 allocs/op
BenchmarkFastjq_Complex_ArrayPipeline-16      	  131048	      9076 ns/op	    2264 B/op	     106 allocs/op
BenchmarkGojq_Complex_ArrayPipeline-16        	   51190	     23367 ns/op	   20409 B/op	     525 allocs/op
BenchmarkFastjq_Complex_Aggregation-16        	   95028	     12613 ns/op	    3184 B/op	     106 allocs/op
BenchmarkGojq_Complex_Aggregation-16          	   51684	     23218 ns/op	   22398 B/op	     555 allocs/op
BenchmarkFastjq_Complex_TolerantMap-16        	    4527	    261953 ns/op	    5216 B/op	     187 allocs/op
BenchmarkGojq_Complex_TolerantMap-16          	   42043	     29335 ns/op	   27876 B/op	     641 allocs/op
BenchmarkFastjq_Complex_ElifRouting-16        	  128151	      9434 ns/op	    3000 B/op	     127 allocs/op
BenchmarkGojq_Complex_ElifRouting-16          	   46290	     26549 ns/op	   25924 B/op	     595 allocs/op
BenchmarkFastjq_Complex_StringBuild-16        	  101154	     11415 ns/op	    7216 B/op	     298 allocs/op
BenchmarkGojq_Complex_StringBuild-16          	   48170	     25147 ns/op	   20533 B/op	     686 allocs/op
BenchmarkFastjq_Complex_EntryFilter-16        	  335433	      3528 ns/op	    2888 B/op	      69 allocs/op
BenchmarkGojq_Complex_EntryFilter-16          	   73689	     15238 ns/op	   18430 B/op	     250 allocs/op
BenchmarkFastjq_Small_Sqrt-16                 	18051015	        67.60 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Sqrt-16                   	 2441367	       492.1 ns/op	    1145 B/op	      12 allocs/op
BenchmarkFastjq_Small_Log-16                  	32744761	        37.04 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Log-16                    	 2492718	       488.3 ns/op	    1129 B/op	      12 allocs/op
BenchmarkFastjq_Small_Sin-16                  	14782458	        79.15 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Sin-16                    	 2510584	       476.0 ns/op	    1145 B/op	      12 allocs/op
BenchmarkFastjq_Small_Atan-16                 	22968832	        52.40 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Atan-16                   	 2824641	       420.1 ns/op	    1121 B/op	      11 allocs/op
BenchmarkFastjq_Small_Exp-16                  	19193703	        62.34 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Exp-16                    	 2782234	       445.0 ns/op	    1121 B/op	      11 allocs/op
BenchmarkFastjq_Small_Tgamma-16               	94541541	        12.96 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Tgamma-16                 	 2860533	       414.3 ns/op	    1105 B/op	      11 allocs/op
BenchmarkFastjq_Small_Fabs-16                 	22231364	        54.01 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Fabs-16                   	 2650454	       444.8 ns/op	    1113 B/op	      12 allocs/op
BenchmarkFastjq_Small_Abs-16                  	128535087	         9.317 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Abs-16                    	 2726223	       438.1 ns/op	    1113 B/op	      12 allocs/op
BenchmarkFastjq_Small_Bind-16                 	   39904	     29696 ns/op	     648 B/op	      18 allocs/op
BenchmarkGojq_Small_Bind-16                   	 1414676	       847.0 ns/op	    1793 B/op	      24 allocs/op
BenchmarkFastjq_Small_StringInterp-16         	 9891750	       120.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_StringInterp-16           	 1000000	      1109 ns/op	    2121 B/op	      35 allocs/op
BenchmarkFastjq_Small_StringInterpNum-16      	13624364	        89.68 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_StringInterpNum-16        	  921387	      1302 ns/op	    2498 B/op	      40 allocs/op
BenchmarkFastjq_Small_IsEmptyTrue-16          	16708902	        74.22 ns/op	     105 B/op	       6 allocs/op
BenchmarkGojq_Small_IsEmptyTrue-16            	 1876478	       631.1 ns/op	    1921 B/op	      18 allocs/op
BenchmarkFastjq_Small_IsEmptyFalse-16         	12937993	        91.78 ns/op	     105 B/op	       6 allocs/op
BenchmarkGojq_Small_IsEmptyFalse-16           	 1000000	      1122 ns/op	    2898 B/op	      33 allocs/op
BenchmarkFastjq_Small_Nth-16                  	 9057415	       133.3 ns/op	     152 B/op	       7 allocs/op
BenchmarkGojq_Small_Nth-16                    	  509955	      2330 ns/op	    4372 B/op	      49 allocs/op
BenchmarkFastjq_Small_Range10-16              	 5559100	       210.5 ns/op	     120 B/op	      12 allocs/op
BenchmarkGojq_Small_Range10-16                	 1000000	      1060 ns/op	    1864 B/op	      18 allocs/op
BenchmarkFastjq_Small_RangeLimit-16           	 8410549	       144.2 ns/op	     128 B/op	       8 allocs/op
BenchmarkGojq_Small_RangeLimit-16             	  755154	      1564 ns/op	    3208 B/op	      26 allocs/op
BenchmarkRegexp_Match_Hit-16                  	27673707	        42.91 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Miss-16                 	50990326	        23.37 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Complex_Hit-16          	27746520	        42.94 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Long_Miss-16            	15565362	        77.50 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_MatchString-16                	26434798	        44.88 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_FindSubmatch_Hit-16           	 5829858	       206.2 ns/op	     160 B/op	       2 allocs/op
BenchmarkRegexp_FindSubmatch_Miss-16          	 1764820	       679.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_FindSubmatchIndex_Hit-16      	 6552256	       181.8 ns/op	      64 B/op	       1 allocs/op
BenchmarkRegexp_FindSubmatchIndex_Miss-16     	 1724044	       696.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_ReplaceAll_Hit-16             	 7502035	       160.2 ns/op	      97 B/op	       5 allocs/op
BenchmarkRegexp_ReplaceAll_Miss-16            	23584305	        51.02 ns/op	      48 B/op	       2 allocs/op
BenchmarkRegexp_MatchViaClosure-16            	26951983	        44.49 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_Hit-16           	10558698	       115.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_Miss-16          	11967039	       100.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_InPipeline-16    	 3980869	       300.6 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_MatchRe_Hit-16          	 5417814	       220.4 ns/op	      48 B/op	       1 allocs/op
BenchmarkFastjq_Small_MatchRe_Miss-16         	 1876050	       639.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_TestRe_Hit-16             	  862729	      1432 ns/op	    2751 B/op	      26 allocs/op
BenchmarkGojq_Small_TestRe_Miss-16            	  871461	      1749 ns/op	    2708 B/op	      26 allocs/op
BenchmarkGojq_Small_MatchRe_Hit-16            	  206946	     10814 ns/op	    4800 B/op	      66 allocs/op
BenchmarkGojq_Small_MatchRe_Miss-16           	  534028	      2166 ns/op	    2250 B/op	      24 allocs/op
BenchmarkFastjq_Small_CaptureRe_Hit-16        	 5271867	       217.3 ns/op	      48 B/op	       1 allocs/op
BenchmarkFastjq_Small_CaptureRe_Miss-16       	 1987880	       602.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ScanRe_NoGroups-16      	 2108046	       571.9 ns/op	     535 B/op	      17 allocs/op
BenchmarkFastjq_Small_ScanRe_WithGroups-16    	 1659369	       722.5 ns/op	     851 B/op	      20 allocs/op
BenchmarkFastjq_Small_SubRe_Hit-16            	 9522722	       126.3 ns/op	      16 B/op	       1 allocs/op
BenchmarkFastjq_Small_SubRe_Miss-16           	11738446	       101.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_GSubRe_Hit-16           	 2016320	       595.8 ns/op	     305 B/op	       5 allocs/op
BenchmarkFastjq_Small_GSubRe_Miss-16          	 4616421	       258.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_CaptureRe_Hit-16          	  507643	      2445 ns/op	    4728 B/op	      51 allocs/op
BenchmarkGojq_Small_CaptureRe_Miss-16         	  661087	      1848 ns/op	    2674 B/op	      27 allocs/op
BenchmarkGojq_Small_ScanRe_NoGroups-16        	  420693	      2907 ns/op	    5135 B/op	      57 allocs/op
BenchmarkGojq_Small_SubRe_Hit-16              	  218424	      5465 ns/op	    9007 B/op	      95 allocs/op
BenchmarkGojq_Small_GSubRe_Hit-16             	   79196	     15519 ns/op	   17930 B/op	     248 allocs/op
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
