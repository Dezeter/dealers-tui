package dealer

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/big"

	"dealers/internal/chain/bindings"
	"dealers/internal/config"
	"dealers/internal/store"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// Commit-reveal window constants (DealersRandomness, CHAIN_REFERENCE §3).
const (
	RevealOffset = 2
	ExpiryWindow = 200
)

// TxSender is the write surface the manager needs — satisfied by *chain.Sender
// and mockable in tests.
type TxSender interface {
	SendAndWait(ctx context.Context, to common.Address, data []byte, value *big.Int) (*types.Receipt, error)
	AGW() common.Address
}

// Manager orchestrates commit-reveal for the fleet. It is the scheduler's
// Resolver. Phase 1 wires PVE; other kinds return an explicit error until their
// phases land. The optional Strategy (ADR-5) is not consulted yet — v1 is
// manual only.
type Manager struct {
	net       config.Network
	sender    TxSender
	reader    *bindings.Reader // for fee reads + pre-action checks; may be nil in tests
	store     *store.Store
	logger    *log.Logger
	drugNames map[uint64]string // id → name for readable logs (optional)
}

// SetDrugNames supplies the drug id→name map used to render trade log lines
// ("bought 5 weed" instead of "drug#4"). Optional; safe to leave unset.
func (m *Manager) SetDrugNames(n map[uint64]string) { m.drugNames = n }

// drugName resolves a drug id to its name, falling back to "drug#N".
func (m *Manager) drugName(id uint64) string {
	if m.drugNames != nil {
		if n := m.drugNames[id]; n != "" {
			return n
		}
	}
	return fmt.Sprintf("drug#%d", id)
}

// NewManager builds a manager. reader enables fee reads + pre-action checks
// (nil in unit tests). logger may be nil.
func NewManager(net config.Network, sender TxSender, reader *bindings.Reader, st *store.Store, logger *log.Logger) *Manager {
	if logger == nil {
		logger = log.Default()
	}
	return &Manager{net: net, sender: sender, reader: reader, store: st, logger: logger}
}

// Execute performs a strategy-emitted Action (the autopilot bridge, ADR-5).
// Commit-reveal actions return their seq; the scheduler resolves them.
func (m *Manager) Execute(ctx context.Context, tokenID uint64, a Action) (uint64, error) {
	switch a.Kind {
	case ActionPVE:
		return m.SubmitPVE(ctx, tokenID, bindings.ChoiceDeal, a.Hustle, a.DrugID, a.Amount)
	case ActionClearHeat:
		return m.SubmitWantedPoster(ctx, tokenID)
	case ActionPVP:
		return m.SubmitPVPAttack(ctx, tokenID, a.DefenderID)
	case ActionTravel:
		return 0, m.Travel(ctx, tokenID, a.DestArea)
	case ActionSellDrop:
		return 0, m.SellDrop(ctx, tokenID, a.DrugID, a.Amount)
	case ActionBreakout:
		return m.SubmitBreakout(ctx, tokenID)
	case ActionPayBail:
		return 0, m.PayBail(ctx, tokenID)
	case ActionMissionCheckIn:
		return 0, m.MissionCheckIn(ctx, tokenID)
	case ActionMissionClaim:
		return 0, m.ClaimMission(ctx, tokenID, a.TemplateID)
	case ActionHeistCheckIn:
		return 0, m.CheckIn(ctx, tokenID)
	case ActionStartHeist:
		return m.StartHeist(ctx, tokenID, a.HeistFamily, a.HeistDifficulty, false)
	case ActionHeistStage:
		return m.CommitStage(ctx, tokenID, a.HeistID)
	case ActionHeistCashOut:
		return 0, m.CashOut(ctx, tokenID, a.HeistID)
	default:
		return 0, fmt.Errorf("execute: unsupported action kind %d", a.Kind)
	}
}

// pveMeta is persisted in pending_actions.meta_json for a PVE round.
type pveMeta struct {
	Choice bindings.Choice     `json:"choice"`
	Hustle bindings.HustleType `json:"hustle"`
	DrugID uint64              `json:"drug_id"`
	Amount uint64              `json:"amount"`
}

