// Command dealers-tui is the multi-dealer TUI client for Dealers.sh.
//
// Subcommands:
//
//	(default)  read-only Fleet Overview TUI (Phase 0)
//	run        commit-reveal daemon: resume pending rounds and resolve them (Phase 1)
//	pve        submit one PVE hustle and drive it to resolution (Phase 1)
//	set-key    store a signer key in the OS keyring
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/big"
	"os"
	"os/signal"
	"strings"
	"time"

	"dealers/internal/allies"
	"dealers/internal/autostrat"
	"dealers/internal/chain"
	"dealers/internal/chain/bindings"
	"dealers/internal/config"
	"dealers/internal/dealer"
	"dealers/internal/engine"
	"dealers/internal/i18n"
	"dealers/internal/preflight"
	"dealers/internal/progstate"
	"dealers/internal/settings"
	"dealers/internal/store"
	"dealers/internal/template"
	"dealers/internal/tui"
	"dealers/internal/update"
	"dealers/internal/wallet"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
)

// version is stamped at build time via -ldflags (see build.ps1); "dev" otherwise.
var version = "dev"

func main() {
	args := os.Args[1:]
	cmd := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	var err error
	switch cmd {
	case "version", "--version", "-v":
		fmt.Println("dealers-tui", version)
		return
	case "setup":
		err = runSetupWizard(configPath(args))
	case "set-key":
		err = runSetKey(args)
	case "run":
		err = runDaemon(args)
	case "pve":
		err = runPVE(args)
	default:
		err = runFleet(args)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// bootstrap performs the shared FR1 sequence: load config, dial, preflight,
// resolve the owner address. It prints a concise bootstrap log.
type boot struct {
	cfg   *config.Config
	net   config.Network
	cl    *chain.Client
	owner common.Address
}

func bootstrap(ctx context.Context, cfgPath string) (*boot, error) {
	// No usable config (missing / placeholder / invalid) → run the interactive
	// setup instead of failing; then reload. No hand-editing required.
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Printf("First-time setup needed (%v).\n", err)
		if serr := runSetupWizard(cfgPath); serr != nil {
			return nil, serr
		}
		cfg, err = config.Load(cfgPath)
		if err != nil {
			return nil, err
		}
	}
	net := cfg.Network()
	owner := common.HexToAddress(cfg.Wallet.Address)
	fmt.Printf("dealers-tui %s\nnetwork:  %s (chain %d)\nowner:    %s\n", version, net.Name, net.ChainID, owner.Hex())

	cl, err := chain.Dial(ctx, net)
	if err != nil {
		return nil, err
	}
	res, err := preflight.Check(ctx, cl, cfg, owner)
	if err != nil {
		cl.Close()
		return nil, fmt.Errorf("preflight failed: %w", err)
	}
	kind := "EOA"
	if code, err := cl.CodeAt(ctx, owner); err == nil && len(code) > 0 {
		kind = "AGW smart wallet"
	}
	fmt.Printf("preflight: block %d · owner %s balance %s ETH · all contracts present\n",
		res.BlockNumber, kind, weiToEth(res.WalletWei))
	for _, w := range res.Warnings {
		fmt.Printf("  ⚠ %s\n", w)
	}
	return &boot{cfg: cfg, net: net, cl: cl, owner: owner}, nil
}

// buildSender resolves the signer key and constructs the AGW sender. Requires a
// configured signer source.
func buildSender(b *boot) (*chain.Sender, error) {
	if !b.cfg.Wallet.HasSigner() {
		return nil, fmt.Errorf("this command needs a signer key — set wallet.source (keyring|env) in config")
	}
	w, err := wallet.Resolve(b.cfg.Wallet)
	if err != nil {
		return nil, err
	}
	if w.Address == b.owner {
		fmt.Printf("signer:   %s (plain EOA)\n", w.Address.Hex())
	} else {
		fmt.Printf("signer:   %s (AGW signer)\n", w.Address.Hex())
	}
	return chain.NewSender(b.cl, b.owner, w.HexKey)
}

// runFleet is the default command: the full TUI. It is read-only unless a
// signer is configured, in which case it also starts the commit-reveal engine
// in the background so actions triggered from the UI are committed, resolved,
// and reflected live — all management happens in the TUI, no subcommands needed.
func runFleet(args []string) error {
	fs := flag.NewFlagSet("fleet", flag.ContinueOnError)
	cfgPath := fs.String("config", "config.json", "path to config.json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	appCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	bootCtx, cancel := context.WithTimeout(appCtx, 30*time.Second)
	b, err := bootstrap(bootCtx, *cfgPath)
	cancel()
	if err != nil {
		return err
	}
	defer b.cl.Close()

	reader := bindings.NewReader(b.cl)
	enumCtx, cancel2 := context.WithTimeout(appCtx, 20*time.Second)
	ids, err := resolveDealers(enumCtx, b.cfg, reader, b.owner)
	cancel2()
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return fmt.Errorf("no dealers found for %s — check wallet.address, mint at dealers.sh, or set dealer_token_ids", b.owner.Hex())
	}
	fmt.Printf("dealers:  %v\n", ids)

	deps := tui.Deps{
		Reader:    reader,
		IDs:       ids,
		Net:       b.net.Name,
		Owner:     b.owner,
		Poll:      time.Duration(b.cfg.PollIntervalSeconds) * time.Second,
		UI:        &tui.UIState{},
		Version:   version,
		BalanceFn: func(ctx context.Context) (*big.Int, error) { return b.cl.BalanceAt(ctx, b.owner) },
	}
	// Best-effort "new release" notice: poll the configured GitHub repo once on
	// startup and surface a newer tag in the header. Any failure is silent.
	if repo := b.cfg.GitHubRepo; repo != "" {
		deps.UpdateCheck = func(ctx context.Context) (string, string) {
			rel, err := update.Latest(ctx, repo)
			if err != nil || !update.Newer(version, rel.Version) {
				return "", ""
			}
			return rel.Version, rel.URL
		}
	}
	if min, ok := new(big.Int).SetString(b.cfg.MinETHRunwayWei, 10); ok {
		deps.MinRunwayWei = min
	}
	// Do-not-attack set: your own fleet + configured allies are fixed
	// (auto-protected); the rest is user-managed via the Allies screen (persisted
	// to allies.json). The contract only blocks attacking the SAME token, not
	// your other dealers, so hide the whole fleet from the PVP picker too.
	fixed := append(append([]uint64{}, ids...), b.cfg.Allies...)
	deps.Allies = allies.Load("allies.json", fixed)
	// UI-managed global toggles (settings.json) — e.g. auto-pay bail. Always
	// available so the preference persists even in read-only.
	deps.Settings = settings.Load("settings.json")
	// UI language (language.json): applies the saved choice process-wide on load;
	// switchable on the Settings screen. Russian by default.
	deps.Lang = i18n.Load("language.json")
	// Area names (FR12) — best-effort, static, fetched once.
	areaCtx, cancelA := context.WithTimeout(appCtx, 20*time.Second)
	if names, err := reader.AreaNames(areaCtx); err == nil {
		deps.AreaNames = names
	}
	cancelA()

	// PVE stake params (static-ish) for the per-action trade limit.
	spCtx, cancelSP := context.WithTimeout(appCtx, 15*time.Second)
	if sp, err := reader.StakeParams(spCtx); err == nil {
		deps.StakeParams = sp
	}
	cancelSP()

	// Drug id→name map (from every area's economy) for readable trade logs.
	drugNames := map[uint64]string{}
	dnCtx, cancelDN := context.WithTimeout(appCtx, 15*time.Second)
	if areas, err := reader.AllAreas(dnCtx); err == nil {
		for _, a := range areas {
			for _, d := range a.Drugs {
				if d.DrugID != nil && d.Name != "" {
					drugNames[d.DrugID.Uint64()] = d.Name
				}
			}
		}
	}
	cancelDN()

	// Leaderboard positions (PvE by rep, PvP by infamy). No on-chain getter, so
	// it's computed by enumerating all dealers — expensive, so refresh it in the
	// background every 60s rather than on every fleet poll.
	lb := dealer.NewLeaderboardCache()
	deps.Leaderboard = lb
	go refreshLeaderboard(appCtx, lb, reader)

	// With a signer, start the engine and enable actions in the UI.
	if b.cfg.Wallet.HasSigner() {
		sender, err := buildSender(b)
		if err != nil {
			return err
		}
		st, err := store.Open(b.cfg.DBPath)
		if err != nil {
			return err
		}
		defer st.Close()

		payBail := func() bool { return deps.Settings.Get(settings.KeyPayBail) }
		tmpls := template.Load("templates.json")
		deps.Templates = tmpls
		// Per-dealer program position (which step, how many reps) — persisted so the
		// sequential program resumes across restarts.
		progState := progstate.Load("progress.json")
		strategy, stratStore := buildProgram(b.cfg, deps.AreaNames, deps.Allies.IsAlly, payBail, tmpls, progAdapter{progState})
		deps.Strategies = stratStore
		eng := engine.New(b.cl, st, sender, ids, strategy, fileLogger())
		eng.Manager().SetDrugNames(drugNames)
		go eng.Run(appCtx)
		deps.Manager = eng.Manager()
		deps.Store = st
		deps.SpentFn = sender.Spent
		ap := eng.Autopilot()
		deps.AutopilotOn = ap.Enabled
		deps.ToggleAutopilot = func() bool { ap.SetEnabled(!ap.Enabled()); return ap.Enabled() }
		fmt.Printf("engine:   running (actions enabled, autopilot OFF, strategy=%s) — logs in dealers.log\n", strategySummary(ids, stratStore))
	} else {
		fmt.Println("engine:   off (no signer) — read-only")
	}

	p := tea.NewProgram(tui.NewApp(deps), tea.WithAltScreen())
	_, err = p.Run()
	return err
}

// progAdapter adapts a progstate.Store to the dealer.ProgState interface.
type progAdapter struct{ s *progstate.Store }

func (a progAdapter) Get(id uint64) (int, int) { p := a.s.Get(id); return p.Step, p.Reps }
func (a progAdapter) Set(id uint64, step, reps int) error {
	return a.s.Set(id, progstate.Pos{Step: step, Reps: reps})
}

// buildProgram wires the sequential Program strategy: it resolves each dealer's
// assigned template LIVE from the autostrat store (so an in-UI reassignment with
// 's' takes effect next tick) and compiles the template's steps into ProgSteps,
// resolving trade zone NAMES to area ids (defaulting to Manhattan/Amsterdam; a
// typo just trades the default route). Editing a template applies next tick too,
// since the steps are re-read every tick.
func buildProgram(cfg *config.Config, areaNames map[uint8]string, isAlly func(uint64) bool, payBail func() bool, ts *template.Store, ps dealer.ProgState) (dealer.Strategy, *autostrat.Store) {
	autostrat.Order = ts.Names() // the fleet 's' selector cycles the template names
	store := autostrat.Load("strategies.json", cfg.AutopilotStrategy, cfg.DealerStrategies)
	defBuy, _ := findAreaID(areaNames, "manhattan")
	defSell, _ := findAreaID(areaNames, "amsterdam")
	resolveArea := func(name string, def uint8) uint8 {
		if name == "" {
			return def
		}
		if id, ok := findAreaID(areaNames, name); ok {
			return id
		}
		return def
	}
	steps := func(id uint64) []dealer.ProgStep {
		t, ok := ts.Get(store.Get(id))
		if !ok {
			return nil
		}
		out := make([]dealer.ProgStep, 0, len(t.Steps))
		for _, s := range t.Steps {
			out = append(out, dealer.ProgStep{
				Action:          s.Action,
				Drug:            s.Drug,
				BuyArea:         resolveArea(s.BuyArea, defBuy),
				SellArea:        resolveArea(s.SellArea, defSell),
				HeistDifficulty: int8(clampDiff(int(s.HeistDifficulty))),
				HeatAt:          s.HeatAt,
				PayBail:         s.PayBail,
				Count:           s.Count,
			})
		}
		return out
	}
	return dealer.NewProgram(steps, ps, isAlly, payBail, defBuy), store
}

// clampDiff bounds a template's heist difficulty to [-1, 2] (-1 = max affordable).
func clampDiff(d int) int {
	if d < -1 {
		return -1
	}
	if d > 2 {
		return 2
	}
	return d
}

// strategySummary returns the single shared tag, or "mixed" when dealers run
// different policies (for the startup line / header chip).
func strategySummary(ids []uint64, store *autostrat.Store) string {
	if len(ids) == 0 || store == nil {
		return ""
	}
	first := store.Get(ids[0])
	for _, id := range ids {
		if store.Get(id) != first {
			return "mixed"
		}
	}
	return first
}

// findAreaID returns the area id whose name contains want (case-insensitive).
func findAreaID(names map[uint8]string, want string) (uint8, bool) {
	want = strings.ToLower(want)
	for id, n := range names {
		if strings.Contains(strings.ToLower(n), want) {
			return id, true
		}
	}
	return 0, false
}

// refreshLeaderboard recomputes leaderboard positions immediately and then every
// 60s until ctx is cancelled. Errors are silent (best-effort UI enrichment).
func refreshLeaderboard(ctx context.Context, lb *dealer.LeaderboardCache, r *bindings.Reader) {
	compute := func() {
		c, cancel := context.WithTimeout(ctx, 45*time.Second)
		_ = lb.Refresh(c, r)
		cancel()
	}
	compute()
	t := time.NewTicker(60 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			compute()
		}
	}
}

