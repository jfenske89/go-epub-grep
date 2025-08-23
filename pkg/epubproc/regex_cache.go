package epubproc

import (
	"regexp"
	"sync"
)

// regexCache provides thread-safe caching of compiled regular expressions.
// This significantly improves performance when the same patterns are used repeatedly.
type regexCache struct {
	mu       sync.RWMutex
	cache    map[string]*regexp.Regexp
	maxSize  int
	accesses map[string]int // Track access frequency for LRU-like eviction
}

// newRegexCache creates a new regex cache with the specified maximum size.
func newRegexCache(maxSize int) *regexCache {
	return &regexCache{
		cache:    make(map[string]*regexp.Regexp),
		maxSize:  maxSize,
		accesses: make(map[string]int),
	}
}

// get retrieves a compiled regex from the cache or compiles and caches a new one.
func (rc *regexCache) get(pattern string) (*regexp.Regexp, error) {
	// Try read lock first for better concurrency
	rc.mu.RLock()
	if re, ok := rc.cache[pattern]; ok {
		rc.mu.RUnlock()
		// Update access count with write lock
		rc.mu.Lock()
		rc.accesses[pattern]++
		rc.mu.Unlock()
		return re, nil
	}
	rc.mu.RUnlock()

	// Need write lock to compile and cache
	rc.mu.Lock()
	defer rc.mu.Unlock()

	// Double-check after acquiring write lock (another goroutine might have added it)
	if re, ok := rc.cache[pattern]; ok {
		rc.accesses[pattern]++
		return re, nil
	}

	// Compile the pattern
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	// Evict least recently used if at capacity
	if len(rc.cache) >= rc.maxSize {
		var lruPattern string
		minAccess := int(^uint(0) >> 1) // Max int
		for p, count := range rc.accesses {
			if count < minAccess {
				minAccess = count
				lruPattern = p
			}
		}
		delete(rc.cache, lruPattern)
		delete(rc.accesses, lruPattern)
	}

	// Cache the compiled regex
	rc.cache[pattern] = re
	rc.accesses[pattern] = 1

	return re, nil
}

// Global regex cache with reasonable size limit
var patternCache = newRegexCache(128)
