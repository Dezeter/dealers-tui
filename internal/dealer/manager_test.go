package dealer

import (
	"bytes"
	"context"
	"io"
	"log"
	"math/big"
	"path/filepath"
	"strings"
	"testing"

	"dealers/internal/chain/bindings"
	"dealers/internal/config"
	"dealers/internal/store"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// mockSender returns canned receipts and records the calldata it was sent.
type mockSender struct {
	agw      common.Address
	receipts []*types.Receipt
	sent     [][]byte
	i        int
}

func (m *mockSender) AGW() common.Address { return m.agw }
func (m *mockSender) SendAndWait(_ context.Context, _ common.Address, data []byte, _ *big.Int) (*types.Receipt, error) {
	m.sent = append(m.sent, data)
	r := m.receipts[m.i]
	m.i++
	return r, nil
}

func testnet(t *testing.T) config.Network {
	t.Helper()
	n, ok := config.Profile("testnet")
	if !ok {
		t.Fatal("no testnet profile")
	}
	return n
}

func commitReceipt(pve common.Address, seq, tokenID, block uint64) *types.Receipt {
	return &types.Receipt{
		Status:      types.ReceiptStatusSuccessful,
		BlockNumber: new(big.Int).SetUint64(block),
		TxHash:      common.HexToHash("0xcommit"),
		Logs: []*types.Log{{
			Address: pve,
			Topics: []common.Hash{
				bindings.EventGameCommitted,
				common.BigToHash(new(big.Int).SetUint64(seq)),
				common.BigToHash(new(big.Int).SetUint64(tokenID)),
				common.HexToHash("0x01"),
			},
		}},
	}
}

func manager(t *testing.T) (*Manager, *store.Store, *mockSender, config.Network) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	net := testnet(t)
	ms := &mockSender{agw: common.HexToAddress("0xEd4234a5f233B5E642D47caff292bdc0591D5656")}
	m := NewManager(net, ms, nil, st, log.New(io.Discard, "", 0)) // nil reader: preflight skipped in tests
	return m, st, ms, net
}

func TestSubmitPVEPersistsPending(t *testing.T) {
	m, st, ms, net := manager(t)
	ms.receipts = []*types.Receipt{commitReceipt(net.Contracts.DealersPVE, 77, 1, 1000)}

	seq, err := m.SubmitPVE(context.Background(), 1, bindings.ChoiceDeal, bindings.HustleBuy, 0, 5)
	if err != nil {
		t.Fatalf("SubmitPVE: %v", err)
	}
	if seq != 77 {
		t.Errorf("seq = %d, want 77", seq)
	}

	rows, _ := st.ListCommitted()
	if len(rows) != 1 {
		t.Fatalf("expected 1 pending, got %d", len(rows))
	}
	p := rows[0]
	if p.RevealBlock != 1000+RevealOffset || p.ExpiryBlock != 1000+ExpiryWindow {
		t.Errorf("windows wrong: reveal=%d expiry=%d", p.RevealBlock, p.ExpiryBlock)
	}
	if p.Kind != store.KindPVE || p.MetaJSON == "" {
		t.Errorf("pending wrong: %+v", p)
	}
	// Commit calldata carries the real commitGame selector.
	if len(ms.sent) != 1 || len(ms.sent[0]) != 4+5*32 {
		t.Errorf("unexpected commit calldata len")
	}
}

