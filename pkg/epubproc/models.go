package epubproc

// SearchRequestRegex represents regex search configuration.
type SearchRequestRegex struct {
	// Pattern is the regex pattern to match
	Pattern string `json:"pattern"`
}

// SearchRequestText represents text search configuration.
type SearchRequestText struct {
	// Value is the text to search for
	Value string `json:"value"`

	// IgnoreCase controls whether to perform case-insensitive search
	IgnoreCase bool `json:"ignoreCase"`
}

// SearchRequestQuery represents the query configuration for searching.
type SearchRequestQuery struct {
	// Regex contains regex search configuration
	Regex *SearchRequestRegex `json:"regex,omitempty"`

	// IsRegex indicates whether this is a regex search
	IsRegex bool `json:"isRegex"`

	// Text contains text search configuration
	Text *SearchRequestText `json:"text,omitempty"`
}

// SearchRequestFilters represents filters used for searching.
type SearchRequestFilters struct {
	// AuthorEquals will filter search results to a specific author
	AuthorEquals string `json:"authorEquals,omitempty"`

	// SeriesEquals will filter search results to a specific series
	SeriesEquals string `json:"seriesEquals,omitempty"`

	// TitleEquals will filter search results to a specific title
	TitleEquals string `json:"titleEquals,omitempty"`

	// FilesIn will filter search results to a specific list of files
	FilesIn []string `json:"filesIn,omitempty"`
}

// SearchRequest represents the configuration for searching within epub files.
type SearchRequest struct {
	// Query contains the search query configuration
	Query SearchRequestQuery `json:"query"`

	// Filters contains optional search filters
	Filters *SearchRequestFilters `json:"filters,omitempty"`

	// Context is the number of context lines to show around each match
	Context int `json:"context"`
}

// Metadata represents the complete metadata extracted from an epub file.
type Metadata struct {
	// Title is the book's title.
	Title string `json:"title"`

	// Authors is the list of book authors.
	Authors []string `json:"authors"`

	// Genres is the list of book genres.
	Genres []string `json:"genres"`

	// Series is the name of the book series, if applicable.
	Series string `json:"series"`

	// SeriesPosition is the position within the series.
	SeriesPosition float64 `json:"seriesPosition"`

	// YearReleased is the year the book was published.
	YearReleased int `json:"yearReleased"`

	// Identifiers contains book identifiers (ISBN, ASIN, DOI, etc.).
	Identifiers map[string]string `json:"identifiers"`
}

// opfMeta represents a <meta> tag in the OPF file.
type opfMeta struct {
	// Name is the name attribute of the meta tag.
	Name string `xml:"name,attr"`

	// Content is the content attribute of the meta tag.
	Content string `xml:"content,attr"`

	// Property is the property attribute of the meta tag.
	Property string `xml:"property,attr"`

	// Scheme is the scheme attribute of the meta tag.
	Scheme string `xml:"scheme,attr"`

	// Value is the text content of the meta tag.
	Value string `xml:",chardata"`
}

// opfIdentifier represents an identifier element in the OPF metadata.
type opfIdentifier struct {
	// ID is the id attribute of the identifier element.
	ID string `xml:"id,attr"`

	// Scheme is the scheme attribute specifying the identifier type.
	Scheme string `xml:"scheme,attr"`

	// Value is the identifier value.
	Value string `xml:",chardata"`
}

// opfPackageFile represents the package file (.opf) in an epub.
type opfPackageFile struct {
	// Metadata contains the metadata section of the OPF file.
	Metadata struct {
		// Title is the book title from the OPF metadata.
		Title string `xml:"title"`

		// Creator is the list of creators (authors) from the OPF metadata.
		Creator []string `xml:"creator"`

		// Subject is the list of subjects (genres) from the OPF metadata.
		Subject []string `xml:"subject"`

		// Date is the publication date from the OPF metadata.
		Date string `xml:"date"`

		// Identifier is the list of identifiers from the OPF metadata.
		Identifier []opfIdentifier `xml:"identifier"`

		// Meta is the list of meta elements from the OPF metadata.
		Meta []opfMeta `xml:"meta"`
	} `xml:"metadata"`
}

// containerXML represents the container.xml file in an epub.
type containerXML struct {
	// Rootfiles contains the list of root files in the epub.
	Rootfiles []rootfile `xml:"rootfiles>rootfile"`
}

// rootfile represents a <rootfile> element in container.xml.
type rootfile struct {
	// FullPath is the path to the OPF file relative to the epub root.
	FullPath string `xml:"full-path,attr"`

	// MediaType is the media type of the root file, typically "application/oebps-package+xml".
	MediaType string `xml:"media-type,attr"`
}

// Match represents a single search result found within an epub file.
type Match struct {
	// The text line containing the match, including any context lines.
	Line string `json:"line"`

	// The name of the file inside the epub where the match was found.
	FileName string `json:"fileName"`
}

// SearchResult represents the complete search result for a single epub file.
type SearchResult struct {
	// Path to the epub file.
	Path string `json:"path"`

	// Metadata of the epub file.
	Metadata `json:"metadata"`

	// A list of matches found in the epub file.
	Matches []Match `json:"matches"`
}
