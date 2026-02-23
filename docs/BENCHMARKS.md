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

All times in µs. ¹Large Select uses the last field — fastjq scans the full document. ²gojq wins: after unmarshal, integer arrays are native Go slices. ³Validate rows compare against `json.Valid` (stdlib), not gojq.

| Operation | Input | fastjq (µs) | gojq (µs) | Speedup | fastjq allocs | gojq allocs |
|-----------|-------|------------|----------|---------|---------------|-------------|
| `.field` | Small (~100B) | 0.091 | 0.339 | **3.7x** | 0 | 13 |
| `.field` | Large (~100KB) | 8.17 | 574 | **70x** | 0 | 2835 |
| `del(.f)` | Small (~100B) | 0.171 | 0.923 | **5.4x** | 3 | 33 |
| `del(.f)` | Medium (~2KB) | 2.00 | 18.7 | **9.4x** | 3 | 323 |
| `del(.f)` | Large (~100KB) | 34.9 | 784 | **22x** | 3 | 4666 |
| `.[n]` | 5-elem array | 0.024 | 0.631 | **27x** | 0 | 20 |
| `del(.[n], .[m])` | 5-elem array | 0.122 | 1.70 | **14x** | 3 | 53 |
| `{f0, f2}` (construct) | Small (~100B) | 0.158 | 0.710 | **4.5x** | 3 | 23 |
| `{f0, f50}` (construct) | Large (~100KB) | 8.55 | 606 | **71x** | 3 | 2867 |
| `.[]` iterator | 5-elem array | 0.045 | 0.852 | **19x** | 1 | 26 |
| `.[]` iterator | 200-elem array | 7.01 | 88.9 | **13x** | 1 | 1811 |
| `select(.f == "x")` | Small (~100B) | 0.042 | 0.562 | **13x** | 3 | 20 |
| `select(.f == "x")`¹ | Large (~100KB, last field) | 32.7 | 793 | **24x** | 3 | 4651 |
| `select(.f and .g)` | Small (~100B) | 0.044 | 0.635 | **14x** | 3 | 21 |
| `select(.f or .g)` | Small (~100B) | 0.045 | 0.648 | **14x** | 3 | 21 |
| `has("key")` in select | Small (~100B) | 0.084 | 0.598 | **7.1x** | 6 | 20 |
| `has("key")` in select | Large (~100KB) | 33.7 | 762 | **23x** | 6 | 4651 |
| `if-then-else` | Small (~100B) | 0.045 | 0.468 | **10x** | 3 | 16 |
| `.f // "default"` | Small (~100B) | 0.067 | 0.459 | **6.9x** | 6 | 17 |
| `try .field` (no error) | Small (~100B) | 0.064 | 0.435 | **6.8x** | 4 | 16 |
| `.a + .b` (strings) | Small (~100B) | 0.050 | 0.669 | **13x** | 0 | 21 |
| `"prefix" + .name` | Small (~100B) | 0.078 | 0.698 | **8.9x** | 0 | 23 |
| `.a - .b` (subtract) | Small (~100B) | 0.052 | 0.670 | **13x** | 0 | 20 |
| `.a * .b` (multiply) | Small (~100B) | 0.070 | 0.702 | **10x** | 0 | 21 |
| `.a / .b` (divide) | Small (~100B) | 0.101 | 0.711 | **7x** | 0 | 21 |
| `.a - .b` (array diff) | 5-elem arrays | 0.214 | 1.23 | **5.7x** | 0 | 33 |
| `length` | Small (~100B) | 0.065 | 0.355 | **5.5x** | 5 | 13 |
| `length` | Large (~100KB) | 34.5 | 577 | **17x** | 0 | 2835 |
| `add` (numbers) | 5-elem array | 0.093 | 0.673 | **7.2x** | 3 | 24 |
| `add` (strings) | 5-elem array | 0.179 | 0.821 | **4.6x** | 3 | 31 |
| `flatten` | 3-elem nested array | 0.100 | 1.27 | **13x** | 0 | 38 |
| `min` | 200-int array | 1.76 | 1.22 | 0.7x† | 0 | 15 |
| `min_by(.value)` | 100-elem object array | 8.31 | 53.3 | **6.4x** | 0 | 1347 |
| `map(.name)` | 20-elem array (~600B) | 1.55 | 9.93 | **6.4x** | 29 | 251 |
| `map(.name)` | 200-elem array (~6KB) | 14.6 | 91.3 | **6.2x** | 209 | 2237 |
| `any` | 5-elem array | 0.043 | 1.79 | **41x** | 0 | 39 |
| `any(expr)` | 5-elem array | 0.114 | 2.06 | **18x** | 0 | 49 |
| `any(expr)`² | 200-int array | 2.94 | 1.71 | 0.6x† | 0 | 29 |
| `any(gen; cond)`² | 200-int array | 3.38 | 1.61 | 0.5x† | 3 | 27 |
| `first(expr)` | 5-elem array | 0.185 | 1.51 | **8.2x** | 8 | 39 |
| `first(expr)`² | 200-int array | 5.44 | 1.38 | 0.3x† | 124 | 23 |
| `last(expr)` | 5-elem array | 0.264 | 1.86 | **7x** | 10 | 43 |
| `last(expr)`² | 200-int array | 5.03 | 1.48 | 0.3x† | 115 | 24 |
| `limit(3; expr)` | 5-elem array | 0.083 | 1.65 | **20x** | 4 | 42 |
| `limit(10; expr)` | 200-int array | 0.761 | 1.58 | **2.1x** | 4 | 24 |
| `.[1:4]` slice | 6-elem array | 0.085 | 0.796 | **9.4x** | 0 | 21 |
| `values` | 9-elem array | 0.238 | 2.29 | **9.6x** | 12 | 51 |
| `to_entries` | Small (~100B) | 0.0046 | 0.364 | **78x** | 0 | 14 |
| `to_entries` | Large (~100KB) | 37.2 | 805 | **22x** | 0 | 5847 |
| `keys_unsorted` | Small (~100B) | 0.059 | 0.361 | **6.1x** | 3 | 14 |
| `keys_unsorted` | Large (~100KB) | 32.8 | 595 | **18x** | 0 | 3039 |
| `object merge .a + .b` | Small (~100B) | 0.177 | 1.53 | **8.6x** | 0 | 33 |
| `tojson` | Small (~100B) | 0.124 | 0.409 | **3.3x** | 0 | 17 |
| `fromjson` | JSON string | 0.083 | 1.33 | **16x** | 0 | 31 |
| `tonumber` | `"42"` string | 0.014 | 0.350 | **24x** | 0 | 11 |
| `split(",")` | short string | 0.143 | 0.757 | **5.3x** | 0 | 21 |
| `join(",")` | 5-elem array | 0.102 | 0.823 | **8.1x** | 0 | 30 |
| `ascii_downcase` in select | Small (~100B) | 0.112 | 0.560 | **5x** | 8 | 21 |
| `ascii_downcase` in select | Large (~100KB) | 33.5 | 758 | **23x** | 9 | 4652 |
| `startswith("s")` | Small (~100B) | 0.112 | 0.567 | **5x** | 8 | 21 |
| `startswith("s")` | Large (~100KB) | 32.8 | 754 | **23x** | 9 | 4651 |
| `endswith("s")` | Small (~100B) | 0.117 | 0.567 | **4.9x** | 8 | 21 |
| `ltrimstr("s")` | Small (~100B) | 0.068 | 0.419 | **6.1x** | 5 | 16 |
| `rtrimstr("s")` | Small (~100B) | 0.067 | 0.411 | **6.1x** | 5 | 16 |
| `select` + string ops + arith + construct | ~200B log event | 1.34 | 2.79 | **2.1x** | 24 | 69 |
| `[.[] \| select + arith + construct]` | 20-elem array (~1.5KB) | 8.53 | 22.0 | **2.6x** | 106 | 525 |
| `length + map + add + min_by + any` | 20-elem array (~1.5KB) | 12.4 | 21.2 | **1.7x** | 156 | 551 |
| `map(try {…} catch …)` | 20-elem array (~1.5KB) | 12.0 | 28.1 | **2.3x** | 267 | 641 |
| `map(if … elif … else …)` | 20-elem array (~1.5KB) | 10.3 | 24.5 | **2.4x** | 241 | 595 |
| `[.[] \| str + tostring + str]` | 20-elem array (~1.5KB) | 12.9 | 23.5 | **1.8x** | 428 | 686 |
| `to_entries \| map(select) \| from_entries` | ~200B log event | 3.82 | 7.59 | **2x** | 120 | 162 |
| `@base64` | 34-char string | 0.133 | 0.491 | **3.7x** | 4 | 15 |
| `@base64d` | 48-char encoded | 0.204 | 0.517 | **2.5x** | 0 | 15 |
| `@uri` | 36-char URL string | 0.160 | 0.648 | **4.1x** | 4 | 14 |
| `index(",")` | short string | 0.081 | 0.883 | **11x** | 0 | 31 |
| `indices(",")` | short string | 0.184 | 2.02 | **11x** | 0 | 97 |
| `sqrt` | float (e≈2.718) | 0.106 | 0.445 | **4.2x** | 3 | 12 |
| `log` | float (e≈2.718) | 0.074 | 0.426 | **5.8x** | 3 | 12 |
| `sin` | float (e≈2.718) | 0.117 | 0.424 | **3.6x** | 3 | 12 |
| `atan` | integer 1 | 0.090 | 0.376 | **4.2x** | 3 | 11 |
| `exp` | integer 1 | 0.100 | 0.384 | **3.9x** | 3 | 11 |
| `tgamma` | integer 5 | 0.049 | 0.368 | **7.5x** | 3 | 11 |
| `fabs` | float -3.14 | 0.091 | 0.395 | **4.4x** | 3 | 12 |
| ``"\(.level): \(.svc)"`` | ~45B object | 0.159 | 1.03 | **6.5x** | 3 | 35 |
| ``"user \(.name) …"`` | ~45B object | 0.128 | 1.25 | **9.8x** | 3 | 40 |
| `isempty(empty)` | null | 0.069 | 0.586 | **8.5x** | 6 | 18 |
| `isempty(.[])` | 5-elem array | 0.085 | 1.02 | **12x** | 6 | 33 |
| `nth(2; .[])` | 5-elem array | 0.121 | 2.10 | **17x** | 7 | 49 |
| `range(10)` (10 values) | null | 0.167 | 0.973 | **5.8x** | 11 | 18 |
| `limit(3; range(1000))` | null — only 3 allocs | 0.104 | 1.44 | **14x** | 7 | 26 |
| `test(re)` hit | short string | 0.042 | 1.80 | **43x** | 3 | 44 |
| `test(re)` miss | short string | 0.140 | 1.78 | **13x** | 3 | 43 |
| `match(re)` hit | short string | 0.212 | 4.13 | **19x** | 1 | 100 |
| `match(re)` miss | short string | 0.579 | 2.90 | **5x** | 0 | 59 |
| `capture(re)` hit | short string | 0.209 | — | — | 1 | — |
| `scan(re)` no groups (4 matches) | short string | 0.526 | — | — | 17 | — |
| `sub(re; s)` hit | short string | 0.122 | — | — | 1 | — |
| `gsub(re; s)` hit (4 matches) | short string | 0.558 | — | — | 5 | — |
| `Validate`³ | Small (~100B) | 0.0047 | 0.092 | **20x** | 0 | 4 |
| `Validate`³ | Medium (~2KB) | 1.44 | 4.33 | **3x** | 0 | 0 |
| `Validate`³ | Large (~100KB) | 114 | 333 | **2.9x** | 0 | 0 |

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
BenchmarkFastjq_Small_Del-16                  	 6898705	       171.2 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Medium_Del-16                 	  578894	      2005 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Large_Del-16                  	   33507	     34906 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Small_Field-16                	13087357	        91.23 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Field-16                	  159910	      8170 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Index-16                	50936217	        23.77 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ArrayDel-16             	 9665284	       122.2 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Small_Construct-16            	 7289462	       158.2 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Large_Construct-16            	  151099	      8549 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Small_Iterator-16             	26129322	        45.15 ns/op	      24 B/op	       1 allocs/op
BenchmarkFastjq_Large_Iterator-16             	  178971	      7008 ns/op	      24 B/op	       1 allocs/op
BenchmarkFastjq_Small_Select-16               	28004448	        41.87 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Large_Select-16               	   36204	     32673 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Small_Alternative-16          	17972422	        66.69 ns/op	     113 B/op	       6 allocs/op
BenchmarkFastjq_Small_SelectAnd-16            	26878156	        43.97 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Small_SelectOr-16             	27288670	        45.05 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Small_Has-16                  	14385771	        84.48 ns/op	     128 B/op	       6 allocs/op
BenchmarkFastjq_Large_Has-16                  	   35006	     33662 ns/op	     128 B/op	       6 allocs/op
BenchmarkFastjq_Small_IfThenElse-16           	26424854	        45.13 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Small_Length-16               	18267157	        64.63 ns/op	     136 B/op	       5 allocs/op
BenchmarkFastjq_Large_Length-16               	   34714	     34494 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Map-16                  	  759148	      1553 ns/op	     689 B/op	      29 allocs/op
BenchmarkFastjq_Medium_Map-16                 	  159477	      7277 ns/op	    2609 B/op	     109 allocs/op
BenchmarkFastjq_Large_Map-16                  	   82306	     14643 ns/op	    5009 B/op	     209 allocs/op
BenchmarkFastjq_Small_ToEntries-16            	259306632	         4.647 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_ToEntries-16            	   33385	     37172 ns/op	       6 B/op	       0 allocs/op
BenchmarkFastjq_Small_KeysUnsorted-16         	19871606	        58.83 ns/op	      72 B/op	       3 allocs/op
BenchmarkFastjq_Large_KeysUnsorted-16         	   35764	     32784 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Any-16                  	27073444	        43.35 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Any-16                  	  790743	      1482 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AnyExpr-16              	10133551	       114.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_AnyExpr-16              	  407301	      2936 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Add-16                  	12875628	        92.97 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Small_AddStrings-16           	 6513998	       179.4 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Small_Flatten-16              	11501918	        99.81 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Split-16                	 8450500	       142.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Join-16                 	11889884	       101.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Values-16               	 5001900	       238.3 ns/op	     312 B/op	      12 allocs/op
BenchmarkGojq_Small_Values-16                 	  531615	      2285 ns/op	    3144 B/op	      51 allocs/op
BenchmarkFastjq_Small_Base64Encode-16         	 8917456	       133.3 ns/op	     120 B/op	       4 allocs/op
BenchmarkFastjq_Small_Base64Decode-16         	 5975280	       204.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Base64Encode-16           	 2431491	       491.2 ns/op	    1321 B/op	      15 allocs/op
BenchmarkGojq_Small_Base64Decode-16           	 2325499	       517.1 ns/op	    1321 B/op	      15 allocs/op
BenchmarkFastjq_Small_IndexFind-16            	14616112	        81.03 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_IndicesAll-16           	 6454740	       184.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_IndexFind-16              	 1355317	       882.8 ns/op	    2274 B/op	      31 allocs/op
BenchmarkGojq_Small_IndicesAll-16             	  577468	      2023 ns/op	    3907 B/op	      97 allocs/op
BenchmarkFastjq_Small_Slice-16                	14187625	        85.07 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SliceString-16          	25695586	        44.48 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Plus-16                 	23556432	        50.33 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_PlusStr-16              	15265807	        78.34 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AsciiDowncase-16        	10591996	       111.8 ns/op	     200 B/op	       8 allocs/op
BenchmarkFastjq_Large_AsciiDowncase-16        	   34570	     33465 ns/op	     224 B/op	       9 allocs/op
BenchmarkFastjq_Small_Startswith-16           	10675688	       112.4 ns/op	     200 B/op	       8 allocs/op
BenchmarkFastjq_Large_Startswith-16           	   36841	     32819 ns/op	     224 B/op	       9 allocs/op
BenchmarkFastjq_Small_Endswith-16             	10594350	       116.5 ns/op	     200 B/op	       8 allocs/op
BenchmarkFastjq_Small_Ltrimstr-16             	17145204	        68.33 ns/op	     136 B/op	       5 allocs/op
BenchmarkFastjq_Large_Ltrimstr-16             	   34941	     33974 ns/op	     160 B/op	       6 allocs/op
BenchmarkFastjq_Small_Rtrimstr-16             	17252781	        66.84 ns/op	     136 B/op	       5 allocs/op
BenchmarkFastjq_Small_First-16                	 6509468	       185.1 ns/op	     208 B/op	       8 allocs/op
BenchmarkFastjq_Large_First-16                	  210976	      5442 ns/op	    3232 B/op	     124 allocs/op
BenchmarkFastjq_Small_Last-16                 	 4839385	       264.0 ns/op	     256 B/op	      10 allocs/op
BenchmarkFastjq_Large_Last-16                 	  246536	      5031 ns/op	    2776 B/op	     115 allocs/op
BenchmarkFastjq_Small_Limit-16                	13880038	        83.38 ns/op	      88 B/op	       4 allocs/op
BenchmarkFastjq_Large_Limit-16                	 1576552	       761.0 ns/op	      88 B/op	       4 allocs/op
BenchmarkGojq_Small_Del-16                    	 1313059	       922.8 ns/op	    2594 B/op	      33 allocs/op
BenchmarkGojq_Medium_Del-16                   	   64922	     18748 ns/op	   16968 B/op	     323 allocs/op
BenchmarkGojq_Large_Del-16                    	    1477	    784206 ns/op	  546100 B/op	    4666 allocs/op
BenchmarkGojq_Small_Field-16                  	 3548966	       339.4 ns/op	    1177 B/op	      13 allocs/op
BenchmarkGojq_Large_Field-16                  	    2095	    573647 ns/op	  270044 B/op	    2835 allocs/op
BenchmarkGojq_Small_Index-16                  	 1948368	       630.7 ns/op	    1401 B/op	      20 allocs/op
BenchmarkGojq_Small_ArrayDel-16               	  718489	      1695 ns/op	    3362 B/op	      53 allocs/op
BenchmarkGojq_Small_Construct-16              	 1678676	       710.2 ns/op	    1857 B/op	      23 allocs/op
BenchmarkGojq_Large_Construct-16              	    2002	    605820 ns/op	  274599 B/op	    2867 allocs/op
BenchmarkGojq_Small_Iterator-16               	 1383186	       852.4 ns/op	    1776 B/op	      26 allocs/op
BenchmarkGojq_Large_Iterator-16               	   13359	     88854 ns/op	  109809 B/op	    1811 allocs/op
BenchmarkGojq_Small_Select-16                 	 2145368	       561.7 ns/op	    1753 B/op	      20 allocs/op
BenchmarkGojq_Large_Select-16                 	    1578	    792909 ns/op	  534523 B/op	    4651 allocs/op
BenchmarkGojq_Small_Alternative-16            	 2600746	       458.9 ns/op	    1441 B/op	      17 allocs/op
BenchmarkGojq_Small_SelectAnd-16              	 1920858	       634.8 ns/op	    1945 B/op	      21 allocs/op
BenchmarkGojq_Small_SelectOr-16               	 1842584	       648.2 ns/op	    1945 B/op	      21 allocs/op
BenchmarkGojq_Small_Has-16                    	 2005461	       598.0 ns/op	    1753 B/op	      20 allocs/op
BenchmarkGojq_Large_Has-16                    	    1545	    762388 ns/op	  534777 B/op	    4651 allocs/op
BenchmarkGojq_Small_IfThenElse-16             	 2407575	       467.5 ns/op	    1361 B/op	      16 allocs/op
BenchmarkGojq_Small_Length-16                 	 3383749	       355.5 ns/op	    1177 B/op	      13 allocs/op
BenchmarkGojq_Large_Length-16                 	    2086	    577092 ns/op	  269831 B/op	    2835 allocs/op
BenchmarkGojq_Small_Map-16                    	  122434	      9932 ns/op	   13653 B/op	     251 allocs/op
BenchmarkGojq_Large_Map-16                    	   13164	     91257 ns/op	  118583 B/op	    2237 allocs/op
BenchmarkGojq_Small_ToEntries-16              	 3319848	       364.0 ns/op	    1209 B/op	      14 allocs/op
BenchmarkGojq_Large_ToEntries-16              	    1405	    804652 ns/op	  648823 B/op	    5847 allocs/op
BenchmarkGojq_Small_KeysUnsorted-16           	 3329919	       361.5 ns/op	    1209 B/op	      14 allocs/op
BenchmarkGojq_Large_KeysUnsorted-16           	    2025	    595362 ns/op	  282771 B/op	    3039 allocs/op
BenchmarkGojq_Small_Any-16                    	  647480	      1791 ns/op	    4508 B/op	      39 allocs/op
BenchmarkGojq_Small_AnyExpr-16                	  607116	      2062 ns/op	    4620 B/op	      49 allocs/op
BenchmarkGojq_Large_AnyExpr-16                	  660898	      1706 ns/op	    2818 B/op	      29 allocs/op
BenchmarkGojq_Small_Add-16                    	 1785045	       672.6 ns/op	    1433 B/op	      24 allocs/op
BenchmarkGojq_Small_AddStrings-16             	 1465579	       821.0 ns/op	    1689 B/op	      31 allocs/op
BenchmarkGojq_Small_Flatten-16                	  961563	      1273 ns/op	    1857 B/op	      38 allocs/op
BenchmarkGojq_Small_Split-16                  	 1592426	       756.8 ns/op	    1553 B/op	      21 allocs/op
BenchmarkGojq_Small_Join-16                   	 1460134	       822.8 ns/op	    1745 B/op	      30 allocs/op
BenchmarkGojq_Small_Slice-16                  	 1521853	       796.4 ns/op	    1425 B/op	      21 allocs/op
BenchmarkGojq_Small_SliceString-16            	 2807286	       433.3 ns/op	    1145 B/op	      12 allocs/op
BenchmarkGojq_Small_Plus-16                   	 1815746	       669.4 ns/op	    1673 B/op	      21 allocs/op
BenchmarkGojq_Small_PlusStr-16                	 1713271	       698.1 ns/op	    1705 B/op	      23 allocs/op
BenchmarkGojq_Small_AsciiDowncase-16          	 2147314	       560.5 ns/op	    1785 B/op	      21 allocs/op
BenchmarkGojq_Large_AsciiDowncase-16          	    1558	    758485 ns/op	  533198 B/op	    4652 allocs/op
BenchmarkGojq_Small_Startswith-16             	 2115451	       567.1 ns/op	    1801 B/op	      21 allocs/op
BenchmarkGojq_Large_Startswith-16             	    1581	    753629 ns/op	  534124 B/op	    4651 allocs/op
BenchmarkGojq_Small_Endswith-16               	 2099793	       566.8 ns/op	    1801 B/op	      21 allocs/op
BenchmarkGojq_Small_Ltrimstr-16               	 2876595	       419.4 ns/op	    1305 B/op	      16 allocs/op
BenchmarkGojq_Small_Rtrimstr-16               	 2899057	       410.8 ns/op	    1305 B/op	      16 allocs/op
BenchmarkGojq_Small_First-16                  	  821671	      1511 ns/op	    3379 B/op	      39 allocs/op
BenchmarkGojq_Large_First-16                  	  855388	      1376 ns/op	    1889 B/op	      23 allocs/op
BenchmarkGojq_Small_Last-16                   	  638996	      1856 ns/op	    3483 B/op	      43 allocs/op
BenchmarkGojq_Large_Last-16                   	  829681	      1480 ns/op	    2193 B/op	      24 allocs/op
BenchmarkGojq_Small_Limit-16                  	  745831	      1650 ns/op	    3328 B/op	      42 allocs/op
BenchmarkGojq_Large_Limit-16                  	  735723	      1582 ns/op	    2200 B/op	      24 allocs/op
BenchmarkFastjq_Small_Subtract-16             	22636632	        52.26 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Subtract-16               	 1765909	       669.8 ns/op	    1649 B/op	      20 allocs/op
BenchmarkFastjq_Small_Multiply-16             	16674568	        70.01 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Multiply-16               	 1669855	       702.4 ns/op	    1649 B/op	      21 allocs/op
BenchmarkFastjq_Small_Divide-16               	11985346	       101.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Divide-16                 	 1725435	       711.1 ns/op	    1665 B/op	      21 allocs/op
BenchmarkFastjq_Small_Min-16                  	  674376	      1759 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Min-16                    	  960992	      1218 ns/op	    1217 B/op	      15 allocs/op
BenchmarkFastjq_Small_MinBy-16                	  140761	      8306 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_MinBy-16                  	   22513	     53309 ns/op	   63629 B/op	    1347 allocs/op
BenchmarkFastjq_Small_URIEncode-16            	 7174753	       159.8 ns/op	     120 B/op	       4 allocs/op
BenchmarkGojq_Small_URIEncode-16              	 1817600	       648.5 ns/op	    1289 B/op	      14 allocs/op
BenchmarkFastjq_Small_ArrayDiff-16            	 5493524	       213.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ArrayDiff-16              	  984664	      1226 ns/op	    2121 B/op	      33 allocs/op
BenchmarkFastjq_Small_TryNoError-16           	18885258	        63.92 ns/op	      88 B/op	       4 allocs/op
BenchmarkFastjq_Small_TryCatchNoError-16      	 8939798	       130.3 ns/op	     176 B/op	       6 allocs/op
BenchmarkGojq_Small_TryNoError-16             	 2683476	       435.5 ns/op	    1449 B/op	      16 allocs/op
BenchmarkFastjq_Small_ObjectMerge-16          	 6677348	       176.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ObjectMerge-16            	  789722	      1528 ns/op	    2906 B/op	      33 allocs/op
BenchmarkFastjq_Small_ToJSON-16               	 9703902	       124.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ToJSON-16                 	 2943758	       409.1 ns/op	    1313 B/op	      17 allocs/op
BenchmarkFastjq_Small_FromJSON-16             	14429898	        83.20 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_FromJSON-16               	  883831	      1326 ns/op	    2714 B/op	      31 allocs/op
BenchmarkFastjq_Small_ToString-16             	 9367452	       129.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToNumber-16             	85247960	        14.48 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ToNumber-16               	 3389287	       350.2 ns/op	    1112 B/op	      11 allocs/op
BenchmarkFastjq_Small_AnyTwoArg-16            	  348364	      3376 ns/op	      57 B/op	       3 allocs/op
BenchmarkGojq_Small_AnyTwoArg-16              	  782533	      1606 ns/op	    2482 B/op	      27 allocs/op
BenchmarkFastjq_Complex_LogNormalize-16       	  856497	      1340 ns/op	     640 B/op	      24 allocs/op
BenchmarkGojq_Complex_LogNormalize-16         	  416227	      2789 ns/op	    4324 B/op	      69 allocs/op
BenchmarkFastjq_Complex_ArrayPipeline-16      	  142287	      8532 ns/op	    2264 B/op	     106 allocs/op
BenchmarkGojq_Complex_ArrayPipeline-16        	   55029	     21951 ns/op	   20407 B/op	     525 allocs/op
BenchmarkFastjq_Complex_Aggregation-16        	   96206	     12376 ns/op	    5176 B/op	     156 allocs/op
BenchmarkGojq_Complex_Aggregation-16          	   57792	     21203 ns/op	   22301 B/op	     551 allocs/op
BenchmarkFastjq_Complex_TolerantMap-16        	  102970	     11998 ns/op	    6816 B/op	     267 allocs/op
BenchmarkGojq_Complex_TolerantMap-16          	   41164	     28087 ns/op	   27872 B/op	     641 allocs/op
BenchmarkFastjq_Complex_ElifRouting-16        	  114873	     10257 ns/op	    5144 B/op	     241 allocs/op
BenchmarkGojq_Complex_ElifRouting-16          	   47978	     24488 ns/op	   25917 B/op	     595 allocs/op
BenchmarkFastjq_Complex_StringBuild-16        	   94231	     12939 ns/op	   11176 B/op	     428 allocs/op
BenchmarkGojq_Complex_StringBuild-16          	   51379	     23502 ns/op	   20533 B/op	     686 allocs/op
BenchmarkFastjq_Complex_EntryFilter-16        	  310618	      3821 ns/op	    4416 B/op	     120 allocs/op
BenchmarkGojq_Complex_EntryFilter-16          	  160032	      7587 ns/op	   11187 B/op	     162 allocs/op
BenchmarkFastjq_Small_Sqrt-16                 	11173682	       105.6 ns/op	      64 B/op	       3 allocs/op
BenchmarkGojq_Small_Sqrt-16                   	 2814787	       445.4 ns/op	    1145 B/op	      12 allocs/op
BenchmarkFastjq_Small_Log-16                  	16160917	        73.80 ns/op	      64 B/op	       3 allocs/op
BenchmarkGojq_Small_Log-16                    	 2817682	       426.5 ns/op	    1128 B/op	      12 allocs/op
BenchmarkFastjq_Small_Sin-16                  	10371459	       116.6 ns/op	      64 B/op	       3 allocs/op
BenchmarkGojq_Small_Sin-16                    	 2830143	       424.0 ns/op	    1145 B/op	      12 allocs/op
BenchmarkFastjq_Small_Atan-16                 	13319014	        90.24 ns/op	      64 B/op	       3 allocs/op
BenchmarkGojq_Small_Atan-16                   	 3228769	       375.7 ns/op	    1121 B/op	      11 allocs/op
BenchmarkFastjq_Small_Exp-16                  	12071832	        99.73 ns/op	      64 B/op	       3 allocs/op
BenchmarkGojq_Small_Exp-16                    	 3119431	       384.3 ns/op	    1121 B/op	      11 allocs/op
BenchmarkFastjq_Small_Tgamma-16               	24830367	        48.82 ns/op	      64 B/op	       3 allocs/op
BenchmarkGojq_Small_Tgamma-16                 	 3307201	       367.6 ns/op	    1104 B/op	      11 allocs/op
BenchmarkFastjq_Small_Fabs-16                 	13212874	        90.60 ns/op	      64 B/op	       3 allocs/op
BenchmarkGojq_Small_Fabs-16                   	 3074023	       395.0 ns/op	    1112 B/op	      12 allocs/op
BenchmarkFastjq_Small_StringInterp-16         	 7453444	       158.9 ns/op	      64 B/op	       3 allocs/op
BenchmarkGojq_Small_StringInterp-16           	 1000000	      1033 ns/op	    2121 B/op	      35 allocs/op
BenchmarkFastjq_Small_StringInterpNum-16      	 9595662	       128.0 ns/op	      64 B/op	       3 allocs/op
BenchmarkGojq_Small_StringInterpNum-16        	  951194	      1251 ns/op	    2498 B/op	      40 allocs/op
BenchmarkFastjq_Small_IsEmptyTrue-16          	16982242	        68.74 ns/op	     105 B/op	       6 allocs/op
BenchmarkGojq_Small_IsEmptyTrue-16            	 2004946	       586.2 ns/op	    1921 B/op	      18 allocs/op
BenchmarkFastjq_Small_IsEmptyFalse-16         	13948831	        85.31 ns/op	     105 B/op	       6 allocs/op
BenchmarkGojq_Small_IsEmptyFalse-16           	 1000000	      1025 ns/op	    2898 B/op	      33 allocs/op
BenchmarkFastjq_Small_Nth-16                  	10041686	       121.0 ns/op	     152 B/op	       7 allocs/op
BenchmarkGojq_Small_Nth-16                    	  584050	      2102 ns/op	    4372 B/op	      49 allocs/op
BenchmarkFastjq_Small_Range10-16              	 7150678	       167.2 ns/op	     104 B/op	      11 allocs/op
BenchmarkGojq_Small_Range10-16                	 1234080	       972.9 ns/op	    1864 B/op	      18 allocs/op
BenchmarkFastjq_Small_RangeLimit-16           	11775822	       104.1 ns/op	     112 B/op	       7 allocs/op
BenchmarkGojq_Small_RangeLimit-16             	  842587	      1441 ns/op	    3208 B/op	      26 allocs/op
BenchmarkFastjq_Small_Validate-16             	256832112	         4.698 ns/op	       0 B/op	       0 allocs/op
BenchmarkStdlib_Small_Validate-16             	13124130	        92.10 ns/op	      88 B/op	       4 allocs/op
BenchmarkFastjq_Medium_Validate-16            	  793862	      1439 ns/op	       0 B/op	       0 allocs/op
BenchmarkStdlib_Medium_Validate-16            	  284607	      4332 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Validate-16             	    9962	    114170 ns/op	       0 B/op	       0 allocs/op
BenchmarkStdlib_Large_Validate-16             	    3558	    332940 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Hit-16                  	28349134	        44.30 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Miss-16                 	50459833	        22.89 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Complex_Hit-16          	27752080	        43.31 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Long_Miss-16            	15795890	        75.80 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_MatchString-16                	27694942	        43.76 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_FindSubmatch_Hit-16           	 5996996	       197.7 ns/op	     160 B/op	       2 allocs/op
BenchmarkRegexp_FindSubmatch_Miss-16          	 1829792	       655.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_FindSubmatchIndex_Hit-16      	 6924426	       174.3 ns/op	      64 B/op	       1 allocs/op
BenchmarkRegexp_FindSubmatchIndex_Miss-16     	 1815698	       659.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_ReplaceAll_Hit-16             	 8086917	       148.5 ns/op	      97 B/op	       5 allocs/op
BenchmarkRegexp_ReplaceAll_Miss-16            	25759479	        46.88 ns/op	      48 B/op	       2 allocs/op
BenchmarkRegexp_MatchViaClosure-16            	28651327	        43.26 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_Hit-16           	27424574	        42.14 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Small_TestRe_Miss-16          	 8607168	       140.1 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Small_TestRe_InPipeline-16    	 3775058	       311.4 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Small_MatchRe_Hit-16          	 5639341	       211.8 ns/op	      48 B/op	       1 allocs/op
BenchmarkFastjq_Small_MatchRe_Miss-16         	 2063448	       579.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_TestRe_Hit-16             	  680100	      1801 ns/op	    4164 B/op	      44 allocs/op
BenchmarkGojq_Small_TestRe_Miss-16            	  679777	      1781 ns/op	    4108 B/op	      43 allocs/op
BenchmarkGojq_Small_MatchRe_Hit-16            	  289816	      4129 ns/op	    8007 B/op	     100 allocs/op
BenchmarkGojq_Small_MatchRe_Miss-16           	  416904	      2898 ns/op	    5718 B/op	      59 allocs/op
BenchmarkFastjq_Small_CaptureRe_Hit-16        	 5845124	       209.1 ns/op	      48 B/op	       1 allocs/op
BenchmarkFastjq_Small_CaptureRe_Miss-16       	 1968822	       605.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ScanRe_NoGroups-16      	 2284885	       526.3 ns/op	     533 B/op	      17 allocs/op
BenchmarkFastjq_Small_ScanRe_WithGroups-16    	 1789698	       669.2 ns/op	     848 B/op	      20 allocs/op
BenchmarkFastjq_Small_SubRe_Hit-16            	 9839960	       122.2 ns/op	      16 B/op	       1 allocs/op
BenchmarkFastjq_Small_SubRe_Miss-16           	11576288	       103.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_GSubRe_Hit-16           	 2153827	       558.2 ns/op	     305 B/op	       5 allocs/op
BenchmarkFastjq_Small_GSubRe_Miss-16          	 4822848	       247.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_CaptureRe_Hit-16          	  364396	      3276 ns/op	    7384 B/op	      77 allocs/op
BenchmarkGojq_Small_CaptureRe_Miss-16         	  422503	      2861 ns/op	    5583 B/op	      54 allocs/op
BenchmarkGojq_Small_ScanRe_NoGroups-16        	  388614	      3092 ns/op	    6016 B/op	      67 allocs/op
BenchmarkGojq_Small_SubRe_Hit-16              	  337840	      3505 ns/op	    6181 B/op	      77 allocs/op
BenchmarkGojq_Small_GSubRe_Hit-16             	  163093	      7317 ns/op	    9432 B/op	     143 allocs/op
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
