package llm

import (
	"fmt"
	"strings"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
)

var strategyGuidance = map[domain.PricingStrategy]string{
	domain.StrategyAnchor: "Anchor pricing: include a high-priced decoy tier so the target " +
		"tier looks reasonably priced by comparison.",
	domain.StrategyFreemium: "Freemium ladder: a genuinely useful free tier that creates a " +
		"natural upgrade path toward paid tiers.",
	domain.StrategyValueBased: "Value-based pricing: price tiers according to the value " +
		"delivered to the audience segment, not production cost.",
}

// systemPrompt instructs the model to always answer via the tool call and
// describes the requested strategy.
func systemPrompt(in domain.GenerationInput) string {
	var b strings.Builder
	b.WriteString("You are a pricing strategist generating one pricing-page variation. ")
	fmt.Fprintf(&b, "Call the %s tool exactly once with your complete answer. ", toolName)
	b.WriteString("Never respond in plain text.\n\n")
	fmt.Fprintf(&b, "Strategy to apply: %s. %s\n", in.Strategy, strategyGuidance[in.Strategy])
	fmt.Fprintf(&b, "Currency: %s.\n", in.Currency)
	return b.String()
}

// userPrompt describes the analyzed site the variation is generated for.
func userPrompt(in domain.GenerationInput) string {
	p := in.SiteProfile
	var b strings.Builder
	fmt.Fprintf(&b, "Product: %s\n", p.Title)
	fmt.Fprintf(&b, "URL: %s\n", p.URL)
	fmt.Fprintf(&b, "Value proposition: %s\n", p.ValueProposition)
	fmt.Fprintf(&b, "Industry: %s\n", p.Industry)
	fmt.Fprintf(&b, "Audience: %s (sophistication: %s, price position: %s)\n",
		p.Audience.Segment, p.Audience.Sophistication, p.Audience.PricePosition)
	if len(p.Keywords) > 0 {
		fmt.Fprintf(&b, "Keywords: %s\n", strings.Join(p.Keywords, ", "))
	}
	return b.String()
}
