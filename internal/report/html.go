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
	"sort"
	"strings"
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
	ByResource  []resourceTypeCount
}

// resourceTypeCount is one row of the per-resource-type findings
// breakdown. Findings never carry SeverityPass (compare only appends a
// Finding when something needs reporting — PASS is the "no findings"
// case), so there is deliberately no Pass field here.
type resourceTypeCount struct {
	Type  string
	Warn  int
	Block int
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
	byResource := map[string]*resourceTypeCount{}
	for _, f := range res.Findings {
		switch f.Severity {
		case compare.SeverityPass:
			data.PassCount++
		case compare.SeverityWarn:
			data.WarnCount++
		case compare.SeverityBlock:
			data.BlockCount++
		}
		rt := string(f.ResourceType)
		c, ok := byResource[rt]
		if !ok {
			c = &resourceTypeCount{Type: rt}
			byResource[rt] = c
		}
		switch f.Severity {
		case compare.SeverityWarn:
			c.Warn++
		case compare.SeverityBlock:
			c.Block++
		}
	}
	for _, c := range byResource {
		data.ByResource = append(data.ByResource, *c)
	}
	sort.Slice(data.ByResource, func(i, j int) bool { return data.ByResource[i].Type < data.ByResource[j].Type })

	return htmlTemplate.Execute(w, data)
}

// findingSearchBlob returns a lowercase, whitespace-joined concatenation
// of a Finding's searchable fields, used as the client-side filter
// toolbar's data-search attribute so the vanilla-JS search box can match
// against rule, resource, and message text without a search library.
func findingSearchBlob(f compare.Finding) string {
	return strings.ToLower(strings.Join([]string{
		string(f.Severity), f.Rule, string(f.ResourceType), f.ResourceID, f.ParentID, f.Message,
	}, " "))
}

var htmlTemplate = template.Must(
	template.New("report").
		Funcs(template.FuncMap{"searchBlob": findingSearchBlob}).
		Parse(htmlTemplateSource),
)

// htmlTemplateSource is a single self-contained document: no external
// stylesheet, script, font, or image reference of any kind, so the
// rendered file works when opened directly from disk with no network
// access. Headings are semantic (h1/h2), the findings list uses a real
// <table> with scoped <th> headers for screen readers, and color is
// always paired with the literal PASS/WARN/BLOCK text.
//
// The one inline <script> block is vanilla JS with no dependency and no
// network access: it only (a) applies a persisted light/dark override
// from localStorage before first paint, wrapped in try/catch since
// localStorage can throw when the file is opened directly from disk
// under some browser privacy settings, and (b) filters/searches the
// findings table client-side. Both degrade harmlessly with JS disabled
// — the table is fully readable unfiltered, and the theme falls back to
// prefers-color-scheme.
const htmlTemplateSource = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<script>
  try {
    var t = localStorage.getItem('amg-report-theme');
    if (t === 'light' || t === 'dark') document.documentElement.setAttribute('data-theme', t);
  } catch (e) {}
