package tui

import tea "github.com/charmbracelet/bubbletea"

// cyrToLat maps ЙЦУКЕН (Russian) layout runes to the Latin key at the same
// physical position. Pressing the physical "B" key with the Russian layout
// active produces "и"; we translate it back to "b" so all shortcuts work
// regardless of the active keyboard layout. Digits, Enter, Esc, Tab, Space and
// arrows are layout-independent and need no mapping.
var cyrToLat = map[rune]rune{
	// top letter row (QWERTYUIOP)
	'й': 'q', 'ц': 'w', 'у': 'e', 'к': 'r', 'е': 't', 'н': 'y', 'г': 'u', 'ш': 'i', 'щ': 'o', 'з': 'p', 'х': '[', 'ъ': ']',
	'Й': 'Q', 'Ц': 'W', 'У': 'E', 'К': 'R', 'Е': 'T', 'Н': 'Y', 'Г': 'U', 'Ш': 'I', 'Щ': 'O', 'З': 'P', 'Х': '{', 'Ъ': '}',
	// home row (ASDFGHJKL)
	'ф': 'a', 'ы': 's', 'в': 'd', 'а': 'f', 'п': 'g', 'р': 'h', 'о': 'j', 'л': 'k', 'д': 'l', 'ж': ';', 'э': '\'',
	'Ф': 'A', 'Ы': 'S', 'В': 'D', 'А': 'F', 'П': 'G', 'Р': 'H', 'О': 'J', 'Л': 'K', 'Д': 'L', 'Ж': ':', 'Э': '"',
	// bottom row (ZXCVBNM)
	'я': 'z', 'ч': 'x', 'с': 'c', 'м': 'v', 'и': 'b', 'т': 'n', 'ь': 'm', 'б': ',', 'ю': '.',
	'Я': 'Z', 'Ч': 'X', 'С': 'C', 'М': 'V', 'И': 'B', 'Т': 'N', 'Ь': 'M', 'Б': '<', 'Ю': '>',
	'ё': '`', 'Ё': '~',
}

// normalizeKey rewrites a single Cyrillic-rune key press to the Latin key at the
// same physical position, leaving everything else untouched.
func normalizeKey(msg tea.KeyMsg) tea.KeyMsg {
	if msg.Type != tea.KeyRunes || len(msg.Runes) != 1 {
		return msg
	}
	if lat, ok := cyrToLat[msg.Runes[0]]; ok {
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{lat}, Alt: msg.Alt}
	}
	return msg
}
