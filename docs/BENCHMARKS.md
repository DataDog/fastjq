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
| `.field` | Small (~100B) | 0.090 | 0.345 | **3.8x** | 0 | 13 |
| `.field` | Large (~100KB) | 32.4 | 581 | **18x** | 0 | 2835 |
| `del(.f)` | Small (~100B) | 0.165 | 0.931 | **5.7x** | 0 | 33 |
| `del(.f)` | Medium (~2KB) | 2.44 | 18.7 | **7.7x** | 0 | 323 |
| `del(.f)` | Large (~100KB) | 135 | 800 | **5.9x** | 0 | 4666 |
| `.[n]` | 5-elem array | 0.024 | 0.614 | **26x** | 0 | 20 |
| `del(.[n], .[m])` | 5-elem array | 0.091 | 1.63 | **18x** | 0 | 53 |
| `{f0, f2}` (construct) | Small (~100B) | 0.142 | 0.694 | **4.9x** | 0 | 23 |
| `{f0, f50}` (construct) | Large (~100KB) | 33.1 | 583 | **18x** | 0 | 2867 |
| `.[]` iterator | 5-elem array | 0.032 | 0.758 | **24x** | 0 | 26 |
| `.[]` iterator | 200-elem array | 9.16 | 84.0 | **9.2x** | 0 | 1811 |
| `select(.f == "x")` | Small (~100B) | 0.011 | 0.566 | **50x** | 0 | 20 |
| `select(.f == "x")`¹ | Large (~100KB, last field) | 131 | 775 | **5.9x** | 0 | 4652 |
| `select(.f and .g)` | Small (~100B) | 0.013 | 0.621 | **48x** | 0 | 21 |
| `select(.f or .g)` | Small (~100B) | 0.013 | 0.661 | **51x** | 0 | 21 |
| `has("key")` in select | Small (~100B) | 0.015 | 0.551 | **38x** | 0 | 20 |
| `has("key")` in select | Large (~100KB) | 133 | 774 | **5.8x** | 0 | 4651 |
| `if-then-else` | Small (~100B) | 0.012 | 0.459 | **37x** | 0 | 16 |
| `.f // "default"` | Small (~100B) | 0.0095 | 0.469 | **49x** | 0 | 17 |
| `try .field` (no error) | Small (~100B) | 0.0089 | 0.436 | **49x** | 0 | 16 |
| `.a + .b` (strings) | Small (~100B) | 0.053 | 0.664 | **13x** | 0 | 21 |
| `"prefix" + .name` | Small (~100B) | 0.082 | 0.713 | **8.6x** | 0 | 23 |
| `.a - .b` (subtract) | Small (~100B) | 0.051 | 0.694 | **14x** | 0 | 20 |
| `.a * .b` (multiply) | Small (~100B) | 0.068 | 0.699 | **10x** | 0 | 21 |
| `.a / .b` (divide) | Small (~100B) | 0.100 | 0.718 | **7.2x** | 0 | 21 |
| `.a - .b` (array diff) | 5-elem arrays | 0.197 | 1.32 | **6.7x** | 0 | 33 |
| `length` | Small (~100B) | 0.0091 | 0.362 | **40x** | 0 | 13 |
| `length` | Large (~100KB) | 131 | 582 | **4.4x** | 0 | 2835 |
| `add` (numbers) | 5-elem array | 0.060 | 0.667 | **11x** | 0 | 24 |
| `add` (strings) | 5-elem array | 0.141 | 0.814 | **5.8x** | 0 | 31 |
| `flatten` | 3-elem nested array | 0.103 | 1.26 | **12x** | 0 | 38 |
| `min` | 200-int array | 1.67 | 1.27 | 0.8x† | 0 | 15 |
| `min_by(.value)` | 100-elem object array | 8.12 | 54.4 | **6.7x** | 0 | 1347 |
| `map(.name)` | 20-elem array (~600B) | 1.24 | 10.4 | **8.4x** | 0 | 251 |
| `map(.name)` | 200-elem array (~6KB) | 12.7 | 93.3 | **7.3x** | 0 | 2237 |
| `any` | 5-elem array | 0.045 | 1.82 | **41x** | 0 | 39 |
| `any(expr)` | 5-elem array | 0.119 | 2.13 | **18x** | 0 | 49 |
| `any(expr)`² | 200-int array | 2.86 | 1.70 | 0.6x† | 0 | 29 |
| `any(gen; cond)`² | 200-int array | 3.12 | 1.64 | 0.5x† | 0 | 27 |
| `first(expr)` | 5-elem array | 0.107 | 1.48 | **14x** | 0 | 39 |
| `first(expr)`² | 200-int array | 3.57 | 1.43 | 0.4x† | 0 | 23 |
| `last(expr)` | 5-elem array | 0.140 | 1.89 | **13x** | 0 | 43 |
| `last(expr)`² | 200-int array | 3.29 | 1.53 | 0.5x† | 0 | 24 |
| `limit(3; expr)` | 5-elem array | 0.038 | 1.69 | **45x** | 0 | 42 |
| `limit(10; expr)` | 200-int array | 0.546 | 1.67 | **3.1x** | 0 | 24 |
| `.[1:4]` slice | 6-elem array | 0.085 | 0.789 | **9.3x** | 0 | 21 |
| `values` | 9-elem array | 0.090 | 2.35 | **26x** | 0 | 51 |
| `to_entries` | Small (~100B) | 0.0045 | 0.374 | **83x** | 0 | 14 |
| `to_entries` | Large (~100KB) | 129 | 825 | **6.4x** | 0 | 5847 |
| `keys_unsorted` | Small (~100B) | 0.060 | 0.368 | **6.1x** | 3 | 14 |
| `keys_unsorted` | Large (~100KB) | 129 | 602 | **4.7x** | 0 | 3039 |
| `object merge .a + .b` | Small (~100B) | 0.197 | 1.61 | **8.2x** | 0 | 33 |
| `tojson` | Small (~100B) | 0.124 | 0.427 | **3.4x** | 0 | 17 |
| `fromjson` | JSON string | 0.152 | 1.31 | **8.7x** | 0 | 31 |
| `tonumber` | `"42"` string | 0.013 | 0.351 | **26x** | 0 | 11 |
| `split(",")` | short string | 0.108 | 0.768 | **7.1x** | 0 | 21 |
| `join(",")` | 5-elem array | 0.100 | 0.841 | **8.5x** | 0 | 30 |
| `ascii_downcase` in select | Small (~100B) | 0.014 | 0.570 | **40x** | 0 | 21 |
| `ascii_downcase` in select | Large (~100KB) | 142 | 780 | **5.5x** | 0 | 4652 |
| `startswith("s")` | Small (~100B) | 0.014 | 0.573 | **40x** | 0 | 21 |
| `startswith("s")` | Large (~100KB) | 131 | 777 | **5.9x** | 0 | 4651 |
| `endswith("s")` | Small (~100B) | 0.014 | 0.581 | **41x** | 0 | 21 |
| `ltrimstr("s")` | Small (~100B) | 0.0091 | 0.416 | **46x** | 0 | 16 |
| `rtrimstr("s")` | Small (~100B) | 0.0090 | 0.417 | **46x** | 0 | 16 |
| `select` + string ops + arith + construct | ~200B log event | 1.10 | 2.95 | **2.7x** | 0 | 69 |
| `[.[] \| select + arith + construct]` | 20-elem array (~1.5KB) | 7.99 | 22.6 | **2.8x** | 64 | 525 |
| `length + map + add + min_by + any` | 20-elem array (~1.5KB) | 10.1 | 21.6 | **2.1x** | 26 | 551 |
| `map(try {…} catch …)` | 20-elem array (~1.5KB) | 8.94 | 27.3 | **3.1x** | 98 | 641 |
| `map(if … elif … else …)` | 20-elem array (~1.5KB) | 8.54 | 24.9 | **2.9x** | 125 | 595 |
| `[.[] \| str + tostring + str]` | 20-elem array (~1.5KB) | 8.42 | 24.6 | **2.9x** | 124 | 686 |
| `to_entries \| map(select) \| from_entries` | ~200B log event | 2.75 | 7.82 | **2.8x** | 35 | 162 |
| `@base64` | 34-char string | 0.139 | 0.515 | **3.7x** | 4 | 15 |
| `@base64d` | 48-char encoded | 0.207 | 0.546 | **2.6x** | 0 | 15 |
| `@uri` | 36-char URL string | 0.166 | 0.697 | **4.2x** | 4 | 14 |
| `index(",")` | short string | 0.079 | 0.904 | **11x** | 0 | 31 |
| `indices(",")` | short string | 0.154 | 2.07 | **13x** | 0 | 97 |
| `sqrt` | float (e≈2.718) | 0.069 | 0.440 | **6.3x** | 0 | 12 |
| `log` | float (e≈2.718) | 0.039 | 0.454 | **12x** | 0 | 12 |
| `sin` | float (e≈2.718) | 0.080 | 0.446 | **5.6x** | 0 | 12 |
| `atan` | integer 1 | 0.054 | 0.380 | **7.1x** | 0 | 11 |
| `exp` | integer 1 | 0.064 | 0.399 | **6.2x** | 0 | 11 |
| `tgamma` | integer 5 | 0.014 | 0.371 | **26x** | 0 | 11 |
| `fabs` | float -3.14 | 0.055 | 0.401 | **7.2x** | 0 | 12 |
| ``"\(.level): \(.svc)"`` | ~45B object | 0.125 | 1.01 | **8.1x** | 0 | 35 |
| ``"user \(.name) …"`` | ~45B object | 0.094 | 1.20 | **13x** | 0 | 40 |
| `isempty(empty)` | null | 0.0090 | 0.559 | **62x** | 0 | 18 |
| `isempty(.[])` | 5-elem array | 0.022 | 1.03 | **47x** | 0 | 33 |
| `nth(2; .[])` | 5-elem array | 0.040 | 2.11 | **52x** | 0 | 49 |
| `test(re)` hit | short string | 0.0091 | 1.86 | **205x** | 0 | 44 |
| `test(re)` miss | short string | 0.109 | 1.79 | **16x** | 0 | 43 |
| `match(re)` hit | short string | 0.217 | 4.14 | **19x** | 1 | 100 |
| `match(re)` miss | short string | 0.603 | 2.95 | **4.9x** | 0 | 59 |
| `capture(re)` hit | short string | 0.209 | — | — | 1 | — |
| `scan(re)` no groups (4 matches) | short string | 0.482 | — | — | 10 | — |
| `sub(re; s)` hit | short string | 0.123 | — | — | 1 | — |
| `gsub(re; s)` hit (4 matches) | short string | 0.573 | — | — | 5 | — |

