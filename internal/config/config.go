// Package config loads operational settings from a single config.json and
// resolves the active compiled-in network profile. Contract addresses and
// endpoints are NOT configurable here — config.json only picks the network and
// operational parameters (TZ NFR6).
package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/ethereum/go-ethereum/common"
)

// Config is the on-disk operational configuration (config.json).
type Config struct {
	// ActiveNetwork selects the compiled-in profile: "mainnet" | "testnet".
	// Development and first runs use "testnet" (TZ risk §3.1).
	ActiveNetwork string `json:"active_network"`

	// Wallet describes where the owner EOA private key comes from (ADR-1:
	// direct owner wallet, no AGW session key in v1).
	Wallet WalletConfig `json:"wallet"`

	// DealerTokenIDs is the explicit list of dealer NFT token IDs to manage.
	// Used as the fallback path when auto-enumeration is unavailable (FR2 /
	// TODO-5). Empty means "auto-discover from the wallet".
	DealerTokenIDs []uint64 `json:"dealer_token_ids,omitempty"`

	// Allies is a list of other players' dealer token IDs that should never be
	// attacked — they are hidden from the PVP target picker.
	Allies []uint64 `json:"allies,omitempty"`

	// AutopilotStrategy is the fleet-default autopilot policy when it's switched
	// on: "farm" (cheapest-drug rep farming), "pve" (weed Manhattan→Amsterdam
	// arbitrage run), "pvp" (raid non-allies, PvE when no targets, sell loot), or
	// "manual" (never acts). The autopilot always STARTS disabled regardless.
	AutopilotStrategy string `json:"autopilot_strategy,omitempty"`

	// DealerStrategies overrides the strategy per dealer NFT (token id → policy),
	// e.g. {"24": "pvp", "25": "pve"}. Any dealer not listed uses
	// AutopilotStrategy. Values must be valid policy names.
	DealerStrategies map[uint64]string `json:"dealer_strategies,omitempty"`

	// PollIntervalSeconds controls the Fleet Overview refresh cadence (FR3).
	PollIntervalSeconds int `json:"poll_interval_seconds,omitempty"`

	// DBPath is the SQLite file location (default "./dealers.db").
	DBPath string `json:"db_path,omitempty"`

	// MinETHRunwayWei warns/blocks batch actions below this balance (FR10/FR11).
	// Stored as a decimal string to survive JSON without float rounding.
	MinETHRunwayWei string `json:"min_eth_runway_wei,omitempty"`

	// GitHubRepo ("owner/name") is polled once on startup for a newer release so
	// the client can show an "update available" notice. Defaults to the upstream
	// repo; set "" to disable the check entirely.
	GitHubRepo string `json:"github_repo,omitempty"`
}

// WalletConfig separates the on-chain OWNER identity from the SIGNER key.
//
// Dealers are usually held by an Abstract Global Wallet (AGW) — a smart-contract
// account whose address (Address) differs from the EOA that signs for it
// (SignerAddress, derived from the key). Reads and ownership use Address;
// transactions are signed by the key (Phase 1). For a plain EOA setup the two
// addresses are identical. The private key itself is never stored here (NFR3).
type WalletConfig struct {
	// Address is the on-chain identity that OWNS the dealer NFTs and is the
	// msg.sender for game actions — the AGW smart-wallet address (or a plain EOA
	// if not using AGW). Used for enumeration and balance. Required.
	Address string `json:"address"`

	// Source of the signer key: "keyring", "env", or "" / "none" for read-only
	// (Phase 0 needs no key). Writes (Phase 1+) require a real source.
	Source string `json:"source,omitempty"`

	// SignerAddress is the OPTIONAL expected EOA derived from the key, used to
	// sanity-check that the right credential is loaded. For AGW this is the
	// session/owner signer EOA and differs from Address; leave empty to skip.
	SignerAddress string `json:"signer_address,omitempty"`

	// KeyringService / KeyringUser identify the credential when Source=keyring.
	KeyringService string `json:"keyring_service,omitempty"`
	KeyringUser    string `json:"keyring_user,omitempty"`

	// EnvVar names the environment variable holding the hex key when Source=env.
	EnvVar string `json:"env_var,omitempty"`
}

