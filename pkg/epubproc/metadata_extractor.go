package epubproc

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/sourcegraph/conc/pool"
)

// MetadataHandler defines a handler function for epub metadata.
type MetadataHandler func(epubPath string, metadata *Metadata) error

// MetadataExtractor defines the interface for extracting metadata from epub files.
type MetadataExtractor interface {
	// ProcessDirectory recursively processes epub files in a directory and passes metadata to a handler function.
	ProcessDirectory(ctx context.Context, epubDir string, handler MetadataHandler) error

	// ProcessFile extracts complete metadata from a single epub file.
	ProcessFile(ctx context.Context, epubPath string) (*Metadata, error)
}

type metadataExtractorImpl struct {
	// maxThreads is the maximum number of worker goroutines to use
	maxThreads int
}

// NewMetadataExtractor creates a new MetadataExtractor instance with the specified concurrency level.
func NewMetadataExtractor(maxThreads int) MetadataExtractor {
	if maxThreads <= 0 {
		// default to number of CPU cores if not specified
		maxThreads = runtime.NumCPU()
	}

	return &metadataExtractorImpl{
		maxThreads: maxThreads,
	}
}

// ProcessDirectory recursively processes epub files in a directory and extracts their metadata.
func (m *metadataExtractorImpl) ProcessDirectory(ctx context.Context, epubDir string, handler MetadataHandler) error {
	p := pool.New().WithContext(ctx).WithCancelOnError()
	paths := make(chan string)

	// track file counts for better error context
	var totalFiles, processedFiles, errorFiles int64
	var fileCountMutex sync.RWMutex

	// producer goroutine to find all .epub files
	p.Go(func(ctx context.Context) error {
		defer close(paths)
		return filepath.WalkDir(epubDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return fmt.Errorf("error walking directory '%s': %w", epubDir, err)
			}

			if !d.IsDir() && strings.HasSuffix(strings.ToLower(d.Name()), ".epub") {
				fileCountMutex.Lock()
				totalFiles++
				fileCountMutex.Unlock()

				select {
				case paths <- path:
				case <-ctx.Done():
					return ctx.Err()
				}
			}

			return nil
		})
	})

	// worker goroutines to process files
	for i := 0; i < m.maxThreads; i++ {
		p.Go(func(ctx context.Context) error {
			for path := range paths {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}

				metadata, err := m.ProcessFile(ctx, path)
				if err != nil {
					// a single corrupt file shouldn't stop the whole process.
					fileCountMutex.Lock()
					errorFiles++
					currentTotalFiles := totalFiles
					currentProcessedFiles := processedFiles
					currentErrorFiles := errorFiles
					fileCountMutex.Unlock()

					log.Err(err).
						Str("path", path).
						Int64("processed", currentProcessedFiles).
						Int64("errors", currentErrorFiles).
						Int64("total", currentTotalFiles).
						Msg("error processing file")
					continue
				}

				if err := handler(path, metadata); err != nil {
					// if handler returns an error, we cancel the context and return the error.
					return err
				}

				fileCountMutex.Lock()
				processedFiles++
				fileCountMutex.Unlock()
			}

			return nil
		})
	}

	err := p.Wait()

	// log final processing summary
	fileCountMutex.RLock()
	finalTotalFiles := totalFiles
	finalProcessedFiles := processedFiles
	finalErrorFiles := errorFiles
	fileCountMutex.RUnlock()

	if finalErrorFiles > 0 {
		log.Info().
			Str("directory", epubDir).
			Int64("total_found", finalTotalFiles).
			Int64("processed", finalProcessedFiles).
			Int64("errors", finalErrorFiles).
			Msg("completed directory processing with some errors")
	} else {
		log.Info().
			Str("directory", epubDir).
			Int64("total_processed", finalProcessedFiles).
			Msg("completed directory processing successfully")
	}

	return err
}