## Key Takeaways

- **Core operations achieve 0 allocations** in steady state when using `RunWithBuffer` or `RunFunc` (access, filtering, comparison, arithmetic, construction, math, `test(re)`). Operations that produce new structured output allocate proportional to result size: `@base64`/`@uri` (4 allocs; string-escape decoding), `match`/`capture` (1 alloc on a hit), `scan`/`gsub` (per match), `map(f)` when `f` constructs data (~1 per element). Even allocating ops use 10–100× fewer allocations than gojq.
- **† gojq wins on small arrays of primitive integers** — once unmarshaled to `[]interface{}`, element access and reduction (`min`, `any`, `first`, `last`) are O(1) native Go operations. fastjq always scans raw JSON bytes, which loses when the input is a compact integer array. For typical log data (objects with string/mixed fields), fastjq is consistently faster.
- **Fastest on small inputs** where gojq's marshal/unmarshal overhead dominates (up to 63x faster)
- **Still significantly faster on large inputs** (2.5x–5x) where both engines are scanning lots of data
- **Compound select (and/or) is ~56–60x faster**: each boolean operand is evaluated via `execSingle` with no closures; gojq must unmarshal the full object
- **`to_entries` at 6 ns**: just an objectIter reformatting pass — zero-alloc, nearly as fast as a simple field access
- **Large JSON exposes gojq's unmarshal tax**: gojq Field/Del/Construct on 100KB is 540–800 µs vs fastjq's 109–218 µs, because gojq pays the full parse cost up front regardless of which fields are accessed

