# Dealers.sh — On-Chain Reference (Go client)

> **Provenance.** Every signature, struct, enum, event, and constant below is extracted
> verbatim from Solidity source at repo `Dealers-sh/dealers-contracts`, branch **`main`**
> (raw base `https://raw.githubusercontent.com/Dealers-sh/dealers-contracts/main/`),
> read on 2026-07-01. Nothing here comes from docs, ABIs, or inference — file + line
> citations are given for every non-obvious fact. Contracts target `pragma solidity ^0.8.28`
> on Abstract chain. `seq` values are **`uint64`**; token/heist/drug/area ids as typed below.

Corrections to the task's assumptions are flagged inline with ⚠️ and summarized in §9.

---

## 0. Contract map

| Module | Contract | Key role |
|---|---|---|
| Read aggregator | `DealersMulticall` (`src/core/DealersMulticall.sol`) | dashboard bundle reads |
| Core state | `DealersCore` (`src/core/DealersCore.sol`) | game state, `config()`, cash, infamy, heat |
| PVE hustle | `DealersPVE` (`src/core/DealersPVE.sol`) | commit/resolveGame |
| PVP | `DealersPVP` (`src/core/DealersPVP.sol`) | commit/resolveAttack |
| Heists | `DealersHeists` (`src/core/DealersHeists.sol`) | start/commit/resolveStage/cashOut/jackpot |
| Actions | `DealersActions` (`src/core/DealersActions.sol`) | travel, bail, bribe, breakout, wanted poster, cash/reset, sellDrop |
| Boosts | `DealersBoosts` (`src/core/DealersBoosts.sol`) | purchaseBoost |
| Claims | `DealersClaims` (`src/core/DealersClaims.sol`) | claimAchievement(s) |
| NFT | `DealersNFT` (`src/nft/DealersNFT.sol`) | mint/reserve, ERC721Enumerable |
| Randomness | `DealersRandomness` (`src/utils/DealersRandomness.sol`) | commit-reveal coordinator |
| Registries | `IAreaRegistry`, `IDrugRegistry` (`src/utils/…`) | area/drug economy |
| Chat | `DealersChatFactory` / `DealersChatRoom` (`src/social/…`) | postMessage |

---

## 1. State reads

### 1.1 `DealersMulticall.getFullDealerState(uint256 tokenId) → FullDealerState`
`src/core/DealersMulticall.sol:201`. Reverts `DealerNotInitialized(tokenId)` if not init.
Struct def `DealersMulticall.sol:40-75` (declaration order — this is the ABI tuple order):

| # | field | type |
|---|---|---|
|1|reputation|uint256| (= `gs.totalReputation`, incl. stash bonus) |
|2|stashBonusRep|uint256|
|3|currentArea|uint8|
|4|previousArea|uint8|
|5|heatLevel|uint8| (effective, lazy-decayed) |
|6|dailyAttemptsRemaining|uint8|
|7|maxAttempts|uint8|
|8|threat|uint8|
|9|armor|uint8|
|10|isInitialized|bool|
|11|isJailed|bool|
|12|isInSafeHouse|bool|
|13|jailChance|uint16|
|14|reputationTitle|string|
|15|cashBalance|uint256|
|16|drugBalances|`DrugBalance[]`| (nested, see below) |
|17|boostActive|bool|
|18|boostExpiry|uint64|
|19|drugMultiplier|uint8|
|20|cashMultiplier|uint8|
|21|repMultiplier|uint8|
|22|freeAreaMovement|bool|
|23|pveWins|uint32|
|24|pveLosses|uint32|
|25|pveTies|uint32|
|26|pvpAttackWins|uint32|
|27|pvpAttackLosses|uint32|
|28|pvpDefendWins|uint32|
|29|pvpDefendLosses|uint32|
|30|lastBreakoutAttempt|uint32|
|31|canBreakoutToday|bool|
|32|attacksReceivedToday|uint8|
|33|maxAttacksPerDay|uint8|
|34|infamy|uint256|

Nested `DrugBalance` (`DealersMulticall.sol:30-35`) — one per drug id, order = `drugRegistry.getAllDrugIds()`:

| field | type |
|---|---|
| drugId | uint256 |
| name | string |
| balance | uint256 |
| rarity | `IDrugRegistry.DrugRarity` (uint8 enum, §7) |

Note: fields 18-22 are **only populated when `boostActive`** (`DealersMulticall.sol:236-243`); zero otherwise.

### 1.2 Area economy
```solidity
function getAreaEconomy(uint8 areaId) external view returns (AreaEconomy)          // :275
function getAllAreas() external view returns (AreaEconomy[] economies)              // :283
```
`getAllAreas` returns `totalAreas + 3` entries: index 0 = safe house (area 0), 1..totalAreas = normal areas, then BLACK_MARKET_AREA (254), then JAIL_AREA (255) (`DealersMulticall.sol:283-296`).

