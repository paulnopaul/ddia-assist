package epub

import (
	"path"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// block renders a block-level node as Markdown (ING-4). sectionID is the
// owner of any figure found inside it.
func (p *parser) block(b *strings.Builder, n *html.Node, sectionID string) {
	switch n.Type {
	case html.TextNode:
		if t := strings.TrimSpace(n.Data); t != "" {
			b.WriteString(collapse(n.Data))
		}
		return
	case html.ElementNode:
	default:
		return
	}
	dt := attr(n, "data-type")
	if dt == "indexterm" {
		return
	}
	switch n.Data {
	case "p":
		b.WriteString("\n\n" + strings.TrimSpace(inline(n)) + "\n\n")
	case "h1", "h2", "h3", "h4", "h5", "h6":
		b.WriteString("\n\n**" + cleanText(n) + "**\n\n")
	case "pre":
		b.WriteString("\n\n```\n" + strings.TrimRight(textOf(n), "\n") + "\n```\n\n")
	case "ul", "ol":
		i := 0
		for li := n.FirstChild; li != nil; li = li.NextSibling {
			if li.Type != html.ElementNode || li.Data != "li" {
				continue
			}
			i++
			marker := "- "
			if n.Data == "ol" {
				marker = itoa(i) + ". "
			}
			var inner strings.Builder
			for c := li.FirstChild; c != nil; c = c.NextSibling {
				p.block(&inner, c, sectionID)
			}
			b.WriteString("\n" + marker + strings.ReplaceAll(tidy(inner.String()), "\n", "\n  "))
		}
		b.WriteString("\n\n")
	case "dl":
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type != html.ElementNode {
				continue
			}
			switch c.Data {
			case "dt":
				b.WriteString("\n\n**" + strings.TrimSpace(inline(c)) + "**")
			case "dd":
				var inner strings.Builder
				for cc := c.FirstChild; cc != nil; cc = cc.NextSibling {
					p.block(&inner, cc, sectionID)
				}
				b.WriteString("\n: " + strings.ReplaceAll(tidy(inner.String()), "\n", "\n  "))
			}
		}
		b.WriteString("\n\n")
	case "blockquote":
		var inner strings.Builder
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			p.block(&inner, c, sectionID)
		}
		b.WriteString("\n\n" + quote(tidy(inner.String())) + "\n\n")
	case "figure":
		p.figure(b, n, sectionID)
	case "img":
		p.figure(b, n, sectionID)
	case "table":
		b.WriteString("\n\n" + table(n) + "\n\n")
	case "br":
		b.WriteString("\n")
	case "div", "aside", "section", "span", "header", "figcaption":
		if dt == "note" || dt == "tip" || dt == "warning" || dt == "caution" || dt == "important" || dt == "sidebar" || n.Data == "aside" {
			var inner strings.Builder
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				p.block(&inner, c, sectionID)
			}
			b.WriteString("\n\n" + quote(tidy(inner.String())) + "\n\n")
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			p.block(b, c, sectionID)
		}
	case "script", "style", "head", "nav":
	default:
		if t := strings.TrimSpace(inline(n)); t != "" {
			b.WriteString(t + " ")
		}
	}
}

// figure records the image and writes a Markdown image reference (ING-5).
func (p *parser) figure(b *strings.Builder, n *html.Node, sectionID string) {
	img := n
	if n.Data != "img" {
		img = find(n, func(x *html.Node) bool { return x.Data == "img" })
	}
	caption := ""
	if n.Data != "img" {
		if h := find(n, func(x *html.Node) bool { return isHeading(x) || x.Data == "figcaption" }); h != nil {
			caption = cleanText(h)
		}
	}
	if caption == "" && img != nil {
		caption = attr(img, "alt")
	}
	if img == nil {
		if caption != "" {
			b.WriteString("\n\n*" + caption + "*\n\n")
		}
		return
	}
	src := path.Join(p.docDir, attr(img, "src"))
	name := path.Base(src)
	if data, err := p.read(src); err == nil {
		p.book.Figures = append(p.book.Figures, Figure{Name: name, SectionID: sectionID, Caption: caption, Data: data})
	}
	b.WriteString("\n\n![" + strings.ReplaceAll(caption, "]", ")") + "](figure:" + name + ")\n\n")
}

// inline renders phrasing content: emphasis, code, links as plain text,
// note references as [n]; index-term anchors are dropped.
func inline(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			switch c.Type {
			case html.TextNode:
				b.WriteString(collapse(c.Data))
			case html.ElementNode:
				if attr(c, "data-type") == "indexterm" {
					continue
				}
				switch c.Data {
				case "em", "i":
					if t := strings.TrimSpace(textOf(c)); t != "" {
						b.WriteString("*" + collapse(t) + "*")
					}
				case "strong", "b":
					if t := strings.TrimSpace(textOf(c)); t != "" {
						b.WriteString("**" + collapse(t) + "**")
					}
				case "code", "tt":
					if t := textOf(c); t != "" {
						b.WriteString("`" + strings.ReplaceAll(collapse(t), "`", "'") + "`")
					}
				case "sup":
					b.WriteString("^" + textOf(c))
				case "br":
					b.WriteString(" ")
				case "img":
				default:
					walk(c)
				}
			}
		}
	}
	walk(n)
	return b.String()
}

func table(n *html.Node) string {
	var rows [][]string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type != html.ElementNode {
				continue
			}
			if c.Data == "tr" {
				var row []string
				for cell := c.FirstChild; cell != nil; cell = cell.NextSibling {
					if cell.Type == html.ElementNode && (cell.Data == "td" || cell.Data == "th") {
						row = append(row, strings.ReplaceAll(strings.TrimSpace(inline(cell)), "|", "\\|"))
					}
				}
				rows = append(rows, row)
				continue
			}
			walk(c)
		}
	}
	walk(n)
	var b strings.Builder
	if cap := find(n, func(x *html.Node) bool { return x.Data == "caption" }); cap != nil {
		b.WriteString("*" + cleanText(cap) + "*\n\n")
	}
	for i, r := range rows {
		b.WriteString("| " + strings.Join(r, " | ") + " |\n")
		if i == 0 {
			b.WriteString("|" + strings.Repeat(" --- |", len(r)) + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func textOf(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && attr(c, "data-type") == "indexterm" {
			continue
		}
		b.WriteString(textOf(c))
	}
	return b.String()
}

func cleanText(n *html.Node) string { return strings.TrimSpace(collapse(textOf(n))) }

var spaces = regexp.MustCompile(`\s+`)

func collapse(s string) string { return spaces.ReplaceAllString(s, " ") }

var blankLines = regexp.MustCompile(`\n[ \t]*\n(\s*\n)+`)
var trailing = regexp.MustCompile(`[ \t]+\n`)

func tidy(s string) string {
	s = trailing.ReplaceAllString(s, "\n")
	s = blankLines.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

func quote(s string) string {
	if s == "" {
		return ""
	}
	return "> " + strings.ReplaceAll(s, "\n", "\n> ")
}

func itoa(i int) string {
	const digits = "0123456789"
	if i < 10 {
		return digits[i : i+1]
	}
	return itoa(i/10) + digits[i%10:i%10+1]
}
