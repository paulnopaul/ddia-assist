package epub

import (
	"bytes"
	"strings"
	"testing"

	"github.com/paulnopaul/ddia-assist/internal/testbook"
)

func parse(t *testing.T, data []byte) *Book {
	t.Helper()
	b, err := Parse(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func byID(b *Book) map[string]Section {
	m := map[string]Section{}
	for _, s := range b.Sections {
		m[s.ID] = s
	}
	return m
}

func TestParse_ING2_ING3_ING7_HTMLBookTree(t *testing.T) {
	b := parse(t, testbook.HTMLBook())
	var ids []string
	for _, s := range b.Sections {
		ids = append(ids, s.ID)
	}
	want := "ch01 ch01.s01 ch01.s01.s01 ch01.s02 ch01.s03 ch01.refs ch02 ch02.s01 ch02.s02"
	if got := strings.Join(ids, " "); got != want {
		t.Fatalf("ids = %s\nwant %s", got, want)
	}
	m := byID(b)
	if m["ch01"].Heading != "Chapter 1. Storage" || m["ch01.s01.s01"].Heading != "Compaction" || m["ch01.s01.s01"].Level != 2 {
		t.Errorf("headings/levels wrong: %+v", m["ch01.s01.s01"])
	}
	if b.Title != "Test Book" {
		t.Errorf("title = %q", b.Title)
	}
}

func TestParse_ING4_Markdown(t *testing.T) {
	m := byID(parse(t, testbook.HTMLBook()))
	c := m["ch01.s01.s01"].Markdown
	for _, want := range []string{"`fsync()`", "*emphasis*", "```\nput(k, v)\nget(k)\n```", "> **Careful**"} {
		if !strings.Contains(c, want) {
			t.Errorf("markdown missing %q:\n%s", want, c)
		}
	}
	if strings.Contains(c, "hidden") {
		t.Error("index term leaked into markdown")
	}
	if !strings.Contains(m["ch01.s02"].Markdown, "- first item") {
		t.Errorf("list missing: %s", m["ch01.s02"].Markdown[len(m["ch01.s02"].Markdown)-60:])
	}
	if !strings.Contains(m["ch02.s01"].Markdown, "| Mode | Latency |") {
		t.Error("table missing")
	}
}

func TestParse_ING5_Figures(t *testing.T) {
	b := parse(t, testbook.HTMLBook())
	if len(b.Figures) != 1 || b.Figures[0].Name != "fig1.png" || b.Figures[0].SectionID != "ch01.s01" || !strings.Contains(b.Figures[0].Caption, "A log") {
		t.Fatalf("figures = %+v", b.Figures)
	}
	if !strings.Contains(byID(b)["ch01.s01"].Markdown, "](figure:fig1.png)") {
		t.Error("figure reference missing from markdown")
	}
}

func TestParse_ING6_References(t *testing.T) {
	m := byID(parse(t, testbook.HTMLBook()))
	if !m["ch01.refs"].IsReference || !strings.Contains(m["ch01.refs"].Markdown, "A paper") {
		t.Fatalf("refs = %+v", m["ch01.refs"])
	}
}

func TestParse_ING3_FlatFallback(t *testing.T) {
	b := parse(t, testbook.Flat())
	var got []string
	for _, s := range b.Sections {
		got = append(got, s.ID+":"+s.Heading)
	}
	want := "ch01:Chapter One ch01.s01:Part A ch01.s01.s01:Detail ch01.s02:Part B"
	if strings.Join(got, " ") != want {
		t.Fatalf("got %v\nwant %s", got, want)
	}
}