// SubmitPVE commits a PVE hustle (buy/sell) for a dealer and persists the
// pending round immediately (FR7). The scheduler resolves it once the reveal
// block is reached. Returns the commit seq.
func (m *Manager) SubmitPVE(ctx context.Context, tokenID uint64, choice bindings.Choice, hustle bindings.HustleType, drugID, amount uint64) (uint64, error) {
	// Preflight: give a clear reason instead of a raw on-chain revert when the
	// dealer can't act (no attempts, jailed, safe house).
	if m.reader != nil {
		if ok, reason, err := m.reader.CanPlay(ctx, tokenID); err == nil && !ok {
			return 0, fmt.Errorf("cannot hustle: %s", bindings.CanPlayReason(reason))
		}
	}
	data, err := bindings.PackCommitGame(tokenID, choice, hustle, drugID, amount)
	if err != nil {
		return 0, fmt.Errorf("pack commitGame: %w", err)
	}
	receipt, err := m.sender.SendAndWait(ctx, m.net.Contracts.DealersPVE, data, nil)
	if err != nil {
		return 0, fmt.Errorf("commit pve: %w", err)
	}
	seq, err := bindings.ParseCommitSeq(receipt.Logs, m.net.Contracts.DealersPVE)
	if err != nil {
		return 0, fmt.Errorf("parse commit seq: %w", err)
	}
	commitBlock := receipt.BlockNumber.Uint64()

	meta, _ := json.Marshal(pveMeta{Choice: choice, Hustle: hustle, DrugID: drugID, Amount: amount})
	if err := m.store.UpsertDealer(store.Dealer{
		TokenID: tokenID, WalletAddress: m.sender.AGW().Hex(), Network: m.net.Name,
	}); err != nil {
		return 0, err
	}
	p := store.Pending{
		Seq:          seq,
		TokenID:      tokenID,
		Kind:         store.KindPVE,
		CommitBlock:  commitBlock,
		RevealBlock:  commitBlock + RevealOffset,
		ExpiryBlock:  commitBlock + ExpiryWindow,
		TxHashCommit: receipt.TxHash.Hex(),
		MetaJSON:     string(meta),
	}
	if err := m.store.InsertPending(p); err != nil {
		return 0, fmt.Errorf("persist pending seq=%d (commit tx %s LANDED but not tracked!): %w",
			seq, receipt.TxHash.Hex(), err)
	}
	m.logger.Printf("PVE commit token=%d seq=%d block=%d reveal=%d", tokenID, seq, commitBlock, p.RevealBlock)
	return seq, nil
}

// Resolve implements scheduler.Resolver. Dispatches by kind.
func (m *Manager) Resolve(ctx context.Context, p store.Pending) error {
	switch p.Kind {
	case store.KindPVE:
		return m.resolvePVE(ctx, p)
	case store.KindPVP:
		return m.resolvePVP(ctx, p)
	case store.KindHeistStage:
		return m.resolveHeistStage(ctx, p)
	case store.KindWantedPoster:
		return m.resolveWantedPoster(ctx, p)
	case store.KindBreakout:
		return m.resolveBreakout(ctx, p)
	default:
		return fmt.Errorf("resolve: unsupported kind %q (seq=%d)", p.Kind, p.Seq)
	}
}

func (m *Manager) resolvePVE(ctx context.Context, p store.Pending) error {
	data, err := bindings.PackResolveGame(p.Seq)
	if err != nil {
		return fmt.Errorf("pack resolveGame: %w", err)
	}
	receipt, err := m.sender.SendAndWait(ctx, m.net.Contracts.DealersPVE, data, nil)
	if err != nil {
		// Leaves the row COMMITTED; the scheduler retries next block (or marks it
		// EXPIRED once the window passes).
		return fmt.Errorf("send resolveGame seq=%d: %w", p.Seq, err)
	}
	res, err := bindings.ParseGameResult(receipt.Logs, m.net.Contracts.DealersPVE)
	if err != nil {
		return fmt.Errorf("parse resolve seq=%d: %w", p.Seq, err)
	}

	if err := m.store.MarkResolved(p.Seq, receipt.TxHash.Hex()); err != nil {
		return fmt.Errorf("mark resolved seq=%d: %w", p.Seq, err)
	}
	summary := pveSummary(res, m.drugName)
	m.store.AppendLog(pveLogEntry(p.TokenID, res, receipt.TxHash.Hex(), summary))
	m.logger.Printf("PVE resolve token=%d seq=%d → %s", p.TokenID, p.Seq, summary)
	return nil
}

// pveSummary renders a one-line outcome for logs, e.g.
// "bought 5 weed — WIN (rep +36, cash -120, heat→1)". drug resolves a drug id to
// its name (may be nil).
func pveSummary(res bindings.GameResult, drug func(uint64) string) string {
	switch {
	case res.Arrested:
		return "ARRESTED"
	case res.Expired:
		return "EXPIRED (reveal window missed)"
	case res.Played:
		trade := ""
		if res.DrugAmount != nil && res.DrugID != nil {
			verb := "traded"
			switch res.HustleType {
			case 0:
				verb = "bought"
			case 1:
				verb = "sold"
			}
			name := fmt.Sprintf("drug#%d", res.DrugID.Uint64())
			if drug != nil {
				name = drug(res.DrugID.Uint64())
			}
			trade = fmt.Sprintf("%s %s %s — ", verb, res.DrugAmount, name)
		}
		return fmt.Sprintf("%s%s (rep %s, cash %s, heat→%d)",
			trade, res.Outcome, bigSigned(res.RepChange), bigSigned(res.CashChange), res.NewHeat)
	default:
		return "unknown"
	}
}

func pveLogEntry(tokenID uint64, res bindings.GameResult, tx, summary string) store.LogEntry {
	e := store.LogEntry{TokenID: tokenID, Kind: store.KindPVE, Summary: summary, TxHash: tx}
	if res.Played {
		if res.CashChange != nil {
			v := res.CashChange.Int64()
			e.CashDelta = &v
		}
		if res.RepChange != nil {
			v := res.RepChange.Int64()
			e.RepDelta = &v
		}
		h := int64(res.NewHeat)
		e.HeatAfter = &h
	}
	return e
}

func bigSigned(v *big.Int) string {
	if v == nil {
		return "0"
	}
	if v.Sign() > 0 {
		return "+" + v.String()
	}
	return v.String()
}

