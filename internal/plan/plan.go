// Package plan groups a chapter's sections into reading units
// (docs/spec/01-ingest.md, ING-9 to ING-12).
package plan

import "strings"

// Section is the planner's view of a section, in document order.
type Section struct {
	ID          string
	ParentID    string
	Chapter     int
	Level       int
	Heading     string
	Words       int
	IsReference bool
}

// Unit is a run of consecutive sections inside one chapter.
type Unit struct {
	Chapter    int
	Title      string
	SectionIDs []string
	Words      int
}

// Word targets: about 400 words per page, so 10–20 pages.
const (
	MinWords  = 4000
	MaxWords  = 8000
	TailWords = 2000
)

type segment struct {
	ids      []string
	headings []string
	words    int
}

// Plan returns units for all chapters. References never belong to a unit.
func Plan(sections []Section) []Unit {
	var units []Unit
	byChapter := map[int][]Section{}
	var order []int
	for _, s := range sections {
		if s.IsReference || s.Chapter == 0 {
			continue
		}
		if _, ok := byChapter[s.Chapter]; !ok {
			order = append(order, s.Chapter)
		}
		byChapter[s.Chapter] = append(byChapter[s.Chapter], s)
	}
	for _, ch := range order {
		units = append(units, planChapter(ch, byChapter[ch])...)
	}
	return units
}

func planChapter(chapter int, secs []Section) []Unit {
	segs := segments(secs)
	var units []Unit
	var cur segment
	closeCur := func() {
		if len(cur.ids) == 0 {
			return
		}
		units = append(units, Unit{Chapter: chapter, Title: title(cur.headings), SectionIDs: cur.ids, Words: cur.words})
		cur = segment{}
	}
	for _, sg := range segs {
		// Close early rather than overshoot, unless the unit is still tiny.
		if cur.words >= TailWords && cur.words+sg.words > MaxWords {
			closeCur()
		}
		cur.ids = append(cur.ids, sg.ids...)
		cur.headings = append(cur.headings, sg.headings...)
		cur.words += sg.words
		if cur.words >= MinWords {
			closeCur()
		}
	}
	if len(cur.ids) > 0 {
		if cur.words < TailWords && len(units) > 0 {
			last := &units[len(units)-1]
			last.SectionIDs = append(last.SectionIDs, cur.ids...)
			last.Words += cur.words
			last.Title = title(append(splitTitle(last.Title), cur.headings...))
		} else {
			closeCur()
		}
	}
	return units
}

// segments splits a chapter into its level-1 sections (with descendants).
// The chapter's intro joins the first one, and a level-1 section longer than
// MaxWords is split at its level-2 children.
func segments(secs []Section) []segment {
	type group struct {
		head  Section
		items []Section
	}
	var intro []Section
	var groups []group
	for _, s := range secs {
		switch {
		case s.Level == 0:
			intro = append(intro, s)
		case s.Level == 1:
			groups = append(groups, group{head: s, items: []Section{s}})
		case len(groups) == 0:
			intro = append(intro, s)
		default:
			groups[len(groups)-1].items = append(groups[len(groups)-1].items, s)
		}
	}
	var out []segment
	for gi, g := range groups {
		items := g.items
		if gi == 0 {
			items = append(append([]Section{}, intro...), items...)
		}
		total := 0
		for _, s := range items {
			total += s.Words
		}
		if total <= MaxWords {
			out = append(out, toSegment(items, g.head.Heading))
			continue
		}
		// Split at level-2 boundaries, packing greedily up to MaxWords.
		var cur []Section
		curWords := 0
		curHead := g.head.Heading
		for _, s := range items {
			if s.Level == 2 && curWords > 0 && curWords+subtreeWords(items, s) > MaxWords {
				out = append(out, toSegment(cur, curHead))
				cur, curWords, curHead = nil, 0, s.Heading
			}
			cur = append(cur, s)
			curWords += s.Words
		}
		out = append(out, toSegment(cur, curHead))
	}
	if len(groups) == 0 && len(intro) > 0 {
		out = append(out, toSegment(intro, intro[0].Heading))
	}
	return out
}

func subtreeWords(items []Section, root Section) int {
	n, in := 0, false
	for _, s := range items {
		if s.ID == root.ID {
			in = true
		} else if in && s.Level <= root.Level {
			break
		}
		if in {
			n += s.Words
		}
	}
	return n
}

func toSegment(items []Section, heading string) segment {
	sg := segment{headings: []string{heading}}
	for _, s := range items {
		sg.ids = append(sg.ids, s.ID)
		sg.words += s.Words
	}
	return sg
}

const titleSep = " – "

// title is the first heading, or "A – B" for a unit spanning several (ING-12).
func title(headings []string) string {
	var hs []string
	for _, h := range headings {
		if h != "" && (len(hs) == 0 || hs[len(hs)-1] != h) {
			hs = append(hs, h)
		}
	}
	switch len(hs) {
	case 0:
		return "Untitled"
	case 1:
		return hs[0]
	default:
		return hs[0] + titleSep + hs[len(hs)-1]
	}
}

func splitTitle(t string) []string { return strings.Split(t, titleSep) }