`AreaEconomy` (`DealersMulticall.sol:92-102`):

| field | type |
|---|---|
| areaId | uint8 |
| areaName | string |
| movementFee | uint256 |
| minReputation | uint256 |
| isActive | bool |
| isSafeHouse | bool |
| isJail | bool |
| dealerCount | uint256 |
| drugs | `AreaDrug[]` |

Nested `AreaDrug` (`DealersMulticall.sol:80-87`):

| field | type |
|---|---|
| drugId | uint256 |
| name | string |
| rarity | `IDrugRegistry.DrugRarity` |
| buyPrice | uint256 |
| sellPrice | uint256 |
| isAvailable | bool |

### 1.3 Drug ids / prices (registries)
```solidity
// IDrugRegistry (src/utils/IDrugRegistry.sol)
getAllDrugIds() → uint256[]                                    // :78
getDrugInfo(uint256 drugId) → DrugInfo                         // :53  {string name; DrugRarity rarity; uint256 baseCashValue; bool isActive}
getDrugBaseCashValue(uint256 drugId) → uint256                 // :58
getDrugRarity(uint256 drugId) → DrugRarity                     // :63
getTotalDrugs() → uint256                                      // :73
// IAreaRegistry (src/utils/IAreaRegistry.sol)
getAreaInfo(uint8) → AreaInfo {string name; uint256 movementFee; uint256 minReputation; bool isActive; bool isSafeHouse; bool isJail}  // :59
getAreaDrugIds(uint8) → uint256[]                              // :113
getDrugPricing(uint8 areaId, uint256 drugId) → (uint256 buyPrice, uint256 sellPrice)  // :118
getAreaDrugConfig(uint8, uint256) → AreaDrugConfig {uint256 drugId; uint256 buyPrice; uint256 sellPrice; bool isAvailable}  // :108
getTotalAreas() → uint8   // :94    isBlackMarket/isJail/isSafeHouse(uint8)→bool
SAFE_HOUSE_AREA()=0, JAIL_AREA()=255, BLACK_MARKET_AREA()=254  // :156-169 (documented constants)
```

### 1.4 Active heist / getHeist (`src/core/DealersHeists.sol`, iface `IDealersHeists.sol`)
```solidity
activeHeist(uint256 tokenId) → uint256 heistId    // 0 if none    (IDealersHeists.sol:130)
getHeist(uint256 heistId) → DailyHeist                          // :137
getDealerHeistStats(uint256 tokenId) → HeistStats              // :142
heistRuns(uint256 tokenId) → uint32                            // :125
```
`DailyHeist` (`IDealersHeists.sol:54-67`):

| field | type |
|---|---|
| family | `HeistFamily` (uint8, §7) |
| difficulty | uint8 |
| currentStage | uint8 (0 = pre-stage, 1..5) |
| status | `HeistStatus` (uint8, §7) |
| ethJackpot | bool |
| jackpotFired | bool |
| entryStake | uint96 |
| currentPot | uint96 |
| commitSeq | uint64 |
| commitTimestamp | uint64 |
| lastActionTime | uint64 |
| tokenId | uint256 |

`HeistStats` (`IDealersHeists.sol:74-81`): `uint32 runs; uint32 stagesCleared; uint32 cashOuts; uint32 setbacks; uint32 busts; uint32 jackpotsWon`.

### 1.5 Core game / config state (`DealersCore`)
```solidity
core.getGameState(uint256 tokenId) → GameState                 // IDealersCore.sol:92
core.getBothGameStates(uint256 t1, uint256 t2) → (GameState, GameState)  // :97
core.config() → CoreConfig                                     // DealersCore.sol:839
core.getEffectiveHeat(uint256) → uint8   core.getInfamy(uint256) → uint256
core.getCashBalance(uint256) → uint256   core.getReputationTitle(uint256) → string
core.getDealerStats(uint256) → (uint8 threat, uint8 armor)
core.BASE_MAX_ATTEMPTS() → uint8 (=5)    core.MAX_REPUTATION (=75000)  core.MAX_INFAMY (=10000)
```
⚠️ There is **no `getFullConfigState`** anywhere in the repo. The config getter is
**`DealersCore.config()`** returning `CoreConfig` (see §8). `GameState` fields (`IDealersCore.sol:47-74`)
are the raw source the multicall derives `FullDealerState` from.

