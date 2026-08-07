package chain

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	ethabi "github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/zksync-sdk/zksync2-go/accounts"
	"github.com/zksync-sdk/zksync2-go/clients"
	zkTypes "github.com/zksync-sdk/zksync2-go/types"
	"github.com/zksync-sdk/zksync2-go/utils"
)

// EOAValidatorAddress is Abstract's EOA validator module. AGW (a Clave-fork
// smart account) validates a transaction signature by unwrapping
// abi.encode(ecdsaSig, EOA_VALIDATOR, hookData[]) and delegating the ECDSA
// check to this module. Ported from rugpullbakery (proven on Abstract mainnet).
var EOAValidatorAddress = common.HexToAddress("0x74b9ae28EC45E3FA11533c7954752597C3De3e7A")

// We always use the EOA-validator wrapper path (not the native k1 path): our
// send volume is tiny, so the ~8% gas saving isn't worth the extra runtime
// fallback machinery. The wrapper is the safe default that works for any AGW.

const listHooksABI = `[{
  "inputs":[{"internalType":"bool","name":"isValidation","type":"bool"}],
  "name":"listHooks",
  "outputs":[{"internalType":"address[]","name":"","type":"address[]"}],
  "stateMutability":"view","type":"function"}]`

// validationHookCount reads how many validation hooks the AGW has installed. The
// wrapper signature must carry exactly one (empty) hookData entry per hook or
// AGWAccount.runValidationHooks reverts. A read error ⇒ 0 (best effort).
func validationHookCount(client *clients.Client, agw common.Address) int {
	parsed, err := ethabi.JSON(strings.NewReader(listHooksABI))
	if err != nil {
		return 0
	}
	contract := bind.NewBoundContract(agw, parsed, client, client, client)
	var out []interface{}
	if err := contract.Call(&bind.CallOpts{}, &out, "listHooks", true); err != nil {
		return 0
	}
	hooks, ok := out[0].([]common.Address)
	if !ok {
		return 0
	}
	return len(hooks)
}

// buildAGWSignature wraps a raw 65-byte ECDSA signature as
// abi.encode(bytes rawSig, address validator, bytes[] hookData) with hookCount
// empty hookData entries.
func buildAGWSignature(rawSig []byte, hookCount int) ([]byte, error) {
	bytesT, _ := ethabi.NewType("bytes", "", nil)
	addressT, _ := ethabi.NewType("address", "", nil)
	bytesArrayT, _ := ethabi.NewType("bytes[]", "", nil)
	args := ethabi.Arguments{{Type: bytesT}, {Type: addressT}, {Type: bytesArrayT}}

	hookData := make([][]byte, hookCount)
	for i := range hookData {
		hookData[i] = []byte{}
	}
	return args.Pack(rawSig, EOAValidatorAddress, hookData)
}

// newAGWSmartAccount builds a zksync2-go SmartAccount for the AGW that signs via
// the EOA-validator wrapper. hookCount is learned once at construction.
func newAGWSmartAccount(agw common.Address, privateKeyHex string, client *clients.Client) *accounts.SmartAccount {
	hookCount := validationHookCount(client, agw)
	signer := agwPayloadSigner(hookCount)
	return accounts.NewSmartAccount(agw, privateKeyHex, &signer, &agwPopulateTransaction, client)
}

// agwPayloadSigner produces the wrapper signature for each payload: raw ECDSA
// (via the SDK, correct v/encoding) then abi.encode wrap.
func agwPayloadSigner(hookCount int) accounts.PayloadSigner {
	return func(ctx context.Context, payload []byte, secret interface{}, client *clients.Client) ([]byte, error) {
		raw, err := accounts.SignPayloadWithECDSA(ctx, payload, secret, client)
		if err != nil {
			return nil, err
		}
		return buildAGWSignature(raw, hookCount)
	}
}

// agwPopulateTransaction fills nonce/gas/fee for an AGW tx. Critically it uses
// eth_estimateGas with from=AGW (NOT zks_estimateFee, which swaps `from` to the
// signer EOA and would estimate the wrong account). Ported from rugpullbakery.
var agwPopulateTransaction accounts.TransactionBuilder = func(ctx context.Context, tx *zkTypes.Transaction, secret interface{}, client *clients.Client) error {
	var err error
	if client == nil {
		return errors.New("client is required")
	}
	if tx.ChainID == nil {
		if tx.ChainID, err = client.ChainID(ctx); err != nil {
			return fmt.Errorf("chain id: %w", err)
		}
	}
	if tx.From == nil {
		return errors.New("from address is required")
	}
	if tx.Nonce == nil {
		nonce, err := client.NonceAt(ctx, *tx.From, nil)
		if err != nil {
			return fmt.Errorf("nonce: %w", err)
		}
		tx.Nonce = new(big.Int).SetUint64(nonce)
	}
	if tx.GasTipCap == nil {
		tx.GasTipCap = common.Big0
	}
	if tx.GasPerPubdata == nil {
		tx.GasPerPubdata = utils.DefaultGasPerPubdataLimit
	}
	if tx.Gas == nil || tx.Gas.Uint64() == 0 {
		gas, err := client.EstimateGas(ctx, ethereum.CallMsg{From: *tx.From, To: tx.To, Value: tx.Value, Data: tx.Data})
		if err != nil {
			return fmt.Errorf("estimate gas: %w", err)
		}
		tx.Gas = new(big.Int).SetUint64(gas * 3 / 2) // 1.5× headroom
	}
	if tx.GasFeeCap == nil {
		gasPrice, err := client.SuggestGasPrice(ctx)
		if err != nil {
			return fmt.Errorf("gas price: %w", err)
		}
		tx.GasFeeCap = gasPrice
	}
	return nil
}
