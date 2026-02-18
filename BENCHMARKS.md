# fastjq Benchmarks

Benchmarks comparing fastjq against [gojq](https://github.com/itchyny/gojq). gojq benchmarks include the full `json.Unmarshal` → execute → `json.Marshal` cycle, since that's the real-world cost of using a traditional jq engine from Go.

All benchmarks run with `go test -bench=. -benchmem`. Results from Apple M4 Max, Go 1.25.

## Summary

| Operation | Input | fastjq | gojq | Speedup | fastjq allocs | gojq allocs |
|-----------|-------|--------|------|---------|---------------|-------------|
| Field access | Small (~100B) | 146 ns | 918 ns | **6.3x** | 0 | 27 |
| Field access | Large (~100KB) | 114 us | 542 us | **4.7x** | 0 | 2,835 |
| Field deletion | Small (~100B) | 156 ns | 2,091 ns | **13x** | 0 | 58 |
| Field deletion | Medium (~2KB) | 2,451 ns | 17,600 ns | **7.2x** | 0 | 323 |
| Field deletion | Large (~100KB) | 152 us | 752 us | **4.9x** | 0 | 4,666 |
| Array index `.[2]` | 5-elem array | 24 ns | 573 ns | **24x** | 0 | 20 |
| Array deletion `del(.[1],.[3])` | 5-elem array | 88 ns | 1,534 ns | **17x** | 0 | 53 |
| Object construction `{f0, f2}` | Small (~100B) | 286 ns | 1,274 ns | **4.5x** | 0 | 37 |
| Object construction `{f0, f50}` | Large (~100KB) | 213 us | 543 us | **2.6x** | 0 | 2,866 |
| Iterator `.[]` | 5-elem array | 31 ns | 712 ns | **23x** | 0 | 26 |
| Iterator `.[]` | 200-elem array | 15 us | 81 us | **5.5x** | 0 | 1,811 |

## Key Takeaways

- **fastjq achieves 0 allocations** across all operations when using `RunWithBuffer` or `RunFunc`
- **Fastest on small inputs** where gojq's marshal/unmarshal overhead dominates (up to 24x faster)
- **Still significantly faster on large inputs** (2.6x–5.5x) where both engines are scanning lots of data
- **Array operations are the biggest win**: indexing and iteration avoid all the overhead of building Go data structures

## Raw Output

```
BenchmarkFastjq_Small_Del-16          	 7652690	       156.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Medium_Del-16         	  462328	      2451 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Del-16          	   10000	    152418 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Field-16        	 8185726	       146.3 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Field-16        	    9751	    114470 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Index-16        	47190139	        24.16 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_ArrayDel-16     	13622598	        88.14 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Small_Construct-16    	 4069268	       286.2 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Construct-16    	    6255	    212768 ns/op	       1 B/op	       0 allocs/op
BenchmarkFastjq_Small_Iterator-16     	39160761	        30.53 ns/op	       0 B/op	       0 allocs/op
BenchmarkFastjq_Large_Iterator-16     	  109627	     14682 ns/op	       0 B/op	       0 allocs/op
BenchmarkGojq_Small_Del-16            	  545800	      2091 ns/op	    3763 B/op	      58 allocs/op
BenchmarkGojq_Medium_Del-16           	   69169	     17600 ns/op	   16967 B/op	     323 allocs/op
BenchmarkGojq_Large_Del-16            	    1615	    751774 ns/op	  535087 B/op	    4666 allocs/op
BenchmarkGojq_Small_Field-16          	 1284374	       917.5 ns/op	    1641 B/op	      27 allocs/op
BenchmarkGojq_Large_Field-16          	    2137	    542147 ns/op	  269948 B/op	    2835 allocs/op
BenchmarkGojq_Small_Index-16          	 2127420	       573.1 ns/op	    1401 B/op	      20 allocs/op
BenchmarkGojq_Small_ArrayDel-16       	  746670	      1534 ns/op	    3362 B/op	      53 allocs/op
BenchmarkGojq_Small_Construct-16      	  901066	      1274 ns/op	    2329 B/op	      37 allocs/op
BenchmarkGojq_Large_Construct-16      	    2224	    543140 ns/op	  274088 B/op	    2866 allocs/op
BenchmarkGojq_Small_Iterator-16       	 1681130	       711.7 ns/op	    1776 B/op	      26 allocs/op
BenchmarkGojq_Large_Iterator-16       	   15067	     80995 ns/op	  109808 B/op	    1811 allocs/op
```

## Reproducing

```bash
go test -bench=. -benchmem -count=3 -benchtime=2s
```
