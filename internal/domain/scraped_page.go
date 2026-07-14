package domain

import (
	"fmt"
	"strings"
)

// ScrapedPage is the raw content extracted from a URL, before LLM
// classification into a SiteProfile.
type ScrapedPage struct {
	URL        string
	Title      string
	Text       string
	SourceType SourceType
}

// Validate checks that a scrape produced enough to classify. A scraper
// implementation returning a page that fails this is signaling it could not
// extract meaningful content, not that the content was merely short.
func (p ScrapedPage) Validate() error {
	if p.URL == "" {
		return fmt.Errorf("%w: missing url", ErrEmptyScrape)
	}
	if strings.TrimSpace(p.Text) == "" {
		return fmt.Errorf("%w: no text extracted from %s", ErrEmptyScrape, p.URL)
	}
	return nil
}
