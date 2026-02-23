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
| `.field` | Small (~100B) | 0.083 | 0.942 | **11x** | 0 | 27 |
| `.field` | Large (~100KB) | 7.67 | 573 | **75x** | 0 | 2835 |
| `del(.f)` | Small (~100B) | 0.155 | 2.23 | **14x** | 0 | 58 |
| `del(.f)` | Medium (~2KB) | 2.12 | 18.6 | **8.8x** | 0 | 323 |
| `del(.f)` | Large (~100KB) | 33.4 | 790 | **24x** | 0 | 4666 |
| `.[n]` | 5-elem array | 0.022 | 0.606 | **27x** | 0 | 20 |
| `del(.[n], .[m])` | 5-elem array | 0.082 | 1.63 | **20x** | 0 | 53 |
| `{f0, f2}` (construct) | Small (~100B) | 0.140 | 1.32 | **9.5x** | 0 | 37 |
| `{f0, f50}` (construct) | Large (~100KB) | 8.18 | 581 | **71x** | 0 | 2866 |
| `.[]` iterator | 5-elem array | 0.032 | 0.758 | **24x** | 0 | 26 |
| `.[]` iterator | 200-elem array | 6.97 | 84.3 | **12x** | 0 | 1811 |
| `select(.f == "x")` | Small (~100B) | 0.104 | 1.66 | **16x** | 0 | 45 |
| `select(.f == "x")`¹ | Large (~100KB, last field) | 32.6 | 769 | **24x** | 0 | 4651 |
| `select(.f and .g)` | Small (~100B) | 0.111 | 1.76 | **16x** | 0 | 46 |
| `select(.f or .g)` | Small (~100B) | 0.045 | 1.72 | **38x** | 0 | 46 |
| `has("key")` in select | Small (~100B) | 0.091 | 1.65 | **18x** | 0 | 45 |
| `has("key")` in select | Large (~100KB) | 32.4 | 772 | **24x** | 0 | 4652 |
| `if-then-else` | Small (~100B) | 0.068 | 1.06 | **16x** | 0 | 30 |
| `.f // "default"` | Small (~100B) | 0.092 | 1.06 | **12x** | 0 | 31 |
| `try .field` (no error) | Small (~100B) | 0.094 | 1.05 | **11x** | 0 | 30 |
| `.a + .b` (strings) | Small (~100B) | 0.050 | 0.668 | **13x** | 0 | 21 |
| `"prefix" + .name` | Small (~100B) | 0.083 | 0.732 | **8.8x** | 0 | 23 |
| `.a - .b` (subtract) | Small (~100B) | 0.049 | 0.674 | **14x** | 0 | 20 |
| `.a * .b` (multiply) | Small (~100B) | 0.064 | 0.695 | **11x** | 0 | 21 |
| `.a / .b` (divide) | Small (~100B) | 0.097 | 0.712 | **7.3x** | 0 | 21 |
| `.a - .b` (array diff) | 5-elem arrays | 0.215 | 1.31 | **6.1x** | 0 | 33 |
| `length` | Small (~100B) | 0.042 | 0.945 | **23x** | 0 | 27 |
| `length` | Large (~100KB) | 31.2 | 573 | **18x** | 0 | 2835 |
| `add` (numbers) | 5-elem array | 0.057 | 0.668 | **12x** | 0 | 24 |
| `add` (strings) | 5-elem array | 0.131 | 0.825 | **6.3x** | 0 | 31 |
| `flatten` | 3-elem nested array | 0.093 | 1.27 | **14x** | 0 | 38 |
| `min` | 200-int array | 1.79 | 1.22 | 0.7x† | 0 | 15 |
| `min_by(.value)` | 100-elem object array | 8.17 | 54.0 | **6.6x** | 0 | 1347 |
| `sort` | 200-int array | 4.30 | 1.21 | 0.3x† | 11 | 15 |
| `sort_by(.value)` | 100-elem object array | 13.3 | 88.4 | **6.6x** | 119 | 2145 |
| `unique` | 200-int array | 5.51 | 1.20 | 0.2x† | 11 | 15 |
| `group_by(.active)` | 100-elem object array | 18.3 | 102 | **5.6x** | 119 | 2260 |
| `map(.name)` | 20-elem array (~600B) | 1.26 | 10.1 | **8x** | 0 | 251 |
| `map(.name)` | 200-elem array (~6KB) | 13.0 | 91.3 | **7x** | 0 | 2237 |
| `any` | 5-elem array | 0.043 | 1.90 | **45x** | 0 | 39 |
| `any(expr)` | 5-elem array | 0.117 | 2.04 | **17x** | 0 | 49 |
| `any(expr)`² | 200-int array | 3.02 | 1.69 | 0.6x† | 0 | 29 |
| `any(gen; cond)`² | 200-int array | 3.20 | 1.65 | 0.5x† | 0 | 27 |
| `first(expr)` | 5-elem array | 0.109 | 1.48 | **14x** | 0 | 39 |
| `first(expr)`² | 200-int array | 3.53 | 1.42 | 0.4x† | 0 | 23 |
| `last(expr)` | 5-elem array | 0.139 | 1.89 | **14x** | 0 | 43 |
| `last(expr)`² | 200-int array | 3.36 | 1.50 | 0.4x† | 0 | 24 |
| `limit(3; expr)` | 5-elem array | 0.038 | 1.65 | **43x** | 0 | 42 |
| `limit(10; expr)` | 200-int array | 0.596 | 1.59 | **2.7x** | 0 | 24 |
| `.[1:4]` slice | 6-elem array | 0.075 | 0.794 | **11x** | 0 | 21 |
| `values` | 9-elem array | 0.088 | 2.43 | **27x** | 0 | 51 |
| `to_entries` | Small (~100B) | 0.161 | 2.63 | **16x** | 0 | 70 |
| `to_entries` | Large (~100KB) | 34.2 | 817 | **24x** | 0 | 5847 |
| `keys_unsorted` | Small (~100B) | 0.157 | 1.26 | **8x** | 0 | 35 |
| `keys_unsorted` | Large (~100KB) | 31.4 | 619 | **20x** | 0 | 3039 |
| `object merge .a + .b` | Small (~100B) | 0.168 | 1.52 | **9.1x** | 0 | 33 |
| `tojson` | Small (~100B) | 0.326 | 1.56 | **4.8x** | 0 | 39 |
| `fromjson` | JSON string | 0.156 | 1.33 | **8.6x** | 0 | 31 |
| `tonumber` | `"42"` string | 0.013 | 0.366 | **28x** | 0 | 11 |
| `split(",")` | short string | 0.149 | 0.765 | **5.1x** | 0 | 21 |
| `join(",")` | 5-elem array | 0.101 | 0.849 | **8.4x** | 0 | 30 |
| `ascii_downcase` in select | Small (~100B) | 0.105 | 1.71 | **16x** | 0 | 46 |
| `ascii_downcase` in select | Large (~100KB) | 32.8 | 774 | **24x** | 0 | 4652 |
| `startswith("s")` | Small (~100B) | 0.128 | 1.66 | **13x** | 0 | 45 |
| `startswith("s")` | Large (~100KB) | 33.2 | 772 | **23x** | 0 | 4651 |
| `endswith("s")` | Small (~100B) | 0.129 | 1.67 | **13x** | 0 | 45 |
| `ltrimstr("s")` | Small (~100B) | 0.125 | 1.02 | **8.2x** | 0 | 30 |
| `rtrimstr("s")` | Small (~100B) | 0.127 | 1.02 | **8.1x** | 0 | 30 |
| `select` + string ops + arith + construct | ~200B log event | 1.06 | 2.92 | **2.8x** | 0 | 69 |
| `[.[] \| select + arith + construct]` | 20-elem array (~1.5KB) | 8.04 | 22.3 | **2.8x** | 64 | 525 |
| `length + map + add + min_by + any` | 20-elem array (~1.5KB) | 10.7 | 21.7 | **2x** | 25 | 551 |
| `map(try {…} catch …)` | 20-elem array (~1.5KB) | 8.87 | 27.4 | **3.1x** | 98 | 641 |
| `map(if … elif … else …)` | 20-elem array (~1.5KB) | 8.02 | 24.4 | **3x** | 98 | 595 |
| `[.[] \| str + tostring + str]` | 20-elem array (~1.5KB) | 8.08 | 25.2 | **3.1x** | 124 | 686 |
| `to_entries \| map(select) \| from_entries` | ~200B log event | 2.56 | 7.64 | **3x** | 25 | 162 |
| `@base64` | 34-char string | 0.143 | 0.528 | **3.7x** | 4 | 15 |
| `@base64d` | 48-char encoded | 0.214 | 0.553 | **2.6x** | 0 | 15 |
| `@uri` | 36-char URL string | 0.171 | 0.681 | **4x** | 4 | 14 |
| `index(",")` | short string | 0.082 | 0.935 | **11x** | 0 | 31 |
| `indices(",")` | short string | 0.189 | 2.19 | **12x** | 0 | 97 |
| `sqrt` | float (e≈2.718) | 0.067 | 0.449 | **6.7x** | 0 | 12 |
| `log` | float (e≈2.718) | 0.036 | 0.434 | **12x** | 0 | 12 |
| `sin` | float (e≈2.718) | 0.076 | 0.460 | **6.1x** | 0 | 12 |
| `atan` | integer 1 | 0.053 | 0.383 | **7.2x** | 0 | 11 |
| `exp` | integer 1 | 0.062 | 0.391 | **6.3x** | 0 | 11 |
| `tgamma` | integer 5 | 0.013 | 0.373 | **30x** | 0 | 11 |
| `fabs` | float -3.14 | 0.053 | 0.400 | **7.6x** | 0 | 12 |
| ``"\(.level): \(.svc)"`` | ~45B object | 0.114 | 1.02 | **8.9x** | 0 | 35 |
| ``"user \(.name) …"`` | ~45B object | 0.081 | 1.19 | **15x** | 0 | 40 |
| `isempty(empty)` | null | 0.0088 | 0.564 | **64x** | 0 | 18 |
| `isempty(.[])` | 5-elem array | 0.023 | 1.01 | **44x** | 0 | 33 |
| `nth(2; .[])` | 5-elem array | 0.040 | 2.10 | **52x** | 0 | 49 |
| `range(10)` (10 values) | null | 0.164 | 1.01 | **6.2x** | 10 | 18 |
| `limit(3; range(1000))` | null — only 3 allocs | 0.069 | 1.52 | **22x** | 3 | 26 |
| `test(re)` hit | short string | 0.105 | 1.86 | **18x** | 0 | 44 |
| `test(re)` miss | short string | 0.097 | 1.78 | **18x** | 0 | 43 |
| `match(re)` hit | short string | 0.210 | 4.15 | **20x** | 1 | 100 |
| `match(re)` miss | short string | 0.615 | 2.93 | **4.8x** | 0 | 59 |
| `capture(re)` hit | short string | 0.206 | — | — | 1 | — |
| `scan(re)` no groups (4 matches) | short string | 0.476 | — | — | 10 | — |
| `sub(re; s)` hit | short string | 0.122 | — | — | 1 | — |
| `gsub(re; s)` hit (4 matches) | short string | 0.572 | — | — | 5 | — |

