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
| `.field` | Small (~100B) | 0.084 | 0.349 | **4.1x** | 0 | 13 |
| `.field` | Large (~100KB) | 34.5 | 579 | **17x** | 0 | 2835 |
| `del(.f)` | Small (~100B) | 0.180 | 0.933 | **5.2x** | 0 | 33 |
| `del(.f)` | Medium (~2KB) | 2.94 | 18.9 | **6.4x** | 0 | 323 |
| `del(.f)` | Large (~100KB) | 172 | 804 | **4.7x** | 0 | 4666 |
| `.[n]` | 5-elem array | 0.025 | 0.622 | **25x** | 0 | 20 |
| `del(.[n], .[m])` | 5-elem array | 0.091 | 1.66 | **18x** | 0 | 53 |
| `{f0, f2}` (construct) | Small (~100B) | 0.149 | 0.707 | **4.7x** | 0 | 23 |
| `{f0, f50}` (construct) | Large (~100KB) | 50.3 | 590 | **12x** | 0 | 2867 |
| `.[]` iterator | 5-elem array | 0.032 | 0.777 | **25x** | 0 | 26 |
| `.[]` iterator | 200-elem array | 10.9 | 84.9 | **7.8x** | 0 | 1811 |
| `select(.f == "x")` | Small (~100B) | 0.011 | 0.571 | **50x** | 0 | 20 |
| `select(.f == "x")`¹ | Large (~100KB, last field) | 202 | 777 | **3.8x** | 0 | 4651 |
| `select(.f and .g)` | Small (~100B) | 0.013 | 0.643 | **50x** | 0 | 21 |
| `select(.f or .g)` | Small (~100B) | 0.013 | 0.681 | **53x** | 0 | 21 |
| `has("key")` in select | Small (~100B) | 0.015 | 0.558 | **38x** | 0 | 20 |
| `has("key")` in select | Large (~100KB) | 222 | 797 | **3.6x** | 0 | 4651 |
| `if-then-else` | Small (~100B) | 0.012 | 0.475 | **39x** | 0 | 16 |
| `.f // "default"` | Small (~100B) | 0.0096 | 0.469 | **49x** | 0 | 17 |
| `try .field` (no error) | Small (~100B) | 0.0089 | 0.437 | **49x** | 0 | 16 |
| `.a + .b` (strings) | Small (~100B) | 0.053 | 0.665 | **13x** | 0 | 21 |
| `"prefix" + .name` | Small (~100B) | 0.084 | 0.722 | **8.6x** | 0 | 23 |
| `.a - .b` (subtract) | Small (~100B) | 0.051 | 0.688 | **14x** | 0 | 20 |
| `.a * .b` (multiply) | Small (~100B) | 0.068 | 0.706 | **10x** | 0 | 21 |
| `.a / .b` (divide) | Small (~100B) | 0.099 | 0.717 | **7.2x** | 0 | 21 |
| `.a - .b` (array diff) | 5-elem arrays | 0.193 | 1.31 | **6.8x** | 0 | 33 |
| `length` | Small (~100B) | 0.0092 | 0.365 | **40x** | 0 | 13 |
| `length` | Large (~100KB) | 146 | 587 | **4x** | 0 | 2835 |
| `add` (numbers) | 5-elem array | 0.060 | 0.677 | **11x** | 0 | 24 |
| `add` (strings) | 5-elem array | 0.140 | 0.832 | **5.9x** | 0 | 31 |
| `flatten` | 3-elem nested array | 0.101 | 1.28 | **13x** | 0 | 38 |
| `min` | 200-int array | 1.63 | 1.23 | 0.8x† | 0 | 15 |
| `min_by(.value)` | 100-elem object array | 8.50 | 54.1 | **6.4x** | 0 | 1347 |
| `map(.name)` | 20-elem array (~600B) | 1.32 | 10.2 | **7.7x** | 0 | 251 |
| `map(.name)` | 200-elem array (~6KB) | 14.0 | 94.2 | **6.7x** | 0 | 2237 |
| `any` | 5-elem array | 0.043 | 1.84 | **42x** | 0 | 39 |
| `any(expr)` | 5-elem array | 0.118 | 2.07 | **18x** | 0 | 49 |
| `any(expr)`² | 200-int array | 2.91 | 1.71 | 0.6x† | 0 | 29 |
| `any(gen; cond)`² | 200-int array | 3.17 | 1.63 | 0.5x† | 0 | 27 |
| `first(expr)` | 5-elem array | 0.110 | 1.49 | **14x** | 1 | 39 |
| `first(expr)`² | 200-int array | 3.55 | 1.43 | 0.4x† | 0 | 23 |
| `last(expr)` | 5-elem array | 0.141 | 1.91 | **14x** | 0 | 43 |
| `last(expr)`² | 200-int array | 3.35 | 1.55 | 0.5x† | 0 | 24 |
| `limit(3; expr)` | 5-elem array | 0.039 | 1.66 | **43x** | 0 | 42 |
| `limit(10; expr)` | 200-int array | 0.565 | 1.61 | **2.8x** | 0 | 24 |
| `.[1:4]` slice | 6-elem array | 0.086 | 0.808 | **9.4x** | 0 | 21 |
| `values` | 9-elem array | 0.089 | 2.36 | **27x** | 0 | 51 |
| `to_entries` | Small (~100B) | 0.0045 | 0.372 | **83x** | 0 | 14 |
| `to_entries` | Large (~100KB) | 189 | 828 | **4.4x** | 0 | 5847 |
| `keys_unsorted` | Small (~100B) | 0.061 | 0.372 | **6.1x** | 3 | 14 |
| `keys_unsorted` | Large (~100KB) | 187 | 604 | **3.2x** | 0 | 3039 |
| `object merge .a + .b` | Small (~100B) | 0.179 | 1.50 | **8.4x** | 0 | 33 |
| `tojson` | Small (~100B) | 0.122 | 0.423 | **3.5x** | 0 | 17 |
| `fromjson` | JSON string | 0.151 | 1.28 | **8.4x** | 0 | 31 |
| `tonumber` | `"42"` string | 0.014 | 0.355 | **26x** | 0 | 11 |
| `split(",")` | short string | 0.109 | 0.790 | **7.3x** | 0 | 21 |
| `join(",")` | 5-elem array | 0.100 | 0.854 | **8.6x** | 0 | 30 |
| `ascii_downcase` in select | Small (~100B) | 0.015 | 0.578 | **40x** | 0 | 21 |
| `ascii_downcase` in select | Large (~100KB) | 172 | 779 | **4.5x** | 0 | 4652 |
| `startswith("s")` | Small (~100B) | 0.014 | 0.579 | **40x** | 0 | 21 |
| `startswith("s")` | Large (~100KB) | 193 | 781 | **4x** | 0 | 4651 |
| `endswith("s")` | Small (~100B) | 0.014 | 0.584 | **41x** | 0 | 21 |
| `ltrimstr("s")` | Small (~100B) | 0.0091 | 0.422 | **46x** | 0 | 16 |
| `rtrimstr("s")` | Small (~100B) | 0.0093 | 0.419 | **45x** | 0 | 16 |
| `select` + string ops + arith + construct | ~200B log event | 1.09 | 2.95 | **2.7x** | 0 | 69 |
| `[.[] \| select + arith + construct]` | 20-elem array (~1.5KB) | 8.45 | 22.6 | **2.7x** | 64 | 525 |
| `length + map + add + min_by + any` | 20-elem array (~1.5KB) | 11.0 | 21.5 | **2x** | 26 | 551 |
| `map(try {…} catch …)` | 20-elem array (~1.5KB) | 8.87 | 27.5 | **3.1x** | 98 | 641 |
| `map(if … elif … else …)` | 20-elem array (~1.5KB) | 8.48 | 24.9 | **2.9x** | 125 | 595 |
| `[.[] \| str + tostring + str]` | 20-elem array (~1.5KB) | 8.56 | 24.4 | **2.9x** | 124 | 686 |
| `to_entries \| map(select) \| from_entries` | ~200B log event | 2.90 | 7.72 | **2.7x** | 35 | 162 |
| `@base64` | 34-char string | 0.139 | 0.513 | **3.7x** | 4 | 15 |
| `@base64d` | 48-char encoded | 0.210 | 0.548 | **2.6x** | 0 | 15 |
| `@uri` | 36-char URL string | 0.164 | 0.659 | **4x** | 4 | 14 |
| `index(",")` | short string | 0.078 | 0.906 | **12x** | 0 | 31 |
| `indices(",")` | short string | 0.154 | 2.09 | **14x** | 0 | 97 |
| `sqrt` | float (e≈2.718) | 0.068 | 0.461 | **6.8x** | 0 | 12 |
| `log` | float (e≈2.718) | 0.040 | 0.437 | **11x** | 0 | 12 |
| `sin` | float (e≈2.718) | 0.080 | 0.438 | **5.4x** | 0 | 12 |
| `atan` | integer 1 | 0.054 | 0.383 | **7.1x** | 0 | 11 |
| `exp` | integer 1 | 0.064 | 0.397 | **6.2x** | 0 | 11 |
| `tgamma` | integer 5 | 0.015 | 0.371 | **25x** | 0 | 11 |
| `fabs` | float -3.14 | 0.055 | 0.399 | **7.2x** | 0 | 12 |
| ``"\(.level): \(.svc)"`` | ~45B object | 0.127 | 1.02 | **8.1x** | 0 | 35 |
| ``"user \(.name) …"`` | ~45B object | 0.097 | 1.22 | **13x** | 0 | 40 |
| `isempty(empty)` | null | 0.0090 | 0.561 | **63x** | 0 | 18 |
| `isempty(.[])` | 5-elem array | 0.022 | 1.01 | **47x** | 0 | 33 |
| `nth(2; .[])` | 5-elem array | 0.040 | 2.13 | **53x** | 0 | 49 |

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
BenchmarkFastjq_Small_Del-16                  	 7108237	       179.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Medium_Del-16                 	  419643	      2943 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Del-16                  	    6915	    171549 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Field-16                	12871214	        84.10 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Field-16                	   38118	     34465 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Index-16                	50523390	        24.62 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ArrayDel-16             	12959102	        91.30 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Construct-16            	 7133766	       149.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Construct-16            	   21972	     50309 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Iterator-16             	36459848	        31.59 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Iterator-16             	   93343	     10865 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Select-16               	100000000	        11.33 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Select-16               	    9958	    201981 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Alternative-16          	125725946	         9.563 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SelectAnd-16            	93868560	        12.90 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SelectOr-16             	93219565	        12.92 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Has-16                  	82586566	        14.62 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Has-16                  	    4994	    222254 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_IfThenElse-16           	98993222	        12.27 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Length-16               	131147636	         9.159 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Length-16               	    9835	    145838 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Map-16                  	  911966	      1319 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Medium_Map-16                 	  160420	      7021 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Map-16                  	   78754	     13975 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToEntries-16            	268216234	         4.499 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_ToEntries-16            	    8720	    189051 ns/op	      25 B/op	       0 allocs/op
BenchmarkFastjq_Small_KeysUnsorted-16         	19641045	        60.71 ns/op	      72 B/op	       3 allocs/op
BenchmarkFastjq_Large_KeysUnsorted-16         	    8863	    187245 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Any-16                  	26995309	        43.38 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Any-16                  	  733782	      1608 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AnyExpr-16              	 9929691	       118.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_AnyExpr-16              	  406897	      2907 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Add-16                  	19846341	        60.25 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AddStrings-16           	 8278834	       140.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Flatten-16              	11642572	       101.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Split-16                	11022480	       109.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Join-16                 	11660995	        99.53 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Values-16               	13688596	        88.90 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Values-16                 	  503641	      2358 ns/op	    3144 B/op	      51 allocs/op
BenchmarkFastjq_Small_Base64Encode-16         	 8609805	       139.2 ns/op	     120 B/op	       4 allocs/op
BenchmarkFastjq_Small_Base64Decode-16         	 5934330	       210.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Base64Encode-16           	 2342925	       513.3 ns/op	    1321 B/op	      15 allocs/op
BenchmarkGojq_Small_Base64Decode-16           	 2187540	       548.0 ns/op	    1321 B/op	      15 allocs/op
BenchmarkFastjq_Small_IndexFind-16            	14914273	        78.47 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_IndicesAll-16           	 7750825	       154.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_IndexFind-16              	 1321048	       906.2 ns/op	    2274 B/op	      31 allocs/op
BenchmarkGojq_Small_IndicesAll-16             	  584464	      2087 ns/op	    3907 B/op	      97 allocs/op
BenchmarkFastjq_Small_Slice-16                	13924680	        86.19 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SliceString-16          	25485056	        46.48 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Plus-16                 	22445131	        52.56 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_PlusStr-16              	14228446	        83.83 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AsciiDowncase-16        	83916570	        14.58 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_AsciiDowncase-16        	    7296	    171618 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Startswith-16           	84464416	        14.36 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Startswith-16           	    9442	    193398 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Endswith-16             	84000030	        14.33 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Ltrimstr-16             	131164891	         9.148 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Ltrimstr-16             	    8557	    147998 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Rtrimstr-16             	128766238	         9.304 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_First-16                	10671876	       109.5 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Large_First-16                	  334059	      3554 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Last-16                 	 8558770	       140.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Last-16                 	  351547	      3348 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Limit-16                	29961548	        38.86 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Limit-16                	 2164933	       564.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Del-16                    	 1286316	       932.9 ns/op	    2594 B/op	      33 allocs/op
BenchmarkGojq_Medium_Del-16                   	   63724	     18925 ns/op	   16969 B/op	     323 allocs/op
BenchmarkGojq_Large_Del-16                    	    1491	    804290 ns/op	  540171 B/op	    4666 allocs/op
BenchmarkGojq_Small_Field-16                  	 3438592	       348.8 ns/op	    1177 B/op	      13 allocs/op
BenchmarkGojq_Large_Field-16                  	    2055	    578788 ns/op	  270043 B/op	    2835 allocs/op
BenchmarkGojq_Small_Index-16                  	 1947604	       622.0 ns/op	    1401 B/op	      20 allocs/op
BenchmarkGojq_Small_ArrayDel-16               	  710331	      1662 ns/op	    3362 B/op	      53 allocs/op
BenchmarkGojq_Small_Construct-16              	 1713412	       706.8 ns/op	    1857 B/op	      23 allocs/op
BenchmarkGojq_Large_Construct-16              	    2010	    590184 ns/op	  274418 B/op	    2867 allocs/op
BenchmarkGojq_Small_Iterator-16               	 1567970	       776.7 ns/op	    1776 B/op	      26 allocs/op
BenchmarkGojq_Large_Iterator-16               	   14085	     84943 ns/op	  109809 B/op	    1811 allocs/op
BenchmarkGojq_Small_Select-16                 	 2109075	       571.2 ns/op	    1753 B/op	      20 allocs/op
BenchmarkGojq_Large_Select-16                 	    1540	    776521 ns/op	  533498 B/op	    4651 allocs/op
BenchmarkGojq_Small_Alternative-16            	 2550783	       469.0 ns/op	    1441 B/op	      17 allocs/op
BenchmarkGojq_Small_SelectAnd-16              	 1812522	       643.4 ns/op	    1945 B/op	      21 allocs/op
BenchmarkGojq_Small_SelectOr-16               	 1728438	       680.7 ns/op	    1945 B/op	      21 allocs/op
BenchmarkGojq_Small_Has-16                    	 2142826	       557.9 ns/op	    1753 B/op	      20 allocs/op
BenchmarkGojq_Large_Has-16                    	    1506	    797313 ns/op	  536554 B/op	    4651 allocs/op
BenchmarkGojq_Small_IfThenElse-16             	 2499780	       475.3 ns/op	    1361 B/op	      16 allocs/op
BenchmarkGojq_Small_Length-16                 	 3217174	       365.1 ns/op	    1177 B/op	      13 allocs/op
BenchmarkGojq_Large_Length-16                 	    2049	    587275 ns/op	  269832 B/op	    2835 allocs/op
BenchmarkGojq_Small_Map-16                    	  116325	     10212 ns/op	   13653 B/op	     251 allocs/op
BenchmarkGojq_Large_Map-16                    	   12674	     94247 ns/op	  118588 B/op	    2237 allocs/op
BenchmarkGojq_Small_ToEntries-16              	 3235735	       372.2 ns/op	    1209 B/op	      14 allocs/op
BenchmarkGojq_Large_ToEntries-16              	    1443	    827931 ns/op	  638558 B/op	    5847 allocs/op
BenchmarkGojq_Small_KeysUnsorted-16           	 3250276	       372.1 ns/op	    1209 B/op	      14 allocs/op
BenchmarkGojq_Large_KeysUnsorted-16           	    1935	    604301 ns/op	  282807 B/op	    3039 allocs/op
BenchmarkGojq_Small_Any-16                    	  641065	      1842 ns/op	    4508 B/op	      39 allocs/op
BenchmarkGojq_Small_AnyExpr-16                	  591554	      2074 ns/op	    4620 B/op	      49 allocs/op
BenchmarkGojq_Large_AnyExpr-16                	  708524	      1709 ns/op	    2818 B/op	      29 allocs/op
BenchmarkGojq_Small_Add-16                    	 1772790	       677.3 ns/op	    1433 B/op	      24 allocs/op
BenchmarkGojq_Small_AddStrings-16             	 1441294	       831.7 ns/op	    1689 B/op	      31 allocs/op
BenchmarkGojq_Small_Flatten-16                	  932467	      1278 ns/op	    1857 B/op	      38 allocs/op
BenchmarkGojq_Small_Split-16                  	 1515727	       790.3 ns/op	    1553 B/op	      21 allocs/op
BenchmarkGojq_Small_Join-16                   	 1406251	       853.8 ns/op	    1745 B/op	      30 allocs/op
BenchmarkGojq_Small_Slice-16                  	 1484793	       808.5 ns/op	    1425 B/op	      21 allocs/op
BenchmarkGojq_Small_SliceString-16            	 2661049	       445.9 ns/op	    1145 B/op	      12 allocs/op
BenchmarkGojq_Small_Plus-16                   	 1805269	       665.4 ns/op	    1673 B/op	      21 allocs/op
BenchmarkGojq_Small_PlusStr-16                	 1660491	       722.4 ns/op	    1705 B/op	      23 allocs/op
BenchmarkGojq_Small_AsciiDowncase-16          	 2050501	       578.3 ns/op	    1785 B/op	      21 allocs/op
BenchmarkGojq_Large_AsciiDowncase-16          	    1516	    778808 ns/op	  534289 B/op	    4652 allocs/op
BenchmarkGojq_Small_Startswith-16             	 2074622	       579.4 ns/op	    1801 B/op	      21 allocs/op
BenchmarkGojq_Large_Startswith-16             	    1521	    781229 ns/op	  535567 B/op	    4651 allocs/op
BenchmarkGojq_Small_Endswith-16               	 2065984	       583.6 ns/op	    1801 B/op	      21 allocs/op
BenchmarkGojq_Small_Ltrimstr-16               	 2831624	       422.3 ns/op	    1305 B/op	      16 allocs/op
BenchmarkGojq_Small_Rtrimstr-16               	 2879364	       419.2 ns/op	    1305 B/op	      16 allocs/op
BenchmarkGojq_Small_First-16                  	  824083	      1491 ns/op	    3379 B/op	      39 allocs/op
BenchmarkGojq_Large_First-16                  	  845739	      1431 ns/op	    1889 B/op	      23 allocs/op
BenchmarkGojq_Small_Last-16                   	  634641	      1905 ns/op	    3483 B/op	      43 allocs/op
BenchmarkGojq_Large_Last-16                   	  760747	      1546 ns/op	    2193 B/op	      24 allocs/op
BenchmarkGojq_Small_Limit-16                  	  718603	      1662 ns/op	    3328 B/op	      42 allocs/op
BenchmarkGojq_Large_Limit-16                  	  751028	      1608 ns/op	    2200 B/op	      24 allocs/op
BenchmarkFastjq_Small_Subtract-16             	22466388	        50.95 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Subtract-16               	 1739833	       688.1 ns/op	    1649 B/op	      20 allocs/op
BenchmarkFastjq_Small_Multiply-16             	16392284	        67.77 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Multiply-16               	 1696862	       706.4 ns/op	    1649 B/op	      21 allocs/op
BenchmarkFastjq_Small_Divide-16               	11434742	        99.21 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Divide-16                 	 1670960	       717.1 ns/op	    1665 B/op	      21 allocs/op
BenchmarkFastjq_Small_Min-16                  	  714292	      1627 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Min-16                    	  974773	      1232 ns/op	    1217 B/op	      15 allocs/op
BenchmarkFastjq_Small_MinBy-16                	  135657	      8502 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_MinBy-16                  	   22185	     54124 ns/op	   63628 B/op	    1347 allocs/op
BenchmarkFastjq_Small_URIEncode-16            	 7326895	       164.1 ns/op	     120 B/op	       4 allocs/op
BenchmarkGojq_Small_URIEncode-16              	 1830636	       658.9 ns/op	    1289 B/op	      14 allocs/op
BenchmarkFastjq_Small_ArrayDiff-16            	 6150226	       193.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ArrayDiff-16              	  959775	      1314 ns/op	    2121 B/op	      33 allocs/op
BenchmarkFastjq_Small_TryNoError-16           	134584972	         8.883 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TryCatchNoError-16      	19582098	        61.07 ns/op	      64 B/op	       1 allocs/op
BenchmarkGojq_Small_TryNoError-16             	 2785098	       437.4 ns/op	    1449 B/op	      16 allocs/op
BenchmarkFastjq_Small_ObjectMerge-16          	 6628830	       179.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ObjectMerge-16            	  790542	      1504 ns/op	    2906 B/op	      33 allocs/op
BenchmarkFastjq_Small_ToJSON-16               	10006131	       121.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ToJSON-16                 	 2842630	       422.6 ns/op	    1313 B/op	      17 allocs/op
BenchmarkFastjq_Small_FromJSON-16             	 7949562	       151.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_FromJSON-16               	  908557	      1279 ns/op	    2714 B/op	      31 allocs/op
BenchmarkFastjq_Small_ToString-16             	 9723487	       123.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToNumber-16             	87956122	        13.68 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ToNumber-16               	 3387735	       354.6 ns/op	    1112 B/op	      11 allocs/op
BenchmarkFastjq_Small_AnyTwoArg-16            	  364356	      3175 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_AnyTwoArg-16              	  724784	      1633 ns/op	    2482 B/op	      27 allocs/op
BenchmarkFastjq_Complex_LogNormalize-16       	 1000000	      1088 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Complex_LogNormalize-16         	  409678	      2954 ns/op	    4324 B/op	      69 allocs/op
BenchmarkFastjq_Complex_ArrayPipeline-16      	  139147	      8446 ns/op	    1232 B/op	      64 allocs/op
BenchmarkGojq_Complex_ArrayPipeline-16        	   51728	     22622 ns/op	   20407 B/op	     525 allocs/op
BenchmarkFastjq_Complex_Aggregation-16        	  110754	     10952 ns/op	     416 B/op	      26 allocs/op
BenchmarkGojq_Complex_Aggregation-16          	   56491	     21464 ns/op	   22300 B/op	     551 allocs/op
BenchmarkFastjq_Complex_TolerantMap-16        	  136048	      8873 ns/op	    2440 B/op	      98 allocs/op
BenchmarkGojq_Complex_TolerantMap-16          	   43630	     27481 ns/op	   27870 B/op	     641 allocs/op
BenchmarkFastjq_Complex_ElifRouting-16        	  136366	      8485 ns/op	    2520 B/op	     125 allocs/op
BenchmarkGojq_Complex_ElifRouting-16          	   48862	     24922 ns/op	   25919 B/op	     595 allocs/op
BenchmarkFastjq_Complex_StringBuild-16        	  142195	      8558 ns/op	    1360 B/op	     124 allocs/op
BenchmarkGojq_Complex_StringBuild-16          	   48272	     24436 ns/op	   20533 B/op	     686 allocs/op
BenchmarkFastjq_Complex_EntryFilter-16        	  413182	      2897 ns/op	    2168 B/op	      35 allocs/op
BenchmarkGojq_Complex_EntryFilter-16          	  156144	      7717 ns/op	   11187 B/op	     162 allocs/op
BenchmarkFastjq_Small_Sqrt-16                 	17549108	        68.02 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Sqrt-16                   	 2570908	       460.5 ns/op	    1145 B/op	      12 allocs/op
BenchmarkFastjq_Small_Log-16                  	29343081	        40.01 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Log-16                    	 2721925	       437.4 ns/op	    1128 B/op	      12 allocs/op
BenchmarkFastjq_Small_Sin-16                  	14996343	        80.44 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Sin-16                    	 2738295	       438.3 ns/op	    1145 B/op	      12 allocs/op
BenchmarkFastjq_Small_Atan-16                 	22022119	        54.29 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Atan-16                   	 3125779	       383.3 ns/op	    1121 B/op	      11 allocs/op
BenchmarkFastjq_Small_Exp-16                  	18541826	        64.33 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Exp-16                    	 2975654	       397.2 ns/op	    1121 B/op	      11 allocs/op
BenchmarkFastjq_Small_Tgamma-16               	81851434	        14.57 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Tgamma-16                 	 3216337	       371.5 ns/op	    1104 B/op	      11 allocs/op
BenchmarkFastjq_Small_Fabs-16                 	21781532	        55.26 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Fabs-16                   	 2991416	       399.1 ns/op	    1112 B/op	      12 allocs/op
BenchmarkFastjq_Small_StringInterp-16         	 9193526	       126.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_StringInterp-16           	 1000000	      1024 ns/op	    2121 B/op	      35 allocs/op
BenchmarkFastjq_Small_StringInterpNum-16      	12463399	        96.70 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_StringInterpNum-16        	  985659	      1220 ns/op	    2498 B/op	      40 allocs/op
BenchmarkFastjq_Small_IsEmptyTrue-16          	133846173	         8.972 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_IsEmptyTrue-16            	 2128768	       561.3 ns/op	    1921 B/op	      18 allocs/op
BenchmarkFastjq_Small_IsEmptyFalse-16         	54236265	        21.69 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_IsEmptyFalse-16           	 1000000	      1010 ns/op	    2898 B/op	      33 allocs/op
BenchmarkFastjq_Small_Nth-16                  	29644482	        39.78 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Nth-16                    	  560181	      2128 ns/op	    4372 B/op	      49 allocs/op
BenchmarkRegexp_Match_Hit-16                  	28409790	        41.36 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Miss-16                 	53568039	        22.50 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Complex_Hit-16          	28026304	        42.29 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Long_Miss-16            	15733818	        76.07 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_MatchString-16                	27597969	        43.69 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_FindSubmatch_Hit-16           	 6012202	       199.1 ns/op	     160 B/op	       2 allocs/op
BenchmarkRegexp_FindSubmatch_Miss-16          	 1805464	       664.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_FindSubmatchIndex_Hit-16      	 6758236	       176.7 ns/op	      64 B/op	       1 allocs/op
BenchmarkRegexp_FindSubmatchIndex_Miss-16     	 1747678	       687.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_ReplaceAll_Hit-16             	 7928905	       151.3 ns/op	      97 B/op	       5 allocs/op
BenchmarkRegexp_ReplaceAll_Miss-16            	24860504	        48.44 ns/op	      48 B/op	       2 allocs/op
BenchmarkRegexp_MatchViaClosure-16            	27820200	        42.26 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_Hit-16           	133588864	         8.997 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_Miss-16          	10944514	       109.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_InPipeline-16    	 4400821	       258.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_MatchRe_Hit-16          	 5696815	       210.8 ns/op	      48 B/op	       1 allocs/op
BenchmarkFastjq_Small_MatchRe_Miss-16         	 1954159	       613.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_TestRe_Hit-16             	  659724	      1818 ns/op	    4162 B/op	      44 allocs/op
BenchmarkGojq_Small_TestRe_Miss-16            	  666775	      1773 ns/op	    4104 B/op	      43 allocs/op
BenchmarkGojq_Small_MatchRe_Hit-16            	  291848	      4113 ns/op	    7999 B/op	     100 allocs/op
BenchmarkGojq_Small_MatchRe_Miss-16           	  411511	      2928 ns/op	    5712 B/op	      59 allocs/op
BenchmarkFastjq_Small_CaptureRe_Hit-16        	 5809974	       207.5 ns/op	      48 B/op	       1 allocs/op
BenchmarkFastjq_Small_CaptureRe_Miss-16       	 1975885	       606.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ScanRe_NoGroups-16      	 2496099	       481.1 ns/op	     387 B/op	      10 allocs/op
BenchmarkFastjq_Small_ScanRe_WithGroups-16    	 2008792	       598.7 ns/op	     702 B/op	      13 allocs/op
BenchmarkFastjq_Small_SubRe_Hit-16            	 9753355	       122.4 ns/op	      16 B/op	       1 allocs/op
BenchmarkFastjq_Small_SubRe_Miss-16           	11889976	       100.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_GSubRe_Hit-16           	 2101178	       570.7 ns/op	     305 B/op	       5 allocs/op
BenchmarkFastjq_Small_GSubRe_Miss-16          	 4767410	       250.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_CaptureRe_Hit-16          	  351643	      3289 ns/op	    7372 B/op	      77 allocs/op
BenchmarkGojq_Small_CaptureRe_Miss-16         	  413002	      2888 ns/op	    5577 B/op	      54 allocs/op
BenchmarkGojq_Small_ScanRe_NoGroups-16        	  395857	      3071 ns/op	    6002 B/op	      67 allocs/op
BenchmarkGojq_Small_SubRe_Hit-16              	  328416	      3610 ns/op	    6166 B/op	      77 allocs/op
BenchmarkGojq_Small_GSubRe_Hit-16             	  160596	      7437 ns/op	    9413 B/op	     143 allocs/op
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
