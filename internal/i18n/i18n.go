// Package i18n provides a tiny message catalog with a runtime-switchable UI
// language — Russian (default) or English. Screens render user-facing text with
// T(key, args...); each screen registers its own messages from a cat_*.go
// init() so the catalog is split by screen and concurrent edits don't collide.
//
// The active language is a process-global (the TUI renders on one goroutine, but
// background code may read it too) guarded by an atomic, so a switch on the
// Settings screen takes effect on the next render with no restart. Persistence
// lives in Store (language.json), mirroring the other UI-managed stores.
package i18n

import (
	"fmt"
	"strings"
	"sync/atomic"
)

// Lang identifies a supported language.
type Lang int32

const (
	RU Lang = iota // Russian (default)
	EN             // English
)

// current holds the active language (atomic; RU == 0 is the zero value/default).
var current atomic.Int32

// Use switches the active language process-wide.
func Use(l Lang) { current.Store(int32(l)) }

// Current returns the active language.
func Current() Lang { return Lang(current.Load()) }

// ParseLang maps a code to a Lang; unknown input returns (RU, false).
func ParseLang(code string) (Lang, bool) {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "en", "eng", "english":
		return EN, true
	case "ru", "rus", "russian", "русский":
		return RU, true
	}
	return RU, false
}

// Code returns the short code ("ru"/"en") for persistence.
func (l Lang) Code() string {
	if l == EN {
		return "en"
	}
	return "ru"
}

// Name returns the language's own-name, for the language switch.
func (l Lang) Name() string {
	if l == EN {
		return "English"
	}
	return "Русский"
}

// catalog maps a message key → [RU, EN]. Populated by add() from cat_*.go inits.
var catalog = map[string][2]string{}

// add merges a screen's messages into the catalog (called from cat_*.go init()).
// A duplicate key overwrites — keys are namespaced per screen to avoid that.
func add(m map[string][2]string) {
	for k, v := range m {
		catalog[k] = v
	}
}

// T returns the message for key in the active language, formatted with args
// (fmt.Sprintf) when any are given. A missing key or empty translation falls
// back (chosen → RU → EN → the key itself) so text is never blank; an unknown
// key renders as the key, making a gap obvious instead of silent.
func T(key string, args ...any) string {
	s := ""
	if m, ok := catalog[key]; ok {
		if i := int(Current()); i >= 0 && i < len(m) {
			s = m[i]
		}
		if s == "" {
			s = m[RU]
		}
		if s == "" {
			s = m[EN]
		}
	}
	if s == "" {
		s = key
	}
	if len(args) > 0 {
		return fmt.Sprintf(s, args...)
	}
	return s
}