// --- Heists (multi-stage commit-reveal + single-tx controls) ---

// HeistETHAddon is the ETH add-on charged when a heist opts into the jackpot
// (DealersHeists.ethAddOn default, CHAIN_REFERENCE §8.3). Non-jackpot heists
// send 0 (the $CASH stake is debited in-game, not ETH).
var HeistETHAddon = big.NewInt(1_000_000_000_000_000) // 0.001 ETH

// maxSeasonEntryFeeWei caps the ETH a bank-heist season enter() may auto-pay. The
// live fee is read from the season (SeasonEntryFee); this is a defensive ceiling so
// a misread or a future season with a wild fee can't silently drain ETH. The known
// V2 fee is 0.001 ETH — 0.01 leaves headroom while blocking anything unreasonable.
var maxSeasonEntryFeeWei = big.NewInt(10_000_000_000_000_000) // 0.01 ETH

// heistMeta is persisted in pending_actions.meta_json for a heist stage round.
type heistMeta struct {
	HeistID uint64 `json:"heist_id"`
}

// StartHeist opens a heist run (pays the difficulty's $CASH stake + 1 attempt).
// ethJackpot adds the ETH add-on to make the run jackpot-eligible. Returns the
// new heist id. The dealer must not already have an active heist.
func (m *Manager) StartHeist(ctx context.Context, tokenID uint64, family bindings.HeistFamily, difficulty uint8, ethJackpot bool) (uint64, error) {
	if m.reader != nil {
		if id, err := m.reader.ActiveHeist(ctx, tokenID); err == nil && id != 0 {
			return 0, fmt.Errorf("dealer already has an active heist (#%d)", id)
		}
	}
	value := big.NewInt(0)
	if ethJackpot {
		value = new(big.Int).Set(HeistETHAddon)
	}
	data, err := bindings.PackStartHeist(tokenID, family, difficulty, ethJackpot)
	if err != nil {
		return 0, fmt.Errorf("pack startHeist: %w", err)
	}
	receipt, err := m.sender.SendAndWait(ctx, m.net.Contracts.DealersHeists, data, value)
	if err != nil {
		return 0, fmt.Errorf("start heist: %w", err)
	}
	heistID, err := bindings.ParseHeistID(receipt.Logs, m.net.Contracts.DealersHeists)
	if err != nil {
		return 0, fmt.Errorf("parse heist id: %w", err)
	}
	if m.store != nil {
		m.store.UpsertDealer(store.Dealer{TokenID: tokenID, WalletAddress: m.sender.AGW().Hex(), Network: m.net.Name})
		m.store.AppendLog(store.LogEntry{TokenID: tokenID, Kind: "heist_start",
			Summary: fmt.Sprintf("started %s heist #%d (difficulty %d%s)", family, heistID, difficulty, jackpotTag(ethJackpot)),
			TxHash:  receipt.TxHash.Hex()})
	}
	m.logger.Printf("heist start token=%d heist=%d family=%s difficulty=%d jackpot=%v", tokenID, heistID, family, difficulty, ethJackpot)
	return heistID, nil
}

// CommitStage commits the next heist stage (from PRE_STAGE starts stage 1; from
// REVEALED_WIN this pushes deeper). The scheduler resolves it at the reveal
// block. tokenID owns the pending row; heistID is carried in meta.
func (m *Manager) CommitStage(ctx context.Context, tokenID, heistID uint64) (uint64, error) {
	data, err := bindings.PackCommitStage(heistID)
	if err != nil {
		return 0, fmt.Errorf("pack commitStage: %w", err)
	}
	receipt, err := m.sender.SendAndWait(ctx, m.net.Contracts.DealersHeists, data, nil)
	if err != nil {
		return 0, fmt.Errorf("commit stage: %w", err)
	}
	seq, err := bindings.ParseStageSeq(receipt.Logs, m.net.Contracts.DealersHeists)
	if err != nil {
		return 0, fmt.Errorf("parse stage seq: %w", err)
	}
	commitBlock := receipt.BlockNumber.Uint64()
	meta, _ := json.Marshal(heistMeta{HeistID: heistID})
	if err := m.store.UpsertDealer(store.Dealer{TokenID: tokenID, WalletAddress: m.sender.AGW().Hex(), Network: m.net.Name}); err != nil {
		return 0, err
	}
	p := store.Pending{
		Seq: seq, TokenID: tokenID, Kind: store.KindHeistStage,
		CommitBlock: commitBlock, RevealBlock: commitBlock + RevealOffset, ExpiryBlock: commitBlock + ExpiryWindow,
		TxHashCommit: receipt.TxHash.Hex(), MetaJSON: string(meta),
	}
	if err := m.store.InsertPending(p); err != nil {
		return 0, fmt.Errorf("persist heist-stage seq=%d (commit tx %s LANDED): %w", seq, receipt.TxHash.Hex(), err)
	}
	m.logger.Printf("heist stage commit token=%d heist=%d seq=%d reveal=%d", tokenID, heistID, seq, p.RevealBlock)
	return seq, nil
}