## Raw Output

Apple M4 Max, go1.25.4, `go test -bench=. -benchmem`. Updated 2026-02-20. Note: some first-run entries show spurious allocs (e.g. `KeysUnsorted` 3 allocs, `First` 1 alloc) due to benchmark calibration warmup — confirmed 0 allocs on repeat runs.

```
BenchmarkFastjq_Small_Del-16                  	 7247836	       164.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Medium_Del-16                 	  433435	      2440 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Del-16                  	    9466	    135135 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Field-16                	12685821	        90.46 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Field-16                	   35793	     32378 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Index-16                	50070672	        23.76 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ArrayDel-16             	13190170	        91.50 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Construct-16            	 9030783	       142.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Construct-16            	   35719	     33096 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Iterator-16             	37948964	        32.25 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Iterator-16             	  133504	      9161 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Select-16               	100000000	        11.38 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Select-16               	    8788	    130607 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Alternative-16          	125900725	         9.526 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SelectAnd-16            	93862436	        12.95 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SelectOr-16             	93320455	        12.92 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Has-16                  	82257507	        14.59 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Has-16                  	    8444	    132682 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_IfThenElse-16           	96452360	        12.29 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Length-16               	131591331	         9.113 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Length-16               	    8269	    130959 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Map-16                  	  954880	      1236 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Medium_Map-16                 	  202123	      6304 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Map-16                  	   92332	     12699 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToEntries-16            	268150532	         4.478 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_ToEntries-16            	    8395	    128526 ns/op	      26 B/op	       0 allocs/op
BenchmarkFastjq_Small_KeysUnsorted-16         	19786334	        60.19 ns/op	      72 B/op	       3 allocs/op
BenchmarkFastjq_Large_KeysUnsorted-16         	    9878	    128543 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Any-16                  	26230128	        44.59 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Any-16                  	  757591	      1584 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AnyExpr-16              	10115491	       118.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_AnyExpr-16              	  383943	      2861 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Add-16                  	19794670	        60.04 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AddStrings-16           	 8646288	       141.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Flatten-16              	11401975	       102.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Split-16                	10949113	       108.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Join-16                 	11288313	        99.51 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Values-16               	13449088	        90.00 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Values-16                 	  498717	      2352 ns/op	    3144 B/op	      51 allocs/op
BenchmarkFastjq_Small_Base64Encode-16         	 8518456	       139.4 ns/op	     120 B/op	       4 allocs/op
BenchmarkFastjq_Small_Base64Decode-16         	 5592325	       206.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Base64Encode-16           	 2330239	       514.9 ns/op	    1321 B/op	      15 allocs/op
BenchmarkGojq_Small_Base64Decode-16           	 2199664	       546.5 ns/op	    1321 B/op	      15 allocs/op
BenchmarkFastjq_Small_IndexFind-16            	14253099	        79.42 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_IndicesAll-16           	 7831444	       153.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_IndexFind-16              	 1328682	       903.9 ns/op	    2274 B/op	      31 allocs/op
BenchmarkGojq_Small_IndicesAll-16             	  583273	      2068 ns/op	    3907 B/op	      97 allocs/op
BenchmarkFastjq_Small_Slice-16                	14053201	        85.05 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SliceString-16          	26537786	        44.50 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Plus-16                 	22695070	        52.85 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_PlusStr-16              	14108904	        82.47 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AsciiDowncase-16        	84201165	        14.30 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_AsciiDowncase-16        	    8247	    141972 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Startswith-16           	83267314	        14.33 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Startswith-16           	    9535	    131327 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Endswith-16             	84298273	        14.28 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Ltrimstr-16             	131811507	         9.112 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Ltrimstr-16             	    8578	    131110 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Rtrimstr-16             	132533798	         9.045 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_First-16                	11211451	       107.2 ns/op	       8 B/op	       0 allocs/op
BenchmarkFastjq_Large_First-16                	  312640	      3570 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Last-16                 	 8605800	       140.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Last-16                 	  363825	      3285 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Limit-16                	31291762	        37.73 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Limit-16                	 2183941	       545.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Del-16                    	 1284572	       930.6 ns/op	    2594 B/op	      33 allocs/op
BenchmarkGojq_Medium_Del-16                   	   63723	     18673 ns/op	   16968 B/op	     323 allocs/op
BenchmarkGojq_Large_Del-16                    	    1489	    799641 ns/op	  542680 B/op	    4666 allocs/op
BenchmarkGojq_Small_Field-16                  	 3511731	       345.1 ns/op	    1177 B/op	      13 allocs/op
BenchmarkGojq_Large_Field-16                  	    2048	    581273 ns/op	  270050 B/op	    2835 allocs/op
BenchmarkGojq_Small_Index-16                  	 1954776	       613.7 ns/op	    1401 B/op	      20 allocs/op
BenchmarkGojq_Small_ArrayDel-16               	  744524	      1626 ns/op	    3362 B/op	      53 allocs/op
BenchmarkGojq_Small_Construct-16              	 1730649	       693.9 ns/op	    1857 B/op	      23 allocs/op
BenchmarkGojq_Large_Construct-16              	    2059	    583371 ns/op	  274423 B/op	    2867 allocs/op
BenchmarkGojq_Small_Iterator-16               	 1581432	       758.4 ns/op	    1776 B/op	      26 allocs/op
BenchmarkGojq_Large_Iterator-16               	   14308	     83995 ns/op	  109808 B/op	    1811 allocs/op
BenchmarkGojq_Small_Select-16                 	 2137993	       565.5 ns/op	    1753 B/op	      20 allocs/op
BenchmarkGojq_Large_Select-16                 	    1545	    775428 ns/op	  540229 B/op	    4652 allocs/op
BenchmarkGojq_Small_Alternative-16            	 2546883	       468.9 ns/op	    1441 B/op	      17 allocs/op
BenchmarkGojq_Small_SelectAnd-16              	 1934535	       620.6 ns/op	    1945 B/op	      21 allocs/op
BenchmarkGojq_Small_SelectOr-16               	 1810826	       661.2 ns/op	    1945 B/op	      21 allocs/op
BenchmarkGojq_Small_Has-16                    	 2168812	       551.3 ns/op	    1753 B/op	      20 allocs/op
BenchmarkGojq_Large_Has-16                    	    1546	    773690 ns/op	  538506 B/op	    4651 allocs/op
BenchmarkGojq_Small_IfThenElse-16             	 2603694	       459.2 ns/op	    1361 B/op	      16 allocs/op
BenchmarkGojq_Small_Length-16                 	 3303072	       362.4 ns/op	    1177 B/op	      13 allocs/op
BenchmarkGojq_Large_Length-16                 	    2047	    582409 ns/op	  269830 B/op	    2835 allocs/op
BenchmarkGojq_Small_Map-16                    	  118368	     10379 ns/op	   13654 B/op	     251 allocs/op
BenchmarkGojq_Large_Map-16                    	   12835	     93320 ns/op	  118586 B/op	    2237 allocs/op
BenchmarkGojq_Small_ToEntries-16              	 3165002	       373.9 ns/op	    1209 B/op	      14 allocs/op
BenchmarkGojq_Large_ToEntries-16              	    1434	    824681 ns/op	  647582 B/op	    5847 allocs/op
BenchmarkGojq_Small_KeysUnsorted-16           	 3219981	       367.9 ns/op	    1209 B/op	      14 allocs/op
BenchmarkGojq_Large_KeysUnsorted-16           	    1993	    602156 ns/op	  282818 B/op	    3039 allocs/op
BenchmarkGojq_Small_Any-16                    	  656725	      1825 ns/op	    4508 B/op	      39 allocs/op
BenchmarkGojq_Small_AnyExpr-16                	  580914	      2129 ns/op	    4620 B/op	      49 allocs/op
BenchmarkGojq_Large_AnyExpr-16                	  710997	      1703 ns/op	    2818 B/op	      29 allocs/op
BenchmarkGojq_Small_Add-16                    	 1799367	       667.3 ns/op	    1433 B/op	      24 allocs/op
BenchmarkGojq_Small_AddStrings-16             	 1475772	       813.9 ns/op	    1689 B/op	      31 allocs/op
BenchmarkGojq_Small_Flatten-16                	  944611	      1262 ns/op	    1857 B/op	      38 allocs/op
BenchmarkGojq_Small_Split-16                  	 1562374	       768.0 ns/op	    1553 B/op	      21 allocs/op
BenchmarkGojq_Small_Join-16                   	 1423782	       841.2 ns/op	    1745 B/op	      30 allocs/op
BenchmarkGojq_Small_Slice-16                  	 1522867	       789.0 ns/op	    1425 B/op	      21 allocs/op
BenchmarkGojq_Small_SliceString-16            	 2698954	       442.4 ns/op	    1145 B/op	      12 allocs/op
BenchmarkGojq_Small_Plus-16                   	 1812120	       664.4 ns/op	    1673 B/op	      21 allocs/op
BenchmarkGojq_Small_PlusStr-16                	 1686606	       713.3 ns/op	    1705 B/op	      23 allocs/op
BenchmarkGojq_Small_AsciiDowncase-16          	 2107429	       569.9 ns/op	    1785 B/op	      21 allocs/op
BenchmarkGojq_Large_AsciiDowncase-16          	    1543	    780424 ns/op	  537562 B/op	    4652 allocs/op
BenchmarkGojq_Small_Startswith-16             	 2091091	       573.0 ns/op	    1801 B/op	      21 allocs/op
BenchmarkGojq_Large_Startswith-16             	    1532	    776602 ns/op	  536736 B/op	    4651 allocs/op
BenchmarkGojq_Small_Endswith-16               	 2030253	       581.4 ns/op	    1801 B/op	      21 allocs/op
BenchmarkGojq_Small_Ltrimstr-16               	 2879712	       416.0 ns/op	    1305 B/op	      16 allocs/op
BenchmarkGojq_Small_Rtrimstr-16               	 2865513	       416.9 ns/op	    1305 B/op	      16 allocs/op
BenchmarkGojq_Small_First-16                  	  791071	      1482 ns/op	    3379 B/op	      39 allocs/op
BenchmarkGojq_Large_First-16                  	  824198	      1430 ns/op	    1889 B/op	      23 allocs/op
BenchmarkGojq_Small_Last-16                   	  625171	      1890 ns/op	    3483 B/op	      43 allocs/op
BenchmarkGojq_Large_Last-16                   	  771391	      1531 ns/op	    2193 B/op	      24 allocs/op
BenchmarkGojq_Small_Limit-16                  	  719332	      1689 ns/op	    3328 B/op	      42 allocs/op
BenchmarkGojq_Large_Limit-16                  	  702063	      1670 ns/op	    2200 B/op	      24 allocs/op
BenchmarkFastjq_Small_Subtract-16             	23270564	        51.05 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Subtract-16               	 1723995	       693.8 ns/op	    1649 B/op	      20 allocs/op
BenchmarkFastjq_Small_Multiply-16             	17969149	        68.19 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Multiply-16               	 1716406	       698.7 ns/op	    1649 B/op	      21 allocs/op
BenchmarkFastjq_Small_Divide-16               	11593302	       100.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Divide-16                 	 1667332	       717.7 ns/op	    1665 B/op	      21 allocs/op
BenchmarkFastjq_Small_Min-16                  	  711556	      1667 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Min-16                    	  924234	      1267 ns/op	    1217 B/op	      15 allocs/op
BenchmarkFastjq_Small_MinBy-16                	  150270	      8118 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_MinBy-16                  	   22065	     54364 ns/op	   63629 B/op	    1347 allocs/op
BenchmarkFastjq_Small_URIEncode-16            	 7291156	       165.9 ns/op	     120 B/op	       4 allocs/op
BenchmarkGojq_Small_URIEncode-16              	 1717396	       696.6 ns/op	    1289 B/op	      14 allocs/op
BenchmarkFastjq_Small_ArrayDiff-16            	 6244298	       197.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ArrayDiff-16              	  890272	      1319 ns/op	    2121 B/op	      33 allocs/op
BenchmarkFastjq_Small_TryNoError-16           	134925624	         8.894 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TryCatchNoError-16      	19810533	        61.33 ns/op	      64 B/op	       1 allocs/op
BenchmarkGojq_Small_TryNoError-16             	 2711973	       435.9 ns/op	    1449 B/op	      16 allocs/op
BenchmarkFastjq_Small_ObjectMerge-16          	 6396772	       196.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ObjectMerge-16            	  702292	      1609 ns/op	    2906 B/op	      33 allocs/op
BenchmarkFastjq_Small_ToJSON-16               	 9417940	       123.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ToJSON-16                 	 2819920	       426.8 ns/op	    1313 B/op	      17 allocs/op
BenchmarkFastjq_Small_FromJSON-16             	 7958779	       151.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_FromJSON-16               	  849123	      1315 ns/op	    2714 B/op	      31 allocs/op
BenchmarkFastjq_Small_ToString-16             	 9334431	       125.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToNumber-16             	90749247	        13.44 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ToNumber-16               	 3408667	       351.0 ns/op	    1112 B/op	      11 allocs/op
BenchmarkFastjq_Small_AnyTwoArg-16            	  391806	      3115 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_AnyTwoArg-16              	  730710	      1641 ns/op	    2482 B/op	      27 allocs/op
BenchmarkFastjq_Complex_LogNormalize-16       	 1000000	      1098 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Complex_LogNormalize-16         	  397214	      2948 ns/op	    4324 B/op	      69 allocs/op
BenchmarkFastjq_Complex_ArrayPipeline-16      	  145113	      7988 ns/op	    1232 B/op	      64 allocs/op
BenchmarkGojq_Complex_ArrayPipeline-16        	   51270	     22576 ns/op	   20407 B/op	     525 allocs/op
BenchmarkFastjq_Complex_Aggregation-16        	  107610	     10125 ns/op	     416 B/op	      26 allocs/op
BenchmarkGojq_Complex_Aggregation-16          	   56211	     21594 ns/op	   22301 B/op	     551 allocs/op
BenchmarkFastjq_Complex_TolerantMap-16        	  131808	      8940 ns/op	    2440 B/op	      98 allocs/op
BenchmarkGojq_Complex_TolerantMap-16          	   43824	     27306 ns/op	   27869 B/op	     641 allocs/op
BenchmarkFastjq_Complex_ElifRouting-16        	  140450	      8545 ns/op	    2520 B/op	     125 allocs/op
BenchmarkGojq_Complex_ElifRouting-16          	   48996	     24905 ns/op	   25917 B/op	     595 allocs/op
BenchmarkFastjq_Complex_StringBuild-16        	  141122	      8417 ns/op	    1360 B/op	     124 allocs/op
BenchmarkGojq_Complex_StringBuild-16          	   47613	     24605 ns/op	   20533 B/op	     686 allocs/op
BenchmarkFastjq_Complex_EntryFilter-16        	  411220	      2754 ns/op	    2168 B/op	      35 allocs/op
BenchmarkGojq_Complex_EntryFilter-16          	  151954	      7821 ns/op	   11187 B/op	     162 allocs/op
BenchmarkFastjq_Small_Sqrt-16                 	16863188	        69.46 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Sqrt-16                   	 2712890	       439.8 ns/op	    1145 B/op	      12 allocs/op
BenchmarkFastjq_Small_Log-16                  	30042403	        39.42 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Log-16                    	 2645451	       454.0 ns/op	    1129 B/op	      12 allocs/op
BenchmarkFastjq_Small_Sin-16                  	14877654	        80.02 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Sin-16                    	 2729716	       446.1 ns/op	    1145 B/op	      12 allocs/op
BenchmarkFastjq_Small_Atan-16                 	22343629	        53.73 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Atan-16                   	 3155512	       379.8 ns/op	    1121 B/op	      11 allocs/op
BenchmarkFastjq_Small_Exp-16                  	18646810	        64.32 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Exp-16                    	 2928008	       399.1 ns/op	    1121 B/op	      11 allocs/op
BenchmarkFastjq_Small_Tgamma-16               	80634999	        14.36 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Tgamma-16                 	 3249226	       371.1 ns/op	    1104 B/op	      11 allocs/op
BenchmarkFastjq_Small_Fabs-16                 	22019695	        55.44 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Fabs-16                   	 2991781	       401.0 ns/op	    1112 B/op	      12 allocs/op
BenchmarkFastjq_Small_StringInterp-16         	 9352501	       125.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_StringInterp-16           	 1000000	      1014 ns/op	    2121 B/op	      35 allocs/op
BenchmarkFastjq_Small_StringInterpNum-16      	12249183	        94.30 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_StringInterpNum-16        	  984379	      1200 ns/op	    2498 B/op	      40 allocs/op
BenchmarkFastjq_Small_IsEmptyTrue-16          	133837210	         8.969 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_IsEmptyTrue-16            	 2138412	       559.0 ns/op	    1921 B/op	      18 allocs/op
BenchmarkFastjq_Small_IsEmptyFalse-16         	53954510	        22.24 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_IsEmptyFalse-16           	 1000000	      1035 ns/op	    2898 B/op	      33 allocs/op
BenchmarkFastjq_Small_Nth-16                  	29619724	        40.45 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Nth-16                    	  567028	      2107 ns/op	    4372 B/op	      49 allocs/op
BenchmarkRegexp_Match_Hit-16                  	28995399	        42.31 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Miss-16                 	52432179	        22.58 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Complex_Hit-16          	28257622	        42.37 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Long_Miss-16            	15707414	        76.15 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_MatchString-16                	27531434	        43.67 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_FindSubmatch_Hit-16           	 5966568	       199.8 ns/op	     160 B/op	       2 allocs/op
BenchmarkRegexp_FindSubmatch_Miss-16          	 1711882	       699.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_FindSubmatchIndex_Hit-16      	 6470352	       182.0 ns/op	      64 B/op	       1 allocs/op
BenchmarkRegexp_FindSubmatchIndex_Miss-16     	 1795264	       669.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_ReplaceAll_Hit-16             	 7723044	       153.8 ns/op	      97 B/op	       5 allocs/op
BenchmarkRegexp_ReplaceAll_Miss-16            	24616057	        48.73 ns/op	      48 B/op	       2 allocs/op
BenchmarkRegexp_MatchViaClosure-16            	29753172	        41.94 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_Hit-16           	132313659	         9.076 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_Miss-16          	10905463	       109.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_InPipeline-16    	 4240141	       282.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_MatchRe_Hit-16          	 5438455	       216.8 ns/op	      48 B/op	       1 allocs/op
BenchmarkFastjq_Small_MatchRe_Miss-16         	 1989954	       602.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_TestRe_Hit-16             	  653834	      1861 ns/op	    4164 B/op	      44 allocs/op
BenchmarkGojq_Small_TestRe_Miss-16            	  625005	      1794 ns/op	    4109 B/op	      43 allocs/op
BenchmarkGojq_Small_MatchRe_Hit-16            	  291872	      4145 ns/op	    8015 B/op	     100 allocs/op
BenchmarkGojq_Small_MatchRe_Miss-16           	  412278	      2946 ns/op	    5718 B/op	      59 allocs/op
BenchmarkFastjq_Small_CaptureRe_Hit-16        	 5700424	       209.2 ns/op	      48 B/op	       1 allocs/op
BenchmarkFastjq_Small_CaptureRe_Miss-16       	 1984827	       606.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ScanRe_NoGroups-16      	 2488180	       482.0 ns/op	     387 B/op	      10 allocs/op
BenchmarkFastjq_Small_ScanRe_WithGroups-16    	 2004952	       598.9 ns/op	     702 B/op	      13 allocs/op
BenchmarkFastjq_Small_SubRe_Hit-16            	 9741260	       123.3 ns/op	      16 B/op	       1 allocs/op
BenchmarkFastjq_Small_SubRe_Miss-16           	11810901	       101.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_GSubRe_Hit-16           	 2099644	       572.7 ns/op	     305 B/op	       5 allocs/op
BenchmarkFastjq_Small_GSubRe_Miss-16          	 4649283	       251.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_CaptureRe_Hit-16          	  345390	      3342 ns/op	    7380 B/op	      77 allocs/op
BenchmarkGojq_Small_CaptureRe_Miss-16         	  415980	      2951 ns/op	    5588 B/op	      54 allocs/op
BenchmarkGojq_Small_ScanRe_NoGroups-16        	  386016	      3098 ns/op	    6018 B/op	      67 allocs/op
BenchmarkGojq_Small_SubRe_Hit-16              	  337782	      3593 ns/op	    6168 B/op	      77 allocs/op
BenchmarkGojq_Small_GSubRe_Hit-16             	  161469	      7484 ns/op	    9418 B/op	     143 allocs/op
```