</script>
<style>
  :root {
    color-scheme: light dark;
    --pass: #1a7f37; --pass-bg: #dafbe1;
    --warn: #9a6700; --warn-bg: #fff8c5;
    --block: #cf222e; --block-bg: #ffebe9;
    --bg: #ffffff; --bg-alt: #f6f8fa; --fg: #1f2328; --border: #d0d7de; --muted: #656d76;
  }
  @media (prefers-color-scheme: dark) {
    :root:not([data-theme="light"]) {
      --pass: #3fb950; --pass-bg: #122117;
      --warn: #d29922; --warn-bg: #2b2111;
      --block: #f85149; --block-bg: #2d1214;
      --bg: #0d1117; --bg-alt: #161b22; --fg: #e6edf3; --border: #30363d; --muted: #8b949e;
    }
  }
  :root[data-theme="dark"] {
    color-scheme: dark;
    --pass: #3fb950; --pass-bg: #122117;
    --warn: #d29922; --warn-bg: #2b2111;
    --block: #f85149; --block-bg: #2d1214;
    --bg: #0d1117; --bg-alt: #161b22; --fg: #e6edf3; --border: #30363d; --muted: #8b949e;
  }
  * { box-sizing: border-box; }
  body {
    margin: 0; padding: 24px 16px 48px;
    background: var(--bg); color: var(--fg);
    font: 15px/1.5 -apple-system, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
  }
  main { max-width: 960px; margin: 0 auto; }
  h1 { font-size: 1.5rem; margin: 0 0 4px; display: inline-block; }
  h2 { font-size: 1.1rem; margin: 32px 0 12px; padding-bottom: 6px; border-bottom: 1px solid var(--border); }
  .top-row { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
  .theme-toggle {
    font: inherit; font-size: 0.8rem; padding: 4px 10px; border-radius: 6px;
    border: 1px solid var(--border); background: var(--bg-alt); color: var(--fg); cursor: pointer;
  }
  .theme-toggle:hover { border-color: var(--muted); }
  .meta { color: var(--muted); font-size: 0.9rem; margin: 8px 0 24px; }
  .meta dt { display: inline; font-weight: 600; }
  .meta dd { display: inline; margin: 0 16px 0 4px; }
  .badge {
    display: inline-block; padding: 4px 12px; border-radius: 6px;
    font-weight: 700; font-size: 0.95rem; letter-spacing: 0.02em;
    border: 1px solid currentColor;
  }
  .badge-PASS { color: var(--pass); background: var(--pass-bg); }
  .badge-WARN { color: var(--warn); background: var(--warn-bg); }
  .badge-BLOCK { color: var(--block); background: var(--block-bg); }
  .counts { color: var(--muted); font-size: 0.9rem; margin-top: 8px; }
  .breakdown { width: 100%; border-collapse: collapse; margin-top: 8px; font-size: 0.88rem; }
  .breakdown th, .breakdown td { text-align: left; padding: 6px 10px; border-bottom: 1px solid var(--border); }
  .breakdown th { color: var(--muted); font-weight: 600; font-size: 0.78rem; text-transform: uppercase; letter-spacing: 0.04em; }
  .toolbar {
    position: sticky; top: 0; z-index: 1; display: flex; gap: 8px; flex-wrap: wrap; align-items: center;
    background: var(--bg); padding: 10px 0; border-bottom: 1px solid var(--border); margin-bottom: 4px;
  }
  .toolbar button {
    font: inherit; font-size: 0.82rem; padding: 5px 12px; border-radius: 999px;
    border: 1px solid var(--border); background: var(--bg-alt); color: var(--fg); cursor: pointer;
  }
  .toolbar button[aria-pressed="true"] { border-color: currentColor; font-weight: 700; }
  .toolbar button[data-severity="PASS"][aria-pressed="true"] { color: var(--pass); }
  .toolbar button[data-severity="WARN"][aria-pressed="true"] { color: var(--warn); }
  .toolbar button[data-severity="BLOCK"][aria-pressed="true"] { color: var(--block); }
  .toolbar input[type="search"] {
    font: inherit; font-size: 0.85rem; padding: 5px 10px; border-radius: 6px;
    border: 1px solid var(--border); background: var(--bg-alt); color: var(--fg);
    flex: 1 1 200px; min-width: 140px;
  }
  .result-count { color: var(--muted); font-size: 0.82rem; margin: 4px 0 12px; }
  .table-wrap { overflow-x: auto; }
  table.findings { width: 100%; border-collapse: collapse; margin-top: 8px; min-width: 560px; }
  caption { text-align: left; color: var(--muted); font-size: 0.85rem; padding-bottom: 8px; }
  th, td {
    text-align: left; padding: 10px 12px; border-bottom: 1px solid var(--border);
    vertical-align: top; font-size: 0.92rem;
  }
  th { font-size: 0.78rem; text-transform: uppercase; letter-spacing: 0.04em; color: var(--muted); }
  tbody tr:nth-child(even) { background: var(--bg-alt); }
  tbody tr:hover { background: var(--warn-bg); }
  td.severity { white-space: nowrap; font-weight: 700; }
  td.severity.PASS { color: var(--pass); }
  td.severity.WARN { color: var(--warn); }
  td.severity.BLOCK { color: var(--block); }
  code { font-family: ui-monospace, "SFMono-Regular", Consolas, monospace; font-size: 0.9em; }
  .empty { color: var(--muted); padding: 16px 0; }
  footer { margin-top: 40px; color: var(--muted); font-size: 0.8rem; }
  @media (max-width: 640px) {
    .toolbar { position: static; }
  }
</style>
</head>
<body>
<main>
  <div class="top-row">
    <h1>Appwrite Migration Guard — report</h1>
    <button type="button" class="theme-toggle" id="theme-toggle" aria-label="Toggle light/dark theme">Toggle theme</button>
  </div>
  <dl class="meta">
    <dt>Source</dt><dd>{{.Result.SourceLabel}}</dd>
    <dt>Destination</dt><dd>{{.Result.DestLabel}}</dd>
    <dt>Generated</dt><dd>{{.GeneratedAt}}</dd>
    <dt>Policy</dt><dd>{{.Result.PolicyName}}</dd>
  </dl>

  <span class="badge badge-{{.Overall}}">Result: {{.Overall}}</span>
  <p class="counts">{{.PassCount}} pass &middot; {{.WarnCount}} warn &middot; {{.BlockCount}} block</p>

  {{if .Result.CoverageMismatch}}
  <p class="badge badge-WARN" style="display:block;margin-top:12px">
    Source and destination requested different resource categories
    (<code>--resources</code>): source={{.Result.SourceCollected}}
    dest={{.Result.DestCollected}}. A category present on only one side
    won't produce a reliable missing/unexpected finding for that category.
  </p>
  {{end}}

  {{if .Result.AmbiguousLabels}}
  <p class="badge badge-WARN" style="display:block;margin-top:12px">
    Source and destination manifests both carry the label
    &quot;{{.Result.SourceLabel}}&quot;. Findings below can't distinguish
    the two sides by name &mdash; re-run <code>amg snapshot --label
    source</code> / <code>--label destination</code> for clearer output.
  </p>
  {{end}}

  {{if .ByResource}}
  <table class="breakdown">
    <caption>Findings by resource type (PASS is never listed here — an unaffected resource type produces no rows, not a "0" row).</caption>
    <thead><tr><th scope="col">Resource type</th><th scope="col">Warn</th><th scope="col">Block</th></tr></thead>
    <tbody>
      {{range .ByResource}}
      <tr><td>{{.Type}}</td><td>{{.Warn}}</td><td>{{.Block}}</td></tr>
      {{end}}
    </tbody>
  </table>
  {{end}}

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
  <div class="toolbar" id="toolbar" role="group" aria-label="Filter findings">
    <button type="button" data-severity="PASS" aria-pressed="true">PASS</button>
    <button type="button" data-severity="WARN" aria-pressed="true">WARN</button>
    <button type="button" data-severity="BLOCK" aria-pressed="true">BLOCK</button>
    <input type="search" id="search" placeholder="Filter by rule, resource, or message&#8230;" aria-label="Filter findings by text">
  </div>
  <p class="result-count" id="result-count" aria-live="polite"></p>
  <div class="table-wrap">
  <table class="findings" id="findings-table">
    <caption>Every finding amg produced comparing source and destination. Rows are ordered by resource type, parent, ID, then rule for reproducibility — not by severity. Filtering happens in your browser only; it never re-runs the comparison or contacts any server.</caption>
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
      <tr data-severity="{{.Severity}}" data-search="{{searchBlob .}}">
        <td class="severity {{.Severity}}">{{.Severity}}</td>
        <td><code>{{.Rule}}</code></td>
        <td>{{.ResourceType}} <code>{{.ResourceID}}</code>{{if .ParentID}}<br><span style="color:var(--muted);font-size:0.85em">in {{.ParentID}}</span>{{end}}</td>
        <td>{{.Message}}</td>
      </tr>
      {{end}}
    </tbody>
  </table>
  </div>
  {{else}}
  <p class="empty">No differences found.</p>
  {{end}}

  <footer>Generated by amg {{.ToolVersion}}. This report never contains API keys or other credentials — amg's manifest and comparison formats have no field that could hold one.</footer>
</main>
<script>
(function () {
  var toggle = document.getElementById('theme-toggle');
  if (toggle) {
    toggle.addEventListener('click', function () {
      var current = document.documentElement.getAttribute('data-theme');
      var next = current === 'dark' ? 'light' : (current === 'light' ? '' : (matchMedia('(prefers-color-scheme: dark)').matches ? 'light' : 'dark'));
      if (next === '') {
        document.documentElement.removeAttribute('data-theme');
      } else {
        document.documentElement.setAttribute('data-theme', next);
      }
      try { localStorage.setItem('amg-report-theme', next || ''); } catch (e) {}
    });
  }

  var toolbar = document.getElementById('toolbar');
  if (!toolbar) return;
  var rows = Array.prototype.slice.call(document.querySelectorAll('#findings-table tbody tr'));
  var search = document.getElementById('search');
  var countEl = document.getElementById('result-count');
  var active = { PASS: true, WARN: true, BLOCK: true };

  function apply() {
    var q = (search.value || '').toLowerCase().trim();
    var shown = 0;
    rows.forEach(function (row) {
      var sev = row.getAttribute('data-severity');
      var text = row.getAttribute('data-search') || '';
      var match = active[sev] && (q === '' || text.indexOf(q) !== -1);
      row.hidden = !match;
      if (match) shown++;
    });
    countEl.textContent = shown + ' of ' + rows.length + ' finding(s) shown';
  }

  toolbar.querySelectorAll('button[data-severity]').forEach(function (btn) {
    btn.addEventListener('click', function () {
      var sev = btn.getAttribute('data-severity');
      active[sev] = !active[sev];
      btn.setAttribute('aria-pressed', String(active[sev]));
      apply();
    });
  });
  search.addEventListener('input', apply);
  apply();
})();
</script>
</body>
</html>
`