func (m *Manager) resolveHeistStage(ctx context.Context, p store.Pending) error {
	data, err := bindings.PackResolveStage(p.Seq)
	if err != nil {
		return fmt.Errorf("pack resolveStage: %w", err)
	}
	receipt, err := m.sender.SendAndWait(ctx, m.net.Contracts.DealersHeists, data, nil)
	if err != nil {
		return fmt.Errorf("send resolveStage seq=%d: %w", p.Seq, err)
	}
	res, err := bindings.ParseStageResult(receipt.Logs, m.net.Contracts.DealersHeists)
	if err != nil {
		return fmt.Errorf("parse heist stage seq=%d: %w", p.Seq, err)
	}
	if err := m.store.MarkResolved(p.Seq, receipt.TxHash.Hex()); err != nil {
		return fmt.Errorf("mark resolved seq=%d: %w", p.Seq, err)
	}
	m.store.AppendLog(store.LogEntry{TokenID: p.TokenID, Kind: store.KindHeistStage,
		Summary: heistStageSummary(res), TxHash: receipt.TxHash.Hex()})
	m.logger.Printf("heist stage resolve token=%d seq=%d → %s", p.TokenID, p.Seq, heistStageSummary(res))
	return nil
}

// CashOut banks a REVEALED_WIN run's current pot (stage ≥ minCashStage).
func (m *Manager) CashOut(ctx context.Context, tokenID, heistID uint64) error {
	data, err := bindings.PackCashOut(heistID)
	if err != nil {
		return fmt.Errorf("pack cashOut: %w", err)
	}
	return m.sendSingleTx(ctx, m.net.Contracts.DealersHeists, data, nil, tokenID, store.KindHeistStage, fmt.Sprintf("heist #%d cashed out", heistID))
}

// AbandonHeist refunds a PRE_STAGE run's $CASH stake (ETH add-on + attempt forfeit).
func (m *Manager) AbandonHeist(ctx context.Context, tokenID, heistID uint64) error {
	data, err := bindings.PackAbandonHeist(heistID)
	if err != nil {
		return fmt.Errorf("pack abandonHeist: %w", err)
	}
	return m.sendSingleTx(ctx, m.net.Contracts.DealersHeists, data, nil, tokenID, store.KindHeistStage, fmt.Sprintf("heist #%d abandoned", heistID))
}

// ClaimJackpot claims a dealer's owed jackpot ETH.
func (m *Manager) ClaimJackpot(ctx context.Context, tokenID uint64) error {
	data, err := bindings.PackClaimJackpot(tokenID)
	if err != nil {
		return fmt.Errorf("pack claimJackpot: %w", err)
	}
	return m.sendSingleTx(ctx, m.net.Contracts.DealersHeists, data, nil, tokenID, "jackpot", "jackpot claimed")
}

func heistStageSummary(res bindings.StageResult) string {
	switch {
	case res.Busted:
		s := fmt.Sprintf("heist BUST at stage %d — stake lost", res.Stage)
		if res.Arrested {
			s += " + ARRESTED"
		}
		return s
	case res.Setback:
		return fmt.Sprintf("heist SETBACK at stage %d — partial pot %s paid", res.Stage, bigStrLocal(res.Pot))
	case res.CashedOut:
		return fmt.Sprintf("heist CLEARED stage %d — auto cash-out, pot %s", res.Stage, bigStrLocal(res.Pot))
	case res.Clean:
		return fmt.Sprintf("heist stage %d CLEAN — pot now %s (push or cash out)", res.Stage, bigStrLocal(res.Pot))
	default:
		return "heist stage resolved"
	}
}

func jackpotTag(on bool) string {
	if on {
		return ", +ETH jackpot"
	}
	return ""
}

// --- PVP (commit-reveal) ---

// pvpMeta is persisted in pending_actions.meta_json for a PVP round.
type pvpMeta struct {
	DefenderID uint64 `json:"defender_id"`
}

// SubmitPVPAttack commits an attack on defenderID. PVP unlocks at REP ≥ 200 and
// requires the target in the same area; the manager preflights canAttack for a
// clear reason. The scheduler resolves the round at the reveal block.
func (m *Manager) SubmitPVPAttack(ctx context.Context, attackerID, defenderID uint64) (uint64, error) {
	if m.reader != nil {
		if ok, reason, err := m.reader.CanAttack(ctx, attackerID, defenderID); err == nil && !ok {
			return 0, fmt.Errorf("cannot attack: %s", bindings.CanAttackReason(reason))
		}
	}
	data, err := bindings.PackCommitAttack(attackerID, defenderID)
	if err != nil {
		return 0, fmt.Errorf("pack commitAttack: %w", err)
	}
	receipt, err := m.sender.SendAndWait(ctx, m.net.Contracts.DealersPVP, data, nil)
	if err != nil {
		return 0, fmt.Errorf("commit attack: %w", err)
	}
	seq, err := bindings.ParsePvpSeq(receipt.Logs, m.net.Contracts.DealersPVP)
	if err != nil {
		return 0, fmt.Errorf("parse pvp seq: %w", err)
	}
	commitBlock := receipt.BlockNumber.Uint64()
	meta, _ := json.Marshal(pvpMeta{DefenderID: defenderID})
	if err := m.store.UpsertDealer(store.Dealer{TokenID: attackerID, WalletAddress: m.sender.AGW().Hex(), Network: m.net.Name}); err != nil {
		return 0, err
	}
	p := store.Pending{
		Seq: seq, TokenID: attackerID, Kind: store.KindPVP,
		CommitBlock: commitBlock, RevealBlock: commitBlock + RevealOffset, ExpiryBlock: commitBlock + ExpiryWindow,
		TxHashCommit: receipt.TxHash.Hex(), MetaJSON: string(meta),
	}
	if err := m.store.InsertPending(p); err != nil {
		return 0, fmt.Errorf("persist pvp seq=%d (commit tx %s LANDED): %w", seq, receipt.TxHash.Hex(), err)
	}
	m.logger.Printf("PVP commit attacker=%d defender=%d seq=%d reveal=%d", attackerID, defenderID, seq, p.RevealBlock)
	return seq, nil
}

