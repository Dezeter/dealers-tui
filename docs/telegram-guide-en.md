# Dealers.sh TUI client — beginner's guide

A terminal program that runs your whole fleet of Dealers.sh NFT dealers from one screen:
buy/sell, PvP, heists, missions, check-ins, jail breakout, cross-zone arbitrage — by hand
or on autopilot. Everything runs on the Abstract chain.

> ⚠️ Unofficial, community-built tool. Not affiliated with or endorsed by Dealers.sh.
> It signs real on-chain transactions with your own key — use at your own risk.

## 1. Install

**Get the program**
- Easiest: download a prebuilt binary for your OS from the GitHub **Releases** page
  (Windows / macOS Intel & Apple Silicon / Linux). Drop it in its own folder.
- Or build it yourself with [Go](https://go.dev/dl/) (version pinned in `go.mod`). Pure Go —
  no C toolchain needed:
  ```sh
  go build -o dealers-tui ./cmd/dealers-tui
  ```

**First run = setup wizard**

Just launch it:
```sh
./dealers-tui
```
On the first run (or whenever no config is found) an interactive wizard opens — no hand-editing
files. It asks for:
- the network (mainnet),
- your **owner wallet address** (the one that holds the dealer NFTs — the address you see in the game),
- the **signer private key**.

The wizard stores the key in your OS keyring and writes a local `config.json` next to the program.
Everything else (allies, strategies, settings, autopilot steps) is managed inside the app and saved
to local JSON files.

**About the wallet (AGW)**
- The address is the **owner** wallet that holds the dealer NFTs (your AGW / game wallet). That's
  what the game shows.
- The private key is the **signer's** key. With an AGW that's a separate EOA. If you play with a
  plain wallet (not AGW), it's just that same address's key.
- The key is **never** written to a file — only to the OS keyring. Re-set it with
  `dealers-tui set-key dealers-tui mainnet-owner`; re-run the wizard with `dealers-tui setup`.

You'll see a grid of dealer cards. You're set. 🎉

## 2. Fleet screen

The fleet is an adaptive grid of cards, one per dealer (the number of columns adapts to terminal
width). The selected card is highlighted. Each card shows:
- **REP** — reputation (main progress; unlocks ranks, zones, PvP)
- **INF** — infamy (PvP fame)
- **Cash** — in-game `$CASH`
- **Energy** ⚡ and **Heat** 🔥 — meter bars (daily action budget / wanted level)
- **Area** — current zone (Manhattan, Amsterdam…)
- **Status** — IDLE / JAILED / SAFEHOUSE
- check-in, missions and the autopilot strategy on the bottom row

Below the grid a live **activity log** streams what each dealer did and the outcome.

**Keys on the fleet screen**
- `↑↓←→` (or `hjkl`) — move around the grid
- `enter` — open a dealer's card (details + manual actions)
- `n` — missions (progress, accept/claim)
- `c` — daily season check-in (bank-heist)
- `s` — cycle the selected dealer's autopilot strategy (pve → pvp → manual)
- `e` — autopilot step editor (order + on/off)
- `A` — autopilot on/off (spends money/energy — **off by default**)
- `m` — market & arbitrage
- `f` — allies (never-attack list)
- `o` — settings
- `r` — refresh · `q` — quit

## 3. Manual actions (dealer card, `enter`)

- `b` — buy a drug
- `s` — sell
- `p` — PvP: attack another dealer (pick a target from the list)
- `t` — travel to another zone (shows the fee and the required REP)
- `h` — start a heist · `g` — next stage · `o` — cash out · `x` — abandon
- `c` — clear heat (wanted poster — free, costs 1 energy, ~50%)
- `a` — restore energy (costs ETH)
- if jailed: `k` — breakout (free), `l` — bail (costs ETH)
- `r` — refresh · `esc` — back

In the buy/sell form: `↑↓` pick a drug (shows price and your holdings), type an amount, `enter` to
send. The program tells you the max: `max N (stake cap | cash | held)`.

> ⚠️ Buying/selling is a **gamble**, not a guaranteed trade: 50% tie (normal trade), 25% win
> (bonus), 25% loss (you lose the stake). It's cash-EV-neutral — the profit comes from the price
> gap between zones, and the risk is what pays you reputation.

## 4. Missions & check-ins

The game has two independent daily systems:
- **Season check-in (bank-heist)** — key `c` on the fleet. Builds a seasonal "focus" streak. The
  first check-in of a new season needs a one-time `$CASH` entry; the app does it for you.
- **Missions (daily/weekly)** — key `n`. The screen shows each mission's progress as a bar. Inside:
  `a` accept/check-in (required each epoch, or progress doesn't count), `c` claim all ready rewards.

The autopilot handles both automatically (check-ins, accepting + claiming missions, and steering
toward them) — see the Auto-mode guide.

## 5. Things a beginner should know

1. **Energy ⚡ ≠ Heat 🔥.** Energy is your daily action budget; when it's gone press `a` (ETH reset)
   or wait for 00:00 UTC. Heat is your wanted level; high heat = arrest risk; clear it with `c`
   (free poster) or it clears when you post bail.
2. **Every action costs 1 energy regardless of size** — so trade in big batches.
3. **Per-trade stake cap.** Low ranks can only trade a little at a time; it grows with rank — level
   up REP.
4. **If arrested (JAILED):** `k` breakout is free (~50%, once per day); `l` bail costs ETH but is
   instant and clears heat. Try `k` first, then `l` or wait for tomorrow.

## 6. Making money (arbitrage)

Press `m` on the fleet screen for a "buy cheap here → sell dear there" board:
```
Heroin  buy 90 @Colombia → sell 240 @Dubai  +150/u
Weed    buy 1  @Manhattan → sell 2 @Amsterdam +1/u
```
The loop:
1. Read a route on the market (`m`)
2. `t` — travel to the buy zone
3. `b` — buy a **big** batch
4. `t` — travel to the sell zone
5. `s` — sell

Notes: travel costs ETH (except Manhattan↔Amsterdam, which is free), and pricier routes need higher
REP (shown in the table). Start with Manhattan→Amsterdam on Weed, then unlock zones as your rep grows.

Good luck, future bosses. 🤝
