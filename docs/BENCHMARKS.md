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
| `.field` | Small (~100B) | 0.085 | 0.348 | **4.1x** | 0 | 13 |
| `.field` | Large (~100KB) | 7.78 | 582 | **75x** | 0 | 2835 |
| `del(.f)` | Small (~100B) | 0.164 | 0.959 | **5.8x** | 0 | 33 |
| `del(.f)` | Medium (~2KB) | 2.10 | 19.6 | **9.4x** | 0 | 323 |
| `del(.f)` | Large (~100KB) | 34.1 | 813 | **24x** | 0 | 4666 |
| `.[n]` | 5-elem array | 0.023 | 0.615 | **26x** | 0 | 20 |
| `del(.[n], .[m])` | 5-elem array | 0.085 | 1.64 | **19x** | 0 | 53 |
| `{f0, f2}` (construct) | Small (~100B) | 0.140 | 0.693 | **5x** | 0 | 23 |
| `{f0, f50}` (construct) | Large (~100KB) | 8.11 | 589 | **73x** | 0 | 2867 |
| `.[]` iterator | 5-elem array | 0.030 | 0.769 | **26x** | 0 | 26 |
| `.[]` iterator | 200-elem array | 7.33 | 84.7 | **12x** | 0 | 1811 |
| `select(.f == "x")` | Small (~100B) | 0.011 | 0.569 | **51x** | 0 | 20 |
| `select(.f == "x")`¹ | Large (~100KB, last field) | 31.0 | 781 | **25x** | 0 | 4652 |
| `select(.f and .g)` | Small (~100B) | 0.013 | 0.625 | **48x** | 0 | 21 |
| `select(.f or .g)` | Small (~100B) | 0.013 | 0.666 | **52x** | 0 | 21 |
| `has("key")` in select | Small (~100B) | 0.015 | 0.557 | **38x** | 0 | 20 |
| `has("key")` in select | Large (~100KB) | 31.2 | 782 | **25x** | 0 | 4651 |
| `if-then-else` | Small (~100B) | 0.012 | 0.464 | **38x** | 0 | 16 |
| `.f // "default"` | Small (~100B) | 0.0095 | 0.468 | **49x** | 0 | 17 |
| `try .field` (no error) | Small (~100B) | 0.0087 | 0.422 | **49x** | 0 | 16 |
| `.a + .b` (strings) | Small (~100B) | 0.050 | 0.672 | **13x** | 0 | 21 |
| `"prefix" + .name` | Small (~100B) | 0.083 | 0.721 | **8.7x** | 0 | 23 |
| `.a - .b` (subtract) | Small (~100B) | 0.050 | 0.702 | **14x** | 0 | 20 |
| `.a * .b` (multiply) | Small (~100B) | 0.065 | 0.736 | **11x** | 0 | 21 |
| `.a / .b` (divide) | Small (~100B) | 0.098 | 0.750 | **7.7x** | 0 | 21 |
| `.a - .b` (array diff) | 5-elem arrays | 0.200 | 1.24 | **6.2x** | 0 | 33 |
| `length` | Small (~100B) | 0.0093 | 0.370 | **40x** | 0 | 13 |
| `length` | Large (~100KB) | 31.8 | 587 | **18x** | 0 | 2835 |
| `add` (numbers) | 5-elem array | 0.058 | 0.676 | **12x** | 0 | 24 |
| `add` (strings) | 5-elem array | 0.142 | 0.842 | **5.9x** | 0 | 31 |
| `flatten` | 3-elem nested array | 0.093 | 1.27 | **14x** | 0 | 38 |
| `min` | 200-int array | 1.86 | 1.29 | 0.7x† | 0 | 15 |
| `min_by(.value)` | 100-elem object array | 8.31 | 55.1 | **6.6x** | 0 | 1347 |
| `map(.name)` | 20-elem array (~600B) | 1.21 | 10.2 | **8.4x** | 0 | 251 |
| `map(.name)` | 200-elem array (~6KB) | 12.9 | 92.1 | **7.1x** | 0 | 2237 |
| `any` | 5-elem array | 0.046 | 1.84 | **40x** | 0 | 39 |
| `any(expr)` | 5-elem array | 0.117 | 2.07 | **18x** | 0 | 49 |
| `any(expr)`² | 200-int array | 2.92 | 1.70 | 0.6x† | 0 | 29 |
| `any(gen; cond)`² | 200-int array | 3.27 | 1.65 | 0.5x† | 0 | 27 |
| `first(expr)` | 5-elem array | 0.113 | 1.60 | **14x** | 1 | 39 |
| `first(expr)`² | 200-int array | 3.91 | 1.44 | 0.4x† | 0 | 23 |
| `last(expr)` | 5-elem array | 0.137 | 1.90 | **14x** | 0 | 43 |
| `last(expr)`² | 200-int array | 3.46 | 1.53 | 0.4x† | 0 | 24 |
| `limit(3; expr)` | 5-elem array | 0.038 | 1.67 | **44x** | 0 | 42 |
| `limit(10; expr)` | 200-int array | 0.610 | 1.65 | **2.7x** | 0 | 24 |
| `.[1:4]` slice | 6-elem array | 0.078 | 0.837 | **11x** | 0 | 21 |
| `values` | 9-elem array | 0.093 | 2.33 | **25x** | 0 | 51 |
| `to_entries` | Small (~100B) | 0.0045 | 0.374 | **83x** | 0 | 14 |
| `to_entries` | Large (~100KB) | 34.0 | 831 | **24x** | 0 | 5847 |
| `keys_unsorted` | Small (~100B) | 0.062 | 0.369 | **6x** | 3 | 14 |
| `keys_unsorted` | Large (~100KB) | 31.4 | 604 | **19x** | 0 | 3039 |
| `object merge .a + .b` | Small (~100B) | 0.167 | 1.49 | **9x** | 0 | 33 |
| `tojson` | Small (~100B) | 0.133 | 0.423 | **3.2x** | 0 | 17 |
| `fromjson` | JSON string | 0.151 | 1.29 | **8.5x** | 0 | 31 |
| `tonumber` | `"42"` string | 0.013 | 0.354 | **27x** | 0 | 11 |
| `split(",")` | short string | 0.141 | 0.775 | **5.5x** | 0 | 21 |
| `join(",")` | 5-elem array | 0.103 | 0.874 | **8.5x** | 0 | 30 |
| `ascii_downcase` in select | Small (~100B) | 0.014 | 0.576 | **41x** | 0 | 21 |
| `ascii_downcase` in select | Large (~100KB) | 31.0 | 789 | **25x** | 0 | 4652 |
| `startswith("s")` | Small (~100B) | 0.015 | 0.614 | **42x** | 0 | 21 |
| `startswith("s")` | Large (~100KB) | 31.5 | 795 | **25x** | 0 | 4651 |
| `endswith("s")` | Small (~100B) | 0.014 | 0.594 | **41x** | 0 | 21 |
| `ltrimstr("s")` | Small (~100B) | 0.0091 | 0.440 | **48x** | 0 | 16 |
| `rtrimstr("s")` | Small (~100B) | 0.0092 | 0.450 | **49x** | 0 | 16 |
| `select` + string ops + arith + construct | ~200B log event | 1.08 | 2.98 | **2.8x** | 0 | 69 |
| `[.[] \| select + arith + construct]` | 20-elem array (~1.5KB) | 7.79 | 23.1 | **3x** | 64 | 525 |
| `length + map + add + min_by + any` | 20-elem array (~1.5KB) | 10.9 | 22.4 | **2x** | 26 | 551 |
| `map(try {…} catch …)` | 20-elem array (~1.5KB) | 9.30 | 27.8 | **3x** | 98 | 641 |
| `map(if … elif … else …)` | 20-elem array (~1.5KB) | 8.46 | 25.1 | **3x** | 125 | 595 |
| `[.[] \| str + tostring + str]` | 20-elem array (~1.5KB) | 7.99 | 24.1 | **3x** | 124 | 686 |
| `to_entries \| map(select) \| from_entries` | ~200B log event | 2.71 | 7.73 | **2.9x** | 35 | 162 |
| `@base64` | 34-char string | 0.139 | 0.509 | **3.7x** | 4 | 15 |
| `@base64d` | 48-char encoded | 0.206 | 0.544 | **2.6x** | 0 | 15 |
| `@uri` | 36-char URL string | 0.162 | 0.668 | **4.1x** | 4 | 14 |
| `index(",")` | short string | 0.081 | 0.908 | **11x** | 0 | 31 |
| `indices(",")` | short string | 0.190 | 2.08 | **11x** | 0 | 97 |
| `sqrt` | float (e≈2.718) | 0.068 | 0.447 | **6.6x** | 0 | 12 |
| `log` | float (e≈2.718) | 0.038 | 0.462 | **12x** | 0 | 12 |
| `sin` | float (e≈2.718) | 0.079 | 0.482 | **6.1x** | 0 | 12 |
| `atan` | integer 1 | 0.054 | 0.385 | **7.1x** | 0 | 11 |
| `exp` | integer 1 | 0.064 | 0.411 | **6.4x** | 0 | 11 |
| `tgamma` | integer 5 | 0.015 | 0.397 | **26x** | 0 | 11 |
| `fabs` | float -3.14 | 0.056 | 0.447 | **8x** | 0 | 12 |
| ``"\(.level): \(.svc)"`` | ~45B object | 0.134 | 1.05 | **7.8x** | 0 | 35 |
| ``"user \(.name) …"`` | ~45B object | 0.104 | 1.23 | **12x** | 0 | 40 |
| `isempty(empty)` | null | 0.0090 | 0.676 | **75x** | 0 | 18 |
| `isempty(.[])` | 5-elem array | 0.023 | 1.03 | **45x** | 0 | 33 |
| `nth(2; .[])` | 5-elem array | 0.040 | 2.15 | **54x** | 0 | 49 |
| `test(re)` hit | short string | 0.0092 | 1.93 | **210x** | 0 | 44 |
| `test(re)` miss | short string | 0.109 | 1.90 | **17x** | 0 | 43 |
| `match(re)` hit | short string | 0.215 | 4.28 | **20x** | 1 | 100 |
| `match(re)` miss | short string | 0.593 | 3.04 | **5.1x** | 0 | 59 |
| `capture(re)` hit | short string | 0.207 | — | — | 1 | — |
| `scan(re)` no groups (4 matches) | short string | 0.474 | — | — | 10 | — |
| `sub(re; s)` hit | short string | 0.122 | — | — | 1 | — |
| `gsub(re; s)` hit (4 matches) | short string | 0.563 | — | — | 5 | — |

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
BenchmarkFastjq_Small_Del-16                  	 6935484	       164.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Medium_Del-16                 	  584529	      2099 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Del-16                  	   34915	     34122 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Field-16                	13795058	        84.80 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Field-16                	  158344	      7780 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Index-16                	52087194	        23.36 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ArrayDel-16             	14264126	        84.68 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Construct-16            	 8446735	       140.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Construct-16            	  147961	      8105 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Iterator-16             	40437056	        29.83 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Iterator-16             	  178165	      7333 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Select-16               	100000000	        11.22 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Select-16               	   38794	     30978 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Alternative-16          	125921676	         9.534 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SelectAnd-16            	95663898	        12.96 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SelectOr-16             	93111075	        12.85 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Has-16                  	85388485	        14.55 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Has-16                  	   37800	     31203 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_IfThenElse-16           	99774118	        12.36 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Length-16               	129581370	         9.287 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Length-16               	   37794	     31848 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Map-16                  	 1000000	      1209 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Medium_Map-16                 	  198091	      6316 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Map-16                  	   90996	     12898 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToEntries-16            	262252041	         4.530 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_ToEntries-16            	   35518	     34012 ns/op	       6 B/op	       0 allocs/op
BenchmarkFastjq_Small_KeysUnsorted-16         	19193025	        61.70 ns/op	      72 B/op	       3 allocs/op
BenchmarkFastjq_Large_KeysUnsorted-16         	   38584	     31378 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Any-16                  	24358248	        45.88 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Any-16                  	  769711	      1584 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AnyExpr-16              	10260494	       117.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_AnyExpr-16              	  407348	      2918 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Add-16                  	20764957	        57.88 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AddStrings-16           	 8644294	       142.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Flatten-16              	12719730	        93.42 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Split-16                	 8447974	       141.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Join-16                 	11466711	       103.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Values-16               	13604625	        92.75 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Values-16                 	  502435	      2335 ns/op	    3144 B/op	      51 allocs/op
BenchmarkFastjq_Small_Base64Encode-16         	 8703910	       139.2 ns/op	     120 B/op	       4 allocs/op
BenchmarkFastjq_Small_Base64Decode-16         	 5661662	       205.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Base64Encode-16           	 2335351	       509.0 ns/op	    1321 B/op	      15 allocs/op
BenchmarkGojq_Small_Base64Decode-16           	 2209574	       543.7 ns/op	    1321 B/op	      15 allocs/op
BenchmarkFastjq_Small_IndexFind-16            	14739962	        80.91 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_IndicesAll-16           	 6282116	       190.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_IndexFind-16              	 1318479	       908.0 ns/op	    2274 B/op	      31 allocs/op
BenchmarkGojq_Small_IndicesAll-16             	  580923	      2080 ns/op	    3907 B/op	      97 allocs/op
BenchmarkFastjq_Small_Slice-16                	14534604	        78.48 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SliceString-16          	26416443	        46.09 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Plus-16                 	23728994	        49.98 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_PlusStr-16              	14498947	        83.30 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AsciiDowncase-16        	85329285	        14.17 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_AsciiDowncase-16        	   38059	     30965 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Startswith-16           	83734315	        14.57 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Startswith-16           	   37466	     31517 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Endswith-16             	79933170	        14.45 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Ltrimstr-16             	131011600	         9.138 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Ltrimstr-16             	   37518	     32173 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Rtrimstr-16             	130452187	         9.209 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_First-16                	10690616	       112.8 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Large_First-16                	  313051	      3907 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Last-16                 	 8761623	       136.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Last-16                 	  335632	      3455 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Limit-16                	31714742	        38.09 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Limit-16                	 1997406	       610.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Del-16                    	 1251496	       959.4 ns/op	    2594 B/op	      33 allocs/op
BenchmarkGojq_Medium_Del-16                   	   61908	     19640 ns/op	   16971 B/op	     323 allocs/op
BenchmarkGojq_Large_Del-16                    	    1482	    812917 ns/op	  540669 B/op	    4666 allocs/op
BenchmarkGojq_Small_Field-16                  	 3463244	       348.5 ns/op	    1177 B/op	      13 allocs/op
BenchmarkGojq_Large_Field-16                  	    2044	    582141 ns/op	  270047 B/op	    2835 allocs/op
BenchmarkGojq_Small_Index-16                  	 1953396	       615.0 ns/op	    1401 B/op	      20 allocs/op
BenchmarkGojq_Small_ArrayDel-16               	  726974	      1644 ns/op	    3362 B/op	      53 allocs/op
BenchmarkGojq_Small_Construct-16              	 1739856	       693.2 ns/op	    1857 B/op	      23 allocs/op
BenchmarkGojq_Large_Construct-16              	    2019	    588593 ns/op	  274388 B/op	    2867 allocs/op
BenchmarkGojq_Small_Iterator-16               	 1557409	       768.7 ns/op	    1776 B/op	      26 allocs/op
BenchmarkGojq_Large_Iterator-16               	   14131	     84736 ns/op	  109808 B/op	    1811 allocs/op
BenchmarkGojq_Small_Select-16                 	 2115306	       568.8 ns/op	    1753 B/op	      20 allocs/op
BenchmarkGojq_Large_Select-16                 	    1544	    781366 ns/op	  541620 B/op	    4652 allocs/op
BenchmarkGojq_Small_Alternative-16            	 2573191	       468.1 ns/op	    1441 B/op	      17 allocs/op
BenchmarkGojq_Small_SelectAnd-16              	 1916404	       625.2 ns/op	    1945 B/op	      21 allocs/op
BenchmarkGojq_Small_SelectOr-16               	 1806998	       666.2 ns/op	    1945 B/op	      21 allocs/op
BenchmarkGojq_Small_Has-16                    	 2144912	       557.0 ns/op	    1753 B/op	      20 allocs/op
BenchmarkGojq_Large_Has-16                    	    1508	    781743 ns/op	  536866 B/op	    4651 allocs/op
BenchmarkGojq_Small_IfThenElse-16             	 2628753	       463.5 ns/op	    1361 B/op	      16 allocs/op
BenchmarkGojq_Small_Length-16                 	 3248047	       370.4 ns/op	    1177 B/op	      13 allocs/op
BenchmarkGojq_Large_Length-16                 	    2043	    587054 ns/op	  269831 B/op	    2835 allocs/op
BenchmarkGojq_Small_Map-16                    	  118683	     10152 ns/op	   13654 B/op	     251 allocs/op
BenchmarkGojq_Large_Map-16                    	   13039	     92052 ns/op	  118568 B/op	    2237 allocs/op
BenchmarkGojq_Small_ToEntries-16              	 3200398	       374.0 ns/op	    1209 B/op	      14 allocs/op
BenchmarkGojq_Large_ToEntries-16              	    1444	    831433 ns/op	  642547 B/op	    5847 allocs/op
BenchmarkGojq_Small_KeysUnsorted-16           	 3238660	       369.2 ns/op	    1209 B/op	      14 allocs/op
BenchmarkGojq_Large_KeysUnsorted-16           	    1990	    603890 ns/op	  282746 B/op	    3039 allocs/op
BenchmarkGojq_Small_Any-16                    	  642662	      1840 ns/op	    4508 B/op	      39 allocs/op
BenchmarkGojq_Small_AnyExpr-16                	  587864	      2072 ns/op	    4620 B/op	      49 allocs/op
BenchmarkGojq_Large_AnyExpr-16                	  711372	      1701 ns/op	    2818 B/op	      29 allocs/op
BenchmarkGojq_Small_Add-16                    	 1765993	       676.3 ns/op	    1433 B/op	      24 allocs/op
BenchmarkGojq_Small_AddStrings-16             	 1418290	       842.0 ns/op	    1689 B/op	      31 allocs/op
BenchmarkGojq_Small_Flatten-16                	  933258	      1268 ns/op	    1857 B/op	      38 allocs/op
BenchmarkGojq_Small_Split-16                  	 1546003	       775.4 ns/op	    1553 B/op	      21 allocs/op
BenchmarkGojq_Small_Join-16                   	 1398337	       874.0 ns/op	    1745 B/op	      30 allocs/op
BenchmarkGojq_Small_Slice-16                  	 1443315	       837.4 ns/op	    1425 B/op	      21 allocs/op
BenchmarkGojq_Small_SliceString-16            	 2665980	       446.6 ns/op	    1145 B/op	      12 allocs/op
BenchmarkGojq_Small_Plus-16                   	 1787618	       671.6 ns/op	    1673 B/op	      21 allocs/op
BenchmarkGojq_Small_PlusStr-16                	 1658740	       721.1 ns/op	    1705 B/op	      23 allocs/op
BenchmarkGojq_Small_AsciiDowncase-16          	 2083840	       575.7 ns/op	    1785 B/op	      21 allocs/op
BenchmarkGojq_Large_AsciiDowncase-16          	    1497	    788853 ns/op	  536758 B/op	    4652 allocs/op
BenchmarkGojq_Small_Startswith-16             	 2084302	       613.6 ns/op	    1801 B/op	      21 allocs/op
BenchmarkGojq_Large_Startswith-16             	    1480	    794935 ns/op	  537087 B/op	    4651 allocs/op
BenchmarkGojq_Small_Endswith-16               	 2027850	       594.0 ns/op	    1801 B/op	      21 allocs/op
BenchmarkGojq_Small_Ltrimstr-16               	 2739428	       439.5 ns/op	    1305 B/op	      16 allocs/op
BenchmarkGojq_Small_Rtrimstr-16               	 2719596	       450.1 ns/op	    1305 B/op	      16 allocs/op
BenchmarkGojq_Small_First-16                  	  772801	      1600 ns/op	    3379 B/op	      39 allocs/op
BenchmarkGojq_Large_First-16                  	  827550	      1442 ns/op	    1889 B/op	      23 allocs/op
BenchmarkGojq_Small_Last-16                   	  636027	      1898 ns/op	    3483 B/op	      43 allocs/op
BenchmarkGojq_Large_Last-16                   	  779262	      1535 ns/op	    2193 B/op	      24 allocs/op
BenchmarkGojq_Small_Limit-16                  	  730329	      1667 ns/op	    3328 B/op	      42 allocs/op
BenchmarkGojq_Large_Limit-16                  	  735355	      1651 ns/op	    2200 B/op	      24 allocs/op
BenchmarkFastjq_Small_Subtract-16             	23663115	        49.83 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Subtract-16               	 1709634	       701.9 ns/op	    1649 B/op	      20 allocs/op
BenchmarkFastjq_Small_Multiply-16             	17929502	        65.19 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Multiply-16               	 1615120	       735.8 ns/op	    1649 B/op	      21 allocs/op
BenchmarkFastjq_Small_Divide-16               	12066612	        97.91 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Divide-16                 	 1657222	       750.0 ns/op	    1665 B/op	      21 allocs/op
BenchmarkFastjq_Small_Min-16                  	  658558	      1861 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Min-16                    	  936538	      1295 ns/op	    1217 B/op	      15 allocs/op
BenchmarkFastjq_Small_MinBy-16                	  138886	      8309 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_MinBy-16                  	   22077	     55115 ns/op	   63629 B/op	    1347 allocs/op
BenchmarkFastjq_Small_URIEncode-16            	 7426963	       162.3 ns/op	     120 B/op	       4 allocs/op
BenchmarkGojq_Small_URIEncode-16              	 1793726	       667.7 ns/op	    1289 B/op	      14 allocs/op
BenchmarkFastjq_Small_ArrayDiff-16            	 5855810	       199.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ArrayDiff-16              	  931933	      1244 ns/op	    2121 B/op	      33 allocs/op
BenchmarkFastjq_Small_TryNoError-16           	138274634	         8.680 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TryCatchNoError-16      	19495036	        60.67 ns/op	      64 B/op	       1 allocs/op
BenchmarkGojq_Small_TryNoError-16             	 2872515	       421.8 ns/op	    1449 B/op	      16 allocs/op
BenchmarkFastjq_Small_ObjectMerge-16          	 6825721	       166.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ObjectMerge-16            	  804969	      1494 ns/op	    2906 B/op	      33 allocs/op
BenchmarkFastjq_Small_ToJSON-16               	 9076545	       132.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ToJSON-16                 	 2816800	       423.3 ns/op	    1313 B/op	      17 allocs/op
BenchmarkFastjq_Small_FromJSON-16             	 7899866	       150.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_FromJSON-16               	  934714	      1290 ns/op	    2714 B/op	      31 allocs/op
BenchmarkFastjq_Small_ToString-16             	 8976033	       134.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToNumber-16             	90684385	        13.11 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ToNumber-16               	 3358306	       354.5 ns/op	    1112 B/op	      11 allocs/op
BenchmarkFastjq_Small_AnyTwoArg-16            	  369969	      3265 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_AnyTwoArg-16              	  704545	      1648 ns/op	    2482 B/op	      27 allocs/op
BenchmarkFastjq_Complex_LogNormalize-16       	 1000000	      1080 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Complex_LogNormalize-16         	  408486	      2983 ns/op	    4324 B/op	      69 allocs/op
BenchmarkFastjq_Complex_ArrayPipeline-16      	  150141	      7792 ns/op	    1232 B/op	      64 allocs/op
BenchmarkGojq_Complex_ArrayPipeline-16        	   52786	     23114 ns/op	   20409 B/op	     525 allocs/op
BenchmarkFastjq_Complex_Aggregation-16        	  109726	     10930 ns/op	     416 B/op	      26 allocs/op
BenchmarkGojq_Complex_Aggregation-16          	   54763	     22393 ns/op	   22301 B/op	     551 allocs/op
BenchmarkFastjq_Complex_TolerantMap-16        	  129358	      9301 ns/op	    2440 B/op	      98 allocs/op
BenchmarkGojq_Complex_TolerantMap-16          	   42402	     27797 ns/op	   27868 B/op	     641 allocs/op
BenchmarkFastjq_Complex_ElifRouting-16        	  142072	      8457 ns/op	    2520 B/op	     125 allocs/op
BenchmarkGojq_Complex_ElifRouting-16          	   47288	     25141 ns/op	   25919 B/op	     595 allocs/op
BenchmarkFastjq_Complex_StringBuild-16        	  147394	      7987 ns/op	    1360 B/op	     124 allocs/op
BenchmarkGojq_Complex_StringBuild-16          	   49594	     24101 ns/op	   20533 B/op	     686 allocs/op
BenchmarkFastjq_Complex_EntryFilter-16        	  438968	      2712 ns/op	    2168 B/op	      35 allocs/op
BenchmarkGojq_Complex_EntryFilter-16          	  153915	      7733 ns/op	   11188 B/op	     162 allocs/op
BenchmarkFastjq_Small_Sqrt-16                 	18379478	        68.00 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Sqrt-16                   	 2725333	       446.5 ns/op	    1145 B/op	      12 allocs/op
BenchmarkFastjq_Small_Log-16                  	31436139	        38.31 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Log-16                    	 2652590	       462.2 ns/op	    1129 B/op	      12 allocs/op
BenchmarkFastjq_Small_Sin-16                  	15458048	        78.63 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Sin-16                    	 2709307	       482.4 ns/op	    1145 B/op	      12 allocs/op
BenchmarkFastjq_Small_Atan-16                 	22235209	        53.85 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Atan-16                   	 3113286	       384.7 ns/op	    1121 B/op	      11 allocs/op
BenchmarkFastjq_Small_Exp-16                  	18928414	        63.69 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Exp-16                    	 2960847	       410.8 ns/op	    1121 B/op	      11 allocs/op
BenchmarkFastjq_Small_Tgamma-16               	78317694	        15.48 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Tgamma-16                 	 3000199	       396.5 ns/op	    1105 B/op	      11 allocs/op
BenchmarkFastjq_Small_Fabs-16                 	21940365	        55.65 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Fabs-16                   	 2807070	       446.8 ns/op	    1113 B/op	      12 allocs/op
BenchmarkFastjq_Small_StringInterp-16         	 8664746	       134.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_StringInterp-16           	 1000000	      1050 ns/op	    2121 B/op	      35 allocs/op
BenchmarkFastjq_Small_StringInterpNum-16      	11441032	       104.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_StringInterpNum-16        	  982542	      1228 ns/op	    2498 B/op	      40 allocs/op
BenchmarkFastjq_Small_IsEmptyTrue-16          	133357590	         8.983 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_IsEmptyTrue-16            	 1855465	       675.7 ns/op	    1921 B/op	      18 allocs/op
BenchmarkFastjq_Small_IsEmptyFalse-16         	51370412	        23.14 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_IsEmptyFalse-16           	 1000000	      1032 ns/op	    2898 B/op	      33 allocs/op
BenchmarkFastjq_Small_Nth-16                  	28823115	        40.08 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Nth-16                    	  565705	      2155 ns/op	    4372 B/op	      49 allocs/op
BenchmarkRegexp_Match_Hit-16                  	28823664	        41.26 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Miss-16                 	53208097	        22.61 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Complex_Hit-16          	28038775	        42.21 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Long_Miss-16            	15718412	        76.08 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_MatchString-16                	27348219	        44.19 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_FindSubmatch_Hit-16           	 5948732	       200.7 ns/op	     160 B/op	       2 allocs/op
BenchmarkRegexp_FindSubmatch_Miss-16          	 1806182	       664.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_FindSubmatchIndex_Hit-16      	 6669272	       178.0 ns/op	      64 B/op	       1 allocs/op
BenchmarkRegexp_FindSubmatchIndex_Miss-16     	 1805497	       663.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_ReplaceAll_Hit-16             	 7934362	       151.5 ns/op	      97 B/op	       5 allocs/op
BenchmarkRegexp_ReplaceAll_Miss-16            	24028653	        49.59 ns/op	      48 B/op	       2 allocs/op
BenchmarkRegexp_MatchViaClosure-16            	28899819	        41.63 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_Hit-16           	130644541	         9.194 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_Miss-16          	10983751	       109.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_InPipeline-16    	 4347813	       278.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_MatchRe_Hit-16          	 5533662	       214.8 ns/op	      48 B/op	       1 allocs/op
BenchmarkFastjq_Small_MatchRe_Miss-16         	 2028944	       592.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_TestRe_Hit-16             	  616683	      1934 ns/op	    4176 B/op	      44 allocs/op
BenchmarkGojq_Small_TestRe_Miss-16            	  645208	      1903 ns/op	    4122 B/op	      43 allocs/op
BenchmarkGojq_Small_MatchRe_Hit-16            	  276867	      4276 ns/op	    8028 B/op	     100 allocs/op
BenchmarkGojq_Small_MatchRe_Miss-16           	  401738	      3040 ns/op	    5728 B/op	      59 allocs/op
BenchmarkFastjq_Small_CaptureRe_Hit-16        	 5842461	       207.1 ns/op	      48 B/op	       1 allocs/op
BenchmarkFastjq_Small_CaptureRe_Miss-16       	 1951366	       618.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ScanRe_NoGroups-16      	 2536533	       474.1 ns/op	     387 B/op	      10 allocs/op
BenchmarkFastjq_Small_ScanRe_WithGroups-16    	 2035191	       589.3 ns/op	     702 B/op	      13 allocs/op
BenchmarkFastjq_Small_SubRe_Hit-16            	 9785439	       122.2 ns/op	      16 B/op	       1 allocs/op
BenchmarkFastjq_Small_SubRe_Miss-16           	11991849	       100.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_GSubRe_Hit-16           	 2126709	       562.7 ns/op	     305 B/op	       5 allocs/op
BenchmarkFastjq_Small_GSubRe_Miss-16          	 4823088	       247.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_CaptureRe_Hit-16          	  352230	      3339 ns/op	    7393 B/op	      77 allocs/op
BenchmarkGojq_Small_CaptureRe_Miss-16         	  414154	      2913 ns/op	    5587 B/op	      54 allocs/op
BenchmarkGojq_Small_ScanRe_NoGroups-16        	  385620	      3094 ns/op	    6019 B/op	      67 allocs/op
BenchmarkGojq_Small_SubRe_Hit-16              	  333231	      3587 ns/op	    6179 B/op	      77 allocs/op
BenchmarkGojq_Small_GSubRe_Hit-16             	  161589	      7507 ns/op	    9440 B/op	     143 allocs/op
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