func (m *Manager) resolvePVP(ctx context.Context, p store.Pending) error {
	data, err := bindings.PackResolveAttack(p.Seq)
	if err != nil {
		return fmt.Errorf("pack resolveAttack: %w", err)
	}
	receipt, err := m.sender.SendAndWait(ctx, m.net.Contracts.DealersPVP, data, nil)
	if err != nil {
		return fmt.Errorf("send resolveAttack seq=%d: %w", p.Seq, err)
	}
	res, err := bindings.ParsePVPResult(receipt.Logs, m.net.Contracts.DealersPVP)
	if err != nil {
		return fmt.Errorf("parse pvp seq=%d: %w", p.Seq, err)
	}
	if err := m.store.MarkResolved(p.Seq, receipt.TxHash.Hex()); err != nil {
		return fmt.Errorf("mark resolved seq=%d: %w", p.Seq, err)
	}

	var meta pvpMeta
	_ = json.Unmarshal([]byte(p.MetaJSON), &meta)
	summary := pvpSummary(res, meta.DefenderID)
	e := store.LogEntry{TokenID: p.TokenID, Kind: store.KindPVP, Summary: summary, TxHash: receipt.TxHash.Hex()}
	if res.Fought {
		rep := int64(res.RepChange)
		heat := int64(res.NewHeat)
		e.RepDelta = &rep
		e.HeatAfter = &heat
		if res.Won && res.CashStolen != nil {
			cash := res.CashStolen.Int64()
			e.CashDelta = &cash
		}
	}
	m.store.AppendLog(e)
	m.logger.Printf("PVP resolve attacker=%d seq=%d → %s", p.TokenID, p.Seq, summary)
	return nil
}

func pvpSummary(res bindings.PVPResult, defenderID uint64) string {
	switch {
	case res.Expired:
		return fmt.Sprintf("PVP vs #%d: expired (window missed)", defenderID)
	case res.Won:
		return fmt.Sprintf("PVP WIN vs #%d: +%s cash, drug#%s ×%s, rep %+d, infamy %+d, heat→%d",
			defenderID, bigStrLocal(res.CashStolen), bigStrLocal(res.DrugIDStolen), bigStrLocal(res.DrugsStolen),
			res.RepChange, res.InfamyChange, res.NewHeat)
	default:
		return fmt.Sprintf("PVP LOSS vs #%d: rep %+d, infamy %+d, heat→%d",
			defenderID, res.RepChange, res.InfamyChange, res.NewHeat)
	}
}

func bigStrLocal(v *big.Int) string {
	if v == nil {
		return "0"
	}
	return v.String()
}

// --- Jail: breakout (free, commit-reveal) + bail (ETH, single-tx) ---

// SubmitBreakout commits a free jailbreak attempt: no ETH, no energy, but only
// once per UTC day and ~50% success. The scheduler resolves it.
func (m *Manager) SubmitBreakout(ctx context.Context, tokenID uint64) (uint64, error) {
	if m.reader != nil {
		if st, err := m.reader.GetFullDealerState(ctx, tokenID); err == nil {
			switch {
			case !st.IsJailed:
				return 0, fmt.Errorf("dealer is not in jail")
			case !st.CanBreakoutToday:
				return 0, fmt.Errorf("already attempted a breakout today — wait for 00:00 UTC or pay bail")
			}
		}
	}
	data, err := bindings.PackCommitBreakout(tokenID)
	if err != nil {
		return 0, fmt.Errorf("pack commitBreakout: %w", err)
	}
	receipt, err := m.sender.SendAndWait(ctx, m.net.Contracts.DealersActions, data, nil)
	if err != nil {
		return 0, fmt.Errorf("commit breakout: %w", err)
	}
	seq, err := bindings.ParseBreakoutSeq(receipt.Logs, m.net.Contracts.DealersActions)
	if err != nil {
		return 0, fmt.Errorf("parse breakout seq: %w", err)
	}
	commitBlock := receipt.BlockNumber.Uint64()
	if err := m.store.UpsertDealer(store.Dealer{TokenID: tokenID, WalletAddress: m.sender.AGW().Hex(), Network: m.net.Name}); err != nil {
		return 0, err
	}
	p := store.Pending{
		Seq: seq, TokenID: tokenID, Kind: store.KindBreakout,
		CommitBlock: commitBlock, RevealBlock: commitBlock + RevealOffset, ExpiryBlock: commitBlock + ExpiryWindow,
		TxHashCommit: receipt.TxHash.Hex(),
	}
	if err := m.store.InsertPending(p); err != nil {
		return 0, fmt.Errorf("persist breakout seq=%d (commit tx %s LANDED): %w", seq, receipt.TxHash.Hex(), err)
	}
	m.logger.Printf("breakout commit token=%d seq=%d reveal=%d", tokenID, seq, p.RevealBlock)
	return seq, nil
}