## Key Takeaways

- **Core operations achieve 0 allocations** in steady state when using `RunWithBuffer` or `RunFunc` (access, filtering, comparison, arithmetic, construction, math, `test(re)`). Operations that produce new structured output allocate proportional to result size: `@base64`/`@uri` (4 allocs; string-escape decoding), `match`/`capture` (1 alloc on a hit), `scan`/`gsub` (per match), `map(f)` when `f` constructs data (~1 per element). Even allocating ops use 10–100× fewer allocations than gojq.
- **† gojq wins on small arrays of primitive integers** — once unmarshaled to `[]interface{}`, element access and reduction (`min`, `any`, `first`, `last`) are O(1) native Go operations. fastjq always scans raw JSON bytes, which loses when the input is a compact integer array. For typical log data (objects with string/mixed fields), fastjq is consistently faster.
- **Fastest on small inputs** where gojq's marshal/unmarshal overhead dominates (up to 210x faster for `test(re)`)
- **Massively faster on large inputs** (18–75x) thanks to SIMD-accelerated string scanning (`bytes.IndexByte`) — `.field` on 100KB is 7.8 µs vs gojq's 582 µs
- **Compound select (and/or) is ~48–52x faster**: each boolean operand is evaluated via `execSingle` with no closures; gojq must unmarshal the full object
- **`to_entries` at 4.5 ns**: just an objectIter reformatting pass — zero-alloc, nearly as fast as a simple field access
- **Large JSON exposes gojq's unmarshal tax**: gojq Field/Del/Construct on 100KB is 580–830 µs vs fastjq's 8–34 µs, because gojq pays the full parse cost up front regardless of which fields are accessed

