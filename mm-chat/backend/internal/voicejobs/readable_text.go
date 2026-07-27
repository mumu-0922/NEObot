package voicejobs

import (
	"bytes"
	"html"
	"regexp"
	"strings"
	"unicode"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	goldmarkhtml "github.com/yuin/goldmark/renderer/html"
	xhtml "golang.org/x/net/html"
)

var htmlBlockElements = map[string]struct{}{
	"address": {}, "article": {}, "aside": {}, "blockquote": {}, "br": {},
	"caption": {}, "dd": {}, "details": {}, "div": {}, "dl": {}, "dt": {},
	"figcaption": {}, "figure": {}, "footer": {}, "form": {}, "h1": {},
	"h2": {}, "h3": {}, "h4": {}, "h5": {}, "h6": {}, "header": {},
	"hr": {}, "li": {}, "main": {}, "nav": {}, "ol": {}, "p": {},
	"pre": {}, "section": {}, "summary": {}, "table": {}, "tbody": {},
	"tfoot": {}, "thead": {}, "tr": {}, "ul": {},
}

var nonVisibleHTMLElements = map[string]struct{}{
	"embed": {}, "iframe": {}, "input": {}, "noscript": {}, "object": {},
	"script": {}, "style": {}, "template": {}, "textarea": {},
}

var (
	residualMarkdownSpan  = regexp.MustCompile(`\*\*([^*\n]+)\*\*|__([^_\n]+)__|~~([^~\n]+)~~`)
	visibleCitationMarker = regexp.MustCompile(`\[([Ww][0-9]+)\]`)
)

// projectReadableText turns persisted Markdown/HTML into the text a rendered
// answer exposes to a reader. It intentionally keeps visible code and link
// labels while removing formatting syntax, HTML metadata, and link targets.
func projectReadableText(value string) string {
	source := []byte(strings.TrimSpace(value))
	if len(source) == 0 {
		return ""
	}

	markdown := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithRendererOptions(goldmarkhtml.WithUnsafe()),
	)
	var rendered bytes.Buffer
	if err := markdown.Convert(source, &rendered); err != nil {
		return ""
	}
	projected := readableHTMLText(rendered.String())
	projected = visibleCitationMarker.ReplaceAllString(projected, "$1")
	return normalizeReadableText(projected)
}

func readableHTMLText(fragment string) string {
	if strings.TrimSpace(fragment) == "" {
		return ""
	}
	nodes, err := xhtml.ParseFragment(strings.NewReader(fragment), nil)
	if err != nil {
		return ""
	}
	var output strings.Builder
	var visit func(*xhtml.Node)
	visit = func(node *xhtml.Node) {
		if node.Type == xhtml.ElementNode {
			name := strings.ToLower(node.Data)
			if _, excluded := nonVisibleHTMLElements[name]; excluded || htmlNodeHidden(node) {
				return
			}
		}
		if node.Type == xhtml.TextNode {
			appendHTMLText(&output, node)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
		if node.Type == xhtml.ElementNode {
			name := strings.ToLower(node.Data)
			if name == "td" || name == "th" {
				output.WriteByte('\t')
			} else if _, block := htmlBlockElements[name]; block {
				output.WriteByte('\n')
			}
		}
	}
	for _, node := range nodes {
		visit(node)
	}
	return normalizeReadableText(output.String())
}

func appendHTMLText(output *strings.Builder, node *xhtml.Node) {
	data := node.Data
	if !htmlNodeHasAncestor(node, "code") {
		data = residualMarkdownSpan.ReplaceAllString(data, "$1$2$3")
	}
	if htmlNodeHasAncestor(node, "pre") {
		output.WriteString(data)
		return
	}
	trimmed := strings.TrimSpace(data)
	if trimmed == "" {
		output.WriteByte(' ')
		return
	}
	if len(strings.TrimLeftFunc(data, unicode.IsSpace)) != len(data) {
		output.WriteByte(' ')
	}
	output.WriteString(strings.Join(strings.Fields(trimmed), " "))
	if len(strings.TrimRightFunc(data, unicode.IsSpace)) != len(data) {
		output.WriteByte(' ')
	}
}

func htmlNodeHasAncestor(node *xhtml.Node, name string) bool {
	for parent := node.Parent; parent != nil; parent = parent.Parent {
		if parent.Type == xhtml.ElementNode && strings.EqualFold(parent.Data, name) {
			return true
		}
	}
	return false
}

func htmlNodeHidden(node *xhtml.Node) bool {
	for _, attribute := range node.Attr {
		if strings.EqualFold(attribute.Key, "style") {
			for _, declaration := range strings.Split(attribute.Val, ";") {
				property, value, ok := strings.Cut(declaration, ":")
				if ok && strings.EqualFold(strings.TrimSpace(property), "display") &&
					strings.EqualFold(strings.TrimSpace(value), "none") {
					return true
				}
			}
		}
	}
	return false
}

func normalizeReadableText(value string) string {
	value = html.UnescapeString(value)
	value = strings.ReplaceAll(value, "\u00a0", " ")
	value = strings.ReplaceAll(value, "\u200b", "")
	lines := strings.Split(strings.ReplaceAll(value, "\r", "\n"), "\n")
	normalized := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			continue
		}
		normalized = append(normalized, line)
	}
	return strings.Join(normalized, "\n")
}