func (m *Manager) resolveBreakout(ctx context.Context, p store.Pending) error {
	data, err := bindings.PackResolveBreakout(p.Seq)
	if err != nil {
		return fmt.Errorf("pack resolveBreakout: %w", err)
	}
	receipt, err := m.sender.SendAndWait(ctx, m.net.Contracts.DealersActions, data, nil)
	if err != nil {
		return fmt.Errorf("send resolveBreakout seq=%d: %w", p.Seq, err)
	}
	res, err := bindings.ParseBreakoutResult(receipt.Logs, m.net.Contracts.DealersActions)
	if err != nil {
		return fmt.Errorf("parse breakout seq=%d: %w", p.Seq, err)
	}
	if err := m.store.MarkResolved(p.Seq, receipt.TxHash.Hex()); err != nil {
		return fmt.Errorf("mark resolved seq=%d: %w", p.Seq, err)
	}
	summary := "breakout SUCCESS — escaped jail"
	switch {
	case res.Expired:
		summary = "breakout expired (window missed) — still jailed"
	case !res.Success:
		summary = "breakout FAILED — still jailed (try again tomorrow or pay bail)"
	}
	m.store.AppendLog(store.LogEntry{TokenID: p.TokenID, Kind: store.KindBreakout, Summary: summary, TxHash: receipt.TxHash.Hex()})
	m.logger.Printf("breakout resolve token=%d seq=%d → %s", p.TokenID, p.Seq, summary)
	return nil
}

// PayBail pays the jail movement fee to release the dealer instantly (also
// resets heat to 0). Costs ETH — use only when the free breakout isn't an option.
func (m *Manager) PayBail(ctx context.Context, tokenID uint64) error {
	if m.reader == nil {
		return fmt.Errorf("bail requires a reader (fee lookup)")
	}
	fee, err := m.reader.MovementFee(ctx, bindings.JailArea)
	if err != nil {
		return fmt.Errorf("read bail fee: %w", err)
	}
	data, err := bindings.PackPayBail(tokenID)
	if err != nil {
		return fmt.Errorf("pack payBail: %w", err)
	}
	return m.sendSingleTx(ctx, m.net.Contracts.DealersActions, data, fee, tokenID, "bail", "bailed out of jail")
}

// --- Heat clear via wanted poster (ETH-free, commit-reveal) ---

// SubmitWantedPoster commits an ETH-free heat clear: it spends 1 attempt and, on
// a ~50% roll at resolve, fully clears heat. This is the ONLY heat-clear path we
// use — bribeCop (which costs ETH) is intentionally never called. The scheduler
// resolves the round when the reveal block arrives.
func (m *Manager) SubmitWantedPoster(ctx context.Context, tokenID uint64) (uint64, error) {
	if m.reader != nil {
		if st, err := m.reader.GetFullDealerState(ctx, tokenID); err == nil {
			switch {
			case st.IsJailed:
				return 0, fmt.Errorf("dealer is jailed")
			case st.HeatLevel == 0:
				return 0, fmt.Errorf("no heat to remove")
			case st.DailyAttemptsRemaining == 0:
				return 0, fmt.Errorf("removing a wanted poster costs 1 attempt — none left (resets 00:00 UTC, or Reset Attempts)")
			}
		}
	}
	data, err := bindings.PackCommitWantedPoster(tokenID)
	if err != nil {
		return 0, fmt.Errorf("pack commitWantedPoster: %w", err)
	}
	receipt, err := m.sender.SendAndWait(ctx, m.net.Contracts.DealersActions, data, nil)
	if err != nil {
		return 0, fmt.Errorf("commit wanted poster: %w", err)
	}
	seq, err := bindings.ParseWantedPosterSeq(receipt.Logs, m.net.Contracts.DealersActions)
	if err != nil {
		return 0, fmt.Errorf("parse wanted-poster seq: %w", err)
	}
	commitBlock := receipt.BlockNumber.Uint64()
	if err := m.store.UpsertDealer(store.Dealer{TokenID: tokenID, WalletAddress: m.sender.AGW().Hex(), Network: m.net.Name}); err != nil {
		return 0, err
	}
	p := store.Pending{
		Seq: seq, TokenID: tokenID, Kind: store.KindWantedPoster,
		CommitBlock: commitBlock, RevealBlock: commitBlock + RevealOffset, ExpiryBlock: commitBlock + ExpiryWindow,
		TxHashCommit: receipt.TxHash.Hex(),
	}
	if err := m.store.InsertPending(p); err != nil {
		return 0, fmt.Errorf("persist wanted-poster seq=%d (commit tx %s LANDED): %w", seq, receipt.TxHash.Hex(), err)
	}
	m.logger.Printf("wanted-poster commit token=%d seq=%d reveal=%d", tokenID, seq, p.RevealBlock)
	return seq, nil
}