// HasSigner reports whether a signer key source is configured (needed for
// write actions; false means read-only).
func (w WalletConfig) HasSigner() bool {
	return w.Source != "" && w.Source != "none"
}

// Defaults applied when fields are omitted from config.json.
const (
	DefaultPollIntervalSeconds = 15
	DefaultDBPath              = "./dealers.db"
	DefaultKeyringService      = "dealers-tui"
	// DefaultGitHubRepo is polled for release-update notifications. Set
	// github_repo to any value without a "/" (e.g. "none") to disable the check.
	DefaultGitHubRepo = "Dezeter/dealers-tui"
)

// Load reads and validates config.json from path, applying defaults.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}
	var c Config
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("parse config %q: %w", path, err)
	}
	c.applyDefaults()
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) applyDefaults() {
	if c.PollIntervalSeconds == 0 {
		c.PollIntervalSeconds = DefaultPollIntervalSeconds
	}
	if c.DBPath == "" {
		c.DBPath = DefaultDBPath
	}
	if c.Wallet.KeyringService == "" {
		c.Wallet.KeyringService = DefaultKeyringService
	}
	if c.GitHubRepo == "" {
		c.GitHubRepo = DefaultGitHubRepo
	}
	if c.AutopilotStrategy == "" {
		c.AutopilotStrategy = "pve"
	}
	// Migrate the retired "farm" strategy to "pve".
	if c.AutopilotStrategy == "farm" {
		c.AutopilotStrategy = "pve"
	}
	for id, name := range c.DealerStrategies {
		if name == "farm" {
			c.DealerStrategies[id] = "pve"
		}
	}
}

// KnownAutopilotStrategies lists the valid autopilot_strategy values.
var KnownAutopilotStrategies = map[string]bool{
	"pve": true, "pvp": true, "manual": true,
}

// Validate rejects configs that would fail loudly later (FR1: no silent abort).
func (c *Config) Validate() error {
	if _, ok := profiles[c.ActiveNetwork]; !ok {
		return fmt.Errorf("active_network %q is not a known profile (want mainnet|testnet)", c.ActiveNetwork)
	}
	switch c.Wallet.Source {
	case "", "none":
		// read-only: no signer key
	case "keyring":
		if c.Wallet.KeyringUser == "" {
			return fmt.Errorf("wallet.source=keyring requires wallet.keyring_user")
		}
	case "env":
		if c.Wallet.EnvVar == "" {
			return fmt.Errorf("wallet.source=env requires wallet.env_var")
		}
	default:
		return fmt.Errorf("wallet.source %q invalid (want keyring|env|none)", c.Wallet.Source)
	}
	if c.Wallet.Address == "" {
		return fmt.Errorf("wallet.address is required (the AGW/owner address that holds the dealers)")
	}
	if !common.IsHexAddress(c.Wallet.Address) {
		return fmt.Errorf("wallet.address %q is not a valid 0x address — edit config.json and set your AGW/owner address", c.Wallet.Address)
	}
	if !KnownAutopilotStrategies[c.AutopilotStrategy] {
		return fmt.Errorf("autopilot_strategy %q invalid (want farm|pve|pvp|manual)", c.AutopilotStrategy)
	}
	for id, name := range c.DealerStrategies {
		if !KnownAutopilotStrategies[name] {
			return fmt.Errorf("dealer_strategies[%d] %q invalid (want farm|pve|pvp|manual)", id, name)
		}
	}
	return nil
}

// Network returns the resolved compiled-in profile for the active network.
func (c *Config) Network() Network {
	return profiles[c.ActiveNetwork]
}

// IsMainnet reports whether the active profile targets Abstract mainnet.
func (c *Config) IsMainnet() bool { return c.ActiveNetwork == "mainnet" }

// Profile returns a compiled-in network profile by name.
func Profile(name string) (Network, bool) {
	n, ok := profiles[name]
	return n, ok
}
