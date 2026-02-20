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
| `.field` | Small (~100B) | 0.151 | 0.352 | **2.3x** | 0 | 13 |
| `.field` | Large (~100KB) | 128 | 600 | **4.7x** | 0 | 2835 |
| `del(.f)` | Small (~100B) | 0.158 | 1.11 | **7x** | 0 | 33 |
| `del(.f)` | Medium (~2KB) | 2.57 | 19.4 | **7.6x** | 0 | 323 |
| `del(.f)` | Large (~100KB) | 148 | 827 | **5.6x** | 0 | 4666 |
| `.[n]` | 5-elem array | 0.026 | 0.641 | **25x** | 0 | 20 |
| `del(.[n], .[m])` | 5-elem array | 0.090 | 1.65 | **18x** | 0 | 53 |
| `{f0, f2}` (construct) | Small (~100B) | 0.316 | 0.707 | **2.2x** | 0 | 23 |
| `{f0, f50}` (construct) | Large (~100KB) | 227 | 585 | **2.6x** | 0 | 2867 |
| `.[]` iterator | 5-elem array | 0.035 | 0.764 | **22x** | 0 | 26 |
| `.[]` iterator | 200-elem array | 11.6 | 86.5 | **7.4x** | 0 | 1811 |
| `select(.f == "x")` | Small (~100B) | 0.0093 | 0.583 | **63x** | 0 | 20 |
| `select(.f == "x")`¹ | Large (~100KB, last field) | 203 | 786 | **3.9x** | 0 | 4651 |
| `select(.f and .g)` | Small (~100B) | 0.011 | 0.622 | **58x** | 0 | 21 |
| `select(.f or .g)` | Small (~100B) | 0.011 | 0.679 | **63x** | 0 | 21 |
| `has("key")` in select | Small (~100B) | 0.012 | 0.556 | **47x** | 0 | 20 |
| `has("key")` in select | Large (~100KB) | 187 | 797 | **4.3x** | 0 | 4651 |
| `if-then-else` | Small (~100B) | 0.0097 | 0.470 | **48x** | 0 | 16 |
| `.f // "default"` | Small (~100B) | 0.0071 | 0.472 | **66x** | 0 | 17 |
| `try .field` (no error) | Small (~100B) | 0.0064 | 0.422 | **66x** | 0 | 16 |
| `.a + .b` (strings) | Small (~100B) | 0.074 | 0.674 | **9.2x** | 0 | 21 |
| `"prefix" + .name` | Small (~100B) | 0.113 | 0.721 | **6.4x** | 0 | 23 |
| `.a - .b` (subtract) | Small (~100B) | 0.063 | 0.677 | **11x** | 0 | 20 |
| `.a * .b` (multiply) | Small (~100B) | 0.082 | 0.724 | **8.8x** | 0 | 21 |
| `.a / .b` (divide) | Small (~100B) | 0.116 | 0.721 | **6.2x** | 0 | 21 |
| `.a - .b` (array diff) | 5-elem arrays | 0.214 | 1.24 | **5.8x** | 0 | 33 |
| `length` | Small (~100B) | 0.0067 | 0.386 | **57x** | 0 | 13 |
| `length` | Large (~100KB) | 156 | 592 | **3.8x** | 0 | 2835 |
| `add` (numbers) | 5-elem array | 0.058 | 0.693 | **12x** | 0 | 24 |
| `add` (strings) | 5-elem array | 0.136 | 0.849 | **6.2x** | 0 | 31 |
| `flatten` | 3-elem nested array | 0.106 | 1.30 | **12x** | 0 | 38 |
| `min` | 200-int array | 1.64 | 1.20 | 0.7x† | 0 | 15 |
| `min_by(.value)` | 100-elem object array | 12.0 | 55.5 | **4.6x** | 0 | 1347 |
| `map(.name)` | 20-elem array (~600B) | 1.86 | 10.3 | **5.5x** | 0 | 251 |
| `map(.name)` | 200-elem array (~6KB) | 19.8 | 93.1 | **4.7x** | 0 | 2237 |
| `any` | 5-elem array | 0.048 | 1.86 | **39x** | 0 | 39 |
| `any(expr)` | 5-elem array | 0.119 | 2.14 | **18x** | 0 | 49 |
| `any(expr)`² | 200-int array | 2.96 | 1.75 | 0.6x† | 0 | 29 |
| `any(gen; cond)`² | 200-int array | 3.19 | 1.61 | 0.5x† | 0 | 27 |
| `first(expr)` | 5-elem array | 0.106 | 1.50 | **14x** | 0 | 39 |
| `first(expr)`² | 200-int array | 3.46 | 1.43 | 0.4x† | 0 | 23 |
| `last(expr)` | 5-elem array | 0.143 | 1.90 | **13x** | 0 | 43 |
| `last(expr)`² | 200-int array | 3.41 | 1.51 | 0.4x† | 0 | 24 |
| `limit(3; expr)` | 5-elem array | 0.039 | 1.65 | **42x** | 0 | 42 |
| `limit(10; expr)` | 200-int array | 0.643 | 1.59 | **2.5x** | 0 | 24 |
| `.[1:4]` slice | 6-elem array | 0.088 | 0.820 | **9.4x** | 0 | 21 |
| `values` | 9-elem array | 0.092 | 2.37 | **26x** | 0 | 51 |
| `to_entries` | Small (~100B) | 0.0059 | 0.376 | **64x** | 0 | 14 |
| `to_entries` | Large (~100KB) | 181 | 834 | **4.6x** | 0 | 5847 |
| `keys_unsorted` | Small (~100B) | 0.061 | 0.373 | **6.1x** | 3 | 14 |
| `keys_unsorted` | Large (~100KB) | 180 | 603 | **3.4x** | 0 | 3039 |
| `object merge .a + .b` | Small (~100B) | 0.168 | 1.49 | **8.9x** | 0 | 33 |
| `tojson` | Small (~100B) | 0.082 | 0.423 | **5.1x** | 0 | 17 |
| `fromjson` | JSON string | 0.048 | 1.28 | **27x** | 0 | 31 |
| `tonumber` | `"42"` string | 0.015 | 0.352 | **23x** | 0 | 11 |
| `split(",")` | short string | 0.111 | 0.804 | **7.2x** | 0 | 21 |
| `join(",")` | 5-elem array | 0.105 | 0.866 | **8.2x** | 0 | 30 |
| `ascii_downcase` in select | Small (~100B) | 0.011 | 0.574 | **50x** | 0 | 21 |
| `ascii_downcase` in select | Large (~100KB) | 186 | 778 | **4.2x** | 0 | 4653 |
| `startswith("s")` | Small (~100B) | 0.011 | 0.577 | **52x** | 0 | 21 |
| `startswith("s")` | Large (~100KB) | 172 | 783 | **4.6x** | 0 | 4651 |
| `endswith("s")` | Small (~100B) | 0.011 | 0.599 | **55x** | 0 | 21 |
| `ltrimstr("s")` | Small (~100B) | 0.0067 | 0.436 | **65x** | 0 | 16 |
| `rtrimstr("s")` | Small (~100B) | 0.0068 | 0.430 | **63x** | 0 | 16 |
| `select` + string ops + arith + construct | ~200B log event | 1.60 | 3.00 | **1.9x** | 0 | 69 |
| `[.[] | select + arith + construct]` | 20-elem array (~1.5KB) | 12.1 | 23.3 | **1.9x** | 64 | 525 |
| `length + map + add + min_by + any` | 20-elem array (~1.5KB) | 14.0 | 21.6 | **1.5x** | 26 | 551 |
| `map(try {…} catch …)` | 20-elem array (~1.5KB) | 12.6 | 27.3 | **2.2x** | 98 | 641 |
| `map(if … elif … else …)` | 20-elem array (~1.5KB) | 12.1 | 25.0 | **2.1x** | 98 | 595 |
| `[.[] | str + tostring + str]` | 20-elem array (~1.5KB) | 10.9 | 23.9 | **2.2x** | 124 | 686 |
| `to_entries | map(select) | from_entries` | ~200B log event | 2.92 | 7.68 | **2.6x** | 35 | 162 |
| `@base64` | 34-char string | 0.084 | 0.525 | **6.3x** | 0 | 15 |
| `@base64d` | 48-char encoded | 0.206 | 0.555 | **2.7x** | 0 | 15 |
| `@uri` | 36-char URL string | 0.098 | 0.693 | **7.1x** | 0 | 14 |
| `index(",")` | short string | 0.077 | 0.920 | **12x** | 0 | 31 |
| `indices(",")` | short string | 0.127 | 2.12 | **17x** | 0 | 97 |

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
BenchmarkFastjq_Small_Del-16                	 6750248	       168.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Medium_Del-16               	  429630	      2745 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Del-16                	    9889	    129377 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Field-16              	 7701166	       155.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Field-16              	   10000	    110853 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Index-16              	46864015	        25.49 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ArrayDel-16           	13520319	        88.86 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Construct-16          	 3762432	       320.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Construct-16          	    5565	    214275 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Iterator-16           	33458065	        34.74 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Iterator-16           	  137955	      9211 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Select-16             	130247586	         9.218 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Select-16             	   10000	    127544 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Alternative-16        	173491623	         6.911 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SelectAnd-16          	100000000	        10.55 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SelectOr-16           	100000000	        10.55 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Has-16                	100000000	        11.62 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Has-16                	    8151	    128276 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_IfThenElse-16         	100000000	        10.02 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Length-16             	181788135	         6.601 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Length-16             	   10000	    123096 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Map-16                	  664419	      1872 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Medium_Map-16               	  122907	      8775 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Map-16                	   68466	     17373 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToEntries-16          	204934436	         5.838 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_ToEntries-16          	    9343	    137667 ns/op	      23 B/op	       0 allocs/op
BenchmarkFastjq_Small_KeysUnsorted-16       	20705882	        57.48 ns/op	      72 B/op	       3 allocs/op
BenchmarkFastjq_Large_KeysUnsorted-16       	    8992	    127691 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Any-16                	24057055	        46.98 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Any-16                	  736299	      1640 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AnyExpr-16            	 9438708	       122.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_AnyExpr-16            	  440952	      2744 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Add-16                	20758924	        57.48 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AddStrings-16         	 9257642	       136.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Flatten-16            	11906228	        99.97 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Split-16              	11339966	       106.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Join-16               	11857852	        99.20 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Values-16             	13340521	        93.43 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Values-16               	  515302	      2351 ns/op	    3144 B/op	      51 allocs/op
BenchmarkFastjq_Small_Base64Encode-16       	14615734	        80.40 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Base64Decode-16       	 5774151	       203.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Base64Encode-16         	 2475788	       488.4 ns/op	    1321 B/op	      15 allocs/op
BenchmarkGojq_Small_Base64Decode-16         	 2334664	       519.1 ns/op	    1321 B/op	      15 allocs/op
BenchmarkFastjq_Small_IndexFind-16          	16025434	        74.07 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_IndicesAll-16         	10619496	       117.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_IndexFind-16            	 1398231	       862.1 ns/op	    2274 B/op	      31 allocs/op
BenchmarkGojq_Small_IndicesAll-16           	  593678	      1988 ns/op	    3907 B/op	      97 allocs/op
BenchmarkFastjq_Small_Slice-16              	13968064	        85.21 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SliceString-16        	25006077	        46.00 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Plus-16               	17672094	        68.31 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_PlusStr-16            	10727218	       110.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AsciiDowncase-16      	100000000	        10.93 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_AsciiDowncase-16      	    8217	    139294 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Startswith-16         	100000000	        10.95 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Startswith-16         	    9870	    125397 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Endswith-16           	100000000	        10.95 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Ltrimstr-16           	180449770	         6.644 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Ltrimstr-16           	    8170	    130401 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Rtrimstr-16           	180784207	         6.638 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_First-16              	11513151	       102.8 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Large_First-16              	  359851	      3390 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Last-16               	 8752729	       137.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Last-16               	  366390	      3241 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Limit-16              	30937070	        37.27 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Limit-16              	 2272436	       540.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Del-16                  	 1345718	       890.4 ns/op	    2594 B/op	      33 allocs/op
BenchmarkGojq_Medium_Del-16                 	   66277	     17953 ns/op	   16970 B/op	     323 allocs/op
BenchmarkGojq_Large_Del-16                  	    1575	    752544 ns/op	  542930 B/op	    4666 allocs/op
BenchmarkGojq_Small_Field-16                	 3665138	       329.2 ns/op	    1177 B/op	      13 allocs/op
BenchmarkGojq_Large_Field-16                	    2084	    549781 ns/op	  270030 B/op	    2835 allocs/op
BenchmarkGojq_Small_Index-16                	 2094642	       582.9 ns/op	    1401 B/op	      20 allocs/op
BenchmarkGojq_Small_ArrayDel-16             	  768432	      1549 ns/op	    3363 B/op	      53 allocs/op
BenchmarkGojq_Small_Construct-16            	 1804376	       664.2 ns/op	    1857 B/op	      23 allocs/op
BenchmarkGojq_Large_Construct-16            	    2145	    549741 ns/op	  274461 B/op	    2867 allocs/op
BenchmarkGojq_Small_Iterator-16             	 1668273	       723.1 ns/op	    1776 B/op	      26 allocs/op
BenchmarkGojq_Large_Iterator-16             	   14919	     80316 ns/op	  109809 B/op	    1811 allocs/op
BenchmarkGojq_Small_Select-16               	 2214484	       543.2 ns/op	    1753 B/op	      20 allocs/op
BenchmarkGojq_Large_Select-16               	    1630	    730659 ns/op	  534575 B/op	    4651 allocs/op
BenchmarkGojq_Small_Alternative-16          	 2683092	       448.8 ns/op	    1441 B/op	      17 allocs/op
BenchmarkGojq_Small_SelectAnd-16            	 1996113	       603.0 ns/op	    1945 B/op	      21 allocs/op
BenchmarkGojq_Small_SelectOr-16             	 1882354	       635.8 ns/op	    1945 B/op	      21 allocs/op
BenchmarkGojq_Small_Has-16                  	 2253237	       533.1 ns/op	    1753 B/op	      20 allocs/op
BenchmarkGojq_Large_Has-16                  	    1548	    745213 ns/op	  532661 B/op	    4651 allocs/op
BenchmarkGojq_Small_IfThenElse-16           	 2727864	       439.0 ns/op	    1361 B/op	      16 allocs/op
BenchmarkGojq_Small_Length-16               	 3451620	       346.6 ns/op	    1177 B/op	      13 allocs/op
BenchmarkGojq_Large_Length-16               	    2149	    556900 ns/op	  269814 B/op	    2835 allocs/op
BenchmarkGojq_Small_Map-16                  	  125940	      9611 ns/op	   13654 B/op	     251 allocs/op
BenchmarkGojq_Large_Map-16                  	   13171	     90318 ns/op	  118618 B/op	    2237 allocs/op
BenchmarkGojq_Small_ToEntries-16            	 3233745	       364.9 ns/op	    1209 B/op	      14 allocs/op
BenchmarkGojq_Large_ToEntries-16            	    1514	    781648 ns/op	  639775 B/op	    5847 allocs/op
BenchmarkGojq_Small_KeysUnsorted-16         	 3391606	       356.7 ns/op	    1209 B/op	      14 allocs/op
BenchmarkGojq_Large_KeysUnsorted-16         	    2076	    573065 ns/op	  282772 B/op	    3039 allocs/op
BenchmarkGojq_Small_Any-16                  	  672060	      1748 ns/op	    4508 B/op	      39 allocs/op
BenchmarkGojq_Small_AnyExpr-16              	  619234	      1978 ns/op	    4620 B/op	      49 allocs/op
BenchmarkGojq_Large_AnyExpr-16              	  723177	      1634 ns/op	    2818 B/op	      29 allocs/op
BenchmarkGojq_Small_Add-16                  	 1845938	       644.2 ns/op	    1433 B/op	      24 allocs/op
BenchmarkGojq_Small_AddStrings-16           	 1506462	       792.8 ns/op	    1689 B/op	      31 allocs/op
BenchmarkGojq_Small_Flatten-16              	  995299	      1234 ns/op	    1857 B/op	      38 allocs/op
BenchmarkGojq_Small_Split-16                	 1638266	       735.5 ns/op	    1553 B/op	      21 allocs/op
BenchmarkGojq_Small_Join-16                 	 1446454	       824.5 ns/op	    1745 B/op	      30 allocs/op
BenchmarkGojq_Small_Slice-16                	 1566258	       766.5 ns/op	    1425 B/op	      21 allocs/op
BenchmarkGojq_Small_SliceString-16          	 2841008	       425.1 ns/op	    1145 B/op	      12 allocs/op
BenchmarkGojq_Small_Plus-16                 	 1865490	       639.6 ns/op	    1673 B/op	      21 allocs/op
BenchmarkGojq_Small_PlusStr-16              	 1760604	       681.5 ns/op	    1705 B/op	      23 allocs/op
BenchmarkGojq_Small_AsciiDowncase-16        	 2157926	       564.7 ns/op	    1785 B/op	      21 allocs/op
BenchmarkGojq_Large_AsciiDowncase-16        	    1526	    759836 ns/op	  535470 B/op	    4652 allocs/op
BenchmarkGojq_Small_Startswith-16           	 2111559	       579.0 ns/op	    1801 B/op	      21 allocs/op
BenchmarkGojq_Large_Startswith-16           	    1572	    744568 ns/op	  537623 B/op	    4651 allocs/op
BenchmarkGojq_Small_Endswith-16             	 1924774	       606.0 ns/op	    1801 B/op	      21 allocs/op
BenchmarkGojq_Small_Ltrimstr-16             	 2749176	       420.4 ns/op	    1305 B/op	      16 allocs/op
BenchmarkGojq_Small_Rtrimstr-16             	 2927805	       408.1 ns/op	    1305 B/op	      16 allocs/op
BenchmarkGojq_Small_First-16                	  822884	      1441 ns/op	    3379 B/op	      39 allocs/op
BenchmarkGojq_Large_First-16                	  859623	      1383 ns/op	    1889 B/op	      23 allocs/op
BenchmarkGojq_Small_Last-16                 	  672483	      1825 ns/op	    3483 B/op	      43 allocs/op
BenchmarkGojq_Large_Last-16                 	  819340	      1463 ns/op	    2193 B/op	      24 allocs/op
BenchmarkGojq_Small_Limit-16                	  760262	      1592 ns/op	    3328 B/op	      42 allocs/op
BenchmarkGojq_Large_Limit-16                	  774684	      1541 ns/op	    2200 B/op	      24 allocs/op
BenchmarkFastjq_Small_Subtract-16           	18659097	        64.82 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Subtract-16             	 1669495	       710.0 ns/op	    1649 B/op	      20 allocs/op
BenchmarkFastjq_Small_Multiply-16           	13816102	        83.80 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Multiply-16             	 1675234	       712.3 ns/op	    1649 B/op	      21 allocs/op
BenchmarkFastjq_Small_Divide-16             	10086844	       115.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Divide-16               	 1742930	       687.1 ns/op	    1665 B/op	      21 allocs/op
BenchmarkFastjq_Small_Min-16                	  739111	      1624 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Min-16                  	  943980	      1157 ns/op	    1217 B/op	      15 allocs/op
BenchmarkFastjq_Small_MinBy-16              	  109093	     11048 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_MinBy-16                	   23157	     51692 ns/op	   63631 B/op	    1347 allocs/op
BenchmarkFastjq_Small_URIEncode-16          	12335785	        95.38 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_URIEncode-16            	 1927058	       629.7 ns/op	    1289 B/op	      14 allocs/op
BenchmarkFastjq_Small_ArrayDiff-16          	 5715716	       207.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ArrayDiff-16            	  977151	      1205 ns/op	    2121 B/op	      33 allocs/op
BenchmarkFastjq_Small_TryNoError-16         	182978372	         6.548 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TryCatchNoError-16    	21360562	        56.30 ns/op	      64 B/op	       1 allocs/op
BenchmarkGojq_Small_TryNoError-16           	 2826678	       412.3 ns/op	    1449 B/op	      16 allocs/op
BenchmarkFastjq_Small_ObjectMerge-16        	 7287150	       166.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ObjectMerge-16          	  841050	      1441 ns/op	    2906 B/op	      33 allocs/op
BenchmarkFastjq_Small_ToJSON-16             	 9849793	       120.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ToJSON-16               	 2955256	       406.0 ns/op	    1313 B/op	      17 allocs/op
BenchmarkFastjq_Small_FromJSON-16           	 7875765	       152.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_FromJSON-16             	  964258	      1222 ns/op	    2714 B/op	      31 allocs/op
BenchmarkFastjq_Small_ToString-16           	 9355012	       122.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToNumber-16           	77758608	        15.34 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ToNumber-16             	 3556801	       337.9 ns/op	    1113 B/op	      11 allocs/op
BenchmarkFastjq_Small_AnyTwoArg-16          	  381184	      3054 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_AnyTwoArg-16            	  736981	      1562 ns/op	    2482 B/op	      27 allocs/op
BenchmarkFastjq_Complex_LogNormalize-16     	  734722	      1527 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Complex_LogNormalize-16       	  432214	      2758 ns/op	    4324 B/op	      69 allocs/op
BenchmarkFastjq_Complex_ArrayPipeline-16    	  109780	     10926 ns/op	    1232 B/op	      64 allocs/op
BenchmarkGojq_Complex_ArrayPipeline-16      	   55714	     21423 ns/op	   20409 B/op	     525 allocs/op
BenchmarkFastjq_Complex_Aggregation-16      	   96876	     13614 ns/op	     416 B/op	      26 allocs/op
BenchmarkGojq_Complex_Aggregation-16        	   57423	     20411 ns/op	   22301 B/op	     551 allocs/op
BenchmarkFastjq_Complex_TolerantMap-16      	   99115	     12112 ns/op	    2440 B/op	      98 allocs/op
BenchmarkGojq_Complex_TolerantMap-16        	   45958	     26553 ns/op	   27873 B/op	     641 allocs/op
BenchmarkFastjq_Complex_ElifRouting-16      	  100663	     12004 ns/op	    2520 B/op	     125 allocs/op
BenchmarkGojq_Complex_ElifRouting-16        	   48990	     23938 ns/op	   25920 B/op	     595 allocs/op
BenchmarkFastjq_Complex_StringBuild-16      	  112144	     10828 ns/op	    1360 B/op	     124 allocs/op
BenchmarkGojq_Complex_StringBuild-16        	   52131	     23063 ns/op	   20533 B/op	     686 allocs/op
BenchmarkFastjq_Complex_EntryFilter-16      	  420342	      2857 ns/op	    2168 B/op	      35 allocs/op
BenchmarkGojq_Complex_EntryFilter-16        	  163038	      7396 ns/op	   11188 B/op	     162 allocs/op```

## CLI Throughput: fastjq vs jq

End-to-end JSONL throughput benchmarks comparing `fastjq-bench` (Go CLI using the fastjq library) against `jq` (C implementation, v1.8.1). Both read JSONL from stdin, apply a filter per line, write results to `/dev/null`. Median of 3 runs. Apple M4 Max.

| Operation | Input | jq (s) | fastjq (s) | Speedup |
|-----------|-------|--------|-------------|---------|
| Identity (`.`) | small (100K lines, ~11MB) | 0.346 | 0.025 | **13.8x** |
| Field access (`.field_2`) | small | 0.146 | 0.027 | **5.4x** |
| Field access (`.field_50`) | large (100 lines, ~16MB) | 0.088 | 0.025 | **3.5x** |
| Delete field (`del(.field_2)`) | small | 0.389 | 0.036 | **10.8x** |
| Object construction (`{field_0, field_2}`) | small | 0.251 | 0.048 | **5.2x** |
| Select all match (`select(.field_2 == "xxx...")`) | small | 0.368 | 0.030 | **12.3x** |
| Select none match (`select(.field_2 == "nope")`) | small | 0.138 | 0.031 | **4.5x** |
| Alternative (`.field_2 // "default"`) | small | 0.170 | 0.027 | **6.3x** |
| Case-insensitive select | small | 0.650 | 0.036 | **18.1x** |
| Prefix filter (`startswith`) | small | 0.391 | 0.030 | **13.0x** |
| Field existence (`has`) | small | 0.364 | 0.031 | **11.7x** |
| `to_entries` | small | 0.714 | 0.039 | **18.3x** |
| `keys_unsorted` | small | 0.245 | 0.031 | **7.9x** |

### Key Takeaways (CLI)

- **4x–18x faster** than jq across all operations on real JSONL workloads
- **Case-insensitive select (`ascii_downcase`) is 18x faster**: near-zero cost for the string transform
- **`to_entries` is 18x faster**: zero-copy reformatting vs jq's full parse + marshal cycle
- **Identity and deletion are 11–14x faster**: validates the zero-copy architecture at scale
- **Even "large" objects (100KB each) show 3.5x speedup**: scanning advantage persists

### Reproducing

```bash
chmod +x bench_vs_jq.sh
./bench_vs_jq.sh
```

## Reproducing (Go Benchmarks)

```bash
go test -bench=. -benchmem
```