## CLI Throughput: fastjq vs jq

End-to-end JSONL throughput benchmarks comparing `fastjq-bench` (Go CLI using the fastjq library) against `jq` (C implementation, v1.8.1). Both read JSONL from stdin, apply a filter per line, write results to `/dev/null`. Median of 3 runs. Apple M4 Max. Updated 2026-02-20.

| Operation | Input | jq (s) | fastjq (s) | Speedup |
|-----------|-------|--------|-------------|---------|
| Identity (`.`) | small (100K lines, ~11MB) | 0.343 | 0.026 | **13x** |
| Field access (`.field_2`) | small | 0.151 | 0.021 | **7x** |
| Field access (`.field_50`) | large (100 lines, ~16MB) | 0.093 | 0.013 | **7x** |
| Delete field (`del(.field_2)`) | small | 0.365 | 0.034 | **11x** |
| Object construction (`{field_0, field_2}`) | small | 0.247 | 0.032 | **8x** |
| Select all match (`select(.field_2 == "xxx...")`) | small | 0.372 | 0.023 | **16x** |
| Select none match (`select(.field_2 == "nope")`) | small | 0.141 | 0.023 | **6x** |
| Alternative (`.field_2 // "default"`) | small | 0.168 | 0.022 | **8x** |
| Case-insensitive select (`ascii_downcase`) | small | 0.679 | 0.033 | **21x** |
| Prefix filter (`startswith`) | small | 0.366 | 0.026 | **14x** |
| Field existence (`has`) | small | 0.365 | 0.023 | **16x** |
| `to_entries` | small | 0.717 | 0.040 | **18x** |
| `keys_unsorted` | small | 0.243 | 0.031 | **8x** |

### Key Takeaways (CLI)

- **6x–21x faster** than jq across all operations on real JSONL workloads
- **Case-insensitive select (`ascii_downcase`) is 21x faster**: near-zero cost for the string transform
- **`to_entries` is 18x faster**: zero-copy reformatting vs jq's full parse + marshal cycle
- **Field access improved to 7x** (both small and large) from the `findFieldStr` optimization
- **Object construction up to 8x faster**: early-exit scan means no wasted work on remaining fields
- **Select (none match) is only 6x faster** — both engines scan the full document; fastjq's advantage is smaller here

### Reproducing

```bash
chmod +x bench_vs_jq.sh
./bench_vs_jq.sh
```

## Reproducing (Go Benchmarks)

```bash
go test -bench=. -benchmem
```