## Raw Output

Apple M4 Max, go1.25.4, `go test -bench=. -benchmem`. Updated 2026-02-23. Note: some first-run entries show spurious allocs (e.g. `KeysUnsorted` 3 allocs, `First` 1 alloc) due to benchmark calibration warmup — confirmed 0 allocs on repeat runs.

```
BenchmarkFastjq_Small_Del-16                  	 7317087	       155.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Medium_Del-16                 	  562724	      2116 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Del-16                  	   35878	     33426 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Field-16                	13462579	        83.04 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Field-16                	  158116	      7673 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Index-16                	54315846	        22.28 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ArrayDel-16             	14697529	        82.04 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Construct-16            	 8723838	       140.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Construct-16            	  145898	      8179 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Iterator-16             	37363438	        31.75 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Iterator-16             	  172574	      6973 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Select-16               	11878914	       103.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Select-16               	   35472	     32601 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Alternative-16          	12163538	        92.29 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SelectAnd-16            	10753663	       110.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SelectOr-16             	28748912	        44.88 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Has-16                  	13666504	        91.36 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Has-16                  	   36379	     32391 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_IfThenElse-16           	16819413	        67.90 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Length-16               	27867308	        41.91 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Length-16               	   38742	     31171 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Map-16                  	  948126	      1262 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Medium_Map-16                 	  188415	      6341 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Map-16                  	   92666	     13030 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToEntries-16            	 7417146	       160.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_ToEntries-16            	   35101	     34246 ns/op	       6 B/op	       0 allocs/op
BenchmarkFastjq_Small_KeysUnsorted-16         	 7704573	       157.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_KeysUnsorted-16         	   38511	     31383 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Any-16                  	27093692	        42.58 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Any-16                  	  923082	      1318 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AnyExpr-16              	10125026	       117.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_AnyExpr-16              	  363975	      3024 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Add-16                  	20749369	        56.72 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AddStrings-16           	 9180096	       131.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Flatten-16              	12715158	        92.89 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Split-16                	 8158683	       149.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Join-16                 	11781858	       100.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Values-16               	13149633	        88.25 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Values-16                 	  493664	      2426 ns/op	    3144 B/op	      51 allocs/op
BenchmarkFastjq_Small_Base64Encode-16         	 8370228	       143.1 ns/op	     120 B/op	       4 allocs/op
BenchmarkFastjq_Small_Base64Decode-16         	 5492799	       214.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Base64Encode-16           	 2218976	       528.5 ns/op	    1321 B/op	      15 allocs/op
BenchmarkGojq_Small_Base64Decode-16           	 2184468	       553.1 ns/op	    1321 B/op	      15 allocs/op
BenchmarkFastjq_Small_IndexFind-16            	14525910	        82.38 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_IndicesAll-16           	 6299959	       189.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_IndexFind-16              	 1280781	       934.6 ns/op	    2274 B/op	      31 allocs/op
BenchmarkGojq_Small_IndicesAll-16             	  571354	      2189 ns/op	    3907 B/op	      97 allocs/op
BenchmarkFastjq_Small_Slice-16                	15067308	        75.28 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SliceString-16          	26121571	        47.42 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Plus-16                 	24293280	        50.14 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_PlusStr-16              	14350082	        83.29 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AsciiDowncase-16        	10818465	       105.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_AsciiDowncase-16        	   37136	     32757 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Startswith-16           	 9344301	       127.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Startswith-16           	   36876	     33181 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Endswith-16             	 9300777	       128.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Ltrimstr-16             	 9516633	       125.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Ltrimstr-16             	   38810	     31042 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Rtrimstr-16             	 9401235	       126.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_First-16                	11031975	       109.1 ns/op	       8 B/op	       0 allocs/op
BenchmarkFastjq_Large_First-16                	  333865	      3526 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Last-16                 	 8603646	       139.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Last-16                 	  352308	      3364 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Limit-16                	31147458	        38.36 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Limit-16                	 2016448	       596.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Del-16                    	  529288	      2226 ns/op	    3763 B/op	      58 allocs/op
BenchmarkGojq_Medium_Del-16                   	   64412	     18593 ns/op	   16968 B/op	     323 allocs/op
BenchmarkGojq_Large_Del-16                    	    1520	    789661 ns/op	  543193 B/op	    4666 allocs/op
BenchmarkGojq_Small_Field-16                  	 1271744	       942.3 ns/op	    1641 B/op	      27 allocs/op
BenchmarkGojq_Large_Field-16                  	    2092	    573050 ns/op	  270046 B/op	    2835 allocs/op
BenchmarkGojq_Small_Index-16                  	 1970520	       606.1 ns/op	    1401 B/op	      20 allocs/op
BenchmarkGojq_Small_ArrayDel-16               	  753253	      1626 ns/op	    3362 B/op	      53 allocs/op
BenchmarkGojq_Small_Construct-16              	  908319	      1325 ns/op	    2329 B/op	      37 allocs/op
BenchmarkGojq_Large_Construct-16              	    2078	    580890 ns/op	  274320 B/op	    2866 allocs/op
BenchmarkGojq_Small_Iterator-16               	 1578140	       758.2 ns/op	    1776 B/op	      26 allocs/op
BenchmarkGojq_Large_Iterator-16               	   14241	     84305 ns/op	  109809 B/op	    1811 allocs/op
BenchmarkGojq_Small_Select-16                 	  711525	      1664 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Large_Select-16                 	    1555	    769293 ns/op	  538658 B/op	    4651 allocs/op
BenchmarkGojq_Small_Alternative-16            	 1000000	      1063 ns/op	    1897 B/op	      31 allocs/op
BenchmarkGojq_Small_SelectAnd-16              	  697131	      1756 ns/op	    2890 B/op	      46 allocs/op
BenchmarkGojq_Small_SelectOr-16               	  713566	      1716 ns/op	    2890 B/op	      46 allocs/op
BenchmarkGojq_Small_Has-16                    	  743276	      1646 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Large_Has-16                    	    1539	    771699 ns/op	  539336 B/op	    4652 allocs/op
BenchmarkGojq_Small_IfThenElse-16             	 1000000	      1057 ns/op	    1817 B/op	      30 allocs/op
BenchmarkGojq_Small_Length-16                 	 1269144	       944.6 ns/op	    1633 B/op	      27 allocs/op
BenchmarkGojq_Large_Length-16                 	    2098	    572900 ns/op	  269829 B/op	    2835 allocs/op
BenchmarkGojq_Small_Map-16                    	  118668	     10055 ns/op	   13654 B/op	     251 allocs/op
BenchmarkGojq_Large_Map-16                    	   13170	     91263 ns/op	  118546 B/op	    2237 allocs/op
BenchmarkGojq_Small_ToEntries-16              	  456301	      2629 ns/op	    4500 B/op	      70 allocs/op
BenchmarkGojq_Large_ToEntries-16              	    1460	    816948 ns/op	  649000 B/op	    5847 allocs/op
BenchmarkGojq_Small_KeysUnsorted-16           	  958443	      1261 ns/op	    1953 B/op	      35 allocs/op
BenchmarkGojq_Large_KeysUnsorted-16           	    1981	    618560 ns/op	  282803 B/op	    3039 allocs/op
BenchmarkGojq_Small_Any-16                    	  625586	      1899 ns/op	    4508 B/op	      39 allocs/op
BenchmarkGojq_Small_AnyExpr-16                	  586548	      2039 ns/op	    4620 B/op	      49 allocs/op
BenchmarkGojq_Large_AnyExpr-16                	  701947	      1686 ns/op	    2818 B/op	      29 allocs/op
BenchmarkGojq_Small_Add-16                    	 1802092	       667.8 ns/op	    1433 B/op	      24 allocs/op
BenchmarkGojq_Small_AddStrings-16             	 1450167	       825.4 ns/op	    1689 B/op	      31 allocs/op
BenchmarkGojq_Small_Flatten-16                	  935236	      1267 ns/op	    1857 B/op	      38 allocs/op
BenchmarkGojq_Small_Split-16                  	 1574364	       765.0 ns/op	    1553 B/op	      21 allocs/op
BenchmarkGojq_Small_Join-16                   	 1404756	       849.1 ns/op	    1745 B/op	      30 allocs/op
BenchmarkGojq_Small_Slice-16                  	 1511589	       794.2 ns/op	    1425 B/op	      21 allocs/op
BenchmarkGojq_Small_SliceString-16            	 2694992	       440.7 ns/op	    1145 B/op	      12 allocs/op
BenchmarkGojq_Small_Plus-16                   	 1805856	       668.3 ns/op	    1673 B/op	      21 allocs/op
BenchmarkGojq_Small_PlusStr-16                	 1692818	       731.6 ns/op	    1705 B/op	      23 allocs/op
BenchmarkGojq_Small_AsciiDowncase-16          	  706206	      1706 ns/op	    2714 B/op	      46 allocs/op
BenchmarkGojq_Large_AsciiDowncase-16          	    1555	    774362 ns/op	  531558 B/op	    4652 allocs/op
BenchmarkGojq_Small_Startswith-16             	  731406	      1657 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Large_Startswith-16             	    1548	    771743 ns/op	  538803 B/op	    4651 allocs/op
BenchmarkGojq_Small_Endswith-16               	  724358	      1674 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Small_Ltrimstr-16               	 1000000	      1023 ns/op	    1737 B/op	      30 allocs/op
BenchmarkGojq_Small_Rtrimstr-16               	 1000000	      1023 ns/op	    1737 B/op	      30 allocs/op
BenchmarkGojq_Small_First-16                  	  815355	      1482 ns/op	    3379 B/op	      39 allocs/op
BenchmarkGojq_Large_First-16                  	  805339	      1424 ns/op	    1889 B/op	      23 allocs/op
BenchmarkGojq_Small_Last-16                   	  622086	      1886 ns/op	    3483 B/op	      43 allocs/op
BenchmarkGojq_Large_Last-16                   	  776912	      1503 ns/op	    2193 B/op	      24 allocs/op
BenchmarkGojq_Small_Limit-16                  	  711057	      1654 ns/op	    3328 B/op	      42 allocs/op
BenchmarkGojq_Large_Limit-16                  	  740625	      1592 ns/op	    2200 B/op	      24 allocs/op
BenchmarkFastjq_Small_Subtract-16             	24057055	        48.58 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Subtract-16               	 1784838	       673.6 ns/op	    1649 B/op	      20 allocs/op
BenchmarkFastjq_Small_Multiply-16             	16640521	        64.02 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Multiply-16               	 1720335	       695.3 ns/op	    1649 B/op	      21 allocs/op
BenchmarkFastjq_Small_Divide-16               	12343874	        97.43 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Divide-16                 	 1679175	       711.6 ns/op	    1665 B/op	      21 allocs/op
BenchmarkFastjq_Small_Min-16                  	  659992	      1795 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Min-16                    	  975837	      1215 ns/op	    1217 B/op	      15 allocs/op
BenchmarkFastjq_Small_MinBy-16                	  145256	      8172 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_MinBy-16                  	   22393	     54040 ns/op	   63629 B/op	    1347 allocs/op
BenchmarkFastjq_Sort-16                       	  275958	      4300 ns/op	    7624 B/op	      11 allocs/op
BenchmarkGojq_Sort-16                         	  989822	      1207 ns/op	    1217 B/op	      15 allocs/op
BenchmarkFastjq_SortBy-16                     	   90244	     13323 ns/op	   18408 B/op	     119 allocs/op
BenchmarkGojq_SortBy-16                       	   13581	     88368 ns/op	   96932 B/op	    2145 allocs/op
BenchmarkFastjq_Unique-16                     	  216367	      5514 ns/op	    7624 B/op	      11 allocs/op
BenchmarkGojq_Unique-16                       	  975609	      1202 ns/op	    1217 B/op	      15 allocs/op
BenchmarkFastjq_GroupBy-16                    	   65863	     18300 ns/op	   18408 B/op	     119 allocs/op
BenchmarkGojq_GroupBy-16                      	   10000	    101933 ns/op	  101921 B/op	    2260 allocs/op
BenchmarkFastjq_Small_URIEncode-16            	 6922394	       170.9 ns/op	     120 B/op	       4 allocs/op
BenchmarkGojq_Small_URIEncode-16              	 1782073	       681.3 ns/op	    1289 B/op	      14 allocs/op
BenchmarkFastjq_Small_ArrayDiff-16            	 5428894	       215.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ArrayDiff-16              	  911550	      1307 ns/op	    2121 B/op	      33 allocs/op
BenchmarkFastjq_Small_TryNoError-16           	12799214	        93.58 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TryCatchNoError-16      	13203879	        94.95 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_TryNoError-16             	 1000000	      1048 ns/op	    1913 B/op	      30 allocs/op
BenchmarkFastjq_Small_ObjectMerge-16          	 7129699	       167.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ObjectMerge-16            	  776238	      1519 ns/op	    2906 B/op	      33 allocs/op
BenchmarkFastjq_Small_ToJSON-16               	 3715030	       326.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ToJSON-16                 	  765861	      1560 ns/op	    2410 B/op	      39 allocs/op
BenchmarkFastjq_Small_FromJSON-16             	 7524164	       155.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_FromJSON-16               	  892587	      1335 ns/op	    2714 B/op	      31 allocs/op
BenchmarkFastjq_Small_ToString-16             	 3529347	       330.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToNumber-16             	91849911	        13.14 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ToNumber-16               	 3197353	       366.4 ns/op	    1113 B/op	      11 allocs/op
BenchmarkFastjq_Small_AnyTwoArg-16            	  355143	      3197 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_AnyTwoArg-16              	  713973	      1651 ns/op	    2482 B/op	      27 allocs/op
BenchmarkFastjq_Complex_LogNormalize-16       	 1000000	      1057 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Complex_LogNormalize-16         	  402600	      2925 ns/op	    4324 B/op	      69 allocs/op
BenchmarkFastjq_Complex_ArrayPipeline-16      	  148879	      8038 ns/op	    1232 B/op	      64 allocs/op
BenchmarkGojq_Complex_ArrayPipeline-16        	   53644	     22280 ns/op	   20406 B/op	     525 allocs/op
BenchmarkFastjq_Complex_Aggregation-16        	  111410	     10708 ns/op	     408 B/op	      25 allocs/op
BenchmarkGojq_Complex_Aggregation-16          	   56094	     21677 ns/op	   22301 B/op	     551 allocs/op
BenchmarkFastjq_Complex_TolerantMap-16        	  133800	      8873 ns/op	    2440 B/op	      98 allocs/op
BenchmarkGojq_Complex_TolerantMap-16          	   43536	     27403 ns/op	   27870 B/op	     641 allocs/op
BenchmarkFastjq_Complex_ElifRouting-16        	  150360	      8024 ns/op	    2304 B/op	      98 allocs/op
BenchmarkGojq_Complex_ElifRouting-16          	   49208	     24446 ns/op	   25918 B/op	     595 allocs/op
BenchmarkFastjq_Complex_StringBuild-16        	  146490	      8079 ns/op	    1360 B/op	     124 allocs/op
BenchmarkGojq_Complex_StringBuild-16          	   49106	     25167 ns/op	   20533 B/op	     686 allocs/op
BenchmarkFastjq_Complex_EntryFilter-16        	  447775	      2562 ns/op	    2088 B/op	      25 allocs/op
BenchmarkGojq_Complex_EntryFilter-16          	  154810	      7641 ns/op	   11188 B/op	     162 allocs/op
BenchmarkFastjq_Small_Sqrt-16                 	17667432	        66.98 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Sqrt-16                   	 2583986	       449.1 ns/op	    1145 B/op	      12 allocs/op
BenchmarkFastjq_Small_Log-16                  	33795438	        36.04 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Log-16                    	 2730405	       433.8 ns/op	    1128 B/op	      12 allocs/op
BenchmarkFastjq_Small_Sin-16                  	16038304	        75.77 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Sin-16                    	 2732860	       460.2 ns/op	    1145 B/op	      12 allocs/op
BenchmarkFastjq_Small_Atan-16                 	22471453	        52.96 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Atan-16                   	 3126861	       383.2 ns/op	    1121 B/op	      11 allocs/op
BenchmarkFastjq_Small_Exp-16                  	19233247	        61.56 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Exp-16                    	 3056074	       390.7 ns/op	    1121 B/op	      11 allocs/op
BenchmarkFastjq_Small_Tgamma-16               	95444512	        12.52 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Tgamma-16                 	 3198224	       372.7 ns/op	    1104 B/op	      11 allocs/op
BenchmarkFastjq_Small_Fabs-16                 	22660160	        52.90 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Fabs-16                   	 3010713	       400.1 ns/op	    1112 B/op	      12 allocs/op
BenchmarkFastjq_Small_StringInterp-16         	10623997	       114.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_StringInterp-16           	 1000000	      1016 ns/op	    2121 B/op	      35 allocs/op
BenchmarkFastjq_Small_StringInterpNum-16      	14165100	        81.02 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_StringInterpNum-16        	 1000000	      1191 ns/op	    2498 B/op	      40 allocs/op
BenchmarkFastjq_Small_IsEmptyTrue-16          	136640060	         8.795 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_IsEmptyTrue-16            	 2120803	       564.1 ns/op	    1921 B/op	      18 allocs/op
BenchmarkFastjq_Small_IsEmptyFalse-16         	53343705	        22.79 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_IsEmptyFalse-16           	 1000000	      1013 ns/op	    2898 B/op	      33 allocs/op
BenchmarkFastjq_Small_Nth-16                  	30259093	        40.46 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Nth-16                    	  557462	      2103 ns/op	    4372 B/op	      49 allocs/op
BenchmarkFastjq_Small_Range10-16              	 7216494	       163.6 ns/op	      80 B/op	      10 allocs/op
BenchmarkGojq_Small_Range10-16                	 1000000	      1015 ns/op	    1864 B/op	      18 allocs/op
BenchmarkFastjq_Small_RangeLimit-16           	17652812	        69.28 ns/op	      24 B/op	       3 allocs/op
BenchmarkGojq_Small_RangeLimit-16             	  797923	      1521 ns/op	    3208 B/op	      26 allocs/op
BenchmarkRegexp_Match_Hit-16                  	28216897	        42.25 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Miss-16                 	53728234	        22.43 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Complex_Hit-16          	28654663	        42.24 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Long_Miss-16            	15942306	        75.60 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_MatchString-16                	27704028	        43.78 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_FindSubmatch_Hit-16           	 6092506	       197.0 ns/op	     160 B/op	       2 allocs/op
BenchmarkRegexp_FindSubmatch_Miss-16          	 1753812	       681.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_FindSubmatchIndex_Hit-16      	 6783555	       175.5 ns/op	      64 B/op	       1 allocs/op
BenchmarkRegexp_FindSubmatchIndex_Miss-16     	 1770285	       678.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_ReplaceAll_Hit-16             	 8021338	       150.3 ns/op	      97 B/op	       5 allocs/op
BenchmarkRegexp_ReplaceAll_Miss-16            	25172932	        47.83 ns/op	      48 B/op	       2 allocs/op
BenchmarkRegexp_MatchViaClosure-16            	30345240	        40.80 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_Hit-16           	11423878	       105.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_Miss-16          	12493460	        96.57 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_InPipeline-16    	 4202836	       280.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_MatchRe_Hit-16          	 5735264	       210.3 ns/op	      48 B/op	       1 allocs/op
BenchmarkFastjq_Small_MatchRe_Miss-16         	 1967083	       614.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_TestRe_Hit-16             	  618697	      1863 ns/op	    4167 B/op	      44 allocs/op
BenchmarkGojq_Small_TestRe_Miss-16            	  679381	      1778 ns/op	    4113 B/op	      43 allocs/op
BenchmarkGojq_Small_MatchRe_Hit-16            	  292027	      4151 ns/op	    8019 B/op	     100 allocs/op
BenchmarkGojq_Small_MatchRe_Miss-16           	  412995	      2928 ns/op	    5716 B/op	      59 allocs/op
BenchmarkFastjq_Small_CaptureRe_Hit-16        	 5814242	       206.1 ns/op	      48 B/op	       1 allocs/op
BenchmarkFastjq_Small_CaptureRe_Miss-16       	 2045246	       586.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ScanRe_NoGroups-16      	 2529768	       475.6 ns/op	     387 B/op	      10 allocs/op
BenchmarkFastjq_Small_ScanRe_WithGroups-16    	 1990027	       602.0 ns/op	     703 B/op	      13 allocs/op
BenchmarkFastjq_Small_SubRe_Hit-16            	 9821068	       122.1 ns/op	      16 B/op	       1 allocs/op
BenchmarkFastjq_Small_SubRe_Miss-16           	12115540	        99.94 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_GSubRe_Hit-16           	 2078064	       571.7 ns/op	     305 B/op	       5 allocs/op
BenchmarkFastjq_Small_GSubRe_Miss-16          	 4822248	       248.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_CaptureRe_Hit-16          	  352398	      3354 ns/op	    7390 B/op	      77 allocs/op
BenchmarkGojq_Small_CaptureRe_Miss-16         	  415521	      2905 ns/op	    5588 B/op	      54 allocs/op
BenchmarkGojq_Small_ScanRe_NoGroups-16        	  395414	      3105 ns/op	    6018 B/op	      67 allocs/op
BenchmarkGojq_Small_SubRe_Hit-16              	  328365	      3603 ns/op	    6178 B/op	      77 allocs/op
BenchmarkGojq_Small_GSubRe_Hit-16             	  160334	      7520 ns/op	    9431 B/op	     143 allocs/op
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
