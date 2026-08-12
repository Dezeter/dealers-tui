package tui

import (
	"os"
	"testing"

	"dealers/internal/i18n"
)

// TestMain pins the UI language to English for the whole tui test suite: the
// English catalog entries are the verbatim original strings, so the existing
// assertions (which check English text) keep working regardless of the RU
// default. Localization itself is covered by internal/i18n's own tests.
func TestMain(m *testing.M) {
	i18n.Use(i18n.EN)
	os.Exit(m.Run())
}
