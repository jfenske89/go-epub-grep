package epubproc

import (
	"sync"
	"testing"
)

// TestRegexCacheCreation verifies that regex cache is created correctly.
func TestRegexCacheCreation(t *testing.T) {
	cache := newRegexCache(10)

	if cache == nil {
		t.Fatal("Expected regex cache, got nil")
	}

	if cache.maxSize != 10 {
		t.Errorf("Expected max size 10, got %d", cache.maxSize)
	}

	if cache.cache == nil {
		t.Fatal("Expected cache map to be initialized")
	}

	if cache.accesses == nil {
		t.Fatal("Expected accesses map to be initialized")
	}
}

// TestRegexCacheGet verifies that patterns are cached and retrieved correctly.
func TestRegexCacheGet(t *testing.T) {
	cache := newRegexCache(5)

	// first get should compile and cache
	pattern := "test.*pattern"
	re1, err := cache.get(pattern)
	if err != nil {
		t.Fatalf("Failed to get pattern: %v", err)
	} else if re1 == nil {
		t.Fatal("Expected compiled regex, got nil")
	}

	// second get should return cached version
	re2, err := cache.get(pattern)
	if err != nil {
		t.Fatalf("Failed to get cached pattern: %v", err)
	}

	// should be the same instance
	if re1 != re2 {
		t.Error("Expected same regex instance from cache")
	}

	// check access count increased
	if cache.accesses[pattern] != 2 {
		t.Errorf("Expected access count 2, got %d", cache.accesses[pattern])
	}
}

// TestRegexCacheInvalidPattern verifies that invalid patterns return errors.
func TestRegexCacheInvalidPattern(t *testing.T) {
	cache := newRegexCache(5)

	// invalid regex pattern
	_, err := cache.get("[invalid")
	if err == nil {
		t.Error("Expected error for invalid pattern, got nil")
	}
}

// TestRegexCacheLRUEviction verifies that least recently used patterns are evicted.
func TestRegexCacheLRUEviction(t *testing.T) {
	cache := newRegexCache(3)

	// fill cache
	patterns := []string{"pattern1", "pattern2", "pattern3"}
	for _, p := range patterns {
		_, err := cache.get(p)
		if err != nil {
			t.Fatalf("Failed to cache pattern %s: %v", p, err)
		}
	}

	// access pattern1 and pattern2 again to increase their access count
	cache.get("pattern1")
	cache.get("pattern2")

	// add a new pattern - should evict pattern3 (least accessed)
	_, err := cache.get("pattern4")
	if err != nil {
		t.Fatalf("Failed to cache pattern4: %v", err)
	}

	// check that pattern3 was evicted
	if _, exists := cache.cache["pattern3"]; exists {
		t.Error("Expected pattern3 to be evicted")
	}

	// check that other patterns still exist
	if _, exists := cache.cache["pattern1"]; !exists {
		t.Error("Expected pattern1 to still be cached")
	} else if _, exists := cache.cache["pattern2"]; !exists {
		t.Error("Expected pattern2 to still be cached")
	} else if _, exists := cache.cache["pattern4"]; !exists {
		t.Error("Expected pattern4 to be cached")
	}
}

// TestRegexCacheConcurrency verifies thread-safe access to the cache.
func TestRegexCacheConcurrency(t *testing.T) {
	cache := newRegexCache(50)

	var wg sync.WaitGroup
	patterns := []string{"p1", "p2", "p3", "p4", "p5"}

	// multiple goroutines accessing the same patterns
	for i := range 10 {
		id := i
		wg.Go(func() {
			for _, p := range patterns {
				_, err := cache.get(p)
				if err != nil {
					t.Errorf("Goroutine %d: Failed to get pattern %s: %v", id, p, err)
				}
			}
		})
	}

	wg.Wait()

	// all patterns should be cached
	for _, p := range patterns {
		if _, exists := cache.cache[p]; !exists {
			t.Errorf("Expected pattern %s to be cached", p)
		}
	}
}