// fileLogger routes engine logs to dealers.log so background chatter never
// corrupts the alt-screen TUI. Falls back to discarding on error.
func fileLogger() *log.Logger {
	f, err := os.OpenFile("dealers.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return log.New(io.Discard, "", 0)
	}
	return log.New(f, "", log.LstdFlags)
}

// runDaemon starts the commit-reveal engine and blocks: it resumes any pending
// rounds and resolves them as their reveal blocks arrive (FR7/FR8). Ctrl+C to
// stop. Safe to restart — persisted rounds are picked up again.
func runDaemon(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	cfgPath := fs.String("config", "config.json", "path to config.json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	dialCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	b, err := bootstrap(dialCtx, *cfgPath)
	if err != nil {
		return err
	}
	defer b.cl.Close()

	sender, err := buildSender(b)
	if err != nil {
		return err
	}
	st, err := store.Open(b.cfg.DBPath)
	if err != nil {
		return err
	}
	defer st.Close()

	eng := engine.New(b.cl, st, sender, nil, dealer.ManualStrategy{}, nil)
	fmt.Println("engine:   running — resuming pending rounds, resolving on reveal. Ctrl+C to stop.")
	eng.Run(ctx)
	fmt.Println("\nengine:   stopped.")
	return nil
}

// runPVE submits one PVE hustle and drives it to resolution in-process.
//
//	dealers-tui pve -token 1 -hustle buy -choice deal -drug 0 -amount 5
func runPVE(args []string) error {
	fs := flag.NewFlagSet("pve", flag.ContinueOnError)
	cfgPath := fs.String("config", "config.json", "path to config.json")
	token := fs.Uint64("token", 0, "dealer token id")
	hustle := fs.String("hustle", "buy", "buy|sell")
	choice := fs.String("choice", "deal", "deal|threaten|bail")
	drug := fs.Uint64("drug", 0, "drug id")
	amount := fs.Uint64("amount", 1, "amount")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *token == 0 {
		return fmt.Errorf("-token is required")
	}
	ch, err := parseChoice(*choice)
	if err != nil {
		return err
	}
	hu, err := parseHustle(*hustle)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	dialCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	b, err := bootstrap(dialCtx, *cfgPath)
	if err != nil {
		return err
	}
	defer b.cl.Close()

	sender, err := buildSender(b)
	if err != nil {
		return err
	}
	st, err := store.Open(b.cfg.DBPath)
	if err != nil {
		return err
	}
	defer st.Close()

	eng := engine.New(b.cl, st, sender, nil, dealer.ManualStrategy{}, nil)
	// Run the engine in the background so the round resolves while we wait.
	go eng.Run(ctx)

	fmt.Printf("submitting PVE %s/%s token=%d drug=%d amount=%d …\n", *hustle, *choice, *token, *drug, *amount)
	commitCtx, cc := context.WithTimeout(ctx, 90*time.Second)
	defer cc()
	seq, err := eng.Manager().SubmitPVE(commitCtx, *token, ch, hu, *drug, *amount)
	if err != nil {
		return err
	}
	fmt.Printf("committed seq=%d — waiting for reveal+resolve …\n", seq)

	return waitResolved(ctx, st, seq, 3*time.Minute)
}

// waitResolved polls the store until the round leaves COMMITTED or times out.
func waitResolved(ctx context.Context, st *store.Store, seq uint64, timeout time.Duration) error {
	deadline := time.After(timeout)
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("timed out waiting for seq=%d to resolve", seq)
		case <-tick.C:
			var status, summary string
			err := st.DB().QueryRow(
				`SELECT p.status, COALESCE(a.summary,'')
				 FROM pending_actions p
				 LEFT JOIN action_log a ON a.token_id = p.token_id
				 WHERE p.seq = ? ORDER BY a.id DESC LIMIT 1`, seq).Scan(&status, &summary)
			if err != nil {
				continue
			}
			if status != store.StatusCommitted {
				fmt.Printf("seq=%d → %s  %s\n", seq, status, summary)
				return nil
			}
		}
	}
}