// ProcessFile extracts complete metadata from a single epub file.
func (m *metadataExtractorImpl) ProcessFile(ctx context.Context, epubPath string) (*Metadata, error) {
	// get file info for better error context
	fileInfo, fileErr := os.Stat(epubPath)

	r, err := zip.OpenReader(epubPath)
	if err != nil {
		if fileErr == nil {
			return nil, fmt.Errorf("failed to open epub '%s' (size: %d bytes): %w", epubPath, fileInfo.Size(), err)
		}
		return nil, fmt.Errorf("failed to open epub '%s': %w", epubPath, err)
	}
	defer func() {
		if err := r.Close(); err != nil {
			log.Warn().Err(err).Str("epub", epubPath).Msg("failed to close epub reader")
		}
	}()

	opfPath, err := findOpfPath(&r.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to find opf path in %s: %w", epubPath, err)
	}

	var opfFile *zip.File
	for _, f := range r.File {
		// OPF path may be relative to the root of the zip archive
		// need to handle cases where the path in container.xml is not clean.
		if f.Name == opfPath {
			opfFile = f
			break
		}
	}

	if opfFile == nil {
		return nil, fmt.Errorf("opf file '%s' not found in epub '%s'", opfPath, epubPath)
	}

	rc, err := opfFile.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open opf file '%s' in epub '%s': %w", opfPath, epubPath, err)
	}
	defer func() {
		if err := rc.Close(); err != nil {
			log.Warn().Err(err).Str("file", opfPath).Msg("failed to close opf file")
		}
	}()

	var opfData opfPackageFile
	decoder := xml.NewDecoder(rc)

	// some epubs have invalid charsets declared, but are utf-8
	// this is a common issue so configure the decoder to be lenient
	decoder.CharsetReader = func(charset string, input io.Reader) (io.Reader, error) {
		return input, nil
	}

	if err := decoder.Decode(&opfData); err != nil {
		return nil, fmt.Errorf("failed to parse opf file '%s' in epub '%s': %w", opfPath, epubPath, err)
	}

	metadata := &Metadata{
		Title:       opfData.Metadata.Title,
		Authors:     opfData.Metadata.Creator,
		Genres:      opfData.Metadata.Subject,
		Identifiers: make(map[string]string),
	}

	if opfData.Metadata.Date != "" {
		// date can be several formats: "2004", "2004-10-02", "2004-10-02T11:00:00Z", and we only want the year
		if t, err := time.Parse(time.RFC3339, opfData.Metadata.Date); err == nil {
			metadata.YearReleased = t.Year()
		} else if len(opfData.Metadata.Date) >= 4 {
			if year, err := strconv.Atoi(opfData.Metadata.Date[:4]); err == nil {
				metadata.YearReleased = year
			}
		}
	}

	// extract identifiers from <identifier> elements
	for _, identifier := range opfData.Metadata.Identifier {
		if identifier.Value != "" {
			key := normalizeIdentifierKey(identifier.Scheme)
			if key == "" {
				// no scheme, try to detect identifier type from the value
				key = detectIdentifierType(identifier.Value)
			}

			if key != "" {
				metadata.Identifiers[key] = strings.TrimSpace(identifier.Value)
			}
		}
	}

	for _, meta := range opfData.Metadata.Meta {
		switch meta.Name {
		case "calibre:series":
			metadata.Series = meta.Content
		case "calibre:series_index":
			if pos, err := strconv.ParseFloat(meta.Content, 64); err == nil {
				metadata.SeriesPosition = pos
			}
		}

		// extract identifiers from meta tags
		if meta.Name != "" && meta.Content != "" {
			key := extractIdentifierFromMetaName(meta.Name)
			if key != "" {
				metadata.Identifiers[key] = strings.TrimSpace(meta.Content)
			}
		}

		// handle EPUB3 property-based identifiers - only extract known identifier properties
		if meta.Property != "" && meta.Value != "" {
			key := extractIdentifierFromProperty(meta.Property)
			if key != "" {
				metadata.Identifiers[key] = strings.TrimSpace(meta.Value)
			}
		}
	}

	return metadata, nil
}

// findOpfPath locates the OPF (Open Packaging Format) file within an epub archive.
func findOpfPath(r *zip.Reader) (string, error) {
	var containerFile *zip.File
	for _, f := range r.File {
		if f.Name == "META-INF/container.xml" {
			containerFile = f
			break
		}
	}

	if containerFile == nil {
		// fallback for non-standard epubs: find the first .opf file.
		for _, f := range r.File {
			if strings.HasSuffix(strings.ToLower(f.Name), ".opf") {
				return f.Name, nil
			}
		}
		return "", fmt.Errorf("META-INF/container.xml not found and no .opf file in archive")
	}

	rc, err := containerFile.Open()
	if err != nil {
		return "", fmt.Errorf("failed to open container.xml: %w", err)
	}
	defer func() {
		if err := rc.Close(); err != nil {
			log.Err(err).Msg("failed to close container.xml")
		}
	}()

	var container containerXML
	if err := xml.NewDecoder(rc).Decode(&container); err != nil {
		return "", fmt.Errorf("failed to parse container.xml: %w", err)
	}

	for _, rf := range container.Rootfiles {
		if rf.MediaType == "application/oebps-package+xml" {
			return rf.FullPath, nil
		}
	}

	return "", fmt.Errorf("no OPF rootfile found in container.xml")
}

