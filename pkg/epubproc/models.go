package epubproc

// SearchRequestRegex represents regex search configuration.
type SearchRequestRegex struct {
	Pattern string `json:"pattern"`
}

// SearchRequestText represents text search configuration.
type SearchRequestText struct {
	Value      string `json:"value"`
	IgnoreCase bool   `json:"ignoreCase"`
}

// SearchRequestQuery represents the query configuration for searching.
type SearchRequestQuery struct {
	Regex *SearchRequestRegex `json:"regex,omitempty"`
	Text  *SearchRequestText  `json:"text,omitempty"`
}

// FilterMatchMode controls how metadata filter values are compared against extracted metadata.
type FilterMatchMode string

const (
	// FilterMatchExact requires the filter value to equal the full field, case-insensitively (default).
	FilterMatchExact FilterMatchMode = "exact"

	// FilterMatchWord matches if the filter value appears as a whole word within the field,
	// case-insensitively (e.g. "tolkien" matches "J.R.R. Tolkien" but not "tolkienesque").
	FilterMatchWord FilterMatchMode = "word"
)

// SearchRequestFilters represents filters used for searching.
type SearchRequestFilters struct {
	AuthorEquals string   `json:"authorEquals,omitempty"`
	SeriesEquals string   `json:"seriesEquals,omitempty"`
	TitleEquals  string   `json:"titleEquals,omitempty"`
	FilesIn      []string `json:"filesIn,omitempty"`

	// MatchMode controls how AuthorEquals/SeriesEquals/TitleEquals are compared. Empty/omitted
	// behaves as FilterMatchExact, so existing library consumers see no behavior change.
	MatchMode FilterMatchMode `json:"matchMode,omitempty"`
}

// SearchRequest represents the configuration for searching within epub files.
type SearchRequest struct {
	Query   SearchRequestQuery    `json:"query"`
	Filters *SearchRequestFilters `json:"filters,omitempty"`

	// Context is the number of context lines to show around each match.
	Context int `json:"context"`
}

// Metadata represents the complete metadata extracted from an epub file.
type Metadata struct {
	Title          string   `json:"title"`
	Authors        []string `json:"authors"`
	Genres         []string `json:"genres"`
	Series         string   `json:"series"`
	SeriesPosition float64  `json:"seriesPosition"`
	YearReleased   int      `json:"yearReleased"`

	// Identifiers contains book identifiers (ISBN, ASIN, DOI, etc.), keyed by identifier type.
	Identifiers map[string]string `json:"identifiers"`
}

// opfMeta represents a <meta> tag in the OPF file.
type opfMeta struct {
	Name     string `xml:"name,attr"`
	Content  string `xml:"content,attr"`
	Property string `xml:"property,attr"`
	Scheme   string `xml:"scheme,attr"`
	Value    string `xml:",chardata"`
}

// opfIdentifier represents an identifier element in the OPF metadata.
type opfIdentifier struct {
	ID     string `xml:"id,attr"`
	Scheme string `xml:"scheme,attr"`
	Value  string `xml:",chardata"`
}

// opfPackageFile represents the package file (.opf) in an epub.
type opfPackageFile struct {
	Metadata struct {
		Title      string          `xml:"title"`
		Creator    []string        `xml:"creator"`
		Subject    []string        `xml:"subject"`
		Date       string          `xml:"date"`
		Identifier []opfIdentifier `xml:"identifier"`
		Meta       []opfMeta       `xml:"meta"`
	} `xml:"metadata"`
}

// containerXML represents the container.xml file in an epub.
type containerXML struct {
	Rootfiles []rootfile `xml:"rootfiles>rootfile"`
}

// rootfile represents a <rootfile> element in container.xml.
type rootfile struct {
	FullPath string `xml:"full-path,attr"`

	// MediaType is the media type of the root file, typically "application/oebps-package+xml".
	MediaType string `xml:"media-type,attr"`
}

// MatchMetadata represents extracted metadata from a single search result.
type MatchMetadata struct {
	// Chapter is the name of the chapter, if found.
	Chapter *string `json:"chapter,omitempty"`
}

// Match represents a single search result found within an epub file.
type Match struct {
	// Line is the text line containing the match, including any context lines.
	Line string `json:"line"`

	// FileName is the name of the file inside the epub where the match was found.
	FileName string `json:"fileName"`

	// Metadata is optional match metadata, populated only if metadata extraction is enabled and a chapter is found.
	Metadata *MatchMetadata `json:"metadata,omitempty"`
}

// SearchResult represents the complete search result for a single epub file.
type SearchResult struct {
	Path     string `json:"path"`
	Metadata `json:"metadata"`
	Matches  []Match `json:"matches"`
}
