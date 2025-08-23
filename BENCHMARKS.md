# Performance Benchmarks

This document describes the performance benchmarks available and how to use them to monitor performance.

## Running Benchmarks

### Basic Benchmark Run
```bash
# Run all benchmarks
go test -bench=. ./pkg/epubproc/

# Run benchmarks with specific duration
go test -bench=. -benchtime=10s ./pkg/epubproc/

# Run benchmarks with memory allocation tracking
go test -bench=. -benchmem ./pkg/epubproc/
```

### Performance Analysis
```bash
# Profile CPU usage during benchmarks
go test -bench=. -cpuprofile=cpu.prof ./pkg/epubproc/

# Profile memory usage during benchmarks  
go test -bench=. -memprofile=mem.prof ./pkg/epubproc/
```

## Performance Baseline (Apple M1 Pro)

### Text File Scanning
| File Size | Time/op | Allocs/op | B/op |
|-----------|---------|-----------|------|
| Small (100 lines) | ~8.8μs | 103 | 7,126 |
| Medium (1K lines) | ~86.8μs | 1,003 | 65,335 |  
| Large (10K lines) | ~868μs | 10,007 | 658,302 |

### HTML File Scanning
| File Size | Time/op | Allocs/op | B/op |
|-----------|---------|-----------|------|
| Small (100 elements) | ~65.8μs | 606 | 42,129 |
| Medium (1K elements) | ~651μs | 6,012 | 401,926 |
| Large (10K elements) | ~3.3ms | 30,022 | 2,158,284 |

### Memory Usage Analysis
| Content Type | Size | Time/op | Memory/op | Allocs/op |
|--------------|------|---------|-----------|-----------|
| Text | Small (100) | ~9.1μs | 7,134 B | 103 |
| Text | Medium (1K) | ~87.1μs | 65,399 B | 1,003 |
| Text | Large (5K) | ~441μs | 328,923 B | 5,005 |
| Text | XLarge (10K) | ~901μs | 660,676 B | 10,008 |
| HTML | Small (100) | ~33.5μs | 23,472 B | 305 |
| HTML | Medium (1K) | ~327μs | 200,120 B | 3,010 |
| HTML | Large (5K) | ~1.7ms | 1,047,555 B | 15,017 |
| HTML | XLarge (10K) | ~3.4ms | 2,155,914 B | 30,022 |

### Concurrency Performance
| Workers | Time/op | Memory/op | Allocs/op |
|---------|---------|-----------|-----------|
| 1 | ~29.1μs | 20,213 B | 305 |
| 2 | ~47.8μs | 41,150 B | 609 |
| 4 | ~82.6μs | 83,838 B | 1,217 |
| 8 | ~120.4μs | 172,602 B | 2,435 |
| 16 | ~213.0μs | 357,990 B | 4,872 |
| 32 | ~405.2μs | 712,892 B | 9,744 |

### Pattern Complexity Impact
| Pattern Type | Time/op | Performance vs Simple |
|--------------|---------|----------------------|
| Simple Regex | ~87.0μs | Baseline |
| Complex Regex | ~133.0μs | +53% slower |
| Case Insensitive | ~850μs | +877% slower |
