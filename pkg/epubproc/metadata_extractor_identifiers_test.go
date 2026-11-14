package epubproc

import (
	"context"
	"maps"
	"path/filepath"
	"testing"
)

// TestExtractIdentifierFromProperty verifies the EPUB3 property names that count as identifiers.
func TestExtractIdentifierFromProperty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		property string
		want     string
	}{
		{"isbn", "isbn"},
		{"ISBN", "isbn"},
		{"  doi ", "doi"},
		{"issn", "issn"},
		{"oclc", "oclc"},
		{"lccn", "lccn"},
		{"title-type", ""},
		{"role", ""},
		{"belongs-to-collection", ""},
		{"", ""},
	}

	for _, test := range tests {
		t.Run(test.property, func(t *testing.T) {
			t.Parallel()

			if got := extractIdentifierFromProperty(test.property); got != test.want {
				t.Errorf("extractIdentifierFromProperty(%q) = %q, want %q", test.property, got, test.want)
			}
		})
	}
}

// TestExtractIdentifierFromMetaName verifies which EPUB2 meta names are read as identifiers.
func TestExtractIdentifierFromMetaName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
	}{
		{"calibre:isbn", "isbn"},
		{"CALIBRE:ISBN", "isbn"},
		{"calibre:asin", "asin"},
		{"calibre:doi", "doi"},
		{"calibre:issn", "issn"},
		{"calibre:oclc", "oclc"},
		{"calibre:lccn", "lccn"},
		{"calibre:google_id", "google"},
		{"calibre:goodreads_id", "goodreads"},
		{"calibre:amazon_id", "amazon"},
		// the generic calibre "<type>_id" fallback
		{"calibre:custom_id", "custom"},
		// these names need the value to find the type, so no key can come from the name
		{"dc:identifier", ""},
		{"dtb:uid", ""},
		// not identifiers
		{"calibre:series", ""},
		{"calibre:series_index", ""},
		{"cover", ""},
		{"", ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := extractIdentifierFromMetaName(test.name); got != test.want {
				t.Errorf("extractIdentifierFromMetaName(%q) = %q, want %q", test.name, got, test.want)
			}
		})
	}
}

// TestNormalizeIdentifierKeyAllSchemes covers the schemes that TestIdentifierNormalization does not.
func TestNormalizeIdentifierKeyAllSchemes(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"ISSN":      "issn",
		"oclc":      "oclc",
		"LCCN":      "lccn",
		"Google":    "google",
		"url":       "uri",
		"URI":       "uri",
		"  isbn  ":  "isbn",
		"uuid":      "uuid",
		"CustomKey": "customkey",
	}

	for scheme, want := range tests {
		t.Run(scheme, func(t *testing.T) {
			t.Parallel()

			if got := normalizeIdentifierKey(scheme); got != want {
				t.Errorf("normalizeIdentifierKey(%q) = %q, want %q", scheme, got, want)
			}
		})
	}
}

// TestDetectIdentifierTypePrefixes covers URN, URL and UUID values that have no scheme attribute.
func TestDetectIdentifierTypePrefixes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		value string
		want  string
	}{
		{"urn:uuid:123e4567-e89b-12d3-a456-426614174000", "urn"},
		{"URN:ISBN:9781234567890", "urn"},
		{"HTTPS://example.com/book", "uri"},
		{"https://example.com/book", "uri"},
		// a bare UUID has no prefix to detect, and must not be mistaken for an ISBN
		{"123e4567-e89b-12d3-a456-426614174000", ""},
		{"  978-1-23456-789-0  ", "isbn"},
		{"123456789X", "isbn"},
	}

	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			t.Parallel()

			if got := detectIdentifierType(test.value); got != test.want {
				t.Errorf("detectIdentifierType(%q) = %q, want %q", test.value, got, test.want)
			}
		})
	}
}

// TestProcessFileIdentifierSources verifies that identifiers are read from every place an OPF can hold them:
// the scheme attribute, a value that identifies itself, EPUB2 calibre meta tags, and EPUB3 meta properties.
func TestProcessFileIdentifierSources(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ids.epub")

	files := map[string]string{
		"META-INF/container.xml": `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`,
		"OEBPS/content.opf": `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="uid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">
    <dc:title>Identifier Book</dc:title>
    <dc:identifier id="uid" opf:scheme="ISBN">9781234567890</dc:identifier>
    <dc:identifier>urn:uuid:123e4567-e89b-12d3-a456-426614174000</dc:identifier>
    <meta name="calibre:google_id" content="g-123"/>
    <meta name="calibre:series" content="Not An Identifier"/>
    <meta property="doi">10.1000/123456</meta>
    <meta property="role">not-an-identifier</meta>
  </metadata>
</package>`,
	}
	if err := createTestZIPWithFiles(path, files); err != nil {
		t.Fatalf("Failed to create test ePUB: %v", err)
	}

	metadata, err := NewMetadataExtractor(1).ProcessFile(context.Background(), path)
	if err != nil {
		t.Fatalf("ProcessFile failed: %v", err)
	}

	want := map[string]string{
		"isbn":   "9781234567890",
		"urn":    "urn:uuid:123e4567-e89b-12d3-a456-426614174000",
		"google": "g-123",
		"doi":    "10.1000/123456",
	}
	if !maps.Equal(metadata.Identifiers, want) {
		t.Errorf("Identifiers = %v, want %v", metadata.Identifiers, want)
	}
	if metadata.Series != "Not An Identifier" {
		t.Errorf("Series = %q, want %q", metadata.Series, "Not An Identifier")
	}
}
