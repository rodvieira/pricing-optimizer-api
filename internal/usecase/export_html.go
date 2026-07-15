package usecase

import (
	"bytes"
	"html/template"
	"log/slog"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
)

// htmlTemplate renders a standalone, presentational pricing page (no
// external stylesheet/script) safe to open directly in a browser.
// html/template auto-escapes every field, so nothing an LLM produced can
// inject markup into the document.
var htmlTemplate = template.Must(template.New("pricing").Funcs(template.FuncMap{
	"formatPrice": formatPrice,
}).Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>{{.Headline}}</title>
</head>
<body>
  <section class="pricing-section">
    <h2>{{.Headline}}</h2>
    {{- if .Subheadline}}
    <p class="subheadline">{{.Subheadline}}</p>
    {{- end}}
    <div class="tiers">
      {{- range .Tiers}}
      <div class="tier{{if .Highlighted}} tier--highlighted{{end}}">
        {{- if .Badge}}
        <span class="badge">{{.Badge}}</span>
        {{- end}}
        <h3>{{.Name}}</h3>
        <p class="price">{{formatPrice .Price}}</p>
        {{- if .Tagline}}
        <p class="tagline">{{.Tagline}}</p>
        {{- end}}
        <ul class="features">
          {{- range .Features}}
          <li>{{.}}</li>
          {{- end}}
        </ul>
        {{- if .CTA}}
        <button type="button">{{.CTA}}</button>
        {{- end}}
      </div>
      {{- end}}
    </div>
  </section>
</body>
</html>
`))

// renderHTML renders v via htmlTemplate.
func renderHTML(v domain.Variation) *domain.ExportResult {
	var buf bytes.Buffer
	if err := htmlTemplate.Execute(&buf, v); err != nil {
		// htmlTemplate is a fixed, well-formed constant executed against a
		// domain.Variation whose referenced fields are all plain
		// strings/slices: this cannot fail in practice. Log rather than
		// propagate, the same degrade-not-crash choice generate.go's
		// parseUUID makes for an equivalent can't-really-happen case.
		slog.Error("export variation: render html", "error", err)
	}

	return &domain.ExportResult{
		Format:      domain.ExportFormatHTML,
		Filename:    "pricing.html",
		ContentType: "text/html",
		Content:     buf.String(),
	}
}
