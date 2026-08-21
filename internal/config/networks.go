package config

import "github.com/ethereum/go-ethereum/common"

// Network is a fully-resolved chain profile. Addresses are compiled-in
// constants (see mainnetProfile / testnetProfile) so they can never drift via
// a hand-edited config file — config.json only selects which profile is active
// (ADR / TZ NFR6: no symlink network switching on Windows).
type Network struct {
	Name      string // "mainnet" | "testnet"
	ChainID   int64
	RPC       string // HTTP JSON-RPC
	WS        string // WebSocket endpoint for block subscriptions
	Explorer  string
	Contracts Contracts
}

// Contracts holds every Dealers.sh contract address for one network.
// Field names mirror the on-chain contract names (TZ §B.2/§B.3, skill.md).
type Contracts struct {
	DealersNFT            common.Address
	DealersCore           common.Address
	DealersActions        common.Address
	DealersPVE            common.Address
	DealersPVP            common.Address
	DealersHeists         common.Address
	DealersBoosts         common.Address
	DEDrugRegistry        common.Address
	DEAreaRegistry        common.Address
	DealersClaims         common.Address
	DealersMulticall      common.Address
	DealersPaymentHandler common.Address
	DealersRandomness     common.Address
	DealersChatFactory    common.Address
	DealerRendererSVG     common.Address
	DealerRendererHTML    common.Address
	// DealersBankHeist is the seasonal competition contract that also hosts the
	// daily checkIn (builds "focus"). Zero on networks where it isn't deployed —
	// the check-in feature guards against a zero address.
	DealersBankHeist common.Address
	// DealersMissions hosts daily/weekly missions: checkIn snapshots the epoch
	// baseline, claim redeems a completed mission's reward. Zero where undeployed.
	DealersMissions common.Address
}

// AppUpvote is the one-time mainnet-only application upvote (TZ §B.4).
// voteForApp(237) on this contract, gas only. Skipped on testnet.
var AppUpvote = struct {
	Contract common.Address
	AppID    int64
}{
	Contract: common.HexToAddress("0x3b50de27506f0a8c1f4122a1e6f470009a76ce2a"),
	AppID:    237,
}

// mainnetProfile — Abstract mainnet, chain 2741 (TZ §B.2).
var mainnetProfile = Network{
	Name:     "mainnet",
	ChainID:  2741,
	RPC:      "https://api.mainnet.abs.xyz",
	WS:       "wss://api.mainnet.abs.xyz/ws",
	Explorer: "https://explorer.abs.xyz",
	Contracts: Contracts{
		DealersNFT:            common.HexToAddress("0x610CcEe1AE4aFF961d043faB379491C2997383F7"),
		DealersCore:           common.HexToAddress("0x0D8d2755a49d30BD57F6a9bA5Fa8a7c9FFF86E8e"),
		DealersActions:        common.HexToAddress("0xa02bccd8Aa2b9067bf22213d25E7E73D3F6cDB6D"),
		DealersPVE:            common.HexToAddress("0x61Ee140E5757366ece5Ee89ea9688c0ea2da88e6"),
		DealersPVP:            common.HexToAddress("0x49090a745Ba1E45c9C0f9c21448Ce965b3798949"),
		DealersHeists:         common.HexToAddress("0x4B7A7E9dD2254c7848Def422cEB517AC6310C90e"),
		DealersBoosts:         common.HexToAddress("0x7cbE9cD59E6D9842b7d2EeBdd7E24836db64545B"),
		DEDrugRegistry:        common.HexToAddress("0xb89125a33eb5FD401a9ef66DECe2A6a060989CcC"),
		DEAreaRegistry:        common.HexToAddress("0xe7598E61738921967f888736A1977b80Da526510"),
		DealersClaims:         common.HexToAddress("0xdBDD44758Deb81B3D88766c6a6fc439960Ea4Ba8"),
		DealersMulticall:      common.HexToAddress("0x0728c4c42eE90909148cb39eeFFf811037D08762"), // paginated getPotentialTargets (Bank Heist V2)
		DealersPaymentHandler: common.HexToAddress("0x798E0f15A34F491eF4A69E9CC626A625bb80A504"),
		DealersRandomness:     common.HexToAddress("0x76f965BdB22f482503Cf0de3C67394d987da400D"),
		DealersChatFactory:    common.HexToAddress("0xB13A49F39eD9146A89d917b4DB4beF1c143e2FFe"),
		DealerRendererSVG:     common.HexToAddress("0x8c99b0c302E774CF50ba6B4763dcB15d84ede31A"),
		DealerRendererHTML:    common.HexToAddress("0x889F5a12DaB04b3f5bB60672FDD599be8A0949d5"),
		DealersBankHeist:      common.HexToAddress("0x987779Fd28E24D9cBeB7c22Eb1AFE1B7771ED5e1"), // V2 (heist contract == reward vault)
		DealersMissions:       common.HexToAddress("0xaf461430D2e2cCd89CFE3Ee335F77a8BF3031F5b"),
	},
}

// testnetProfile — Abstract testnet, chain 11124 (TZ §B.3). Development target.
var testnetProfile = Network{
	Name:     "testnet",
	ChainID:  11124,
	RPC:      "https://api.testnet.abs.xyz",
	WS:       "wss://api.testnet.abs.xyz/ws",
	Explorer: "https://sepolia.abscan.org",
	Contracts: Contracts{
		DealersNFT:            common.HexToAddress("0xCa4BC92b565A110952933C90f581A7765415e6Ed"),
		DealersCore:           common.HexToAddress("0x8dC006a61012F1a6f3EAd24eEfaf0e634d0635f4"),
		DealersActions:        common.HexToAddress("0x407B2507B7371834D7616E3F0A69274124119598"),
		DealersPVE:            common.HexToAddress("0x9D6dc92F71416943aB7ee2653c681dC403107149"),
		DealersPVP:            common.HexToAddress("0x73551b83d47Fd830d7571b4EB81059Ce92820F68"),
		DealersHeists:         common.HexToAddress("0x36B7941C93E7DaA916dbA0ea42841Fc54CbC2325"),
		DealersBoosts:         common.HexToAddress("0x6bD8B5D350798ad850F255Cb093D1be46Cc0B163"),
		DEDrugRegistry:        common.HexToAddress("0xe9260db65A5B62Ccc09da4100c47F66dC39b4a6B"),
		DEAreaRegistry:        common.HexToAddress("0x543513cea0Dfe318224508c5dD0E05298e776f44"),
		DealersClaims:         common.HexToAddress("0xaA6AfB0fCDB58135A12c06cD5b6D551A6507F14A"),
		DealersMulticall:      common.HexToAddress("0xF616a24AfC1C51B2514c5a7f63bcc792ac700F5b"),
		DealersPaymentHandler: common.HexToAddress("0x759C484eAb3B56757E05597A5597bfC982BEAA76"),
		DealersRandomness:     common.HexToAddress("0x92bAeD7386ec8738075eD271cb3747CbFFA175c1"),
		DealersChatFactory:    common.HexToAddress("0x46309780bdFe7Fed9075dd0BC37E55c57D5C91a7"),
		DealerRendererSVG:     common.HexToAddress("0x026fE01BC06Bc56e52cdB77BF0Aba6c119d32583"),
		DealerRendererHTML:    common.HexToAddress("0x20cdad6AEC735B2FA65Edd35d18A55127cdD6C03"),
	},
}

// profiles maps active_network → resolved profile.
var profiles = map[string]Network{
	"mainnet": mainnetProfile,
	"testnet": testnetProfile,
}
