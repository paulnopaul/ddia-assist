package plan

import (
	"fmt"
	"testing"
)

// chapter builds a chapter with one level-1 section per entry of words.
func chapter(ch int, intro int, words ...int) []Section {
	secs := []Section{{ID: fmt.Sprintf("ch%02d", ch), Chapter: ch, Level: 0, Heading: "Intro", Words: intro}}
	for i, w := range words {
		secs = append(secs, Section{ID: fmt.Sprintf("ch%02d.s%02d", ch, i+1), Chapter: ch, Level: 1, Heading: fmt.Sprintf("S%d", i+1), Words: w})
	}
	return secs
}

func TestPlan_ING9_NeverCrossesChapter(t *testing.T) {
	secs := append(chapter(1, 100, 500), chapter(2, 100, 500)...)
	units := Plan(secs)
	if len(units) != 2 {
		t.Fatalf("got %d units, want 2 (one per chapter)", len(units))
	}
	for _, u := range units {
		for _, id := range u.SectionIDs {
			if id[:4] != fmt.Sprintf("ch%02d", u.Chapter) {
				t.Fatalf("unit in chapter %d contains %s", u.Chapter, id)
			}
		}
	}
}

func TestPlan_ING10_TargetsWordRange(t *testing.T) {
	units := Plan(chapter(1, 300, 2500, 2500, 3000, 3000, 2000, 2500, 400))
	for i, u := range units {
		if u.Words > MaxWords {
			t.Errorf("unit %d has %d words, more than %d", i, u.Words, MaxWords)
		}
		if i < len(units)-1 && u.Words < MinWords {
			t.Errorf("unit %d has %d words, fewer than %d", i, u.Words, MinWords)
		}
	}
	total := 0
	for _, u := range units {
		total += u.Words
	}
	if total != 300+2500+2500+3000+3000+2000+2500+400 {
		t.Fatalf("words lost: %d", total)
	}
}

func TestPlan_ING10_IntroJoinsFirstUnitAndTinyTailMerges(t *testing.T) {
	units := Plan(chapter(1, 300, 4500, 4500, 600))
	if len(units) != 2 {
		t.Fatalf("got %d units, want 2", len(units))
	}
	if units[0].SectionIDs[0] != "ch01" {
		t.Errorf("intro not in first unit: %v", units[0].SectionIDs)
	}
	if got := units[1].SectionIDs; got[len(got)-1] != "ch01.s03" {
		t.Errorf("tail not merged into last unit: %v", got)
	}
}

func TestPlan_ING10_SplitsLongSectionAtSubsections(t *testing.T) {
	secs := []Section{
		{ID: "ch01", Chapter: 1, Level: 0, Words: 100},
		{ID: "ch01.s01", Chapter: 1, Level: 1, Heading: "Big", Words: 200},
		{ID: "ch01.s01.s01", Chapter: 1, Level: 2, Heading: "A", Words: 5000},
		{ID: "ch01.s01.s02", Chapter: 1, Level: 2, Heading: "B", Words: 5000},
	}
	units := Plan(secs)
	if len(units) != 2 || units[1].SectionIDs[0] != "ch01.s01.s02" {
		t.Fatalf("units = %+v", units)
	}
}

func TestPlan_ReferencesExcluded(t *testing.T) {
	secs := append(chapter(1, 100, 500), Section{ID: "ch01.refs", Chapter: 1, Level: 1, Words: 9000, IsReference: true})
	units := Plan(secs)
	if len(units) != 1 || units[0].Words != 600 {
		t.Fatalf("units = %+v", units)
	}
}

func TestTitle_ING12(t *testing.T) {
	if got := title([]string{"A"}); got != "A" {
		t.Error(got)
	}
	if got := title([]string{"A", "B", "C"}); got != "A – C" {
		t.Error(got)
	}
}
