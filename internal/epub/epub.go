// Package epub turns an EPUB into a tree of sections with Markdown bodies
// (docs/spec/01-ingest.md, ING-1 to ING-7).
//
// O'Reilly books use HTMLBook markup: nested <section data-type="sect1|sect2|…">
// elements, each opening with a heading. When a chapter has no nested
// sections, the parser falls back to splitting on h2–h4 headings.
package epub

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"strings"

	"golang.org/x/net/html"
)

// Book is the parsed result. Sections are in document order.
type Book struct {
	Title    string
	Sections []Section
	Figures  []Figure
}

// Section is one node of the heading tree. Level 0 is a chapter.
type Section struct {
	ID          string // ch05, ch05.s02, ch05.s02.s01, ch05.refs
	ParentID    string
	Chapter     int // 1-based chapter number; 0 for front matter
	Level       int
	Heading     string
	Markdown    string
	Words       int
	IsReference bool
}

// Figure is an image referenced from a section's Markdown as figure:<Name>.
type Figure struct {
	Name      string // stable file name, e.g. ddia_0501.png
	SectionID string
	Caption   string
	Data      []byte
}

// Parse reads an EPUB 2 or 3 file (ING-1).
func Parse(r io.ReaderAt, size int64) (*Book, error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("not a zip/epub: %w", err)
	}
	files := map[string]*zip.File{}
	for _, f := range zr.File {
		files[f.Name] = f
	}
	read := func(name string) ([]byte, error) {
		f, ok := files[name]
		if !ok {
			return nil, fmt.Errorf("missing %s", name)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return io.ReadAll(rc)
	}

	var container struct {
		Rootfiles []struct {
			FullPath string `xml:"full-path,attr"`
		} `xml:"rootfiles>rootfile"`
	}
	b, err := read("META-INF/container.xml")
	if err != nil {
		return nil, err
	}
	if err := xml.Unmarshal(b, &container); err != nil || len(container.Rootfiles) == 0 {
		return nil, fmt.Errorf("bad container.xml: %v", err)
	}
	opfPath := container.Rootfiles[0].FullPath
	var opf struct {
		Title    string `xml:"metadata>title"`
		Manifest []struct {
			ID   string `xml:"id,attr"`
			Href string `xml:"href,attr"`
		} `xml:"manifest>item"`
		Spine []struct {
			IDRef string `xml:"idref,attr"`
		} `xml:"spine>itemref"`
	}
	if b, err = read(opfPath); err != nil {
		return nil, err
	}
	if err := xml.Unmarshal(b, &opf); err != nil {
		return nil, fmt.Errorf("bad OPF: %w", err)
	}
	hrefs := map[string]string{}
	for _, it := range opf.Manifest {
		hrefs[it.ID] = path.Join(path.Dir(opfPath), it.Href)
	}

	book := &Book{Title: strings.TrimSpace(opf.Title)}
	chapterNo := 0
	// ING-2: chapter order comes from the spine.
	for _, ref := range opf.Spine {
		docPath, ok := hrefs[ref.IDRef]
		if !ok {
			continue
		}
		raw, err := read(docPath)
		if err != nil {
			return nil, err
		}
		doc, err := html.Parse(bytes.NewReader(raw))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", docPath, err)
		}
		root, kind := chapterRoot(doc)
		if kind != "chapter" && kind != "appendix" {
			continue // front and back matter are not studied
		}
		chapterNo++
		p := &parser{book: book, chapter: chapterNo, docDir: path.Dir(docPath), read: read}
		id := fmt.Sprintf("ch%02d", chapterNo)
		if kind == "appendix" {
			id = fmt.Sprintf("app%c", 'A'+rune(chapterNo-1))
		}
		p.parseChapter(root, id)
	}
	if chapterNo == 0 {
		return nil, fmt.Errorf("no chapters found in spine")
	}
	return book, nil
}

// chapterRoot finds the HTMLBook chapter element, or falls back to <body>
// when the spine item looks like a chapter (it has an h1 or h2).
func chapterRoot(doc *html.Node) (*html.Node, string) {
	if n := find(doc, func(n *html.Node) bool {
		t := attr(n, "data-type")
		return n.Data == "section" && (t == "chapter" || t == "appendix")
	}); n != nil {
		return n, attr(n, "data-type")
	}
	if find(doc, func(n *html.Node) bool { return n.Data == "section" && attr(n, "data-type") != "" }) != nil {
		return nil, "other" // HTMLBook front/back matter (preface, index, …)
	}
	body := find(doc, func(n *html.Node) bool { return n.Data == "body" })
	if body != nil && find(body, func(n *html.Node) bool { return n.Data == "h1" || n.Data == "h2" }) != nil {
		return body, "chapter"
	}
	return nil, "other"
}

type parser struct {
	book    *Book
	chapter int
	docDir  string
	read    func(string) ([]byte, error)
}

func (p *parser) add(s Section) int {
	p.book.Sections = append(p.book.Sections, s)
	return len(p.book.Sections) - 1
}

func (p *parser) parseChapter(root *html.Node, id string) {
	heading := ""
	if h := firstHeading(root); h != nil {
		heading = cleanText(h)
	}
	if heading == "" {
		heading = id
	}
	idx := p.add(Section{ID: id, Chapter: p.chapter, Level: 0, Heading: heading})
	nested := find(root, func(n *html.Node) bool { return isSubsection(n) }) != nil
	if nested {
		p.walkSection(root, idx)
	} else {
		p.walkFlat(root, idx)
	}
	p.finish()
}

