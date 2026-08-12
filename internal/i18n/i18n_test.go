package i18n

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseLang(t *testing.T) {
	cases := map[string]Lang{"ru": RU, "RU": RU, "russian": RU, "en": EN, "English": EN}
	for in, want := range cases {
		if got, ok := ParseLang(in); !ok || got != want {
			t.Errorf("ParseLang(%q) = (%v,%v), want (%v,true)", in, got, ok, want)
		}
	}
	if _, ok := ParseLang("xx"); ok {
		t.Error("ParseLang(\"xx\") should be !ok")
	}
}

func TestTFallsBackAndFormats(t *testing.T) {
	add(map[string][2]string{
		"test.hi":     {"Привет %s", "Hi %s"},
		"test.ruOnly": {"ТолькоРусский", ""},
	})
	Use(RU)
	if got := T("test.hi", "Боб"); got != "Привет Боб" {
		t.Errorf("RU format = %q", got)
	}
	Use(EN)
	if got := T("test.hi", "Bob"); got != "Hi Bob" {
		t.Errorf("EN format = %q", got)
	}
	// EN empty → falls back to RU so nothing renders blank.
	if got := T("test.ruOnly"); got != "ТолькоРусский" {
		t.Errorf("empty-EN fallback = %q", got)
	}
	// Unknown key renders as the key itself.
	if got := T("test.nope"); got != "test.nope" {
		t.Errorf("unknown key = %q", got)
	}
	Use(RU) // restore default for other tests
}

func TestStorePersistsAndApplies(t *testing.T) {
	p := filepath.Join(t.TempDir(), "language.json")
	s := Load(p) // missing file → RU default
	if s.Lang() != RU || Current() != RU {
		t.Fatalf("default lang = %v / global %v, want RU", s.Lang(), Current())
	}
	l, err := s.Toggle()
	if err != nil || l != EN || Current() != EN {
		t.Fatalf("Toggle → (%v,%v), global %v; want EN", l, err, Current())
	}
	// Reload from disk: the choice survives and re-applies globally.
	Use(RU)
	if Load(p).Lang() != EN || Current() != EN {
		t.Error("language did not persist/apply on reload")
	}
	Use(RU)
}

func TestSaveIsValidJSON(t *testing.T) {
	p := filepath.Join(t.TempDir(), "language.json")
	if err := save(p, EN); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(p)
	if want := "\"language\": \"en\""; !contains(string(data), want) {
		t.Errorf("saved file %s missing %q", data, want)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
