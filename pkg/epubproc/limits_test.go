package epubproc

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// withLimits sets small limits for one test and restores the old ones afterwards. Tests that use it must not run in
// parallel, because the limits are shared by the whole package.
func withLimits(t *testing.T, l entryLimits) {
	t.Helper()

	old := limits
	limits = l
	t.Cleanup(func() { limits = old })
}

// TestLimitedReader verifies that a read stops with an error, not with a quiet end of data, when it is over the limit.
func TestLimitedReader(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		max      int64
		wantData string
		wantErr  bool
	}{
		{"under the limit", "abc", 10, "abc", false},
		{"exactly at the limit", "abcde", 5, "abcde", false},
		{"one byte over the limit", "abcdef", 5, "abcde", true},
		{"far over the limit", strings.Repeat("x", 1000), 10, strings.Repeat("x", 10), true},
		{"empty data with a zero limit", "", 0, "", false},
		{"data with a zero limit", "a", 0, "", true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := io.ReadAll(newLimitedReader(strings.NewReader(test.data), test.max))

			if string(got) != test.wantData {
				t.Errorf("expected data %q, got %q", test.wantData, got)
			}
			if test.wantErr != errors.Is(err, errEntryTooLarge) {
				t.Errorf("expected errEntryTooLarge = %t, got error %v", test.wantErr, err)
			}
			if !test.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestLimitedReaderSmallReads verifies the limit also holds when the caller reads one byte at a time.
func TestLimitedReaderSmallReads(t *testing.T) {
	reader := newLimitedReader(strings.NewReader("abcdef"), 4)
	buf := make([]byte, 1)

	var got []byte
	var err error
	for {
		var n int
		n, err = reader.Read(buf)
		got = append(got, buf[:n]...)
		if err != nil {
			break
		}
	}

	if string(got) != "abcd" || !errors.Is(err, errEntryTooLarge) {
		t.Errorf("expected %q and errEntryTooLarge, got %q and %v", "abcd", got, err)
	}
}

// TestGrepInEpubLimits verifies the size limits on the files that are scanned.
func TestGrepInEpubLimits(t *testing.T) {
	big := strings.Repeat("a", 2048) + " target"

	tests := []struct {
		name    string
		limits  entryLimits
		files   map[string]string
		wantErr bool
	}{
		{
			name:    "one file over the file limit",
			limits:  entryLimits{xml: 1 << 20, content: 1024, total: 1 << 20},
			files:   map[string]string{"chapter.html": "<p>" + big + "</p>"},
			wantErr: true,
		},
		{
			name:   "files under the limits",
			limits: entryLimits{xml: 1 << 20, content: 1 << 20, total: 1 << 20},
			files:  map[string]string{"chapter.html": "<p>" + big + "</p>"},
		},
		{
			name:    "each file is small but together they are over the total",
			limits:  entryLimits{xml: 1 << 20, content: 4096, total: 5000},
			files:   map[string]string{"a.html": "<p>" + big + "</p>", "b.html": "<p>" + big + "</p>", "c.txt": big},
			wantErr: true,
		},
		{
			name:   "a large image is not scanned and does not count",
			limits: entryLimits{xml: 1 << 20, content: 1024, total: 1024},
			files:  map[string]string{"cover.png": big, "style.css": big, "chapter.html": "<p>target</p>"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			withLimits(t, test.limits)

			path := filepath.Join(t.TempDir(), "book.epub")
			if err := createTestZIPWithFiles(path, test.files); err != nil {
				t.Fatalf("Failed to create test ePUB: %v", err)
			}

			matches, err := grepInEpub(context.Background(), path, mustPattern("target"), 0)

			if test.wantErr {
				if !errors.Is(err, errEntryTooLarge) {
					t.Fatalf("expected errEntryTooLarge, got: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("grepInEpub failed: %v", err)
			}
			if len(matches) == 0 {
				t.Error("expected matches in a book under the limits")
			}
		})
	}
}

// TestProcessXMLFileLimit verifies that a table of contents over the xml limit is skipped.
func TestProcessXMLFileLimit(t *testing.T) {
	withLimits(t, entryLimits{xml: 16, content: 1 << 20, total: 1 << 20})

	data := []byte("<ncx>" + strings.Repeat("x", 100) + "</ncx>")
	file := zipEntryFile(t, &zip.FileHeader{
		Name:               "toc.ncx",
		Method:             zip.Store,
		CRC32:              crc32.ChecksumIEEE(data),
		CompressedSize64:   uint64(len(data)),
		UncompressedSize64: uint64(len(data)),
	}, data)

	called := false
	processXMLFile(file, func([]byte) { called = true })

	if called {
		t.Error("expected the handler not to run for a file over the xml limit")
	}
}

// TestProcessFileXMLLimit verifies that metadata extraction fails for a package file over the xml limit.
func TestProcessFileXMLLimit(t *testing.T) {
	withLimits(t, entryLimits{xml: 64, content: 1 << 20, total: 1 << 20})

	path := filepath.Join(t.TempDir(), "book.epub")
	files := map[string]string{
		"content.opf": `<package><metadata><title>` + strings.Repeat("x", 200) + `</title></metadata></package>`,
	}
	if err := createTestZIPWithFiles(path, files); err != nil {
		t.Fatalf("Failed to create test ePUB: %v", err)
	}

	_, err := NewMetadataExtractor(1).ProcessFile(context.Background(), path)

	if !errors.Is(err, errEntryTooLarge) {
		t.Errorf("expected errEntryTooLarge, got: %v", err)
	}
}

// TestSearchSkipsOversizedBook verifies that one book over a limit is skipped and the others are still returned.
func TestSearchSkipsOversizedBook(t *testing.T) {
	withLimits(t, entryLimits{xml: 1 << 20, content: 1024, total: 1 << 20})

	dir := t.TempDir()
	mustCreateEPUB(t, dir, "small.epub", "<p>target</p>")
	mustCreateEPUB(t, dir, "huge.epub", "<p>target "+strings.Repeat("a", 4096)+"</p>")

	got := searchAll(t, NewFileSearch(dir, 2, false), textRequest("target", false))

	if len(got) != 1 || filepath.Base(got[0].Path) != "small.epub" {
		t.Errorf("expected only small.epub, got %+v", got)
	}
}

// writeZeroEntry adds an entry of n zero bytes. Zeros compress to almost nothing, so this is a small file on disk
// that would be very large if it were read, like a real zip bomb.
func writeZeroEntry(t *testing.T, writer *zip.Writer, name string, n int) {
	t.Helper()

	entry, err := writer.Create(name)
	if err != nil {
		t.Fatalf("Failed to create zip entry: %v", err)
	}

	chunk := bytes.Repeat([]byte{0}, 1<<20)
	for written := 0; written < n; written += len(chunk) {
		if _, err := entry.Write(chunk); err != nil {
			t.Fatalf("Failed to write zip entry: %v", err)
		}
	}
}

// TestDefaultLimitsStopZipBombs uses the real limits with files that are small on disk and large when unpacked.
func TestDefaultLimitsStopZipBombs(t *testing.T) {
	if testing.Short() {
		t.Skip("builds files that unpack to tens of megabytes")
	}

	dir := t.TempDir()

	createBomb := func(name, entry string, size int) string {
		path := filepath.Join(dir, name)
		file, err := os.Create(path)
		if err != nil {
			t.Fatalf("Failed to create %s: %v", path, err)
		}
		defer file.Close()

		writer := zip.NewWriter(file)
		writeZeroEntry(t, writer, entry, size)
		if err := writer.Close(); err != nil {
			t.Fatalf("Failed to close zip: %v", err)
		}
		return path
	}

	t.Run("package file", func(t *testing.T) {
		path := createBomb("opf.epub", "content.opf", int(limits.xml)+(1<<20))

		_, err := NewMetadataExtractor(1).ProcessFile(context.Background(), path)

		if !errors.Is(err, errEntryTooLarge) {
			t.Errorf("expected errEntryTooLarge, got: %v", err)
		}
	})

	t.Run("html file", func(t *testing.T) {
		path := createBomb("html.epub", "chapter.html", int(limits.content)+(1<<20))

		_, err := grepInEpub(context.Background(), path, mustPattern("target"), 0)

		if !errors.Is(err, errEntryTooLarge) {
			t.Errorf("expected errEntryTooLarge, got: %v", err)
		}
	})
}

func mustPattern(pattern string) *regexp.Regexp {
	return regexp.MustCompile(pattern)
}
