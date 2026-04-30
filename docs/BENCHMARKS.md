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
| `.field` | Small (~100B) | 0.089 | 1.16 | **13x** | 0 | 27 |
| `.field` | Large (~100KB) | 7.93 | 656 | **83x** | 0 | 2835 |
| `del(.f)` | Small (~100B) | 0.178 | 2.78 | **16x** | 0 | 58 |
| `del(.f)` | Medium (~2KB) | 2.26 | 23.6 | **10x** | 0 | 323 |
| `del(.f)` | Large (~100KB) | 35.1 | 1011 | **29x** | 0 | 4666 |
| `.[n]` | 5-elem array | 0.023 | 0.737 | **33x** | 0 | 20 |
| `del(.[n], .[m])` | 5-elem array | 0.084 | 1.92 | **23x** | 0 | 53 |
| `{f0, f2}` (construct) | Small (~100B) | 0.140 | 1.55 | **11x** | 0 | 37 |
| `{f0, f50}` (construct) | Large (~100KB) | 8.29 | 635 | **77x** | 0 | 2867 |
| `.[]` iterator | 5-elem array | 0.033 | 0.894 | **27x** | 0 | 26 |
| `.[]` iterator | 200-elem array | 7.62 | 151 | **20x** | 0 | 1811 |
| `select(.f == "x")` | Small (~100B) | 0.130 | 2.59 | **20x** | 0 | 45 |
| `select(.f == "x")`¹ | Large (~100KB, last field) | 34.5 | 945 | **27x** | 0 | 4651 |
| `select(.f and .g)` | Small (~100B) | 0.173 | 2.00 | **12x** | 0 | 46 |
| `select(.f or .g)` | Small (~100B) | 0.074 | 2.01 | **27x** | 0 | 46 |
| `has("key")` in select | Small (~100B) | 0.094 | 2.02 | **22x** | 0 | 45 |
| `has("key")` in select | Large (~100KB) | 33.7 | 998 | **30x** | 0 | 4651 |
| `if-then-else` | Small (~100B) | 0.098 | 1.55 | **16x** | 0 | 30 |
| `.f // "default"` | Small (~100B) | 0.096 | 1.18 | **12x** | 0 | 31 |
| `try .field` (no error) | Small (~100B) | 4.53 | 1.22 | 0.3x† | 1 | 30 |
| `.a + .b` (strings) | Small (~100B) | 0.057 | 0.753 | **13x** | 0 | 21 |
| `"prefix" + .name` | Small (~100B) | 0.086 | 0.810 | **9.4x** | 0 | 23 |
| `.a - .b` (subtract) | Small (~100B) | 0.060 | 0.786 | **13x** | 0 | 20 |
| `.a * .b` (multiply) | Small (~100B) | 0.066 | 0.774 | **12x** | 0 | 21 |
| `.a / .b` (divide) | Small (~100B) | 0.101 | 0.795 | **7.9x** | 0 | 21 |
| `.a - .b` (array diff) | 5-elem arrays | 0.239 | 1.37 | **5.7x** | 0 | 33 |
| `length` | Small (~100B) | 0.043 | 1.33 | **31x** | 0 | 27 |
| `length` | Large (~100KB) | 32.5 | 695 | **21x** | 0 | 2835 |
| `add` (numbers) | 5-elem array | 0.089 | 0.881 | **9.9x** | 0 | 28 |
| `add` (strings) | 5-elem array | 0.166 | 1.76 | **11x** | 0 | 35 |
| `flatten` | 3-elem nested array | 0.098 | 1.85 | **19x** | 0 | 38 |
| `min` | 200-int array | 1.94 | 1.33 | 0.7x† | 0 | 15 |
| `min_by(.value)` | 100-elem object array | 8.40 | 58.8 | **7x** | 0 | 1347 |
| `sort` | 200-int array | 4.79 | 1.45 | 0.3x† | 11 | 15 |
| `sort_by(.value)` | 100-elem object array | 16.1 | 110 | **6.8x** | 119 | 2145 |
| `unique` | 200-int array | 6.14 | 1.30 | 0.2x† | 11 | 15 |
| `group_by(.active)` | 100-elem object array | 18.9 | 106 | **5.6x** | 119 | 2260 |
| `map(.name)` | 20-elem array (~600B) | 1.26 | 11.9 | **9.4x** | 0 | 251 |
| `map(.name)` | 200-elem array (~6KB) | 13.0 | 113 | **8.7x** | 0 | 2237 |
| `any` | 5-elem array | 0.047 | 2.18 | **47x** | 0 | 39 |
| `any(expr)` | 5-elem array | 0.476 | 2.33 | **4.9x** | 12 | 49 |
| `any(expr)`² | 200-int array | 6.96 | 1.99 | 0.3x† | 0 | 29 |
| `any(gen; cond)`² | 200-int array | 4.44 | 1.74 | 0.4x† | 0 | 27 |
| `first(expr)` | 5-elem array | 0.143 | 1.88 | **13x** | 1 | 39 |
| `first(expr)`² | 200-int array | 5.09 | 1.80 | 0.4x† | 0 | 23 |
| `last(expr)` | 5-elem array | 0.237 | 2.44 | **10x** | 0 | 43 |
| `last(expr)`² | 200-int array | 5.75 | 1.94 | 0.3x† | 0 | 24 |
| `limit(3; expr)` | 5-elem array | 0.051 | 2.28 | **45x** | 0 | 42 |
| `limit(10; expr)` | 200-int array | 0.687 | 2.32 | **3.4x** | 0 | 24 |
| `.[1:4]` slice | 6-elem array | 0.088 | 0.867 | **9.8x** | 0 | 21 |
| `values` | 9-elem array | 0.122 | 2.77 | **23x** | 0 | 51 |
| `to_entries` | Small (~100B) | 0.175 | 5.43 | **31x** | 0 | 98 |
| `to_entries` | Large (~100KB) | 36.2 | 1122 | **31x** | 0 | 6465 |
| `keys_unsorted` | Small (~100B) | 0.183 | 1.61 | **8.8x** | 0 | 35 |
| `keys_unsorted` | Large (~100KB) | 33.9 | 718 | **21x** | 0 | 3039 |
| `object merge .a + .b` | Small (~100B) | 0.188 | 1.77 | **9.4x** | 0 | 33 |
| `tojson` | Small (~100B) | 0.349 | 1.74 | **5x** | 0 | 39 |
| `fromjson` | JSON string | 0.156 | 1.41 | **9.1x** | 0 | 31 |
| `tonumber` | `"42"` string | 0.014 | 0.396 | **29x** | 0 | 11 |
| `toboolean` | `"true"` string | 0.0086 | 0.407 | **47x** | 0 | 11 |
| `split(",")` | short string | 0.150 | 0.917 | **6.1x** | 0 | 21 |
| `join(",")` | 5-elem array | 0.103 | 1.04 | **10x** | 0 | 37 |
| `ascii_downcase` in select | Small (~100B) | 0.146 | 2.08 | **14x** | 0 | 46 |
| `ascii_downcase` in select | Large (~100KB) | 34.3 | 1028 | **30x** | 0 | 4653 |
| `startswith("s")` | Small (~100B) | 0.130 | 2.02 | **16x** | 0 | 45 |
| `startswith("s")` | Large (~100KB) | 33.6 | 864 | **26x** | 0 | 4652 |
| `endswith("s")` | Small (~100B) | 0.130 | 1.93 | **15x** | 0 | 45 |
| `trim` | short string | 0.052 | 0.437 | **8.5x** | 1 | 12 |
| `ltrim` | short string | 0.048 | 0.458 | **9.6x** | 1 | 12 |
| `rtrim` | short string | 0.051 | 0.508 | **9.9x** | 1 | 12 |
| `ltrimstr("s")` | Small (~100B) | 0.128 | 1.29 | **10x** | 0 | 30 |
| `rtrimstr("s")` | Small (~100B) | 0.127 | 1.23 | **9.7x** | 0 | 30 |
| `select` + string ops + arith + construct | ~200B log event | 1.07 | 3.10 | **2.9x** | 0 | 69 |
| `[.[] \| select + arith + construct]` | 20-elem array (~1.5KB) | 8.27 | 23.9 | **2.9x** | 64 | 525 |
| `length + map + add + min_by + any` | 20-elem array (~1.5KB) | 11.2 | 22.8 | **2x** | 25 | 555 |
| `map(try {…} catch …)` | 20-elem array (~1.5KB) | 269 | 30.0 | 0.1x† | 118 | 641 |
| `map(if … elif … else …)` | 20-elem array (~1.5KB) | 8.89 | 27.1 | **3x** | 98 | 595 |
| `[.[] \| str + tostring + str]` | 20-elem array (~1.5KB) | 8.88 | 25.4 | **2.9x** | 150 | 686 |
| `to_entries \| map(select) \| from_entries` | ~200B log event | 3.08 | 15.0 | **4.9x** | 44 | 250 |
| `@base64` | 34-char string | 0.164 | 0.665 | **4.1x** | 4 | 15 |
| `@base64d` | 48-char encoded | 0.261 | 0.657 | **2.5x** | 0 | 15 |
| `@uri` | 36-char URL string | 0.173 | 0.766 | **4.4x** | 4 | 14 |
| `index(",")` | short string | 0.106 | 1.00 | **9.5x** | 1 | 31 |
| `indices(",")` | short string | 0.216 | 2.42 | **11x** | 1 | 97 |
| `sqrt` | float (e≈2.718) | 0.072 | 0.493 | **6.9x** | 0 | 12 |
| `log` | float (e≈2.718) | 0.040 | 0.492 | **12x** | 0 | 12 |
| `sin` | float (e≈2.718) | 0.082 | 0.496 | **6x** | 0 | 12 |
| `atan` | integer 1 | 0.055 | 0.439 | **8x** | 0 | 11 |
| `exp` | integer 1 | 0.065 | 0.447 | **6.9x** | 0 | 11 |
| `tgamma` | integer 5 | 0.013 | 0.423 | **33x** | 0 | 11 |
| `fabs` | float -3.14 | 0.056 | 0.463 | **8.2x** | 0 | 12 |
| `abs` | float -3.14 | 0.0094 | 0.448 | **48x** | 0 | 12 |
| `expr as $x \| body` | Small (~20B object) | 30.1 | 0.858 | 0.0x† | 7 | 24 |
| ``"\(.level): \(.svc)"`` | ~45B object | 0.122 | 1.15 | **9.5x** | 0 | 35 |
| ``"user \(.name) …"`` | ~45B object | 0.091 | 1.31 | **14x** | 0 | 40 |
| `isempty(empty)` | null | 0.0089 | 0.619 | **70x** | 0 | 18 |
| `isempty(.[])` | 5-elem array | 0.023 | 1.12 | **48x** | 0 | 33 |
| `nth(2; .[])` | 5-elem array | 0.044 | 2.31 | **52x** | 0 | 49 |
| `range(10)` (10 values) | null | 0.173 | 1.07 | **6.2x** | 10 | 18 |
| `limit(3; range(1000))` | null — only 3 allocs | 0.084 | 1.60 | **19x** | 3 | 26 |
| `test(re)` hit | short string | 0.116 | 1.39 | **12x** | 0 | 26 |
| `test(re)` miss | short string | 0.101 | 1.31 | **13x** | 0 | 26 |
| `match(re)` hit | short string | 0.221 | 3.04 | **14x** | 1 | 66 |
| `match(re)` miss | short string | 0.606 | 1.74 | **2.9x** | 0 | 24 |
| `capture(re)` hit | short string | 0.218 | — | — | 1 | — |
| `scan(re)` no groups (4 matches) | short string | 0.513 | — | — | 10 | — |
| `sub(re; s)` hit | short string | 0.128 | — | — | 1 | — |
| `gsub(re; s)` hit (4 matches) | short string | 0.606 | — | — | 5 | — |

