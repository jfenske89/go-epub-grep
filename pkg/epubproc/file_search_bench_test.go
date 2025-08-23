package epubproc

import (
	"context"
	"fmt"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// generateLargeTextContent creates a large text file content for benchmarking.
func generateLargeTextContent(lines int, pattern string) string {
	// initialize string builder and pre-allocate memory
	var builder strings.Builder
	builder.Grow(lines * 100)

	for i := range lines {
		if i%100 == 0 {
			// insert the pattern every 100 lines
			builder.WriteString(fmt.Sprintf("Line %d contains %s for testing purposes.\n", i, pattern))
		} else {
			builder.WriteString(fmt.Sprintf("Line %d has some regular content without any special words.\n", i))
		}
	}

	return builder.String()
}

// generateLargeHTMLContent creates a large HTML file content for benchmarking.
func generateLargeHTMLContent(elements int, pattern string) string {
	// initialize string builder and pre-allocate memory
	var builder strings.Builder
	builder.Grow(elements * 200)

	builder.WriteString("<html><body>\n")

	for i := range elements {
		if i%50 == 0 {
			// insert the pattern every 50 elements
			builder.WriteString(fmt.Sprintf("<p>Element %d contains %s for testing purposes.</p>\n", i, pattern))
		} else {
			builder.WriteString(fmt.Sprintf("<div>Element %d has some regular content without any special words.</div>\n", i))
		}
	}

	builder.WriteString("</body></html>\n")
	return builder.String()
}

// BenchmarkScanTextFile_Small benchmarks text file scanning with small files.
func BenchmarkScanTextFile_Small(b *testing.B) {
	content := generateLargeTextContent(100, "target")
	pattern, _ := regexp.Compile("target")

	b.ReportAllocs()

	for b.Loop() {
		reader := strings.NewReader(content)
		matches := scanTextFile(reader, pattern, "test.txt", 0)
		if len(matches) == 0 {
			b.Fatal("Expected matches but got none")
		}
	}
}

// BenchmarkScanTextFile_Medium benchmarks text file scanning with medium files.
func BenchmarkScanTextFile_Medium(b *testing.B) {
	content := generateLargeTextContent(1000, "target")
	pattern, _ := regexp.Compile("target")

	b.ReportAllocs()

	for b.Loop() {
		reader := strings.NewReader(content)
		matches := scanTextFile(reader, pattern, "test.txt", 0)
		if len(matches) == 0 {
			b.Fatal("Expected matches but got none")
		}
	}
}

// BenchmarkScanTextFile_Large benchmarks text file scanning with large files.
func BenchmarkScanTextFile_Large(b *testing.B) {
	content := generateLargeTextContent(10000, "target")
	pattern, _ := regexp.Compile("target")

	b.ReportAllocs()

	for b.Loop() {
		reader := strings.NewReader(content)
		matches := scanTextFile(reader, pattern, "test.txt", 0)
		if len(matches) == 0 {
			b.Fatal("Expected matches but got none")
		}
	}
}

// BenchmarkScanTextFile_WithContext benchmarks text scanning with context lines.
func BenchmarkScanTextFile_WithContext(b *testing.B) {
	content := generateLargeTextContent(1000, "target")
	pattern, _ := regexp.Compile("target")

	b.ReportAllocs()

	for b.Loop() {
		reader := strings.NewReader(content)
		matches := scanTextFile(reader, pattern, "test.txt", 2)
		if len(matches) == 0 {
			b.Fatal("Expected matches but got none")
		}
	}
}

// BenchmarkScanHTMLFile_Small benchmarks HTML file scanning with small files.
func BenchmarkScanHTMLFile_Small(b *testing.B) {
	content := generateLargeHTMLContent(100, "target")
	pattern, _ := regexp.Compile("target")
	ctx := context.Background()

	b.ReportAllocs()

	for b.Loop() {
		reader := strings.NewReader(content)
		matches := scanHTMLFile(ctx, reader, pattern, "test.html", 0)
		if len(matches) == 0 {
			b.Fatal("Expected matches but got none")
		}
	}
}

// BenchmarkScanHTMLFile_Medium benchmarks HTML file scanning with medium files.
func BenchmarkScanHTMLFile_Medium(b *testing.B) {
	content := generateLargeHTMLContent(1000, "target")
	pattern, _ := regexp.Compile("target")
	ctx := context.Background()

	b.ReportAllocs()

	for b.Loop() {
		reader := strings.NewReader(content)
		matches := scanHTMLFile(ctx, reader, pattern, "test.html", 0)
		if len(matches) == 0 {
			b.Fatal("Expected matches but got none")
		}
	}
}

// BenchmarkScanHTMLFile_Large benchmarks HTML file scanning with large files.
func BenchmarkScanHTMLFile_Large(b *testing.B) {
	content := generateLargeHTMLContent(5000, "target")
	pattern, _ := regexp.Compile("target")
	ctx := context.Background()

	b.ReportAllocs()

	for b.Loop() {
		reader := strings.NewReader(content)
		matches := scanHTMLFile(ctx, reader, pattern, "test.html", 0)
		if len(matches) == 0 {
			b.Fatal("Expected matches but got none")
		}
	}
}

// BenchmarkScanHTMLFile_WithContext benchmarks HTML scanning with context lines.
func BenchmarkScanHTMLFile_WithContext(b *testing.B) {
	content := generateLargeHTMLContent(1000, "target")
	pattern, _ := regexp.Compile("target")
	ctx := context.Background()

	b.ReportAllocs()

	for b.Loop() {
		reader := strings.NewReader(content)
		matches := scanHTMLFile(ctx, reader, pattern, "test.html", 2)
		if len(matches) == 0 {
			b.Fatal("Expected matches but got none")
		}
	}
}

// BenchmarkConcurrentTextScanning benchmarks concurrent text file scanning.
func BenchmarkConcurrentTextScanning(b *testing.B) {
	content := generateLargeTextContent(500, "target")
	pattern, _ := regexp.Compile("target")
	numWorkers := runtime.NumCPU()

	b.ReportAllocs()

	for b.Loop() {
		var wg sync.WaitGroup
		for range numWorkers {
			wg.Go(func() {
				reader := strings.NewReader(content)
				matches := scanTextFile(reader, pattern, "test.txt", 0)
				if len(matches) == 0 {
					b.Error("Expected matches but got none")
				}
			})
		}

		wg.Wait()
	}
}

// BenchmarkConcurrentHTMLScanning benchmarks concurrent HTML file scanning.
func BenchmarkConcurrentHTMLScanning(b *testing.B) {
	content := generateLargeHTMLContent(500, "target")
	pattern, _ := regexp.Compile("target")
	numWorkers := runtime.NumCPU()
	ctx := context.Background()

	b.ReportAllocs()

	for b.Loop() {
		var wg sync.WaitGroup
		for range numWorkers {
			wg.Go(func() {
				reader := strings.NewReader(content)
				matches := scanHTMLFile(ctx, reader, pattern, "test.html", 0)
				if len(matches) == 0 {
					b.Error("Expected matches but got none")
				}
			})
		}

		wg.Wait()
	}
}

// BenchmarkRegexVsTextSearch compares regex pattern matching vs simple text search.
func BenchmarkRegexVsTextSearch(b *testing.B) {
	content := generateLargeTextContent(1000, "target")

	b.Run("SimpleRegex", func(b *testing.B) {
		pattern, _ := regexp.Compile("target")
		b.ResetTimer()
		b.ReportAllocs()

		for b.Loop() {
			reader := strings.NewReader(content)
			matches := scanTextFile(reader, pattern, "test.txt", 0)
			if len(matches) == 0 {
				b.Fatal("Expected matches but got none")
			}
		}
	})

	b.Run("ComplexRegex", func(b *testing.B) {
		pattern, _ := regexp.Compile("t[aeiou]rg[aeiou]t")
		b.ResetTimer()
		b.ReportAllocs()

		for b.Loop() {
			reader := strings.NewReader(content)
			matches := scanTextFile(reader, pattern, "test.txt", 0)
			if len(matches) == 0 {
				b.Fatal("Expected matches but got none")
			}
		}
	})

	b.Run("CaseInsensitiveRegex", func(b *testing.B) {
		pattern, _ := regexp.Compile("(?i)TARGET")
		b.ResetTimer()
		b.ReportAllocs()

		for b.Loop() {
			reader := strings.NewReader(content)
			matches := scanTextFile(reader, pattern, "test.txt", 0)
			if len(matches) == 0 {
				b.Fatal("Expected matches but got none")
			}
		}
	})
}

// BenchmarkPoolEffectiveness measures the effectiveness of object pooling.
func BenchmarkPoolEffectiveness(b *testing.B) {
	content := generateLargeTextContent(200, "target")
	pattern, _ := regexp.Compile("target")

	b.Run("WithPool", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()

		for b.Loop() {
			reader := strings.NewReader(content)
			matches := scanTextFile(reader, pattern, "test.txt", 0)
			if len(matches) == 0 {
				b.Fatal("Expected matches but got none")
			}
		}
	})
}

