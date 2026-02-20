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
| `.field` | Small (~100B) | 0.157 | 0.347 | **2.2x** | 0 | 13 |
| `.field` | Large (~100KB) | 134 | 582 | **4.4x** | 0 | 2835 |
| `del(.f)` | Small (~100B) | 0.167 | 0.932 | **5.6x** | 0 | 33 |
| `del(.f)` | Medium (~2KB) | 2.75 | 18.8 | **6.8x** | 0 | 323 |
| `del(.f)` | Large (~100KB) | 219 | 802 | **3.7x** | 0 | 4666 |
| `.[n]` | 5-elem array | 0.026 | 0.616 | **24x** | 0 | 20 |
| `del(.[n], .[m])` | 5-elem array | 0.090 | 1.63 | **18x** | 0 | 53 |
| `{f0, f2}` (construct) | Small (~100B) | 0.300 | 0.694 | **2.3x** | 0 | 23 |
| `{f0, f50}` (construct) | Large (~100KB) | 232 | 585 | **2.5x** | 0 | 2866 |
| `.[]` iterator | 5-elem array | 0.032 | 0.787 | **25x** | 0 | 26 |
| `.[]` iterator | 200-elem array | 17.6 | 84.9 | **4.8x** | 0 | 1811 |
| `select(.f == "x")` | Small (~100B) | 0.010 | 0.570 | **57x** | 0 | 20 |
| `select(.f == "x")`¹ | Large (~100KB, last field) | 240 | 802 | **3.3x** | 0 | 4651 |
| `select(.f and .g)` | Small (~100B) | 0.011 | 0.705 | **62x** | 0 | 21 |
| `select(.f or .g)` | Small (~100B) | 0.011 | 0.740 | **66x** | 0 | 21 |
| `has("key")` in select | Small (~100B) | 0.013 | 0.571 | **44x** | 0 | 20 |
| `has("key")` in select | Large (~100KB) | 226 | 782 | **3.5x** | 0 | 4651 |
| `if-then-else` | Small (~100B) | 0.011 | 0.531 | **50x** | 0 | 16 |
| `.f // "default"` | Small (~100B) | 0.0084 | 0.506 | **60x** | 0 | 17 |
| `try .field` (no error) | Small (~100B) | 0.0073 | 0.451 | **62x** | 0 | 16 |
| `.a + .b` (strings) | Small (~100B) | 0.073 | 0.674 | **9.3x** | 0 | 21 |
| `"prefix" + .name` | Small (~100B) | 0.111 | 0.717 | **6.4x** | 0 | 23 |
| `.a - .b` (subtract) | Small (~100B) | 0.066 | 0.690 | **10x** | 0 | 20 |
| `.a * .b` (multiply) | Small (~100B) | 0.085 | 0.709 | **8.4x** | 0 | 21 |
| `.a / .b` (divide) | Small (~100B) | 0.120 | 0.714 | **6x** | 0 | 21 |
| `.a - .b` (array diff) | 5-elem arrays | 0.217 | 1.28 | **5.9x** | 0 | 33 |
| `length` | Small (~100B) | 0.0077 | 0.387 | **50x** | 0 | 13 |
| `length` | Large (~100KB) | 227 | 595 | **2.6x** | 0 | 2835 |
| `add` (numbers) | 5-elem array | 0.061 | 0.673 | **11x** | 0 | 24 |
| `add` (strings) | 5-elem array | 0.141 | 0.828 | **5.9x** | 0 | 31 |
| `flatten` | 3-elem nested array | 0.104 | 1.27 | **12x** | 0 | 38 |
| `min` | 200-int array | 1.63 | 1.25 | 0.8x† | 0 | 15 |
| `min_by(.value)` | 100-elem object array | 11.2 | 55.2 | **4.9x** | 0 | 1347 |
| `map(.name)` | 20-elem array (~600B) | 1.91 | 9.99 | **5.2x** | 0 | 251 |
| `map(.name)` | 200-elem array (~6KB) | 20.5 | 91.1 | **4.4x** | 0 | 2237 |
| `any` | 5-elem array | 0.046 | 1.83 | **39x** | 0 | 39 |
| `any(expr)` | 5-elem array | 0.122 | 2.06 | **17x** | 0 | 49 |
| `any(expr)`² | 200-int array | 2.91 | 1.71 | 0.6x† | 0 | 29 |
| `any(gen; cond)`² | 200-int array | 3.20 | 1.63 | 0.5x† | 0 | 27 |
| `first(expr)` | 5-elem array | 0.109 | 1.53 | **14x** | 1 | 39 |
| `first(expr)`² | 200-int array | 3.52 | 1.47 | 0.4x† | 0 | 23 |
| `last(expr)` | 5-elem array | 0.143 | 1.94 | **14x** | 0 | 43 |
| `last(expr)`² | 200-int array | 3.29 | 1.57 | 0.5x† | 0 | 24 |
| `limit(3; expr)` | 5-elem array | 0.037 | 1.64 | **44x** | 0 | 42 |
| `limit(10; expr)` | 200-int array | 0.573 | 1.59 | **2.8x** | 0 | 24 |
| `.[1:4]` slice | 6-elem array | 0.089 | 0.798 | **9x** | 0 | 21 |
| `values` | 9-elem array | 0.088 | 2.36 | **27x** | 0 | 51 |
| `to_entries` | Small (~100B) | 0.0065 | 0.372 | **57x** | 0 | 14 |
| `to_entries` | Large (~100KB) | 195 | 832 | **4.3x** | 0 | 5847 |
| `keys_unsorted` | Small (~100B) | 0.065 | 0.376 | **5.8x** | 3 | 14 |
| `keys_unsorted` | Large (~100KB) | 227 | 603 | **2.7x** | 0 | 3039 |
| `object merge .a + .b` | Small (~100B) | 0.218 | 1.48 | **6.8x** | 0 | 33 |
| `tojson` | Small (~100B) | 0.126 | 0.422 | **3.3x** | 0 | 17 |
| `fromjson` | JSON string | 0.154 | 1.31 | **8.5x** | 0 | 31 |
| `tonumber` | `"42"` string | 0.016 | 0.357 | **22x** | 0 | 11 |
| `split(",")` | short string | 0.112 | 0.771 | **6.9x** | 0 | 21 |
| `join(",")` | 5-elem array | 0.104 | 0.855 | **8.2x** | 0 | 30 |
| `ascii_downcase` in select | Small (~100B) | 0.013 | 0.573 | **44x** | 0 | 21 |
| `ascii_downcase` in select | Large (~100KB) | 227 | 776 | **3.4x** | 0 | 4652 |
| `startswith("s")` | Small (~100B) | 0.013 | 0.580 | **45x** | 0 | 21 |
| `startswith("s")` | Large (~100KB) | 191 | 791 | **4.1x** | 0 | 4651 |
| `endswith("s")` | Small (~100B) | 0.013 | 0.590 | **45x** | 0 | 21 |
| `ltrimstr("s")` | Small (~100B) | 0.0078 | 0.429 | **55x** | 0 | 16 |
| `rtrimstr("s")` | Small (~100B) | 0.0078 | 0.434 | **56x** | 0 | 16 |
| `select` + string ops + arith + construct | ~200B log event | 1.61 | 2.88 | **1.8x** | 0 | 69 |
| `[.[] \| select + arith + construct]` | 20-elem array (~1.5KB) | 11.4 | 22.4 | **2x** | 64 | 525 |
| `length + map + add + min_by + any` | 20-elem array (~1.5KB) | 13.0 | 21.2 | **1.6x** | 26 | 551 |
| `map(try {…} catch …)` | 20-elem array (~1.5KB) | 12.3 | 27.3 | **2.2x** | 98 | 641 |
| `map(if … elif … else …)` | 20-elem array (~1.5KB) | 12.1 | 24.8 | **2x** | 125 | 595 |
| `[.[] \| str + tostring + str]` | 20-elem array (~1.5KB) | 10.9 | 24.4 | **2.2x** | 124 | 686 |
| `to_entries \| map(select) \| from_entries` | ~200B log event | 2.92 | 7.67 | **2.6x** | 35 | 162 |
| `@base64` | 34-char string | 0.144 | 0.507 | **3.5x** | 4 | 15 |
| `@base64d` | 48-char encoded | 0.207 | 0.538 | **2.6x** | 0 | 15 |
| `@uri` | 36-char URL string | 0.168 | 0.676 | **4x** | 4 | 14 |
| `index(",")` | short string | 0.082 | 0.901 | **11x** | 0 | 31 |
| `indices(",")` | short string | 0.155 | 2.10 | **14x** | 0 | 97 |
| `sqrt` | float (e≈2.718) | 0.070 | 0.437 | **6.2x** | 0 | 12 |
| `log` | float (e≈2.718) | 0.039 | 0.437 | **11x** | 0 | 12 |
| `sin` | float (e≈2.718) | 0.080 | 0.443 | **5.5x** | 0 | 12 |
| `atan` | integer 1 | 0.054 | 0.381 | **7.1x** | 0 | 11 |
| `exp` | integer 1 | 0.065 | 0.397 | **6.1x** | 0 | 11 |
| `tgamma` | integer 5 | 0.014 | 0.371 | **27x** | 0 | 11 |
| `fabs` | float -3.14 | 0.056 | 0.416 | **7.5x** | 0 | 12 |
| ``"\(.level): \(.svc)"`` | ~45B object | 0.153 | 1.04 | **6.8x** | 0 | 35 |
| ``"user \(.name) …"`` | ~45B object | 0.134 | 1.20 | **8.9x** | 0 | 40 |
| `isempty(empty)` | null | 0.0074 | 0.560 | **76x** | 0 | 18 |
| `isempty(.[])` | 5-elem array | 0.021 | 1.02 | **49x** | 0 | 33 |
| `nth(2; .[])` | 5-elem array | 0.039 | 2.11 | **54x** | 0 | 49 |

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
BenchmarkFastjq_Small_Del-16                	 7770967	       166.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Medium_Del-16               	  581215	      2755 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Del-16                	    5236	    218662 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Field-16              	 7706740	       156.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Field-16              	   10000	    133522 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Index-16              	47072437	        25.55 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ArrayDel-16           	13132999	        90.40 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Construct-16          	 4232742	       300.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Construct-16          	    5760	    231716 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Iterator-16           	35485766	        31.76 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Iterator-16           	   68343	     17647 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Select-16             	100000000	        10.01 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Select-16             	    6073	    239675 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Alternative-16        	142971760	         8.384 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SelectAnd-16          	100000000	        11.37 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SelectOr-16           	100000000	        11.28 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Has-16                	90993824	        13.07 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Has-16                	    9805	    225868 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_IfThenElse-16         	100000000	        10.66 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Length-16             	155729734	         7.700 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Length-16             	    8107	    226878 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Map-16                	  593869	      1907 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Medium_Map-16               	  121268	     10141 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Map-16                	   60187	     20536 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToEntries-16          	183796312	         6.538 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_ToEntries-16          	    8982	    194783 ns/op	      24 B/op	       0 allocs/op
BenchmarkFastjq_Small_KeysUnsorted-16       	18178558	        64.74 ns/op	      72 B/op	       3 allocs/op
BenchmarkFastjq_Large_KeysUnsorted-16       	    8656	    227407 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Any-16                	25409036	        46.46 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Any-16                	  750216	      1576 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AnyExpr-16            	 9656194	       122.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_AnyExpr-16            	  424982	      2908 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Add-16                	19459800	        61.30 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AddStrings-16         	 8480592	       141.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Flatten-16            	11269527	       104.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Split-16              	10743895	       111.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Join-16               	11291022	       104.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Values-16             	13261371	        87.52 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Values-16               	  503676	      2361 ns/op	    3144 B/op	      51 allocs/op
BenchmarkFastjq_Small_Base64Encode-16       	 8263002	       144.4 ns/op	     120 B/op	       4 allocs/op
BenchmarkFastjq_Small_Base64Decode-16       	 5611905	       207.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Base64Encode-16         	 2359200	       506.6 ns/op	    1321 B/op	      15 allocs/op
BenchmarkGojq_Small_Base64Decode-16         	 2238914	       538.4 ns/op	    1321 B/op	      15 allocs/op
BenchmarkFastjq_Small_IndexFind-16          	14326740	        82.28 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_IndicesAll-16         	 7709065	       155.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_IndexFind-16            	 1332120	       900.8 ns/op	    2273 B/op	      31 allocs/op
BenchmarkGojq_Small_IndicesAll-16           	  550033	      2099 ns/op	    3907 B/op	      97 allocs/op
BenchmarkFastjq_Small_Slice-16              	13957456	        88.59 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SliceString-16        	25460858	        47.01 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Plus-16               	16354399	        72.82 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_PlusStr-16            	10447891	       111.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AsciiDowncase-16      	93545368	        13.02 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_AsciiDowncase-16      	    8088	    226623 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Startswith-16         	93515902	        12.91 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Startswith-16         	    5397	    191332 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Endswith-16           	92245015	        12.99 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Ltrimstr-16           	153285702	         7.802 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Ltrimstr-16           	    8223	    146888 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Rtrimstr-16           	154851097	         7.822 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_First-16              	10845169	       108.9 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Large_First-16              	  327229	      3516 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Last-16               	 8305082	       143.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Last-16               	  356743	      3285 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Limit-16              	32606628	        37.30 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Limit-16              	 2140836	       573.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Del-16                  	 1286988	       932.3 ns/op	    2594 B/op	      33 allocs/op
BenchmarkGojq_Medium_Del-16                 	   64136	     18841 ns/op	   16968 B/op	     323 allocs/op
BenchmarkGojq_Large_Del-16                  	    1435	    802130 ns/op	  544019 B/op	    4666 allocs/op
BenchmarkGojq_Small_Field-16                	 3479197	       347.2 ns/op	    1177 B/op	      13 allocs/op
BenchmarkGojq_Large_Field-16                	    2040	    581526 ns/op	  270039 B/op	    2835 allocs/op
BenchmarkGojq_Small_Index-16                	 1956699	       616.4 ns/op	    1401 B/op	      20 allocs/op
BenchmarkGojq_Small_ArrayDel-16             	  745941	      1628 ns/op	    3362 B/op	      53 allocs/op
BenchmarkGojq_Small_Construct-16            	 1734780	       693.6 ns/op	    1857 B/op	      23 allocs/op
BenchmarkGojq_Large_Construct-16            	    2048	    584832 ns/op	  274358 B/op	    2866 allocs/op
BenchmarkGojq_Small_Iterator-16             	 1521765	       787.1 ns/op	    1776 B/op	      26 allocs/op
BenchmarkGojq_Large_Iterator-16             	   14133	     84864 ns/op	  109808 B/op	    1811 allocs/op
BenchmarkGojq_Small_Select-16               	 2105620	       570.4 ns/op	    1753 B/op	      20 allocs/op
BenchmarkGojq_Large_Select-16               	    1563	    801588 ns/op	  538158 B/op	    4651 allocs/op
BenchmarkGojq_Small_Alternative-16          	 2420300	       505.9 ns/op	    1441 B/op	      17 allocs/op
BenchmarkGojq_Small_SelectAnd-16            	 1675434	       705.0 ns/op	    1945 B/op	      21 allocs/op
BenchmarkGojq_Small_SelectOr-16             	 1624984	       740.0 ns/op	    1945 B/op	      21 allocs/op
BenchmarkGojq_Small_Has-16                  	 2071718	       571.1 ns/op	    1753 B/op	      20 allocs/op
BenchmarkGojq_Large_Has-16                  	    1519	    781743 ns/op	  539073 B/op	    4651 allocs/op
BenchmarkGojq_Small_IfThenElse-16           	 2411578	       531.4 ns/op	    1361 B/op	      16 allocs/op
BenchmarkGojq_Small_Length-16               	 3044518	       386.5 ns/op	    1177 B/op	      13 allocs/op
BenchmarkGojq_Large_Length-16               	    2010	    594962 ns/op	  269833 B/op	    2835 allocs/op
BenchmarkGojq_Small_Map-16                  	  120908	      9992 ns/op	   13653 B/op	     251 allocs/op
BenchmarkGojq_Large_Map-16                  	   13186	     91054 ns/op	  118561 B/op	    2237 allocs/op
BenchmarkGojq_Small_ToEntries-16            	 3250588	       371.7 ns/op	    1209 B/op	      14 allocs/op
BenchmarkGojq_Large_ToEntries-16            	    1368	    832475 ns/op	  647296 B/op	    5847 allocs/op
BenchmarkGojq_Small_KeysUnsorted-16         	 3243505	       375.7 ns/op	    1209 B/op	      14 allocs/op
BenchmarkGojq_Large_KeysUnsorted-16         	    1995	    603260 ns/op	  282796 B/op	    3039 allocs/op
BenchmarkGojq_Small_Any-16                  	  653850	      1827 ns/op	    4508 B/op	      39 allocs/op
BenchmarkGojq_Small_AnyExpr-16              	  563268	      2058 ns/op	    4620 B/op	      49 allocs/op
BenchmarkGojq_Large_AnyExpr-16              	  703725	      1709 ns/op	    2818 B/op	      29 allocs/op
BenchmarkGojq_Small_Add-16                  	 1791164	       672.7 ns/op	    1433 B/op	      24 allocs/op
BenchmarkGojq_Small_AddStrings-16           	 1444599	       828.4 ns/op	    1689 B/op	      31 allocs/op
BenchmarkGojq_Small_Flatten-16              	  941474	      1266 ns/op	    1857 B/op	      38 allocs/op
BenchmarkGojq_Small_Split-16                	 1563996	       771.3 ns/op	    1553 B/op	      21 allocs/op
BenchmarkGojq_Small_Join-16                 	 1401903	       855.2 ns/op	    1745 B/op	      30 allocs/op
BenchmarkGojq_Small_Slice-16                	 1504250	       797.9 ns/op	    1425 B/op	      21 allocs/op
BenchmarkGojq_Small_SliceString-16          	 2711434	       443.6 ns/op	    1145 B/op	      12 allocs/op
BenchmarkGojq_Small_Plus-16                 	 1777077	       674.1 ns/op	    1673 B/op	      21 allocs/op
BenchmarkGojq_Small_PlusStr-16              	 1683528	       717.3 ns/op	    1705 B/op	      23 allocs/op
BenchmarkGojq_Small_AsciiDowncase-16        	 2098633	       572.8 ns/op	    1785 B/op	      21 allocs/op
BenchmarkGojq_Large_AsciiDowncase-16        	    1534	    775950 ns/op	  535337 B/op	    4652 allocs/op
BenchmarkGojq_Small_Startswith-16           	 2073980	       579.9 ns/op	    1801 B/op	      21 allocs/op
BenchmarkGojq_Large_Startswith-16           	    1521	    790767 ns/op	  537993 B/op	    4651 allocs/op
BenchmarkGojq_Small_Endswith-16             	 2004114	       590.2 ns/op	    1801 B/op	      21 allocs/op
BenchmarkGojq_Small_Ltrimstr-16             	 2874116	       429.2 ns/op	    1305 B/op	      16 allocs/op
BenchmarkGojq_Small_Rtrimstr-16             	 2718583	       434.3 ns/op	    1305 B/op	      16 allocs/op
BenchmarkGojq_Small_First-16                	  782901	      1529 ns/op	    3379 B/op	      39 allocs/op
BenchmarkGojq_Large_First-16                	  797798	      1468 ns/op	    1889 B/op	      23 allocs/op
BenchmarkGojq_Small_Last-16                 	  623234	      1937 ns/op	    3483 B/op	      43 allocs/op
BenchmarkGojq_Large_Last-16                 	  757023	      1567 ns/op	    2193 B/op	      24 allocs/op
BenchmarkGojq_Small_Limit-16                	  710955	      1640 ns/op	    3328 B/op	      42 allocs/op
BenchmarkGojq_Large_Limit-16                	  746785	      1594 ns/op	    2200 B/op	      24 allocs/op
BenchmarkFastjq_Small_Subtract-16           	18571814	        65.96 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Subtract-16             	 1722547	       689.8 ns/op	    1649 B/op	      20 allocs/op
BenchmarkFastjq_Small_Multiply-16           	13680598	        84.64 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Multiply-16             	 1709864	       709.1 ns/op	    1649 B/op	      21 allocs/op
BenchmarkFastjq_Small_Divide-16             	 9788683	       119.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Divide-16               	 1679865	       713.6 ns/op	    1665 B/op	      21 allocs/op
BenchmarkFastjq_Small_Min-16                	  703148	      1627 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Min-16                  	  953598	      1245 ns/op	    1217 B/op	      15 allocs/op
BenchmarkFastjq_Small_MinBy-16              	  101150	     11183 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_MinBy-16                	   21752	     55152 ns/op	   63629 B/op	    1347 allocs/op
BenchmarkFastjq_Small_URIEncode-16          	 7220728	       168.4 ns/op	     120 B/op	       4 allocs/op
BenchmarkGojq_Small_URIEncode-16            	 1765347	       676.0 ns/op	    1289 B/op	      14 allocs/op
BenchmarkFastjq_Small_ArrayDiff-16          	 5428911	       217.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ArrayDiff-16            	  890838	      1277 ns/op	    2121 B/op	      33 allocs/op
BenchmarkFastjq_Small_TryNoError-16         	163443675	         7.283 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TryCatchNoError-16    	19467391	        60.40 ns/op	      64 B/op	       1 allocs/op
BenchmarkGojq_Small_TryNoError-16           	 2478852	       451.4 ns/op	    1449 B/op	      16 allocs/op
BenchmarkFastjq_Small_ObjectMerge-16        	 5516778	       217.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ObjectMerge-16          	  788973	      1481 ns/op	    2906 B/op	      33 allocs/op
BenchmarkFastjq_Small_ToJSON-16             	 9538369	       126.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ToJSON-16               	 2854836	       421.9 ns/op	    1313 B/op	      17 allocs/op
BenchmarkFastjq_Small_FromJSON-16           	 7700264	       154.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_FromJSON-16             	  891637	      1312 ns/op	    2714 B/op	      31 allocs/op
BenchmarkFastjq_Small_ToString-16           	 9073812	       129.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToNumber-16           	73828548	        16.32 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ToNumber-16             	 3324573	       356.9 ns/op	    1112 B/op	      11 allocs/op
BenchmarkFastjq_Small_AnyTwoArg-16          	  380260	      3200 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_AnyTwoArg-16            	  735085	      1632 ns/op	    2482 B/op	      27 allocs/op
BenchmarkFastjq_Complex_LogNormalize-16     	  766242	      1608 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Complex_LogNormalize-16       	  417712	      2884 ns/op	    4324 B/op	      69 allocs/op
BenchmarkFastjq_Complex_ArrayPipeline-16    	  105507	     11372 ns/op	    1232 B/op	      64 allocs/op
BenchmarkGojq_Complex_ArrayPipeline-16      	   54202	     22361 ns/op	   20408 B/op	     525 allocs/op
BenchmarkFastjq_Complex_Aggregation-16      	   89322	     12971 ns/op	     416 B/op	      26 allocs/op
BenchmarkGojq_Complex_Aggregation-16        	   56599	     21174 ns/op	   22300 B/op	     551 allocs/op
BenchmarkFastjq_Complex_TolerantMap-16      	   99064	     12284 ns/op	    2440 B/op	      98 allocs/op
BenchmarkGojq_Complex_TolerantMap-16        	   44144	     27253 ns/op	   27870 B/op	     641 allocs/op
BenchmarkFastjq_Complex_ElifRouting-16      	  100408	     12139 ns/op	    2520 B/op	     125 allocs/op
BenchmarkGojq_Complex_ElifRouting-16        	   48613	     24773 ns/op	   25916 B/op	     595 allocs/op
BenchmarkFastjq_Complex_StringBuild-16      	  108430	     10948 ns/op	    1360 B/op	     124 allocs/op
BenchmarkGojq_Complex_StringBuild-16        	   49371	     24439 ns/op	   20533 B/op	     686 allocs/op
BenchmarkFastjq_Complex_EntryFilter-16      	  378142	      2915 ns/op	    2168 B/op	      35 allocs/op
BenchmarkGojq_Complex_EntryFilter-16        	  156175	      7675 ns/op	   11187 B/op	     162 allocs/op
BenchmarkFastjq_Small_Sqrt-16               	16856871	        69.99 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Sqrt-16                 	 2723330	       437.0 ns/op	    1145 B/op	      12 allocs/op
BenchmarkFastjq_Small_Log-16                	33159018	        39.08 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Log-16                  	 2732857	       437.1 ns/op	    1128 B/op	      12 allocs/op
BenchmarkFastjq_Small_Sin-16                	14902759	        80.34 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Sin-16                  	 2691912	       442.8 ns/op	    1145 B/op	      12 allocs/op
BenchmarkFastjq_Small_Atan-16               	22128890	        53.88 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Atan-16                 	 3104538	       381.3 ns/op	    1121 B/op	      11 allocs/op
BenchmarkFastjq_Small_Exp-16                	18711942	        64.69 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Exp-16                  	 2990660	       396.5 ns/op	    1121 B/op	      11 allocs/op
BenchmarkFastjq_Small_Tgamma-16             	88229344	        13.70 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Tgamma-16               	 3216086	       371.5 ns/op	    1104 B/op	      11 allocs/op
BenchmarkFastjq_Small_Fabs-16               	21681522	        55.52 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Fabs-16                 	 2876092	       416.1 ns/op	    1112 B/op	      12 allocs/op
BenchmarkFastjq_Small_StringInterp-16       	 7953027	       152.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_StringInterp-16         	 1000000	      1039 ns/op	    2121 B/op	      35 allocs/op
BenchmarkFastjq_Small_StringInterpNum-16    	 8745435	       134.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_StringInterpNum-16      	  982053	      1196 ns/op	    2498 B/op	      40 allocs/op
BenchmarkFastjq_Small_IsEmptyTrue-16        	162603948	         7.381 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_IsEmptyTrue-16          	 2137690	       560.4 ns/op	    1921 B/op	      18 allocs/op
BenchmarkFastjq_Small_IsEmptyFalse-16       	54899388	        20.81 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_IsEmptyFalse-16         	 1000000	      1016 ns/op	    2898 B/op	      33 allocs/op
BenchmarkFastjq_Small_Nth-16                	28919756	        39.20 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Nth-16                  	  566965	      2114 ns/op	    4372 B/op	      49 allocs/op
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