func parseChoice(s string) (bindings.Choice, error) {
	switch strings.ToLower(s) {
	case "deal":
		return bindings.ChoiceDeal, nil
	case "threaten":
		return bindings.ChoiceThreaten, nil
	case "bail":
		return bindings.ChoiceBail, nil
	}
	return 0, fmt.Errorf("invalid choice %q (deal|threaten|bail)", s)
}

func parseHustle(s string) (bindings.HustleType, error) {
	switch strings.ToLower(s) {
	case "buy":
		return bindings.HustleBuy, nil
	case "sell":
		return bindings.HustleSell, nil
	}
	return 0, fmt.Errorf("invalid hustle %q (buy|sell)", s)
}

// resolveDealers uses the explicit config list when provided, otherwise
// auto-discovers via ERC721Enumerable tokensOfOwner (FR2).
func resolveDealers(ctx context.Context, cfg *config.Config, r *bindings.Reader, owner common.Address) ([]uint64, error) {
	if len(cfg.DealerTokenIDs) > 0 {
		return cfg.DealerTokenIDs, nil
	}
	ids, err := r.TokensOfOwner(ctx, owner)
	if err != nil {
		return nil, fmt.Errorf("auto-discover dealers: %w (set dealer_token_ids in config to bypass)", err)
	}
	return ids, nil
}

