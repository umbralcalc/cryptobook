package conservation

import (
	"testing"

	"github.com/umbralcalc/cryptobook/pkg/claims"
)

// TestConservation is the binding test named by every claim in behaviour.go.
func TestConservation(t *testing.T) {
	for _, claim := range ObservedBehaviour() {
		t.Run(claim.ID, func(t *testing.T) {
			if err := claims.Verify(claim); err != nil {
				t.Errorf("%v", err)
			}
		})
	}
}

// TestEveryAgeModelIsDiscovered guards the discovery itself. Both claims above check
// whatever the glob returns, so a glob that quietly returned nothing — or stopped matching
// a renamed config — would leave them passing while testing less than they say.
func TestEveryAgeModelIsDiscovered(t *testing.T) {
	models, err := discover()
	if err != nil {
		t.Fatal(err)
	}
	found := make(map[string]bool, len(models))
	for _, m := range models {
		found[m.file] = true
		if m.levelWidth != m.ncoh {
			t.Errorf("%s sums %d of its %d cohorts per level; the arrival damping cannot "+
				"see the rest", m.file, m.levelWidth, m.ncoh)
		}
		if m.ncoh*nGroups != m.width {
			t.Errorf("%s declares ages width %d but %d cohorts across %d groups (%d slots); "+
				"the layout this check reads does not describe it", m.file, m.width,
				m.ncoh, nGroups, m.ncoh*nGroups)
		}
	}
	for _, want := range []string{"lob_ages.yaml", "lob_ages_finite.yaml", "lob_ages12.yaml"} {
		if !found[want] {
			t.Errorf("%s was not discovered — if it was renamed, this list and the claims "+
				"that contrast these three configs must move with it", want)
		}
	}
}