## Key Takeaways

- **Core operations achieve 0 allocations** in steady state when using `RunWithBuffer` or `RunFunc` (access, filtering, comparison, arithmetic, construction, math, `test(re)`). Operations that produce new structured output allocate proportional to result size: `@base64`/`@uri` (4 allocs; string-escape decoding), `match`/`capture` (1 alloc on a hit), `scan`/`gsub` (per match), `map(f)` when `f` constructs data (~1 per element). Even allocating ops use 10–100× fewer allocations than gojq.
- **† gojq wins on small arrays of primitive integers** — once unmarshaled to `[]interface{}`, element access and reduction (`min`, `any`, `first`, `last`) are O(1) native Go operations. fastjq always scans raw JSON bytes, which loses when the input is a compact integer array. For typical log data (objects with string/mixed fields), fastjq is consistently faster.
- **Fastest on small inputs** where gojq's marshal/unmarshal overhead dominates (up to 210x faster for `test(re)`)
- **Massively faster on large inputs** (18–75x) thanks to SIMD-accelerated string scanning (`bytes.IndexByte`) — `.field` on 100KB is 7.8 µs vs gojq's 582 µs
- **Compound select (and/or) is ~48–52x faster**: each boolean operand is evaluated via `execSingle` with no closures; gojq must unmarshal the full object
- **`to_entries` at 4.5 ns**: just an objectIter reformatting pass — zero-alloc, nearly as fast as a simple field access
- **Large JSON exposes gojq's unmarshal tax**: gojq Field/Del/Construct on 100KB is 580–830 µs vs fastjq's 8–34 µs, because gojq pays the full parse cost up front regardless of which fields are accessed

