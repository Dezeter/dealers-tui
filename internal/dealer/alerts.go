package dealer

import (
	"fmt"
	"math/big"
	"sort"
)

// AlertLevel ranks an alert. Crit sorts before Warn.
type AlertLevel int

const (
	AlertWarn AlertLevel = iota
	AlertCrit
)

// Alert is one fleet-wide condition worth surfacing (FR10).
type Alert struct {
	Level AlertLevel
	Text  string
}

// FleetAlerts derives the alert list from the latest fleet snapshots plus the
// wallet balance. Rules (FR10): a jailed dealer is critical; heat ≥ 4 is a
// warning (arrest roll ≈ heat×0.7%); an ETH balance below the configured runway
// is critical because write actions will start failing.
func FleetAlerts(snaps []Snapshot, balanceWei, minRunwayWei *big.Int) []Alert {
	var out []Alert
	for _, s := range snaps {
		if s.State == nil {
			continue
		}
		switch {
		case s.State.IsJailed:
			out = append(out, Alert{AlertCrit, fmt.Sprintf("#%d JAILED", s.TokenID)})
		case s.State.HeatLevel >= 4:
			out = append(out, Alert{AlertWarn, fmt.Sprintf("#%d heat %d/5", s.TokenID, s.State.HeatLevel)})
		}
	}
	if balanceWei != nil && minRunwayWei != nil && minRunwayWei.Sign() > 0 && balanceWei.Cmp(minRunwayWei) < 0 {
		out = append(out, Alert{AlertCrit, fmt.Sprintf("ETH runway low: %s < %s", EthStr(balanceWei), EthStr(minRunwayWei))})
	}

	sort.SliceStable(out, func(i, j int) bool { return out[i].Level > out[j].Level })
	return out
}

// EthStr renders a wei amount as a short ETH string.
func EthStr(wei *big.Int) string {
	if wei == nil {
		return "?"
	}
	f := new(big.Float).Quo(new(big.Float).SetInt(wei), big.NewFloat(1e18))
	return f.Text('f', 4) + " ETH"
}
