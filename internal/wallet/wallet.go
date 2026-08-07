// Package wallet resolves the owner EOA private key from a non-plaintext
// source (NFR3). v1 supports the Windows Credential Manager (via go-keyring)
// and an ACL-limited environment variable fallback. The key is never written
// to disk or logs.
package wallet

import (
	"crypto/ecdsa"
	"fmt"
	"os"
	"strings"

	"dealers/internal/config"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/zalando/go-keyring"
)

// Wallet is a resolved signer EOA. HexKey is the normalized private key (no 0x)
// needed by zksync2-go's SmartAccount constructor; keep it in memory only.
type Wallet struct {
	Key     *ecdsa.PrivateKey
	Address common.Address
	HexKey  string
}

// Resolve loads the signer private key per cfg.Wallet. If SignerAddress is set,
// the derived EOA must match it (guards against the wrong credential). It is NOT
// checked against Address: for AGW the signer EOA legitimately differs from the
// owner (AGW) address (FR1: fail loud, but only on a real mismatch).
func Resolve(cfg config.WalletConfig) (*Wallet, error) {
	var hexKey string
	var err error

	switch cfg.Source {
	case "keyring":
		hexKey, err = keyring.Get(cfg.KeyringService, cfg.KeyringUser)
		if err != nil {
			return nil, fmt.Errorf("keyring get %s/%s: %w (store the key with: dealers-tui set-key)",
				cfg.KeyringService, cfg.KeyringUser, err)
		}
	case "env":
		hexKey = os.Getenv(cfg.EnvVar)
		if hexKey == "" {
			return nil, fmt.Errorf("env var %s is empty", cfg.EnvVar)
		}
	default:
		return nil, fmt.Errorf("unsupported wallet.source %q", cfg.Source)
	}

	normalized := strings.TrimPrefix(strings.TrimSpace(hexKey), "0x")
	key, err := crypto.HexToECDSA(normalized)
	if err != nil {
		return nil, fmt.Errorf("invalid private key from %s source: %w", cfg.Source, err)
	}
	addr := crypto.PubkeyToAddress(key.PublicKey)

	if cfg.SignerAddress != "" && !strings.EqualFold(cfg.SignerAddress, addr.Hex()) {
		return nil, fmt.Errorf("resolved signer address %s does not match configured wallet.signer_address %s",
			addr.Hex(), cfg.SignerAddress)
	}
	return &Wallet{Key: key, Address: addr, HexKey: normalized}, nil
}

// StoreKey saves a hex private key into the OS keyring (helper for a set-key
// subcommand so the user never puts the key in a file).
func StoreKey(service, user, hexKey string) error {
	return keyring.Set(service, user, hexKey)
}
