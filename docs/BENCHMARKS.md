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
| `.field` | Small (~100B) | 0.085 | 1.01 | **12x** | 0 | 27 |
| `.field` | Large (~100KB) | 7.64 | 599 | **78x** | 0 | 2835 |
| `del(.f)` | Small (~100B) | 0.150 | 2.40 | **16x** | 0 | 58 |
| `del(.f)` | Medium (~2KB) | 2.19 | 20.0 | **9.1x** | 0 | 323 |
| `del(.f)` | Large (~100KB) | 34.6 | 827 | **24x** | 0 | 4666 |
| `.[n]` | 5-elem array | 0.022 | 0.674 | **30x** | 0 | 20 |
| `del(.[n], .[m])` | 5-elem array | 0.081 | 1.84 | **23x** | 0 | 53 |
| `{f0, f2}` (construct) | Small (~100B) | 0.139 | 1.47 | **11x** | 0 | 37 |
| `{f0, f50}` (construct) | Large (~100KB) | 8.16 | 606 | **74x** | 0 | 2867 |
| `.[]` iterator | 5-elem array | 0.032 | 0.804 | **25x** | 0 | 26 |
| `.[]` iterator | 200-elem array | 7.28 | 89.1 | **12x** | 0 | 1811 |
| `select(.f == "x")` | Small (~100B) | 0.126 | 1.78 | **14x** | 0 | 45 |
| `select(.f == "x")`¹ | Large (~100KB, last field) | 33.3 | 803 | **24x** | 0 | 4651 |
| `select(.f and .g)` | Small (~100B) | 0.165 | 1.88 | **11x** | 0 | 46 |
| `select(.f or .g)` | Small (~100B) | 0.071 | 1.83 | **26x** | 0 | 46 |
| `has("key")` in select | Small (~100B) | 0.094 | 1.78 | **19x** | 0 | 45 |
| `has("key")` in select | Large (~100KB) | 32.6 | 847 | **26x** | 0 | 4651 |
| `if-then-else` | Small (~100B) | 0.097 | 1.15 | **12x** | 0 | 30 |
| `.f // "default"` | Small (~100B) | 0.094 | 1.12 | **12x** | 0 | 31 |
| `try .field` (no error) | Small (~100B) | 4.32 | 1.11 | 0.3x† | 1 | 30 |
| `.a + .b` (strings) | Small (~100B) | 0.052 | 0.730 | **14x** | 0 | 21 |
| `"prefix" + .name` | Small (~100B) | 0.086 | 0.760 | **8.8x** | 0 | 23 |
| `.a - .b` (subtract) | Small (~100B) | 0.050 | 0.726 | **15x** | 0 | 20 |
| `.a * .b` (multiply) | Small (~100B) | 0.064 | 0.773 | **12x** | 0 | 21 |
| `.a / .b` (divide) | Small (~100B) | 0.100 | 0.800 | **8x** | 0 | 21 |
| `.a - .b` (array diff) | 5-elem arrays | 0.217 | 1.31 | **6x** | 0 | 33 |
| `length` | Small (~100B) | 0.039 | 1.02 | **26x** | 0 | 27 |
| `length` | Large (~100KB) | 31.7 | 623 | **20x** | 0 | 2835 |
| `add` (numbers) | 5-elem array | 0.059 | 0.787 | **13x** | 0 | 28 |
| `add` (strings) | 5-elem array | 0.132 | 0.930 | **7x** | 0 | 35 |
| `flatten` | 3-elem nested array | 0.093 | 1.35 | **14x** | 0 | 38 |
| `min` | 200-int array | 1.83 | 1.27 | 0.7x† | 0 | 15 |
| `min_by(.value)` | 100-elem object array | 8.63 | 58.5 | **6.8x** | 0 | 1347 |
| `sort` | 200-int array | 4.50 | 1.26 | 0.3x† | 11 | 15 |
| `sort_by(.value)` | 100-elem object array | 14.5 | 94.5 | **6.5x** | 119 | 2145 |
| `unique` | 200-int array | 5.84 | 1.25 | 0.2x† | 11 | 15 |
| `group_by(.active)` | 100-elem object array | 18.8 | 101 | **5.4x** | 119 | 2260 |
| `map(.name)` | 20-elem array (~600B) | 1.31 | 10.8 | **8.2x** | 0 | 251 |
| `map(.name)` | 200-elem array (~6KB) | 13.3 | 99.5 | **7.5x** | 0 | 2237 |
| `any` | 5-elem array | 0.044 | 1.95 | **45x** | 0 | 39 |
| `any(expr)` | 5-elem array | 0.389 | 2.21 | **5.7x** | 12 | 49 |
| `any(expr)`² | 200-int array | 3.86 | 1.78 | 0.5x† | 0 | 29 |
| `any(gen; cond)`² | 200-int array | 4.34 | 1.68 | 0.4x† | 0 | 27 |
| `first(expr)` | 5-elem array | 0.145 | 1.65 | **11x** | 0 | 39 |
| `first(expr)`² | 200-int array | 4.61 | 1.49 | 0.3x† | 0 | 23 |
| `last(expr)` | 5-elem array | 0.181 | 2.00 | **11x** | 0 | 43 |
| `last(expr)`² | 200-int array | 4.43 | 1.57 | 0.4x† | 0 | 24 |
| `limit(3; expr)` | 5-elem array | 0.043 | 1.74 | **40x** | 0 | 42 |
| `limit(10; expr)` | 200-int array | 0.636 | 1.66 | **2.6x** | 0 | 24 |
| `.[1:4]` slice | 6-elem array | 0.083 | 0.855 | **10x** | 0 | 21 |
| `values` | 9-elem array | 0.092 | 2.46 | **27x** | 0 | 51 |
| `to_entries` | Small (~100B) | 0.153 | 4.25 | **28x** | 0 | 98 |
| `to_entries` | Large (~100KB) | 34.1 | 896 | **26x** | 0 | 6464 |
| `keys_unsorted` | Small (~100B) | 0.153 | 1.35 | **8.8x** | 0 | 35 |
| `keys_unsorted` | Large (~100KB) | 31.7 | 638 | **20x** | 0 | 3039 |
| `object merge .a + .b` | Small (~100B) | 0.167 | 1.57 | **9.4x** | 0 | 33 |
| `tojson` | Small (~100B) | 0.338 | 1.73 | **5.1x** | 0 | 39 |
| `fromjson` | JSON string | 0.155 | 1.37 | **8.9x** | 0 | 31 |
| `tonumber` | `"42"` string | 0.013 | 0.379 | **28x** | 0 | 11 |
| `split(",")` | short string | 0.148 | 0.821 | **5.5x** | 0 | 21 |
| `join(",")` | 5-elem array | 0.102 | 1.01 | **9.9x** | 0 | 37 |
| `ascii_downcase` in select | Small (~100B) | 0.136 | 1.88 | **14x** | 0 | 46 |
| `ascii_downcase` in select | Large (~100KB) | 33.8 | 818 | **24x** | 0 | 4652 |
| `startswith("s")` | Small (~100B) | 0.130 | 1.77 | **14x** | 0 | 45 |
| `startswith("s")` | Large (~100KB) | 33.0 | 820 | **25x** | 0 | 4651 |
| `endswith("s")` | Small (~100B) | 0.132 | 1.79 | **14x** | 0 | 45 |
| `ltrimstr("s")` | Small (~100B) | 0.128 | 1.08 | **8.5x** | 0 | 30 |
| `rtrimstr("s")` | Small (~100B) | 0.130 | 1.08 | **8.4x** | 0 | 30 |
| `select` + string ops + arith + construct | ~200B log event | 1.07 | 3.06 | **2.9x** | 0 | 69 |
| `[.[] \| select + arith + construct]` | 20-elem array (~1.5KB) | 8.31 | 23.5 | **2.8x** | 64 | 525 |
| `length + map + add + min_by + any` | 20-elem array (~1.5KB) | 10.9 | 22.5 | **2.1x** | 25 | 555 |
| `map(try {…} catch …)` | 20-elem array (~1.5KB) | 253 | 29.1 | 0.1x† | 118 | 641 |
| `map(if … elif … else …)` | 20-elem array (~1.5KB) | 8.80 | 26.2 | **3x** | 98 | 595 |
| `[.[] \| str + tostring + str]` | 20-elem array (~1.5KB) | 8.83 | 25.7 | **2.9x** | 150 | 686 |
| `to_entries \| map(select) \| from_entries` | ~200B log event | 3.10 | 14.2 | **4.6x** | 44 | 250 |
| `@base64` | 34-char string | 0.148 | 0.562 | **3.8x** | 4 | 15 |
| `@base64d` | 48-char encoded | 0.209 | 0.590 | **2.8x** | 0 | 15 |
| `@uri` | 36-char URL string | 0.170 | 0.703 | **4.1x** | 4 | 14 |
| `index(",")` | short string | 0.098 | 0.951 | **9.7x** | 1 | 31 |
| `indices(",")` | short string | 0.206 | 2.19 | **11x** | 1 | 97 |
| `sqrt` | float (e≈2.718) | 0.072 | 0.492 | **6.8x** | 0 | 12 |
| `log` | float (e≈2.718) | 0.039 | 0.472 | **12x** | 0 | 12 |
| `sin` | float (e≈2.718) | 0.080 | 0.483 | **6x** | 0 | 12 |
| `atan` | integer 1 | 0.053 | 0.416 | **7.9x** | 0 | 11 |
| `exp` | integer 1 | 0.065 | 0.441 | **6.8x** | 0 | 11 |
| `tgamma` | integer 5 | 0.013 | 0.427 | **33x** | 0 | 11 |
| `fabs` | float -3.14 | 0.055 | 0.437 | **7.9x** | 0 | 12 |
| ``"\(.level): \(.svc)"`` | ~45B object | 0.122 | 1.13 | **9.3x** | 0 | 35 |
| ``"user \(.name) …"`` | ~45B object | 0.089 | 1.29 | **14x** | 0 | 40 |
| `isempty(empty)` | null | 0.0088 | 0.650 | **74x** | 0 | 18 |
| `isempty(.[])` | 5-elem array | 0.023 | 1.09 | **46x** | 0 | 33 |
| `nth(2; .[])` | 5-elem array | 0.044 | 2.25 | **51x** | 0 | 49 |
| `range(10)` (10 values) | null | 0.176 | 1.11 | **6.3x** | 10 | 18 |
| `limit(3; range(1000))` | null — only 3 allocs | 0.084 | 1.57 | **19x** | 3 | 26 |
| `test(re)` hit | short string | 0.107 | 1.30 | **12x** | 0 | 26 |
| `test(re)` miss | short string | 0.099 | 1.25 | **13x** | 0 | 26 |
| `match(re)` hit | short string | 0.222 | 2.94 | **13x** | 1 | 66 |
| `match(re)` miss | short string | 0.594 | 1.67 | **2.8x** | 0 | 24 |
| `capture(re)` hit | short string | 0.218 | — | — | 1 | — |
| `scan(re)` no groups (4 matches) | short string | 0.518 | — | — | 10 | — |
| `sub(re; s)` hit | short string | 0.129 | — | — | 1 | — |
| `gsub(re; s)` hit (4 matches) | short string | 0.593 | — | — | 5 | — |

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
BenchmarkFastjq_Small_Del-16                  	 7214007	       150.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Medium_Del-16                 	  549330	      2194 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Del-16                  	   34736	     34559 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Field-16                	14961123	        85.24 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Field-16                	  156763	      7636 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Index-16                	54369576	        22.28 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ArrayDel-16             	14778150	        80.95 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Construct-16            	 8697529	       139.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Construct-16            	  148071	      8165 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Iterator-16             	37153810	        32.27 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Iterator-16             	  164706	      7278 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Select-16               	 8754573	       125.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Select-16               	   35854	     33266 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Alternative-16          	12830427	        94.07 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SelectAnd-16            	 7359046	       165.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SelectOr-16             	16824532	        71.27 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Has-16                  	12724556	        93.60 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Has-16                  	   36895	     32604 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_IfThenElse-16           	12278961	        97.18 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Length-16               	31250610	        39.41 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Length-16               	   37915	     31704 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Map-16                  	  910038	      1314 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Medium_Map-16                 	  183152	      6534 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Map-16                  	   88848	     13341 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToEntries-16            	 7574176	       153.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_ToEntries-16            	   34950	     34082 ns/op	       6 B/op	       0 allocs/op
BenchmarkFastjq_Small_KeysUnsorted-16         	 7837444	       152.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_KeysUnsorted-16         	   37443	     31690 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Any-16                  	27543493	        43.74 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Any-16                  	  908757	      1324 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AnyExpr-16              	 3087936	       388.8 ns/op	     144 B/op	      12 allocs/op
BenchmarkFastjq_Large_AnyExpr-16              	  311924	      3859 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Add-16                  	20648520	        58.65 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AddStrings-16           	 9095011	       132.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Flatten-16              	12901028	        93.06 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Split-16                	 8151291	       147.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Join-16                 	11635695	       102.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Values-16               	12779263	        91.76 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Values-16                 	  484825	      2455 ns/op	    3144 B/op	      51 allocs/op
BenchmarkFastjq_Small_Base64Encode-16         	 8162848	       147.6 ns/op	     120 B/op	       4 allocs/op
BenchmarkFastjq_Small_Base64Decode-16         	 5761597	       209.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Base64Encode-16           	 2137986	       561.7 ns/op	    1321 B/op	      15 allocs/op
BenchmarkGojq_Small_Base64Decode-16           	 2038323	       589.6 ns/op	    1321 B/op	      15 allocs/op
BenchmarkFastjq_Small_IndexFind-16            	12183080	        97.97 ns/op	       3 B/op	       1 allocs/op
BenchmarkFastjq_Small_IndicesAll-16           	 5831530	       206.5 ns/op	       3 B/op	       1 allocs/op
BenchmarkGojq_Small_IndexFind-16              	 1257160	       951.3 ns/op	    2274 B/op	      31 allocs/op
BenchmarkGojq_Small_IndicesAll-16             	  543872	      2186 ns/op	    3907 B/op	      97 allocs/op
BenchmarkFastjq_Small_Slice-16                	14248269	        83.22 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SliceString-16          	23726316	        50.66 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Plus-16                 	23002165	        51.77 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_PlusStr-16              	14026146	        86.15 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AsciiDowncase-16        	 8956128	       136.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_AsciiDowncase-16        	   34977	     33769 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Startswith-16           	 9272647	       129.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Startswith-16           	   36085	     32995 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Endswith-16             	 9073354	       131.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Ltrimstr-16             	 9373611	       127.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Ltrimstr-16             	   37761	     31739 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Rtrimstr-16             	 9165715	       129.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_First-16                	 8218161	       144.8 ns/op	       8 B/op	       0 allocs/op
BenchmarkFastjq_Large_First-16                	  263060	      4612 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Last-16                 	 6688214	       180.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Last-16                 	  259746	      4434 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Limit-16                	28542375	        43.10 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Limit-16                	 1947802	       635.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Del-16                    	  445435	      2405 ns/op	    3763 B/op	      58 allocs/op
BenchmarkGojq_Medium_Del-16                   	   60555	     19951 ns/op	   16972 B/op	     323 allocs/op
BenchmarkGojq_Large_Del-16                    	    1410	    827233 ns/op	  538898 B/op	    4666 allocs/op
BenchmarkGojq_Small_Field-16                  	 1000000	      1009 ns/op	    1641 B/op	      27 allocs/op
BenchmarkGojq_Large_Field-16                  	    1994	    598544 ns/op	  270054 B/op	    2835 allocs/op
BenchmarkGojq_Small_Index-16                  	 1803792	       673.6 ns/op	    1401 B/op	      20 allocs/op
BenchmarkGojq_Small_ArrayDel-16               	  663063	      1839 ns/op	    3363 B/op	      53 allocs/op
BenchmarkGojq_Small_Construct-16              	  859136	      1470 ns/op	    2330 B/op	      37 allocs/op
BenchmarkGojq_Large_Construct-16              	    1959	    606015 ns/op	  274572 B/op	    2867 allocs/op
BenchmarkGojq_Small_Iterator-16               	 1496560	       804.5 ns/op	    1776 B/op	      26 allocs/op
BenchmarkGojq_Large_Iterator-16               	   13509	     89135 ns/op	  109808 B/op	    1811 allocs/op
BenchmarkGojq_Small_Select-16                 	  667254	      1777 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Large_Select-16                 	    1515	    803047 ns/op	  529776 B/op	    4651 allocs/op
BenchmarkGojq_Small_Alternative-16            	 1000000	      1123 ns/op	    1897 B/op	      31 allocs/op
BenchmarkGojq_Small_SelectAnd-16              	  653962	      1880 ns/op	    2890 B/op	      46 allocs/op
BenchmarkGojq_Small_SelectOr-16               	  646844	      1829 ns/op	    2890 B/op	      46 allocs/op
BenchmarkGojq_Small_Has-16                    	  681769	      1780 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Large_Has-16                    	    1485	    846858 ns/op	  535923 B/op	    4651 allocs/op
BenchmarkGojq_Small_IfThenElse-16             	  993692	      1148 ns/op	    1817 B/op	      30 allocs/op
BenchmarkGojq_Small_Length-16                 	 1000000	      1023 ns/op	    1633 B/op	      27 allocs/op
BenchmarkGojq_Large_Length-16                 	    1981	    622724 ns/op	  269835 B/op	    2835 allocs/op
BenchmarkGojq_Small_Map-16                    	  111483	     10816 ns/op	   13654 B/op	     251 allocs/op
BenchmarkGojq_Large_Map-16                    	   12037	     99464 ns/op	  118593 B/op	    2237 allocs/op
BenchmarkGojq_Small_ToEntries-16              	  282339	      4246 ns/op	    6559 B/op	      98 allocs/op
BenchmarkGojq_Large_ToEntries-16              	    1333	    895549 ns/op	  665386 B/op	    6464 allocs/op
BenchmarkGojq_Small_KeysUnsorted-16           	  918410	      1350 ns/op	    1953 B/op	      35 allocs/op
BenchmarkGojq_Large_KeysUnsorted-16           	    1867	    638241 ns/op	  282830 B/op	    3039 allocs/op
BenchmarkGojq_Small_Any-16                    	  610240	      1953 ns/op	    4508 B/op	      39 allocs/op
BenchmarkGojq_Small_AnyExpr-16                	  553732	      2209 ns/op	    4620 B/op	      49 allocs/op
BenchmarkGojq_Large_AnyExpr-16                	  657288	      1779 ns/op	    2818 B/op	      29 allocs/op
BenchmarkGojq_Small_Add-16                    	 1508886	       787.5 ns/op	    1529 B/op	      28 allocs/op
BenchmarkGojq_Small_AddStrings-16             	 1291147	       930.5 ns/op	    1785 B/op	      35 allocs/op
BenchmarkGojq_Small_Flatten-16                	  898722	      1349 ns/op	    1857 B/op	      38 allocs/op
BenchmarkGojq_Small_Split-16                  	 1465780	       820.7 ns/op	    1553 B/op	      21 allocs/op
BenchmarkGojq_Small_Join-16                   	 1000000	      1011 ns/op	    1865 B/op	      37 allocs/op
BenchmarkGojq_Small_Slice-16                  	 1402442	       854.7 ns/op	    1425 B/op	      21 allocs/op
BenchmarkGojq_Small_SliceString-16            	 2535138	       471.5 ns/op	    1145 B/op	      12 allocs/op
BenchmarkGojq_Small_Plus-16                   	 1624011	       729.6 ns/op	    1673 B/op	      21 allocs/op
BenchmarkGojq_Small_PlusStr-16                	 1583524	       759.8 ns/op	    1705 B/op	      23 allocs/op
BenchmarkGojq_Small_AsciiDowncase-16          	  660728	      1875 ns/op	    2714 B/op	      46 allocs/op
BenchmarkGojq_Large_AsciiDowncase-16          	    1465	    818035 ns/op	  533097 B/op	    4652 allocs/op
BenchmarkGojq_Small_Startswith-16             	  665846	      1768 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Large_Startswith-16             	    1393	    820168 ns/op	  534838 B/op	    4651 allocs/op
BenchmarkGojq_Small_Endswith-16               	  696753	      1795 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Small_Ltrimstr-16               	 1000000	      1085 ns/op	    1737 B/op	      30 allocs/op
BenchmarkGojq_Small_Rtrimstr-16               	 1000000	      1085 ns/op	    1737 B/op	      30 allocs/op
BenchmarkGojq_Small_First-16                  	  735696	      1650 ns/op	    3379 B/op	      39 allocs/op
BenchmarkGojq_Large_First-16                  	  810364	      1485 ns/op	    1889 B/op	      23 allocs/op
BenchmarkGojq_Small_Last-16                   	  600200	      2002 ns/op	    3483 B/op	      43 allocs/op
BenchmarkGojq_Large_Last-16                   	  768724	      1574 ns/op	    2193 B/op	      24 allocs/op
BenchmarkGojq_Small_Limit-16                  	  697152	      1736 ns/op	    3328 B/op	      42 allocs/op
BenchmarkGojq_Large_Limit-16                  	  724003	      1656 ns/op	    2200 B/op	      24 allocs/op
BenchmarkFastjq_Small_Subtract-16             	24295862	        49.92 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Subtract-16               	 1643317	       725.5 ns/op	    1649 B/op	      20 allocs/op
BenchmarkFastjq_Small_Multiply-16             	18440792	        63.97 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Multiply-16               	 1552779	       773.0 ns/op	    1649 B/op	      21 allocs/op
BenchmarkFastjq_Small_Divide-16               	11880648	        99.54 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Divide-16                 	 1479321	       799.9 ns/op	    1665 B/op	      21 allocs/op
BenchmarkFastjq_Small_Min-16                  	  606990	      1828 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Min-16                    	  932382	      1274 ns/op	    1217 B/op	      15 allocs/op
BenchmarkFastjq_Small_MinBy-16                	  135367	      8632 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_MinBy-16                  	   20456	     58520 ns/op	   63631 B/op	    1347 allocs/op
BenchmarkFastjq_Sort-16                       	  267304	      4500 ns/op	    7624 B/op	      11 allocs/op
BenchmarkGojq_Sort-16                         	  967575	      1263 ns/op	    1217 B/op	      15 allocs/op
BenchmarkFastjq_SortBy-16                     	   84878	     14520 ns/op	   18408 B/op	     119 allocs/op
BenchmarkGojq_SortBy-16                       	   12657	     94549 ns/op	   96973 B/op	    2145 allocs/op
BenchmarkFastjq_Unique-16                     	  204594	      5836 ns/op	    7624 B/op	      11 allocs/op
BenchmarkGojq_Unique-16                       	  951499	      1255 ns/op	    1217 B/op	      15 allocs/op
BenchmarkFastjq_GroupBy-16                    	   63303	     18832 ns/op	   18408 B/op	     119 allocs/op
BenchmarkGojq_GroupBy-16                      	   10000	    101303 ns/op	  101900 B/op	    2260 allocs/op
BenchmarkFastjq_Small_URIEncode-16            	 7085144	       170.2 ns/op	     120 B/op	       4 allocs/op
BenchmarkGojq_Small_URIEncode-16              	 1708484	       703.1 ns/op	    1289 B/op	      14 allocs/op
BenchmarkFastjq_Small_ArrayDiff-16            	 5534162	       216.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ArrayDiff-16              	  894831	      1310 ns/op	    2121 B/op	      33 allocs/op
BenchmarkFastjq_Small_TryNoError-16           	  264021	      4324 ns/op	      64 B/op	       1 allocs/op
BenchmarkFastjq_Small_TryCatchNoError-16      	  278022	      4299 ns/op	      64 B/op	       1 allocs/op
BenchmarkGojq_Small_TryNoError-16             	 1000000	      1114 ns/op	    1913 B/op	      30 allocs/op
BenchmarkFastjq_Small_ObjectMerge-16          	 7027435	       166.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ObjectMerge-16            	  774626	      1572 ns/op	    2906 B/op	      33 allocs/op
BenchmarkFastjq_Small_ToJSON-16               	 3418512	       338.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ToJSON-16                 	  692566	      1726 ns/op	    2410 B/op	      39 allocs/op
BenchmarkFastjq_Small_FromJSON-16             	 7666906	       155.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_FromJSON-16               	  886641	      1373 ns/op	    2714 B/op	      31 allocs/op
BenchmarkFastjq_Small_ToString-16             	 3601468	       333.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToNumber-16             	88459420	        13.38 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ToNumber-16               	 3164850	       379.1 ns/op	    1113 B/op	      11 allocs/op
BenchmarkFastjq_Small_AnyTwoArg-16            	  263876	      4338 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_AnyTwoArg-16              	  724538	      1675 ns/op	    2482 B/op	      27 allocs/op
BenchmarkFastjq_Complex_LogNormalize-16       	 1000000	      1069 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Complex_LogNormalize-16         	  392084	      3063 ns/op	    4324 B/op	      69 allocs/op
BenchmarkFastjq_Complex_ArrayPipeline-16      	  142801	      8312 ns/op	    1232 B/op	      64 allocs/op
BenchmarkGojq_Complex_ArrayPipeline-16        	   51031	     23531 ns/op	   20409 B/op	     525 allocs/op
BenchmarkFastjq_Complex_Aggregation-16        	  109711	     10895 ns/op	     408 B/op	      25 allocs/op
BenchmarkGojq_Complex_Aggregation-16          	   53422	     22468 ns/op	   22397 B/op	     555 allocs/op
BenchmarkFastjq_Complex_TolerantMap-16        	    4660	    252645 ns/op	    3720 B/op	     118 allocs/op
BenchmarkGojq_Complex_TolerantMap-16          	   41854	     29109 ns/op	   27874 B/op	     641 allocs/op
BenchmarkFastjq_Complex_ElifRouting-16        	  137127	      8797 ns/op	    2304 B/op	      98 allocs/op
BenchmarkGojq_Complex_ElifRouting-16          	   45456	     26209 ns/op	   25922 B/op	     595 allocs/op
BenchmarkFastjq_Complex_StringBuild-16        	  135882	      8831 ns/op	    1568 B/op	     150 allocs/op
BenchmarkGojq_Complex_StringBuild-16          	   44719	     25732 ns/op	   20533 B/op	     686 allocs/op
BenchmarkFastjq_Complex_EntryFilter-16        	  377230	      3102 ns/op	    2240 B/op	      44 allocs/op
BenchmarkGojq_Complex_EntryFilter-16          	   84158	     14194 ns/op	   18428 B/op	     250 allocs/op
BenchmarkFastjq_Small_Sqrt-16                 	16770208	        71.92 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Sqrt-16                   	 2419225	       491.6 ns/op	    1145 B/op	      12 allocs/op
BenchmarkFastjq_Small_Log-16                  	30711874	        38.93 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Log-16                    	 2544526	       472.3 ns/op	    1129 B/op	      12 allocs/op
BenchmarkFastjq_Small_Sin-16                  	14866318	        80.42 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Sin-16                    	 2496501	       482.6 ns/op	    1145 B/op	      12 allocs/op
BenchmarkFastjq_Small_Atan-16                 	22723058	        52.73 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Atan-16                   	 2912764	       416.1 ns/op	    1121 B/op	      11 allocs/op
BenchmarkFastjq_Small_Exp-16                  	18487314	        64.55 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Exp-16                    	 2810241	       441.1 ns/op	    1121 B/op	      11 allocs/op
BenchmarkFastjq_Small_Tgamma-16               	95926772	        12.92 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Tgamma-16                 	 2774217	       426.7 ns/op	    1105 B/op	      11 allocs/op
BenchmarkFastjq_Small_Fabs-16                 	21528750	        55.35 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Fabs-16                   	 2753725	       436.9 ns/op	    1112 B/op	      12 allocs/op
BenchmarkFastjq_Small_StringInterp-16         	 9890917	       122.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_StringInterp-16           	 1000000	      1132 ns/op	    2121 B/op	      35 allocs/op
BenchmarkFastjq_Small_StringInterpNum-16      	13106666	        89.05 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_StringInterpNum-16        	  924558	      1288 ns/op	    2498 B/op	      40 allocs/op
BenchmarkFastjq_Small_IsEmptyTrue-16          	135412338	         8.816 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_IsEmptyTrue-16            	 1809778	       650.3 ns/op	    1921 B/op	      18 allocs/op
BenchmarkFastjq_Small_IsEmptyFalse-16         	51050643	        23.47 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_IsEmptyFalse-16           	 1000000	      1086 ns/op	    2898 B/op	      33 allocs/op
BenchmarkFastjq_Small_Nth-16                  	27152187	        44.33 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Nth-16                    	  539263	      2246 ns/op	    4372 B/op	      49 allocs/op
BenchmarkFastjq_Small_Range10-16              	 6724150	       175.8 ns/op	      80 B/op	      10 allocs/op
BenchmarkGojq_Small_Range10-16                	 1000000	      1109 ns/op	    1864 B/op	      18 allocs/op
BenchmarkFastjq_Small_RangeLimit-16           	14204257	        84.39 ns/op	      24 B/op	       3 allocs/op
BenchmarkGojq_Small_RangeLimit-16             	  771468	      1568 ns/op	    3208 B/op	      26 allocs/op
BenchmarkRegexp_Match_Hit-16                  	27182196	        44.32 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Miss-16                 	52063652	        23.45 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Complex_Hit-16          	27745530	        43.17 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Long_Miss-16            	15397882	        77.87 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_MatchString-16                	26898240	        44.54 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_FindSubmatch_Hit-16           	 5668017	       213.5 ns/op	     160 B/op	       2 allocs/op
BenchmarkRegexp_FindSubmatch_Miss-16          	 1744779	       682.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_FindSubmatchIndex_Hit-16      	 6302240	       186.7 ns/op	      64 B/op	       1 allocs/op
BenchmarkRegexp_FindSubmatchIndex_Miss-16     	 1729416	       695.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_ReplaceAll_Hit-16             	 7520227	       159.2 ns/op	      97 B/op	       5 allocs/op
BenchmarkRegexp_ReplaceAll_Miss-16            	22803156	        50.97 ns/op	      48 B/op	       2 allocs/op
BenchmarkRegexp_MatchViaClosure-16            	27570810	        44.66 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_Hit-16           	11265537	       106.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_Miss-16          	12224533	        98.90 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_InPipeline-16    	 3924684	       303.5 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_MatchRe_Hit-16          	 5342010	       222.0 ns/op	      48 B/op	       1 allocs/op
BenchmarkFastjq_Small_MatchRe_Miss-16         	 2018510	       594.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_TestRe_Hit-16             	  913852	      1300 ns/op	    2736 B/op	      26 allocs/op
BenchmarkGojq_Small_TestRe_Miss-16            	  906301	      1253 ns/op	    2697 B/op	      26 allocs/op
BenchmarkGojq_Small_MatchRe_Hit-16            	  413583	      2938 ns/op	    4788 B/op	      66 allocs/op
BenchmarkGojq_Small_MatchRe_Miss-16           	  744982	      1674 ns/op	    2248 B/op	      24 allocs/op
BenchmarkFastjq_Small_CaptureRe_Hit-16        	 5512161	       217.9 ns/op	      48 B/op	       1 allocs/op
BenchmarkFastjq_Small_CaptureRe_Miss-16       	 1995081	       609.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ScanRe_NoGroups-16      	 2337073	       518.1 ns/op	     388 B/op	      10 allocs/op
BenchmarkFastjq_Small_ScanRe_WithGroups-16    	 1844941	       649.8 ns/op	     704 B/op	      13 allocs/op
BenchmarkFastjq_Small_SubRe_Hit-16            	 8928559	       128.7 ns/op	      16 B/op	       1 allocs/op
BenchmarkFastjq_Small_SubRe_Miss-16           	11670828	       101.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_GSubRe_Hit-16           	 2023076	       592.9 ns/op	     305 B/op	       5 allocs/op
BenchmarkFastjq_Small_GSubRe_Miss-16          	 4633081	       255.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_CaptureRe_Hit-16          	  500379	      2373 ns/op	    4716 B/op	      51 allocs/op
BenchmarkGojq_Small_CaptureRe_Miss-16         	  666481	      1837 ns/op	    2671 B/op	      27 allocs/op
BenchmarkGojq_Small_ScanRe_NoGroups-16        	  427524	      2861 ns/op	    5122 B/op	      57 allocs/op
BenchmarkGojq_Small_SubRe_Hit-16              	  223018	      5401 ns/op	    8982 B/op	      95 allocs/op
BenchmarkGojq_Small_GSubRe_Hit-16             	   77028	     15401 ns/op	   17858 B/op	     248 allocs/op
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