`IDealersCore.GameState` order: `currentArea u8, previousArea u8, heatLevel u8, dailyAttemptsRemaining u8, reputation u256, totalReputation u256, isInitialized bool, isJailed bool, isInSafeHouse bool, cashBalance u256, boostActive bool, boostExpiresAt u64, freeAreaMovement bool, drugMultiplier u8, repMultiplier u8, cashMultiplier u8, extraAttempts u8, jailChance u16, repWinBonus i16, repTieBonus i16, repLossPenalty i16, repCap i16, threat u8, armor u8, lastBreakoutAttempt u32, infamy u256`.

### 1.6 PVP potential targets
```solidity
getPotentialTargets(uint256 attackerId, uint256 offset, uint256 limit)
    → (PVPTarget[] targets, uint256 totalInArea)                // DealersMulticall.sol:444
calculateWinChance(uint256 attackerId, uint256 defenderId) → uint256 (25..75)  // :388
canAttack(uint256 attackerId, uint256 defenderId) → (bool canFight, uint8 reason)  // :403  reason 0=ok,1..12 blocker
canPlay(uint256 tokenId) → (bool isPlayable, uint8 reason)     // :339  reason 0=ok,1=notInit,2=jailed,3=safehouse,4=noAttempts
previewHustle(uint256 tokenId, uint256 drugId, uint256 amount)
    → (int16 winRep, int16 tieRep, int16 lossRep, uint256 cashValueOnSell, uint256 cashCostOnBuy)  // :359
```
`PVPTarget` (`DealersMulticall.sol:107-117`): `uint256 tokenId; uint256 reputation; uint8 threat; uint8 armor; uint8 attemptsRemaining; uint256 winChance; uint256 lossChance; bool canAttackNow; uint256 infamy`.

---

## 2. Commit-reveal actions

Pattern: `commit*` (only NFT owner) opens a randomness round returning/storing a `uint64 seq`;
`resolve*` (**permissionless — anyone may call**) reads `randomness.reveal(seq)`. An expired
reveal window is treated as a terminal loss/bust (see §3). Each dealer may have only one active
round per module (`RoundPending` / `HeistActive` on double-commit).

| Action | Contract | Commit fn (signature) | payable | seq path | Commit event | Resolve fn | Resolve event(s) |
|---|---|---|---|---|---|---|---|
| PVE hustle | DealersPVE | `commitGame(uint256 tokenId, uint8 choice, HustleType hustleType, uint256 drugId, uint256 amount) returns (uint64 seq)` | no | **returned** `seq` | ✅ `GameCommitted` | `resolveGame(uint64 seq)` | `GamePlayed` (win/tie/loss), `DealerArrested` (jailed), `GameExpired` (expiry) |
| PVP attack | DealersPVP | `commitAttack(uint256 attackerId, uint256 defenderId) returns (uint64 seq)` | no | **returned** `seq` | ⚠️ `PvpCommitted` | `resolveAttack(uint64 seq)` | `PVPBattleResult` (+`LootDropped`), `DealerArrested`, `PvpExpired`, `PvpAttackerJailedExternally` |
| Heist stage | DealersHeists | `commitStage(uint256 heistId)` **(returns nothing)** | no | seq **only in event** & `DailyHeist.commitSeq` | ✅ `StageCommitted` | `resolveStage(uint64 seq)` | `StageWon` / `HeistSetback`+`HeistPaid` / `HeistBusted`(+`HeistArrest`) / `HeistCashedOut`+`HeistPaid` |
| Jailbreak | DealersActions | `commitBreakout(uint256 tokenId) returns (uint64 seq)` | no | **returned** `seq` | `BreakoutCommitted` | `resolveBreakout(uint64 seq)` | `BreakoutAttempted`, `BreakoutExpired` |
| Wanted poster | DealersActions | `commitWantedPoster(uint256 tokenId) returns (uint64 seq)` | no | **returned** `seq` | `WantedPosterCommitted` | `resolveWantedPoster(uint64 seq)` | `WantedPosterRemoved`, `WantedPosterExpired` |

> ⚠️ **Task assumed `GameCommitted` (PVE) — CORRECT.** ⚠️ **Task assumed `StageCommitted` (heist) — CORRECT.**
> ⚠️ **PVP has no `AttackCommitted`/`AttackResolved`** — the events are `PvpCommitted` and `PVPBattleResult`.
> ⚠️ **`commitStage` does NOT return the seq** (return type is void); read it from the `StageCommitted`
> event or from `getHeist(heistId).commitSeq` (`DealersHeists.sol:302-321`).

### 2.1 Event field layouts (exact — for Go log decoding)

