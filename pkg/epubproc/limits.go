package epubproc

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"math"
)

// entryLimits caps how much data is read from an epub. An epub is a zip file, and a small zip can expand to a very
// large amount of data, so a damaged or hostile file must not be able to use all memory or CPU of the host program.
type entryLimits struct {
	// xml is the largest size of one package, table of contents or container file.
	xml int64

	// content is the largest size of one text or HTML file.
	content int64

	// total is the largest combined size of all text and HTML files that are scanned in one epub.
	total int64
}

// limits is a variable, not a constant, so that tests can use small values instead of building very large files.
// The values are far above the size of real books, where the largest single files are a few megabytes.
var limits = entryLimits{
	xml:     32 << 20,
	content: 64 << 20,
	total:   1 << 30,
}

// errEntryTooLarge is the cause when an epub is over one of the limits.
var errEntryTooLarge = errors.New("epub entry is over the size limit")

// checkEntrySize rejects an entry whose declared size is over max. The zip reader fails an entry that holds more
// or fewer bytes than its header says, so the declared size can be trusted before any data is read.
func checkEntrySize(f *zip.File, max int64) error {
	if size := declaredSize(f); size > max {
		return fmt.Errorf("entry %q is %d bytes and the limit is %d: %w", f.Name, size, max, errEntryTooLarge)
	}
	return nil
}

// declaredSize returns the unpacked size from the zip header. A size that does not fit in an int64 is reported as the
// largest int64, which is over every limit.
func declaredSize(f *zip.File) int64 {
	if f.UncompressedSize64 > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(f.UncompressedSize64)
}

// limitedReader reads at most left bytes and then fails with errEntryTooLarge. Unlike io.LimitReader, it does not
// end the data quietly, which would make a cut-off file look like a complete one.
type limitedReader struct {
	r    io.Reader
	left int64
}

func newLimitedReader(r io.Reader, max int64) *limitedReader {
	return &limitedReader{r: r, left: max}
}

func (l *limitedReader) Read(p []byte) (int, error) {
	if l.left < 0 {
		return 0, errEntryTooLarge
	}

	// ask for one byte more than is allowed, so that going over the limit is noticed
	if int64(len(p)) > l.left+1 {
		p = p[:l.left+1]
	}

	n, err := l.r.Read(p)
	l.left -= int64(n)
	if l.left < 0 {
		// drop the extra byte, because it is over the limit
		return n - 1, errEntryTooLarge
	}
	return n, err
}