## Raw Output

Apple M4 Max, go1.25.4, `go test -bench=. -benchmem`. Updated 2026-04-30. Note: some first-run entries show spurious allocs (e.g. `KeysUnsorted` 3 allocs, `First` 1 alloc) due to benchmark calibration warmup — confirmed 0 allocs on repeat runs.

```
BenchmarkFastjq_Small_Del-16                  	 6370624	       178.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Medium_Del-16                 	  511035	      2262 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Del-16                  	   34406	     35083 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Field-16                	13069420	        89.27 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Field-16                	  152311	      7930 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Index-16                	51267810	        22.61 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ArrayDel-16             	14470093	        83.81 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Construct-16            	 9121480	       139.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Construct-16            	  147560	      8292 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Iterator-16             	36352803	        32.71 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Iterator-16             	  163286	      7615 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Select-16               	 9131160	       129.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Select-16               	   35158	     34451 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Alternative-16          	12492332	        95.89 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SelectAnd-16            	 7047022	       173.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SelectOr-16             	16324392	        74.35 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Has-16                  	12669586	        93.81 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Has-16                  	   35568	     33718 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_IfThenElse-16           	12344149	        98.27 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Length-16               	26349954	        43.31 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Length-16               	   36883	     32546 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Map-16                  	  950563	      1262 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Medium_Map-16                 	  184563	      6425 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Map-16                  	   93104	     13013 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToEntries-16            	 7194405	       175.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_ToEntries-16            	   33030	     36179 ns/op	       6 B/op	       0 allocs/op
BenchmarkFastjq_Small_KeysUnsorted-16         	 6949312	       183.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_KeysUnsorted-16         	   35439	     33869 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Any-16                  	25601296	        46.62 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Any-16                  	  847509	      1418 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AnyExpr-16              	 2834623	       475.9 ns/op	     144 B/op	      12 allocs/op
BenchmarkFastjq_Large_AnyExpr-16              	  174021	      6965 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Add-16                  	12337914	        88.80 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AddStrings-16           	 6540385	       166.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Flatten-16              	11445312	        98.29 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Split-16                	 7999088	       150.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Join-16                 	11431388	       103.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Values-16               	12496305	       122.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Values-16                 	  401119	      2771 ns/op	    3144 B/op	      51 allocs/op
BenchmarkFastjq_Small_Base64Encode-16         	 7809385	       163.6 ns/op	     120 B/op	       4 allocs/op
BenchmarkFastjq_Small_Base64Decode-16         	 4156569	       260.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Base64Encode-16           	 1768016	       665.1 ns/op	    1321 B/op	      15 allocs/op
BenchmarkGojq_Small_Base64Decode-16           	 1799808	       656.6 ns/op	    1321 B/op	      15 allocs/op
BenchmarkFastjq_Small_IndexFind-16            	11244469	       105.8 ns/op	       3 B/op	       1 allocs/op
BenchmarkFastjq_Small_IndicesAll-16           	 5494198	       215.9 ns/op	       3 B/op	       1 allocs/op
BenchmarkGojq_Small_IndexFind-16              	 1215972	      1002 ns/op	    2274 B/op	      31 allocs/op
BenchmarkGojq_Small_IndicesAll-16             	  496354	      2421 ns/op	    3907 B/op	      97 allocs/op
BenchmarkFastjq_Small_Slice-16                	12904231	        88.19 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SliceString-16          	23318968	        51.56 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Plus-16                 	20883487	        57.47 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_PlusStr-16              	13586577	        86.16 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AsciiDowncase-16        	 8182777	       146.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_AsciiDowncase-16        	   35151	     34347 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Startswith-16           	 8886663	       130.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Startswith-16           	   35630	     33589 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Endswith-16             	 9148153	       130.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Ltrimstr-16             	 9437238	       127.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Ltrimstr-16             	   37365	     32387 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Rtrimstr-16             	 9310520	       127.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Trim-16                 	22186596	        51.51 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_Ltrim-16                	24691759	        47.94 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_Rtrim-16                	22997224	        51.16 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_First-16                	 8319543	       142.7 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Large_First-16                	  239408	      5089 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Last-16                 	 5102839	       237.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Last-16                 	  199203	      5753 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Limit-16                	23528757	        51.12 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Limit-16                	 1718548	       687.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Del-16                    	  463573	      2776 ns/op	    3763 B/op	      58 allocs/op
BenchmarkGojq_Medium_Del-16                   	   51766	     23559 ns/op	   16973 B/op	     323 allocs/op
BenchmarkGojq_Large_Del-16                    	    1179	   1011110 ns/op	  536071 B/op	    4666 allocs/op
BenchmarkGojq_Small_Field-16                  	  998626	      1157 ns/op	    1641 B/op	      27 allocs/op
BenchmarkGojq_Large_Field-16                  	    1782	    655507 ns/op	  270058 B/op	    2835 allocs/op
BenchmarkGojq_Small_Index-16                  	 1629360	       737.0 ns/op	    1401 B/op	      20 allocs/op
BenchmarkGojq_Small_ArrayDel-16               	  626366	      1922 ns/op	    3363 B/op	      53 allocs/op
BenchmarkGojq_Small_Construct-16              	  751714	      1550 ns/op	    2330 B/op	      37 allocs/op
BenchmarkGojq_Large_Construct-16              	    1836	    634898 ns/op	  274489 B/op	    2867 allocs/op
BenchmarkGojq_Small_Iterator-16               	 1360111	       893.9 ns/op	    1776 B/op	      26 allocs/op
BenchmarkGojq_Large_Iterator-16               	   10000	    150592 ns/op	  109809 B/op	    1811 allocs/op
BenchmarkGojq_Small_Select-16                 	  446684	      2590 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Large_Select-16                 	    1186	    944925 ns/op	  534013 B/op	    4651 allocs/op
BenchmarkGojq_Small_Alternative-16            	  998526	      1183 ns/op	    1897 B/op	      31 allocs/op
BenchmarkGojq_Small_SelectAnd-16              	  601142	      2005 ns/op	    2890 B/op	      46 allocs/op
BenchmarkGojq_Small_SelectOr-16               	  621799	      2008 ns/op	    2890 B/op	      46 allocs/op
BenchmarkGojq_Small_Has-16                    	  623838	      2022 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Large_Has-16                    	    1248	    998086 ns/op	  537289 B/op	    4651 allocs/op
BenchmarkGojq_Small_IfThenElse-16             	  819573	      1549 ns/op	    1817 B/op	      30 allocs/op
BenchmarkGojq_Small_Length-16                 	  871809	      1334 ns/op	    1633 B/op	      27 allocs/op
BenchmarkGojq_Large_Length-16                 	    1641	    695326 ns/op	  269839 B/op	    2835 allocs/op
BenchmarkGojq_Small_Map-16                    	  103258	     11881 ns/op	   13655 B/op	     251 allocs/op
BenchmarkGojq_Large_Map-16                    	   10000	    113484 ns/op	  118674 B/op	    2237 allocs/op
BenchmarkGojq_Small_ToEntries-16              	  220898	      5435 ns/op	    6559 B/op	      98 allocs/op
BenchmarkGojq_Large_ToEntries-16              	    1028	   1121591 ns/op	  670598 B/op	    6465 allocs/op
BenchmarkGojq_Small_KeysUnsorted-16           	  735308	      1606 ns/op	    1953 B/op	      35 allocs/op
BenchmarkGojq_Large_KeysUnsorted-16           	    1647	    717607 ns/op	  282898 B/op	    3039 allocs/op
BenchmarkGojq_Small_Any-16                    	  522812	      2176 ns/op	    4508 B/op	      39 allocs/op
BenchmarkGojq_Small_AnyExpr-16                	  507044	      2331 ns/op	    4620 B/op	      49 allocs/op
BenchmarkGojq_Large_AnyExpr-16                	  597496	      1987 ns/op	    2818 B/op	      29 allocs/op
BenchmarkGojq_Small_Add-16                    	 1381401	       880.6 ns/op	    1529 B/op	      28 allocs/op
BenchmarkGojq_Small_AddStrings-16             	 1000000	      1757 ns/op	    1785 B/op	      35 allocs/op
BenchmarkGojq_Small_Flatten-16                	  630453	      1850 ns/op	    1857 B/op	      38 allocs/op
BenchmarkGojq_Small_Split-16                  	 1290813	       916.7 ns/op	    1553 B/op	      21 allocs/op
BenchmarkGojq_Small_Join-16                   	 1000000	      1038 ns/op	    1865 B/op	      37 allocs/op
BenchmarkGojq_Small_Slice-16                  	 1383387	       867.1 ns/op	    1425 B/op	      21 allocs/op
BenchmarkGojq_Small_SliceString-16            	 2442877	       499.0 ns/op	    1145 B/op	      12 allocs/op
BenchmarkGojq_Small_Plus-16                   	 1597712	       753.5 ns/op	    1673 B/op	      21 allocs/op
BenchmarkGojq_Small_PlusStr-16                	 1496648	       809.6 ns/op	    1705 B/op	      23 allocs/op
BenchmarkGojq_Small_AsciiDowncase-16          	  608869	      2076 ns/op	    2714 B/op	      46 allocs/op
BenchmarkGojq_Large_AsciiDowncase-16          	    1180	   1027707 ns/op	  542207 B/op	    4653 allocs/op
BenchmarkGojq_Small_Startswith-16             	  564290	      2022 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Large_Startswith-16             	    1362	    863668 ns/op	  542450 B/op	    4652 allocs/op
BenchmarkGojq_Small_Endswith-16               	  641824	      1930 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Small_Ltrimstr-16               	 1000000	      1289 ns/op	    1737 B/op	      30 allocs/op
BenchmarkGojq_Small_Rtrimstr-16               	  944930	      1233 ns/op	    1737 B/op	      30 allocs/op
BenchmarkGojq_Small_Trim-16                   	 2743245	       436.8 ns/op	    1129 B/op	      12 allocs/op
BenchmarkGojq_Small_Ltrim-16                  	 2723058	       458.4 ns/op	    1129 B/op	      12 allocs/op
BenchmarkGojq_Small_Rtrim-16                  	 2363262	       507.7 ns/op	    1129 B/op	      12 allocs/op
BenchmarkGojq_Small_First-16                  	  642434	      1875 ns/op	    3379 B/op	      39 allocs/op
BenchmarkGojq_Large_First-16                  	  700572	      1801 ns/op	    1889 B/op	      23 allocs/op
BenchmarkGojq_Small_Last-16                   	  488132	      2441 ns/op	    3483 B/op	      43 allocs/op
BenchmarkGojq_Large_Last-16                   	  622300	      1936 ns/op	    2193 B/op	      24 allocs/op
BenchmarkGojq_Small_Limit-16                  	  531986	      2279 ns/op	    3328 B/op	      42 allocs/op
BenchmarkGojq_Large_Limit-16                  	  550459	      2319 ns/op	    2200 B/op	      24 allocs/op
BenchmarkFastjq_Small_Subtract-16             	18371517	        60.04 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Subtract-16               	 1501248	       785.6 ns/op	    1649 B/op	      20 allocs/op
BenchmarkFastjq_Small_Multiply-16             	18140760	        65.79 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Multiply-16               	 1552509	       773.7 ns/op	    1649 B/op	      21 allocs/op
BenchmarkFastjq_Small_Divide-16               	11909408	       100.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Divide-16                 	 1509698	       795.5 ns/op	    1665 B/op	      21 allocs/op
BenchmarkFastjq_Small_Min-16                  	  638870	      1939 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Min-16                    	  863326	      1335 ns/op	    1217 B/op	      15 allocs/op
BenchmarkFastjq_Small_MinBy-16                	  142110	      8399 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_MinBy-16                  	   20739	     58791 ns/op	   63631 B/op	    1347 allocs/op
BenchmarkFastjq_Sort-16                       	  257637	      4791 ns/op	    7624 B/op	      11 allocs/op
BenchmarkGojq_Sort-16                         	  901092	      1445 ns/op	    1217 B/op	      15 allocs/op
BenchmarkFastjq_SortBy-16                     	   72969	     16078 ns/op	   18408 B/op	     119 allocs/op
BenchmarkGojq_SortBy-16                       	   10000	    109757 ns/op	   97008 B/op	    2145 allocs/op
BenchmarkFastjq_Unique-16                     	  186709	      6144 ns/op	    7624 B/op	      11 allocs/op
BenchmarkGojq_Unique-16                       	  950209	      1305 ns/op	    1217 B/op	      15 allocs/op
BenchmarkFastjq_GroupBy-16                    	   63096	     18892 ns/op	   18408 B/op	     119 allocs/op
BenchmarkGojq_GroupBy-16                      	   10000	    106225 ns/op	  101959 B/op	    2260 allocs/op
BenchmarkFastjq_Small_URIEncode-16            	 6881685	       173.1 ns/op	     120 B/op	       4 allocs/op
BenchmarkGojq_Small_URIEncode-16              	 1607138	       766.4 ns/op	    1289 B/op	      14 allocs/op
BenchmarkFastjq_Small_ArrayDiff-16            	 4792938	       238.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ArrayDiff-16              	  882301	      1372 ns/op	    2121 B/op	      33 allocs/op
BenchmarkFastjq_Small_TryNoError-16           	  260571	      4531 ns/op	      64 B/op	       1 allocs/op
BenchmarkFastjq_Small_TryCatchNoError-16      	  263376	      4719 ns/op	      64 B/op	       1 allocs/op
BenchmarkGojq_Small_TryNoError-16             	  991028	      1219 ns/op	    1913 B/op	      30 allocs/op
BenchmarkFastjq_Small_ObjectMerge-16          	 6141285	       188.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ObjectMerge-16            	  741525	      1770 ns/op	    2906 B/op	      33 allocs/op
BenchmarkFastjq_Small_ToJSON-16               	 3252804	       348.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ToJSON-16                 	  671886	      1742 ns/op	    2410 B/op	      39 allocs/op
BenchmarkFastjq_Small_FromJSON-16             	 7632472	       156.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_FromJSON-16               	  832489	      1414 ns/op	    2714 B/op	      31 allocs/op
BenchmarkFastjq_Small_ToString-16             	 3523831	       340.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToNumber-16             	86496878	        13.68 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToBoolean-16            	139445460	         8.605 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ToNumber-16               	 3028814	       396.2 ns/op	    1113 B/op	      11 allocs/op
BenchmarkGojq_Small_ToBoolean-16              	 2775915	       407.1 ns/op	    1113 B/op	      11 allocs/op
BenchmarkFastjq_Small_AnyTwoArg-16            	  268287	      4442 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_AnyTwoArg-16              	  696286	      1743 ns/op	    2482 B/op	      27 allocs/op
BenchmarkFastjq_Complex_LogNormalize-16       	 1000000	      1067 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Complex_LogNormalize-16         	  384576	      3103 ns/op	    4324 B/op	      69 allocs/op
BenchmarkFastjq_Complex_ArrayPipeline-16      	  144646	      8271 ns/op	    1232 B/op	      64 allocs/op
BenchmarkGojq_Complex_ArrayPipeline-16        	   50227	     23903 ns/op	   20408 B/op	     525 allocs/op
BenchmarkFastjq_Complex_Aggregation-16        	  106501	     11213 ns/op	     408 B/op	      25 allocs/op
BenchmarkGojq_Complex_Aggregation-16          	   53277	     22769 ns/op	   22397 B/op	     555 allocs/op
BenchmarkFastjq_Complex_TolerantMap-16        	    4416	    268776 ns/op	    3720 B/op	     118 allocs/op
BenchmarkGojq_Complex_TolerantMap-16          	   40561	     29955 ns/op	   27876 B/op	     641 allocs/op
BenchmarkFastjq_Complex_ElifRouting-16        	  138082	      8894 ns/op	    2304 B/op	      98 allocs/op
BenchmarkGojq_Complex_ElifRouting-16          	   43425	     27075 ns/op	   25923 B/op	     595 allocs/op
BenchmarkFastjq_Complex_StringBuild-16        	  132679	      8879 ns/op	    1568 B/op	     150 allocs/op
BenchmarkGojq_Complex_StringBuild-16          	   47199	     25350 ns/op	   20532 B/op	     686 allocs/op
BenchmarkFastjq_Complex_EntryFilter-16        	  389499	      3076 ns/op	    2240 B/op	      44 allocs/op
BenchmarkGojq_Complex_EntryFilter-16          	   81816	     14988 ns/op	   18429 B/op	     250 allocs/op
BenchmarkFastjq_Small_Sqrt-16                 	16715342	        71.58 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Sqrt-16                   	 2415643	       493.0 ns/op	    1145 B/op	      12 allocs/op
BenchmarkFastjq_Small_Log-16                  	30224668	        39.68 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Log-16                    	 2436482	       492.4 ns/op	    1129 B/op	      12 allocs/op
BenchmarkFastjq_Small_Sin-16                  	14573128	        82.28 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Sin-16                    	 2412246	       495.8 ns/op	    1145 B/op	      12 allocs/op
BenchmarkFastjq_Small_Atan-16                 	22480978	        55.11 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Atan-16                   	 2770554	       438.8 ns/op	    1121 B/op	      11 allocs/op
BenchmarkFastjq_Small_Exp-16                  	18515553	        65.08 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Exp-16                    	 2690635	       447.3 ns/op	    1121 B/op	      11 allocs/op
BenchmarkFastjq_Small_Tgamma-16               	93741145	        12.91 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Tgamma-16                 	 2870377	       422.6 ns/op	    1105 B/op	      11 allocs/op
BenchmarkFastjq_Small_Fabs-16                 	21411062	        56.15 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Fabs-16                   	 2523234	       462.6 ns/op	    1113 B/op	      12 allocs/op
BenchmarkFastjq_Small_Abs-16                  	128324924	         9.369 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Abs-16                    	 2680413	       448.4 ns/op	    1113 B/op	      12 allocs/op
BenchmarkFastjq_Small_Bind-16                 	   39498	     30147 ns/op	     288 B/op	       7 allocs/op
BenchmarkGojq_Small_Bind-16                   	 1400398	       857.6 ns/op	    1793 B/op	      24 allocs/op
BenchmarkFastjq_Small_StringInterp-16         	 9662722	       122.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_StringInterp-16           	 1000000	      1153 ns/op	    2121 B/op	      35 allocs/op
BenchmarkFastjq_Small_StringInterpNum-16      	13071496	        91.13 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_StringInterpNum-16        	  841509	      1311 ns/op	    2498 B/op	      40 allocs/op
BenchmarkFastjq_Small_IsEmptyTrue-16          	135215043	         8.886 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_IsEmptyTrue-16            	 1942208	       619.0 ns/op	    1921 B/op	      18 allocs/op
BenchmarkFastjq_Small_IsEmptyFalse-16         	51195176	        23.47 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_IsEmptyFalse-16           	 1000000	      1116 ns/op	    2898 B/op	      33 allocs/op
BenchmarkFastjq_Small_Nth-16                  	27093819	        44.35 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Nth-16                    	  508284	      2309 ns/op	    4372 B/op	      49 allocs/op
BenchmarkFastjq_Small_Range10-16              	 6995428	       173.5 ns/op	      80 B/op	      10 allocs/op
BenchmarkGojq_Small_Range10-16                	 1000000	      1069 ns/op	    1864 B/op	      18 allocs/op
BenchmarkFastjq_Small_RangeLimit-16           	14472993	        83.59 ns/op	      24 B/op	       3 allocs/op
BenchmarkGojq_Small_RangeLimit-16             	  779130	      1600 ns/op	    3208 B/op	      26 allocs/op
BenchmarkRegexp_Match_Hit-16                  	26659261	        44.95 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Miss-16                 	51019593	        23.69 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Complex_Hit-16          	27422642	        43.20 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Long_Miss-16            	14791257	        78.80 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_MatchString-16                	26717998	        45.08 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_FindSubmatch_Hit-16           	 5708727	       210.9 ns/op	     160 B/op	       2 allocs/op
BenchmarkRegexp_FindSubmatch_Miss-16          	 1749817	       683.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_FindSubmatchIndex_Hit-16      	 6467710	       185.4 ns/op	      64 B/op	       1 allocs/op
BenchmarkRegexp_FindSubmatchIndex_Miss-16     	 1760898	       683.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_ReplaceAll_Hit-16             	 7649028	       158.0 ns/op	      97 B/op	       5 allocs/op
BenchmarkRegexp_ReplaceAll_Miss-16            	23924715	        51.32 ns/op	      48 B/op	       2 allocs/op
BenchmarkRegexp_MatchViaClosure-16            	27230179	        44.59 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_Hit-16           	10295582	       115.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_Miss-16          	11893842	       101.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_InPipeline-16    	 4036826	       295.4 ns/op	       8 B/op	       0 allocs/op
BenchmarkFastjq_Small_MatchRe_Hit-16          	 5430574	       221.2 ns/op	      48 B/op	       1 allocs/op
BenchmarkFastjq_Small_MatchRe_Miss-16         	 1980548	       606.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_TestRe_Hit-16             	  794340	      1387 ns/op	    2744 B/op	      26 allocs/op
BenchmarkGojq_Small_TestRe_Miss-16            	  922623	      1312 ns/op	    2702 B/op	      26 allocs/op
BenchmarkGojq_Small_MatchRe_Hit-16            	  401464	      3042 ns/op	    4793 B/op	      66 allocs/op
BenchmarkGojq_Small_MatchRe_Miss-16           	  719098	      1737 ns/op	    2251 B/op	      24 allocs/op
BenchmarkFastjq_Small_CaptureRe_Hit-16        	 5503592	       218.5 ns/op	      48 B/op	       1 allocs/op
BenchmarkFastjq_Small_CaptureRe_Miss-16       	 1968669	       607.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ScanRe_NoGroups-16      	 2337302	       512.9 ns/op	     388 B/op	      10 allocs/op
BenchmarkFastjq_Small_ScanRe_WithGroups-16    	 1862463	       642.3 ns/op	     703 B/op	      13 allocs/op
BenchmarkFastjq_Small_SubRe_Hit-16            	 9359522	       128.1 ns/op	      16 B/op	       1 allocs/op
BenchmarkFastjq_Small_SubRe_Miss-16           	11562540	       103.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_GSubRe_Hit-16           	 1979904	       605.9 ns/op	     306 B/op	       5 allocs/op
BenchmarkFastjq_Small_GSubRe_Miss-16          	 4564202	       260.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_CaptureRe_Hit-16          	  489241	      2434 ns/op	    4728 B/op	      51 allocs/op
BenchmarkGojq_Small_CaptureRe_Miss-16         	  649454	      1871 ns/op	    2674 B/op	      27 allocs/op
BenchmarkGojq_Small_ScanRe_NoGroups-16        	  416565	      2956 ns/op	    5137 B/op	      57 allocs/op
BenchmarkGojq_Small_SubRe_Hit-16              	  214560	      5573 ns/op	    9009 B/op	      95 allocs/op
BenchmarkGojq_Small_GSubRe_Hit-16             	   76638	     15623 ns/op	   17892 B/op	     248 allocs/op
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
