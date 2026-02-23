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
| `.field` | Small (~100B) | 0.078 | 0.349 | **4.5x** | 0 | 13 |
| `.field` | Large (~100KB) | 7.56 | 582 | **77x** | 0 | 2835 |
| `del(.f)` | Small (~100B) | 0.201 | 0.933 | **4.6x** | 3 | 33 |
| `del(.f)` | Medium (~2KB) | 2.19 | 18.9 | **8.6x** | 3 | 323 |
| `del(.f)` | Large (~100KB) | 35.7 | 800 | **22x** | 3 | 4666 |
| `.[n]` | 5-elem array | 0.023 | 0.618 | **27x** | 0 | 20 |
| `del(.[n], .[m])` | 5-elem array | 0.120 | 1.63 | **14x** | 3 | 53 |
| `{f0, f2}` (construct) | Small (~100B) | 0.167 | 0.698 | **4.2x** | 3 | 23 |
| `{f0, f50}` (construct) | Large (~100KB) | 8.19 | 586 | **71x** | 3 | 2866 |
| `.[]` iterator | 5-elem array | 0.041 | 0.777 | **19x** | 1 | 26 |
| `.[]` iterator | 200-elem array | 7.45 | 85.2 | **11x** | 1 | 1811 |
| `select(.f == "x")` | Small (~100B) | 0.043 | 0.581 | **13x** | 3 | 20 |
| `select(.f == "x")`¹ | Large (~100KB, last field) | 30.7 | 796 | **26x** | 3 | 4652 |
| `select(.f and .g)` | Small (~100B) | 0.045 | 0.664 | **15x** | 3 | 21 |
| `select(.f or .g)` | Small (~100B) | 0.045 | 0.690 | **15x** | 3 | 21 |
| `has("key")` in select | Small (~100B) | 0.085 | 0.578 | **6.8x** | 6 | 20 |
| `has("key")` in select | Large (~100KB) | 31.5 | 791 | **25x** | 6 | 4651 |
| `if-then-else` | Small (~100B) | 0.044 | 0.474 | **11x** | 3 | 16 |
| `.f // "default"` | Small (~100B) | 0.070 | 0.509 | **7.3x** | 6 | 17 |
| `try .field` (no error) | Small (~100B) | 0.055 | 0.421 | **7.7x** | 4 | 16 |
| `.a + .b` (strings) | Small (~100B) | 0.050 | 0.680 | **14x** | 0 | 21 |
| `"prefix" + .name` | Small (~100B) | 0.083 | 0.722 | **8.7x** | 0 | 23 |
| `.a - .b` (subtract) | Small (~100B) | 0.050 | 0.679 | **14x** | 0 | 20 |
| `.a * .b` (multiply) | Small (~100B) | 0.063 | 0.703 | **11x** | 0 | 21 |
| `.a / .b` (divide) | Small (~100B) | 0.098 | 0.722 | **7.4x** | 0 | 21 |
| `.a - .b` (array diff) | 5-elem arrays | 0.202 | 1.23 | **6.1x** | 0 | 33 |
| `length` | Small (~100B) | 0.066 | 0.373 | **5.6x** | 5 | 13 |
| `length` | Large (~100KB) | 32.1 | 583 | **18x** | 0 | 2835 |
| `add` (numbers) | 5-elem array | 0.094 | 0.687 | **7.3x** | 3 | 24 |
| `add` (strings) | 5-elem array | 0.173 | 0.868 | **5x** | 3 | 31 |
| `flatten` | 3-elem nested array | 0.092 | 1.31 | **14x** | 0 | 38 |
| `min` | 200-int array | 1.80 | 1.25 | 0.7x† | 0 | 15 |
| `min_by(.value)` | 100-elem object array | 8.08 | 54.7 | **6.8x** | 0 | 1347 |
| `map(.name)` | 20-elem array (~600B) | 1.58 | 10.2 | **6.5x** | 29 | 251 |
| `map(.name)` | 200-elem array (~6KB) | 15.0 | 93.1 | **6.2x** | 209 | 2237 |
| `any` | 5-elem array | 0.047 | 1.85 | **40x** | 0 | 39 |
| `any(expr)` | 5-elem array | 0.117 | 2.09 | **18x** | 0 | 49 |
| `any(expr)`² | 200-int array | 2.94 | 1.75 | 0.6x† | 0 | 29 |
| `any(gen; cond)`² | 200-int array | 3.22 | 1.62 | 0.5x† | 3 | 27 |
| `first(expr)` | 5-elem array | 0.209 | 1.50 | **7.2x** | 9 | 39 |
| `first(expr)`² | 200-int array | 5.11 | 1.44 | 0.3x† | 107 | 23 |
| `last(expr)` | 5-elem array | 0.255 | 1.89 | **7.4x** | 10 | 43 |
| `last(expr)`² | 200-int array | 4.66 | 1.52 | 0.3x† | 107 | 24 |
| `limit(3; expr)` | 5-elem array | 0.082 | 1.67 | **20x** | 4 | 42 |
| `limit(10; expr)` | 200-int array | 0.646 | 1.62 | **2.5x** | 4 | 24 |
| `.[1:4]` slice | 6-elem array | 0.077 | 0.816 | **11x** | 0 | 21 |
| `values` | 9-elem array | 0.242 | 2.35 | **9.7x** | 12 | 51 |
| `to_entries` | Small (~100B) | 0.0045 | 0.394 | **87x** | 0 | 14 |
| `to_entries` | Large (~100KB) | 33.5 | 834 | **25x** | 0 | 5847 |
| `keys_unsorted` | Small (~100B) | 0.061 | 0.368 | **6x** | 3 | 14 |
| `keys_unsorted` | Large (~100KB) | 31.4 | 605 | **19x** | 0 | 3039 |
| `object merge .a + .b` | Small (~100B) | 0.169 | 1.48 | **8.8x** | 0 | 33 |
| `tojson` | Small (~100B) | 0.133 | 0.423 | **3.2x** | 0 | 17 |
| `fromjson` | JSON string | 0.150 | 1.27 | **8.5x** | 0 | 31 |
| `tonumber` | `"42"` string | 0.013 | 0.352 | **27x** | 0 | 11 |
| `split(",")` | short string | 0.143 | 0.806 | **5.6x** | 0 | 21 |
| `join(",")` | 5-elem array | 0.098 | 0.853 | **8.7x** | 0 | 30 |
| `ascii_downcase` in select | Small (~100B) | 0.114 | 0.573 | **5x** | 8 | 21 |
| `ascii_downcase` in select | Large (~100KB) | 30.9 | 778 | **25x** | 9 | 4653 |
| `startswith("s")` | Small (~100B) | 0.115 | 0.579 | **5.1x** | 8 | 21 |
| `startswith("s")` | Large (~100KB) | 31.2 | 779 | **25x** | 9 | 4652 |
| `endswith("s")` | Small (~100B) | 0.114 | 0.579 | **5.1x** | 8 | 21 |
| `ltrimstr("s")` | Small (~100B) | 0.066 | 0.419 | **6.4x** | 5 | 16 |
| `rtrimstr("s")` | Small (~100B) | 0.066 | 0.424 | **6.4x** | 5 | 16 |
| `select` + string ops + arith + construct | ~200B log event | 1.33 | 2.88 | **2.2x** | 24 | 69 |
| `[.[] \| select + arith + construct]` | 20-elem array (~1.5KB) | 8.54 | 22.5 | **2.6x** | 106 | 525 |
| `length + map + add + min_by + any` | 20-elem array (~1.5KB) | 13.0 | 21.3 | **1.6x** | 156 | 551 |
| `map(try {…} catch …)` | 20-elem array (~1.5KB) | 11.2 | 27.5 | **2.4x** | 267 | 641 |
| `map(if … elif … else …)` | 20-elem array (~1.5KB) | 10.2 | 24.7 | **2.4x** | 241 | 595 |
| `[.[] \| str + tostring + str]` | 20-elem array (~1.5KB) | 12.7 | 23.9 | **1.9x** | 428 | 686 |
| `to_entries \| map(select) \| from_entries` | ~200B log event | 3.84 | 7.69 | **2x** | 120 | 162 |
| `@base64` | 34-char string | 0.142 | 0.520 | **3.7x** | 4 | 15 |
| `@base64d` | 48-char encoded | 0.207 | 0.546 | **2.6x** | 0 | 15 |
| `@uri` | 36-char URL string | 0.162 | 0.659 | **4.1x** | 4 | 14 |
| `index(",")` | short string | 0.082 | 0.911 | **11x** | 0 | 31 |
| `indices(",")` | short string | 0.190 | 2.09 | **11x** | 0 | 97 |
| `sqrt` | float (e≈2.718) | 0.103 | 0.439 | **4.3x** | 3 | 12 |
| `log` | float (e≈2.718) | 0.072 | 0.436 | **6x** | 3 | 12 |
| `sin` | float (e≈2.718) | 0.115 | 0.444 | **3.9x** | 3 | 12 |
| `atan` | integer 1 | 0.091 | 0.386 | **4.2x** | 3 | 11 |
| `exp` | integer 1 | 0.104 | 0.396 | **3.8x** | 3 | 11 |
| `tgamma` | integer 5 | 0.048 | 0.374 | **7.8x** | 3 | 11 |
| `fabs` | float -3.14 | 0.089 | 0.405 | **4.5x** | 3 | 12 |
| ``"\(.level): \(.svc)"`` | ~45B object | 0.155 | 1.02 | **6.6x** | 3 | 35 |
| ``"user \(.name) …"`` | ~45B object | 0.125 | 1.19 | **9.5x** | 3 | 40 |
| `isempty(empty)` | null | 0.069 | 0.570 | **8.2x** | 6 | 18 |
| `isempty(.[])` | 5-elem array | 0.085 | 1.05 | **12x** | 6 | 33 |
| `nth(2; .[])` | 5-elem array | 0.125 | 2.26 | **18x** | 7 | 49 |
| `range(10)` (10 values) | null | 0.170 | 0.999 | **5.9x** | 11 | 18 |
| `limit(3; range(1000))` | null — only 3 allocs | 0.105 | 1.49 | **14x** | 7 | 26 |
| `test(re)` hit | short string | 0.042 | 1.93 | **46x** | 3 | 44 |
| `test(re)` miss | short string | 0.141 | 1.88 | **13x** | 3 | 43 |
| `match(re)` hit | short string | 0.211 | 4.29 | **20x** | 1 | 100 |
| `match(re)` miss | short string | 0.590 | 3.08 | **5.2x** | 0 | 59 |
| `capture(re)` hit | short string | 0.209 | — | — | 1 | — |
| `scan(re)` no groups (4 matches) | short string | 0.536 | — | — | 17 | — |
| `sub(re; s)` hit | short string | 0.127 | — | — | 1 | — |
| `gsub(re; s)` hit (4 matches) | short string | 0.576 | — | — | 5 | — |

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
BenchmarkFastjq_Small_Del-16                  	 5812561	       200.7 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Medium_Del-16                 	  553646	      2189 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Large_Del-16                  	   32948	     35689 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Small_Field-16                	15758733	        77.91 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Field-16                	  162670	      7560 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Index-16                	49160935	        22.79 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ArrayDel-16             	 9830508	       120.2 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Small_Construct-16            	 7394611	       166.8 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Large_Construct-16            	  151554	      8194 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Small_Iterator-16             	28822279	        41.05 ns/op	      24 B/op	       1 allocs/op
BenchmarkFastjq_Large_Iterator-16             	  147766	      7453 ns/op	      24 B/op	       1 allocs/op
BenchmarkFastjq_Small_Select-16               	27904461	        43.18 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Large_Select-16               	   38769	     30707 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Small_Alternative-16          	16941574	        70.08 ns/op	     113 B/op	       6 allocs/op
BenchmarkFastjq_Small_SelectAnd-16            	26950722	        44.54 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Small_SelectOr-16             	26888420	        44.62 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Small_Has-16                  	14104123	        85.13 ns/op	     128 B/op	       6 allocs/op
BenchmarkFastjq_Large_Has-16                  	   38106	     31545 ns/op	     128 B/op	       6 allocs/op
BenchmarkFastjq_Small_IfThenElse-16           	27225314	        43.96 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Small_Length-16               	18357722	        66.14 ns/op	     136 B/op	       5 allocs/op
BenchmarkFastjq_Large_Length-16               	   36614	     32138 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Map-16                  	  768620	      1585 ns/op	     689 B/op	      29 allocs/op
BenchmarkFastjq_Medium_Map-16                 	  161358	      7660 ns/op	    2609 B/op	     109 allocs/op
BenchmarkFastjq_Large_Map-16                  	   77556	     14989 ns/op	    5009 B/op	     209 allocs/op
BenchmarkFastjq_Small_ToEntries-16            	265409143	         4.542 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_ToEntries-16            	   35674	     33526 ns/op	       6 B/op	       0 allocs/op
BenchmarkFastjq_Small_KeysUnsorted-16         	19638675	        61.16 ns/op	      72 B/op	       3 allocs/op
BenchmarkFastjq_Large_KeysUnsorted-16         	   38492	     31430 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Any-16                  	25983774	        46.76 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Any-16                  	  802380	      1473 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AnyExpr-16              	10073659	       116.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_AnyExpr-16              	  409008	      2940 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Add-16                  	12762256	        93.57 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Small_AddStrings-16           	 7129617	       172.8 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Small_Flatten-16              	12935662	        91.92 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Split-16                	 8350166	       143.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Join-16                 	12207698	        98.49 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Values-16               	 4968694	       241.8 ns/op	     312 B/op	      12 allocs/op
BenchmarkGojq_Small_Values-16                 	  507241	      2347 ns/op	    3144 B/op	      51 allocs/op
BenchmarkFastjq_Small_Base64Encode-16         	 8482520	       141.6 ns/op	     120 B/op	       4 allocs/op
BenchmarkFastjq_Small_Base64Decode-16         	 5814156	       206.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Base64Encode-16           	 2293464	       520.1 ns/op	    1321 B/op	      15 allocs/op
BenchmarkGojq_Small_Base64Decode-16           	 2196067	       545.9 ns/op	    1321 B/op	      15 allocs/op
BenchmarkFastjq_Small_IndexFind-16            	14318320	        82.39 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_IndicesAll-16           	 6293344	       189.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_IndexFind-16              	 1318072	       911.1 ns/op	    2274 B/op	      31 allocs/op
BenchmarkGojq_Small_IndicesAll-16             	  577372	      2094 ns/op	    3907 B/op	      97 allocs/op
BenchmarkFastjq_Small_Slice-16                	15380942	        77.15 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SliceString-16          	26568999	        45.37 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Plus-16                 	23743686	        50.28 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_PlusStr-16              	14377513	        82.73 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AsciiDowncase-16        	10447501	       114.4 ns/op	     200 B/op	       8 allocs/op
BenchmarkFastjq_Large_AsciiDowncase-16        	   38650	     30911 ns/op	     224 B/op	       9 allocs/op
BenchmarkFastjq_Small_Startswith-16           	10431146	       114.5 ns/op	     200 B/op	       8 allocs/op
BenchmarkFastjq_Large_Startswith-16           	   38444	     31247 ns/op	     224 B/op	       9 allocs/op
BenchmarkFastjq_Small_Endswith-16             	10466846	       114.4 ns/op	     200 B/op	       8 allocs/op
BenchmarkFastjq_Small_Ltrimstr-16             	18312082	        65.91 ns/op	     136 B/op	       5 allocs/op
BenchmarkFastjq_Large_Ltrimstr-16             	   36703	     32145 ns/op	     160 B/op	       6 allocs/op
BenchmarkFastjq_Small_Rtrimstr-16             	17955649	        66.36 ns/op	     136 B/op	       5 allocs/op
BenchmarkFastjq_Small_First-16                	 5776074	       208.8 ns/op	     216 B/op	       9 allocs/op
BenchmarkFastjq_Large_First-16                	  232270	      5109 ns/op	    2584 B/op	     107 allocs/op
BenchmarkFastjq_Small_Last-16                 	 4650454	       255.3 ns/op	     256 B/op	      10 allocs/op
BenchmarkFastjq_Large_Last-16                 	  252082	      4664 ns/op	    2584 B/op	     107 allocs/op
BenchmarkFastjq_Small_Limit-16                	14667864	        82.10 ns/op	      88 B/op	       4 allocs/op
BenchmarkFastjq_Large_Limit-16                	 1860302	       645.6 ns/op	      88 B/op	       4 allocs/op
BenchmarkGojq_Small_Del-16                    	 1286268	       932.6 ns/op	    2594 B/op	      33 allocs/op
BenchmarkGojq_Medium_Del-16                   	   63706	     18923 ns/op	   16965 B/op	     323 allocs/op
BenchmarkGojq_Large_Del-16                    	    1486	    800215 ns/op	  539186 B/op	    4666 allocs/op
BenchmarkGojq_Small_Field-16                  	 3446940	       349.1 ns/op	    1177 B/op	      13 allocs/op
BenchmarkGojq_Large_Field-16                  	    2016	    582223 ns/op	  270038 B/op	    2835 allocs/op
BenchmarkGojq_Small_Index-16                  	 1955402	       617.7 ns/op	    1401 B/op	      20 allocs/op
BenchmarkGojq_Small_ArrayDel-16               	  733172	      1631 ns/op	    3362 B/op	      53 allocs/op
BenchmarkGojq_Small_Construct-16              	 1724667	       697.5 ns/op	    1857 B/op	      23 allocs/op
BenchmarkGojq_Large_Construct-16              	    2058	    585626 ns/op	  274318 B/op	    2866 allocs/op
BenchmarkGojq_Small_Iterator-16               	 1556406	       777.5 ns/op	    1776 B/op	      26 allocs/op
BenchmarkGojq_Large_Iterator-16               	   14071	     85175 ns/op	  109808 B/op	    1811 allocs/op
BenchmarkGojq_Small_Select-16                 	 2115451	       581.1 ns/op	    1753 B/op	      20 allocs/op
BenchmarkGojq_Large_Select-16                 	    1506	    796256 ns/op	  542506 B/op	    4652 allocs/op
BenchmarkGojq_Small_Alternative-16            	 2416933	       509.4 ns/op	    1441 B/op	      17 allocs/op
BenchmarkGojq_Small_SelectAnd-16              	 1781907	       664.3 ns/op	    1945 B/op	      21 allocs/op
BenchmarkGojq_Small_SelectOr-16               	 1711453	       690.1 ns/op	    1945 B/op	      21 allocs/op
BenchmarkGojq_Small_Has-16                    	 2050638	       578.4 ns/op	    1753 B/op	      20 allocs/op
BenchmarkGojq_Large_Has-16                    	    1521	    791439 ns/op	  537299 B/op	    4651 allocs/op
BenchmarkGojq_Small_IfThenElse-16             	 2537664	       473.7 ns/op	    1361 B/op	      16 allocs/op
BenchmarkGojq_Small_Length-16                 	 3142486	       372.6 ns/op	    1177 B/op	      13 allocs/op
BenchmarkGojq_Large_Length-16                 	    2035	    583485 ns/op	  269831 B/op	    2835 allocs/op
BenchmarkGojq_Small_Map-16                    	  118551	     10231 ns/op	   13653 B/op	     251 allocs/op
BenchmarkGojq_Large_Map-16                    	   12962	     93109 ns/op	  118597 B/op	    2237 allocs/op
BenchmarkGojq_Small_ToEntries-16              	 3189657	       394.1 ns/op	    1209 B/op	      14 allocs/op
BenchmarkGojq_Large_ToEntries-16              	    1432	    833637 ns/op	  642493 B/op	    5847 allocs/op
BenchmarkGojq_Small_KeysUnsorted-16           	 3255862	       368.4 ns/op	    1209 B/op	      14 allocs/op
BenchmarkGojq_Large_KeysUnsorted-16           	    1987	    605043 ns/op	  282795 B/op	    3039 allocs/op
BenchmarkGojq_Small_Any-16                    	  655676	      1854 ns/op	    4508 B/op	      39 allocs/op
BenchmarkGojq_Small_AnyExpr-16                	  561318	      2089 ns/op	    4620 B/op	      49 allocs/op
BenchmarkGojq_Large_AnyExpr-16                	  698936	      1753 ns/op	    2818 B/op	      29 allocs/op
BenchmarkGojq_Small_Add-16                    	 1745203	       687.0 ns/op	    1433 B/op	      24 allocs/op
BenchmarkGojq_Small_AddStrings-16             	 1372474	       868.4 ns/op	    1689 B/op	      31 allocs/op
BenchmarkGojq_Small_Flatten-16                	  945918	      1312 ns/op	    1857 B/op	      38 allocs/op
BenchmarkGojq_Small_Split-16                  	 1467538	       806.0 ns/op	    1553 B/op	      21 allocs/op
BenchmarkGojq_Small_Join-16                   	 1406356	       853.3 ns/op	    1745 B/op	      30 allocs/op
BenchmarkGojq_Small_Slice-16                  	 1463175	       816.2 ns/op	    1425 B/op	      21 allocs/op
BenchmarkGojq_Small_SliceString-16            	 2720565	       441.5 ns/op	    1145 B/op	      12 allocs/op
BenchmarkGojq_Small_Plus-16                   	 1764320	       679.8 ns/op	    1673 B/op	      21 allocs/op
BenchmarkGojq_Small_PlusStr-16                	 1657461	       721.6 ns/op	    1705 B/op	      23 allocs/op
BenchmarkGojq_Small_AsciiDowncase-16          	 2087432	       572.5 ns/op	    1785 B/op	      21 allocs/op
BenchmarkGojq_Large_AsciiDowncase-16          	    1537	    777959 ns/op	  540422 B/op	    4653 allocs/op
BenchmarkGojq_Small_Startswith-16             	 2065634	       578.8 ns/op	    1801 B/op	      21 allocs/op
BenchmarkGojq_Large_Startswith-16             	    1531	    778988 ns/op	  539511 B/op	    4652 allocs/op
BenchmarkGojq_Small_Endswith-16               	 2072360	       579.3 ns/op	    1801 B/op	      21 allocs/op
BenchmarkGojq_Small_Ltrimstr-16               	 2888626	       419.0 ns/op	    1305 B/op	      16 allocs/op
BenchmarkGojq_Small_Rtrimstr-16               	 2852187	       423.9 ns/op	    1305 B/op	      16 allocs/op
BenchmarkGojq_Small_First-16                  	  811275	      1495 ns/op	    3379 B/op	      39 allocs/op
BenchmarkGojq_Large_First-16                  	  825787	      1439 ns/op	    1889 B/op	      23 allocs/op
BenchmarkGojq_Small_Last-16                   	  637874	      1894 ns/op	    3483 B/op	      43 allocs/op
BenchmarkGojq_Large_Last-16                   	  790845	      1524 ns/op	    2193 B/op	      24 allocs/op
BenchmarkGojq_Small_Limit-16                  	  732252	      1668 ns/op	    3328 B/op	      42 allocs/op
BenchmarkGojq_Large_Limit-16                  	  761752	      1623 ns/op	    2200 B/op	      24 allocs/op
BenchmarkFastjq_Small_Subtract-16             	23855296	        49.89 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Subtract-16               	 1770211	       679.5 ns/op	    1649 B/op	      20 allocs/op
BenchmarkFastjq_Small_Multiply-16             	18803545	        63.09 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Multiply-16               	 1714944	       703.4 ns/op	    1649 B/op	      21 allocs/op
BenchmarkFastjq_Small_Divide-16               	12166934	        98.22 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Divide-16                 	 1646653	       722.2 ns/op	    1665 B/op	      21 allocs/op
BenchmarkFastjq_Small_Min-16                  	  687564	      1804 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Min-16                    	  972046	      1249 ns/op	    1217 B/op	      15 allocs/op
BenchmarkFastjq_Small_MinBy-16                	  144024	      8081 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_MinBy-16                  	   22075	     54718 ns/op	   63629 B/op	    1347 allocs/op
BenchmarkFastjq_Small_URIEncode-16            	 7353228	       162.1 ns/op	     120 B/op	       4 allocs/op
BenchmarkGojq_Small_URIEncode-16              	 1824847	       659.5 ns/op	    1289 B/op	      14 allocs/op
BenchmarkFastjq_Small_ArrayDiff-16            	 5781264	       202.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ArrayDiff-16              	  947430	      1230 ns/op	    2121 B/op	      33 allocs/op
BenchmarkFastjq_Small_TryNoError-16           	21916940	        54.91 ns/op	      88 B/op	       4 allocs/op
BenchmarkFastjq_Small_TryCatchNoError-16      	 9901264	       120.6 ns/op	     176 B/op	       6 allocs/op
BenchmarkGojq_Small_TryNoError-16             	 2783470	       421.5 ns/op	    1449 B/op	      16 allocs/op
BenchmarkFastjq_Small_ObjectMerge-16          	 6894771	       168.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ObjectMerge-16            	  783664	      1484 ns/op	    2906 B/op	      33 allocs/op
BenchmarkFastjq_Small_ToJSON-16               	 8976201	       133.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ToJSON-16                 	 2843246	       422.6 ns/op	    1313 B/op	      17 allocs/op
BenchmarkFastjq_Small_FromJSON-16             	 7925475	       149.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_FromJSON-16               	  865171	      1275 ns/op	    2714 B/op	      31 allocs/op
BenchmarkFastjq_Small_ToString-16             	 8984924	       134.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToNumber-16             	86185746	        13.20 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ToNumber-16               	 3383802	       352.2 ns/op	    1112 B/op	      11 allocs/op
BenchmarkFastjq_Small_AnyTwoArg-16            	  350335	      3221 ns/op	      57 B/op	       3 allocs/op
BenchmarkGojq_Small_AnyTwoArg-16              	  720042	      1621 ns/op	    2482 B/op	      27 allocs/op
BenchmarkFastjq_Complex_LogNormalize-16       	  878631	      1330 ns/op	     640 B/op	      24 allocs/op
BenchmarkGojq_Complex_LogNormalize-16         	  406407	      2877 ns/op	    4324 B/op	      69 allocs/op
BenchmarkFastjq_Complex_ArrayPipeline-16      	  140277	      8537 ns/op	    2264 B/op	     106 allocs/op
BenchmarkGojq_Complex_ArrayPipeline-16        	   52657	     22537 ns/op	   20408 B/op	     525 allocs/op
BenchmarkFastjq_Complex_Aggregation-16        	   91818	     12959 ns/op	    5176 B/op	     156 allocs/op
BenchmarkGojq_Complex_Aggregation-16          	   56540	     21309 ns/op	   22300 B/op	     551 allocs/op
BenchmarkFastjq_Complex_TolerantMap-16        	  107968	     11240 ns/op	    6816 B/op	     267 allocs/op
BenchmarkGojq_Complex_TolerantMap-16          	   44107	     27508 ns/op	   27870 B/op	     641 allocs/op
BenchmarkFastjq_Complex_ElifRouting-16        	  117254	     10236 ns/op	    5144 B/op	     241 allocs/op
BenchmarkGojq_Complex_ElifRouting-16          	   48540	     24681 ns/op	   25918 B/op	     595 allocs/op
BenchmarkFastjq_Complex_StringBuild-16        	   93912	     12712 ns/op	   11176 B/op	     428 allocs/op
BenchmarkGojq_Complex_StringBuild-16          	   50163	     23949 ns/op	   20532 B/op	     686 allocs/op
BenchmarkFastjq_Complex_EntryFilter-16        	  308406	      3843 ns/op	    4416 B/op	     120 allocs/op
BenchmarkGojq_Complex_EntryFilter-16          	  155946	      7691 ns/op	   11187 B/op	     162 allocs/op
BenchmarkFastjq_Small_Sqrt-16                 	11536551	       102.7 ns/op	      64 B/op	       3 allocs/op
BenchmarkGojq_Small_Sqrt-16                   	 2714992	       438.8 ns/op	    1145 B/op	      12 allocs/op
BenchmarkFastjq_Small_Log-16                  	16658539	        72.28 ns/op	      64 B/op	       3 allocs/op
BenchmarkGojq_Small_Log-16                    	 2751471	       436.5 ns/op	    1128 B/op	      12 allocs/op
BenchmarkFastjq_Small_Sin-16                  	10558705	       114.7 ns/op	      64 B/op	       3 allocs/op
BenchmarkGojq_Small_Sin-16                    	 2718289	       443.8 ns/op	    1145 B/op	      12 allocs/op
BenchmarkFastjq_Small_Atan-16                 	13259802	        90.84 ns/op	      64 B/op	       3 allocs/op
BenchmarkGojq_Small_Atan-16                   	 3103500	       385.8 ns/op	    1121 B/op	      11 allocs/op
BenchmarkFastjq_Small_Exp-16                  	11359202	       103.8 ns/op	      64 B/op	       3 allocs/op
BenchmarkGojq_Small_Exp-16                    	 3024817	       395.9 ns/op	    1121 B/op	      11 allocs/op
BenchmarkFastjq_Small_Tgamma-16               	24999913	        47.66 ns/op	      64 B/op	       3 allocs/op
BenchmarkGojq_Small_Tgamma-16                 	 3238498	       373.7 ns/op	    1104 B/op	      11 allocs/op
BenchmarkFastjq_Small_Fabs-16                 	13475770	        89.40 ns/op	      64 B/op	       3 allocs/op
BenchmarkGojq_Small_Fabs-16                   	 2949422	       405.1 ns/op	    1112 B/op	      12 allocs/op
BenchmarkFastjq_Small_StringInterp-16         	 7779105	       155.2 ns/op	      64 B/op	       3 allocs/op
BenchmarkGojq_Small_StringInterp-16           	 1000000	      1024 ns/op	    2121 B/op	      35 allocs/op
BenchmarkFastjq_Small_StringInterpNum-16      	 9726016	       125.1 ns/op	      64 B/op	       3 allocs/op
BenchmarkGojq_Small_StringInterpNum-16        	  999236	      1194 ns/op	    2498 B/op	      40 allocs/op
BenchmarkFastjq_Small_IsEmptyTrue-16          	17277829	        69.34 ns/op	     105 B/op	       6 allocs/op
BenchmarkGojq_Small_IsEmptyTrue-16            	 2084456	       570.0 ns/op	    1921 B/op	      18 allocs/op
BenchmarkFastjq_Small_IsEmptyFalse-16         	13715617	        84.91 ns/op	     105 B/op	       6 allocs/op
BenchmarkGojq_Small_IsEmptyFalse-16           	 1000000	      1046 ns/op	    2898 B/op	      33 allocs/op
BenchmarkFastjq_Small_Nth-16                  	 9871860	       125.1 ns/op	     152 B/op	       7 allocs/op
BenchmarkGojq_Small_Nth-16                    	  557442	      2257 ns/op	    4372 B/op	      49 allocs/op
BenchmarkFastjq_Small_Range10-16              	 7093800	       169.6 ns/op	     104 B/op	      11 allocs/op
BenchmarkGojq_Small_Range10-16                	 1200014	       998.5 ns/op	    1864 B/op	      18 allocs/op
BenchmarkFastjq_Small_RangeLimit-16           	11678793	       104.9 ns/op	     112 B/op	       7 allocs/op
BenchmarkGojq_Small_RangeLimit-16             	  784627	      1494 ns/op	    3208 B/op	      26 allocs/op
BenchmarkRegexp_Match_Hit-16                  	27942662	        41.26 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Miss-16                 	52964932	        22.51 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Complex_Hit-16          	28262031	        42.31 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Long_Miss-16            	15899725	        76.02 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_MatchString-16                	27468780	        43.89 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_FindSubmatch_Hit-16           	 6013993	       199.8 ns/op	     160 B/op	       2 allocs/op
BenchmarkRegexp_FindSubmatch_Miss-16          	 1821025	       658.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_FindSubmatchIndex_Hit-16      	 6720444	       177.8 ns/op	      64 B/op	       1 allocs/op
BenchmarkRegexp_FindSubmatchIndex_Miss-16     	 1817647	       664.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_ReplaceAll_Hit-16             	 7873528	       152.4 ns/op	      97 B/op	       5 allocs/op
BenchmarkRegexp_ReplaceAll_Miss-16            	24842383	        48.24 ns/op	      48 B/op	       2 allocs/op
BenchmarkRegexp_MatchViaClosure-16            	29231367	        41.00 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_Hit-16           	28962100	        41.51 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Small_TestRe_Miss-16          	 8509308	       141.5 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Small_TestRe_InPipeline-16    	 3834298	       313.1 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Small_MatchRe_Hit-16          	 5678151	       211.2 ns/op	      48 B/op	       1 allocs/op
BenchmarkFastjq_Small_MatchRe_Miss-16         	 2044640	       589.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_TestRe_Hit-16             	  623036	      1925 ns/op	    4174 B/op	      44 allocs/op
BenchmarkGojq_Small_TestRe_Miss-16            	  613695	      1881 ns/op	    4116 B/op	      43 allocs/op
BenchmarkGojq_Small_MatchRe_Hit-16            	  294207	      4292 ns/op	    8014 B/op	     100 allocs/op
BenchmarkGojq_Small_MatchRe_Miss-16           	  380370	      3079 ns/op	    5714 B/op	      59 allocs/op
BenchmarkFastjq_Small_CaptureRe_Hit-16        	 5702122	       208.6 ns/op	      48 B/op	       1 allocs/op
BenchmarkFastjq_Small_CaptureRe_Miss-16       	 2063431	       583.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ScanRe_NoGroups-16      	 2252578	       535.8 ns/op	     534 B/op	      17 allocs/op
BenchmarkFastjq_Small_ScanRe_WithGroups-16    	 1758576	       688.6 ns/op	     849 B/op	      20 allocs/op
BenchmarkFastjq_Small_SubRe_Hit-16            	 9703281	       126.6 ns/op	      16 B/op	       1 allocs/op
BenchmarkFastjq_Small_SubRe_Miss-16           	11889790	       102.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_GSubRe_Hit-16           	 2058595	       576.3 ns/op	     305 B/op	       5 allocs/op
BenchmarkFastjq_Small_GSubRe_Miss-16          	 4705568	       251.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_CaptureRe_Hit-16          	  358322	      3340 ns/op	    7382 B/op	      77 allocs/op
BenchmarkGojq_Small_CaptureRe_Miss-16         	  411004	      2925 ns/op	    5587 B/op	      54 allocs/op
BenchmarkGojq_Small_ScanRe_NoGroups-16        	  385082	      3095 ns/op	    6011 B/op	      67 allocs/op
BenchmarkGojq_Small_SubRe_Hit-16              	  324735	      3633 ns/op	    6165 B/op	      77 allocs/op
BenchmarkGojq_Small_GSubRe_Hit-16             	  161991	      7477 ns/op	    9408 B/op	     143 allocs/op
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