**`GameCommitted`** (`DealersPVE.sol:93-104`):
```
seq u64 indexed, tokenId u256 indexed, player address indexed,
choice u8, hustleType HustleType(u8), drugId u256, amount u256,
price u256, cashDelta int256, drugDelta int256
```
**`GamePlayed`** (`DealersPVE.sol:74-89`):
```
tokenId u256 indexed, player address indexed, playerChoice u8, houseChoice u8, outcome u8,
hustleType HustleType(u8), drugId u256, drugAmount u256, cashChange int256, reputationChange int256,
drugBalanceChange int256, newHeatLevel u8, stakedCash u256, stakedDrug u256
```
**`GameExpired`** (`DealersPVE.sol:105`): `seq u64 indexed, tokenId u256 indexed`
**`DealerArrested`** (PVE, `DealersPVE.sol:91`): `tokenId u256 indexed, player address indexed, jailChance u16`

**`PvpCommitted`** (`DealersPVP.sol:101-109`):
```
seq u64 indexed, attackerId u256 indexed, defenderId u256 indexed,
attackerThreat u8, defenderArmor u8, winChancePct u16, attackerJailChance u16
```
**`PVPBattleResult`** (`DealersPVP.sol:85-97`):
```
attacker u256 indexed, defender u256 indexed, attackerWon bool, drugIdStolen u256, drugsStolen u256,
cashStolen u256, attackerRepChange int16, defenderRepChange int16, attackerInfamyChange int16,
winChancePct u16, newHeatLevelAttacker u8
```
`LootDropped` (`:99`): `attackerId u256 indexed, drugId u256 indexed`.
`PvpExpired` (`:110`): `seq u64 indexed, attackerId u256 indexed`.
`PvpAttackerJailedExternally` (`:111`): `seq u64 indexed, attackerId u256 indexed`.
`DealerArrested` (PVP, `:113`): `tokenId u256 indexed, jailChance u16`.

**`StageCommitted`** (`IDealersHeists.sol:97`): `heistId u256 indexed, seq u64 indexed, tokenId u256 indexed, stage u8`
**`StageWon`** (`:98`): `heistId u256 indexed, tokenId u256 indexed, stage u8, pot u96`
**`HeistSetback`** (`:102`): `heistId u256 indexed, tokenId u256 indexed, stage u8, partialPot u96`
**`HeistBusted`** (`:103`): `heistId u256 indexed, tokenId u256 indexed, stage u8`
**`HeistArrest`** (`:107`): `heistId u256 indexed, tokenId u256 indexed`
**`HeistCashedOut`** (`:108`): `heistId u256 indexed, tokenId u256 indexed, pot u96`
**`HeistForceFinalized`** (`:109`): `heistId u256 indexed, tokenId u256 indexed, pot u96`
**`HeistPaid`** (`:110`): `heistId u256 indexed, tokenId u256 indexed, family HeistFamily(u8), cashPaid u256`
**`HeistStarted`** (`:87-95`): `heistId u256 indexed, tokenId u256 indexed, player address indexed, family HeistFamily(u8), difficulty u8, ethJackpot bool, cashStake u96`
Jackpot: `JackpotRolling(pythSeq u64 indexed, heistId u256 indexed, tokenId u256 indexed, stage u8)`, `JackpotWon(pythSeq u64 indexed, tokenId u256 indexed, value u256)`, `JackpotClaimed(tokenId u256 indexed, to address indexed, value u256)`, `JackpotSkipped`, `JackpotReclaimed` (`IDealersHeists.sol:112-116`).

**`BreakoutCommitted`** (`DealersActions.sol:150`): `seq u64 indexed, tokenId u256 indexed`
**`BreakoutAttempted`** (`:46`): `tokenId u256 indexed, success bool, exitArea u8`
**`BreakoutExpired`** (`:151`): `seq u64 indexed, tokenId u256 indexed`
**`WantedPosterCommitted`** (`:293`): `seq u64 indexed, tokenId u256 indexed`
**`WantedPosterRemoved`** (`:47`): `tokenId u256 indexed, success bool`
**`WantedPosterExpired`** (`:294`): `seq u64 indexed, tokenId u256 indexed`

---

## 3. Commit-reveal constants (`DealersRandomness`, `src/utils/DealersRandomness.sol`)

⚠️ Task assumed `REVEAL_OFFSET=2` and `EXPIRY=200` — **BOTH CORRECT**, and these are the real
on-chain constant names. Source `DealersRandomness.sol:25-26`:
```solidity
uint64 public constant REVEAL_OFFSET = 2;    // reveal from blockhash(commitBlock + 2)
uint64 public constant EXPIRY_WINDOW = 200;  // valid window = (rb, rb + 200] blocks
```
- `commit()` (auth-gated, `:46`): `revealBlock = block.number + 2`, returns `uint64 seq` (starts at 1).
- `reveal(seq)` **view** (`:56`): reverts `UnknownSeq` (rb==0), `TooEarly` (`block.number <= rb`),
  `Expired` (`block.number > rb + 200`). Digest = `keccak256(blockhash(rb), seq, msg.sender)` —
  **msg.sender is mixed in**, so the resolving contract must match the committing consumer.
