package tui

import (
	"strings"
	"testing"

	"dealers/internal/i18n"

	"github.com/charmbracelet/lipgloss"
)

// TestRenderCardFixedHeightRU guards that switching to Russian (whose labels are
// longer than the English originals) does not break the fixed-height card grid:
// content is still hard-clipped to the inner width, so every card stays exactly
// cardHeight rows at any width. Mirrors TestRenderCardFixedHeight but in RU.
func TestRenderCardFixedHeightRU(t *testing.T) {
	i18n.Use(i18n.RU)
	defer i18n.Use(i18n.EN)
	longArea := Deps{AreaNames: map[uint8]string{2: "Very Long District Name"}}
	for _, w := range []int{cardMinWidth, 50, cardMaxWidth} {
		for _, s := range testSnaps() {
			if h := lipgloss.Height(renderCard(longArea, s, false, w)); h != cardHeight {
				t.Errorf("RU card #%d at width %d: height %d, want %d", s.TokenID, w, h, cardHeight)
			}
		}
	}
}

// TestFleetHintLocalized confirms the fleet hint actually changes with the
// language (a smoke test that i18n is wired into the view, not just the catalog).
func TestFleetHintLocalized(t *testing.T) {
	i18n.Use(i18n.RU)
	defer i18n.Use(i18n.EN)
	if ru := i18n.T("fleet.hint"); !strings.Contains(ru, "миссии") {
		t.Errorf("RU fleet hint not localized: %q", ru)
	}
	i18n.Use(i18n.EN)
	if en := i18n.T("fleet.hint"); !strings.Contains(en, "missions") {
		t.Errorf("EN fleet hint wrong: %q", en)
	}
}
