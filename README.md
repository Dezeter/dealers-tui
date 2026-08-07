# dealers-tui

A terminal (TUI) client for the on-chain game **[Dealers.sh](https://dealers.sh)** on the
Abstract network. Manage a whole fleet of dealer NFTs from one wallet: watch state live,
trade/raid/heist by hand, or let the built-in **autopilot** play for you.

> Unofficial, community-built tool. Not affiliated with or endorsed by Dealers.sh.
> Use at your own risk — it signs real on-chain transactions with your own key.

## Features

- **Fleet dashboard** — every dealer as a live card (rep, cash, heat, energy, area, missions, strategy), auto-refreshing.
- **Manual actions** — buy/sell, PvP attacks, heists, travel, jail breakout/bail, black-market loot sales, wanted-poster (heat) clearing.
- **Missions** — see daily/weekly progress; accept (check-in) and claim rewards.
- **Autopilot** — opt-in, per-NFT strategies (`pve` trade run / `pvp` raid / `manual`), with a
  **configurable step pipeline** you can reorder and toggle in-app (missions, heists, stars,
  season check-in, core job).
- **Safety** — your private key lives only in the OS keyring (never in files); autopilot starts
  disabled; your own fleet and an allies list are never attacked.

## Download

Grab a prebuilt binary for your OS from the [**Releases**](../../releases) page
(Windows / macOS Intel & Apple Silicon / Linux, amd64 & arm64).

## Run

```sh
# Windows: just double-click dealers-tui.exe, or from a terminal:
./dealers-tui
```

On first run it launches an interactive setup wizard — no config editing needed. It asks for your
owner/AGW wallet address and signer key, stores the key in the OS keyring, and writes a local
`config.json`. Everything else (allies, per-dealer strategies, settings, autopilot steps) is managed
inside the app and saved to local JSON files.

In-app: `↑↓←→` select · `enter` details · `A` toggle autopilot · `s` per-dealer strategy ·
`e` edit autopilot steps · `n` missions · `o` settings · `q` quit.

## Build from source

Requires [Go](https://go.dev/dl/) (see `go.mod` for the version). Pure Go — no C toolchain needed.

```sh
go build -trimpath -ldflags "-s -w" -o dealers-tui ./cmd/dealers-tui
```

Cross-compile (example, Windows amd64):

```sh
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -o dealers-tui.exe ./cmd/dealers-tui
```

`make build` and `build.ps1` are also provided. Tests: `go test ./...`.

## Docs

Setup & usage guide — [English](docs/telegram-guide-en.md) · [Русский](docs/telegram-guide-ru.md)
What the autopilot does, step by step — [English](docs/auto-mode-en.md) · [Русский](docs/auto-mode-ru.md)
Game basics — [English](docs/game-basics-en.md) · [Русский](docs/game-basics-ru.md)

- [`docs/CHAIN_REFERENCE.md`](docs/CHAIN_REFERENCE.md) — on-chain contract reference (for developers)

## Security

- The signer private key is stored in your operating system's keyring, never written to disk by this app.
- Do not commit `config.json` or the local `*.json` state files — they are git-ignored.
- The autopilot can spend in-game `$CASH` (missions, heists, season entry) and, only if you enable the
  bail toggle, ETH for jail bail. Review the Settings and Steps screens before enabling it.

## License

[MIT](LICENSE) © 2026 Dezeter