func (m *Manager) resolveWantedPoster(ctx context.Context, p store.Pending) error {
	data, err := bindings.PackResolveWantedPoster(p.Seq)
	if err != nil {
		return fmt.Errorf("pack resolveWantedPoster: %w", err)
	}
	receipt, err := m.sender.SendAndWait(ctx, m.net.Contracts.DealersActions, data, nil)
	if err != nil {
		return fmt.Errorf("send resolveWantedPoster seq=%d: %w", p.Seq, err)
	}
	res, err := bindings.ParseWantedPosterResult(receipt.Logs, m.net.Contracts.DealersActions)
	if err != nil {
		return fmt.Errorf("parse wanted-poster seq=%d: %w", p.Seq, err)
	}
	if err := m.store.MarkResolved(p.Seq, receipt.TxHash.Hex()); err != nil {
		return fmt.Errorf("mark resolved seq=%d: %w", p.Seq, err)
	}
	summary := "wanted poster: heat cleared"
	switch {
	case res.Expired:
		summary = "wanted poster: expired (window missed)"
	case !res.Success:
		summary = "wanted poster: failed roll — heat unchanged (attempt spent)"
	}
	m.store.AppendLog(store.LogEntry{TokenID: p.TokenID, Kind: store.KindWantedPoster, Summary: summary, TxHash: receipt.TxHash.Hex()})
	m.logger.Printf("wanted-poster resolve token=%d seq=%d → %s", p.TokenID, p.Seq, summary)
	return nil
}

// SellDrop sells exotic loot in the black market — a guaranteed sale (no PVE
// gamble, no energy), single-tx. Regular buy/sell (commitGame) reverts in the
// black market, so this is the only way to trade there.
func (m *Manager) SellDrop(ctx context.Context, tokenID, drugID, amount uint64) error {
	data, err := bindings.PackSellDrop(tokenID, drugID, amount)
	if err != nil {
		return fmt.Errorf("pack sellDrop: %w", err)
	}
	return m.sendSingleTx(ctx, m.net.Contracts.DealersActions, data, nil, tokenID,
		"black_market_sell", fmt.Sprintf("black market: sold %d × drug#%d", amount, drugID))
}

// MissionCheckIn snapshots the dealer's mission baseline for the current epoch
// (gas only). Progress on daily/weekly missions is measured from this point, so
// it must run once per epoch before missions can complete.
func (m *Manager) MissionCheckIn(ctx context.Context, tokenID uint64) error {
	to := m.net.Contracts.DealersMissions
	if to == (common.Address{}) {
		return fmt.Errorf("missions not available on this network")
	}
	data, err := bindings.PackMissionCheckIn(tokenID)
	if err != nil {
		return fmt.Errorf("pack mission checkIn: %w", err)
	}
	err = m.sendSingleTx(ctx, to, data, nil, tokenID, "mission_checkin", "mission check-in (accepted today's missions)")
	if err == nil && m.reader != nil {
		m.reader.InvalidateMissions(tokenID)
	}
	return err
}

// ClaimMission redeems a completed mission's reward (gas only).
func (m *Manager) ClaimMission(ctx context.Context, tokenID, templateID uint64) error {
	to := m.net.Contracts.DealersMissions
	if to == (common.Address{}) {
		return fmt.Errorf("missions not available on this network")
	}
	data, err := bindings.PackMissionClaim(tokenID, templateID)
	if err != nil {
		return fmt.Errorf("pack mission claim: %w", err)
	}
	err = m.sendSingleTx(ctx, to, data, nil, tokenID, "mission_claim", fmt.Sprintf("claimed mission #%d reward", templateID))
	if err == nil && m.reader != nil {
		m.reader.InvalidateMissions(tokenID)
	}
	return err
}

