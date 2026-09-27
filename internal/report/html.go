// Package report renders a compare.Result as a static, self-contained
// HTML document. It has no dependency on a browser-side framework and
// requests no external resource — everything (styles, content) is
// inlined into a single file, per spec sections 27/28:
//
//   - "no external SaaS dependency; no remote JavaScript required"
//   - "escape user-controlled strings" — every dynamic value goes
//     through html/template, which auto-escapes by default; nothing here
//     ever wraps a Finding field in template.HTML.
//   - "no secrets" — Result (see internal/compare) has no field that
//     could ever hold a credential, so there is nothing to redact.
//   - PASS/WARN/BLOCK is always rendered as text, never conveyed by
//     color alone.
package report

import (
	"fmt"
	"html/template"
	"io"
	"time"

	"github.com/sailwalpranjal/appwrite-migration-guard/internal/compare"
	"github.com/sailwalpranjal/appwrite-migration-guard/internal/version"
)

type templateData struct {
	Title       string
	Result      *compare.Result
	Overall     compare.Severity
	GeneratedAt string
	ToolVersion string
	PassCount   int
	WarnCount   int
	BlockCount  int
}

// WriteHTML renders res as a complete HTML document to w.
func WriteHTML(w io.Writer, res *compare.Result) error {
	data := templateData{
		Title:       fmt.Sprintf("amg report: %s vs %s", res.SourceLabel, res.DestLabel),
		Result:      res,
		Overall:     res.Overall(),
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		ToolVersion: version.Version,
	}
	for _, f := range res.Findings {
		switch f.Severity {
		case compare.SeverityPass:
			data.PassCount++
		case compare.SeverityWarn:
			data.WarnCount++
		case compare.SeverityBlock:
			data.BlockCount++
		}
	}
	return htmlTemplate.Execute(w, data)
}

var htmlTemplate = template.Must(template.New("report").Parse(htmlTemplateSource))

// htmlTemplateSource is a single self-contained document: no external
// stylesheet, script, font, or image reference of any kind, so the
// rendered file works when opened directly from disk with no network
// access. Headings are semantic (h1/h2), the findings list uses a real
// <table> with scoped <th> headers for screen readers, and color is
// always paired with the literal PASS/WARN/BLOCK text.
const htmlTemplateSource = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<style>
  :root {
    color-scheme: light dark;
    --pass: #1a7f37; --warn: #9a6700; --block: #cf222e;
    --bg: #ffffff; --fg: #1f2328; --border: #d0d7de; --muted: #656d76;
  }
  @media (prefers-color-scheme: dark) {
    :root { --bg: #0d1117; --fg: #e6edf3; --border: #30363d; --muted: #8b949e; }
  }
  * { box-sizing: border-box; }
  body {
    margin: 0; padding: 24px 16px 48px;
    background: var(--bg); color: var(--fg);
    font: 15px/1.5 -apple-system, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
  }
  main { max-width: 880px; margin: 0 auto; }
  h1 { font-size: 1.5rem; margin: 0 0 4px; }
  h2 { font-size: 1.1rem; margin: 32px 0 12px; }
  .meta { color: var(--muted); font-size: 0.9rem; margin: 0 0 24px; }
  .meta dt { display: inline; font-weight: 600; }
  .meta dd { display: inline; margin: 0 16px 0 4px; }
  .badge {
    display: inline-block; padding: 4px 12px; border-radius: 6px;
    font-weight: 700; font-size: 0.95rem; letter-spacing: 0.02em;
    border: 1px solid currentColor;
  }
  .badge-PASS { color: var(--pass); }
  .badge-WARN { color: var(--warn); }
  .badge-BLOCK { color: var(--block); }
  .counts { color: var(--muted); font-size: 0.9rem; margin-top: 8px; }
  table { width: 100%; border-collapse: collapse; margin-top: 8px; }
  caption { text-align: left; color: var(--muted); font-size: 0.85rem; padding-bottom: 8px; }
  th, td {
    text-align: left; padding: 10px 12px; border-bottom: 1px solid var(--border);
    vertical-align: top; font-size: 0.92rem;
  }
  th { font-size: 0.78rem; text-transform: uppercase; letter-spacing: 0.04em; color: var(--muted); }
  td.severity { white-space: nowrap; font-weight: 700; }
  td.severity.PASS { color: var(--pass); }
  td.severity.WARN { color: var(--warn); }
  td.severity.BLOCK { color: var(--block); }
  code { font-family: ui-monospace, "SFMono-Regular", Consolas, monospace; font-size: 0.9em; }
  .empty { color: var(--muted); padding: 16px 0; }
  footer { margin-top: 40px; color: var(--muted); font-size: 0.8rem; }
</style>
</head>
<body>
<main>
  <h1>Appwrite Migration Guard — report</h1>
  <dl class="meta">
    <dt>Source</dt><dd>{{.Result.SourceLabel}}</dd>
    <dt>Destination</dt><dd>{{.Result.DestLabel}}</dd>
    <dt>Generated</dt><dd>{{.GeneratedAt}}</dd>
    <dt>Policy</dt><dd>{{.Result.PolicyName}}</dd>
  </dl>

  <span class="badge badge-{{.Overall}}">Result: {{.Overall}}</span>
  <p class="counts">{{.PassCount}} pass &middot; {{.WarnCount}} warn &middot; {{.BlockCount}} block</p>

  <section class="coverage">
    <h2>Verification coverage</h2>
    <p>A <strong>PASS</strong> means no difference was found <em>in what amg
    checks</em> — not a proof this migration is complete, consistent, or
    safe to cut over to. This applies to every run, regardless of result.</p>
    <ul>
      <li><strong>Always checked:</strong> resource existence, permissions, config, table schema (columns/indexes), file content (via MD5 signature)</li>
      <li><strong>Checked if enabled:</strong> row content (<code>--sample-rows</code>, off by default)</li>
      <li><strong>Never checked:</strong> relationships between resources, function/site code or deployed runtime behavior (config only), application-level invariants</li>
    </ul>
  </section>

  <h2>Findings</h2>
  {{if .Result.Findings}}
  <table>
    <caption>Every finding amg produced comparing source and destination. Rows are ordered by resource type, parent, ID, then rule for reproducibility — not by severity.</caption>
    <thead>
      <tr>
        <th scope="col">Status</th>
        <th scope="col">Rule</th>
        <th scope="col">Resource</th>
        <th scope="col">Detail</th>
      </tr>
    </thead>
    <tbody>
      {{range .Result.Findings}}
      <tr>
        <td class="severity {{.Severity}}">{{.Severity}}</td>
        <td><code>{{.Rule}}</code></td>
        <td>{{.ResourceType}} <code>{{.ResourceID}}</code>{{if .ParentID}}<br><span style="color:var(--muted);font-size:0.85em">in {{.ParentID}}</span>{{end}}</td>
        <td>{{.Message}}</td>
      </tr>
      {{end}}
    </tbody>
  </table>
  {{else}}
  <p class="empty">No differences found.</p>
  {{end}}

  <footer>Generated by amg {{.ToolVersion}}. This report never contains API keys or other credentials — amg's manifest and comparison formats have no field that could hold one.</footer>
</main>
</body>
</html>
`