// configPath resolves -config from a subcommand's args (default config.json).
func configPath(args []string) string {
	fs := flag.NewFlagSet("cfg", flag.ContinueOnError)
	p := fs.String("config", "config.json", "path to config.json")
	_ = fs.Parse(args)
	return *p
}

// runSetupWizard interactively collects the network, wallet address, and signer
// key, writes a valid config.json, and stores the key in the OS keyring — so no
// hand-editing is ever needed.
func runSetupWizard(path string) error {
	fmt.Println("\n──  dealers-tui setup  ──")
	r := bufio.NewReader(os.Stdin)

	const net = "mainnet"

	var addr string
	for {
		addr = ask(r, "Your wallet address — the AGW/owner that holds your dealers (0x…)", "")
		if common.IsHexAddress(addr) {
			break
		}
		fmt.Println("  ✗ not a valid 0x address, try again")
	}

	fmt.Print("Paste your signer private key (input is visible — keep your screen private): ")
	line, err := r.ReadString('\n')
	if err != nil {
		return err
	}
	key := strings.TrimSpace(line)
	if key == "" {
		return fmt.Errorf("empty private key")
	}
	// Validate the key parses and derive the signer address for confirmation
	// (for AGW this differs from the owner address above — that's expected).
	pk, perr := crypto.HexToECDSA(strings.TrimPrefix(key, "0x"))
	if perr != nil {
		return fmt.Errorf("invalid private key: %w", perr)
	}
	signer := crypto.PubkeyToAddress(pk.PublicKey)

	keyUser := net + "-owner"
	cfg := config.Config{
		ActiveNetwork:   net,
		Wallet:          config.WalletConfig{Address: addr, Source: "keyring", KeyringService: config.DefaultKeyringService, KeyringUser: keyUser},
		MinETHRunwayWei: "5000000000000000",
	}
	data, err := json.MarshalIndent(&cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := wallet.StoreKey(config.DefaultKeyringService, keyUser, key); err != nil {
		return fmt.Errorf("store key in keyring: %w", err)
	}
	fmt.Printf("✓ setup complete — wrote %s, stored key (signer %s).\n\n", path, shortAddr(signer))
	return nil
}

// shortAddr abbreviates an address for the setup confirmation line.
func shortAddr(a common.Address) string {
	h := a.Hex()
	return h[:6] + "…" + h[len(h)-4:]
}

// ask prompts for a value, showing an optional default.
func ask(r *bufio.Reader, prompt, def string) string {
	if def != "" {
		fmt.Printf("%s [%s]: ", prompt, def)
	} else {
		fmt.Printf("%s: ", prompt)
	}
	line, _ := r.ReadString('\n')
	if v := strings.TrimSpace(line); v != "" {
		return v
	}
	return def
}

// runSetKey stores a hex private key into the OS keyring. The key is read from
// stdin so it never lands in shell history or argv (NFR3).
//
//	dealers-tui set-key <service> <user>
func runSetKey(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: dealers-tui set-key <service> <user>")
	}
	fmt.Print("paste private key (input is visible; keep your terminal private): ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return err
	}
	key := strings.TrimSpace(line)
	if key == "" {
		return fmt.Errorf("empty key")
	}
	if err := wallet.StoreKey(args[0], args[1], key); err != nil {
		return err
	}
	fmt.Printf("stored key under %s/%s\n", args[0], args[1])
	return nil
}

// weiToEth formats wei as a short ETH string for the bootstrap log.
func weiToEth(wei *big.Int) string {
	if wei == nil {
		return "?"
	}
	f := new(big.Float).Quo(new(big.Float).SetInt(wei), big.NewFloat(params.Ether))
	return f.Text('f', 5)
}

// keep dealer import used (TxSender type reference for engine wiring clarity).
var _ dealer.TxSender = (*chain.Sender)(nil)