- `isExpired(seq)` (`:64`): `rb != 0 && block.number > rb + 200`.

**NFT reveal is a separate mechanism** — `DealersNFT.REVEAL_DELAY = 2` (`DealersNFT.sol:59`),
uses a 256-block blockhash horizon with re-anchoring, unrelated to `DealersRandomness`.

---

## 4. Single-tx actions

Value semantics: fees are read live from `core.config()` unless noted; `msg.value` must be `>= fee`,
excess is refunded (`_settleMovementFee`/`_settleMarketplaceFee`, `DealersActions.sol:511-533`).
`owner()` pays 0 for reset/cash-topup.

| Action | Contract | Signature | payable | value required |
|---|---|---|---|---|
| travel | DealersActions | `travel(uint256 tokenId, uint8 destinationArea)` | ✅ | `areaRegistry.getMovementFee(dest)`; **0** if free-movement boost, first move, or entering/exiting black market (`:217-261`). Black-market entry needs `infamy >= 10` (`BLACK_MARKET_MIN_INFAMY`, `:28,244`) |
| payBail | DealersActions | `payBail(uint256 tokenId)` | ✅ | `areaRegistry.getMovementFee(JAIL_AREA)` (`:115-133`) |
| bribeCop | DealersActions | `bribeCop(uint256 tokenId)` | ✅ | `core.config().bribeCopFee` (`:267`) |
| purchaseCash | DealersActions | `purchaseCash(uint256 tokenId)` | ✅ | `config().cashTopupPrice` (0 for owner); reverts `CashBalanceTooHigh` if bal ≥ `cashPurchaseThreshold`; credits `cashTopupAmount` (`:359-372`). **No `onlyDealerOwner`** |
| purchaseAttemptReset | DealersActions | `purchaseAttemptReset(uint256 tokenId)` | ✅ | `config().attemptResetFee` (0 for owner) (`:346-353`). **No `onlyDealerOwner`** |
| sellDrop | DealersActions | `sellDrop(uint256 tokenId, uint256 drugId, uint256 amount)` | ❌ | must be in black market; pays `sellPrice*amount` $CASH (`:380-394`) |
| purchaseBoost | DealersBoosts | `purchaseBoost(uint256 dealerId, uint256 tierId)` | ✅ | `boostTiers[tierId].price` (0 for owner); only upgrade to strictly pricier tier while boosted (`DealersBoosts.sol:257-294`). Batch: `purchaseBoostBatch(uint256[] dealerIds, uint256 tierId)` payable (`:302`) |
| startHeist | DealersHeists | `startHeist(uint256 tokenId, HeistFamily family, uint8 difficulty, bool ethJackpot) returns (uint256 heistId)` | ✅ | if `ethJackpot`: `msg.value == ethAddOn` (0.001 ETH) **exactly**; else `msg.value == 0` (`DealersHeists.sol:219-247`). Debits `difficultyConfigs[difficulty].cashEntry` + 1 attempt |
| cashOut | DealersHeists | `cashOut(uint256 heistId)` | ❌ | only `REVEALED_WIN` and `currentStage >= minCashStage(=2)` (`:398-404`) |
| abandonHeist | DealersHeists | `abandonHeist(uint256 heistId)` | ❌ | only `PRE_STAGE`; full $CASH refund, ETH add-on + attempt forfeit (`:282-291`) |
| forceFinalize | DealersHeists | `forceFinalize(uint256 heistId)` | ❌ | permissionless after `IDLE_TIMEOUT` (24h) (`:410-416`) |
| claimJackpot | DealersHeists | `claimJackpot(uint256 tokenId)` | ❌ | pays `jackpotOwed[tokenId]` ETH to current NFT owner; reverts `NothingToClaim` if 0 (`:484-492`) |
| claimAchievement | DealersClaims | `claimAchievement(uint256 tokenId, uint256 achievementId)` | ❌ | owner-only; threshold checked on-chain (`DealersClaims.sol:150`) |
| claimAchievements | DealersClaims | `claimAchievements(uint256 tokenId, uint256[] calldata achievementIds)` | ❌ | batch of above (`:156`) |
| postMessage | DealersChatFactory | `postMessage(bytes32 _roomKey, uint16 tokenId, string calldata text)` | ❌ | owner of `tokenId`; text 1..256 bytes; `cooldown`(=30s) between posts; optional gate (`DealersChatFactory.sol:179-202`) |

