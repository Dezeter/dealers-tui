# Dealers.sh — the basics

A fully on-chain mafia strategy game on Abstract. Your dealer NFT **is** your character/save — all
progress lives on the blockchain, no server.

The core idea is simple: deal drugs — buy low, sell high, climb the ranks, fight other dealers, run
heists, and clear missions.

## Your stats

- **REP (reputation)** — main progress. Grows from trades; unlocks ranks, zones and PvP.
- **Cash (`$CASH`)** — in-game money for buys, heists and season entry.
- **Stash** — your inventory of drugs by type.
- **Energy (⚡)** — how many actions you can take per day (resets at 00:00 UTC).
- **Heat** — wanted level 0–5. Higher = higher arrest chance.
- **Infamy** — PvP fame, earned by beating other dealers.

> ⚠️ REP isn't only up: it includes a live bonus from your stash's value, so selling stash can
> temporarily drop your displayed rep (even below the PvP gate). That's normal.

## The basic loop

1. Buy drugs in a zone (spend `$CASH` + 1 energy).
2. Sell them (gain `$CASH` + 1 energy).
3. Every trade earns REP — that's how you grow.

> ⚠️ Every trade is a gamble: 50% tie (normal trade), 25% bonus, 25% bust (you lose the stake). On
> average it's cash-neutral — the profit comes from the price gap between zones, and reputation is
> what the risk pays you.

> ⚠️ Every action raises Heat. Pile up too much and your arrest chance climbs.

## Heat & jail

- The higher your Heat, the higher the arrest chance on each action.
- Clear Heat with the **wanted poster**: costs 1 attempt, no ETH, ~50% for a full clear. (The game
  also has a paid ETH reset — this tool deliberately never uses it.)
- If arrested: you lose some rep and a bit of stash. Get out by **breakout** (free, ~50%, once a day)
  or **bail** (ETH, instant, clears heat).

## Ranks (driven by REP)

Outsider(0) → Associate(100) → Dealer(250) → Soldier(600) → Capo(1500) → Consigliere(3000) →
Underboss(5500) → Don(10k) → Godfather(22k) → Legend(50k+)

What unlocks as you climb:
- REP 200 → PvP (attacking others)
- Soldier / 600 → Heists
- New zones (below)

## Zones (travel)

Different zones have different prices and different drugs. Moving between zones is the money-maker
(buy cheap in one, sell dear in another).

- **Manhattan** — free, from zero
- **Amsterdam** — free, REP 250
- **Colombia · Hong Kong · Seoul · Tokyo · Dubai** — unlock as REP grows (travel costs a little ETH)
- **Black Market** — special zone: entry gated on infamy (~10); it's where you sell PvP loot (normal
  trading doesn't work there)

> 💡 Manhattan ↔ Amsterdam is the free pair. It's enough to reach the first ranks and PvP with no ETH
> spend at all.

## PvP (from REP 200)

Attack another dealer in your zone. Win → take some of their cash, a bit of stash, +REP, +Infamy.
Lose → −REP and Infamy. There's a daily attack limit.

## Heists (from Soldier)

Push-your-luck: stake `$CASH` and advance through stages. Each cleared stage grows the pot but the
bust risk rises too. From a certain stage you can "bank the pot", or press your luck. A bust loses
the stake. Higher difficulty means a bigger stake and a higher REP requirement.

## Missions & season check-ins

On top of the core loop there are two daily reward systems:
- **Missions** — daily and weekly tasks (make N deals, win N PvP, clear N heist stages, etc.) that pay
  rewards. Each epoch you must "accept" a mission (check-in) or its progress doesn't count, then claim
  the reward when it's done.
- **Season check-in (bank-heist)** — a separate system: check in once a day to the current season and
  build "focus". The first check-in of a new season needs a one-time `$CASH` entry.

The client shows mission progress and can do all of this for you on autopilot.

## Where to start (first steps)

1. Sit in Manhattan, buy cheap drugs (Weed) and sell — grind rep.
2. Watch your Heat — clear it with the poster when it's high.
3. Hit REP 250 → Amsterdam unlocks. Start hauling Weed back and forth (it's pricier there).
4. REP 200+ → try PvP.
5. Don't skip the daily check-in and accepting/claiming missions — those are free rewards.
6. Out of energy → wait for 00:00 UTC (or reset for ETH).
7. Don't chase the far zones early — they need REP.

The beginner's golden rule: with zero ETH you can genuinely reach the first ranks and PvP on just the
Manhattan/Amsterdam pair. Spending ETH (boosts, far zones, heists) is optional, once you're comfortable.

Good luck, future bosses. 🤝
