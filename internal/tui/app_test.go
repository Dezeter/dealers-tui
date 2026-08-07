package tui

import (
	"math/big"
	"strings"
	"testing"

	"dealers/internal/dealer"
)

func TestAlertBar(t *testing.T) {
	a := App{alerts: []dealer.Alert{
		{Level: dealer.AlertCrit, Text: "#3 JAILED"},
		{Level: dealer.AlertWarn, Text: "#2 heat 5/5"},
	}}
	bar := a.alertBar()
	if !strings.Contains(bar, "JAILED") || !strings.Contains(bar, "heat 5/5") {
		t.Errorf("alert bar missing content: %q", bar)
	}

	if (App{}).alertBar() != "" {
		t.Error("expected empty alert bar with no alerts")
	}
}

func TestWalletLine(t *testing.T) {
	a := App{
		balance: big.NewInt(144786521250000), // ~0.0001 ETH
		deps:    Deps{SpentFn: func() *big.Int { return big.NewInt(2_000_000_000_000_000) }},
	}
	line := a.walletLine()
	if !strings.Contains(line, "bal ") || !strings.Contains(line, "spent ") {
		t.Errorf("wallet line missing balance/spent: %q", line)
	}

	// No spend yet → no "spent" segment.
	a2 := App{balance: big.NewInt(1e18), deps: Deps{SpentFn: func() *big.Int { return big.NewInt(0) }}}
	if strings.Contains(a2.walletLine(), "spent") {
		t.Errorf("should not show spent when zero: %q", a2.walletLine())
	}
}

func TestAppTitleHyperlink(t *testing.T) {
	out := appTitle()
	if !strings.Contains(stripANSI(out), "Dealers Manager by") || !strings.Contains(out, "Dezeter") {
		t.Errorf("title text wrong: %q", out)
	}
	// "Dezeter" is an OSC 8 hyperlink to the author's profile.
	if !strings.Contains(out, "\x1b]8;;"+dezeterURL+"\x1b\\Dezeter\x1b]8;;\x1b\\") {
		t.Errorf("Dezeter is not a well-formed OSC 8 link: %q", out)
	}
}

func TestOSC8Wrapping(t *testing.T) {
	got := osc8("https://example.com", "click")
	want := "\x1b]8;;https://example.com\x1b\\click\x1b]8;;\x1b\\"
	if got != want {
		t.Errorf("osc8 = %q, want %q", got, want)
	}
}

func TestAreaNameFallback(t *testing.T) {
	d := Deps{AreaNames: map[uint8]string{2: "Amsterdam"}}
	if d.AreaName(2) != "Amsterdam" {
		t.Errorf("AreaName(2) = %q, want Amsterdam", d.AreaName(2))
	}
	if d.AreaName(9) != "9" {
		t.Errorf("AreaName(9) = %q, want fallback 9", d.AreaName(9))
	}
}
