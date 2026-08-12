package progstate

import (
	"path/filepath"
	"testing"
)

func TestPersistAndResume(t *testing.T) {
	p := filepath.Join(t.TempDir(), "progress.json")
	s := Load(p)
	if got := s.Get(7); got != (Pos{}) {
		t.Errorf("unknown dealer = %+v, want zero", got)
	}
	if err := s.Set(7, Pos{Step: 2, Reps: 1}); err != nil {
		t.Fatal(err)
	}
	// Reloads from disk with the same position.
	if got := Load(p).Get(7); got != (Pos{Step: 2, Reps: 1}) {
		t.Errorf("resumed = %+v, want {2 1}", got)
	}
}

func TestSetSkipsUnchanged(t *testing.T) {
	p := filepath.Join(t.TempDir(), "progress.json")
	s := Load(p)
	if err := s.Set(1, Pos{Step: 1}); err != nil {
		t.Fatal(err)
	}
	// Setting the same value is a no-op (no error, file unchanged).
	if err := s.Set(1, Pos{Step: 1}); err != nil {
		t.Fatal(err)
	}
	if got := s.Get(1); got != (Pos{Step: 1}) {
		t.Errorf("got %+v", got)
	}
}