> ⚠️ `postMessage` takes **`uint16 tokenId`** (not uint256) and a **`bytes32 _roomKey`**.
> The player-facing entrypoint is on **`DealersChatFactory`**; `DealersChatRoom.postMessage(uint16,string)`
> is factory-only (`DealersChatRoom.sol:33-34`). Room emits `MessagePosted(uint16 tokenId, uint40 timestamp, string text)`;
> factory emits `MessageRouted(bytes32 indexed roomKey, uint16 indexed tokenId)`.

---

## 5. DealersNFT (`src/nft/DealersNFT.sol`)

### 5.1 ERC721Enumerable — ✅ YES [TODO-5]
`contract DealersNFT is ERC721Enumerable, ReentrancyGuard, Ownable, IERC2981` (`:44`).
Inherits OZ `ERC721Enumerable`, so **`totalSupply()`, `tokenByIndex(uint256)`, and
`tokenOfOwnerByIndex(address,uint256)` are all available** (used internally at `:757,780,812`).
Convenience wrappers: `tokensOfOwner(address) → uint256[]` (`:807`), `pendingTokensOf(address) → uint256[]`
(unrevealed only, `:775`). `supportsInterface` also advertises `IERC2981` (`:845`).

### 5.2 Mint / recruit [TODO-2]
```solidity
function mint(address dest, uint256 count) external payable                        // :230  public mint
function reserve(uint256 nftAmount) external onlyOwner                             // :186
function reserveTo(uint256 nftAmount, address recipient) external onlyOwner        // :195
function reserveToMany(uint256 nftAmount, address[] recipients) external onlyOwner // :209
```
- **Price:** `msg.value >= mintPrice * count`, `mintPrice` default **0.01 ether** (`:100`), excess refunded (`:244`).
- **Per-wallet cap:** `MAX_PER_WALLET = 10` (`:54`); enforced on `mint` via
  `checkAndUpdateBuyerMintCount` on cumulative `mintCount[msg.sender]` (`:171-176,235`).
  `reserve*` are owner-only and skip the per-wallet cap.
- **Supply cap:** `MAX_SUPPLY = 10000` (`:52`), `checkAndUpdateTotalMinted` (`:164-169`).
  (Note: metadata/description strings say "8,888" but the enforced cap is 10000.)
- **Gating:** `mint` requires `mintOpen == true` and not `paused` (`:238`).
- Mint = commit: token minted + `initializeDealer` runs immediately; artwork bound later via
  `resolve(uint256 tokenId)` / `resolveMany(uint256[])` (permissionless, `:288,302`).
  `ROYALTY_PERCENTAGE = 500` (5%, `:53`). `getMintConfig()` returns `(bool open, uint256 price, uint256 maxPerWallet, uint256 currentSupply, uint256 maxSupply)` (`:749`).

---

## 6. Deal / Threaten / Bail matchup [TODO-1]

**One-line:** it is **not** true rock-paper-scissors between the three named moves — the outcome is
a biased house roll independent of *which* move you pick; the house's shown "choice" is derived from
your move so the UI can render RPS, where **WIN = house plays `(yourChoice+1)%3`, TIE = house plays
your move, LOSS = house plays `(yourChoice+2)%3`**.

Odds are fixed by `tieChance`/`winChance` (defaults **50 / 25**, loss = 25), *not* by move choice.
Source `DealersPVE._calculateBiasedHouseChoice` (`DealersPVE.sol:390-405`):
```solidity
function _calculateBiasedHouseChoice(uint8 roll, uint8 playerChoice)
    internal view returns (uint8 houseChoice, uint8 outcome) {
    if (roll < tieChance) {                       // roll 0..49  → TIE
        houseChoice = playerChoice;    outcome = 1;              // TIE
    } else if (roll < tieChance + winChance) {    // roll 50..74 → WIN
        houseChoice = (playerChoice + 1) % 3;   outcome = 0;     // WIN
    } else {                                      // roll 75..99 → LOSS
        houseChoice = (playerChoice + 2) % 3;   outcome = 2;     // LOSS
    }
}
```
`roll = uint8(outcomeRng % 100)` where `outcomeRng = (rand >> 16) & 0xFFFF` (`DealersPVE.sol:286,297`).
outcome enum: **0=WIN, 1=TIE, 2=LOSS** (matches `IDealersPVE.GameOutcome`, §7). Payout branching by
BUY/SELL and outcome is in `_computeBuyOutcome`/`_computeSellOutcome` (`:407-457`): WIN keeps stake +
gains; TIE loses stake but gains goods; LOSS loses stake, no goods. A jail check runs first every
resolve (`rollJailCheck` on `arrestRng = rand & 0xFFFF`, `:285,289`); on hit it emits `DealerArrested`
and skips the RPS outcome. Rep is stake-scaled and capped (`_calculateScaledRep`, `:483-503`).
`repStakeDivisor` default **50** (`:38`); `stakeDivisorSlopeBps=2500`, `stakeHeadroomBps=10000` (`:45-46`).

