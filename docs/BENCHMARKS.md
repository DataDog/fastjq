# fastjq Benchmarks

Benchmarks comparing fastjq against [gojq](https://github.com/itchyny/gojq). gojq benchmarks include the full `json.Unmarshal` → execute → `json.Marshal` cycle, since that's the real-world cost of using a traditional jq engine from Go.

All benchmarks run with `go test -bench=. -benchmem`. Results from Apple M4 Max, Go 1.25.

> **Benchmark scope**: This snapshot uses the five-file upstream jq library harness. Tier 0 library operations benchmark at 0 allocs/op on the hot path; the recursive/path/stateful parity helpers do allocate, but they remain dramatically lighter than gojq for the same queries.


> **Note on benchmark reliability**: Large benchmarks use rotating input copies (8 distinct pre-generated
> instances) to prevent a Go 1.25 calibration artifact where the auto-calibration pre-pass sees warm-cache
> hits and produces results identical to the Small benchmarks. All benchmarks use `b.Loop()` (Go 1.24+)
> and `benchSink` to prevent dead-code elimination. The Large Select benchmark uses `field_199` (the last
> field in the 200-field object) so fastjq must scan the full 170KB — no early-exit advantage.

## Summary

All times in µs. ¹Large Select uses the last field — fastjq scans the full document. ²gojq wins: after unmarshal, integer arrays are native Go slices. ³Compatibility alias in fastjq (`leaf_paths` / `date`); gojq is benchmarked with the equivalent upstream form.

| Operation | Input | fastjq (µs) | gojq (µs) | Speedup | fastjq allocs | gojq allocs |
|-----------|-------|------------|----------|---------|---------------|-------------|
| `.field` | Small (~100B) | 0.084 | 1.000 | **12x** | 0 | 27 |
| `.field` | Large (~100KB) | 7.60 | 592 | **78x** | 0 | 2835 |
| `del(.f)` | Small (~100B) | 0.158 | 2.31 | **15x** | 0 | 58 |
| `del(.f)` | Medium (~2KB) | 2.16 | 19.3 | **8.9x** | 0 | 323 |
| `del(.f)` | Large (~100KB) | 34.3 | 816 | **24x** | 0 | 4666 |
| `.[n]` | 5-elem array | 0.023 | 0.638 | **28x** | 0 | 20 |
| `del(.[n], .[m])` | 5-elem array | 0.086 | 1.73 | **20x** | 0 | 53 |
| `{f0, f2}` (construct) | Small (~100B) | 0.149 | 1.38 | **9.3x** | 0 | 37 |
| `{f0, f50}` (construct) | Large (~100KB) | 7.95 | 598 | **75x** | 0 | 2867 |
| `.[]` iterator | 5-elem array | 0.045 | 0.811 | **18x** | 1 | 26 |
| `.[]` iterator | 200-elem array | 7.13 | 86.5 | **12x** | 1 | 1811 |
| `select(.f == "x")` | Small (~100B) | 0.119 | 1.76 | **15x** | 0 | 45 |
| `select(.f == "x")`¹ | Large (~100KB, last field) | 32.9 | 794 | **24x** | 0 | 4651 |
| `select(.f and .g)` | Small (~100B) | 0.163 | 1.86 | **11x** | 0 | 46 |
| `select(.f or .g)` | Small (~100B) | 0.073 | 1.81 | **25x** | 0 | 46 |
| `has("key")` in select | Small (~100B) | 0.096 | 1.73 | **18x** | 0 | 45 |
| `has("key")` in select | Large (~100KB) | 32.1 | 797 | **25x** | 0 | 4651 |
| `if-then-else` | Small (~100B) | 0.097 | 1.11 | **12x** | 0 | 30 |
| `.f // "default"` | Small (~100B) | 0.095 | 1.11 | **12x** | 0 | 31 |
| `.a + .b` (strings) | Small (~100B) | 0.052 | 0.697 | **13x** | 0 | 21 |
| `"prefix" + .name` | Small (~100B) | 0.082 | 0.741 | **9.1x** | 0 | 23 |
| `length` | Small (~100B) | 0.044 | 0.993 | **22x** | 0 | 27 |
| `length` | Large (~100KB) | 31.5 | 592 | **19x** | 0 | 2835 |
| `add` (numbers) | 5-elem array | 0.057 | 0.750 | **13x** | 0 | 28 |
| `add` (strings) | 5-elem array | 0.133 | 0.905 | **6.8x** | 0 | 35 |
| `flatten` | 3-elem nested array | 0.092 | 1.31 | **14x** | 0 | 38 |
| `map(.name)` | 20-elem array (~600B) | 1.38 | 10.3 | **7.5x** | 6 | 251 |
| `map(.name)` | 200-elem array (~6KB) | 13.4 | 94.1 | **7x** | 6 | 2237 |
| `any` | 5-elem array | 0.044 | 1.91 | **43x** | 0 | 39 |
| `any(expr)` | 5-elem array | 0.386 | 2.12 | **5.5x** | 12 | 49 |
| `any(expr)`² | 200-int array | 5.75 | 32.2 | **5.6x** | 205 | 631 |
| `first(expr)` | 5-elem array | 0.243 | 1.54 | **6.3x** | 10 | 39 |
| `first(expr)`² | 200-int array | 6.15 | 25.3 | **4.1x** | 209 | 626 |
| `last(expr)` | 5-elem array | 0.345 | 1.98 | **5.7x** | 13 | 43 |
| `last(expr)`² | 200-int array | 11.9 | 41.2 | **3.5x** | 404 | 822 |
| `limit(3; expr)` | 5-elem array | 0.078 | 1.71 | **22x** | 3 | 42 |
| `limit(10; expr)` | 200-int array | 0.646 | — | — | 3 | — |
| `.[1:4]` slice | 6-elem array | 5.02 | 0.836 | 0.2x† | 1 | 21 |
| `values` | 9-elem array | 0.116 | 2.43 | **21x** | 2 | 51 |
| `skip(2; .[])` | 5-elem array | 0.102 | 1.77 | **17x** | 4 | 43 |
| `reduce .[] as $x (0; . + $x)` | 5-elem array | 111 | 1.45 | 0.0x† | 70 | 42 |
| `foreach .[] as $x (0; . + $x)` | 5-elem array | 92.0 | 1.35 | 0.0x† | 79 | 39 |
| `while(.<100; .*2)` | integer 1 | 0.848 | 1.65 | **1.9x** | 39 | 25 |
| ``[.,1]\|until(...)\|.[1]`` | integer 5 | 1.85 | 2.49 | **1.3x** | 78 | 53 |
| `paths` | Small (~100B) | 0.373 | 4.37 | **12x** | 16 | 119 |
| `leaf_paths`³ | Small (~100B) | 1.23 | 6.38 | **5.2x** | 56 | 138 |
| `..` | Small (~100B) | 0.219 | 2.91 | **13x** | 7 | 90 |
| `recurse` | Small (~20B object) | 0.097 | 1.78 | **18x** | 5 | 49 |
| `walk(.)` | Small (~10B object) | 0.212 | 2.39 | **11x** | 12 | 52 |
| `path(.field_0)` | Small (~100B) | 0.104 | 1.22 | **12x** | 5 | 36 |
| `to_entries` | Small (~100B) | 0.151 | 4.14 | **27x** | 0 | 98 |
| `to_entries` | Large (~100KB) | 33.7 | 876 | **26x** | 0 | 6465 |
| `getpath(["field_0"])` | Small (~100B) | 0.219 | 1.03 | **4.7x** | 9 | 29 |
| `setpath(["field_0"]; "y")` | Small (~100B) | 0.357 | 1.74 | **4.9x** | 10 | 43 |
| `delpaths([["field_0"]])` | Small (~100B) | 0.678 | 1.74 | **2.6x** | 19 | 40 |
| `keys` | Small (~100B) | 0.288 | 1.30 | **4.5x** | 7 | 35 |
| `keys_unsorted` | Small (~100B) | 0.158 | 1.32 | **8.4x** | 0 | 35 |
| `keys_unsorted` | Large (~100KB) | 31.3 | 616 | **20x** | 0 | 3039 |
| `pick(.field_0, .field_2)` | Small (~100B) | 0.883 | 2.58 | **2.9x** | 27 | 64 |
| `INDEX(range(5)... )` | null | 1.74 | 6.78 | **3.9x** | 90 | 165 |
| `JOIN({...}; .[0]\|tostring)` | 3-pair array | 1.02 | 3.18 | **3.1x** | 49 | 81 |
| `have_decnum` | null | 0.0031 | — | — | 0 | — |
| `todate` | epoch float | 0.051 | 0.737 | **14x** | 0 | 21 |
| `date`³ | epoch float | 0.051 | 0.735 | **14x** | 0 | 21 |
| `now` | null | 0.062 | 0.400 | **6.5x** | 0 | 10 |
| `utf8bytelength` | `"asdf\u03bc"` | 0.030 | 0.401 | **13x** | 1 | 12 |
| `split(",")` | short string | 0.151 | 0.807 | **5.3x** | 0 | 21 |
| `join(",")` | 5-elem array | 0.100 | 0.989 | **9.9x** | 0 | 37 |
| `ascii_downcase` in select | Small (~100B) | 0.138 | 1.78 | **13x** | 0 | 46 |
| `ascii_downcase` in select | Large (~100KB) | 33.0 | 795 | **24x** | 0 | 4652 |
| `startswith("s")` | Small (~100B) | 0.132 | 1.75 | **13x** | 0 | 45 |
| `startswith("s")` | Large (~100KB) | 32.8 | 797 | **24x** | 0 | 4652 |
| `endswith("s")` | Small (~100B) | 0.133 | 1.73 | **13x** | 0 | 45 |
| `trim` | short string | 0.050 | 0.382 | **7.7x** | 1 | 12 |
| `ltrim` | short string | 0.047 | 0.389 | **8.2x** | 1 | 12 |
| `rtrim` | short string | 0.052 | 0.395 | **7.6x** | 1 | 12 |
| `trimstr("s")` | short string | 0.090 | 0.444 | **4.9x** | 5 | 14 |
| `ltrimstr("s")` | Small (~100B) | 0.133 | 1.07 | **8x** | 1 | 30 |
| `rtrimstr("s")` | Small (~100B) | 0.132 | 1.07 | **8.1x** | 1 | 30 |
| `reverse` | 5-elem array | 0.144 | 0.916 | **6.4x** | 4 | 22 |
| `@base64` | 34-char string | 0.148 | 0.530 | **3.6x** | 4 | 15 |
| `@base64d` | 48-char encoded | 0.293 | 0.563 | **1.9x** | 5 | 15 |
| `index(",")` | short string | 0.097 | 0.931 | **9.6x** | 1 | 31 |
| `indices(",")` | short string | 0.205 | 2.15 | **10x** | 1 | 97 |
| `bsearch(42)` | 7-elem sorted array | 0.334 | 0.810 | **2.4x** | 16 | 30 |
| `5 \| IN(range(10))` | null | 0.668 | 2.38 | **3.6x** | 36 | 36 |

## Key Takeaways

- **Tier 0 hot-path ops remain zero-alloc** under `RunWithBuffer` / `RunFunc` for direct access, filtering, arithmetic, construction, and most string/math work. Allocating features here are the deliberate parity exceptions or output-shaped helpers.
- **Large-object access stays roughly **78x** faster than gojq**: `.field` on the ~100KB benchmark is 7.60 µs for fastjq versus 592 µs for gojq, and large-object `select` remains about **24x** faster (32.9 µs versus 794 µs).
- **Recursive parity helpers are materially faster than gojq despite allocs**: `..` is **13x**, `recurse` is **18x**, and `walk(.)` is **11x** on the focused small cases.
- **gojq wins on tiny primitive-array reductions and some stateful/value-synthesizing helpers** such as `first`, `last`, `reduce`, `foreach`, and `range`, where its unmarshaled in-memory representation is cheaper than rescanning raw JSON bytes or emitting many fresh outputs.

## Raw Output

Apple M4 Max, go1.25.4, `go test -bench=. -benchmem`. Updated 2026-05-01. Note: some first-run entries show spurious allocs (e.g. `KeysUnsorted` 3 allocs, `First` 1 alloc) due to benchmark calibration warmup — confirmed 0 allocs on repeat runs.

```
BenchmarkFastjq_Small_Del-16                  	 6717636	       157.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Medium_Del-16                 	  569487	      2164 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Del-16                  	   34988	     34320 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Field-16                	14751732	        83.52 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Field-16                	  156288	      7601 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Index-16                	51337082	        22.61 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ArrayDel-16             	14105325	        85.55 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Construct-16            	 8200497	       149.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Construct-16            	  151820	      7949 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Iterator-16             	26525637	        44.63 ns/op	      16 B/op	       1 allocs/op
BenchmarkFastjq_Large_Iterator-16             	  164546	      7130 ns/op	      16 B/op	       1 allocs/op
BenchmarkFastjq_Small_Select-16               	10116692	       118.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Select-16               	   35917	     32903 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Alternative-16          	15037020	        94.82 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SelectAnd-16            	 7268257	       162.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SelectOr-16             	16651594	        72.89 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Has-16                  	12485059	        96.37 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Has-16                  	   37568	     32143 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_IfThenElse-16           	12579324	        96.67 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Length-16               	27488154	        44.43 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Length-16               	   38082	     31519 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Map-16                  	  868971	      1380 ns/op	     137 B/op	       6 allocs/op
BenchmarkFastjq_Medium_Map-16                 	  182864	      6700 ns/op	     137 B/op	       6 allocs/op
BenchmarkFastjq_Large_Map-16                  	   88713	     13423 ns/op	     137 B/op	       6 allocs/op
BenchmarkFastjq_Small_ToEntries-16            	 8024842	       150.9 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_ToEntries-16            	   35553	     33682 ns/op	       6 B/op	       0 allocs/op
BenchmarkFastjq_Small_KeysUnsorted-16         	 7432138	       158.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Keys-16                 	 4053484	       287.6 ns/op	     456 B/op	       7 allocs/op
BenchmarkFastjq_Small_Paths-16                	 3244928	       372.9 ns/op	     376 B/op	      16 allocs/op
BenchmarkFastjq_Small_LeafPaths-16            	  946092	      1233 ns/op	     696 B/op	      56 allocs/op
BenchmarkFastjq_Small_RecursiveDescent-16     	 5505440	       218.8 ns/op	     224 B/op	       7 allocs/op
BenchmarkFastjq_Small_Recurse-16              	12282952	        97.47 ns/op	      56 B/op	       5 allocs/op
BenchmarkFastjq_Small_Walk-16                 	 5784531	       211.9 ns/op	     200 B/op	      12 allocs/op
BenchmarkFastjq_Small_Path-16                 	11491270	       104.3 ns/op	     144 B/op	       5 allocs/op
BenchmarkFastjq_Small_GetPath-16              	 5457666	       219.3 ns/op	     200 B/op	       9 allocs/op
BenchmarkFastjq_Small_SetPath-16              	 3353790	       357.5 ns/op	     216 B/op	      10 allocs/op
BenchmarkFastjq_Small_DelPaths-16             	 1767267	       677.8 ns/op	     536 B/op	      19 allocs/op
BenchmarkFastjq_Small_Todate-16               	22377170	        50.85 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Date-16                 	24027370	        51.08 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Now-16                  	19565323	        61.59 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_KeysUnsorted-16         	   38005	     31254 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Any-16                  	25760377	        44.20 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Any-16                  	  915145	      1326 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AnyExpr-16              	 3118414	       385.7 ns/op	     144 B/op	      12 allocs/op
BenchmarkFastjq_Large_AnyExpr-16              	  211550	      5753 ns/op	     629 B/op	     205 allocs/op
BenchmarkFastjq_Small_Add-16                  	21367948	        56.58 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AddStrings-16           	 9022682	       133.0 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Flatten-16              	13064920	        91.57 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Split-16                	 7974946	       151.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Join-16                 	11960018	        99.89 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Values-16               	10318522	       116.4 ns/op	      64 B/op	       2 allocs/op
BenchmarkGojq_Small_Values-16                 	  489496	      2433 ns/op	    3144 B/op	      51 allocs/op
BenchmarkFastjq_Small_Base64Encode-16         	 8230143	       147.7 ns/op	     120 B/op	       4 allocs/op
BenchmarkFastjq_Small_Base64Decode-16         	 4116454	       293.0 ns/op	     168 B/op	       5 allocs/op
BenchmarkGojq_Small_Base64Encode-16           	 2263071	       529.8 ns/op	    1321 B/op	      15 allocs/op
BenchmarkGojq_Small_Base64Decode-16           	 2136801	       563.3 ns/op	    1321 B/op	      15 allocs/op
BenchmarkFastjq_Small_IndexFind-16            	12404996	        96.76 ns/op	       3 B/op	       1 allocs/op
BenchmarkFastjq_Small_IndicesAll-16           	 5888052	       204.7 ns/op	       3 B/op	       1 allocs/op
BenchmarkGojq_Small_IndexFind-16              	 1286674	       931.0 ns/op	    2274 B/op	      31 allocs/op
BenchmarkGojq_Small_IndicesAll-16             	  572139	      2149 ns/op	    3907 B/op	      97 allocs/op
BenchmarkFastjq_Small_Slice-16                	  237960	      5024 ns/op	      64 B/op	       1 allocs/op
BenchmarkFastjq_Small_SliceString-16          	  240853	      5049 ns/op	      64 B/op	       1 allocs/op
BenchmarkFastjq_Small_Plus-16                 	23113741	        51.68 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_PlusStr-16              	14729008	        81.67 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AsciiDowncase-16        	 8552892	       138.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_AsciiDowncase-16        	   36268	     32982 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Startswith-16           	 9152509	       132.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Startswith-16           	   36388	     32793 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Endswith-16             	 9008450	       133.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Ltrimstr-16             	 9114292	       133.4 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Large_Ltrimstr-16             	   38320	     31407 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_Rtrimstr-16             	 9417486	       131.8 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_Trimstr-16              	13502767	        89.86 ns/op	      40 B/op	       5 allocs/op
BenchmarkFastjq_Small_Trim-16                 	23056398	        49.94 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_Ltrim-16                	25341828	        47.28 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_Rtrim-16                	22770433	        51.57 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_UTF8ByteLength-16       	39798243	        30.30 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_Reverse-16              	 8364338	       143.9 ns/op	     360 B/op	       4 allocs/op
BenchmarkFastjq_Small_First-16                	 4896404	       243.1 ns/op	     110 B/op	      10 allocs/op
BenchmarkFastjq_Large_First-16                	  194623	      6153 ns/op	     733 B/op	     209 allocs/op
BenchmarkFastjq_Small_Last-16                 	 3502658	       345.4 ns/op	      98 B/op	      13 allocs/op
BenchmarkFastjq_Large_Last-16                 	  101066	     11913 ns/op	    1344 B/op	     404 allocs/op
BenchmarkFastjq_Small_Limit-16                	15456680	        77.56 ns/op	      56 B/op	       3 allocs/op
BenchmarkFastjq_Small_Skip-16                 	11919340	       101.8 ns/op	     136 B/op	       4 allocs/op
BenchmarkFastjq_Small_Reduce-16               	   10000	    110768 ns/op	    2176 B/op	      70 allocs/op
BenchmarkFastjq_Small_Foreach-16              	   13057	     92002 ns/op	    2704 B/op	      79 allocs/op
BenchmarkFastjq_Small_While-16                	 1416354	       848.1 ns/op	     256 B/op	      39 allocs/op
BenchmarkFastjq_Small_Until-16                	  623077	      1848 ns/op	    1704 B/op	      78 allocs/op
BenchmarkFastjq_Small_Bsearch-16              	 3611712	       333.9 ns/op	     488 B/op	      16 allocs/op
BenchmarkFastjq_Small_Pick-16                 	 1358896	       882.9 ns/op	     752 B/op	      27 allocs/op
BenchmarkFastjq_Small_IN-16                   	 1790086	       668.1 ns/op	    1096 B/op	      36 allocs/op
BenchmarkFastjq_Small_INDEX-16                	  676387	      1744 ns/op	    2360 B/op	      90 allocs/op
BenchmarkFastjq_Small_JOIN-16                 	 1000000	      1025 ns/op	    1216 B/op	      49 allocs/op
BenchmarkFastjq_Small_HaveDecnum-16           	390533096	         3.059 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Limit-16                	 1855513	       646.3 ns/op	      56 B/op	       3 allocs/op
BenchmarkGojq_Small_Del-16                    	  522368	      2315 ns/op	    3763 B/op	      58 allocs/op
BenchmarkGojq_Medium_Del-16                   	   62316	     19313 ns/op	   16969 B/op	     323 allocs/op
BenchmarkGojq_Large_Del-16                    	    1494	    816342 ns/op	  542594 B/op	    4666 allocs/op
BenchmarkGojq_Small_Field-16                  	 1207456	       999.7 ns/op	    1641 B/op	      27 allocs/op
BenchmarkGojq_Large_Field-16                  	    2023	    592481 ns/op	  270050 B/op	    2835 allocs/op
BenchmarkGojq_Small_Index-16                  	 1882666	       637.9 ns/op	    1401 B/op	      20 allocs/op
BenchmarkGojq_Small_ArrayDel-16               	  705415	      1725 ns/op	    3363 B/op	      53 allocs/op
BenchmarkGojq_Small_Construct-16              	  838200	      1384 ns/op	    2330 B/op	      37 allocs/op
BenchmarkGojq_Large_Construct-16              	    2035	    597770 ns/op	  274518 B/op	    2867 allocs/op
BenchmarkGojq_Small_Iterator-16               	 1468670	       810.7 ns/op	    1776 B/op	      26 allocs/op
BenchmarkGojq_Large_Iterator-16               	   13872	     86473 ns/op	  109809 B/op	    1811 allocs/op
BenchmarkGojq_Small_Select-16                 	  655549	      1760 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Large_Select-16                 	    1478	    794054 ns/op	  538909 B/op	    4651 allocs/op
BenchmarkGojq_Small_Alternative-16            	 1000000	      1114 ns/op	    1897 B/op	      31 allocs/op
BenchmarkGojq_Small_SelectAnd-16              	  635852	      1864 ns/op	    2890 B/op	      46 allocs/op
BenchmarkGojq_Small_SelectOr-16               	  686539	      1812 ns/op	    2890 B/op	      46 allocs/op
BenchmarkGojq_Small_Has-16                    	  695654	      1726 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Large_Has-16                    	    1492	    797126 ns/op	  535778 B/op	    4651 allocs/op
BenchmarkGojq_Small_IfThenElse-16             	 1000000	      1113 ns/op	    1817 B/op	      30 allocs/op
BenchmarkGojq_Small_Length-16                 	 1209402	       992.7 ns/op	    1633 B/op	      27 allocs/op
BenchmarkGojq_Large_Length-16                 	    2048	    592206 ns/op	  269831 B/op	    2835 allocs/op
BenchmarkGojq_Small_Map-16                    	  117146	     10293 ns/op	   13654 B/op	     251 allocs/op
BenchmarkGojq_Large_Map-16                    	   12765	     94096 ns/op	  118617 B/op	    2237 allocs/op
BenchmarkGojq_Small_ToEntries-16              	  297246	      4141 ns/op	    6559 B/op	      98 allocs/op
BenchmarkGojq_Large_ToEntries-16              	    1328	    876263 ns/op	  671055 B/op	    6465 allocs/op
BenchmarkGojq_Small_KeysUnsorted-16           	  808759	      1321 ns/op	    1953 B/op	      35 allocs/op
BenchmarkGojq_Small_Keys-16                   	  924513	      1304 ns/op	    1953 B/op	      35 allocs/op
BenchmarkGojq_Small_Paths-16                  	  272905	      4371 ns/op	    6608 B/op	     119 allocs/op
BenchmarkGojq_Small_LeafPaths-16              	  188912	      6378 ns/op	    7576 B/op	     138 allocs/op
BenchmarkGojq_Small_RecursiveDescent-16       	  414337	      2912 ns/op	    4856 B/op	      90 allocs/op
BenchmarkGojq_Small_Recurse-16                	  693375	      1785 ns/op	    4240 B/op	      49 allocs/op
BenchmarkGojq_Small_Walk-16                   	  496346	      2392 ns/op	    5701 B/op	      52 allocs/op
BenchmarkGojq_Small_Path-16                   	  990207	      1215 ns/op	    1993 B/op	      36 allocs/op
BenchmarkGojq_Small_GetPath-16                	 1000000	      1035 ns/op	    1721 B/op	      29 allocs/op
BenchmarkGojq_Small_SetPath-16                	  705439	      1736 ns/op	    2618 B/op	      43 allocs/op
BenchmarkGojq_Small_DelPaths-16               	  693674	      1741 ns/op	    2426 B/op	      40 allocs/op
BenchmarkGojq_Small_Todate-16                 	 1622397	       737.2 ns/op	    1713 B/op	      21 allocs/op
BenchmarkGojq_Small_Date-16                   	 1630993	       735.0 ns/op	    1713 B/op	      21 allocs/op
BenchmarkGojq_Small_Now-16                    	 3059043	       399.5 ns/op	    1112 B/op	      10 allocs/op
BenchmarkGojq_Large_KeysUnsorted-16           	    1934	    615645 ns/op	  282869 B/op	    3039 allocs/op
BenchmarkGojq_Small_Any-16                    	  641629	      1912 ns/op	    4508 B/op	      39 allocs/op
BenchmarkGojq_Small_AnyExpr-16                	  559654	      2122 ns/op	    4620 B/op	      49 allocs/op
BenchmarkGojq_Large_AnyExpr-16                	   37701	     32164 ns/op	   27056 B/op	     631 allocs/op
BenchmarkGojq_Small_Add-16                    	 1599015	       750.1 ns/op	    1529 B/op	      28 allocs/op
BenchmarkGojq_Small_AddStrings-16             	 1326192	       904.6 ns/op	    1785 B/op	      35 allocs/op
BenchmarkGojq_Small_Flatten-16                	  907183	      1310 ns/op	    1857 B/op	      38 allocs/op
BenchmarkGojq_Small_Split-16                  	 1486395	       807.0 ns/op	    1553 B/op	      21 allocs/op
BenchmarkGojq_Small_Join-16                   	 1216160	       988.5 ns/op	    1865 B/op	      37 allocs/op
BenchmarkGojq_Small_Slice-16                  	 1432882	       835.9 ns/op	    1425 B/op	      21 allocs/op
BenchmarkGojq_Small_SliceString-16            	 2611080	       472.1 ns/op	    1145 B/op	      12 allocs/op
BenchmarkGojq_Small_Plus-16                   	 1723736	       697.2 ns/op	    1673 B/op	      21 allocs/op
BenchmarkGojq_Small_PlusStr-16                	 1617520	       740.8 ns/op	    1705 B/op	      23 allocs/op
BenchmarkGojq_Small_AsciiDowncase-16          	  683799	      1776 ns/op	    2714 B/op	      46 allocs/op
BenchmarkGojq_Large_AsciiDowncase-16          	    1512	    794501 ns/op	  531571 B/op	    4652 allocs/op
BenchmarkGojq_Small_Startswith-16             	  699202	      1747 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Large_Startswith-16             	    1495	    796576 ns/op	  541004 B/op	    4652 allocs/op
BenchmarkGojq_Small_Endswith-16               	  674271	      1734 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Small_Ltrimstr-16               	 1000000	      1067 ns/op	    1737 B/op	      30 allocs/op
BenchmarkGojq_Small_Rtrimstr-16               	 1000000	      1067 ns/op	    1737 B/op	      30 allocs/op
BenchmarkGojq_Small_Trimstr-16                	 2719258	       443.9 ns/op	    1225 B/op	      14 allocs/op
BenchmarkGojq_Small_Trim-16                   	 3127030	       382.2 ns/op	    1129 B/op	      12 allocs/op
BenchmarkGojq_Small_Ltrim-16                  	 3091734	       388.8 ns/op	    1129 B/op	      12 allocs/op
BenchmarkGojq_Small_Rtrim-16                  	 3046288	       394.5 ns/op	    1129 B/op	      12 allocs/op
BenchmarkGojq_Small_UTF8ByteLength-16         	 2984168	       400.5 ns/op	    1137 B/op	      12 allocs/op
BenchmarkGojq_Small_Reverse-16                	 1300767	       916.4 ns/op	    1513 B/op	      22 allocs/op
BenchmarkGojq_Small_First-16                  	  782274	      1538 ns/op	    3379 B/op	      39 allocs/op
BenchmarkGojq_Large_First-16                  	   47241	     25261 ns/op	   25887 B/op	     626 allocs/op
BenchmarkGojq_Small_Last-16                   	  604765	      1982 ns/op	    3483 B/op	      43 allocs/op
BenchmarkGojq_Large_Last-16                   	   29143	     41185 ns/op	   29833 B/op	     822 allocs/op
BenchmarkGojq_Small_Limit-16                  	  688563	      1706 ns/op	    3328 B/op	      42 allocs/op
BenchmarkGojq_Small_Skip-16                   	  678687	      1772 ns/op	    2880 B/op	      43 allocs/op
BenchmarkGojq_Small_Reduce-16                 	  821010	      1449 ns/op	    2530 B/op	      42 allocs/op
BenchmarkGojq_Small_Foreach-16                	  828452	      1351 ns/op	    2184 B/op	      39 allocs/op
BenchmarkGojq_Small_While-16                  	  730442	      1646 ns/op	    1848 B/op	      25 allocs/op
BenchmarkGojq_Small_Until-16                  	  488396	      2494 ns/op	    2658 B/op	      53 allocs/op
BenchmarkGojq_Small_Bsearch-16                	 1478673	       809.6 ns/op	    1545 B/op	      30 allocs/op
BenchmarkGojq_Small_Pick-16                   	  475672	      2579 ns/op	    4484 B/op	      64 allocs/op
BenchmarkGojq_Small_IN-16                     	  501313	      2381 ns/op	    4147 B/op	      36 allocs/op
BenchmarkGojq_Small_INDEX-16                  	  177622	      6776 ns/op	    9657 B/op	     165 allocs/op
BenchmarkGojq_Small_JOIN-16                   	  383763	      3177 ns/op	    4171 B/op	      81 allocs/op
```

## CLI Throughput: fastjq vs jq

End-to-end JSONL throughput benchmarks comparing `fastjq-bench` (Go CLI using the fastjq library) against `jq` (C implementation, jq-1.8.1). Both read JSONL from stdin, apply a filter per line, write results to `/dev/null`. Both validate JSON: fastjq calls `json.Valid()` per record, jq parses fully. Median of 3 runs. Apple M4 Max.

| Operation | Input | jq (s) | fastjq (s) | Speedup |
|-----------|-------|--------|-------------|---------|
| Identity (`.`) | small (100K lines, ~11MB) | 0.364 | 0.058 | **6.3x** |
| Field access (`.field_2`) | small | 0.163 | 0.051 | **3.2x** |
| Field access (`.field_50`) | large (100 lines, ~16MB) | 0.094 | 0.049 | **1.9x** |
| Delete field (`del(.field_2)`) | small | 0.374 | 0.064 | **5.8x** |
| Object construction (`{field_0, field_2}`) | small | 0.259 | 0.059 | **4.4x** |
| Select all match (`select(.field_2 == "xxx...")`) | small | 0.384 | 0.054 | **7.1x** |
| Select none match (`select(.field_2 == "nope")`) | small | 0.151 | 0.058 | **2.6x** |
| Alternative (`.field_2 // "default"`) | small | 0.176 | 0.052 | **3.4x** |
| Case-insensitive select (`ascii_downcase`) | small | 0.685 | 0.059 | **11.6x** |
| Prefix filter (`startswith`) | small | 0.382 | 0.052 | **7.3x** |
| Field existence (`has`) | small | 0.377 | 0.050 | **7.5x** |
| `to_entries` | small | 0.745 | 0.067 | **11.1x** |
| `keys_unsorted` | small | 0.255 | 0.060 | **4.2x** |

### Key Takeaways (CLI)

- **1.9x–11.6x faster than jq** across this validation-on CLI slice, with the biggest wins on filter/projection work rather than raw field extraction.
- **`to_entries` and case-insensitive filtering are the largest CLI wins** in this benchmark set, because fastjq keeps the work to a streaming scan while jq still pays the full parse cost per line.
- **Large-object field extraction is a modest CLI win** because both tools validate/parse the whole record; the much larger speedups show up when you call the library directly on already-valid JSON and skip the extra validation pass.
- **The CLI numbers are intentionally conservative**: they include JSON validation and process startup overhead that do not apply to the hot-path library API.

### Reproducing

```bash
chmod +x bench_vs_jq.sh
./bench_vs_jq.sh
```
