package epubproc

import (
	"archive/zip"
	"bytes"
	"context"
	"hash/crc32"
	"maps"
	"regexp"
	"strings"
	"testing"
)

// TestScanHTMLFileInlineTags verifies that inline tags do not add or remove spaces, so a phrase that is
// split by markup can still be found, while block tags still separate lines.
func TestScanHTMLFileInlineTags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		html     string
		pattern  string
		wantLine string // empty means no match is expected
	}{
		{
			name:     "punctuation after an inline tag",
			html:     "<p><i>word</i>, then more</p>",
			pattern:  `word, then`,
			wantLine: "word, then more",
		},
		{
			name:     "one word split by a span",
			html:     "<p><span>T</span>he end</p>",
			pattern:  `The end`,
			wantLine: "The end",
		},
		{
			name:     "spaces from the source are kept around inline tags",
			html:     "<p>Hello <b>big</b> world</p>",
			pattern:  `Hello big world`,
			wantLine: "Hello big world",
		},
		{
			name:     "nested inline tags",
			html:     "<p>a<em>b<strong>c</strong></em>d</p>",
			pattern:  `abcd`,
			wantLine: "abcd",
		},
		{
			name:    "paragraphs stay on separate lines",
			html:    "<p>one</p><p>two</p>",
			pattern: `one two`,
		},
		{
			name:    "table cells stay on separate lines",
			html:    "<table><tr><td>a</td><td>b</td></tr></table>",
			pattern: `ab`,
		},
		{
			name:     "line break separates lines",
			html:     "<p>first<br/>second</p>",
			pattern:  `^second$`,
			wantLine: "second",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			matches := scanHTMLFile(context.Background(), strings.NewReader(test.html),
				regexp.MustCompile(test.pattern), "chapter.html", 0)

			if test.wantLine == "" {
				if len(matches) != 0 {
					t.Fatalf("Expected no match, got %+v", matches)
				}
				return
			}

			if len(matches) != 1 {
				t.Fatalf("Expected 1 match, got %d: %+v", len(matches), matches)
			}
			if matches[0].Line != test.wantLine {
				t.Errorf("Expected line %q, got %q", test.wantLine, matches[0].Line)
			}
		})
	}
}

// zipEntryFile builds an in-memory zip with one entry and returns that entry. The header is used as given, so a
// test can create an entry that cannot be opened or read.
func zipEntryFile(t *testing.T, header *zip.FileHeader, rawData []byte) *zip.File {
	t.Helper()

	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)

	entry, err := writer.CreateRaw(header)
	if err != nil {
		t.Fatalf("Failed to create zip entry: %v", err)
	}
	if _, err := entry.Write(rawData); err != nil {
		t.Fatalf("Failed to write zip entry: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Failed to close zip writer: %v", err)
	}

	reader, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("Failed to read zip: %v", err)
	}
	if len(reader.File) != 1 {
		t.Fatalf("Expected 1 zip entry, got %d", len(reader.File))
	}

	return reader.File[0]
}

// TestProcessXmlFile verifies that the handler runs only when the entry can be opened and fully read.
func TestProcessXmlFile(t *testing.T) {
	t.Parallel()

	data := []byte("<ncx>content</ncx>")

	tests := []struct {
		name        string
		header      zip.FileHeader
		wantHandled bool
	}{
		{
			name: "valid entry",
			header: zip.FileHeader{
				Name:               "toc.ncx",
				Method:             zip.Store,
				CRC32:              crc32.ChecksumIEEE(data),
				CompressedSize64:   uint64(len(data)),
				UncompressedSize64: uint64(len(data)),
			},
			wantHandled: true,
		},
		{
			// open fails because no decompressor is registered for this method
			name: "unsupported compression method",
			header: zip.FileHeader{
				Name:               "toc.ncx",
				Method:             99,
				CompressedSize64:   uint64(len(data)),
				UncompressedSize64: uint64(len(data)),
			},
		},
		{
			// open works, but reading to the end fails the checksum
			name: "checksum mismatch",
			header: zip.FileHeader{
				Name:               "toc.ncx",
				Method:             zip.Store,
				CRC32:              crc32.ChecksumIEEE(data) + 1,
				CompressedSize64:   uint64(len(data)),
				UncompressedSize64: uint64(len(data)),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			header := test.header
			file := zipEntryFile(t, &header, data)

			var got []byte
			var handled bool
			processXMLFile(file, func(xmlBytes []byte) {
				handled = true
				got = xmlBytes
			})

			if handled != test.wantHandled {
				t.Fatalf("Expected handler called = %t, got %t", test.wantHandled, handled)
			}
			if handled && !bytes.Equal(got, data) {
				t.Errorf("Expected handler to receive %q, got %q", data, got)
			}
		})
	}
}

// TestScanHTMLFileIgnoresCode verifies that the text of <script> and <style> is not searchable, and that the
// text after them is.
func TestScanHTMLFileIgnoresCode(t *testing.T) {
	t.Parallel()

	page := `<html><head><style>.hidden-token { color: red; }</style>
<script>var hiddenToken = "hidden-token";</script></head>
<body><p>visible text</p><script>other("hidden-token")</script><p>after code</p></body></html>`

	tests := []struct {
		pattern string
		want    int
	}{
		{"hidden-token", 0},
		{"hiddenToken", 0},
		{"visible text", 1},
		{"after code", 1},
	}

	for _, test := range tests {
		t.Run(test.pattern, func(t *testing.T) {
			t.Parallel()

			matches := scanHTMLFile(context.Background(), strings.NewReader(page), regexp.MustCompile(test.pattern), "page.html", 0)

			if len(matches) != test.want {
				t.Errorf("Expected %d matches for %q, got %+v", test.want, test.pattern, matches)
			}
		})
	}
}

// TestProcessTableOfContentsFirstHeadingWins verifies the chapter name for a file that has several nav points.
func TestProcessTableOfContentsFirstHeadingWins(t *testing.T) {
	t.Parallel()

	ncx := []byte(`<?xml version="1.0"?>
<ncx xmlns="http://www.daisy.org/z3986/2005/ncx/"><navMap>
<navPoint id="a"><navLabel><text>Chapter One</text></navLabel><content src="text/ch1.html#start"/></navPoint>
<navPoint id="b"><navLabel><text>Chapter One, Part Two</text></navLabel><content src="text/ch1.html#part2"/></navPoint>
<navPoint id="c"><navLabel><text>Chapter Two</text></navLabel><content src="text/ch2.html"/></navPoint>
<navPoint id="d"><navLabel><text>Chapter Two, Notes</text></navLabel><content src="text/ch2.html#notes"/></navPoint>
</navMap></ncx>`)

	file := zipEntryFile(t, &zip.FileHeader{
		Name:               "toc.ncx",
		Method:             zip.Store,
		CRC32:              crc32.ChecksumIEEE(ncx),
		CompressedSize64:   uint64(len(ncx)),
		UncompressedSize64: uint64(len(ncx)),
	}, ncx)

	got := processTableOfContents(file)

	want := map[string]string{
		"ch1.html#start": "Chapter One",
		"ch1.html#part2": "Chapter One, Part Two",
		"ch1.html":       "Chapter One",
		"ch2.html":       "Chapter Two",
		"ch2.html#notes": "Chapter Two, Notes",
	}
	if !maps.Equal(got, want) {
		t.Errorf("chapter map = %v, want %v", got, want)
	}
}
