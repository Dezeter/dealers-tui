# Auto mode (autopilot) — what it does

Turn it on once (key `A` on the fleet screen) and the bot plays all your dealers around the clock.
Every second it checks each dealer and does the most important thing, in order. While autopilot is
on, the screen refreshes once per second and a live activity log streams under the fleet.

The step order below is a **recipe** — and you can change it (see the end).

1. **🔓 Get out of jail.** A jailed dealer immediately tries the free breakout (1 attempt/day, ~50%).
   It does **not** pay ETH bail by default — only if you enable that in settings (`o`).
2. **🎯 Season check-in (heist).** While a season is running it checks in once a day to build season
   progress. If the dealer hasn't joined the season yet, it joins (a one-time `$CASH` entry fee).
3. **⭐ Clear stars (heat).** At 3★ or higher it spends one free attempt (wanted poster) to drop heat
   and avoid jail. Never spends ETH on this.
4. **📋 Claim rewards & accept missions.** Claims completed missions (daily first, then weekly) and
   "accepts" new ones so their progress starts counting.
5. **🧭 Follow missions.** If an open mission needs activity the strategy wouldn't normally do (e.g. a
   trader dealer with a PvP mission), the bot temporarily switches, finishes the mission, then returns
   to its strategy. Priority: daily → then weekly.
6. **💰 Run heists (for heist missions).** Starts a heist at the highest affordable difficulty, pushes
   to the first cashable stage (≥ 2) and banks it — reliably, no hangs. Once the heist mission is done
   it starts no new runs. Runs per day are capped (default 3) so heists can't eat all the energy.
7. **🎮 Run its strategy (core).** With everything urgent handled, the dealer does its main job (below).

---

## Strategies (chosen **per dealer**, key `s`)

- **💊 PvE — trading.** Buys weed on Manhattan (stockpiles), travels to Amsterdam, sells higher, comes
  back for more. Out of energy → retreats to the Black Market.
- **🔫 PvP — raiding.** Attacks anyone not on the allies list. No target nearby → trades like PvE for a
  bit (which also moves it around and re-probes for targets). Out of energy → goes to the Black Market
  and sells the loot.
- **🚫 Manual — does nothing** (autopilot leaves that dealer alone).

> The old **Farm** strategy was removed — it bought 1 unit at a time and earned nothing. Anyone who
> was on `farm` was migrated to `pve`.

---

## Configurable steps (key `e`) and settings (key `o`)

- **`e` — step editor.** Shows the order of steps 2–7 (check-in, stars, missions, follow-missions,
  heists, core). Toggle on/off (`space`), move up/down (`[` / `]`), reset to default (`r`). Changes
  apply on the next tick — no restart. Example: put `core` above `heists` so trading isn't starved.
  > ⚠️ `core` is **greedy** — if the dealer has energy it always acts, so steps placed **after** core
  > rarely run. Put anything that must take priority **above** core.
- **`o` — settings.** One toggle so far: "pay ETH bail after a failed breakout" — **off by default**.
  Turn it on and a dealer that fails its free breakout will pay ETH bail.

---

## Good to know

- The bot **never** attacks your own dealers or your allies (the never-attack list, key `f`).
- It doesn't spend ETH on its own: stars are cleared for free, and bail is paid only if you enabled it.
- Season entry and heists cost `$CASH` (in-game currency), not ETH.
- Each dealer can run its own strategy at the same time.
- Everything shows in the live log: `#25 bought 5 weed — WIN`, `#24 attacked #142 — WIN`, etc.

> ⚠️ Heists run at the highest affordable difficulty (the stake is large — you can lose). The bot
> pushes to a cash-out stage and banks the pot rather than gambling all the way to the top.

▶️ Autopilot always starts **OFF** — you flip it on with `A`, so nothing is spent by accident.
