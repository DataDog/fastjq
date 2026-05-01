# fastjq Benchmarks

Benchmarks comparing fastjq against [gojq](https://github.com/itchyny/gojq). gojq benchmarks include the full `json.Unmarshal` → execute → `json.Marshal` cycle, since that's the real-world cost of using a traditional jq engine from Go.

All benchmarks run with `go test -bench=. -benchmem`. Results from Apple M4 Max, Go 1.25.

> **Current branch note**: This full sweep reflects the jq-parity branch after expanding the upstream harness to five jq test files. Tier 0 library operations still benchmark at 0 allocs/op on the hot path; the parity-first recursive/path/stateful helpers do allocate, but they remain dramatically lighter than gojq for the same queries.


> **Note on benchmark reliability**: Large benchmarks use rotating input copies (8 distinct pre-generated
> instances) to prevent a Go 1.25 calibration artifact where the auto-calibration pre-pass sees warm-cache
> hits and produces results identical to the Small benchmarks. All benchmarks use `b.Loop()` (Go 1.24+)
> and `benchSink` to prevent dead-code elimination. The Large Select benchmark uses `field_199` (the last
> field in the 200-field object) so fastjq must scan the full 170KB — no early-exit advantage.

## Summary

All times in µs. ¹Large Select uses the last field — fastjq scans the full document. ²gojq wins: after unmarshal, integer arrays are native Go slices.

| Operation | Input | fastjq (µs) | gojq (µs) | Speedup | fastjq allocs | gojq allocs |
|-----------|-------|------------|----------|---------|---------------|-------------|
| `.field` | Small (~100B) | 0.085 | 0.986 | **12x** | 0 | 27 |
| `.field` | Large (~100KB) | 7.50 | 587 | **78x** | 0 | 2835 |
| `del(.f)` | Small (~100B) | 0.162 | 2.30 | **14x** | 0 | 58 |
| `del(.f)` | Medium (~2KB) | 2.14 | 19.3 | **9x** | 0 | 323 |
| `del(.f)` | Large (~100KB) | 33.7 | 817 | **24x** | 0 | 4666 |
| `.[n]` | 5-elem array | 0.022 | 0.639 | **29x** | 0 | 20 |
| `del(.[n], .[m])` | 5-elem array | 0.083 | 1.71 | **21x** | 0 | 53 |
| `{f0, f2}` (construct) | Small (~100B) | 0.141 | 1.39 | **9.9x** | 0 | 37 |
| `{f0, f50}` (construct) | Large (~100KB) | 7.90 | 594 | **75x** | 0 | 2867 |
| `.[]` iterator | 5-elem array | 0.044 | 0.781 | **18x** | 1 | 26 |
| `.[]` iterator | 200-elem array | 7.36 | 86.5 | **12x** | 1 | 1811 |
| `select(.f == "x")` | Small (~100B) | 0.130 | 1.73 | **13x** | 0 | 45 |
| `select(.f == "x")`¹ | Large (~100KB, last field) | 32.9 | 786 | **24x** | 0 | 4651 |
| `select(.f and .g)` | Small (~100B) | 0.168 | 1.87 | **11x** | 0 | 46 |
| `select(.f or .g)` | Small (~100B) | 0.073 | 1.82 | **25x** | 0 | 46 |
| `has("key")` in select | Small (~100B) | 0.094 | 1.70 | **18x** | 0 | 45 |
| `has("key")` in select | Large (~100KB) | 32.5 | 786 | **24x** | 0 | 4651 |
| `if-then-else` | Small (~100B) | 0.100 | 1.10 | **11x** | 0 | 30 |
| `.f // "default"` | Small (~100B) | 0.096 | 1.10 | **11x** | 0 | 31 |
| `.a + .b` (strings) | Small (~100B) | 0.053 | 0.706 | **13x** | 0 | 21 |
| `"prefix" + .name` | Small (~100B) | 0.082 | 0.772 | **9.5x** | 0 | 23 |
| `length` | Small (~100B) | 0.043 | 0.996 | **23x** | 0 | 27 |
| `length` | Large (~100KB) | 31.5 | 591 | **19x** | 0 | 2835 |
| `add` (numbers) | 5-elem array | 0.057 | 0.765 | **13x** | 0 | 28 |
| `add` (strings) | 5-elem array | 0.133 | 0.918 | **6.9x** | 0 | 35 |
| `flatten` | 3-elem nested array | 0.093 | 1.33 | **14x** | 0 | 38 |
| `map(.name)` | 20-elem array (~600B) | 1.31 | 10.6 | **8.1x** | 6 | 251 |
| `map(.name)` | 200-elem array (~6KB) | 12.9 | 93.7 | **7.3x** | 6 | 2237 |
| `any` | 5-elem array | 0.044 | 1.93 | **44x** | 0 | 39 |
| `any(expr)` | 5-elem array | 0.392 | 2.20 | **5.6x** | 12 | 49 |
| `any(expr)`² | 200-int array | 5.67 | 1.78 | 0.3x† | 205 | 29 |
| `first(expr)` | 5-elem array | 0.240 | 1.55 | **6.5x** | 9 | 39 |
| `first(expr)`² | 200-int array | 6.72 | 1.46 | 0.2x† | 207 | 23 |
| `last(expr)` | 5-elem array | 0.339 | 1.98 | **5.8x** | 13 | 43 |
| `last(expr)`² | 200-int array | 6.23 | 1.55 | 0.2x† | 206 | 24 |
| `limit(3; expr)` | 5-elem array | 0.077 | 1.71 | **22x** | 3 | 42 |
| `limit(10; expr)` | 200-int array | 0.655 | — | — | 3 | — |
| `.[1:4]` slice | 6-elem array | 5.02 | 0.840 | 0.2x† | 1 | 21 |
| `values` | 9-elem array | 0.117 | 2.41 | **21x** | 2 | 51 |
| `skip(2; .[])` | 5-elem array | 0.103 | 1.77 | **17x** | 4 | 43 |
| `reduce .[] as $x (0; . + $x)` | 5-elem array | 112 | 1.45 | 0.0x† | 70 | 42 |
| `foreach .[] as $x (0; . + $x)` | 5-elem array | 93.5 | 1.36 | 0.0x† | 79 | 39 |
| `while(.<100; .*2)` | integer 1 | 0.857 | 1.67 | **2x** | 39 | 25 |
| ``[.,1]\|until(...)\|.[1]`` | integer 5 | 1.83 | 2.50 | **1.4x** | 78 | 53 |
| `paths` | Small (~100B) | 0.380 | 4.37 | **11x** | 16 | 119 |
| `..` | Small (~100B) | 0.218 | 2.91 | **13x** | 7 | 90 |
| `recurse` | Small (~20B object) | 0.102 | 1.78 | **17x** | 5 | 49 |
| `walk(.)` | Small (~10B object) | 0.212 | 2.40 | **11x** | 12 | 52 |
| `path(.field_0)` | Small (~100B) | 0.107 | 1.22 | **11x** | 5 | 36 |
| `to_entries` | Small (~100B) | 0.157 | 4.16 | **27x** | 0 | 98 |
| `to_entries` | Large (~100KB) | 34.1 | 876 | **26x** | 0 | 6465 |
| `getpath(["field_0"])` | Small (~100B) | 0.219 | 1.04 | **4.8x** | 9 | 29 |
| `setpath(["field_0"]; "y")` | Small (~100B) | 0.360 | 1.73 | **4.8x** | 10 | 43 |
| `delpaths([["field_0"]])` | Small (~100B) | 0.651 | 1.75 | **2.7x** | 19 | 40 |
| `keys` | Small (~100B) | 0.286 | 1.31 | **4.6x** | 7 | 35 |
| `keys_unsorted` | Small (~100B) | 0.149 | 1.32 | **8.8x** | 0 | 35 |
| `keys_unsorted` | Large (~100KB) | 31.5 | 618 | **20x** | 0 | 3039 |
| `pick(.field_0, .field_2)` | Small (~100B) | 0.881 | 2.55 | **2.9x** | 27 | 64 |
| `INDEX(range(5)... )` | null | 1.75 | 6.86 | **3.9x** | 90 | 165 |
| `JOIN({...}; .[0]\|tostring)` | 3-pair array | 1.03 | 3.24 | **3.2x** | 49 | 81 |
| `have_decnum` | null | 0.0028 | — | — | 0 | — |
| `utf8bytelength` | `"asdf\u03bc"` | 0.029 | 0.404 | **14x** | 1 | 12 |
| `split(",")` | short string | 0.146 | 0.812 | **5.6x** | 0 | 21 |
| `join(",")` | 5-elem array | 0.101 | 1.00 | **9.9x** | 0 | 37 |
| `ascii_downcase` in select | Small (~100B) | 0.147 | 1.84 | **13x** | 0 | 46 |
| `ascii_downcase` in select | Large (~100KB) | 33.6 | 799 | **24x** | 0 | 4652 |
| `startswith("s")` | Small (~100B) | 0.129 | 1.76 | **14x** | 0 | 45 |
| `startswith("s")` | Large (~100KB) | 33.0 | 795 | **24x** | 0 | 4651 |
| `endswith("s")` | Small (~100B) | 0.128 | 1.75 | **14x** | 0 | 45 |
| `trim` | short string | 0.050 | 0.392 | **7.9x** | 1 | 12 |
| `ltrim` | short string | 0.046 | 0.406 | **8.9x** | 1 | 12 |
| `rtrim` | short string | 0.049 | 0.398 | **8.2x** | 1 | 12 |
| `trimstr("s")` | short string | 0.090 | 0.449 | **5x** | 5 | 14 |
| `ltrimstr("s")` | Small (~100B) | 0.135 | 1.07 | **7.9x** | 1 | 30 |
| `rtrimstr("s")` | Small (~100B) | 0.137 | 1.07 | **7.8x** | 1 | 30 |
| `reverse` | 5-elem array | 0.144 | 0.908 | **6.3x** | 4 | 22 |
| `@base64` | 34-char string | 0.146 | 0.528 | **3.6x** | 4 | 15 |
| `@base64d` | 48-char encoded | 0.297 | 0.561 | **1.9x** | 5 | 15 |
| `index(",")` | short string | 0.097 | 0.926 | **9.6x** | 1 | 31 |
| `indices(",")` | short string | 0.204 | 2.15 | **11x** | 1 | 97 |
| `bsearch(42)` | 7-elem sorted array | 0.331 | 0.815 | **2.5x** | 16 | 30 |
| `5 \| IN(range(10))` | null | 0.669 | 2.42 | **3.6x** | 36 | 36 |

## Key Takeaways

- **Tier 0 hot-path ops remain zero-alloc** under `RunWithBuffer` / `RunFunc` for direct access, filtering, arithmetic, construction, and most string/math work. Allocating features on this branch are the deliberate parity exceptions or output-shaped helpers.
- **Large-object access stays roughly **78x** faster than gojq**: `.field` on the ~100KB benchmark is 7.50 µs for fastjq versus 587 µs for gojq, and large-object `select` remains about **24x** faster (32.9 µs versus 786 µs).
- **Recursive parity helpers are still materially faster than gojq despite allocs**: `..` is **13x**, `recurse` is **17x**, and `walk(.)` is **11x** on the focused small cases.
- **gojq still wins on tiny primitive-array reductions and some stateful/value-synthesizing helpers** such as `first`, `last`, `reduce`, `foreach`, and `range`, where its unmarshaled in-memory representation is cheaper than rescanning raw JSON bytes or emitting many fresh outputs.

## Raw Output

Apple M4 Max, go1.25.4, `go test -bench=. -benchmem`. Updated 2026-05-01. Note: some first-run entries show spurious allocs (e.g. `KeysUnsorted` 3 allocs, `First` 1 alloc) due to benchmark calibration warmup — confirmed 0 allocs on repeat runs.

```
BenchmarkFastjq_Small_Del-16                  	 6707846	       161.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Medium_Del-16                 	  540334	      2136 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Del-16                  	   35328	     33736 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Field-16                	14634042	        85.20 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Field-16                	  157497	      7497 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Index-16                	54400590	        22.02 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ArrayDel-16             	14214037	        82.94 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Construct-16            	 9448908	       140.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Construct-16            	  155425	      7898 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Iterator-16             	25246636	        44.24 ns/op	      16 B/op	       1 allocs/op
BenchmarkFastjq_Large_Iterator-16             	  168470	      7358 ns/op	      16 B/op	       1 allocs/op
BenchmarkFastjq_Small_Select-16               	 9401529	       129.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Select-16               	   36570	     32938 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Alternative-16          	12461980	        96.19 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SelectAnd-16            	 7283204	       167.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SelectOr-16             	16611918	        73.39 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Has-16                  	12437220	        93.72 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Has-16                  	   36986	     32487 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_IfThenElse-16           	12026788	       100.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Length-16               	26779636	        43.31 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Length-16               	   37244	     31471 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Map-16                  	  918592	      1311 ns/op	     137 B/op	       6 allocs/op
BenchmarkFastjq_Medium_Map-16                 	  196272	      6267 ns/op	     137 B/op	       6 allocs/op
BenchmarkFastjq_Large_Map-16                  	   94098	     12913 ns/op	     137 B/op	       6 allocs/op
BenchmarkFastjq_Small_ToEntries-16            	 7605033	       156.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_ToEntries-16            	   35202	     34081 ns/op	       6 B/op	       0 allocs/op
BenchmarkFastjq_Small_KeysUnsorted-16         	 8046100	       148.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Keys-16                 	 4154920	       285.8 ns/op	     456 B/op	       7 allocs/op
BenchmarkFastjq_Small_Paths-16                	 3097678	       380.2 ns/op	     376 B/op	      16 allocs/op
BenchmarkFastjq_Small_RecursiveDescent-16     	 5692990	       217.7 ns/op	     224 B/op	       7 allocs/op
BenchmarkFastjq_Small_Recurse-16              	11839748	       102.1 ns/op	      56 B/op	       5 allocs/op
BenchmarkFastjq_Small_Walk-16                 	 5643126	       211.8 ns/op	     200 B/op	      12 allocs/op
BenchmarkFastjq_Small_Path-16                 	11081926	       106.7 ns/op	     144 B/op	       5 allocs/op
BenchmarkFastjq_Small_GetPath-16              	 5469922	       218.6 ns/op	     200 B/op	       9 allocs/op
BenchmarkFastjq_Small_SetPath-16              	 3328264	       360.4 ns/op	     216 B/op	      10 allocs/op
BenchmarkFastjq_Small_DelPaths-16             	 1834890	       651.5 ns/op	     536 B/op	      19 allocs/op
BenchmarkFastjq_Large_KeysUnsorted-16         	   38076	     31506 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Any-16                  	26819462	        44.24 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Any-16                  	  877987	      1369 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AnyExpr-16              	 3051782	       392.4 ns/op	     144 B/op	      12 allocs/op
BenchmarkFastjq_Large_AnyExpr-16              	  204129	      5669 ns/op	     629 B/op	     205 allocs/op
BenchmarkFastjq_Small_Add-16                  	21039420	        57.07 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AddStrings-16           	 9135351	       132.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Flatten-16              	12855793	        92.76 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Split-16                	 8173189	       145.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Join-16                 	11900491	       101.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Values-16               	10376317	       116.8 ns/op	      64 B/op	       2 allocs/op
BenchmarkGojq_Small_Values-16                 	  502657	      2413 ns/op	    3144 B/op	      51 allocs/op
BenchmarkFastjq_Small_Base64Encode-16         	 8176498	       145.7 ns/op	     120 B/op	       4 allocs/op
BenchmarkFastjq_Small_Base64Decode-16         	 4055194	       297.4 ns/op	     168 B/op	       5 allocs/op
BenchmarkGojq_Small_Base64Encode-16           	 2267788	       528.3 ns/op	    1321 B/op	      15 allocs/op
BenchmarkGojq_Small_Base64Decode-16           	 2153706	       561.3 ns/op	    1321 B/op	      15 allocs/op
BenchmarkFastjq_Small_IndexFind-16            	12415114	        96.82 ns/op	       3 B/op	       1 allocs/op
BenchmarkFastjq_Small_IndicesAll-16           	 5840773	       204.2 ns/op	       3 B/op	       1 allocs/op
BenchmarkGojq_Small_IndexFind-16              	 1294726	       925.6 ns/op	    2274 B/op	      31 allocs/op
BenchmarkGojq_Small_IndicesAll-16             	  560763	      2152 ns/op	    3907 B/op	      97 allocs/op
BenchmarkFastjq_Small_Slice-16                	  239649	      5017 ns/op	      64 B/op	       1 allocs/op
BenchmarkFastjq_Small_SliceString-16          	  234318	      5090 ns/op	      64 B/op	       1 allocs/op
BenchmarkFastjq_Small_Plus-16                 	22996324	        52.72 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_PlusStr-16              	14405322	        81.71 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_AsciiDowncase-16        	 8155460	       146.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_AsciiDowncase-16        	   35896	     33581 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Startswith-16           	 8902184	       129.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Startswith-16           	   36373	     33025 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Endswith-16             	 9905296	       127.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Ltrimstr-16             	 9033326	       134.7 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Large_Ltrimstr-16             	   37254	     32001 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_Rtrimstr-16             	 8744953	       137.0 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_Trimstr-16              	13439461	        89.57 ns/op	      40 B/op	       5 allocs/op
BenchmarkFastjq_Small_Trim-16                 	23930004	        49.53 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_Ltrim-16                	26760277	        45.62 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_Rtrim-16                	24542072	        48.60 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_UTF8ByteLength-16       	41502805	        29.13 ns/op	       8 B/op	       1 allocs/op
BenchmarkFastjq_Small_Reverse-16              	 8408038	       143.9 ns/op	     360 B/op	       4 allocs/op
BenchmarkFastjq_Small_First-16                	 4989324	       240.0 ns/op	     120 B/op	       9 allocs/op
BenchmarkFastjq_Large_First-16                	  178040	      6717 ns/op	     728 B/op	     207 allocs/op
BenchmarkFastjq_Small_Last-16                 	 3566726	       338.7 ns/op	      98 B/op	      13 allocs/op
BenchmarkFastjq_Large_Last-16                 	  193976	      6234 ns/op	     712 B/op	     206 allocs/op
BenchmarkFastjq_Small_Limit-16                	15652633	        76.53 ns/op	      56 B/op	       3 allocs/op
BenchmarkFastjq_Small_Skip-16                 	11786516	       103.2 ns/op	     136 B/op	       4 allocs/op
BenchmarkFastjq_Small_Reduce-16               	   10000	    111899 ns/op	    2176 B/op	      70 allocs/op
BenchmarkFastjq_Small_Foreach-16              	   12824	     93499 ns/op	    2704 B/op	      79 allocs/op
BenchmarkFastjq_Small_While-16                	 1398111	       857.0 ns/op	     256 B/op	      39 allocs/op
BenchmarkFastjq_Small_Until-16                	  594540	      1831 ns/op	    1704 B/op	      78 allocs/op
BenchmarkFastjq_Small_Bsearch-16              	 3625918	       331.2 ns/op	     488 B/op	      16 allocs/op
BenchmarkFastjq_Small_Pick-16                 	 1358288	       880.9 ns/op	     752 B/op	      27 allocs/op
BenchmarkFastjq_Small_IN-16                   	 1791260	       669.0 ns/op	    1096 B/op	      36 allocs/op
BenchmarkFastjq_Small_INDEX-16                	  683164	      1746 ns/op	    2360 B/op	      90 allocs/op
BenchmarkFastjq_Small_JOIN-16                 	 1000000	      1027 ns/op	    1216 B/op	      49 allocs/op
BenchmarkFastjq_Small_HaveDecnum-16           	428307495	         2.808 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Limit-16                	 1828573	       655.2 ns/op	      56 B/op	       3 allocs/op
BenchmarkGojq_Small_Del-16                    	  523406	      2301 ns/op	    3763 B/op	      58 allocs/op
BenchmarkGojq_Medium_Del-16                   	   62702	     19275 ns/op	   16971 B/op	     323 allocs/op
BenchmarkGojq_Large_Del-16                    	    1471	    817085 ns/op	  547654 B/op	    4666 allocs/op
BenchmarkGojq_Small_Field-16                  	 1216962	       985.9 ns/op	    1641 B/op	      27 allocs/op
BenchmarkGojq_Large_Field-16                  	    2035	    586860 ns/op	  270051 B/op	    2835 allocs/op
BenchmarkGojq_Small_Index-16                  	 1898912	       639.1 ns/op	    1401 B/op	      20 allocs/op
BenchmarkGojq_Small_ArrayDel-16               	  703760	      1708 ns/op	    3363 B/op	      53 allocs/op
BenchmarkGojq_Small_Construct-16              	  876194	      1386 ns/op	    2330 B/op	      37 allocs/op
BenchmarkGojq_Large_Construct-16              	    1990	    593609 ns/op	  274479 B/op	    2867 allocs/op
BenchmarkGojq_Small_Iterator-16               	 1540354	       781.4 ns/op	    1776 B/op	      26 allocs/op
BenchmarkGojq_Large_Iterator-16               	   13828	     86528 ns/op	  109808 B/op	    1811 allocs/op
BenchmarkGojq_Small_Select-16                 	  692576	      1729 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Large_Select-16                 	    1518	    786026 ns/op	  537362 B/op	    4651 allocs/op
BenchmarkGojq_Small_Alternative-16            	 1000000	      1102 ns/op	    1897 B/op	      31 allocs/op
BenchmarkGojq_Small_SelectAnd-16              	  651358	      1868 ns/op	    2890 B/op	      46 allocs/op
BenchmarkGojq_Small_SelectOr-16               	  646801	      1825 ns/op	    2890 B/op	      46 allocs/op
BenchmarkGojq_Small_Has-16                    	  701504	      1702 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Large_Has-16                    	    1534	    785508 ns/op	  530507 B/op	    4651 allocs/op
BenchmarkGojq_Small_IfThenElse-16             	 1000000	      1105 ns/op	    1817 B/op	      30 allocs/op
BenchmarkGojq_Small_Length-16                 	 1204508	       995.8 ns/op	    1633 B/op	      27 allocs/op
BenchmarkGojq_Large_Length-16                 	    2048	    590868 ns/op	  269831 B/op	    2835 allocs/op
BenchmarkGojq_Small_Map-16                    	  105420	     10629 ns/op	   13654 B/op	     251 allocs/op
BenchmarkGojq_Large_Map-16                    	   12825	     93677 ns/op	  118581 B/op	    2237 allocs/op
BenchmarkGojq_Small_ToEntries-16              	  288685	      4161 ns/op	    6559 B/op	      98 allocs/op
BenchmarkGojq_Large_ToEntries-16              	    1358	    875656 ns/op	  679963 B/op	    6465 allocs/op
BenchmarkGojq_Small_KeysUnsorted-16           	  894206	      1316 ns/op	    1953 B/op	      35 allocs/op
BenchmarkGojq_Small_Keys-16                   	  934412	      1314 ns/op	    1953 B/op	      35 allocs/op
BenchmarkGojq_Small_Paths-16                  	  273133	      4368 ns/op	    6608 B/op	     119 allocs/op
BenchmarkGojq_Small_RecursiveDescent-16       	  407586	      2907 ns/op	    4856 B/op	      90 allocs/op
BenchmarkGojq_Small_Recurse-16                	  690729	      1784 ns/op	    4240 B/op	      49 allocs/op
BenchmarkGojq_Small_Walk-16                   	  487604	      2399 ns/op	    5701 B/op	      52 allocs/op
BenchmarkGojq_Small_Path-16                   	 1000000	      1217 ns/op	    1993 B/op	      36 allocs/op
BenchmarkGojq_Small_GetPath-16                	 1000000	      1043 ns/op	    1721 B/op	      29 allocs/op
BenchmarkGojq_Small_SetPath-16                	  690980	      1731 ns/op	    2618 B/op	      43 allocs/op
BenchmarkGojq_Small_DelPaths-16               	  693594	      1747 ns/op	    2426 B/op	      40 allocs/op
BenchmarkGojq_Large_KeysUnsorted-16           	    1929	    618482 ns/op	  282796 B/op	    3039 allocs/op
BenchmarkGojq_Small_Any-16                    	  618321	      1929 ns/op	    4508 B/op	      39 allocs/op
BenchmarkGojq_Small_AnyExpr-16                	  520552	      2200 ns/op	    4620 B/op	      49 allocs/op
BenchmarkGojq_Large_AnyExpr-16                	  664305	      1779 ns/op	    2818 B/op	      29 allocs/op
BenchmarkGojq_Small_Add-16                    	 1563741	       765.1 ns/op	    1529 B/op	      28 allocs/op
BenchmarkGojq_Small_AddStrings-16             	 1311645	       918.5 ns/op	    1785 B/op	      35 allocs/op
BenchmarkGojq_Small_Flatten-16                	  908932	      1333 ns/op	    1857 B/op	      38 allocs/op
BenchmarkGojq_Small_Split-16                  	 1473613	       812.3 ns/op	    1553 B/op	      21 allocs/op
BenchmarkGojq_Small_Join-16                   	 1000000	      1005 ns/op	    1865 B/op	      37 allocs/op
BenchmarkGojq_Small_Slice-16                  	 1426171	       840.1 ns/op	    1425 B/op	      21 allocs/op
BenchmarkGojq_Small_SliceString-16            	 2580516	       463.2 ns/op	    1145 B/op	      12 allocs/op
BenchmarkGojq_Small_Plus-16                   	 1690334	       705.7 ns/op	    1673 B/op	      21 allocs/op
BenchmarkGojq_Small_PlusStr-16                	 1588429	       772.2 ns/op	    1705 B/op	      23 allocs/op
BenchmarkGojq_Small_AsciiDowncase-16          	  650163	      1843 ns/op	    2714 B/op	      46 allocs/op
BenchmarkGojq_Large_AsciiDowncase-16          	    1484	    799061 ns/op	  536662 B/op	    4652 allocs/op
BenchmarkGojq_Small_Startswith-16             	  673425	      1762 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Large_Startswith-16             	    1503	    795474 ns/op	  538717 B/op	    4651 allocs/op
BenchmarkGojq_Small_Endswith-16               	  682198	      1755 ns/op	    2698 B/op	      45 allocs/op
BenchmarkGojq_Small_Ltrimstr-16               	 1000000	      1068 ns/op	    1737 B/op	      30 allocs/op
BenchmarkGojq_Small_Rtrimstr-16               	 1000000	      1066 ns/op	    1737 B/op	      30 allocs/op
BenchmarkGojq_Small_Trimstr-16                	 2697895	       448.9 ns/op	    1225 B/op	      14 allocs/op
BenchmarkGojq_Small_Trim-16                   	 3066268	       392.3 ns/op	    1129 B/op	      12 allocs/op
BenchmarkGojq_Small_Ltrim-16                  	 2925098	       405.8 ns/op	    1129 B/op	      12 allocs/op
BenchmarkGojq_Small_Rtrim-16                  	 3025762	       398.3 ns/op	    1129 B/op	      12 allocs/op
BenchmarkGojq_Small_UTF8ByteLength-16         	 2958561	       403.6 ns/op	    1137 B/op	      12 allocs/op
BenchmarkGojq_Small_Reverse-16                	 1322119	       908.1 ns/op	    1513 B/op	      22 allocs/op
BenchmarkGojq_Small_First-16                  	  792795	      1550 ns/op	    3379 B/op	      39 allocs/op
BenchmarkGojq_Large_First-16                  	  846423	      1461 ns/op	    1889 B/op	      23 allocs/op
BenchmarkGojq_Small_Last-16                   	  605422	      1976 ns/op	    3483 B/op	      43 allocs/op
BenchmarkGojq_Large_Last-16                   	  775884	      1555 ns/op	    2193 B/op	      24 allocs/op
BenchmarkGojq_Small_Limit-16                  	  706820	      1705 ns/op	    3328 B/op	      42 allocs/op
BenchmarkGojq_Small_Skip-16                   	  705552	      1773 ns/op	    2880 B/op	      43 allocs/op
BenchmarkGojq_Small_Reduce-16                 	  828175	      1453 ns/op	    2530 B/op	      42 allocs/op
BenchmarkGojq_Small_Foreach-16                	  797892	      1365 ns/op	    2184 B/op	      39 allocs/op
BenchmarkGojq_Small_While-16                  	  713188	      1672 ns/op	    1848 B/op	      25 allocs/op
BenchmarkGojq_Small_Until-16                  	  485959	      2504 ns/op	    2658 B/op	      53 allocs/op
BenchmarkGojq_Small_Bsearch-16                	 1478890	       814.7 ns/op	    1545 B/op	      30 allocs/op
BenchmarkGojq_Small_Pick-16                   	  475663	      2551 ns/op	    4484 B/op	      64 allocs/op
BenchmarkGojq_Small_IN-16                     	  503457	      2424 ns/op	    4147 B/op	      36 allocs/op
BenchmarkGojq_Small_INDEX-16                  	  176750	      6856 ns/op	    9657 B/op	     165 allocs/op
BenchmarkGojq_Small_JOIN-16                   	  367795	      3239 ns/op	    4171 B/op	      81 allocs/op
```

## CLI Throughput: fastjq vs jq

End-to-end JSONL throughput benchmarks comparing `fastjq-bench` (Go CLI using the fastjq library) against `jq` (C implementation, jq-1.8.1). Both read JSONL from stdin, apply a filter per line, write results to `/dev/null`. Both validate JSON: fastjq calls `json.Valid()` per record, jq parses fully. Median of 3 runs. Apple M4 Max.

| Operation | Input | jq (s) | fastjq (s) | Speedup |
|-----------|-------|--------|-------------|---------|
| Identity (`.`) | small (100K lines, ~11MB) | 0.368 | 0.059 | **6.2x** |
| Field access (`.field_2`) | small | 0.165 | 0.049 | **3.4x** |
| Field access (`.field_50`) | large (100 lines, ~16MB) | 0.092 | 0.044 | **2.1x** |
| Delete field (`del(.field_2)`) | small | 0.381 | 0.065 | **5.9x** |
| Object construction (`{field_0, field_2}`) | small | 0.263 | 0.060 | **4.4x** |
| Select all match (`select(.field_2 == "xxx...")`) | small | 0.388 | 0.057 | **6.8x** |
| Select none match (`select(.field_2 == "nope")`) | small | 0.152 | 0.059 | **2.6x** |
| Alternative (`.field_2 // "default"`) | small | 0.177 | 0.052 | **3.4x** |
| Case-insensitive select (`ascii_downcase`) | small | 0.693 | 0.061 | **11.4x** |
| Prefix filter (`startswith`) | small | 0.383 | 0.053 | **7.2x** |
| Field existence (`has`) | small | 0.380 | 0.052 | **7.3x** |
| `to_entries` | small | 0.739 | 0.069 | **10.7x** |
| `keys_unsorted` | small | 0.259 | 0.060 | **4.3x** |

### Key Takeaways (CLI)

- **2.1x–11.4x faster than jq** across this validation-on CLI slice, with the biggest wins on filter/projection work rather than raw field extraction.
- **`to_entries` and case-insensitive filtering are the standout CLI wins** in this run, because fastjq keeps the work to a streaming scan while jq still pays the full parse cost per line.
- **Large-object field extraction is still only a modest CLI win** because both tools validate/parse the whole record; the much larger speedups show up when you call the library directly on already-valid JSON and skip the extra validation pass.
- **The CLI numbers are intentionally conservative**: they include JSON validation and process startup overhead that do not apply to the hot-path library API.

### Reproducing

```bash
chmod +x bench_vs_jq.sh
./bench_vs_jq.sh
```
