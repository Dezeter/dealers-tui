package tui

import (
	"path/filepath"
	"strings"
	"testing"

	"dealers/internal/dealer"
	"dealers/internal/recipe"

	tea "github.com/charmbracelet/bubbletea"
)

func testSteps(t *testing.T) StepsModel {
	store := recipe.Load(filepath.Join(t.TempDir(), "recipes.json"), dealer.DefaultStepOrder())
	return NewSteps(Deps{Recipe: store})
}

func TestStepsToggleAndReorder(t *testing.T) {
	m := testSteps(t)
	n := len(dealer.DefaultStepOrder())

	// Toggle the first step off.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	all := m.deps.Recipe.All()
	if all[0].On {
		t.Error("enter should toggle the selected step off")
	}
	// Move the selected (index 0) down with "]".
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
	if m.cursor != 1 {
		t.Errorf("cursor should follow the moved step to 1, got %d", m.cursor)
	}
	if m.deps.Recipe.All()[1].ID != dealer.DefaultStepOrder()[0] {
		t.Error("the moved step should now be at index 1")
	}
	// Reset restores default.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	got := m.deps.Recipe.Enabled()
	if len(got) != n {
		t.Errorf("reset should re-enable all %d steps, got %d", n, len(got))
	}
}

func TestStepsViewRendersCoreAndBoxes(t *testing.T) {
	out := stripANSI(testSteps(t).View())
	if !strings.Contains(out, "AUTOPILOT STEPS") || !strings.Contains(out, "Core") {
		t.Errorf("view missing title/core:\n%s", out)
	}
	if !strings.Contains(out, "[x]") {
		t.Errorf("default steps should render as checked:\n%s", out)
	}
}

func TestStepsBack(t *testing.T) {
	m := testSteps(t)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("esc should emit a command")
	}
	if _, ok := cmd().(backToFleetMsg); !ok {
		t.Error("esc should return to the fleet")
	}
}