---

## 7. Enums (member order → integer value)

| Enum | Source | Members (value = index) |
|---|---|---|
| PVE `GameChoice` | `IDealersPVE.sol:9-13` | `DEAL=0, THREATEN=1, BAIL=2` (validated `choice <= 2`, `DealersPVE.sol:172`) |
| PVE `GameOutcome` | `IDealersPVE.sol:14-18` | `WIN=0, TIE=1, LOSS=2` |
| PVE `HustleType` | `IDealersPVE.sol:19-22` | `BUY=0, SELL=1` |
| Heist `HeistFamily` | `IDealersHeists.sol:17-20` | `SUPPLY=0, CASH=1` |
| Heist `HeistStatus` | `IDealersHeists.sol:22-31` | `NONE=0, PRE_STAGE=1, COMMITTED=2, REVEALED_WIN=3, BUSTED=4, CASHED_OUT=5, ABANDONED=6, SETBACK=7` |
| Heist difficulty | `DealersHeists.sol:59,177-179` | plain `uint8` 0/1/2 (no enum); `DIFFICULTIES=3`. Defaults: `0` repGate 600/stake 600, `1` repGate 1500/stake 4000, `2` repGate 5500/stake 25000 |
| `DrugRarity` | `IDrugRegistry.sol:18-23` | `COMMON=0, UNCOMMON=1, RARE=2, LEGENDARY=3` |
| Claims `ConditionType` | `DealersClaims.sol:35-57` | `NONE=0, PVE_WINS=1, PVE_LOSSES=2, PVE_TIES=3, PVE_TOTAL=4, PVP_ATTACK_WINS=5, PVP_DEFEND_WINS=6, PVP_TOTAL_WINS=7, REPUTATION=8, CASH_BALANCE=9, DRUG_BALANCE=10, PVE_DEAL_CHOICES=11, PVE_THREATEN_CHOICES=12, PVE_BAIL_CHOICES=13, HEIST_RUNS=14, HEIST_STAGES_CLEARED=15, HEIST_CASHOUTS=16, HEIST_SETBACKS=17, HEIST_BUSTS=18, HEIST_JACKPOTS_WON=19` |
| Claims `RewardType` | `DealersClaims.sol:28-33` | `REPUTATION=0, CASH=1, DRUG=2, ATTEMPTS=3` |

---

## 8. Config snapshot [TODO-6]

### 8.1 Read from `DealersCore.config() → CoreConfig` (`DealersCore.sol:78-91`; defaults `:200-209`)

| field | type | default |
|---|---|---|
| attemptResetFee | uint256 | 0.001 ether |
| bribeCopFee | uint256 | 0.001 ether |
| cashTopupPrice | uint256 | 0.001 ether |
| cashTopupAmount | uint256 | 100 |
| cashPurchaseThreshold | uint256 | 10 |
| jailRepPenaltyPercent | uint8 | 10 |
| jailRepPenaltyCap | uint256 | 50 |
| wantedPosterSuccessChance | uint8 | 50 |
| breakoutSuccessChance | uint8 | 50 |
| jailDrugConfiscationPercent | uint8 | 3 |
| starterCash | uint256 | 250 |
| jailChancePerHeat | uint16 | 5 (per-mille: 0.5%/heat) |

> **Bail cost is NOT in CoreConfig** — it is `areaRegistry.getMovementFee(JAIL_AREA())` (`DealersActions.sol:119`).
> **Clear-heat cost** = `bribeCopFee` (instant, ETH) or free-but-attempt-costing wanted poster (`wantedPosterSuccessChance`).

### 8.2 Read from module storage (not CoreConfig)
- **PVE** (`DealersPVE`, public vars): `repStakeDivisor` **50**, `tieChance` **50**, `winChance` **25**,
  `stakeDivisorSlopeBps` **2500**, `stakeHeadroomBps` **10000** (`DealersPVE.sol:35-46`). ⚠️ There is
  **no `repMultiplier` config** — `repMultiplier` is a per-dealer boost field (default 100 in Core,
  `DealersCore.sol:361`), surfaced in `GameState`/`FullDealerState`.
- **PVP** (`DealersPVP.config() → PVPConfig`, defaults `DealersPVP.sol:158-172`): `minReputation 200,
  baseWinChance 50, minWinChance 25, maxWinChance 75, maxAttacksPerDay 3, drugStealPercent 2,
  cashStealPercent 2, rarityWeightCommon 75, rarityWeightUncommon 20, rarityWeightRare 5,
  repRangePercent 25, defenderRepBonus 2, repRangeThreshold 22000`.
