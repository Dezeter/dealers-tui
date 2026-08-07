package chain

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/zksync-sdk/zksync2-go/accounts"
	"github.com/zksync-sdk/zksync2-go/clients"
	zkTypes "github.com/zksync-sdk/zksync2-go/types"
)

// Sender signs and submits AGW transactions and waits for their receipts. All
// sends are serialized by a mutex so the single wallet never has two in-flight
// txs racing for the same nonce (NFR2) — combined with waiting for each receipt
// before returning, nonce collisions are impossible by construction.
type Sender struct {
	agw     common.Address
	chainID *big.Int
	zk      *clients.Client
	account *accounts.SmartAccount
	eth     receiptClient // for typed receipts/logs (go-ethereum)

	mu sync.Mutex

	spentMu sync.Mutex
	spent   *big.Int // cumulative session spend (gas + value), NFR4
}

// receiptClient is the subset of ethclient used to poll receipts (lets tests
// substitute a fake).
type receiptClient interface {
	TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error)
}

// NewSender wires an AGW sender onto an existing chain client. keyHex is the
// signer EOA's private key (hex, with or without 0x); agw is the smart-wallet
// address it signs for.
func NewSender(cl *Client, agw common.Address, keyHex string) (*Sender, error) {
	zk, err := clients.Dial(cl.Net.RPC)
	if err != nil {
		return nil, fmt.Errorf("dial zksync rpc %s: %w", cl.Net.RPC, err)
	}
	return &Sender{
		agw:     agw,
		chainID: new(big.Int).Set(cl.ChainID),
		zk:      zk,
		account: newAGWSmartAccount(agw, keyHex, zk),
		eth:     cl.RPC,
		spent:   big.NewInt(0),
	}, nil
}

// AGW returns the smart-wallet address this sender acts as.
func (s *Sender) AGW() common.Address { return s.agw }

// Spent returns the cumulative ETH spent this session (gas + value), in wei.
func (s *Sender) Spent() *big.Int {
	s.spentMu.Lock()
	defer s.spentMu.Unlock()
	return new(big.Int).Set(s.spent)
}

func (s *Sender) addSpend(r *types.Receipt, value *big.Int) {
	cost := new(big.Int)
	if r != nil && r.EffectiveGasPrice != nil {
		cost.Mul(new(big.Int).SetUint64(r.GasUsed), r.EffectiveGasPrice)
	}
	if value != nil {
		cost.Add(cost, value)
	}
	s.spentMu.Lock()
	s.spent.Add(s.spent, cost)
	s.spentMu.Unlock()
}

// SendAndWait submits a contract call from the AGW and blocks until the receipt
// is available. A reverted tx (status 0) returns the receipt with an error.
func (s *Sender) SendAndWait(ctx context.Context, to common.Address, data []byte, value *big.Int) (*types.Receipt, error) {
	if value == nil {
		value = big.NewInt(0)
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	tx := &zkTypes.Transaction{
		To:      &to,
		Data:    data,
		Value:   value,
		From:    &s.agw,
		ChainID: s.chainID,
	}
	hash, err := s.account.SendTransaction(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("send tx to %s: %w", to.Hex(), err)
	}
	receipt, err := s.waitReceipt(ctx, hash)
	if receipt != nil {
		s.addSpend(receipt, value) // count spend even on a reverted tx (gas is burned)
	}
	return receipt, err
}

// waitReceipt polls for the receipt until it lands or ctx is cancelled.
func (s *Sender) waitReceipt(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
	const interval = 500 * time.Millisecond
	for {
		r, err := s.eth.TransactionReceipt(ctx, hash)
		if err == nil {
			if r.Status == types.ReceiptStatusFailed {
				return r, fmt.Errorf("tx %s reverted on chain", hash.Hex())
			}
			return r, nil
		}
		if !errors.Is(err, ethereum.NotFound) {
			// transient RPC error — keep polling, surface only on ctx timeout
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("waiting for receipt %s: %w", hash.Hex(), ctx.Err())
		case <-time.After(interval):
		}
	}
}