// normalizeIdentifierKey converts various identifier scheme names to standardized keys.
func normalizeIdentifierKey(scheme string) string {
	scheme = strings.ToLower(strings.TrimSpace(scheme))

	switch scheme {
	case "isbn", "isbn-10", "isbn-13":
		return "isbn"
	case "asin":
		return "asin"
	case "doi":
		return "doi"
	case "issn":
		return "issn"
	case "oclc":
		return "oclc"
	case "lccn":
		return "lccn"
	case "google":
		return "google"
	case "goodreads":
		return "goodreads"
	case "amazon":
		return "amazon"
	case "uri", "url":
		return "uri"
	default:
		return scheme
	}
}

// extractIdentifierFromMetaName extracts identifier keys from EPUB2-style meta name attributes.
func extractIdentifierFromMetaName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))

	// only extract from explicitly known identifier meta names
	switch name {
	case "dc:identifier", "dtb:uid":
		// these need scheme detection from content
		return ""
	case "calibre:isbn":
		return "isbn"
	case "calibre:asin":
		return "asin"
	case "calibre:doi":
		return "doi"
	case "calibre:issn":
		return "issn"
	case "calibre:oclc":
		return "oclc"
	case "calibre:lccn":
		return "lccn"
	case "calibre:google_id":
		return "google"
	case "calibre:goodreads_id":
		return "goodreads"
	case "calibre:amazon_id":
		return "amazon"
	}

	// try to fallback to calibre-style identifiers
	if strings.HasPrefix(name, "calibre:") && strings.HasSuffix(name, "_id") {
		// extract the identifier type from calibre meta names like "calibre:google_id"
		identType := strings.TrimSuffix(strings.TrimPrefix(name, "calibre:"), "_id")
		return normalizeIdentifierKey(identType)
	}

	return ""
}

// extractIdentifierFromProperty extracts identifier keys from EPUB3-style property attributes.
func extractIdentifierFromProperty(property string) string {
	property = strings.ToLower(strings.TrimSpace(property))

	// Only extract from standard ePUB3 identifier properties
	switch property {
	case "isbn":
		return "isbn"
	case "doi":
		return "doi"
	case "issn":
		return "issn"
	case "oclc":
		return "oclc"
	case "lccn":
		return "lccn"
	}

	return ""
}

// detectIdentifierType attempts to automatically detect the identifier type from its value.
func detectIdentifierType(value string) string {
	value = strings.TrimSpace(value)

	// remove common prefixes and clean the value
	cleanValue := strings.ReplaceAll(value, "-", "")
	cleanValue = strings.ReplaceAll(cleanValue, " ", "")

	// ISBN detection (10 or 13 digits)
	if len(cleanValue) == 10 || len(cleanValue) == 13 {
		if isNumeric(cleanValue) || (len(cleanValue) == 10 && isISBN10(cleanValue)) {
			return "isbn"
		}
	}

	// ASIN detection (10 alphanumeric characters starting with B)
	if len(value) == 10 && strings.HasPrefix(strings.ToUpper(value), "B") {
		return "asin"
	}

	// DOI detection (starts with "10.")
	if strings.HasPrefix(value, "10.") {
		return "doi"
	}

	// URL detection
	if strings.HasPrefix(strings.ToLower(value), "http://") ||
		strings.HasPrefix(strings.ToLower(value), "https://") {
		return "uri"
	}

	// URN detection
	if strings.HasPrefix(strings.ToLower(value), "urn:") {
		return "urn"
	}

	return ""
}

// isNumeric checks if a string contains only numeric digits (0-9).
func isNumeric(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// isISBN10 validates whether a string matches the ISBN-10 format.
func isISBN10(s string) bool {
	if len(s) != 10 {
		return false
	}

	for i, r := range s {
		if i == 9 && (r == 'X' || r == 'x') {
			// final character can be X
			continue
		} else if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