// walkSection handles HTMLBook: the section's own content goes into idx,
// nested sections become children, footnote blocks become reference sections.
func (p *parser) walkSection(n *html.Node, idx int) {
	var buf strings.Builder
	skippedHeading := false
	childNo := 0
	flush := func() {
		p.book.Sections[idx].Markdown += buf.String()
		buf.Reset()
	}
	var visit func(n *html.Node)
	visit = func(n *html.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			switch {
			case isSubsection(c):
				flush()
				childNo++
				parent := p.book.Sections[idx]
				child := Section{
					ID:       fmt.Sprintf("%s.s%02d", parent.ID, childNo),
					ParentID: parent.ID,
					Chapter:  p.chapter,
					Level:    parent.Level + 1,
				}
				if h := firstHeading(c); h != nil {
					child.Heading = cleanText(h)
				}
				p.walkSection(c, p.add(child))
			case c.Type == html.ElementNode && attr(c, "data-type") == "footnotes":
				flush()
				p.addReferences(c)
			case !skippedHeading && isHeading(c):
				skippedHeading = true // the section's own title
			case isWrapper(c):
				visit(c) // HTMLBook wraps section bodies in <div class="sect1">
			default:
				p.block(&buf, c, p.book.Sections[idx].ID)
			}
		}
	}
	visit(n)
	flush()
}

// isWrapper reports a plain <div> that only groups a section's content and
// holds its heading or nested sections.
func isWrapper(n *html.Node) bool {
	if n.Type != html.ElementNode || n.Data != "div" || attr(n, "data-type") != "" {
		return false
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if isHeading(c) || isSubsection(c) || (c.Type == html.ElementNode && attr(c, "data-type") == "footnotes") {
			return true
		}
	}
	return false
}

// walkFlat is the fallback for chapters without nested sections: h2–h4
// start new sections at levels 1–3 (ING-3).
func (p *parser) walkFlat(root *html.Node, chapterIdx int) {
	stack := []int{chapterIdx} // stack[level] = section index
	cur := chapterIdx
	counters := map[string]int{}
	skippedTitle := false
	var visit func(n *html.Node)
	visit = func(n *html.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if isHeading(c) {
				lvl := int(c.Data[1] - '1') // h2 → 1
				if c.Data == "h1" && !skippedTitle {
					skippedTitle = true
					continue
				}
				if lvl >= 1 && lvl <= 3 {
					for len(stack) > lvl {
						stack = stack[:len(stack)-1]
					}
					for len(stack) < lvl {
						stack = append(stack, stack[len(stack)-1])
					}
					parent := p.book.Sections[stack[lvl-1]]
					counters[parent.ID]++
					cur = p.add(Section{
						ID:       fmt.Sprintf("%s.s%02d", parent.ID, counters[parent.ID]),
						ParentID: parent.ID,
						Chapter:  p.chapter,
						Level:    lvl,
						Heading:  cleanText(c),
					})
					stack = append(stack, cur)
					continue
				}
			}
			if c.Type == html.ElementNode && (c.Data == "div" || c.Data == "section") && attr(c, "data-type") == "" && find(c, isHeading) != nil {
				visit(c) // wrapper div: look inside
				continue
			}
			var buf strings.Builder
			p.block(&buf, c, p.book.Sections[cur].ID)
			p.book.Sections[cur].Markdown += buf.String()
		}
	}
	visit(root)
}

func (p *parser) addReferences(n *html.Node) {
	var buf strings.Builder
	title := "References"
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if isHeading(c) {
			title = cleanText(c)
			continue
		}
		p.block(&buf, c, "")
	}
	if strings.TrimSpace(buf.String()) == "" {
		return
	}
	chapterID := ""
	for i := len(p.book.Sections) - 1; i >= 0; i-- {
		if p.book.Sections[i].Level == 0 {
			chapterID = p.book.Sections[i].ID
			break
		}
	}
	id := chapterID + ".refs"
	for _, s := range p.book.Sections {
		if s.ID == id {
			id = chapterID + ".refs2"
		}
	}
	p.add(Section{ID: id, ParentID: chapterID, Chapter: p.chapter, Level: 1, Heading: title, Markdown: buf.String(), IsReference: true})
}

// finish tidies Markdown and counts words for the chapter just parsed.
func (p *parser) finish() {
	for i := range p.book.Sections {
		s := &p.book.Sections[i]
		if s.Chapter != p.chapter {
			continue
		}
		s.Markdown = tidy(s.Markdown)
		s.Words = len(strings.Fields(s.Markdown))
	}
}

func isSubsection(n *html.Node) bool {
	if n.Type != html.ElementNode || n.Data != "section" {
		return false
	}
	return strings.HasPrefix(attr(n, "data-type"), "sect")
}

func isHeading(n *html.Node) bool {
	if n.Type != html.ElementNode || len(n.Data) != 2 || n.Data[0] != 'h' {
		return false
	}
	return n.Data[1] >= '1' && n.Data[1] <= '6'
}

func firstHeading(n *html.Node) *html.Node {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if isHeading(c) {
			return c
		}
		if c.Type == html.ElementNode && (c.Data == "header" || isWrapper(c)) {
			if h := firstHeading(c); h != nil {
				return h
			}
		}
	}
	return nil
}

func find(n *html.Node, pred func(*html.Node) bool) *html.Node {
	if n.Type == html.ElementNode && pred(n) {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if r := find(c, pred); r != nil {
			return r
		}
	}
	return nil
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
