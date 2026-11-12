package store

import "testing"

func TestConceptRange_SES18_FollowsUnitSize(t *testing.T) {
	sec := func(words int) Section { return Section{Words: words} }
	small := Unit{Words: 1200, Sections: []Section{sec(600), sec(600)}}
	big := Unit{Words: 7000, Sections: []Section{sec(1000), sec(1000), sec(1000), sec(1000), sec(1000), sec(1000), sec(1000), {Words: 100}, {Words: 900, IsReference: true}}}
	slo, shi := small.ConceptRange()
	blo, bhi := big.ConceptRange()
	if slo != MinConcepts || shi < slo {
		t.Errorf("small unit range = %d-%d", slo, shi)
	}
	if blo != 7 || bhi != 15 {
		t.Errorf("big unit range = %d-%d, want 7-15 (7 content sections, 7000/400 capped)", blo, bhi)
	}
	if blo <= slo || bhi <= shi {
		t.Errorf("range should grow with the unit: small %d-%d, big %d-%d", slo, shi, blo, bhi)
	}
}
