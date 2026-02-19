# fastjq Benchmarks

Benchmarks comparing fastjq against [gojq](https://github.com/itchyny/gojq). gojq benchmarks include the full `json.Unmarshal` → execute → `json.Marshal` cycle, since that's the real-world cost of using a traditional jq engine from Go.

All benchmarks run with `go test -bench=. -benchmem`. Results from Apple M4 Max, Go 1.25.

> **Note on benchmark reliability**: Large benchmarks use rotating input copies (8 distinct pre-generated
> instances) to prevent a Go 1.25 calibration artifact where the auto-calibration pre-pass sees warm-cache
> hits and produces results identical to the Small benchmarks. All benchmarks use `b.Loop()` (Go 1.24+)
> and `benchSink` to prevent dead-code elimination. The Large Select benchmark uses `field_199` (the last
> field in the 200-field object) so fastjq must scan the full 170KB — no early-exit advantage.

## Summary

All times in µs. Small/Medium values are sub-microsecond but shown with enough precision to compare.

| Operation | Input | fastjq (µs) | gojq (µs) | Speedup | fastjq allocs | gojq allocs |
|-----------|-------|------------|----------|---------|---------------|-------------|
| Field access | Small (~100B) | 0.144 | 0.327 | **2.3x** | 0 | 13 |
| Field access | Large (~100KB) | 109 | 543 | **5.0x** | 0 | 2,835 |
| Field deletion | Small (~100B) | 0.158 | 0.892 | **5.6x** | 0 | 33 |
| Field deletion | Medium (~2KB) | 2.7 | 18.0 | **6.7x** | 0 | 323 |
| Field deletion | Large (~100KB) | 155 | 766 | **4.9x** | 0 | 4,666 |
| Array index `.[2]` | 5-elem array | 0.025 | 0.588 | **24x** | 0 | 20 |
| Array deletion `del(.[1],.[3])` | 5-elem array | 0.089 | 1.575 | **18x** | 0 | 53 |
| Object construction `{f0, f2}` | Small (~100B) | 0.263 | 0.665 | **2.5x** | 0 | 23 |
| Object construction `{f0, f50}` | Large (~100KB) | 218 | 553 | **2.5x** | 0 | 2,867 |
| Iterator `.[]` | 5-elem array | 0.031 | 0.729 | **24x** | 0 | 26 |
| Iterator `.[]` | 200-elem array | 9.7 | 80 | **8.2x** | 0 | 1,811 |
| Select `select(.f == "x")` | Small (~100B) | 0.0094 | 0.574 | **61x** | 0 | 20 |
| Select `select(.f == "x")` | Large (~100KB, last field) | 16 | 770 | **48x** | 0 | 4,651 |
| Select with `and` | Small (~100B) | 0.011 | 0.607 | **56x** | 0 | 21 |
| Select with `or` | Small (~100B) | 0.011 | 0.653 | **60x** | 0 | 21 |
| `has("key")` in select | Small (~100B) | 0.012 | 0.554 | **48x** | 0 | 20 |
| `if-then-else` | Small (~100B) | 0.0098 | 0.446 | **46x** | 0 | 16 |
| `length` | Small (~100B) | 0.0064 | 0.363 | **56x** | 0 | 13 |
| `map(.name)` | 20-elem array (~600B) | 1.99 | 9.98 | **5.0x** | 0 | 251 |
| `to_entries` | Small (~100B) | 0.0062 | 0.359 | **58x** | 0 | 14 |
| `with_entries(select(...))` | Small (~100B) | 0.0054 | 0.491 | **91x** | 0 | 19 |
| `ascii_downcase` in select | Small (~100B) | 0.011 | 0.565 | **54x** | 0 | 21 |
| `startswith("s")` in select | Small (~100B) | 0.011 | 0.567 | **52x** | 0 | 21 |
| `endswith("s")` in select | Small (~100B) | 0.011 | 0.567 | **52x** | 0 | 21 |
| `ltrimstr("s")` | Small (~100B) | 0.0060 | 0.408 | **68x** | 0 | 16 |
| `rtrimstr("s")` | Small (~100B) | 0.0061 | 0.412 | **68x** | 0 | 16 |
| Alternative `.f // "default"` | Small (~100B) | 0.0072 | 0.456 | **63x** | 0 | 17 |

## Key Takeaways

- **fastjq achieves 0 allocations** across all operations in steady state when using `RunWithBuffer` or `RunFunc`
- **Fastest on small inputs** where gojq's marshal/unmarshal overhead dominates (up to 63x faster)
- **Still significantly faster on large inputs** (2.5x–5x) where both engines are scanning lots of data
- **Compound select (and/or) is ~56–60x faster**: each boolean operand is evaluated via `execSingle` with no closures; gojq must unmarshal the full object
- **`to_entries` at 6 ns**: just an objectIter reformatting pass — zero-alloc, nearly as fast as a simple field access
- **Large JSON exposes gojq's unmarshal tax**: gojq Field/Del/Construct on 100KB is 540–800 µs vs fastjq's 109–218 µs, because gojq pays the full parse cost up front regardless of which fields are accessed

## Raw Output

```
BenchmarkFastjq_Small_Del-16            	 7231394	       156.4 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Medium_Del-16           	  533917	      2240 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Del-16            	   10000	    128710 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Field-16          	 8185920	       137.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Field-16          	   10000	    110219 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Index-16          	46221399	        25.43 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ArrayDel-16       	13094140	        90.45 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Construct-16      	 4511020	       243.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Construct-16      	    5940	    206220 ns/op	       1 B/op	       0 allocs/op
BenchmarkFastjq_Small_Iterator-16       	39724246	        30.84 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Iterator-16       	  125816	      9353 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Select-16         	127535204	         9.412 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Select-16         	   70533	     15886 ns/op	      52 B/op	       0 allocs/op
BenchmarkFastjq_Small_Alternative-16    	166693396	         7.202 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SelectAnd-16      	100000000	        10.95 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_SelectOr-16       	100000000	        10.95 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Has-16            	100000000	        11.60 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_IfThenElse-16     	122896479	         9.767 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Length-16         	190670870	         6.268 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Map-16            	  618403	      1992 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ToEntries-16      	192997694	         6.237 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_WithEntries-16    	223646692	         5.358 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Del-16              	 1236974	       962.8 ns/op	    2594 B/op	      33 allocs/op
BenchmarkGojq_Medium_Del-16             	   63924	     18917 ns/op	   16967 B/op	     323 allocs/op
BenchmarkGojq_Large_Del-16              	    1486	    798991 ns/op	  539192 B/op	    4666 allocs/op
BenchmarkGojq_Small_Field-16            	 3622412	       335.4 ns/op	    1177 B/op	      13 allocs/op
BenchmarkGojq_Large_Field-16            	    2100	    574230 ns/op	  270049 B/op	    2835 allocs/op
BenchmarkGojq_Small_Index-16            	 1951258	       614.7 ns/op	    1401 B/op	      20 allocs/op
BenchmarkGojq_Small_ArrayDel-16         	  722425	      1644 ns/op	    3362 B/op	      53 allocs/op
BenchmarkGojq_Small_Construct-16        	 1735911	       693.3 ns/op	    1857 B/op	      23 allocs/op
BenchmarkGojq_Large_Construct-16        	    2079	    582535 ns/op	  274545 B/op	    2867 allocs/op
BenchmarkGojq_Small_Iterator-16         	 1599500	       746.3 ns/op	    1776 B/op	      26 allocs/op
BenchmarkGojq_Large_Iterator-16         	   14175	     84583 ns/op	  109808 B/op	    1811 allocs/op
BenchmarkGojq_Small_Select-16           	 2073288	       574.2 ns/op	    1753 B/op	      20 allocs/op
BenchmarkGojq_Large_Select-16           	    1522	    769832 ns/op	  539361 B/op	    4651 allocs/op
BenchmarkGojq_Small_Alternative-16      	 2649824	       455.9 ns/op	    1441 B/op	      17 allocs/op
BenchmarkGojq_Small_SelectAnd-16        	 1993189	       606.5 ns/op	    1945 B/op	      21 allocs/op
BenchmarkGojq_Small_SelectOr-16         	 1838578	       652.7 ns/op	    1945 B/op	      21 allocs/op
BenchmarkGojq_Small_Has-16              	 2147979	       553.7 ns/op	    1753 B/op	      20 allocs/op
BenchmarkGojq_Small_IfThenElse-16       	 2712096	       446.3 ns/op	    1361 B/op	      16 allocs/op
BenchmarkGojq_Small_Length-16           	 3338461	       363.0 ns/op	    1177 B/op	      13 allocs/op
BenchmarkGojq_Small_Map-16              	  117776	     10116 ns/op	   13653 B/op	     251 allocs/op
BenchmarkGojq_Small_ToEntries-16        	 3356006	       359.4 ns/op	    1209 B/op	      14 allocs/op
BenchmarkGojq_Small_WithEntries-16      	 2439589	       490.9 ns/op	    1529 B/op	      19 allocs/op
BenchmarkFastjq_Small_AsciiDowncase-16  	100000000	        10.55 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Startswith-16     	100000000	        10.52 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Endswith-16       	100000000	        10.58 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Ltrimstr-16       	197886992	         6.044 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Rtrimstr-16       	198682116	         6.061 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_AsciiDowncase-16    	 2136060	       565.1 ns/op	    1785 B/op	      21 allocs/op
BenchmarkGojq_Small_Startswith-16       	 2127511	       566.5 ns/op	    1801 B/op	      21 allocs/op
BenchmarkGojq_Small_Endswith-16         	 2098352	       567.3 ns/op	    1801 B/op	      21 allocs/op
BenchmarkGojq_Small_Ltrimstr-16         	 2939564	       407.9 ns/op	    1305 B/op	      16 allocs/op
BenchmarkGojq_Small_Rtrimstr-16         	 2907825	       412.4 ns/op	    1305 B/op	      16 allocs/op
```

## CLI Throughput: fastjq vs jq

End-to-end JSONL throughput benchmarks comparing `fastjq-bench` (Go CLI using the fastjq library) against `jq` (C implementation, v1.8.1). Both read JSONL from stdin, apply a filter per line, write results to `/dev/null`. Median of 3 runs. Apple M4 Max.

| Operation | Input | jq (s) | fastjq (s) | Speedup |
|-----------|-------|--------|-------------|---------|
| Identity (`.`) | small (100K lines, ~11MB) | 0.323 | 0.031 | **10.4x** |
| Field access (`.field_2`) | small | 0.149 | 0.024 | **6.2x** |
| Field access (`.field_50`) | large (100 lines, ~16MB) | 0.085 | 0.022 | **3.9x** |
| Delete field (`del(.field_2)`) | small | 0.336 | 0.033 | **10.2x** |
| Object construction (`{field_0, field_2}`) | small | 0.246 | 0.048 | **5.1x** |
| Select all match (`select(.field_2 == "xxx...")`) | small | 0.368 | 0.031 | **11.9x** |
| Select none match (`select(.field_2 == "nope")`) | small | 0.131 | 0.030 | **4.4x** |
| Alternative (`.field_2 // "default"`) | small | 0.154 | 0.029 | **5.3x** |

### Key Takeaways (CLI)

- **4x–12x faster** than jq across all operations on real JSONL workloads
- **Select (all match) is 12x faster**: the most important filter for log processing
- **Identity and deletion are 10x faster**: validates the zero-copy architecture at scale
- **Even "large" objects (100KB each) show 4x speedup**: scanning advantage persists

### Reproducing

```bash
chmod +x bench_vs_jq.sh
./bench_vs_jq.sh
```

## Reproducing (Go Benchmarks)

```bash
go test -bench=. -benchmem
```