- **Boosts** (`DealersBoosts.getBoostTier(tierId) → BoostTier {uint256 price; uint64 duration;
  uint8 drugMultiplier; uint8 repMultiplier; uint8 extraAttempts; bool freeAreaMovement;
  uint8 cashMultiplier; bool isActive}`, `DealersBoosts.sol:38-47`).

### 8.3 Hardcoded constants (immutable in source)

| Constant | Value | Source |
|---|---|---|
| `DealersRandomness.REVEAL_OFFSET` | 2 | DealersRandomness.sol:25 |
| `DealersRandomness.EXPIRY_WINDOW` | 200 | :26 |
| `DealersCore.BASE_MAX_ATTEMPTS` | 5 | DealersCore.sol:33 |
| `DealersCore.MAX_REPUTATION` | 75000 | :109 |
| `DealersCore.MAX_INFAMY` | 10000 | :110 |
| `DealersCore.DECAY_GRACE_PERIOD` | 7 days | :53 |
| `DealersCore.DECAY_RATE_PER_DAY` | 1 | :54 |
| `DealersCore.INFAMY_DECAY_MULTIPLIER` | 2 | :55 |
| `DealersActions.BLACK_MARKET_MIN_INFAMY` | 10 | DealersActions.sol:28 |
| `DealersHeists.IDLE_TIMEOUT` | 24 hours | DealersHeists.sol:51 |
| `DealersHeists.JACKPOT_TIMEOUT` | 24 hours | :56 |
| `DealersHeists.STAGES` / `DIFFICULTIES` | 5 / 3 | :58-59 |
| `DealersHeists.ethAddOn` (mutable) | 0.001 ether | :71 |
| `DealersHeists.minCashStage` (mutable) | 2 | :74 |
| `DealersHeists.bustRepPenalty` (mutable) | 3 | :74 |
| `DealersHeists.jackpotReserveBps` (mutable) | 6000 | :72 |
| `DealersNFT.MAX_SUPPLY` | 10000 | DealersNFT.sol:52 |
| `DealersNFT.MAX_PER_WALLET` | 10 | :54 |
| `DealersNFT.ROYALTY_PERCENTAGE` | 500 (5%) | :53 |
| `DealersNFT.REVEAL_DELAY` | 2 | :59 |
| `DealersNFT.mintPrice` (mutable) | 0.01 ether | :100 |
| `DealersChatFactory.MAX_MESSAGE_LENGTH` | 256 | DealersChatFactory.sol:73 |
| `DealersChatFactory.cooldown` (mutable) | 30 (s) | :85 |
| `DealersChatRoom` buffer | 64 messages | DealersChatRoom.sol:18 |

Heist stage tables (mutable, defaults `DealersHeists.sol:155-173`): `stageWinOdds [72,62,52,42,32]`,
`stageSetbackOdds [20,28,33,38,40]` (bust = 100−clean−setback = [8,10,15,20,28]),
`stageSetbackKeepBps [5000,4500,4000,3500,3000]`, `stagePotMinBps [10000,18000,30000,52000,100000]`,
`stagePotMaxBps [14000,28000,46000,78000,160000]`, `stageRepReward [0,6,12,21,36]`.

---

## 9. Summary of corrections to prior assumptions

| Assumption | Verdict | Reality |
|---|---|---|
| PVE commit event = `GameCommitted` | ✅ correct | `DealersPVE.sol:93` |
| Heist commit event = `StageCommitted` | ✅ correct | `IDealersHeists.sol:97` |
| `REVEAL_OFFSET = 2` | ✅ correct | `DealersRandomness.sol:25` |
| `EXPIRY = 200` | ✅ correct | `EXPIRY_WINDOW = 200`, `DealersRandomness.sol:26` |
| PVP events `AttackCommitted`/`AttackResolved` | ❌ wrong | actual: `PvpCommitted` / `PVPBattleResult` (+`PvpExpired`, `PvpAttackerJailedExternally`) |
| `commitStage` returns seq | ❌ wrong | returns void; seq via `StageCommitted` event or `getHeist().commitSeq` |
| `getFullConfigState` getter | ❌ wrong | no such fn; use `DealersCore.config()` → `CoreConfig` |
| `repMultiplier` is a config value | ❌ wrong | it is a per-dealer boost field (default 100), not in any config struct |
| Deal/Threaten/Bail is true RPS | ❌ nuance | outcome is a fixed biased roll (tie 50 / win 25 / loss 25) independent of the move; house "choice" is derived from yours |
| `postMessage(uint256 tokenId,...)` | ❌ wrong | `DealersChatFactory.postMessage(bytes32 roomKey, uint16 tokenId, string text)` |
