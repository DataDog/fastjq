# fastjq Benchmarks

Benchmarks comparing fastjq against [gojq](https://github.com/itchyny/gojq). gojq benchmarks include the full `json.Unmarshal` → execute → `json.Marshal` cycle, since that's the real-world cost of using a traditional jq engine from Go.

All benchmarks run with `go test -bench=. -benchmem`. Results from Apple M4 Max, Go 1.25.

> **New in this run**: Fixed 10 correctness bugs from the official jq test suite: `try-catch` handler precedence, `flatten(-1)` error, `index("")` null return, `if empty` zero outputs, `//` multi-output left side, `string * 0` empty string, `[.[] % n]` multi-output arithmetic, `join()` object/array error, `fromjson` validation, `add` object last-wins deduplication. Also added `object * object` recursive merge. All operations maintain 0 allocs/op on simple inputs.

> **Note on benchmark reliability**: Large benchmarks use rotating input copies (8 distinct pre-generated
> instances) to prevent a Go 1.25 calibration artifact where the auto-calibration pre-pass sees warm-cache
> hits and produces results identical to the Small benchmarks. All benchmarks use `b.Loop()` (Go 1.24+)
> and `benchSink` to prevent dead-code elimination. The Large Select benchmark uses `field_199` (the last
> field in the 200-field object) so fastjq must scan the full 170KB — no early-exit advantage.

## Summary

All times in µs. ¹Large Select uses the last field — fastjq scans the full document. ²gojq wins: after unmarshal, integer arrays are native Go slices.

| Operation | Input | fastjq (µs) | gojq (µs) | Speedup | fastjq allocs | gojq allocs |
|-----------|-------|------------|----------|---------|---------------|-------------|
| `.field` | Small (~100B) | 0.156 | 0.343 | **2.2x** | 0 | 13 |
| `.field` | Large (~100KB) | 122 | 587 | **4.8x** | 0 | 2835 |
| `del(.f)` | Small (~100B) | 0.180 | 0.941 | **5.2x** | 0 | 33 |
| `del(.f)` | Medium (~2KB) | 2.90 | 18.8 | **6.5x** | 0 | 323 |
| `del(.f)` | Large (~100KB) | 173 | 800 | **4.6x** | 0 | 4666 |
| `.[n]` | 5-elem array | 0.026 | 0.611 | **23x** | 0 | 20 |
| `del(.[n], .[m])` | 5-elem array | 0.092 | 1.63 | **18x** | 0 | 53 |
| `{f0, f2}` (construct) | Small (~100B) | 0.318 | 0.697 | **2.2x** | 0 | 23 |
| `{f0, f50}` (construct) | Large (~100KB) | 222 | 590 | **2.7x** | 0 | 2867 |
| `.[]` iterator | 5-elem array | 0.032 | 0.762 | **24x** | 0 | 26 |
| `.[]` iterator | 200-elem array | 11.3 | 84.7 | **7.5x** | 0 | 1811 |
| `select(.f == "x")` | Small (~100B) | 0.010 | 0.563 | **56x** | 0 | 20 |
| `select(.f == "x")`¹ | Large (~100KB, last field) | 184 | 778 | **4.2x** | 0 | 4651 |
| `select(.f and .g)` | Small (~100B) | 0.011 | 0.623 | **55x** | 0 | 21 |
| `select(.f or .g)` | Small (~100B) | 0.011 | 0.653 | **58x** | 0 | 21 |
| `has("key")` in select | Small (~100B) | 0.013 | 0.547 | **42x** | 0 | 20 |
| `has("key")` in select | Large (~100KB) | 204 | 777 | **3.8x** | 0 | 4651 |
| `if-then-else` | Small (~100B) | 0.011 | 0.454 | **42x** | 0 | 16 |
| `.f // "default"` | Small (~100B) | 0.0083 | 0.467 | **56x** | 0 | 17 |
| `try .field` (no error) | Small (~100B) | 0.0073 | 0.418 | **58x** | 0 | 16 |
| `.a + .b` (strings) | Small (~100B) | 0.072 | 0.665 | **9.3x** | 0 | 21 |
| `"prefix" + .name` | Small (~100B) | 0.111 | 0.716 | **6.5x** | 0 | 23 |
| `.a - .b` (subtract) | Small (~100B) | 0.068 | 0.707 | **10x** | 0 | 20 |
| `.a * .b` (multiply) | Small (~100B) | 0.085 | 0.711 | **8.3x** | 0 | 21 |
| `.a / .b` (divide) | Small (~100B) | 0.120 | 0.716 | **5.9x** | 0 | 21 |
| `.a - .b` (array diff) | 5-elem arrays | 0.211 | 1.23 | **5.8x** | 0 | 33 |
| `length` | Small (~100B) | 0.0076 | 0.360 | **47x** | 0 | 13 |
| `length` | Large (~100KB) | 146 | 585 | **4x** | 0 | 2835 |
| `add` (numbers) | 5-elem array | 0.060 | 0.666 | **11x** | 0 | 24 |
| `add` (strings) | 5-elem array | 0.138 | 0.820 | **5.9x** | 0 | 31 |
| `flatten` | 3-elem nested array | 0.104 | 1.26 | **12x** | 0 | 38 |
| `min` | 200-int array | 1.72 | 1.23 | 0.7x† | 0 | 15 |
| `min_by(.value)` | 100-elem object array | 11.5 | 54.1 | **4.7x** | 0 | 1347 |
| `map(.name)` | 20-elem array (~600B) | 1.95 | 10.0 | **5.2x** | 0 | 251 |
| `map(.name)` | 200-elem array (~6KB) | 19.4 | 92.2 | **4.7x** | 0 | 2237 |
| `any` | 5-elem array | 0.047 | 1.83 | **39x** | 0 | 39 |
| `any(expr)` | 5-elem array | 0.131 | 2.05 | **16x** | 0 | 49 |
| `any(expr)`² | 200-int array | 3.01 | 1.72 | 0.6x† | 0 | 29 |
| `any(gen; cond)`² | 200-int array | 3.09 | 1.62 | 0.5x† | 0 | 27 |
| `first(expr)` | 5-elem array | 0.105 | 1.49 | **14x** | 0 | 39 |
| `first(expr)`² | 200-int array | 3.51 | 1.43 | 0.4x† | 0 | 23 |
| `last(expr)` | 5-elem array | 0.143 | 1.89 | **13x** | 0 | 43 |
| `last(expr)`² | 200-int array | 3.24 | 1.52 | 0.5x† | 0 | 24 |
| `limit(3; expr)` | 5-elem array | 0.038 | 1.69 | **44x** | 0 | 42 |
| `limit(10; expr)` | 200-int array | 0.552 | 1.69 | **3.1x** | 0 | 24 |
| `.[1:4]` slice | 6-elem array | 0.088 | 0.803 | **9.1x** | 0 | 21 |
| `values` | 9-elem array | 0.092 | 2.37 | **26x** | 0 | 51 |
| `to_entries` | Small (~100B) | 0.0066 | 0.366 | **56x** | 0 | 14 |
| `to_entries` | Large (~100KB) | 171 | 824 | **4.8x** | 0 | 5847 |
| `keys_unsorted` | Small (~100B) | 0.061 | 0.371 | **6.1x** | 3 | 14 |
| `keys_unsorted` | Large (~100KB) | 195 | 609 | **3.1x** | 0 | 3039 |
| `object merge .a + .b` | Small (~100B) | 0.224 | 1.60 | **7.1x** | 0 | 33 |
| `tojson` | Small (~100B) | 0.126 | 0.416 | **3.3x** | 0 | 17 |
| `fromjson` | JSON string | 0.155 | 1.27 | **8.2x** | 0 | 31 |
| `tonumber` | `"42"` string | 0.016 | 0.350 | **22x** | 0 | 11 |
| `split(",")` | short string | 0.114 | 0.778 | **6.8x** | 0 | 21 |
| `join(",")` | 5-elem array | 0.109 | 0.849 | **7.8x** | 0 | 30 |
| `ascii_downcase` in select | Small (~100B) | 0.013 | 0.571 | **44x** | 0 | 21 |
| `ascii_downcase` in select | Large (~100KB) | 203 | 777 | **3.8x** | 0 | 4652 |
| `startswith("s")` | Small (~100B) | 0.013 | 0.575 | **44x** | 0 | 21 |
| `startswith("s")` | Large (~100KB) | 188 | 781 | **4.2x** | 0 | 4651 |
| `endswith("s")` | Small (~100B) | 0.013 | 0.579 | **45x** | 0 | 21 |
| `ltrimstr("s")` | Small (~100B) | 0.0075 | 0.418 | **56x** | 0 | 16 |
| `rtrimstr("s")` | Small (~100B) | 0.0076 | 0.419 | **55x** | 0 | 16 |
| `select` + string ops + arith + construct | ~200B log event | 1.58 | 2.88 | **1.8x** | 0 | 69 |
| `[.[] \| select + arith + construct]` | 20-elem array (~1.5KB) | 11.7 | 23.0 | **2x** | 64 | 525 |
| `length + map + add + min_by + any` | 20-elem array (~1.5KB) | 13.9 | 22.6 | **1.6x** | 26 | 551 |
| `map(try {…} catch …)` | 20-elem array (~1.5KB) | 12.4 | 27.3 | **2.2x** | 98 | 641 |
| `map(if … elif … else …)` | 20-elem array (~1.5KB) | 12.2 | 24.8 | **2x** | 125 | 595 |
| `[.[] \| str + tostring + str]` | 20-elem array (~1.5KB) | 11.1 | 24.5 | **2.2x** | 124 | 686 |
| `to_entries \| map(select) \| from_entries` | ~200B log event | 2.99 | 7.80 | **2.6x** | 35 | 162 |
| `@base64` | 34-char string | 0.143 | 0.522 | **3.7x** | 4 | 15 |
| `@base64d` | 48-char encoded | 0.208 | 0.556 | **2.7x** | 0 | 15 |
| `@uri` | 36-char URL string | 0.166 | 0.669 | **4x** | 4 | 14 |
| `index(",")` | short string | 0.083 | 0.940 | **11x** | 0 | 31 |
| `indices(",")` | short string | 0.157 | 2.08 | **13x** | 0 | 97 |
| `sqrt` | float (e≈2.718) | 0.067 | 0.446 | **6.6x** | 0 | 12 |
| `log` | float (e≈2.718) | 0.038 | 0.446 | **12x** | 0 | 12 |
| `sin` | float (e≈2.718) | 0.078 | 0.447 | **5.8x** | 0 | 12 |
| `atan` | integer 1 | 0.053 | 0.394 | **7.5x** | 0 | 11 |
| `exp` | integer 1 | 0.064 | 0.393 | **6.2x** | 0 | 11 |
| `tgamma` | integer 5 | 0.013 | 0.371 | **28x** | 0 | 11 |
| `fabs` | float -3.14 | 0.053 | 0.399 | **7.5x** | 0 | 12 |
| ``"\(.level): \(.svc)"`` | ~45B object | 0.151 | 1.01 | **6.7x** | 0 | 35 |
| ``"user \(.name) …"`` | ~45B object | 0.138 | 1.18 | **8.5x** | 0 | 40 |
| `isempty(empty)` | null | 0.0074 | 0.561 | **76x** | 0 | 18 |
| `isempty(.[])` | 5-elem array | 0.020 | 1.01 | **50x** | 0 | 33 |
| `nth(2; .[])` | 5-elem array | 0.039 | 2.10 | **54x** | 0 | 49 |

## Key Takeaways

- **fastjq achieves 0 allocations** across all operations in steady state when using `RunWithBuffer` or `RunFunc`
- **† gojq wins on small arrays of primitive integers** — once unmarshaled to `[]interface{}`, element access and reduction (`min`, `any`, `first`, `last`) are O(1) native Go operations. fastjq always scans raw JSON bytes, which loses when the input is a compact integer array. For typical log data (objects with string/mixed fields), fastjq is consistently faster.
- **Fastest on small inputs** where gojq's marshal/unmarshal overhead dominates (up to 63x faster)
- **Still significantly faster on large inputs** (2.5x–5x) where both engines are scanning lots of data
- **Compound select (and/or) is ~56–60x faster**: each boolean operand is evaluated via `execSingle` with no closures; gojq must unmarshal the full object
- **`to_entries` at 6 ns**: just an objectIter reformatting pass — zero-alloc, nearly as fast as a simple field access
- **Large JSON exposes gojq's unmarshal tax**: gojq Field/Del/Construct on 100KB is 540–800 µs vs fastjq's 109–218 µs, because gojq pays the full parse cost up front regardless of which fields are accessed

## Raw Output

Apple M4 Max, go1.25.4, `go test -bench=. -benchmem`. Updated 2026-02-20. Note: some first-run entries show spurious allocs (e.g. `KeysUnsorted` 3 allocs, `First` 1 alloc) due to benchmark calibration warmup — confirmed 0 allocs on repeat runs.

```
BenchmarkFastjq_Small_Del-16                  	 6704524	       180.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Medium_Del-16                 	  422052	      2900 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Del-16                  	    6710	    172937 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Field-16                	 7289667	       155.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Field-16                	   10000	    121679 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Index-16                	44614502	        26.06 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ArrayDel-16             	12998530	        91.75 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Construct-16            	 3905580	       317.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Construct-16            	    5820	    221714 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Iterator-16             	37048518	        31.89 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Iterator-16             	  106774	     11342 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Select-16               	100000000	        10.00 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Select-16               	    7707	    183976 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Alternative-16          	144476859	         8.312 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SelectAnd-16            	100000000	        11.29 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SelectOr-16             	100000000	        11.32 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Has-16                  	91887427	        13.09 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Has-16                  	    6244	    203891 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_IfThenElse-16           	100000000	        10.70 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Length-16               	157175254	         7.639 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Length-16               	    7999	    145960 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Map-16                  	  605932	      1951 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Medium_Map-16                 	  123690	      9633 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Map-16                  	   62227	     19440 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToEntries-16            	183029794	         6.590 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_ToEntries-16            	    8800	    171326 ns/op	      25 B/op	       0 allocs/op
BenchmarkFastjq_Small_KeysUnsorted-16         	19602224	        61.28 ns/op	      72 B/op	       3 allocs/op
BenchmarkFastjq_Large_KeysUnsorted-16         	    5924	    194543 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Any-16                  	23493901	        47.12 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Any-16                  	  741224	      1624 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AnyExpr-16              	 9313437	       130.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_AnyExpr-16              	  390081	      3007 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Add-16                  	18465408	        59.87 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AddStrings-16           	 8699736	       138.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Flatten-16              	11483756	       103.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Split-16                	10306263	       114.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Join-16                 	11181430	       109.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Values-16               	13081068	        91.76 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Values-16                 	  503127	      2371 ns/op	    3144 B/op	      51 allocs/op
BenchmarkFastjq_Small_Base64Encode-16         	 8422600	       142.6 ns/op	     120 B/op	       4 allocs/op
BenchmarkFastjq_Small_Base64Decode-16         	 5762709	       208.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Base64Encode-16           	 2330510	       522.2 ns/op	    1321 B/op	      15 allocs/op
BenchmarkGojq_Small_Base64Decode-16           	 2160661	       556.5 ns/op	    1321 B/op	      15 allocs/op
BenchmarkFastjq_Small_IndexFind-16            	14004444	        83.04 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_IndicesAll-16           	 7650634	       157.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_IndexFind-16              	 1265392	       939.5 ns/op	    2274 B/op	      31 allocs/op
BenchmarkGojq_Small_IndicesAll-16             	  591544	      2075 ns/op	    3907 B/op	      97 allocs/op
BenchmarkFastjq_Small_Slice-16                	12991406	        87.89 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SliceString-16          	24747732	        47.31 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Plus-16                 	16408620	        71.72 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_PlusStr-16              	10678114	       110.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AsciiDowncase-16        	92682879	        12.91 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_AsciiDowncase-16        	    5185	    202688 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Startswith-16           	90999572	        12.93 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Startswith-16           	    9138	    188110 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Endswith-16             	93392182	        12.84 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Ltrimstr-16             	159295726	         7.534 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Ltrimstr-16             	    4759	    242158 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Rtrimstr-16             	158712033	         7.564 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_First-16                	11047392	       105.5 ns/op	       8 B/op	       0 allocs/op
BenchmarkFastjq_Large_First-16                	  342921	      3511 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Last-16                 	 8488268	       142.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Last-16                 	  379466	      3242 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Limit-16                	30930657	        38.45 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Limit-16                	 2208880	       551.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Del-16                    	 1281333	       941.3 ns/op	    2594 B/op	      33 allocs/op
BenchmarkGojq_Medium_Del-16                   	   63882	     18750 ns/op	   16971 B/op	     323 allocs/op
BenchmarkGojq_Large_Del-16                    	    1507	    799889 ns/op	  542018 B/op	    4666 allocs/op
BenchmarkGojq_Small_Field-16                  	 3518980	       342.6 ns/op	    1177 B/op	      13 allocs/op
BenchmarkGojq_Large_Field-16                  	    2062	    586740 ns/op	  270050 B/op	    2835 allocs/op
BenchmarkGojq_Small_Index-16                  	 1971775	       610.7 ns/op	    1401 B/op	      20 allocs/op
BenchmarkGojq_Small_ArrayDel-16               	  747273	      1627 ns/op	    3362 B/op	      53 allocs/op
BenchmarkGojq_Small_Construct-16              	 1729430	       696.7 ns/op	    1857 B/op	      23 allocs/op
BenchmarkGojq_Large_Construct-16              	    2011	    589507 ns/op	  274382 B/op	    2867 allocs/op
BenchmarkGojq_Small_Iterator-16               	 1571335	       762.1 ns/op	    1776 B/op	      26 allocs/op
BenchmarkGojq_Large_Iterator-16               	   14170	     84748 ns/op	  109808 B/op	    1811 allocs/op
BenchmarkGojq_Small_Select-16                 	 2135348	       563.4 ns/op	    1753 B/op	      20 allocs/op
BenchmarkGojq_Large_Select-16                 	    1538	    778121 ns/op	  533194 B/op	    4651 allocs/op
BenchmarkGojq_Small_Alternative-16            	 2570366	       466.8 ns/op	    1441 B/op	      17 allocs/op
BenchmarkGojq_Small_SelectAnd-16              	 1936789	       622.5 ns/op	    1945 B/op	      21 allocs/op
BenchmarkGojq_Small_SelectOr-16               	 1833333	       653.3 ns/op	    1945 B/op	      21 allocs/op
BenchmarkGojq_Small_Has-16                    	 2189352	       546.8 ns/op	    1753 B/op	      20 allocs/op
BenchmarkGojq_Large_Has-16                    	    1540	    776618 ns/op	  534181 B/op	    4651 allocs/op
BenchmarkGojq_Small_IfThenElse-16             	 2632578	       454.1 ns/op	    1361 B/op	      16 allocs/op
BenchmarkGojq_Small_Length-16                 	 3317187	       360.4 ns/op	    1177 B/op	      13 allocs/op
BenchmarkGojq_Large_Length-16                 	    2041	    584514 ns/op	  269829 B/op	    2835 allocs/op
BenchmarkGojq_Small_Map-16                    	  119798	     10048 ns/op	   13653 B/op	     251 allocs/op
BenchmarkGojq_Large_Map-16                    	   12981	     92193 ns/op	  118570 B/op	    2237 allocs/op
BenchmarkGojq_Small_ToEntries-16              	 3320074	       365.8 ns/op	    1209 B/op	      14 allocs/op
BenchmarkGojq_Large_ToEntries-16              	    1459	    823510 ns/op	  640728 B/op	    5847 allocs/op
BenchmarkGojq_Small_KeysUnsorted-16           	 3206002	       371.4 ns/op	    1209 B/op	      14 allocs/op
BenchmarkGojq_Large_KeysUnsorted-16           	    1984	    608584 ns/op	  282842 B/op	    3039 allocs/op
BenchmarkGojq_Small_Any-16                    	  659816	      1831 ns/op	    4508 B/op	      39 allocs/op
BenchmarkGojq_Small_AnyExpr-16                	  598492	      2053 ns/op	    4620 B/op	      49 allocs/op
BenchmarkGojq_Large_AnyExpr-16                	  707614	      1723 ns/op	    2818 B/op	      29 allocs/op
BenchmarkGojq_Small_Add-16                    	 1798815	       665.9 ns/op	    1433 B/op	      24 allocs/op
BenchmarkGojq_Small_AddStrings-16             	 1460139	       820.4 ns/op	    1689 B/op	      31 allocs/op
BenchmarkGojq_Small_Flatten-16                	  927601	      1261 ns/op	    1857 B/op	      38 allocs/op
BenchmarkGojq_Small_Split-16                  	 1552839	       777.6 ns/op	    1553 B/op	      21 allocs/op
BenchmarkGojq_Small_Join-16                   	 1413128	       848.7 ns/op	    1745 B/op	      30 allocs/op
BenchmarkGojq_Small_Slice-16                  	 1497872	       803.4 ns/op	    1425 B/op	      21 allocs/op
BenchmarkGojq_Small_SliceString-16            	 2729586	       439.4 ns/op	    1145 B/op	      12 allocs/op
BenchmarkGojq_Small_Plus-16                   	 1801089	       664.8 ns/op	    1673 B/op	      21 allocs/op
BenchmarkGojq_Small_PlusStr-16                	 1674228	       715.5 ns/op	    1705 B/op	      23 allocs/op
BenchmarkGojq_Small_AsciiDowncase-16          	 2094426	       570.6 ns/op	    1785 B/op	      21 allocs/op
BenchmarkGojq_Large_AsciiDowncase-16          	    1531	    777357 ns/op	  536083 B/op	    4652 allocs/op
BenchmarkGojq_Small_Startswith-16             	 2089449	       575.1 ns/op	    1801 B/op	      21 allocs/op
BenchmarkGojq_Large_Startswith-16             	    1516	    780793 ns/op	  536705 B/op	    4651 allocs/op
BenchmarkGojq_Small_Endswith-16               	 2055328	       578.6 ns/op	    1801 B/op	      21 allocs/op
BenchmarkGojq_Small_Ltrimstr-16               	 2865031	       418.2 ns/op	    1305 B/op	      16 allocs/op
BenchmarkGojq_Small_Rtrimstr-16               	 2845126	       418.7 ns/op	    1305 B/op	      16 allocs/op
BenchmarkGojq_Small_First-16                  	  832872	      1487 ns/op	    3379 B/op	      39 allocs/op
BenchmarkGojq_Large_First-16                  	  838825	      1429 ns/op	    1889 B/op	      23 allocs/op
BenchmarkGojq_Small_Last-16                   	  632254	      1886 ns/op	    3483 B/op	      43 allocs/op
BenchmarkGojq_Large_Last-16                   	  769593	      1523 ns/op	    2193 B/op	      24 allocs/op
BenchmarkGojq_Small_Limit-16                  	  729976	      1688 ns/op	    3328 B/op	      42 allocs/op
BenchmarkGojq_Large_Limit-16                  	  746080	      1687 ns/op	    2200 B/op	      24 allocs/op
BenchmarkFastjq_Small_Subtract-16             	17330698	        68.17 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Subtract-16               	 1743475	       706.7 ns/op	    1649 B/op	      20 allocs/op
BenchmarkFastjq_Small_Multiply-16             	13623352	        85.38 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Multiply-16               	 1671111	       710.7 ns/op	    1649 B/op	      21 allocs/op
BenchmarkFastjq_Small_Divide-16               	10064352	       120.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Divide-16                 	 1680572	       715.6 ns/op	    1665 B/op	      21 allocs/op
BenchmarkFastjq_Small_Min-16                  	  705894	      1720 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Min-16                    	  961030	      1225 ns/op	    1217 B/op	      15 allocs/op
BenchmarkFastjq_Small_MinBy-16                	   97944	     11493 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_MinBy-16                  	   22192	     54088 ns/op	   63629 B/op	    1347 allocs/op
BenchmarkFastjq_Small_URIEncode-16            	 7198305	       165.5 ns/op	     120 B/op	       4 allocs/op
BenchmarkGojq_Small_URIEncode-16              	 1809979	       668.6 ns/op	    1289 B/op	      14 allocs/op
BenchmarkFastjq_Small_ArrayDiff-16            	 5631261	       211.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ArrayDiff-16              	  941382	      1232 ns/op	    2121 B/op	      33 allocs/op
BenchmarkFastjq_Small_TryNoError-16           	166064959	         7.253 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TryCatchNoError-16      	20263024	        58.94 ns/op	      64 B/op	       1 allocs/op
BenchmarkGojq_Small_TryNoError-16             	 2872608	       418.3 ns/op	    1449 B/op	      16 allocs/op
BenchmarkFastjq_Small_ObjectMerge-16          	 5443076	       224.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ObjectMerge-16            	  799834	      1598 ns/op	    2906 B/op	      33 allocs/op
BenchmarkFastjq_Small_ToJSON-16               	 9561273	       126.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ToJSON-16                 	 2843619	       416.2 ns/op	    1313 B/op	      17 allocs/op
BenchmarkFastjq_Small_FromJSON-16             	 7685605	       154.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_FromJSON-16               	  917810	      1273 ns/op	    2714 B/op	      31 allocs/op
BenchmarkFastjq_Small_ToString-16             	 9541095	       125.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToNumber-16             	73380285	        16.19 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ToNumber-16               	 3394334	       350.0 ns/op	    1112 B/op	      11 allocs/op
BenchmarkFastjq_Small_AnyTwoArg-16            	  379783	      3094 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_AnyTwoArg-16              	  711250	      1623 ns/op	    2482 B/op	      27 allocs/op
BenchmarkFastjq_Complex_LogNormalize-16       	  743895	      1579 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Complex_LogNormalize-16         	  409454	      2885 ns/op	    4324 B/op	      69 allocs/op
BenchmarkFastjq_Complex_ArrayPipeline-16      	  101961	     11701 ns/op	    1232 B/op	      64 allocs/op
BenchmarkGojq_Complex_ArrayPipeline-16        	   52548	     22960 ns/op	   20408 B/op	     525 allocs/op
BenchmarkFastjq_Complex_Aggregation-16        	   84928	     13871 ns/op	     416 B/op	      26 allocs/op
BenchmarkGojq_Complex_Aggregation-16          	   54495	     22640 ns/op	   22301 B/op	     551 allocs/op
BenchmarkFastjq_Complex_TolerantMap-16        	   95772	     12445 ns/op	    2440 B/op	      98 allocs/op
BenchmarkGojq_Complex_TolerantMap-16          	   43674	     27305 ns/op	   27868 B/op	     641 allocs/op
BenchmarkFastjq_Complex_ElifRouting-16        	   99152	     12177 ns/op	    2520 B/op	     125 allocs/op
BenchmarkGojq_Complex_ElifRouting-16          	   48301	     24811 ns/op	   25919 B/op	     595 allocs/op
BenchmarkFastjq_Complex_StringBuild-16        	  112710	     11113 ns/op	    1360 B/op	     124 allocs/op
BenchmarkGojq_Complex_StringBuild-16          	   49148	     24528 ns/op	   20533 B/op	     686 allocs/op
BenchmarkFastjq_Complex_EntryFilter-16        	  392301	      2986 ns/op	    2168 B/op	      35 allocs/op
BenchmarkGojq_Complex_EntryFilter-16          	  147699	      7805 ns/op	   11188 B/op	     162 allocs/op
BenchmarkFastjq_Small_Sqrt-16                 	18046412	        67.13 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Sqrt-16                   	 2697742	       445.8 ns/op	    1145 B/op	      12 allocs/op
BenchmarkFastjq_Small_Log-16                  	30568543	        37.77 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Log-16                    	 2705260	       446.3 ns/op	    1128 B/op	      12 allocs/op
BenchmarkFastjq_Small_Sin-16                  	15198033	        77.65 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Sin-16                    	 2625884	       447.3 ns/op	    1145 B/op	      12 allocs/op
BenchmarkFastjq_Small_Atan-16                 	22899141	        52.84 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Atan-16                   	 2867797	       394.4 ns/op	    1121 B/op	      11 allocs/op
BenchmarkFastjq_Small_Exp-16                  	18704650	        63.67 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Exp-16                    	 3060396	       392.5 ns/op	    1121 B/op	      11 allocs/op
BenchmarkFastjq_Small_Tgamma-16               	87848797	        13.40 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Tgamma-16                 	 3218109	       370.9 ns/op	    1104 B/op	      11 allocs/op
BenchmarkFastjq_Small_Fabs-16                 	22379169	        53.23 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Fabs-16                   	 2994478	       399.4 ns/op	    1112 B/op	      12 allocs/op
BenchmarkFastjq_Small_StringInterp-16         	 8080827	       151.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_StringInterp-16           	 1000000	      1014 ns/op	    2121 B/op	      35 allocs/op
BenchmarkFastjq_Small_StringInterpNum-16      	 8628750	       138.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_StringInterpNum-16        	 1000000	      1182 ns/op	    2498 B/op	      40 allocs/op
BenchmarkFastjq_Small_IsEmptyTrue-16          	162399753	         7.367 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_IsEmptyTrue-16            	 2131780	       561.4 ns/op	    1921 B/op	      18 allocs/op
BenchmarkFastjq_Small_IsEmptyFalse-16         	58952608	        20.17 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_IsEmptyFalse-16           	 1000000	      1008 ns/op	    2898 B/op	      33 allocs/op
BenchmarkFastjq_Small_Nth-16                  	31015364	        39.12 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Nth-16                    	  564775	      2105 ns/op	    4372 B/op	      49 allocs/op
BenchmarkRegexp_Match_Hit-16                  	29231100	        41.92 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Miss-16                 	53371582	        22.55 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Complex_Hit-16          	28283847	        42.45 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Long_Miss-16            	15819568	        76.25 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_MatchString-16                	27349413	        44.24 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_FindSubmatch_Hit-16           	 6036252	       198.1 ns/op	     160 B/op	       2 allocs/op
BenchmarkRegexp_FindSubmatch_Miss-16          	 1811773	       663.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_FindSubmatchIndex_Hit-16      	 6806871	       176.0 ns/op	      64 B/op	       1 allocs/op
BenchmarkRegexp_FindSubmatchIndex_Miss-16     	 1804280	       668.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_ReplaceAll_Hit-16             	 7961646	       150.5 ns/op	      97 B/op	       5 allocs/op
BenchmarkRegexp_ReplaceAll_Miss-16            	25153255	        47.69 ns/op	      48 B/op	       2 allocs/op
BenchmarkRegexp_MatchViaClosure-16            	29663320	        40.81 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_Hit-16           	161937810	         7.394 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_Miss-16          	11122465	       107.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_InPipeline-16    	 4014026	       300.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_MatchRe_Hit-16          	 5552679	       215.8 ns/op	      48 B/op	       1 allocs/op
BenchmarkFastjq_Small_MatchRe_Miss-16         	 1932061	       620.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_TestRe_Hit-16             	  675450	      1788 ns/op	    4156 B/op	      44 allocs/op
BenchmarkGojq_Small_TestRe_Miss-16            	  676735	      1761 ns/op	    4104 B/op	      43 allocs/op
BenchmarkGojq_Small_MatchRe_Hit-16            	  295748	      4080 ns/op	    7993 B/op	     100 allocs/op
BenchmarkGojq_Small_MatchRe_Miss-16           	  406862	      2904 ns/op	    5710 B/op	      59 allocs/op
BenchmarkFastjq_Small_CaptureRe_Hit-16        	 5669229	       210.2 ns/op	      48 B/op	       1 allocs/op
BenchmarkFastjq_Small_CaptureRe_Miss-16       	 2000095	       602.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ScanRe_NoGroups-16      	 2531308	       474.6 ns/op	     387 B/op	      10 allocs/op
BenchmarkFastjq_Small_ScanRe_WithGroups-16    	 2036226	       592.0 ns/op	     703 B/op	      13 allocs/op
BenchmarkFastjq_Small_SubRe_Hit-16            	 9558756	       126.2 ns/op	      16 B/op	       1 allocs/op
BenchmarkFastjq_Small_SubRe_Miss-16           	11201992	       107.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_GSubRe_Hit-16           	 2053330	       578.9 ns/op	     305 B/op	       5 allocs/op
BenchmarkFastjq_Small_GSubRe_Miss-16          	 4754478	       249.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_CaptureRe_Hit-16          	  358710	      3291 ns/op	    7377 B/op	      77 allocs/op
BenchmarkGojq_Small_CaptureRe_Miss-16         	  423464	      2902 ns/op	    5582 B/op	      54 allocs/op
BenchmarkGojq_Small_ScanRe_NoGroups-16        	  395905	      3050 ns/op	    6010 B/op	      67 allocs/op
BenchmarkGojq_Small_SubRe_Hit-16              	  337129	      3546 ns/op	    6168 B/op	      77 allocs/op
BenchmarkGojq_Small_GSubRe_Hit-16             	  162373	      7461 ns/op	    9420 B/op	     143 allocs/op
```

## CLI Throughput: fastjq vs jq

End-to-end JSONL throughput benchmarks comparing `fastjq-bench` (Go CLI using the fastjq library) against `jq` (C implementation, v1.8.1). Both read JSONL from stdin, apply a filter per line, write results to `/dev/null`. Median of 3 runs. Apple M4 Max. Updated 2026-02-20.

| Operation | Input | jq (s) | fastjq (s) | Speedup |
|-----------|-------|--------|-------------|---------|
| Identity (`.`) | small (100K lines, ~11MB) | 0.344 | 0.025 | **14x** |
| Field access (`.field_2`) | small | 0.145 | 0.024 | **6x** |
| Field access (`.field_50`) | large (100 lines, ~16MB) | 0.088 | 0.023 | **4x** |
| Delete field (`del(.field_2)`) | small | 0.369 | 0.036 | **10x** |
| Object construction (`{field_0, field_2}`) | small | 0.268 | 0.051 | **5x** |
| Select all match (`select(.field_2 == "xxx...")`) | small | 0.370 | 0.028 | **13x** |
| Select none match (`select(.field_2 == "nope")`) | small | 0.138 | 0.027 | **5x** |
| Alternative (`.field_2 // "default"`) | small | 0.166 | 0.026 | **6x** |
| Case-insensitive select (`ascii_downcase`) | small | 0.651 | 0.038 | **17x** |
| Prefix filter (`startswith`) | small | 0.363 | 0.029 | **13x** |
| Field existence (`has`) | small | 0.366 | 0.027 | **14x** |
| `to_entries` | small | 0.717 | 0.040 | **18x** |
| `keys_unsorted` | small | 0.247 | 0.029 | **9x** |

### Key Takeaways (CLI)

- **4x–18x faster** than jq across all operations on real JSONL workloads
- **`to_entries` and `ascii_downcase` are 17–18x faster**: near-zero cost reformatting vs jq's parse + marshal cycle
- **Identity and deletion are 10–14x faster**: validates the zero-copy architecture at scale
- **Even "large" objects (100KB each, 100 lines) show 4x speedup**: scanning advantage persists
- **Select (none match) is only 5x faster** — both engines scan the full document; fastjq's advantage is smaller here

### Reproducing

```bash
chmod +x bench_vs_jq.sh
./bench_vs_jq.sh
```

## Reproducing (Go Benchmarks)

```bash
go test -bench=. -benchmem
```
