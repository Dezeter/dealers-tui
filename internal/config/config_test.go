package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTemp(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadValidTestnet(t *testing.T) {
	p := writeTemp(t, `{
		"active_network": "testnet",
		"wallet": {"source": "env", "address": "0x610CcEe1AE4aFF961d043faB379491C2997383F7", "env_var": "DEALERS_KEY"}
	}`)
	c, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.PollIntervalSeconds != DefaultPollIntervalSeconds {
		t.Errorf("default poll interval not applied: %d", c.PollIntervalSeconds)
	}
	if c.DBPath != DefaultDBPath {
		t.Errorf("default db path not applied: %q", c.DBPath)
	}
	net := c.Network()
	if net.ChainID != 11124 {
		t.Errorf("testnet chain id = %d, want 11124", net.ChainID)
	}
	if net.WS != "wss://api.testnet.abs.xyz/ws" {
		t.Errorf("unexpected ws endpoint %q", net.WS)
	}
	if c.IsMainnet() {
		t.Error("IsMainnet() true for testnet")
	}
}

func TestRejectUnknownNetwork(t *testing.T) {
	p := writeTemp(t, `{"active_network":"devnet","wallet":{"source":"env","address":"0x1","env_var":"X"}}`)
	if _, err := Load(p); err == nil {
		t.Fatal("expected error for unknown network")
	}
}

func TestRejectBadWalletSource(t *testing.T) {
	p := writeTemp(t, `{"active_network":"testnet","wallet":{"source":"file","address":"0x1"}}`)
	if _, err := Load(p); err == nil {
		t.Fatal("expected error for unsupported wallet source")
	}
}

func TestRejectKeyringWithoutUser(t *testing.T) {
	p := writeTemp(t, `{"active_network":"testnet","wallet":{"source":"keyring","address":"0x1"}}`)
	if _, err := Load(p); err == nil {
		t.Fatal("expected error: keyring source without keyring_user")
	}
}

func TestRejectUnknownField(t *testing.T) {
	p := writeTemp(t, `{"active_network":"testnet","wallet":{"source":"env","address":"0x1","env_var":"X"},"typo_field":true}`)
	if _, err := Load(p); err == nil {
		t.Fatal("expected error for unknown config field")
	}
}

func TestRejectPlaceholderAddress(t *testing.T) {
	p := writeTemp(t, `{"active_network":"mainnet","wallet":{"source":"keyring","keyring_user":"x","address":"0xPUT_YOUR_ADDRESS_HERE"}}`)
	if _, err := Load(p); err == nil {
		t.Error("expected error for a non-hex placeholder address")
	}
}

func TestDealerStrategiesParseAndValidate(t *testing.T) {
	p := writeTemp(t, `{
		"active_network":"mainnet",
		"wallet":{"source":"env","address":"0x610CcEe1AE4aFF961d043faB379491C2997383F7","env_var":"X"},
		"autopilot_strategy":"farm",
		"dealer_strategies":{"24":"pvp","25":"pve"}
	}`)
	c, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.DealerStrategies[24] != "pvp" || c.DealerStrategies[25] != "pve" {
		t.Errorf("per-dealer strategies parsed wrong: %+v", c.DealerStrategies)
	}
}

func TestRejectBadDealerStrategy(t *testing.T) {
	p := writeTemp(t, `{
		"active_network":"mainnet",
		"wallet":{"source":"env","address":"0x610CcEe1AE4aFF961d043faB379491C2997383F7","env_var":"X"},
		"dealer_strategies":{"24":"bogus"}
	}`)
	if _, err := Load(p); err == nil {
		t.Error("expected error for an unknown per-dealer strategy")
	}
}

func TestMainnetProfileResolves(t *testing.T) {
	p := writeTemp(t, `{"active_network":"mainnet","wallet":{"source":"env","address":"0x610CcEe1AE4aFF961d043faB379491C2997383F7","env_var":"X"}}`)
	c, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !c.IsMainnet() || c.Network().ChainID != 2741 {
		t.Errorf("mainnet not resolved: mainnet=%v chain=%d", c.IsMainnet(), c.Network().ChainID)
	}
	if c.Network().Contracts.DealersMulticall.Hex() != "0x39249C625D7a6C952A5aC389510839eB1bB33099" {
		t.Errorf("mainnet multicall addr wrong: %s", c.Network().Contracts.DealersMulticall.Hex())
	}
}
