package bindings

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestPackCommitGameSelector(t *testing.T) {
	data, err := PackCommitGame(1, ChoiceDeal, HustleBuy, 0, 5)
	if err != nil {
		t.Fatalf("pack: %v", err)
	}
	if len(data) < 4 {
		t.Fatal("calldata too short")
	}
	// Selector must equal the method ID for the real signature.
	want := pveABI.Methods["commitGame"].ID
	if string(data[:4]) != string(want) {
		t.Errorf("selector mismatch: got %x want %x", data[:4], want)
	}
	// 5 args × 32 bytes + 4 selector.
	if len(data) != 4+5*32 {
		t.Errorf("calldata len = %d, want %d", len(data), 4+5*32)
	}
}

func TestParseCommitSeq(t *testing.T) {
	pve := common.HexToAddress("0x9D6dc92F71416943aB7ee2653c681dC403107149")
	ev := pveABI.Events["GameCommitted"]

	seq := uint64(4242)
	seqTopic := common.BigToHash(new(big.Int).SetUint64(seq))
	logs := []*types.Log{
		{Address: common.HexToAddress("0xdead"), Topics: []common.Hash{{0x1}}}, // noise
		{Address: pve, Topics: []common.Hash{ev.ID, seqTopic, common.BigToHash(big.NewInt(1)), {}}},
	}
	got, err := ParseCommitSeq(logs, pve)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got != seq {
		t.Errorf("seq = %d, want %d", got, seq)
	}

	if _, err := ParseCommitSeq(logs[:1], pve); err == nil {
		t.Error("expected error when no GameCommitted log present")
	}
}

func TestParseGameResultWin(t *testing.T) {
	pve := common.HexToAddress("0x9D6dc92F71416943aB7ee2653c681dC403107149")
	ev := pveABI.Events["GamePlayed"]

	// Encode the non-indexed payload for a WIN.
	data, err := ev.Inputs.NonIndexed().Pack(
		uint8(0),                 // playerChoice DEAL
		uint8(0),                 // houseChoice
		uint8(OutcomeWin),        // outcome WIN
		uint8(0),                 // hustleType BUY
		big.NewInt(0),            // drugId
		big.NewInt(5),            // drugAmount
		big.NewInt(-120),         // cashChange
		big.NewInt(36),           // reputationChange
		big.NewInt(5),            // drugBalanceChange
		uint8(1),                 // newHeatLevel
		big.NewInt(120),          // stakedCash
		big.NewInt(0),            // stakedDrug
	)
	if err != nil {
		t.Fatalf("pack GamePlayed data: %v", err)
	}
	logs := []*types.Log{{
		Address: pve,
		Topics:  []common.Hash{ev.ID, common.BigToHash(big.NewInt(1)), {}},
		Data:    data,
	}}

	res, err := ParseGameResult(logs, pve)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !res.Played || res.Arrested || res.Expired {
		t.Fatalf("flags wrong: %+v", res)
	}
	if res.Outcome != OutcomeWin || res.Outcome.String() != "WIN" {
		t.Errorf("outcome = %v", res.Outcome)
	}
	if res.RepChange.Int64() != 36 || res.CashChange.Int64() != -120 || res.NewHeat != 1 {
		t.Errorf("deltas wrong: rep=%s cash=%s heat=%d", res.RepChange, res.CashChange, res.NewHeat)
	}
}

func TestParseGameResultArrestedAndExpired(t *testing.T) {
	pve := common.HexToAddress("0x9D6dc92F71416943aB7ee2653c681dC403107149")

	arr := pveABI.Events["DealerArrested"]
	arrData, _ := arr.Inputs.NonIndexed().Pack(uint16(35))
	arrLogs := []*types.Log{{Address: pve, Topics: []common.Hash{arr.ID, common.BigToHash(big.NewInt(1)), {}}, Data: arrData}}
	res, err := ParseGameResult(arrLogs, pve)
	if err != nil || !res.Arrested || res.Played {
		t.Fatalf("arrested parse wrong: res=%+v err=%v", res, err)
	}

	exp := pveABI.Events["GameExpired"]
	expLogs := []*types.Log{{Address: pve, Topics: []common.Hash{exp.ID, common.BigToHash(big.NewInt(7)), common.BigToHash(big.NewInt(1))}}}
	res2, err := ParseGameResult(expLogs, pve)
	if err != nil || !res2.Expired || res2.Played {
		t.Fatalf("expired parse wrong: res=%+v err=%v", res2, err)
	}
}
