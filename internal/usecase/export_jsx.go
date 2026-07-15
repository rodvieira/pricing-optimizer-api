package usecase

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
)

// renderJSX renders v as a standalone React functional component. Every
// dynamic string is embedded as a JSON-encoded JS string literal expression
// (e.g. {"Simple, transparent pricing"}) rather than interpolated into JSX
// markup directly: JSON string escaping neutralizes quotes, backslashes, and
// curly braces in LLM-generated content, so nothing the model produced can
// break out of the literal or be parsed as a nested JSX expression.
func renderJSX(v domain.Variation) *domain.ExportResult {
	var b strings.Builder

	b.WriteString("export default function PricingSection() {\n")
	b.WriteString("  return (\n")
	b.WriteString("    <section className=\"pricing-section\">\n")
	fmt.Fprintf(&b, "      <h2>{%s}</h2>\n", jsxString(v.Headline))
	if v.Subheadline != "" {
		fmt.Fprintf(&b, "      <p className=\"subheadline\">{%s}</p>\n", jsxString(v.Subheadline))
	}
	b.WriteString("      <div className=\"tiers\">\n")
	for _, t := range v.Tiers {
		writeJSXTier(&b, t)
	}
	b.WriteString("      </div>\n")
	b.WriteString("    </section>\n")
	b.WriteString("  );\n")
	b.WriteString("}\n")

	return &domain.ExportResult{
		Format:      domain.ExportFormatJSX,
		Filename:    "PricingSection.tsx",
		ContentType: "text/plain",
		Content:     b.String(),
	}
}

func writeJSXTier(b *strings.Builder, t domain.PricingTier) {
	class := "tier"
	if t.Highlighted {
		class += " tier--highlighted"
	}
	fmt.Fprintf(b, "        <div className=%s>\n", jsxString(class))
	if t.Badge != "" {
		fmt.Fprintf(b, "          <span className=\"badge\">{%s}</span>\n", jsxString(t.Badge))
	}
	fmt.Fprintf(b, "          <h3>{%s}</h3>\n", jsxString(t.Name))
	fmt.Fprintf(b, "          <p className=\"price\">{%s}</p>\n", jsxString(formatPrice(t.Price)))
	if t.Tagline != "" {
		fmt.Fprintf(b, "          <p className=\"tagline\">{%s}</p>\n", jsxString(t.Tagline))
	}
	b.WriteString("          <ul className=\"features\">\n")
	for _, f := range t.Features {
		fmt.Fprintf(b, "            <li>{%s}</li>\n", jsxString(f))
	}
	b.WriteString("          </ul>\n")
	if t.CTA != "" {
		fmt.Fprintf(b, "          <button type=\"button\">{%s}</button>\n", jsxString(t.CTA))
	}
	b.WriteString("        </div>\n")
}

// jsxString JSON-encodes s into a double-quoted JS string literal safe to
// embed inside a JSX expression ({ }) or as a plain string attribute value:
// see renderJSX's doc comment for why.
func jsxString(s string) string {
	encoded, err := json.Marshal(s)
	if err != nil {
		// json.Marshal only fails on a string containing invalid UTF-8,
		// which domain values never do; fall back to an empty literal
		// rather than letting a response renderer error out.
		return `""`
	}
	return string(encoded)
}
