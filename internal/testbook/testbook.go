// Package testbook builds small synthetic EPUBs for tests. The repo is public,
// so tests never use real book content.
package testbook

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
)

// PNG is a 1×1 transparent image.
var PNG = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89\x00\x00\x00\rIDATx\x9cc\xf8\x0f\x00\x00\x01\x01\x00\x05\x18\xd8N\x00\x00\x00\x00IEND\xaeB`\x82")

// Words returns n filler words with a marker word for search tests.
func Words(n int, marker string) string {
	w := make([]string, n)
	for i := range w {
		w[i] = "lorem"
	}
	if n > 0 {
		w[n/2] = marker
	}
	return strings.Join(w, " ")
}

// HTMLBook returns an EPUB 3 laid out like an O'Reilly book: a preface
// (skipped), two chapters with nested sections, a figure, a note, a code
// block and a references block.
func HTMLBook() []byte {
	ch1 := `<section data-type="chapter"><div class="chapter"><h1>Chapter 1. Storage</h1>
<p>` + Words(300, "introword") + `</p>
<section data-type="sect1"><div class="sect1"><h1>Logs</h1>
<p>` + Words(2500, "appendonly") + `</p>
<figure><div class="figure"><img src="assets/fig1.png" alt="alt text"/><h6><span class="label">Figure 1-1. </span>A log</h6></div></figure>
<section data-type="sect2"><div class="sect2"><h2>Compaction</h2>
<p>` + Words(1800, "compaction") + ` <a data-type="indexterm" data-primary="hidden"></a><a data-type="noteref" href="#r1">1</a></p>
<div data-type="note"><h1>Careful</h1><p>A note with <code>fsync()</code> and <em>emphasis</em>.</p></div>
<pre data-type="programlisting">put(k, v)
get(k)</pre>
</div></section></div></section>
<section data-type="sect1"><div class="sect1"><h1>Trees</h1>
<p>` + Words(4200, "btree") + `</p>
<ul><li><p>first item</p></li><li><p>second item</p></li></ul>
</div></section>
<section data-type="sect1"><div class="sect1"><h1>Summary</h1><p>` + Words(400, "summaryword") + `</p></div></section>
</div><div data-type="footnotes"><h5>References</h5><p data-type="footnote" id="r1">[1] Someone, "A paper", 2001.</p></div></section>`
	ch2 := `<section data-type="chapter"><div class="chapter"><h1>Chapter 2. Replication</h1>
<p>` + Words(200, "replintro") + `</p>
<section data-type="sect1"><div class="sect1"><h1>Leaders</h1><p>` + Words(3000, "leaderword") + `</p>
<table><tr><th>Mode</th><th>Latency</th></tr><tr><td>sync</td><td>high</td></tr></table>
</div></section>
<section data-type="sect1"><div class="sect1"><h1>Quorums</h1><p>` + Words(1500, "quorumword") + `</p></div></section>
</div></section>`
	preface := `<section data-type="preface"><h1>Preface</h1><p>` + Words(100, "prefaceword") + `</p></section>`
	return Build("Test Book", []Doc{{Name: "preface.html", Body: preface}, {Name: "ch01.html", Body: ch1}, {Name: "ch02.html", Body: ch2}},
		map[string][]byte{"assets/fig1.png": PNG})
}

// Flat returns an EPUB without HTMLBook sections, split only by headings.
func Flat() []byte {
	ch := `<h1>Chapter One</h1><p>` + Words(100, "flatintro") + `</p>
<h2>Part A</h2><p>` + Words(500, "parta") + `</p>
<h3>Detail</h3><p>` + Words(200, "detail") + `</p>
<h2>Part B</h2><p>` + Words(300, "partb") + `</p>`
	return Build("Flat Book", []Doc{{Name: "c1.xhtml", Body: ch}}, nil)
}

type Doc struct{ Name, Body string }

// Build assembles an EPUB 3 from spine documents and extra files under OEBPS/.
func Build(title string, docs []Doc, files map[string][]byte) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	put := func(name string, data []byte) {
		w, _ := zw.Create(name)
		w.Write(data)
	}
	put("mimetype", []byte("application/epub+zip"))
	put("META-INF/container.xml", []byte(`<?xml version="1.0"?><container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`))
	var manifest, spine strings.Builder
	for i, d := range docs {
		fmt.Fprintf(&manifest, `<item id="d%d" href="%s" media-type="application/xhtml+xml"/>`, i, d.Name)
		fmt.Fprintf(&spine, `<itemref idref="d%d"/>`, i)
		put("OEBPS/"+d.Name, []byte(`<?xml version="1.0" encoding="UTF-8"?><html xmlns="http://www.w3.org/1999/xhtml"><head><title>x</title></head><body>`+d.Body+`</body></html>`))
	}
	for name, data := range files {
		put("OEBPS/"+name, data)
	}
	put("OEBPS/content.opf", []byte(`<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf" version="3.0"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>`+title+`</dc:title></metadata><manifest>`+manifest.String()+`</manifest><spine>`+spine.String()+`</spine></package>`))
	zw.Close()
	return buf.Bytes()
}