// BenchmarkMemoryUsage provides insights into memory usage patterns.
func BenchmarkMemoryUsage(b *testing.B) {
	sizes := []struct {
		name  string
		lines int
	}{
		{"Small_100", 100},
		{"Medium_1000", 1000},
		{"Large_5000", 5000},
		{"ExtraLarge_10000", 10000},
	}

	pattern, _ := regexp.Compile("target")

	for _, size := range sizes {
		b.Run("Text_"+size.name, func(b *testing.B) {
			content := generateLargeTextContent(size.lines, "target")
			b.ResetTimer()
			b.ReportAllocs()

			var m1, m2 runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&m1)

			for b.Loop() {
				reader := strings.NewReader(content)
				matches := scanTextFile(reader, pattern, "test.txt", 0)
				if len(matches) == 0 {
					b.Fatal("Expected matches but got none")
				}
			}

			runtime.GC()
			runtime.ReadMemStats(&m2)

			b.ReportMetric(float64(m2.TotalAlloc-m1.TotalAlloc)/float64(b.N), "alloc/op")
		})

		b.Run("HTML_"+size.name, func(b *testing.B) {
			content := generateLargeHTMLContent(size.lines/2, "target") // HTML is more verbose
			ctx := context.Background()
			b.ResetTimer()
			b.ReportAllocs()

			var m1, m2 runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&m1)

			for b.Loop() {
				reader := strings.NewReader(content)
				matches := scanHTMLFile(ctx, reader, pattern, "test.html", 0)
				if len(matches) == 0 {
					b.Fatal("Expected matches but got none")
				}
			}

			runtime.GC()
			runtime.ReadMemStats(&m2)

			b.ReportMetric(float64(m2.TotalAlloc-m1.TotalAlloc)/float64(b.N), "alloc/op")
		})
	}
}

// BenchmarkHighConcurrency tests performance under high concurrent load.
func BenchmarkHighConcurrency(b *testing.B) {
	concurrencyLevels := []int{1, 2, 4, 8, 16, 32}
	content := generateLargeTextContent(300, "target")
	pattern, _ := regexp.Compile("target")

	for _, concurrency := range concurrencyLevels {
		b.Run(fmt.Sprintf("Concurrency_%d", concurrency), func(b *testing.B) {
			b.ResetTimer()
			b.ReportAllocs()

			for b.Loop() {
				var wg sync.WaitGroup
				for range concurrency {
					wg.Go(func() {
						reader := strings.NewReader(content)
						matches := scanTextFile(reader, pattern, "test.txt", 0)
						if len(matches) == 0 {
							b.Error("Expected matches but got none")
						}
					})
				}

				wg.Wait()
			}
		})
	}
}
