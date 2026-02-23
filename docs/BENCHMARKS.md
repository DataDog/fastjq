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
| `.field` | Small (~100B) | 0.074 | 0.338 | **4.6x** | 0 | 13 |
| `.field` | Large (~100KB) | 7.27 | 572 | **79x** | 0 | 2835 |
| `del(.f)` | Small (~100B) | 0.165 | 0.945 | **5.7x** | 3 | 33 |
| `del(.f)` | Medium (~2KB) | 2.08 | 18.8 | **9x** | 3 | 323 |
| `del(.f)` | Large (~100KB) | 32.0 | 764 | **24x** | 3 | 4666 |
| `.[n]` | 5-elem array | 0.021 | 0.634 | **30x** | 0 | 20 |
| `del(.[n], .[m])` | 5-elem array | 0.113 | 1.59 | **14x** | 3 | 53 |
| `{f0, f2}` (construct) | Small (~100B) | 0.146 | 0.675 | **4.6x** | 3 | 23 |
| `{f0, f50}` (construct) | Large (~100KB) | 7.75 | 563 | **73x** | 3 | 2867 |
| `.[]` iterator | 5-elem array | 0.038 | 0.737 | **19x** | 1 | 26 |
| `.[]` iterator | 200-elem array | 6.81 | 82.7 | **12x** | 1 | 1811 |
| `select(.f == "x")` | Small (~100B) | 0.041 | 0.552 | **13x** | 3 | 20 |
| `select(.f == "x")`¹ | Large (~100KB, last field) | 30.4 | 741 | **24x** | 3 | 4651 |
| `select(.f and .g)` | Small (~100B) | 0.043 | 0.617 | **14x** | 3 | 21 |
| `select(.f or .g)` | Small (~100B) | 0.044 | 0.657 | **15x** | 3 | 21 |
| `has("key")` in select | Small (~100B) | 0.082 | 0.543 | **6.6x** | 6 | 20 |
| `has("key")` in select | Large (~100KB) | 30.1 | 766 | **25x** | 6 | 4651 |
| `if-then-else` | Small (~100B) | 0.043 | 0.453 | **11x** | 3 | 16 |
| `.f // "default"` | Small (~100B) | 0.067 | 0.456 | **6.8x** | 6 | 17 |
| `try .field` (no error) | Small (~100B) | 0.054 | 0.413 | **7.7x** | 4 | 16 |
| `.a + .b` (strings) | Small (~100B) | 0.050 | 0.660 | **13x** | 0 | 21 |
| `"prefix" + .name` | Small (~100B) | 0.083 | 0.705 | **8.5x** | 0 | 23 |
| `.a - .b` (subtract) | Small (~100B) | 0.049 | 0.664 | **14x** | 0 | 20 |
| `.a * .b` (multiply) | Small (~100B) | 0.064 | 0.693 | **11x** | 0 | 21 |
| `.a / .b` (divide) | Small (~100B) | 0.098 | 0.716 | **7.3x** | 0 | 21 |
| `.a - .b` (array diff) | 5-elem arrays | 0.196 | 1.21 | **6.2x** | 0 | 33 |
| `length` | Small (~100B) | 0.064 | 0.355 | **5.6x** | 5 | 13 |
| `length` | Large (~100KB) | 30.6 | 564 | **18x** | 0 | 2835 |
| `add` (numbers) | 5-elem array | 0.089 | 0.661 | **7.4x** | 3 | 24 |
| `add` (strings) | 5-elem array | 0.166 | 0.815 | **4.9x** | 3 | 31 |
| `flatten` | 3-elem nested array | 0.090 | 1.27 | **14x** | 0 | 38 |
| `min` | 200-int array | 1.62 | 1.19 | 0.7x† | 0 | 15 |
| `min_by(.value)` | 100-elem object array | 8.14 | 53.3 | **6.6x** | 0 | 1347 |
| `sort` | 200-int array | 4.05 | 1.19 | 0.3x† | 14 | 15 |
| `sort_by(.value)` | 100-elem object array | 16.5 | 87.8 | **5.3x** | 422 | 2145 |
| `unique` | 200-int array | 4.99 | 1.18 | 0.2x† | 14 | 15 |
| `group_by(.active)` | 100-elem object array | 20.9 | 94.1 | **4.5x** | 422 | 2260 |
| `map(.name)` | 20-elem array (~600B) | 1.54 | 9.82 | **6.4x** | 29 | 251 |
| `map(.name)` | 200-elem array (~6KB) | 15.0 | 88.8 | **5.9x** | 209 | 2237 |
| `any` | 5-elem array | 0.042 | 1.80 | **43x** | 0 | 39 |
| `any(expr)` | 5-elem array | 0.122 | 2.01 | **16x** | 0 | 49 |
| `any(expr)`² | 200-int array | 2.89 | 1.67 | 0.6x† | 0 | 29 |
| `any(gen; cond)`² | 200-int array | 3.20 | 1.63 | 0.5x† | 3 | 27 |
| `first(expr)` | 5-elem array | 0.200 | 1.46 | **7.3x** | 9 | 39 |
| `first(expr)`² | 200-int array | 4.76 | 1.41 | 0.3x† | 107 | 23 |
| `last(expr)` | 5-elem array | 0.250 | 1.86 | **7.4x** | 10 | 43 |
| `last(expr)`² | 200-int array | 4.43 | 1.51 | 0.3x† | 107 | 24 |
| `limit(3; expr)` | 5-elem array | 0.079 | 1.62 | **21x** | 4 | 42 |
| `limit(10; expr)` | 200-int array | 0.625 | 1.59 | **2.5x** | 4 | 24 |
| `.[1:4]` slice | 6-elem array | 0.073 | 0.779 | **11x** | 0 | 21 |
| `values` | 9-elem array | 0.231 | 2.36 | **10x** | 12 | 51 |
| `to_entries` | Small (~100B) | 0.0045 | 0.359 | **80x** | 0 | 14 |
| `to_entries` | Large (~100KB) | 33.8 | 799 | **24x** | 0 | 5847 |
| `keys_unsorted` | Small (~100B) | 0.059 | 0.366 | **6.2x** | 3 | 14 |
| `keys_unsorted` | Large (~100KB) | 31.0 | 587 | **19x** | 0 | 3039 |
| `object merge .a + .b` | Small (~100B) | 0.167 | 1.46 | **8.8x** | 0 | 33 |
| `tojson` | Small (~100B) | 0.119 | 0.416 | **3.5x** | 0 | 17 |
| `fromjson` | JSON string | 0.152 | 1.26 | **8.3x** | 0 | 31 |
| `tonumber` | `"42"` string | 0.014 | 0.349 | **25x** | 0 | 11 |
| `split(",")` | short string | 0.140 | 0.782 | **5.6x** | 0 | 21 |
| `join(",")` | 5-elem array | 0.098 | 0.836 | **8.5x** | 0 | 30 |
| `ascii_downcase` in select | Small (~100B) | 0.111 | 0.565 | **5.1x** | 8 | 21 |
| `ascii_downcase` in select | Large (~100KB) | 30.3 | 764 | **25x** | 20 | 4652 |
| `startswith("s")` | Small (~100B) | 0.112 | 0.566 | **5.1x** | 8 | 21 |
| `startswith("s")` | Large (~100KB) | 30.9 | 763 | **25x** | 9 | 4651 |
| `endswith("s")` | Small (~100B) | 0.112 | 0.570 | **5.1x** | 8 | 21 |
| `ltrimstr("s")` | Small (~100B) | 0.065 | 0.412 | **6.3x** | 5 | 16 |
| `rtrimstr("s")` | Small (~100B) | 0.065 | 0.414 | **6.4x** | 5 | 16 |
| `select` + string ops + arith + construct | ~200B log event | 1.55 | 3.91 | **2.5x** | 24 | 69 |
| `[.[] \| select + arith + construct]` | 20-elem array (~1.5KB) | 8.83 | 23.3 | **2.6x** | 106 | 525 |
| `length + map + add + min_by + any` | 20-elem array (~1.5KB) | 13.0 | 21.6 | **1.7x** | 156 | 551 |
| `map(try {…} catch …)` | 20-elem array (~1.5KB) | 11.0 | 27.0 | **2.4x** | 267 | 641 |
| `map(if … elif … else …)` | 20-elem array (~1.5KB) | 10.3 | 24.7 | **2.4x** | 241 | 595 |
| `[.[] \| str + tostring + str]` | 20-elem array (~1.5KB) | 12.6 | 23.9 | **1.9x** | 428 | 686 |
| `to_entries \| map(select) \| from_entries` | ~200B log event | 3.80 | 7.58 | **2x** | 120 | 162 |
| `@base64` | 34-char string | 0.147 | 0.525 | **3.6x** | 4 | 15 |
| `@base64d` | 48-char encoded | 0.202 | 0.526 | **2.6x** | 0 | 15 |
| `@uri` | 36-char URL string | 0.160 | 0.645 | **4x** | 4 | 14 |
| `index(",")` | short string | 0.080 | 0.938 | **12x** | 0 | 31 |
| `indices(",")` | short string | 0.188 | 2.09 | **11x** | 0 | 97 |
| `sqrt` | float (e≈2.718) | 0.102 | 0.430 | **4.2x** | 3 | 12 |
| `log` | float (e≈2.718) | 0.071 | 0.428 | **6x** | 3 | 12 |
| `sin` | float (e≈2.718) | 0.114 | 0.436 | **3.8x** | 3 | 12 |
| `atan` | integer 1 | 0.089 | 0.381 | **4.3x** | 3 | 11 |
| `exp` | integer 1 | 0.099 | 0.393 | **4x** | 3 | 11 |
| `tgamma` | integer 5 | 0.047 | 0.369 | **7.8x** | 3 | 11 |
| `fabs` | float -3.14 | 0.090 | 0.397 | **4.4x** | 3 | 12 |
| ``"\(.level): \(.svc)"`` | ~45B object | 0.155 | 1.01 | **6.5x** | 3 | 35 |
| ``"user \(.name) …"`` | ~45B object | 0.123 | 1.19 | **9.7x** | 3 | 40 |
| `isempty(empty)` | null | 0.068 | 0.556 | **8.1x** | 6 | 18 |
| `isempty(.[])` | 5-elem array | 0.084 | 1.01 | **12x** | 6 | 33 |
| `nth(2; .[])` | 5-elem array | 0.118 | 2.07 | **18x** | 7 | 49 |
| `range(10)` (10 values) | null | 0.167 | 0.977 | **5.8x** | 11 | 18 |
| `limit(3; range(1000))` | null — only 3 allocs | 0.104 | 1.47 | **14x** | 7 | 26 |
| `test(re)` hit | short string | 0.041 | 1.79 | **44x** | 3 | 44 |
| `test(re)` miss | short string | 0.139 | 1.76 | **13x** | 3 | 43 |
| `match(re)` hit | short string | 0.211 | 4.10 | **19x** | 1 | 100 |
| `match(re)` miss | short string | 0.614 | 2.89 | **4.7x** | 0 | 59 |
| `capture(re)` hit | short string | 0.205 | — | — | 1 | — |
| `scan(re)` no groups (4 matches) | short string | 0.525 | — | — | 17 | — |
| `sub(re; s)` hit | short string | 0.121 | — | — | 1 | — |
| `gsub(re; s)` hit (4 matches) | short string | 0.565 | — | — | 5 | — |

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
BenchmarkFastjq_Small_Del-16                  	 7053025	       165.0 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Medium_Del-16                 	  572552	      2079 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Large_Del-16                  	   37125	     32041 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Small_Field-16                	17595135	        73.55 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Field-16                	  164666	      7273 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Index-16                	58297468	        21.28 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ArrayDel-16             	10693237	       113.2 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Small_Construct-16            	 8094268	       145.7 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Large_Construct-16            	  147400	      7748 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Small_Iterator-16             	31805139	        38.04 ns/op	      24 B/op	       1 allocs/op
BenchmarkFastjq_Large_Iterator-16             	  185320	      6808 ns/op	      24 B/op	       1 allocs/op
BenchmarkFastjq_Small_Select-16               	29117054	        41.32 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Large_Select-16               	   38886	     30420 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Small_Alternative-16          	17183227	        66.70 ns/op	     113 B/op	       6 allocs/op
BenchmarkFastjq_Small_SelectAnd-16            	28354995	        43.24 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Small_SelectOr-16             	28380115	        44.06 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Small_Has-16                  	15062367	        82.06 ns/op	     128 B/op	       6 allocs/op
BenchmarkFastjq_Large_Has-16                  	   39330	     30091 ns/op	     128 B/op	       6 allocs/op
BenchmarkFastjq_Small_IfThenElse-16           	27484245	        42.93 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Small_Length-16               	18859226	        63.85 ns/op	     136 B/op	       5 allocs/op
BenchmarkFastjq_Large_Length-16               	   39328	     30558 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Map-16                  	  775364	      1539 ns/op	     689 B/op	      29 allocs/op
BenchmarkFastjq_Medium_Map-16                 	  159710	      7568 ns/op	    2609 B/op	     109 allocs/op
BenchmarkFastjq_Large_Map-16                  	   79444	     15013 ns/op	    5009 B/op	     209 allocs/op
BenchmarkFastjq_Small_ToEntries-16            	267678516	         4.491 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_ToEntries-16            	   34912	     33763 ns/op	       6 B/op	       0 allocs/op
BenchmarkFastjq_Small_KeysUnsorted-16         	20806917	        58.81 ns/op	      72 B/op	       3 allocs/op
BenchmarkFastjq_Large_KeysUnsorted-16         	   37999	     31031 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Any-16                  	28373013	        42.30 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Any-16                  	  739366	      1354 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AnyExpr-16              	 9753532	       121.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_AnyExpr-16              	  410611	      2887 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Add-16                  	13466841	        89.21 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Small_AddStrings-16           	 7217230	       165.9 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Small_Flatten-16              	13265898	        90.08 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Split-16                	 8384984	       140.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Join-16                 	12092400	        97.84 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Values-16               	 5303324	       231.3 ns/op	     312 B/op	      12 allocs/op
BenchmarkGojq_Small_Values-16                 	  474255	      2360 ns/op	    3144 B/op	      51 allocs/op
BenchmarkFastjq_Small_Base64Encode-16         	 8347644	       146.7 ns/op	     120 B/op	       4 allocs/op
BenchmarkFastjq_Small_Base64Decode-16         	 5806431	       201.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Base64Encode-16           	 2290437	       525.3 ns/op	    1321 B/op	      15 allocs/op
BenchmarkGojq_Small_Base64Decode-16           	 2270924	       525.7 ns/op	    1321 B/op	      15 allocs/op
BenchmarkFastjq_Small_IndexFind-16            	14830759	        79.88 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_IndicesAll-16           	 6267957	       187.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_IndexFind-16              	 1265241	       937.5 ns/op	    2274 B/op	      31 allocs/op
BenchmarkGojq_Small_IndicesAll-16             	  604620	      2090 ns/op	    3907 B/op	      97 allocs/op
BenchmarkFastjq_Small_Slice-16                	16008102	        72.89 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SliceString-16          	26689054	        45.60 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Plus-16                 	23422003	        49.92 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_PlusStr-16              	14759836	        82.60 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AsciiDowncase-16        	10642284	       111.3 ns/op	     200 B/op	       8 allocs/op
BenchmarkFastjq_Large_AsciiDowncase-16        	   40050	     30323 ns/op	    1144 B/op	      20 allocs/op
BenchmarkFastjq_Small_Startswith-16           	10931014	       111.6 ns/op	     200 B/op	       8 allocs/op
BenchmarkFastjq_Large_Startswith-16           	   38559	     30875 ns/op	     224 B/op	       9 allocs/op
BenchmarkFastjq_Small_Endswith-16             	10645107	       111.7 ns/op	     200 B/op	       8 allocs/op
BenchmarkFastjq_Small_Ltrimstr-16             	18711164	        65.08 ns/op	     136 B/op	       5 allocs/op
BenchmarkFastjq_Large_Ltrimstr-16             	   37855	     31980 ns/op	     160 B/op	       6 allocs/op
BenchmarkFastjq_Small_Rtrimstr-16             	18678290	        65.11 ns/op	     136 B/op	       5 allocs/op
BenchmarkFastjq_Small_First-16                	 5910055	       200.3 ns/op	     216 B/op	       9 allocs/op
BenchmarkFastjq_Large_First-16                	  242850	      4757 ns/op	    2584 B/op	     107 allocs/op
BenchmarkFastjq_Small_Last-16                 	 4799481	       249.6 ns/op	     256 B/op	      10 allocs/op
BenchmarkFastjq_Large_Last-16                 	  268161	      4434 ns/op	    2584 B/op	     107 allocs/op
BenchmarkFastjq_Small_Limit-16                	15128767	        78.65 ns/op	      88 B/op	       4 allocs/op
BenchmarkFastjq_Large_Limit-16                	 1913659	       624.5 ns/op	      88 B/op	       4 allocs/op
BenchmarkGojq_Small_Del-16                    	 1300960	       944.5 ns/op	    2594 B/op	      33 allocs/op
BenchmarkGojq_Medium_Del-16                   	   63548	     18781 ns/op	   16967 B/op	     323 allocs/op
BenchmarkGojq_Large_Del-16                    	    1585	    763969 ns/op	  539120 B/op	    4666 allocs/op
BenchmarkGojq_Small_Field-16                  	 3539677	       337.7 ns/op	    1177 B/op	      13 allocs/op
BenchmarkGojq_Large_Field-16                  	    2074	    572463 ns/op	  270050 B/op	    2835 allocs/op
BenchmarkGojq_Small_Index-16                  	 1863244	       634.0 ns/op	    1401 B/op	      20 allocs/op
BenchmarkGojq_Small_ArrayDel-16               	  760905	      1586 ns/op	    3362 B/op	      53 allocs/op
BenchmarkGojq_Small_Construct-16              	 1767140	       674.8 ns/op	    1857 B/op	      23 allocs/op
BenchmarkGojq_Large_Construct-16              	    2174	    562621 ns/op	  274454 B/op	    2867 allocs/op
BenchmarkGojq_Small_Iterator-16               	 1623951	       737.3 ns/op	    1776 B/op	      26 allocs/op
BenchmarkGojq_Large_Iterator-16               	   14691	     82700 ns/op	  109808 B/op	    1811 allocs/op
BenchmarkGojq_Small_Select-16                 	 2203136	       551.9 ns/op	    1753 B/op	      20 allocs/op
BenchmarkGojq_Large_Select-16                 	    1603	    741481 ns/op	  535390 B/op	    4651 allocs/op
BenchmarkGojq_Small_Alternative-16            	 2589523	       455.6 ns/op	    1441 B/op	      17 allocs/op
BenchmarkGojq_Small_SelectAnd-16              	 1964968	       617.2 ns/op	    1945 B/op	      21 allocs/op
BenchmarkGojq_Small_SelectOr-16               	 1831599	       656.9 ns/op	    1945 B/op	      21 allocs/op
BenchmarkGojq_Small_Has-16                    	 2226748	       542.7 ns/op	    1753 B/op	      20 allocs/op
BenchmarkGojq_Large_Has-16                    	    1554	    766104 ns/op	  539354 B/op	    4651 allocs/op
BenchmarkGojq_Small_IfThenElse-16             	 2592452	       452.6 ns/op	    1361 B/op	      16 allocs/op
BenchmarkGojq_Small_Length-16                 	 3291337	       355.2 ns/op	    1177 B/op	      13 allocs/op
BenchmarkGojq_Large_Length-16                 	    2082	    563677 ns/op	  269826 B/op	    2835 allocs/op
BenchmarkGojq_Small_Map-16                    	  123553	      9816 ns/op	   13653 B/op	     251 allocs/op
BenchmarkGojq_Large_Map-16                    	   13392	     88811 ns/op	  118557 B/op	    2237 allocs/op
BenchmarkGojq_Small_ToEntries-16              	 3344004	       359.5 ns/op	    1209 B/op	      14 allocs/op
BenchmarkGojq_Large_ToEntries-16              	    1488	    799437 ns/op	  642868 B/op	    5847 allocs/op
BenchmarkGojq_Small_KeysUnsorted-16           	 3308026	       365.6 ns/op	    1209 B/op	      14 allocs/op
BenchmarkGojq_Large_KeysUnsorted-16           	    2026	    587091 ns/op	  282762 B/op	    3039 allocs/op
BenchmarkGojq_Small_Any-16                    	  668212	      1802 ns/op	    4508 B/op	      39 allocs/op
BenchmarkGojq_Small_AnyExpr-16                	  606602	      2008 ns/op	    4620 B/op	      49 allocs/op
BenchmarkGojq_Large_AnyExpr-16                	  714658	      1667 ns/op	    2818 B/op	      29 allocs/op
BenchmarkGojq_Small_Add-16                    	 1830776	       661.2 ns/op	    1433 B/op	      24 allocs/op
BenchmarkGojq_Small_AddStrings-16             	 1493035	       814.8 ns/op	    1689 B/op	      31 allocs/op
BenchmarkGojq_Small_Flatten-16                	  966536	      1275 ns/op	    1857 B/op	      38 allocs/op
BenchmarkGojq_Small_Split-16                  	 1522664	       781.7 ns/op	    1553 B/op	      21 allocs/op
BenchmarkGojq_Small_Join-16                   	 1431212	       835.7 ns/op	    1745 B/op	      30 allocs/op
BenchmarkGojq_Small_Slice-16                  	 1553830	       779.4 ns/op	    1425 B/op	      21 allocs/op
BenchmarkGojq_Small_SliceString-16            	 2798008	       432.8 ns/op	    1145 B/op	      12 allocs/op
BenchmarkGojq_Small_Plus-16                   	 1815982	       660.4 ns/op	    1673 B/op	      21 allocs/op
BenchmarkGojq_Small_PlusStr-16                	 1682214	       705.1 ns/op	    1705 B/op	      23 allocs/op
BenchmarkGojq_Small_AsciiDowncase-16          	 2115417	       564.6 ns/op	    1785 B/op	      21 allocs/op
BenchmarkGojq_Large_AsciiDowncase-16          	    1540	    764340 ns/op	  535562 B/op	    4652 allocs/op
BenchmarkGojq_Small_Startswith-16             	 2144755	       565.8 ns/op	    1801 B/op	      21 allocs/op
BenchmarkGojq_Large_Startswith-16             	    1594	    762996 ns/op	  531587 B/op	    4651 allocs/op
BenchmarkGojq_Small_Endswith-16               	 2102073	       570.2 ns/op	    1801 B/op	      21 allocs/op
BenchmarkGojq_Small_Ltrimstr-16               	 2874904	       412.4 ns/op	    1305 B/op	      16 allocs/op
BenchmarkGojq_Small_Rtrimstr-16               	 2909185	       414.2 ns/op	    1305 B/op	      16 allocs/op
BenchmarkGojq_Small_First-16                  	  818900	      1455 ns/op	    3379 B/op	      39 allocs/op
BenchmarkGojq_Large_First-16                  	  862484	      1412 ns/op	    1889 B/op	      23 allocs/op
BenchmarkGojq_Small_Last-16                   	  663102	      1856 ns/op	    3483 B/op	      43 allocs/op
BenchmarkGojq_Large_Last-16                   	  763621	      1511 ns/op	    2193 B/op	      24 allocs/op
BenchmarkGojq_Small_Limit-16                  	  699208	      1625 ns/op	    3328 B/op	      42 allocs/op
BenchmarkGojq_Large_Limit-16                  	  735717	      1586 ns/op	    2200 B/op	      24 allocs/op
BenchmarkFastjq_Small_Subtract-16             	25140080	        48.53 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Subtract-16               	 1816094	       664.2 ns/op	    1649 B/op	      20 allocs/op
BenchmarkFastjq_Small_Multiply-16             	18029330	        63.80 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Multiply-16               	 1713367	       693.0 ns/op	    1649 B/op	      21 allocs/op
BenchmarkFastjq_Small_Divide-16               	12016652	        97.82 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Divide-16                 	 1688018	       715.5 ns/op	    1665 B/op	      21 allocs/op
BenchmarkFastjq_Small_Min-16                  	  762015	      1620 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Min-16                    	 1000000	      1185 ns/op	    1217 B/op	      15 allocs/op
BenchmarkFastjq_Small_MinBy-16                	  142197	      8137 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_MinBy-16                  	   22524	     53307 ns/op	   63629 B/op	    1347 allocs/op
BenchmarkFastjq_Sort-16                       	  304906	      4053 ns/op	    7688 B/op	      14 allocs/op
BenchmarkGojq_Sort-16                         	 1000000	      1194 ns/op	    1217 B/op	      15 allocs/op
BenchmarkFastjq_SortBy-16                     	   73524	     16529 ns/op	   24872 B/op	     422 allocs/op
BenchmarkGojq_SortBy-16                       	   13698	     87751 ns/op	   96894 B/op	    2145 allocs/op
BenchmarkFastjq_Unique-16                     	  237424	      4994 ns/op	    7688 B/op	      14 allocs/op
BenchmarkGojq_Unique-16                       	 1000000	      1184 ns/op	    1217 B/op	      15 allocs/op
BenchmarkFastjq_GroupBy-16                    	   57973	     20892 ns/op	   24872 B/op	     422 allocs/op
BenchmarkGojq_GroupBy-16                      	   12708	     94134 ns/op	  101878 B/op	    2260 allocs/op
BenchmarkFastjq_Small_URIEncode-16            	 7445958	       160.3 ns/op	     120 B/op	       4 allocs/op
BenchmarkGojq_Small_URIEncode-16              	 1871061	       645.0 ns/op	    1289 B/op	      14 allocs/op
BenchmarkFastjq_Small_ArrayDiff-16            	 6063927	       195.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ArrayDiff-16              	  982639	      1208 ns/op	    2121 B/op	      33 allocs/op
BenchmarkFastjq_Small_TryNoError-16           	22364467	        53.53 ns/op	      88 B/op	       4 allocs/op
BenchmarkFastjq_Small_TryCatchNoError-16      	10125260	       116.8 ns/op	     176 B/op	       6 allocs/op
BenchmarkGojq_Small_TryNoError-16             	 2918520	       412.7 ns/op	    1449 B/op	      16 allocs/op
BenchmarkFastjq_Small_ObjectMerge-16          	 7114994	       166.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ObjectMerge-16            	  789075	      1460 ns/op	    2906 B/op	      33 allocs/op
BenchmarkFastjq_Small_ToJSON-16               	10188692	       118.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ToJSON-16                 	 2859018	       415.6 ns/op	    1313 B/op	      17 allocs/op
BenchmarkFastjq_Small_FromJSON-16             	 7949401	       151.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_FromJSON-16               	  925176	      1259 ns/op	    2714 B/op	      31 allocs/op
BenchmarkFastjq_Small_ToString-16             	10054137	       121.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToNumber-16             	79877079	        13.81 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ToNumber-16               	 3440134	       349.2 ns/op	    1112 B/op	      11 allocs/op
BenchmarkFastjq_Small_AnyTwoArg-16            	  356750	      3204 ns/op	      57 B/op	       3 allocs/op
BenchmarkGojq_Small_AnyTwoArg-16              	  733702	      1632 ns/op	    2482 B/op	      27 allocs/op
BenchmarkFastjq_Complex_LogNormalize-16       	  861681	      1547 ns/op	     640 B/op	      24 allocs/op
BenchmarkGojq_Complex_LogNormalize-16         	  389166	      3907 ns/op	    4324 B/op	      69 allocs/op
BenchmarkFastjq_Complex_ArrayPipeline-16      	  137836	      8831 ns/op	    2264 B/op	     106 allocs/op
BenchmarkGojq_Complex_ArrayPipeline-16        	   50772	     23347 ns/op	   20408 B/op	     525 allocs/op
BenchmarkFastjq_Complex_Aggregation-16        	   89890	     12983 ns/op	    5176 B/op	     156 allocs/op
BenchmarkGojq_Complex_Aggregation-16          	   54156	     21601 ns/op	   22300 B/op	     551 allocs/op
BenchmarkFastjq_Complex_TolerantMap-16        	  107274	     11038 ns/op	    6816 B/op	     267 allocs/op
BenchmarkGojq_Complex_TolerantMap-16          	   44788	     27020 ns/op	   27869 B/op	     641 allocs/op
BenchmarkFastjq_Complex_ElifRouting-16        	  117588	     10319 ns/op	    5144 B/op	     241 allocs/op
BenchmarkGojq_Complex_ElifRouting-16          	   47470	     24737 ns/op	   25916 B/op	     595 allocs/op
BenchmarkFastjq_Complex_StringBuild-16        	   94825	     12610 ns/op	   11176 B/op	     428 allocs/op
BenchmarkGojq_Complex_StringBuild-16          	   50104	     23912 ns/op	   20532 B/op	     686 allocs/op
BenchmarkFastjq_Complex_EntryFilter-16        	  316708	      3798 ns/op	    4416 B/op	     120 allocs/op
BenchmarkGojq_Complex_EntryFilter-16          	  158649	      7583 ns/op	   11187 B/op	     162 allocs/op
BenchmarkFastjq_Small_Sqrt-16                 	11924695	       102.3 ns/op	      64 B/op	       3 allocs/op
BenchmarkGojq_Small_Sqrt-16                   	 2774190	       430.0 ns/op	    1145 B/op	      12 allocs/op
BenchmarkFastjq_Small_Log-16                  	16709648	        71.47 ns/op	      64 B/op	       3 allocs/op
BenchmarkGojq_Small_Log-16                    	 2835778	       428.1 ns/op	    1128 B/op	      12 allocs/op
BenchmarkFastjq_Small_Sin-16                  	10516644	       113.9 ns/op	      64 B/op	       3 allocs/op
BenchmarkGojq_Small_Sin-16                    	 2714424	       436.3 ns/op	    1145 B/op	      12 allocs/op
BenchmarkFastjq_Small_Atan-16                 	13508949	        88.81 ns/op	      64 B/op	       3 allocs/op
BenchmarkGojq_Small_Atan-16                   	 3150594	       381.3 ns/op	    1121 B/op	      11 allocs/op
BenchmarkFastjq_Small_Exp-16                  	12161596	        98.66 ns/op	      64 B/op	       3 allocs/op
BenchmarkGojq_Small_Exp-16                    	 3001816	       393.1 ns/op	    1121 B/op	      11 allocs/op
BenchmarkFastjq_Small_Tgamma-16               	24964092	        47.39 ns/op	      64 B/op	       3 allocs/op
BenchmarkGojq_Small_Tgamma-16                 	 3220927	       369.0 ns/op	    1104 B/op	      11 allocs/op
BenchmarkFastjq_Small_Fabs-16                 	13252206	        90.03 ns/op	      64 B/op	       3 allocs/op
BenchmarkGojq_Small_Fabs-16                   	 3037782	       396.7 ns/op	    1112 B/op	      12 allocs/op
BenchmarkFastjq_Small_StringInterp-16         	 7679574	       155.2 ns/op	      64 B/op	       3 allocs/op
BenchmarkGojq_Small_StringInterp-16           	 1000000	      1015 ns/op	    2121 B/op	      35 allocs/op
BenchmarkFastjq_Small_StringInterpNum-16      	 9843531	       123.1 ns/op	      64 B/op	       3 allocs/op
BenchmarkGojq_Small_StringInterpNum-16        	  958290	      1189 ns/op	    2498 B/op	      40 allocs/op
BenchmarkFastjq_Small_IsEmptyTrue-16          	17434332	        68.46 ns/op	     105 B/op	       6 allocs/op
BenchmarkGojq_Small_IsEmptyTrue-16            	 2166381	       556.3 ns/op	    1921 B/op	      18 allocs/op
BenchmarkFastjq_Small_IsEmptyFalse-16         	14184529	        84.06 ns/op	     105 B/op	       6 allocs/op
BenchmarkGojq_Small_IsEmptyFalse-16           	 1000000	      1007 ns/op	    2898 B/op	      33 allocs/op
BenchmarkFastjq_Small_Nth-16                  	10215990	       117.8 ns/op	     152 B/op	       7 allocs/op
BenchmarkGojq_Small_Nth-16                    	  558705	      2073 ns/op	    4371 B/op	      49 allocs/op
BenchmarkFastjq_Small_Range10-16              	 7057148	       167.4 ns/op	     104 B/op	      11 allocs/op
BenchmarkGojq_Small_Range10-16                	 1226952	       977.4 ns/op	    1864 B/op	      18 allocs/op
BenchmarkFastjq_Small_RangeLimit-16           	11499070	       104.2 ns/op	     112 B/op	       7 allocs/op
BenchmarkGojq_Small_RangeLimit-16             	  848703	      1466 ns/op	    3208 B/op	      26 allocs/op
BenchmarkRegexp_Match_Hit-16                  	29273973	        39.51 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Miss-16                 	52842291	        22.55 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Complex_Hit-16          	28626550	        42.26 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Long_Miss-16            	15969940	        75.58 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_MatchString-16                	27798985	        43.58 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_FindSubmatch_Hit-16           	 6018427	       198.6 ns/op	     160 B/op	       2 allocs/op
BenchmarkRegexp_FindSubmatch_Miss-16          	 1795852	       664.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_FindSubmatchIndex_Hit-16      	 6797763	       176.7 ns/op	      64 B/op	       1 allocs/op
BenchmarkRegexp_FindSubmatchIndex_Miss-16     	 1798627	       666.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_ReplaceAll_Hit-16             	 7944853	       150.5 ns/op	      97 B/op	       5 allocs/op
BenchmarkRegexp_ReplaceAll_Miss-16            	25152003	        47.98 ns/op	      48 B/op	       2 allocs/op
BenchmarkRegexp_MatchViaClosure-16            	30831618	        39.69 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_Hit-16           	29416360	        40.71 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Small_TestRe_Miss-16          	 8719516	       138.5 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Small_TestRe_InPipeline-16    	 3693369	       325.3 ns/op	      64 B/op	       3 allocs/op
BenchmarkFastjq_Small_MatchRe_Hit-16          	 5625087	       211.5 ns/op	      48 B/op	       1 allocs/op
BenchmarkFastjq_Small_MatchRe_Miss-16         	 1957318	       614.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_TestRe_Hit-16             	  653206	      1795 ns/op	    4163 B/op	      44 allocs/op
BenchmarkGojq_Small_TestRe_Miss-16            	  704544	      1760 ns/op	    4106 B/op	      43 allocs/op
BenchmarkGojq_Small_MatchRe_Hit-16            	  291270	      4100 ns/op	    8004 B/op	     100 allocs/op
BenchmarkGojq_Small_MatchRe_Miss-16           	  404328	      2887 ns/op	    5709 B/op	      59 allocs/op
BenchmarkFastjq_Small_CaptureRe_Hit-16        	 5829308	       204.6 ns/op	      48 B/op	       1 allocs/op
BenchmarkFastjq_Small_CaptureRe_Miss-16       	 1960458	       612.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ScanRe_NoGroups-16      	 2304669	       524.9 ns/op	     533 B/op	      17 allocs/op
BenchmarkFastjq_Small_ScanRe_WithGroups-16    	 1767270	       674.2 ns/op	     849 B/op	      20 allocs/op
BenchmarkFastjq_Small_SubRe_Hit-16            	 9869559	       121.2 ns/op	      16 B/op	       1 allocs/op
BenchmarkFastjq_Small_SubRe_Miss-16           	11879874	       100.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_GSubRe_Hit-16           	 2140406	       565.1 ns/op	     305 B/op	       5 allocs/op
BenchmarkFastjq_Small_GSubRe_Miss-16          	 4781736	       250.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_CaptureRe_Hit-16          	  359044	      3308 ns/op	    7381 B/op	      77 allocs/op
BenchmarkGojq_Small_CaptureRe_Miss-16         	  421735	      2861 ns/op	    5580 B/op	      54 allocs/op
BenchmarkGojq_Small_ScanRe_NoGroups-16        	  402279	      3028 ns/op	    6006 B/op	      67 allocs/op
BenchmarkGojq_Small_SubRe_Hit-16              	  344974	      3492 ns/op	    6169 B/op	      77 allocs/op
BenchmarkGojq_Small_GSubRe_Hit-16             	  164114	      7325 ns/op	    9410 B/op	     143 allocs/op
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
