package dealer

import (
	"math/big"
	"testing"
)

func TestRankDealers(t *testing.T) {
	scores := []dealerScore{
		{ID: 24, Rep: big.NewInt(188), Infamy: big.NewInt(0)},
		{ID: 26, Rep: big.NewInt(525), Infamy: big.NewInt(8)},
		{ID: 25, Rep: big.NewInt(175), Infamy: big.NewInt(0)}, // ties #24 on infamy → smaller id wins
		{ID: 10, Rep: big.NewInt(525), Infamy: big.NewInt(3)}, // ties #26 on rep → smaller id wins
	}
	r := rankDealers(scores)

	// PVE by rep desc, tie → smaller id: 10 & 26 both 525 → #10 first (id 10<26),
	// then 26, then 24 (188), then 25 (175).
	if r[10].Pve != 1 || r[26].Pve != 2 || r[24].Pve != 3 || r[25].Pve != 4 {
		t.Errorf("PVE ranks wrong: 10=%d 26=%d 24=%d 25=%d", r[10].Pve, r[26].Pve, r[24].Pve, r[25].Pve)
	}

	// PVP by infamy desc: 26(8) #1, 10(3) #2, then 24 & 25 both 0 → smaller id
	// (24) #3, 25 #4.
	if r[26].Pvp != 1 || r[10].Pvp != 2 || r[24].Pvp != 3 || r[25].Pvp != 4 {
		t.Errorf("PVP ranks wrong: 26=%d 10=%d 24=%d 25=%d", r[26].Pvp, r[10].Pvp, r[24].Pvp, r[25].Pvp)
	}
}

func TestLeaderboardCacheGet(t *testing.T) {
	c := NewLeaderboardCache()
	if _, ok := c.Get(1); ok {
		t.Error("empty cache should return ok=false")
	}
	c.mu.Lock()
	c.ranks = map[uint64]Ranks{7: {Pve: 3, Pvp: 5}}
	c.mu.Unlock()
	if r, ok := c.Get(7); !ok || r.Pve != 3 || r.Pvp != 5 {
		t.Errorf("Get(7) = %+v ok=%v", r, ok)
	}
}