func TestResolvePVEWinMarksResolvedAndLogs(t *testing.T) {
	m, st, ms, net := manager(t)
	pve := net.Contracts.DealersPVE

	// First receipt = commit, second = resolve (a WIN).
	ms.receipts = []*types.Receipt{
		commitReceipt(pve, 88, 2, 2000),
		playedReceipt(pve, bindings.OutcomeWin, 36, -120, 1),
	}

	seq, err := m.SubmitPVE(context.Background(), 2, bindings.ChoiceDeal, bindings.HustleBuy, 0, 5)
	if err != nil {
		t.Fatalf("commit: %v", err)
	}

	// Resolve via the scheduler.Resolver entrypoint.
	p := store.Pending{Seq: seq, TokenID: 2, Kind: store.KindPVE}
	if err := m.Resolve(context.Background(), p); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if rows, _ := st.ListCommitted(); len(rows) != 0 {
		t.Errorf("still COMMITTED after resolve: %d", len(rows))
	}
	var status, summary string
	var rep int64
	if err := st.DB().QueryRow(`SELECT status FROM pending_actions WHERE seq=88`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != store.StatusResolved {
		t.Errorf("status = %q, want RESOLVED", status)
	}
	if err := st.DB().QueryRow(`SELECT summary, rep_delta FROM action_log WHERE token_id=2`).Scan(&summary, &rep); err != nil {
		t.Fatalf("no action_log row: %v", err)
	}
	if rep != 36 {
		t.Errorf("logged rep_delta = %d, want 36", rep)
	}
}

func pvpCommitReceipt(pvp common.Address, seq, attacker, defender, block uint64) *types.Receipt {
	return &types.Receipt{
		Status:      types.ReceiptStatusSuccessful,
		BlockNumber: new(big.Int).SetUint64(block),
		TxHash:      common.HexToHash("0xpvpcommit"),
		Logs: []*types.Log{{
			Address: pvp,
			Topics: []common.Hash{
				bindings.EventPvpCommitted,
				common.BigToHash(new(big.Int).SetUint64(seq)),
				common.BigToHash(new(big.Int).SetUint64(attacker)),
				common.BigToHash(new(big.Int).SetUint64(defender)),
			},
		}},
	}
}

func pvpResolveReceipt(pvp common.Address, attacker, defender uint64, won bool, cash int64, rep, infamy int16) *types.Receipt {
	data, err := bindings.PackPVPBattleResultData(won, big.NewInt(6), big.NewInt(2), big.NewInt(cash), rep, 0, infamy, 55, 3)
	if err != nil {
		panic(err)
	}
	return &types.Receipt{
		Status:      types.ReceiptStatusSuccessful,
		BlockNumber: big.NewInt(5002),
		TxHash:      common.HexToHash("0xpvpresolve"),
		Logs: []*types.Log{{
			Address: pvp,
			Topics:  []common.Hash{bindings.EventPVPBattleResult, common.BigToHash(new(big.Int).SetUint64(attacker)), common.BigToHash(new(big.Int).SetUint64(defender))},
			Data:    data,
		}},
	}
}

func TestPVPAttackWinAndLoss(t *testing.T) {
	m, st, ms, net := manager(t)
	pvp := net.Contracts.DealersPVP

	// WIN case: commit then resolve-win.
	ms.receipts = []*types.Receipt{
		pvpCommitReceipt(pvp, 70, 5, 99, 5000),
		pvpResolveReceipt(pvp, 5, 99, true, 800, 12, 3),
	}
	seq, err := m.SubmitPVPAttack(context.Background(), 5, 99)
	if err != nil {
		t.Fatalf("SubmitPVPAttack: %v", err)
	}
	if seq != 70 {
		t.Errorf("seq = %d, want 70", seq)
	}
	rows, _ := st.ListCommitted()
	if len(rows) != 1 || rows[0].Kind != store.KindPVP || !strings.Contains(rows[0].MetaJSON, "99") {
		t.Fatalf("pending wrong: %+v", rows)
	}
	if err := m.Resolve(context.Background(), store.Pending{Seq: 70, TokenID: 5, Kind: store.KindPVP, MetaJSON: `{"defender_id":99}`}); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	var summary string
	var rep int64
	st.DB().QueryRow(`SELECT summary, rep_delta FROM action_log WHERE token_id=5 ORDER BY id DESC LIMIT 1`).Scan(&summary, &rep)
	if !strings.Contains(summary, "PVP WIN vs #99") || rep != 12 {
		t.Errorf("win summary/rep wrong: %q rep=%d", summary, rep)
	}

	// LOSS case (reset the mock's receipt cursor).
	ms.receipts = []*types.Receipt{
		pvpCommitReceipt(pvp, 71, 5, 100, 5100),
		pvpResolveReceipt(pvp, 5, 100, false, 0, -10, -1),
	}
	ms.i = 0
	if _, err := m.SubmitPVPAttack(context.Background(), 5, 100); err != nil {
		t.Fatalf("commit loss: %v", err)
	}
	if err := m.Resolve(context.Background(), store.Pending{Seq: 71, TokenID: 5, Kind: store.KindPVP, MetaJSON: `{"defender_id":100}`}); err != nil {
		t.Fatalf("resolve loss: %v", err)
	}
	st.DB().QueryRow(`SELECT summary FROM action_log WHERE token_id=5 ORDER BY id DESC LIMIT 1`).Scan(&summary)
	if !strings.Contains(summary, "PVP LOSS vs #100") {
		t.Errorf("loss summary wrong: %q", summary)
	}
}

func breakoutCommitReceipt(actions common.Address, seq, tokenID, block uint64) *types.Receipt {
	return &types.Receipt{
		Status: types.ReceiptStatusSuccessful, BlockNumber: new(big.Int).SetUint64(block), TxHash: common.HexToHash("0xbkcommit"),
		Logs: []*types.Log{{Address: actions, Topics: []common.Hash{
			bindings.EventBreakoutCommitted,
			common.BigToHash(new(big.Int).SetUint64(seq)),
			common.BigToHash(new(big.Int).SetUint64(tokenID)),
		}}},
	}
}

func breakoutResolveReceipt(actions common.Address, tokenID uint64, success bool) *types.Receipt {
	data, err := bindings.PackBreakoutAttemptedData(success, 1)
	if err != nil {
		panic(err)
	}
	return &types.Receipt{
		Status: types.ReceiptStatusSuccessful, BlockNumber: big.NewInt(7002), TxHash: common.HexToHash("0xbkresolve"),
		Logs: []*types.Log{{Address: actions, Topics: []common.Hash{bindings.EventBreakoutAttempted,
			common.BigToHash(new(big.Int).SetUint64(tokenID))}, Data: data}},
	}
}

func TestBreakoutCommitResolve(t *testing.T) {
	m, st, ms, net := manager(t)
	actions := net.Contracts.DealersActions

	// Success path.
	ms.receipts = []*types.Receipt{
		breakoutCommitReceipt(actions, 33, 7, 7000),
		breakoutResolveReceipt(actions, 7, true),
	}
	seq, err := m.SubmitBreakout(context.Background(), 7)
	if err != nil {
		t.Fatalf("SubmitBreakout: %v", err)
	}
	rows, _ := st.ListCommitted()
	if len(rows) != 1 || rows[0].Kind != store.KindBreakout {
		t.Fatalf("pending wrong: %+v", rows)
	}
	if err := m.Resolve(context.Background(), store.Pending{Seq: seq, TokenID: 7, Kind: store.KindBreakout}); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	var summary string
	st.DB().QueryRow(`SELECT summary FROM action_log WHERE token_id=7 ORDER BY id DESC LIMIT 1`).Scan(&summary)
	if !strings.Contains(summary, "SUCCESS") {
		t.Errorf("success summary = %q", summary)
	}

	// Failed roll.
	ms.receipts = []*types.Receipt{
		breakoutCommitReceipt(actions, 34, 7, 7010),
		breakoutResolveReceipt(actions, 7, false),
	}
	ms.i = 0
	if _, err := m.SubmitBreakout(context.Background(), 7); err != nil {
		t.Fatalf("commit 2: %v", err)
	}
	if err := m.Resolve(context.Background(), store.Pending{Seq: 34, TokenID: 7, Kind: store.KindBreakout}); err != nil {
		t.Fatalf("resolve fail: %v", err)
	}
	st.DB().QueryRow(`SELECT summary FROM action_log WHERE token_id=7 ORDER BY id DESC LIMIT 1`).Scan(&summary)
	if !strings.Contains(summary, "FAILED") {
		t.Errorf("fail summary = %q", summary)
	}
}

func TestSellDrop(t *testing.T) {
	m, st, ms, _ := manager(t)
	ms.receipts = []*types.Receipt{{
		Status: types.ReceiptStatusSuccessful, BlockNumber: big.NewInt(1), TxHash: common.HexToHash("0xdrop"),
	}}
	if err := m.SellDrop(context.Background(), 1, 5, 3); err != nil {
		t.Fatalf("SellDrop: %v", err)
	}
	// Went to DealersActions (single-tx), logged, no pending round.
	if len(ms.sent) != 1 {
		t.Fatalf("expected 1 tx, got %d", len(ms.sent))
	}
	if rows, _ := st.ListCommitted(); len(rows) != 0 {
		t.Errorf("sellDrop should not create a pending round, got %d", len(rows))
	}
	var summary string
	st.DB().QueryRow(`SELECT summary FROM action_log WHERE token_id=1 AND kind='black_market_sell'`).Scan(&summary)
	if !strings.Contains(summary, "black market") {
		t.Errorf("log summary = %q", summary)
	}
}

func TestPvESummaryReadableTrade(t *testing.T) {
	drug := func(id uint64) string {
		if id == 4 {
			return "weed"
		}
		return ""
	}
	res := bindings.GameResult{
		Played: true, Outcome: bindings.OutcomeWin, NewHeat: 1,
		RepChange: big.NewInt(36), CashChange: big.NewInt(-120),
		HustleType: 0, DrugID: big.NewInt(4), DrugAmount: big.NewInt(5),
	}
	got := pveSummary(res, drug)
	if !strings.Contains(got, "bought 5 weed") || !strings.Contains(got, "WIN") {
		t.Errorf("summary = %q, want 'bought 5 weed … WIN'", got)
	}
	// A sell renders "sold".
	res.HustleType = 1
	if got := pveSummary(res, drug); !strings.Contains(got, "sold 5 weed") {
		t.Errorf("sell summary = %q, want 'sold 5 weed'", got)
	}
}

func TestMissionActions(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	net := testnet(t)
	net.Contracts.DealersMissions = common.HexToAddress("0xaf461430D2e2cCd89CFE3Ee335F77a8BF3031F5b")
	ms := &mockSender{agw: common.HexToAddress("0xEd4234a5f233B5E642D47caff292bdc0591D5656")}
	m := NewManager(net, ms, nil, st, log.New(io.Discard, "", 0))

	// Check-in (accept today's missions).
	ms.receipts = []*types.Receipt{{Status: types.ReceiptStatusSuccessful, BlockNumber: big.NewInt(1), TxHash: common.HexToHash("0xci")}}
	if err := m.MissionCheckIn(context.Background(), 7); err != nil {
		t.Fatalf("MissionCheckIn: %v", err)
	}
	// Claim a specific mission.
	ms.receipts = []*types.Receipt{{Status: types.ReceiptStatusSuccessful, BlockNumber: big.NewInt(2), TxHash: common.HexToHash("0xcl")}}
	ms.i = 0
	if err := m.ClaimMission(context.Background(), 7, 3); err != nil {
		t.Fatalf("ClaimMission: %v", err)
	}
	if len(ms.sent) != 2 {
		t.Fatalf("expected 2 single-tx sends, got %d", len(ms.sent))
	}
	var n int
	st.DB().QueryRow(`SELECT COUNT(*) FROM action_log WHERE token_id=7 AND kind IN ('mission_checkin','mission_claim')`).Scan(&n)
	if n != 2 {
		t.Errorf("expected 2 mission log rows, got %d", n)
	}
}

func TestCheckInWithoutReaderJustChecksIn(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	net := testnet(t)
	net.Contracts.DealersBankHeist = common.HexToAddress("0xE219B3E8909Ebc26404080618339b947075FAF2B")
	ms := &mockSender{agw: common.HexToAddress("0xEd4234a5f233B5E642D47caff292bdc0591D5656")}
	m := NewManager(net, ms, nil, st, log.New(io.Discard, "", 0)) // nil reader → no enter probe

	ms.receipts = []*types.Receipt{{Status: types.ReceiptStatusSuccessful, BlockNumber: big.NewInt(1), TxHash: common.HexToHash("0xci")}}
	if err := m.CheckIn(context.Background(), 24); err != nil {
		t.Fatalf("CheckIn: %v", err)
	}
	// Exactly one tx (the checkIn), no enter, and it is the checkIn calldata.
	if len(ms.sent) != 1 {
		t.Fatalf("expected 1 tx, got %d", len(ms.sent))
	}
	wantCall, _ := bindings.PackCheckIn(24)
	if !bytes.Equal(ms.sent[0], wantCall) {
		t.Errorf("first tx is not checkIn(24): %x", ms.sent[0])
	}
}

func TestMissionActionsGuardWhenUndeployed(t *testing.T) {
	m, _, _, _ := manager(t) // testnet: DealersMissions is zero
	if err := m.MissionCheckIn(context.Background(), 1); err == nil {
		t.Error("MissionCheckIn should error when the contract isn't deployed")
	}
	if err := m.ClaimMission(context.Background(), 1, 2); err == nil {
		t.Error("ClaimMission should error when the contract isn't deployed")
	}
}

func TestPayBailRequiresReader(t *testing.T) {
	m, _, _, _ := manager(t) // nil reader
	if err := m.PayBail(context.Background(), 1); err == nil {
		t.Error("PayBail should error without a reader (fee lookup)")
	}
}

func TestExecuteDispatch(t *testing.T) {
	m, st, ms, net := manager(t)

	// ActionPVE → SubmitPVE (commit receipt).
	ms.receipts = []*types.Receipt{commitReceipt(net.Contracts.DealersPVE, 5, 1, 100)}
	if _, err := m.Execute(context.Background(), 1, Action{Kind: ActionPVE, Hustle: bindings.HustleBuy, DrugID: 4, Amount: 1}); err != nil {
		t.Fatalf("Execute PVE: %v", err)
	}
	if rows, _ := st.ListCommitted(); len(rows) != 1 || rows[0].Kind != store.KindPVE {
		t.Fatalf("PVE not persisted: %+v", rows)
	}

	// ActionClearHeat → SubmitWantedPoster.
	ms.receipts = []*types.Receipt{wantedCommitReceipt(net.Contracts.DealersActions, 6, 1, 110)}
	ms.i = 0
	if _, err := m.Execute(context.Background(), 1, Action{Kind: ActionClearHeat}); err != nil {
		t.Fatalf("Execute ClearHeat: %v", err)
	}
	var kind string
	st.DB().QueryRow(`SELECT kind FROM pending_actions WHERE seq=6`).Scan(&kind)
	if kind != store.KindWantedPoster {
		t.Errorf("clear-heat kind = %q, want wanted_poster", kind)
	}

	// ActionPVP → SubmitPVPAttack (commit receipt, defender carried in meta).
	ms.receipts = []*types.Receipt{pvpCommitReceipt(net.Contracts.DealersPVP, 8, 1, 42, 120)}
	ms.i = 0
	if _, err := m.Execute(context.Background(), 1, Action{Kind: ActionPVP, DefenderID: 42}); err != nil {
		t.Fatalf("Execute PVP: %v", err)
	}
	var pvpMeta string
	st.DB().QueryRow(`SELECT meta_json FROM pending_actions WHERE seq=8`).Scan(&pvpMeta)
	if !strings.Contains(pvpMeta, "42") {
		t.Errorf("PVP defender not in meta: %q", pvpMeta)
	}

	// ActionSellDrop → SellDrop (single-tx black-market sale).
	ms.receipts = []*types.Receipt{{Status: types.ReceiptStatusSuccessful, BlockNumber: big.NewInt(1), TxHash: common.HexToHash("0xd")}}
	ms.i = 0
	if _, err := m.Execute(context.Background(), 1, Action{Kind: ActionSellDrop, DrugID: 9, Amount: 2}); err != nil {
		t.Fatalf("Execute SellDrop: %v", err)
	}
	var sellKind string
	st.DB().QueryRow(`SELECT kind FROM action_log WHERE token_id=1 AND kind='black_market_sell' ORDER BY id DESC LIMIT 1`).Scan(&sellKind)
	if sellKind != "black_market_sell" {
		t.Errorf("sellDrop not logged, got kind %q", sellKind)
	}

	// ActionTravel → Travel (needs a reader for the fee; nil reader here proves
	// the dispatch reached Travel by surfacing its reader-required error).
	if _, err := m.Execute(context.Background(), 1, Action{Kind: ActionTravel, DestArea: 7}); err == nil {
		t.Error("Execute Travel without a reader should error")
	}

	// ActionPayBail → PayBail (also reader-gated; the error proves it routed there).
	if _, err := m.Execute(context.Background(), 1, Action{Kind: ActionPayBail}); err == nil {
		t.Error("Execute PayBail without a reader should error")
	}

	// ActionBreakout → SubmitBreakout (commit receipt, pending breakout round).
	ms.receipts = []*types.Receipt{breakoutCommitReceipt(net.Contracts.DealersActions, 12, 1, 130)}
	ms.i = 0
	if _, err := m.Execute(context.Background(), 1, Action{Kind: ActionBreakout}); err != nil {
		t.Fatalf("Execute Breakout: %v", err)
	}
	var bkKind string
	st.DB().QueryRow(`SELECT kind FROM pending_actions WHERE seq=12`).Scan(&bkKind)
	if bkKind != store.KindBreakout {
		t.Errorf("breakout kind = %q, want %s", bkKind, store.KindBreakout)
	}

	// Unknown kind → error.
	if _, err := m.Execute(context.Background(), 1, Action{Kind: ActionNone}); err == nil {
		t.Error("expected error for unknown action kind")
	}
}

func heistStartReceipt(h common.Address, heistID, tokenID, block uint64) *types.Receipt {
	return &types.Receipt{
		Status: types.ReceiptStatusSuccessful, BlockNumber: new(big.Int).SetUint64(block), TxHash: common.HexToHash("0xhstart"),
		Logs: []*types.Log{{Address: h, Topics: []common.Hash{
			bindings.EventHeistStarted,
			common.BigToHash(new(big.Int).SetUint64(heistID)),
			common.BigToHash(new(big.Int).SetUint64(tokenID)),
			{},
		}}},
	}
}

func stageCommitReceipt(h common.Address, heistID, seq, tokenID, block uint64) *types.Receipt {
	return &types.Receipt{
		Status: types.ReceiptStatusSuccessful, BlockNumber: new(big.Int).SetUint64(block), TxHash: common.HexToHash("0xstagecommit"),
		Logs: []*types.Log{{Address: h, Topics: []common.Hash{
			bindings.EventStageCommitted,
			common.BigToHash(new(big.Int).SetUint64(heistID)),
			common.BigToHash(new(big.Int).SetUint64(seq)), // seq is topic[2]
			common.BigToHash(new(big.Int).SetUint64(tokenID)),
		}}},
	}
}

func stageWonReceipt(h common.Address, heistID, tokenID uint64, stage uint8, pot int64) *types.Receipt {
	data, err := bindings.PackStageWonData(stage, big.NewInt(pot))
	if err != nil {
		panic(err)
	}
	return &types.Receipt{
		Status: types.ReceiptStatusSuccessful, BlockNumber: big.NewInt(6002), TxHash: common.HexToHash("0xstagewon"),
		Logs: []*types.Log{{Address: h, Topics: []common.Hash{bindings.EventStageWon,
			common.BigToHash(new(big.Int).SetUint64(heistID)), common.BigToHash(new(big.Int).SetUint64(tokenID))}, Data: data}},
	}
}

func heistBustReceipt(h common.Address, heistID, tokenID uint64, stage uint8) *types.Receipt {
	data, err := bindings.PackHeistBustedData(stage)
	if err != nil {
		panic(err)
	}
	return &types.Receipt{
		Status: types.ReceiptStatusSuccessful, BlockNumber: big.NewInt(6003), TxHash: common.HexToHash("0xbust"),
		Logs: []*types.Log{{Address: h, Topics: []common.Hash{bindings.EventHeistBusted,
			common.BigToHash(new(big.Int).SetUint64(heistID)), common.BigToHash(new(big.Int).SetUint64(tokenID))}, Data: data}},
	}
}

func TestHeistStartCommitResolve(t *testing.T) {
	m, st, ms, net := manager(t)
	h := net.Contracts.DealersHeists

	// Start → commit stage → resolve CLEAN.
	ms.receipts = []*types.Receipt{
		heistStartReceipt(h, 500, 26, 6000),
		stageCommitReceipt(h, 500, 88, 26, 6001),
		stageWonReceipt(h, 500, 26, 1, 900),
	}
	heistID, err := m.StartHeist(context.Background(), 26, bindings.FamilyCash, 0, false)
	if err != nil {
		t.Fatalf("StartHeist: %v", err)
	}
	if heistID != 500 {
		t.Errorf("heistID = %d, want 500", heistID)
	}

	seq, err := m.CommitStage(context.Background(), 26, heistID)
	if err != nil {
		t.Fatalf("CommitStage: %v", err)
	}
	if seq != 88 {
		t.Errorf("seq = %d, want 88 (topic[2])", seq)
	}
	rows, _ := st.ListCommitted()
	if len(rows) != 1 || rows[0].Kind != store.KindHeistStage || !strings.Contains(rows[0].MetaJSON, "500") {
		t.Fatalf("pending wrong: %+v", rows)
	}

	if err := m.Resolve(context.Background(), store.Pending{Seq: 88, TokenID: 26, Kind: store.KindHeistStage, MetaJSON: `{"heist_id":500}`}); err != nil {
		t.Fatalf("Resolve clean: %v", err)
	}
	var summary string
	st.DB().QueryRow(`SELECT summary FROM action_log WHERE token_id=26 AND kind='heist_stage' ORDER BY id DESC LIMIT 1`).Scan(&summary)
	if !strings.Contains(summary, "stage 1 CLEAN") {
		t.Errorf("clean summary wrong: %q", summary)
	}

	// A later stage that busts.
	ms.receipts = []*types.Receipt{
		stageCommitReceipt(h, 500, 89, 26, 6010),
		heistBustReceipt(h, 500, 26, 3),
	}
	ms.i = 0
	if _, err := m.CommitStage(context.Background(), 26, heistID); err != nil {
		t.Fatalf("CommitStage 2: %v", err)
	}
	if err := m.Resolve(context.Background(), store.Pending{Seq: 89, TokenID: 26, Kind: store.KindHeistStage, MetaJSON: `{"heist_id":500}`}); err != nil {
		t.Fatalf("Resolve bust: %v", err)
	}
	st.DB().QueryRow(`SELECT summary FROM action_log WHERE token_id=26 AND kind='heist_stage' ORDER BY id DESC LIMIT 1`).Scan(&summary)
	if !strings.Contains(summary, "BUST at stage 3") {
		t.Errorf("bust summary wrong: %q", summary)
	}
}

func wantedCommitReceipt(actions common.Address, seq, tokenID, block uint64) *types.Receipt {
	return &types.Receipt{
		Status:      types.ReceiptStatusSuccessful,
		BlockNumber: new(big.Int).SetUint64(block),
		TxHash:      common.HexToHash("0xwpcommit"),
		Logs: []*types.Log{{
			Address: actions,
			Topics: []common.Hash{
				bindings.EventWantedPosterCommitted,
				common.BigToHash(new(big.Int).SetUint64(seq)),
				common.BigToHash(new(big.Int).SetUint64(tokenID)),
			},
		}},
	}
}

func wantedResolveReceipt(actions common.Address, tokenID uint64, success bool) *types.Receipt {
	data, err := bindings.PackWantedPosterRemovedData(success)
	if err != nil {
		panic(err)
	}
	return &types.Receipt{
		Status:      types.ReceiptStatusSuccessful,
		BlockNumber: big.NewInt(3002),
		TxHash:      common.HexToHash("0xwpresolve"),
		Logs: []*types.Log{{
			Address: actions,
			Topics:  []common.Hash{bindings.EventWantedPosterRemoved, common.BigToHash(new(big.Int).SetUint64(tokenID))},
			Data:    data,
		}},
	}
}

func TestWantedPosterCommitAndResolve(t *testing.T) {
	m, st, ms, net := manager(t)
	actions := net.Contracts.DealersActions

	// nil reader in test → preflight skipped; commit then resolve (success).
	ms.receipts = []*types.Receipt{
		wantedCommitReceipt(actions, 55, 3, 3000),
		wantedResolveReceipt(actions, 3, true),
	}

	seq, err := m.SubmitWantedPoster(context.Background(), 3)
	if err != nil {
		t.Fatalf("SubmitWantedPoster: %v", err)
	}
	if seq != 55 {
		t.Errorf("seq = %d, want 55", seq)
	}
	rows, _ := st.ListCommitted()
	if len(rows) != 1 || rows[0].Kind != store.KindWantedPoster {
		t.Fatalf("pending wrong: %+v", rows)
	}

	if err := m.Resolve(context.Background(), store.Pending{Seq: 55, TokenID: 3, Kind: store.KindWantedPoster}); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	var summary string
	if err := st.DB().QueryRow(`SELECT summary FROM action_log WHERE token_id=3 ORDER BY id DESC LIMIT 1`).Scan(&summary); err != nil {
		t.Fatal(err)
	}
	if summary != "wanted poster: heat cleared" {
		t.Errorf("success summary = %q", summary)
	}
}

func TestWantedPosterFailedRoll(t *testing.T) {
	m, st, ms, net := manager(t)
	actions := net.Contracts.DealersActions
	ms.receipts = []*types.Receipt{
		wantedCommitReceipt(actions, 56, 4, 4000),
		wantedResolveReceipt(actions, 4, false), // roll missed
	}
	if _, err := m.SubmitWantedPoster(context.Background(), 4); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := m.Resolve(context.Background(), store.Pending{Seq: 56, TokenID: 4, Kind: store.KindWantedPoster}); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	var summary string
	st.DB().QueryRow(`SELECT summary FROM action_log WHERE token_id=4 ORDER BY id DESC LIMIT 1`).Scan(&summary)
	if !strings.Contains(summary, "failed roll") {
		t.Errorf("expected failed-roll summary, got %q", summary)
	}
}

// playedReceipt builds a resolve receipt carrying a GamePlayed(outcome) log.
func playedReceipt(pve common.Address, outcome bindings.Outcome, rep, cash int64, heat uint8) *types.Receipt {
	data, err := bindings.PackGamePlayedData(
		0, 0, outcome, 0,
		big.NewInt(0), big.NewInt(5),
		big.NewInt(cash), big.NewInt(rep), big.NewInt(5),
		heat, big.NewInt(120), big.NewInt(0),
	)
	if err != nil {
		panic(err)
	}
	return &types.Receipt{
		Status:      types.ReceiptStatusSuccessful,
		BlockNumber: big.NewInt(2002),
		TxHash:      common.HexToHash("0xresolve"),
		Logs: []*types.Log{{
			Address: pve,
			Topics:  []common.Hash{bindings.EventGamePlayed, common.BigToHash(big.NewInt(2)), {}},
			Data:    data,
		}},
	}
}
