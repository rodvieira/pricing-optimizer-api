package llm

import (
	"fmt"
	"strings"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
)

// classifySiteSystemPrompt instructs the model to always answer via the
// classification tool call.
func classifySiteSystemPrompt() string {
	var b strings.Builder
	b.WriteString("You are a product marketing analyst classifying a scraped web page. ")
	fmt.Fprintf(&b, "Call the %s tool exactly once with your complete answer. ", classifySiteToolName)
	b.WriteString("Never respond in plain text.\n")
	return b.String()
}

// classifySiteUserPrompt describes the scraped page content to classify.
func classifySiteUserPrompt(page domain.ScrapedPage) string {
	var b strings.Builder
	fmt.Fprintf(&b, "URL: %s\n", page.URL)
	fmt.Fprintf(&b, "Title: %s\n", page.Title)
	fmt.Fprintf(&b, "Page text:\n%s\n", page.Text)
	return b.String()
}
