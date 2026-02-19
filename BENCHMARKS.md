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
| Select `select(.f == "x")` | Small (~100B) | 0.0075 | 0.558 | **74x** | 0 | 20 |
| Select `select(.f == "x")` | Large (~100KB, last field) | 21 | 788 | **38x** | 0 | 4,651 |
| Alternative `.f // "default"` | Small (~100B) | 0.0062 | 0.453 | **73x** | 0 | 17 |

## Key Takeaways

- **fastjq achieves 0 allocations** across all operations when using `RunWithBuffer` or `RunFunc`
- **Fastest on small inputs** where gojq's marshal/unmarshal overhead dominates (up to 74x faster)
- **Still significantly faster on large inputs** (2.5x–5x) where both engines are scanning lots of data
- **Select is 74x faster on small, 38x faster on large**: the Large Select benchmark scans the full 170KB
  to find the last field — even at worst case, fastjq avoids the unmarshal/marshal overhead entirely
- **Large JSON exposes gojq's unmarshal tax**: gojq Field/Del/Construct on 100KB is 540–766 µs vs fastjq's
  109–218 µs, because gojq pays the full parse cost up front regardless of which fields are accessed

## Raw Output

```
BenchmarkFastjq_Small_Del-16            	 7749934	       158.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Medium_Del-16           	  526438	      2691 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Del-16            	    9807	    155444 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Field-16          	 8433512	       143.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Field-16          	   10000	    109431 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Index-16          	48809836	        25.04 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ArrayDel-16       	13376950	        88.92 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Construct-16      	 4529384	       263.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Construct-16      	    5274	    218483 ns/op	       1 B/op	       0 allocs/op
BenchmarkFastjq_Small_Iterator-16       	35375359	        31.28 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Iterator-16       	  134950	      9738 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Select-16         	159180238	         7.544 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Select-16         	   59142	     20796 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Alternative-16    	193015660	         6.217 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Del-16              	 1343878	       892.4 ns/op	    2594 B/op	      33 allocs/op
BenchmarkGojq_Medium_Del-16             	   66754	     17979 ns/op	   16969 B/op	     323 allocs/op
BenchmarkGojq_Large_Del-16              	    1557	    765652 ns/op	  542571 B/op	    4666 allocs/op
BenchmarkGojq_Small_Field-16            	 3685694	       326.8 ns/op	    1177 B/op	      13 allocs/op
BenchmarkGojq_Large_Field-16            	    2192	    542715 ns/op	  270053 B/op	    2835 allocs/op
BenchmarkGojq_Small_Index-16            	 2042554	       588.0 ns/op	    1401 B/op	      20 allocs/op
BenchmarkGojq_Small_ArrayDel-16         	  750957	      1575 ns/op	    3362 B/op	      53 allocs/op
BenchmarkGojq_Small_Construct-16        	 1794679	       664.9 ns/op	    1857 B/op	      23 allocs/op
BenchmarkGojq_Large_Construct-16        	    2066	    553048 ns/op	  274364 B/op	    2867 allocs/op
BenchmarkGojq_Small_Iterator-16         	 1651243	       729.3 ns/op	    1776 B/op	      26 allocs/op
BenchmarkGojq_Large_Iterator-16         	   14850	     80491 ns/op	  109808 B/op	    1811 allocs/op
BenchmarkGojq_Small_Select-16           	 2165504	       558.4 ns/op	    1753 B/op	      20 allocs/op
BenchmarkGojq_Large_Select-16           	    1465	    787551 ns/op	  535609 B/op	    4651 allocs/op
BenchmarkGojq_Small_Alternative-16      	 2627552	       453.4 ns/op	    1441 B/op	      17 allocs/op
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
