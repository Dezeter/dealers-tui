package i18n

import (
	"encoding/json"
	"os"
	"sync"
)

// Store persists the chosen UI language to a small JSON file (language.json) and
// applies it to the process-global on Load — mirroring the other UI-managed
// stores (allies/strategies/settings/recipes/templates). A missing or invalid
// file means the Russian default.
type Store struct {
	path string
	mu   sync.Mutex
	lang Lang
}

type langFile struct {
	Language string `json:"language"`
}

// Load reads the language file, applies it globally, and returns the store.
func Load(path string) *Store {
	s := &Store{path: path, lang: RU}
	if data, err := os.ReadFile(path); err == nil {
		var f langFile
		if json.Unmarshal(data, &f) == nil {
			if l, ok := ParseLang(f.Language); ok {
				s.lang = l
			}
		}
	}
	Use(s.lang)
	return s
}

// Lang returns the stored language.
func (s *Store) Lang() Lang {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lang
}

// Set applies l globally and persists it.
func (s *Store) Set(l Lang) error {
	s.mu.Lock()
	s.lang = l
	s.mu.Unlock()
	Use(l)
	return save(s.path, l)
}

// Toggle flips RU↔EN, applies + persists, and returns the new language.
func (s *Store) Toggle() (Lang, error) {
	s.mu.Lock()
	if s.lang == RU {
		s.lang = EN
	} else {
		s.lang = RU
	}
	l := s.lang
	s.mu.Unlock()
	Use(l)
	return l, save(s.path, l)
}

func save(path string, l Lang) error {
	data, err := json.MarshalIndent(langFile{Language: l.Code()}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
