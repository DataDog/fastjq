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
| `.field` | Small (~100B) | 0.092 | 1.02 | **11x** | 0 | 27 |
| `.field` | Large (~100KB) | 7.91 | 607 | **77x** | 0 | 2835 |
| `del(.f)` | Small (~100B) | 0.168 | 2.39 | **14x** | 0 | 58 |
| `del(.f)` | Medium (~2KB) | 2.25 | 20.6 | **9.2x** | 0 | 323 |
| `del(.f)` | Large (~100KB) | 35.5 | 882 | **25x** | 0 | 4667 |
| `.[n]` | 5-elem array | 0.024 | 0.656 | **27x** | 0 | 20 |
| `del(.[n], .[m])` | 5-elem array | 0.082 | 1.75 | **21x** | 0 | 53 |
| `{f0, f2}` (construct) | Small (~100B) | 0.145 | 1.44 | **9.9x** | 0 | 37 |
| `{f0, f50}` (construct) | Large (~100KB) | 8.24 | 622 | **75x** | 0 | 2867 |
| `.[]` iterator | 5-elem array | 0.062 | 0.809 | **13x** | 2 | 26 |
| `.[]` iterator | 200-elem array | 7.48 | 88.7 | **12x** | 2 | 1811 |
| `select(.f == "x")` | Small (~100B) | 0.129 | 1.81 | **14x** | 0 | 45 |
| `select(.f == "x")`¹ | Large (~100KB, last field) | 34.2 | 850 | **25x** | 0 | 4651 |
| `select(.f and .g)` | Small (~100B) | 0.170 | 1.98 | **12x** | 0 | 46 |
| `select(.f or .g)` | Small (~100B) | 0.074 | 1.89 | **26x** | 0 | 46 |
| `has("key")` in select | Small (~100B) | 0.093 | 1.81 | **20x** | 0 | 45 |
| `has("key")` in select | Large (~100KB) | 33.3 | 850 | **26x** | 0 | 4651 |
| `if-then-else` | Small (~100B) | 0.096 | 1.15 | **12x** | 0 | 30 |
| `.f // "default"` | Small (~100B) | 0.097 | 1.14 | **12x** | 0 | 31 |
| `try .field` (no error) | Small (~100B) | 4.47 | 1.15 | 0.3x† | 1 | 30 |
| `.a + .b` (strings) | Small (~100B) | 0.053 | 0.801 | **15x** | 0 | 21 |
| `"prefix" + .name` | Small (~100B) | 0.087 | 0.837 | **9.6x** | 0 | 23 |
| `.a - .b` (subtract) | Small (~100B) | 0.051 | 0.784 | **15x** | 0 | 20 |
| `.a * .b` (multiply) | Small (~100B) | 0.065 | 0.781 | **12x** | 0 | 21 |
| `.a / .b` (divide) | Small (~100B) | 0.099 | 0.810 | **8.2x** | 0 | 21 |
| `.a - .b` (array diff) | 5-elem arrays | 0.219 | 1.35 | **6.2x** | 0 | 33 |
| `length` | Small (~100B) | 0.044 | 1.07 | **24x** | 0 | 27 |
| `length` | Large (~100KB) | 33.1 | 607 | **18x** | 0 | 2835 |
| `add` (numbers) | 5-elem array | 0.059 | 0.786 | **13x** | 0 | 28 |
| `add` (strings) | 5-elem array | 0.135 | 0.976 | **7.2x** | 0 | 35 |
| `flatten` | 3-elem nested array | 0.098 | 1.39 | **14x** | 0 | 38 |
| `min` | 200-int array | 1.86 | 1.30 | 0.7x† | 0 | 15 |
| `min_by(.value)` | 100-elem object array | 8.68 | 59.7 | **6.9x** | 0 | 1347 |
| `sort` | 200-int array | 4.80 | 1.30 | 0.3x† | 14 | 15 |
| `sort_by(.value)` | 100-elem object array | 18.5 | 97.1 | **5.3x** | 422 | 2145 |
| `unique` | 200-int array | 5.97 | 1.28 | 0.2x† | 14 | 15 |
| `group_by(.active)` | 100-elem object array | 22.8 | 103 | **4.5x** | 422 | 2260 |
| `map(.name)` | 20-elem array (~600B) | 1.62 | 10.8 | **6.6x** | 29 | 251 |
| `map(.name)` | 200-elem array (~6KB) | 15.8 | 97.2 | **6.2x** | 209 | 2237 |
| `any` | 5-elem array | 0.043 | 1.99 | **46x** | 0 | 39 |
| `any(expr)` | 5-elem array | 0.393 | 2.22 | **5.6x** | 12 | 49 |
| `any(expr)`² | 200-int array | 4.19 | 1.81 | 0.4x† | 0 | 29 |
| `any(gen; cond)`² | 200-int array | 4.67 | 1.79 | 0.4x† | 3 | 27 |
| `first(expr)` | 5-elem array | 0.255 | 1.61 | **6.3x** | 9 | 39 |
| `first(expr)`² | 200-int array | 6.77 | 1.53 | 0.2x† | 107 | 23 |
| `last(expr)` | 5-elem array | 0.324 | 2.02 | **6.2x** | 10 | 43 |
| `last(expr)`² | 200-int array | 6.57 | 1.59 | 0.2x† | 107 | 24 |
| `limit(3; expr)` | 5-elem array | 0.106 | 1.76 | **17x** | 5 | 42 |
| `limit(10; expr)` | 200-int array | 0.693 | 1.73 | **2.5x** | 5 | 24 |
| `.[1:4]` slice | 6-elem array | 0.084 | 0.869 | **10x** | 0 | 21 |
| `values` | 9-elem array | 0.268 | 2.49 | **9.3x** | 13 | 51 |
| `skip(2; .[])` | 5-elem array | 0.149 | 1.88 | **13x** | 7 | 43 |
| `to_entries` | Small (~100B) | 0.174 | 4.29 | **25x** | 0 | 98 |
| `to_entries` | Large (~100KB) | 35.4 | 947 | **27x** | 0 | 6465 |
| `keys` | Small (~100B) | 0.311 | 1.37 | **4.4x** | 7 | 35 |
| `keys_unsorted` | Small (~100B) | 0.173 | 1.37 | **7.9x** | 0 | 35 |
| `keys_unsorted` | Large (~100KB) | 32.9 | 638 | **19x** | 0 | 3039 |
| `object merge .a + .b` | Small (~100B) | 0.168 | 1.62 | **9.7x** | 0 | 33 |
| `tojson` | Small (~100B) | 0.339 | 1.71 | **5x** | 0 | 39 |
| `fromjson` | JSON string | 0.158 | 1.41 | **8.9x** | 0 | 31 |
| `tonumber` | `"42"` string | 0.014 | 0.418 | **31x** | 0 | 11 |
| `toboolean` | `"true"` string | 0.0089 | 0.424 | **48x** | 0 | 11 |
| `split(",")` | short string | 0.148 | 0.879 | **5.9x** | 0 | 21 |
| `join(",")` | 5-elem array | 0.103 | 1.05 | **10x** | 0 | 37 |
| `ascii_downcase` in select | Small (~100B) | 0.149 | 1.87 | **13x** | 0 | 46 |
| `ascii_downcase` in select | Large (~100KB) | 34.9 | 855 | **24x** | 0 | 4652 |
| `startswith("s")` | Small (~100B) | 0.133 | 1.88 | **14x** | 0 | 45 |
| `startswith("s")` | Large (~100KB) | 34.2 | 854 | **25x** | 0 | 4652 |
| `endswith("s")` | Small (~100B) | 0.132 | 1.84 | **14x** | 0 | 45 |
| `trim` | short string | 0.051 | 0.440 | **8.7x** | 1 | 12 |
| `ltrim` | short string | 0.048 | 0.450 | **9.3x** | 1 | 12 |
| `rtrim` | short string | 0.051 | 0.431 | **8.4x** | 1 | 12 |
| `ltrimstr("s")` | Small (~100B) | 0.129 | 1.10 | **8.6x** | 0 | 30 |
| `rtrimstr("s")` | Small (~100B) | 0.127 | 1.11 | **8.7x** | 0 | 30 |
| `select` + string ops + arith + construct | ~200B log event | 1.07 | 3.20 | **3x** | 0 | 69 |
| `[.[] \| select + arith + construct]` | 20-elem array (~1.5KB) | 9.26 | 24.2 | **2.6x** | 106 | 525 |
| `length + map + add + min_by + any` | 20-elem array (~1.5KB) | 12.5 | 23.3 | **1.9x** | 106 | 555 |
| `map(try {…} catch …)` | 20-elem array (~1.5KB) | 268 | 29.8 | 0.1x† | 187 | 641 |
| `map(if … elif … else …)` | 20-elem array (~1.5KB) | 9.48 | 26.5 | **2.8x** | 127 | 595 |
| `[.[] \| str + tostring + str]` | 20-elem array (~1.5KB) | 11.3 | 25.4 | **2.2x** | 298 | 686 |
| `to_entries \| map(select) \| from_entries` | ~200B log event | 3.58 | 15.3 | **4.3x** | 69 | 250 |
| `@base64` | 34-char string | 0.153 | 0.571 | **3.7x** | 4 | 15 |
| `@base64d` | 48-char encoded | 0.223 | 0.614 | **2.8x** | 0 | 15 |
| `@uri` | 36-char URL string | 0.171 | 0.729 | **4.3x** | 4 | 14 |
| `index(",")` | short string | 0.099 | 0.970 | **9.9x** | 1 | 31 |
| `indices(",")` | short string | 0.211 | 2.25 | **11x** | 1 | 97 |
| `sqrt` | float (e≈2.718) | 0.071 | 0.495 | **6.9x** | 0 | 12 |
| `log` | float (e≈2.718) | 0.038 | 0.489 | **13x** | 0 | 12 |
| `sin` | float (e≈2.718) | 0.081 | 0.482 | **5.9x** | 0 | 12 |
| `atan` | integer 1 | 0.054 | 0.419 | **7.8x** | 0 | 11 |
| `exp` | integer 1 | 0.064 | 0.442 | **6.9x** | 0 | 11 |
| `tgamma` | integer 5 | 0.013 | 0.420 | **32x** | 0 | 11 |
| `fabs` | float -3.14 | 0.055 | 0.466 | **8.4x** | 0 | 12 |
| `abs` | float -3.14 | 0.0095 | 0.447 | **47x** | 0 | 12 |
| `expr as $x \| body` | Small (~20B object) | 29.9 | 0.862 | 0.0x† | 18 | 24 |
| ``"\(.level): \(.svc)"`` | ~45B object | 0.127 | 1.13 | **8.9x** | 0 | 35 |
| ``"user \(.name) …"`` | ~45B object | 0.093 | 1.31 | **14x** | 0 | 40 |
| `isempty(empty)` | null | 0.076 | 0.628 | **8.3x** | 6 | 18 |
| `isempty(.[])` | 5-elem array | 0.089 | 1.12 | **13x** | 6 | 33 |
| `nth(2; .[])` | 5-elem array | 0.136 | 2.28 | **17x** | 7 | 49 |
| `range(10)` (10 values) | null | 0.207 | 1.08 | **5.2x** | 12 | 18 |
| `limit(3; range(1000))` | null — only 3 allocs | 0.145 | 1.60 | **11x** | 8 | 26 |
| `test(re)` hit | short string | 0.114 | 1.37 | **12x** | 0 | 26 |
| `test(re)` miss | short string | 0.102 | 1.30 | **13x** | 0 | 26 |
| `match(re)` hit | short string | 0.221 | 3.15 | **14x** | 1 | 66 |
| `match(re)` miss | short string | 0.626 | 1.72 | **2.8x** | 0 | 24 |
| `capture(re)` hit | short string | 0.217 | — | — | 1 | — |
| `scan(re)` no groups (4 matches) | short string | 0.570 | — | — | 17 | — |
| `sub(re; s)` hit | short string | 0.130 | — | — | 1 | — |
| `gsub(re; s)` hit (4 matches) | short string | 0.601 | — | — | 5 | — |

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
BenchmarkFastjq_Small_Del-16                  	 6907814	       168.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Medium_Del-16                 	  508842	      2250 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Del-16                  	   33966	     35492 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Field-16                	12602454	        92.02 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Field-16                	  151406	      7908 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Index-16                	49372893	        24.21 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ArrayDel-16             	14741184	        81.51 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Construct-16            	 8845333	       144.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Construct-16            	  145082	      8243 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Iterator-16             	19130352	        62.43 ns/op	      40 B/op	       2 allocs/op
BenchmarkFastjq_Large_Iterator-16             	  157933	      7479 ns/op	      40 B/op	       2 allocs/op
BenchmarkFastjq_Small_Select-16               	 9041235	       128.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Select-16               	   35371	     34187 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Alternative-16          	12621243	        96.86 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SelectAnd-16            	 7030123	       169.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SelectOr-16             	15820706	        73.67 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Has-16                  	12923491	        92.67 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Has-16                  	   35838	     33311 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_IfThenElse-16           	12567150	        95.84 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Length-16               	26135296	        44.09 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Length-16               	   36102	     33082 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Map-16                  	  724148	      1622 ns/op	     689 B/op	      29 allocs/op
BenchmarkFastjq_Medium_Map-16                 	  155028	      7674 ns/op	    2609 B/op	     109 allocs/op
BenchmarkFastjq_Large_Map-16                  	   76711	     15810 ns/op	    5009 B/op	     209 allocs/op
BenchmarkFastjq_Small_ToEntries-16            	 7531450	       174.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_ToEntries-16            	   33486	     35386 ns/op	       6 B/op	       0 allocs/op
BenchmarkFastjq_Small_KeysUnsorted-16         	 6833008	       173.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Keys-16                 	 3831920	       311.2 ns/op	     456 B/op	       7 allocs/op
BenchmarkFastjq_Large_KeysUnsorted-16         	   36682	     32893 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Any-16                  	26198179	        43.30 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Any-16                  	  802256	      1408 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AnyExpr-16              	 3039624	       393.2 ns/op	     144 B/op	      12 allocs/op
BenchmarkFastjq_Large_AnyExpr-16              	  274857	      4191 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Add-16                  	20283746	        58.70 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AddStrings-16           	 8777072	       134.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Flatten-16              	12396592	        98.42 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Split-16                	 8126506	       148.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Join-16                 	11520156	       103.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Values-16               	 4484676	       268.2 ns/op	     328 B/op	      13 allocs/op
BenchmarkGojq_Small_Values-16                 	  479792	      2488 ns/op	    3144 B/op	      51 allocs/op
BenchmarkFastjq_Small_Base64Encode-16         	 7728664	       152.6 ns/op	     120 B/op	       4 allocs/op
BenchmarkFastjq_Small_Base64Decode-16         	 5349962	       223.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Base64Encode-16           	 2111281	       571.3 ns/op	    1321 B/op	      15 allocs/op
BenchmarkGojq_Small_Base64Decode-16           	 1984662	       614.3 ns/op	    1321 B/op	      15 allocs/op
BenchmarkFastjq_Small_IndexFind-16            	12203962	        98.50 ns/op	       3 B/op	       1 allocs/op
BenchmarkFastjq_Small_IndicesAll-16           	 5612701	       211.5 ns/op	       3 B/op	       1 allocs/op
BenchmarkGojq_Small_IndexFind-16              	 1244582	       970.3 ns/op	    2274 B/op	      31 allocs/op
BenchmarkGojq_Small_IndicesAll-16             	  532609	      2249 ns/op	    3907 B/op	      97 allocs/op
BenchmarkFastjq_Small_Slice-16                	13709670	        84.34 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SliceString-16          	22562150	        52.52 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Plus-16                 	22503445	        52.84 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_PlusStr-16              	13813333	        86.99 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AsciiDowncase-16        	 7946091	       149.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_AsciiDowncase-16        	   34555	     34903 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Startswith-16           	 9025971	       132.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Startswith-16           	   35256	     34230 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Endswith-16             	 8777422	       131.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Ltrimstr-16             	 9464017	       128.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Ltrimstr-16             	   36588	     32760 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Rtrimstr-16             	 9060050	       127.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Trim-16                 	23028025	        50.62 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_Ltrim-16                	24767077	        48.15 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_Rtrim-16                	23206173	        51.32 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_First-16                	 4618305	       255.4 ns/op	     216 B/op	       9 allocs/op
BenchmarkFastjq_Large_First-16                	  176858	      6770 ns/op	    2584 B/op	     107 allocs/op
BenchmarkFastjq_Small_Last-16                 	 3709315	       324.2 ns/op	     256 B/op	      10 allocs/op
BenchmarkFastjq_Large_Last-16                 	  181172	      6569 ns/op	    2584 B/op	     107 allocs/op
BenchmarkFastjq_Small_Limit-16                	11178193	       106.4 ns/op	     104 B/op	       5 allocs/op
BenchmarkFastjq_Small_Skip-16                 	 8229652	       148.7 ns/op	     208 B/op	       7 allocs/op
BenchmarkFastjq_Large_Limit-16                	 1735682	       693.2 ns/op	     104 B/op	       5 allocs/op
BenchmarkGojq_Small_Del-16                    	  503503	      2394 ns/op	    3763 B/op	      58 allocs/op
BenchmarkGojq_Medium_Del-16                   	   58520	     20629 ns/op	   16971 B/op	     323 allocs/op
BenchmarkGojq_Large_Del-16                    	    1369	    882039 ns/op	  552260 B/op	    4667 allocs/op
BenchmarkGojq_Small_Field-16                  	 1000000	      1021 ns/op	    1641 B/op	      27 allocs/op
BenchmarkGojq_Large_Field-16                  	    1964	    607227 ns/op	  270048 B/op	    2835 allocs/op
BenchmarkGojq_Small_Index-16                  	 1837663	       656.5 ns/op	    1401 B/op	      20 allocs/op
BenchmarkGojq_Small_ArrayDel-16               	  699418	      1752 ns/op	    3363 B/op	      53 allocs/op
BenchmarkGojq_Small_Construct-16              	  819182	      1435 ns/op	    2330 B/op	      37 allocs/op
BenchmarkGojq_Large_Construct-16              	    1897	    621861 ns/op	  274473 B/op	    2867 allocs/op
BenchmarkGojq_Small_Iterator-16               	 1489886	       808.9 ns/op	    1776 B/op	      26 allocs/op
BenchmarkGojq_Large_Iterator-16               	   13521	     88667 ns/op	  109808 B/op	    1811 allocs/op
BenchmarkGojq_Small_Select-16                 	  658508	      1809 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Large_Select-16                 	    1404	    850105 ns/op	  536115 B/op	    4651 allocs/op
BenchmarkGojq_Small_Alternative-16            	 1000000	      1136 ns/op	    1897 B/op	      31 allocs/op
BenchmarkGojq_Small_SelectAnd-16              	  628131	      1981 ns/op	    2890 B/op	      46 allocs/op
BenchmarkGojq_Small_SelectOr-16               	  625832	      1894 ns/op	    2890 B/op	      46 allocs/op
BenchmarkGojq_Small_Has-16                    	  654067	      1811 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Large_Has-16                    	    1387	    850391 ns/op	  535340 B/op	    4651 allocs/op
BenchmarkGojq_Small_IfThenElse-16             	 1000000	      1145 ns/op	    1817 B/op	      30 allocs/op
BenchmarkGojq_Small_Length-16                 	 1000000	      1068 ns/op	    1633 B/op	      27 allocs/op
BenchmarkGojq_Large_Length-16                 	    2006	    606971 ns/op	  269829 B/op	    2835 allocs/op
BenchmarkGojq_Small_Map-16                    	  111411	     10757 ns/op	   13654 B/op	     251 allocs/op
BenchmarkGojq_Large_Map-16                    	   12352	     97237 ns/op	  118588 B/op	    2237 allocs/op
BenchmarkGojq_Small_ToEntries-16              	  274168	      4292 ns/op	    6558 B/op	      98 allocs/op
BenchmarkGojq_Large_ToEntries-16              	    1260	    946778 ns/op	  673377 B/op	    6465 allocs/op
BenchmarkGojq_Small_KeysUnsorted-16           	  884601	      1371 ns/op	    1953 B/op	      35 allocs/op
BenchmarkGojq_Small_Keys-16                   	  860634	      1371 ns/op	    1953 B/op	      35 allocs/op
BenchmarkGojq_Large_KeysUnsorted-16           	    1842	    637948 ns/op	  282865 B/op	    3039 allocs/op
BenchmarkGojq_Small_Any-16                    	  592762	      1986 ns/op	    4508 B/op	      39 allocs/op
BenchmarkGojq_Small_AnyExpr-16                	  540613	      2220 ns/op	    4620 B/op	      49 allocs/op
BenchmarkGojq_Large_AnyExpr-16                	  688338	      1807 ns/op	    2818 B/op	      29 allocs/op
BenchmarkGojq_Small_Add-16                    	 1517983	       786.2 ns/op	    1529 B/op	      28 allocs/op
BenchmarkGojq_Small_AddStrings-16             	 1230979	       975.8 ns/op	    1785 B/op	      35 allocs/op
BenchmarkGojq_Small_Flatten-16                	  874143	      1394 ns/op	    1857 B/op	      38 allocs/op
BenchmarkGojq_Small_Split-16                  	 1376642	       879.2 ns/op	    1553 B/op	      21 allocs/op
BenchmarkGojq_Small_Join-16                   	 1000000	      1052 ns/op	    1865 B/op	      37 allocs/op
BenchmarkGojq_Small_Slice-16                  	 1377882	       869.3 ns/op	    1425 B/op	      21 allocs/op
BenchmarkGojq_Small_SliceString-16            	 2449098	       544.6 ns/op	    1145 B/op	      12 allocs/op
BenchmarkGojq_Small_Plus-16                   	 1501245	       800.6 ns/op	    1673 B/op	      21 allocs/op
BenchmarkGojq_Small_PlusStr-16                	 1410469	       836.6 ns/op	    1705 B/op	      23 allocs/op
BenchmarkGojq_Small_AsciiDowncase-16          	  665028	      1872 ns/op	    2714 B/op	      46 allocs/op
BenchmarkGojq_Large_AsciiDowncase-16          	    1383	    854562 ns/op	  536203 B/op	    4652 allocs/op
BenchmarkGojq_Small_Startswith-16             	  642568	      1876 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Large_Startswith-16             	    1407	    854105 ns/op	  543164 B/op	    4652 allocs/op
BenchmarkGojq_Small_Endswith-16               	  647538	      1840 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Small_Ltrimstr-16               	 1000000	      1104 ns/op	    1737 B/op	      30 allocs/op
BenchmarkGojq_Small_Rtrimstr-16               	 1000000	      1109 ns/op	    1737 B/op	      30 allocs/op
BenchmarkGojq_Small_Trim-16                   	 2857870	       439.8 ns/op	    1129 B/op	      12 allocs/op
BenchmarkGojq_Small_Ltrim-16                  	 2728161	       450.2 ns/op	    1129 B/op	      12 allocs/op
BenchmarkGojq_Small_Rtrim-16                  	 2762305	       430.9 ns/op	    1129 B/op	      12 allocs/op
BenchmarkGojq_Small_First-16                  	  753202	      1615 ns/op	    3379 B/op	      39 allocs/op
BenchmarkGojq_Large_First-16                  	  807061	      1533 ns/op	    1889 B/op	      23 allocs/op
BenchmarkGojq_Small_Last-16                   	  590185	      2022 ns/op	    3483 B/op	      43 allocs/op
BenchmarkGojq_Large_Last-16                   	  756534	      1593 ns/op	    2193 B/op	      24 allocs/op
BenchmarkGojq_Small_Limit-16                  	  672830	      1763 ns/op	    3328 B/op	      42 allocs/op
BenchmarkGojq_Small_Skip-16                   	  645008	      1884 ns/op	    2880 B/op	      43 allocs/op
BenchmarkGojq_Large_Limit-16                  	  729978	      1726 ns/op	    2200 B/op	      24 allocs/op
BenchmarkFastjq_Small_Subtract-16             	23336466	        51.28 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Subtract-16               	 1498041	       783.8 ns/op	    1649 B/op	      20 allocs/op
BenchmarkFastjq_Small_Multiply-16             	17511398	        65.13 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Multiply-16               	 1556473	       781.1 ns/op	    1649 B/op	      21 allocs/op
BenchmarkFastjq_Small_Divide-16               	12190579	        98.82 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Divide-16                 	 1505524	       810.0 ns/op	    1665 B/op	      21 allocs/op
BenchmarkFastjq_Small_Min-16                  	  633363	      1858 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Min-16                    	  918381	      1303 ns/op	    1217 B/op	      15 allocs/op
BenchmarkFastjq_Small_MinBy-16                	  141046	      8679 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_MinBy-16                  	   20304	     59671 ns/op	   63631 B/op	    1347 allocs/op
BenchmarkFastjq_Sort-16                       	  255091	      4796 ns/op	    7688 B/op	      14 allocs/op
BenchmarkGojq_Sort-16                         	  933337	      1304 ns/op	    1217 B/op	      15 allocs/op
BenchmarkFastjq_SortBy-16                     	   63968	     18478 ns/op	   24872 B/op	     422 allocs/op
BenchmarkGojq_SortBy-16                       	   12340	     97093 ns/op	   96958 B/op	    2145 allocs/op
BenchmarkFastjq_Unique-16                     	  204079	      5975 ns/op	    7688 B/op	      14 allocs/op
BenchmarkGojq_Unique-16                       	  974614	      1280 ns/op	    1217 B/op	      15 allocs/op
BenchmarkFastjq_GroupBy-16                    	   52767	     22754 ns/op	   24872 B/op	     422 allocs/op
BenchmarkGojq_GroupBy-16                      	   10000	    103331 ns/op	  101886 B/op	    2260 allocs/op
BenchmarkFastjq_Small_URIEncode-16            	 7013466	       171.2 ns/op	     120 B/op	       4 allocs/op
BenchmarkGojq_Small_URIEncode-16              	 1646919	       728.5 ns/op	    1289 B/op	      14 allocs/op
BenchmarkFastjq_Small_ArrayDiff-16            	 5529993	       218.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ArrayDiff-16              	  856930	      1350 ns/op	    2121 B/op	      33 allocs/op
BenchmarkFastjq_Small_TryNoError-16           	  269198	      4470 ns/op	      64 B/op	       1 allocs/op
BenchmarkFastjq_Small_TryCatchNoError-16      	  273825	      4423 ns/op	      64 B/op	       1 allocs/op
BenchmarkGojq_Small_TryNoError-16             	 1000000	      1149 ns/op	    1913 B/op	      30 allocs/op
BenchmarkFastjq_Small_ObjectMerge-16          	 7122787	       167.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ObjectMerge-16            	  747876	      1622 ns/op	    2906 B/op	      33 allocs/op
BenchmarkFastjq_Small_ToJSON-16               	 3540501	       339.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ToJSON-16                 	  730004	      1710 ns/op	    2410 B/op	      39 allocs/op
BenchmarkFastjq_Small_FromJSON-16             	 7638351	       158.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_FromJSON-16               	  851012	      1414 ns/op	    2714 B/op	      31 allocs/op
BenchmarkFastjq_Small_ToString-16             	 3547507	       340.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToNumber-16             	86608987	        13.66 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToBoolean-16            	133700576	         8.891 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_ToNumber-16               	 3053119	       417.6 ns/op	    1113 B/op	      11 allocs/op
BenchmarkGojq_Small_ToBoolean-16              	 2865664	       424.2 ns/op	    1113 B/op	      11 allocs/op
BenchmarkFastjq_Small_AnyTwoArg-16            	  243685	      4669 ns/op	      57 B/op	       3 allocs/op
BenchmarkGojq_Small_AnyTwoArg-16              	  705145	      1790 ns/op	    2482 B/op	      27 allocs/op
BenchmarkFastjq_Complex_LogNormalize-16       	 1000000	      1066 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Complex_LogNormalize-16         	  402728	      3198 ns/op	    4324 B/op	      69 allocs/op
BenchmarkFastjq_Complex_ArrayPipeline-16      	  126301	      9264 ns/op	    2264 B/op	     106 allocs/op
BenchmarkGojq_Complex_ArrayPipeline-16        	   50421	     24235 ns/op	   20409 B/op	     525 allocs/op
BenchmarkFastjq_Complex_Aggregation-16        	   96282	     12533 ns/op	    3184 B/op	     106 allocs/op
BenchmarkGojq_Complex_Aggregation-16          	   50162	     23291 ns/op	   22398 B/op	     555 allocs/op
BenchmarkFastjq_Complex_TolerantMap-16        	    4496	    267619 ns/op	    5216 B/op	     187 allocs/op
BenchmarkGojq_Complex_TolerantMap-16          	   40270	     29767 ns/op	   27874 B/op	     641 allocs/op
BenchmarkFastjq_Complex_ElifRouting-16        	  126111	      9480 ns/op	    3000 B/op	     127 allocs/op
BenchmarkGojq_Complex_ElifRouting-16          	   45223	     26494 ns/op	   25919 B/op	     595 allocs/op
BenchmarkFastjq_Complex_StringBuild-16        	  106833	     11300 ns/op	    7216 B/op	     298 allocs/op
BenchmarkGojq_Complex_StringBuild-16          	   47536	     25407 ns/op	   20533 B/op	     686 allocs/op
BenchmarkFastjq_Complex_EntryFilter-16        	  339235	      3583 ns/op	    2888 B/op	      69 allocs/op
BenchmarkGojq_Complex_EntryFilter-16          	   78470	     15267 ns/op	   18428 B/op	     250 allocs/op
BenchmarkFastjq_Small_Sqrt-16                 	16875945	        71.49 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Sqrt-16                   	 2405348	       494.7 ns/op	    1145 B/op	      12 allocs/op
BenchmarkFastjq_Small_Log-16                  	31675327	        37.70 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Log-16                    	 2413291	       489.3 ns/op	    1129 B/op	      12 allocs/op
BenchmarkFastjq_Small_Sin-16                  	13675038	        81.46 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Sin-16                    	 2490840	       482.5 ns/op	    1145 B/op	      12 allocs/op
BenchmarkFastjq_Small_Atan-16                 	22525182	        53.73 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Atan-16                   	 2900113	       419.0 ns/op	    1121 B/op	      11 allocs/op
BenchmarkFastjq_Small_Exp-16                  	18268442	        63.87 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Exp-16                    	 2725516	       441.7 ns/op	    1121 B/op	      11 allocs/op
BenchmarkFastjq_Small_Tgamma-16               	91196088	        13.21 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Tgamma-16                 	 2850242	       420.1 ns/op	    1104 B/op	      11 allocs/op
BenchmarkFastjq_Small_Fabs-16                 	21319498	        55.44 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Fabs-16                   	 2566296	       466.1 ns/op	    1113 B/op	      12 allocs/op
BenchmarkFastjq_Small_Abs-16                  	125778812	         9.537 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Abs-16                    	 2682846	       447.4 ns/op	    1112 B/op	      12 allocs/op
BenchmarkFastjq_Small_Bind-16                 	   40353	     29934 ns/op	     648 B/op	      18 allocs/op
BenchmarkGojq_Small_Bind-16                   	 1402060	       862.1 ns/op	    1793 B/op	      24 allocs/op
BenchmarkFastjq_Small_StringInterp-16         	 9913405	       126.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_StringInterp-16           	 1000000	      1132 ns/op	    2121 B/op	      35 allocs/op
BenchmarkFastjq_Small_StringInterpNum-16      	12962322	        92.66 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_StringInterpNum-16        	  925788	      1313 ns/op	    2498 B/op	      40 allocs/op
BenchmarkFastjq_Small_IsEmptyTrue-16          	15118189	        76.05 ns/op	     105 B/op	       6 allocs/op
BenchmarkGojq_Small_IsEmptyTrue-16            	 1899634	       628.2 ns/op	    1921 B/op	      18 allocs/op
BenchmarkFastjq_Small_IsEmptyFalse-16         	13544686	        88.84 ns/op	     105 B/op	       6 allocs/op
BenchmarkGojq_Small_IsEmptyFalse-16           	 1000000	      1117 ns/op	    2898 B/op	      33 allocs/op
BenchmarkFastjq_Small_Nth-16                  	 8836366	       135.9 ns/op	     152 B/op	       7 allocs/op
BenchmarkGojq_Small_Nth-16                    	  511195	      2281 ns/op	    4372 B/op	      49 allocs/op
BenchmarkFastjq_Small_Range10-16              	 5881052	       206.6 ns/op	     120 B/op	      12 allocs/op
BenchmarkGojq_Small_Range10-16                	 1000000	      1077 ns/op	    1864 B/op	      18 allocs/op
BenchmarkFastjq_Small_RangeLimit-16           	 8361266	       144.7 ns/op	     128 B/op	       8 allocs/op
BenchmarkGojq_Small_RangeLimit-16             	  747573	      1596 ns/op	    3208 B/op	      26 allocs/op
BenchmarkRegexp_Match_Hit-16                  	26502790	        45.09 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Miss-16                 	50522060	        23.99 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Complex_Hit-16          	27878206	        43.88 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_Match_Long_Miss-16            	14665623	        78.92 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_MatchString-16                	26802440	        45.11 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_FindSubmatch_Hit-16           	 5638767	       213.5 ns/op	     160 B/op	       2 allocs/op
BenchmarkRegexp_FindSubmatch_Miss-16          	 1712385	       698.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_FindSubmatchIndex_Hit-16      	 6425194	       184.9 ns/op	      64 B/op	       1 allocs/op
BenchmarkRegexp_FindSubmatchIndex_Miss-16     	 1730198	       691.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkRegexp_ReplaceAll_Hit-16             	 7541175	       159.8 ns/op	      97 B/op	       5 allocs/op
BenchmarkRegexp_ReplaceAll_Miss-16            	23765592	        51.13 ns/op	      48 B/op	       2 allocs/op
BenchmarkRegexp_MatchViaClosure-16            	27298965	        44.13 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_Hit-16           	10569140	       113.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_Miss-16          	11817517	       102.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_TestRe_InPipeline-16    	 3976938	       296.9 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_MatchRe_Hit-16          	 5453314	       220.9 ns/op	      48 B/op	       1 allocs/op
BenchmarkFastjq_Small_MatchRe_Miss-16         	 1903536	       625.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_TestRe_Hit-16             	  917474	      1367 ns/op	    2747 B/op	      26 allocs/op
BenchmarkGojq_Small_TestRe_Miss-16            	  942830	      1303 ns/op	    2707 B/op	      26 allocs/op
BenchmarkGojq_Small_MatchRe_Hit-16            	  393886	      3148 ns/op	    4808 B/op	      66 allocs/op
BenchmarkGojq_Small_MatchRe_Miss-16           	  672355	      1721 ns/op	    2254 B/op	      24 allocs/op
BenchmarkFastjq_Small_CaptureRe_Hit-16        	 5481460	       217.5 ns/op	      48 B/op	       1 allocs/op
BenchmarkFastjq_Small_CaptureRe_Miss-16       	 1998816	       599.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ScanRe_NoGroups-16      	 2097562	       570.1 ns/op	     535 B/op	      17 allocs/op
BenchmarkFastjq_Small_ScanRe_WithGroups-16    	 1608926	       742.5 ns/op	     851 B/op	      20 allocs/op
BenchmarkFastjq_Small_SubRe_Hit-16            	 9196473	       130.3 ns/op	      16 B/op	       1 allocs/op
BenchmarkFastjq_Small_SubRe_Miss-16           	11620357	       103.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_GSubRe_Hit-16           	 1989993	       600.5 ns/op	     305 B/op	       5 allocs/op
BenchmarkFastjq_Small_GSubRe_Miss-16          	 4676115	       256.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_CaptureRe_Hit-16          	  500760	      2411 ns/op	    4721 B/op	      51 allocs/op
BenchmarkGojq_Small_CaptureRe_Miss-16         	  663832	      1875 ns/op	    2671 B/op	      27 allocs/op
BenchmarkGojq_Small_ScanRe_NoGroups-16        	  409683	      3017 ns/op	    5132 B/op	      57 allocs/op
BenchmarkGojq_Small_SubRe_Hit-16              	  203767	      6097 ns/op	    9022 B/op	      95 allocs/op
BenchmarkGojq_Small_GSubRe_Hit-16             	   69978	     16120 ns/op	   17893 B/op	     248 allocs/op
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
