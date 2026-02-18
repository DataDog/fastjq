# fastjq Benchmarks

Benchmarks comparing fastjq against [gojq](https://github.com/itchyny/gojq). gojq benchmarks include the full `json.Unmarshal` → execute → `json.Marshal` cycle, since that's the real-world cost of using a traditional jq engine from Go.

All benchmarks run with `go test -bench=. -benchmem`. Results from Apple M4 Max, Go 1.25.

> **Note on benchmark reliability**: gojq Large benchmarks use rotating input copies to prevent a Go 1.25
> calibration artifact where the auto-calibration pre-pass sees warm-cache hits, sets b.N far too high, and
> produces results identical to the Small benchmarks despite the 1000x size difference. All benchmarks use
> `b.Loop()` (Go 1.24+) and `benchSink` to prevent dead-code elimination.

## Summary

| Operation | Input | fastjq | gojq | Speedup | fastjq allocs | gojq allocs |
|-----------|-------|--------|------|---------|---------------|-------------|
| Field access | Small (~100B) | 141 ns | 318 ns | **2.3x** | 0 | 13 |
| Field access | Large (~100KB) | 103 µs | 527 µs | **5.1x** | 0 | 2,835 |
| Field deletion | Small (~100B) | 163 ns | 864 ns | **5.3x** | 0 | 33 |
| Field deletion | Medium (~2KB) | 2,479 ns | 17,195 ns | **6.9x** | 0 | 323 |
| Field deletion | Large (~100KB) | 130 µs | 730 µs | **5.6x** | 0 | 4,666 |
| Array index `.[2]` | 5-elem array | 25 ns | 575 ns | **23x** | 0 | 20 |
| Array deletion `del(.[1],.[3])` | 5-elem array | 88 ns | 1,509 ns | **17x** | 0 | 53 |
| Object construction `{f0, f2}` | Small (~100B) | 287 ns | 653 ns | **2.3x** | 0 | 23 |
| Object construction `{f0, f50}` | Large (~100KB) | 201 µs | 531 µs | **2.6x** | 0 | 2,867 |
| Iterator `.[]` | 5-elem array | 31 ns | 703 ns | **23x** | 0 | 26 |
| Iterator `.[]` | 200-elem array | 9.2 µs | 77 µs | **8.4x** | 0 | 1,811 |
| Select `select(.f == "x")` | Small (~100B) | 7.4 ns | 527 ns | **71x** | 0 | 20 |
| Select `select(.f == "x")` | Large (~100KB) | 7.4 ns | 710 µs | **96,000x** | 0 | 4,651 |
| Alternative `.f // "default"` | Small (~100B) | 6.1 ns | 431 ns | **71x** | 0 | 17 |

## Key Takeaways

- **fastjq achieves 0 allocations** across all operations when using `RunWithBuffer` or `RunFunc`
- **Fastest on small inputs** where gojq's marshal/unmarshal overhead dominates (up to 71x faster)
- **Still significantly faster on large inputs** (2.6x–5.6x) where both engines are scanning lots of data
- **Select/filter is 71x faster on small, 96,000x faster on large**: fastjq scans only to the compared field
  and returns early; gojq must unmarshal the full 170KB object before evaluating the condition
- **Large JSON exposes gojq's unmarshal tax**: gojq Field/Del/Construct on 100KB is 500–730 µs vs fastjq's
  100–200 µs, because gojq pays the full parse cost up front regardless of which fields are accessed

## Raw Output

```
BenchmarkFastjq_Small_Del-16            	 6568147	       162.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Medium_Del-16           	  444159	      2479 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Del-16            	   10000	    129783 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Field-16          	 7778565	       140.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Field-16          	   10000	    102527 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Index-16          	47918618	        24.74 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ArrayDel-16       	13452028	        87.60 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Construct-16      	 4102221	       286.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Construct-16      	    6504	    201181 ns/op	       1 B/op	       0 allocs/op
BenchmarkFastjq_Small_Iterator-16       	37619470	        31.35 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Iterator-16       	  116716	      9179 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Select-16         	161349463	         7.420 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Select-16         	162953089	         7.404 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Alternative-16    	197595048	         6.069 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Del-16              	 1383667	       864.4 ns/op	    2594 B/op	      33 allocs/op
BenchmarkGojq_Medium_Del-16             	   67668	     17195 ns/op	   16967 B/op	     323 allocs/op
BenchmarkGojq_Large_Del-16              	    1610	    730253 ns/op	  538454 B/op	    4666 allocs/op
BenchmarkGojq_Small_Field-16            	 3810121	       318.1 ns/op	    1177 B/op	      13 allocs/op
BenchmarkGojq_Large_Field-16            	    2281	    526765 ns/op	  270054 B/op	    2835 allocs/op
BenchmarkGojq_Small_Index-16            	 2074956	       575.4 ns/op	    1401 B/op	      20 allocs/op
BenchmarkGojq_Small_ArrayDel-16         	  796376	      1509 ns/op	    3362 B/op	      53 allocs/op
BenchmarkGojq_Small_Construct-16        	 1811300	       653.4 ns/op	    1857 B/op	      23 allocs/op
BenchmarkGojq_Large_Construct-16        	    2256	    531076 ns/op	  274369 B/op	    2867 allocs/op
BenchmarkGojq_Small_Iterator-16         	 1708077	       702.7 ns/op	    1776 B/op	      26 allocs/op
BenchmarkGojq_Large_Iterator-16         	   15484	     77388 ns/op	  109809 B/op	    1811 allocs/op
BenchmarkGojq_Small_Select-16           	 2275237	       526.7 ns/op	    1753 B/op	      20 allocs/op
BenchmarkGojq_Large_Select-16           	    1664	    710121 ns/op	  538453 B/op	    4651 allocs/op
BenchmarkGojq_Small_Alternative-16      	 2793718	       430.8 ns/op	    1441 B/op	      17 allocs/op
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