// CheckIn performs the daily bank-heist check-in for one dealer (builds "focus"
// for the active season; gas only, no $CASH/ETH stake). Reverts on-chain if the
// dealer already checked in today, is jailed, or no season is live — callers
// that batch this should pre-skip jailed/already-done dealers.
func (m *Manager) CheckIn(ctx context.Context, tokenID uint64) error {
	to := m.net.Contracts.DealersBankHeist
	if to == (common.Address{}) {
		return fmt.Errorf("check-in not available on this network")
	}
	// A new season requires a one-time enter() before checkIn works — auto-join if
	// this dealer hasn't entered the current season. In Bank Heist V2 enter() is
	// payable (an ETH entry fee read live via SeasonEntryFee, sent as the tx value)
	// AND it records that day's check-in itself: after entering, focusState shows
	// count=1/lastDay=today, so a follow-up checkIn the same day reverts. So when we
	// enter here we're done — return without the standalone checkIn.
	if m.reader != nil {
		if season, err := m.reader.ActiveSeason(ctx); err == nil {
			if joined, err := m.reader.Entered(ctx, season, tokenID); err == nil && !joined {
				fee, err := m.reader.SeasonEntryFee(ctx, season)
				if err != nil {
					return fmt.Errorf("read season %d entry fee: %w", season, err)
				}
				// Sanity cap: never send a surprising amount of ETH if a season ever
				// reports an outsized fee (misread / contract change). 0.001 ETH is the
				// known fee; 0.01 leaves generous headroom while blocking anything wild.
				if fee.Cmp(maxSeasonEntryFeeWei) > 0 {
					return fmt.Errorf("skip check-in: dealer %d season entry fee %s wei exceeds the %s wei cap",
						tokenID, fee, maxSeasonEntryFeeWei)
				}
				// Preflight the entry (with the ETH value) so we don't burn gas on a
				// guaranteed revert — e.g. a dealer below the rep gate. Only a DEFINITIVE
				// simulated revert (perr==nil && !can) skips; an inconclusive read
				// (perr!=nil) falls through and attempts anyway, so an RPC hiccup never
				// blocks a real check-in.
				if m.sender != nil {
					if can, perr := m.reader.CanEnterSeason(ctx, m.sender.AGW(), tokenID, fee); perr == nil && !can {
						return fmt.Errorf("skip check-in: dealer %d can't enter season %d (rep gate or insufficient ETH)", tokenID, season)
					}
				}
				data, err := bindings.PackEnter(tokenID)
				if err != nil {
					return fmt.Errorf("pack enter: %w", err)
				}
				if err := m.sendSingleTx(ctx, to, data, fee, tokenID, "season_enter", fmt.Sprintf("entered heist season %d (%s wei)", season, fee)); err != nil {
					return fmt.Errorf("enter season: %w", err)
				}
				// enter() already counted today's check-in — refresh the cache and stop.
				m.reader.InvalidateCheckins()
				return nil
			}
		}
	}
	data, err := bindings.PackCheckIn(tokenID)
	if err != nil {
		return fmt.Errorf("pack checkIn: %w", err)
	}
	err = m.sendSingleTx(ctx, to, data, nil, tokenID, "checkin", "daily check-in")
	if err == nil && m.reader != nil {
		// Refresh the CheckedInToday cache so the autopilot doesn't re-check-in
		// (and revert) on the next tick within the TTL.
		m.reader.InvalidateCheckins()
	}
	return err
}

// ClaimSeason collects a dealer's ended-season reward (DealersBankHeist.claim).
// It's gas-only on our side and pays the ETH reward to the owner AGW. Callers
// should preflight with Reader.CanClaimSeason so a not-yet-due season doesn't burn
// gas on a guaranteed revert.
func (m *Manager) ClaimSeason(ctx context.Context, tokenID, seasonID uint64) error {
	to := m.net.Contracts.DealersBankHeist
	if to == (common.Address{}) {
		return fmt.Errorf("season claim not available on this network")
	}
	data, err := bindings.PackClaim(seasonID, tokenID)
	if err != nil {
		return fmt.Errorf("pack claim: %w", err)
	}
	return m.sendSingleTx(ctx, to, data, nil, tokenID, "season_claim", fmt.Sprintf("claimed season %d reward", seasonID))
}

// ResetAttempts pays purchaseAttemptReset to refill the dealer's daily attempts.
func (m *Manager) ResetAttempts(ctx context.Context, tokenID uint64) error {
	fee, err := m.attemptResetFee(ctx)
	if err != nil {
		return err
	}
	data, err := bindings.PackPurchaseAttemptReset(tokenID)
	if err != nil {
		return fmt.Errorf("pack purchaseAttemptReset: %w", err)
	}
	return m.sendSingleTx(ctx, m.net.Contracts.DealersActions, data, fee, tokenID, "reset_attempts", "attempts reset")
}

// Travel moves the dealer to another area, paying the area's movement fee (0 on
// free routes).
func (m *Manager) Travel(ctx context.Context, tokenID uint64, areaID uint8) error {
	if m.reader == nil {
		return fmt.Errorf("travel requires a reader (fee lookup)")
	}
	fee, err := m.reader.MovementFee(ctx, areaID)
	if err != nil {
		return fmt.Errorf("read movement fee: %w", err)
	}
	data, err := bindings.PackTravel(tokenID, areaID)
	if err != nil {
		return fmt.Errorf("pack travel: %w", err)
	}
	return m.sendSingleTx(ctx, m.net.Contracts.DealersActions, data, fee, tokenID, "travel", fmt.Sprintf("travel to area %d", areaID))
}

func (m *Manager) attemptResetFee(ctx context.Context) (*big.Int, error) {
	if m.reader == nil {
		return nil, fmt.Errorf("reset attempts requires a reader (fee lookup)")
	}
	cfg, err := m.reader.Config(ctx)
	if err != nil {
		return nil, fmt.Errorf("read fees: %w", err)
	}
	return cfg.AttemptResetFee, nil
}

// sendSingleTx sends a contract call, waits for the receipt, and logs it.
func (m *Manager) sendSingleTx(ctx context.Context, to common.Address, data []byte, value *big.Int, tokenID uint64, kind, summary string) error {
	receipt, err := m.sender.SendAndWait(ctx, to, data, value)
	if err != nil {
		return fmt.Errorf("%s: %w", kind, err)
	}
	if m.store != nil {
		m.store.UpsertDealer(store.Dealer{TokenID: tokenID, WalletAddress: m.sender.AGW().Hex(), Network: m.net.Name})
		m.store.AppendLog(store.LogEntry{TokenID: tokenID, Kind: kind, Summary: summary, TxHash: receipt.TxHash.Hex()})
	}
	m.logger.Printf("%s token=%d tx=%s", kind, tokenID, receipt.TxHash.Hex())
	return nil
}
