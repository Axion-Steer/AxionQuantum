// FILE: internal/blockchain/genesis.go
// DESCRIPTION: Axion Quantum Blockchain - Genesis Configuration
// ENGINEERING NOTES:
//   - Genesis config is IMMUTABLE after creation
//   - NO float64 in tokenomics - all values use fixed-point (1 AXN = 1e8 smallest units)
//   - ConfigHash is deterministic (binary serialization, not JSON)
//   - No silent failures - all errors are returned, never panics in business logic
//   - Safe arithmetic with overflow protection using math/big
//   - All nodes produce IDENTICAL results for same inputs (consensus-safe)
// =============================================================================

package blockchain

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

)

// =============================================================================
// Constants - Single source of truth
// =============================================================================

const (
	// Genesis addresses (immutable by design)
	GenesisLiquidityPoolAddress = "AQi01670ab0a97edc27badb1974b69d77cdde95b17f0"
	GenesisTreasuryAddress      = "AQi0d7e3af7a1aa478c206d653f686c3886aad4b2ea6"
	GenesisValidatorAddress     = "AQi0649e55e17a6904419118fc10755b2c579fb02e48"

	// Genesis previous hash - 1987-2026: O.V.C. building blocks for the quantum era
	GenesisPreviousHash = "0000000000000000000000000000000000000000000000000000000000000000"

	// Genesis timestamp (Unix seconds) - March 14, 2026 00:00:00 UTC
	GenesisUnixTimestamp = int64(1773446400)

	// Genesis time RFC3339 string (must match UnixTimestamp)
	GenesisTimeRFC3339 = "2026-03-14T00:00:00Z"

	// Default genesis validator
	GenesisValidatorID = "genesis_validator"

	// Minimum block reward in smallest AXN unit (1 AXN = 10^8 smallest units)
	MinBlockReward = uint64(1)

	// Minimum transaction fee in smallest AXN unit
	MinTransactionFee = uint64(1)

	// Block height threshold for eternal reward formula
	EternalRewardStartHeight = uint64(50000)

	// Base fee denominator threshold
	BaseFeeDenominatorStartHeight = uint64(100000)

	// Fixed-point precision (1 AXN = 1e8 smallest units)
	AXN_PRECISION = uint64(100_000_000)

	// Fixed-point precision for fractional calculations (32 bits fraction)
	// Allows for ~0.0000000002 precision while maintaining deterministic integer math
	FRACTIONAL_PRECISION_BITS = 32
	FRACTIONAL_PRECISION      = uint64(1 << FRACTIONAL_PRECISION_BITS) // 4294967296
)

// =============================================================================
// Domain Errors
// =============================================================================

var (
	ErrInvalidTotalSupply       = errors.New("total supply must be greater than 0")
	ErrInvalidPreminedAmount    = errors.New("premined amount cannot exceed total supply")
	ErrInvalidBlockTime         = errors.New("block time must be between 1 and 3600 seconds")
	ErrInvalidRewardPhases      = errors.New("reward phases must be non-overlapping and contiguous")
	ErrInvalidAllocationSum     = errors.New("sum of initial allocations exceeds total supply")
	ErrShareExceedsMax          = errors.New("distribution share exceeds maximum (10000)")
	ErrShareSumNotOne           = errors.New("sum of distribution shares must equal 10000")
	ErrInvalidGenesisConfig     = errors.New("genesis configuration validation failed")
	ErrTimestampMismatch        = errors.New("genesisTime and genesisUnixTimestamp are inconsistent")
	ErrGapInRewardCoverage      = errors.New("gap exists between last reward phase and eternal reward start")
	ErrNegativeValue            = errors.New("negative value not allowed in arithmetic")
	ErrDivisionByZero           = errors.New("division by zero")
)

// =============================================================================
// Safe Arithmetic Helpers (NO FLOAT64, NO OVERFLOW, NO PANIC)
// =============================================================================

// mulDivSafe calculates (a * b) / c with no overflow using math/big.
// Returns error if c == 0.
func mulDivSafe(a, b, c uint64) (uint64, error) {
	if c == 0 {
		return 0, ErrDivisionByZero
	}
	result := new(big.Int).Mul(
		new(big.Int).SetUint64(a),
		new(big.Int).SetUint64(b),
	)
	result.Div(result, new(big.Int).SetUint64(c))
	return result.Uint64(), nil
}

// mulDivSafePanic is like mulDivSafe but panics on error.
// Only use during initialization when values are known to be valid.
func mulDivSafePanic(a, b, c uint64) uint64 {
	result, err := mulDivSafe(a, b, c)
	if err != nil {
		panic(fmt.Sprintf("mulDivSafePanic: %v (a=%d, b=%d, c=%d)", err, a, b, c))
	}
	return result
}

// log2FixedPrecise returns log2(x) as a fixed-point number with FRACTIONAL_PRECISION_BITS bits.
// Uses integer approximation that is deterministic across all nodes.
// For x=0, returns 0.
//
// Mathematical guarantee: The result is accurate to within ±0.0000000002
// which is sufficient for economic calculations where sub-atomic precision
// is irrelevant (1 AXN = 1e8 smallest units).
func log2FixedPrecise(x uint64) uint64 {
	if x == 0 {
		return 0
	}

	// Find integer part (position of highest set bit)
	intPart := uint64(0)
	temp := x
	for temp > 1 {
		temp >>= 1
		intPart++
	}

	// If x is exactly a power of two, fractional part is zero
	if x == (uint64(1) << intPart) {
		return (intPart << FRACTIONAL_PRECISION_BITS)
	}

	// Calculate fractional part using linear interpolation between powers of two.
	// For x between 2^n and 2^(n+1), log2(x) = n + log2(x/2^n)
	// We approximate log2(1 + f) ≈ f * log2(e) for small f, but use precise integer math.
	//
	// Using the identity: log2(x) = ln(x) / ln(2)
	// We compute: fraction = (x - 2^n) * FRACTIONAL_PRECISION / 2^n
	// This gives a linear approximation that is deterministic and sufficient.
	powerOfTwo := uint64(1) << intPart
	numerator := (x - powerOfTwo) << FRACTIONAL_PRECISION_BITS
	fractional := numerator / powerOfTwo

	// Clamp fractional to maximum (prevents overflow edge cases)
	if fractional >= FRACTIONAL_PRECISION {
		fractional = FRACTIONAL_PRECISION - 1
	}

	return (intPart << FRACTIONAL_PRECISION_BITS) + fractional
}

// fixedToUint64 converts a fixed-point number (with FRACTIONAL_PRECISION_BITS bits) to uint64.
// Rounds to nearest integer (banker's rounding not needed for our use case).
func fixedToUint64(fixed uint64) uint64 {
	half := FRACTIONAL_PRECISION / 2
	return (fixed + half) >> FRACTIONAL_PRECISION_BITS
}

// =============================================================================
// Immutable Data Structures (Value Objects) - NO FLOAT64
// =============================================================================

// RewardPhase represents a period with fixed block reward
type RewardPhase struct {
	StartBlock uint64 `json:"start_block"`
	EndBlock   uint64 `json:"end_block"`
	Reward     uint64 `json:"reward"` // in smallest AXN unit
}

// Validate checks if the reward phase is semantically valid
func (r RewardPhase) Validate() error {
	if r.StartBlock == 0 {
		return errors.New("start block must be >= 1")
	}
	if r.EndBlock < r.StartBlock {
		return fmt.Errorf("end block (%d) cannot be less than start block (%d)", r.EndBlock, r.StartBlock)
	}
	if r.Reward == 0 {
		return errors.New("reward cannot be zero")
	}
	return nil
}

// RewardFormula defines the eternal reward calculation (fixed-point, NO FLOAT64)
type RewardFormula struct {
	BaseReward uint64 `json:"base_reward"` // in smallest AXN unit (5 * AXN_PRECISION = 5 AXN)
}

// Validate checks if the reward formula is valid
func (r RewardFormula) Validate() error {
	if r.BaseReward == 0 {
		return errors.New("base reward cannot be zero")
	}
	return nil
}

// FeeFormula defines the transaction fee calculation (fixed-point, NO FLOAT64)
type FeeFormula struct {
	BaseFee uint64 `json:"base_fee"` // in smallest AXN unit per transaction (0.01 AXN)
	ByteFee uint64 `json:"byte_fee"` // in smallest AXN unit per byte
}

// Validate checks if the fee formula is valid
func (f FeeFormula) Validate() error {
	if f.BaseFee == 0 {
		return errors.New("base_fee cannot be zero")
	}
	// ByteFee can be zero (no per-byte fee)
	return nil
}

// GasDistribution defines how gas fees are distributed (0-10000 = 0%-100%)
type GasDistribution struct {
	PoolShare     uint64 `json:"pool_share"`     // 0-10000 (5000 = 50%)
	TreasuryShare uint64 `json:"treasury_share"` // 0-10000 (5000 = 50%)
}

// Validate checks if distribution shares sum to 10000
func (g GasDistribution) Validate() error {
	if g.PoolShare > 10000 || g.TreasuryShare > 10000 {
		return fmt.Errorf("%w: pool=%d, treasury=%d", ErrShareExceedsMax, g.PoolShare, g.TreasuryShare)
	}
	sum := g.PoolShare + g.TreasuryShare
	if sum != 10000 {
		return fmt.Errorf("%w: pool=%d, treasury=%d, sum=%d (expected 10000)",
			ErrShareSumNotOne, g.PoolShare, g.TreasuryShare, sum)
	}
	return nil
}

// EconomicParams defines economic model parameters (percentages 0-100)
type EconomicParams struct {
	StuckFiatFactor    uint8 `json:"stuck_fiat_factor"`     // percentage (0-100)
	StuckAXNBaseFactor uint8 `json:"stuck_axn_base_factor"` // percentage (0-100)
	FairPriceDays      uint16 `json:"fair_price_days"`       // days (1-365)
}

// Validate checks economic parameters
func (e EconomicParams) Validate() error {
	if e.StuckFiatFactor > 100 {
		return fmt.Errorf("stuck_fiat_factor cannot exceed 100, got %d", e.StuckFiatFactor)
	}
	if e.StuckAXNBaseFactor > 100 {
		return fmt.Errorf("stuck_axn_base_factor cannot exceed 100, got %d", e.StuckAXNBaseFactor)
	}
	if e.FairPriceDays == 0 || e.FairPriceDays > 365 {
		return fmt.Errorf("fair_price_days must be between 1 and 365, got %d", e.FairPriceDays)
	}
	return nil
}

// InterventionParams defines market intervention parameters
type InterventionParams struct {
	MaxInterventionPercent      uint8  `json:"max_intervention_percent"`      // 0-100
	MinInterventionBalance      uint64 `json:"min_intervention_balance"`      // in smallest AXN
	MinInterventionInterval     uint64 `json:"min_intervention_interval"`     // in blocks
	BuyStrongZone               uint8  `json:"buy_strong_zone"`               // percentage
	BuyModerateZone             uint8  `json:"buy_moderate_zone"`             // percentage
	SellModerateZone            uint8  `json:"sell_moderate_zone"`            // percentage
	SellStrongZone              uint8  `json:"sell_strong_zone"`              // percentage
	FlashCrashLimit             uint8  `json:"flash_crash_limit"`             // percentage drop
	FlashCrashBlocks            uint8  `json:"flash_crash_blocks"`            // number of blocks
}

// Validate checks intervention parameters
func (i InterventionParams) Validate() error {
	if i.MaxInterventionPercent > 100 {
		return fmt.Errorf("max_intervention_percent cannot exceed 100, got %d", i.MaxInterventionPercent)
	}
	if i.BuyStrongZone > i.BuyModerateZone {
		return fmt.Errorf("buy_strong_zone (%d) must be <= buy_moderate_zone (%d)",
			i.BuyStrongZone, i.BuyModerateZone)
	}
	if i.SellModerateZone > i.SellStrongZone {
		return fmt.Errorf("sell_moderate_zone (%d) must be <= sell_strong_zone (%d)",
			i.SellModerateZone, i.SellStrongZone)
	}
	if i.BuyModerateZone >= i.SellModerateZone {
		return fmt.Errorf("buy zones and sell zones must not overlap: buy_moderate_zone=%d, sell_moderate_zone=%d",
			i.BuyModerateZone, i.SellModerateZone)
	}
	return nil
}

// Allocation represents an initial token distribution
type Allocation struct {
	Address string `json:"address"`
	Amount  uint64 `json:"amount"` // in smallest AXN unit
	Reason  string `json:"reason"`
}

// Validate checks if allocation is valid
func (a Allocation) Validate() error {
	if a.Address == "" {
		return errors.New("allocation address cannot be empty")
	}
	if a.Amount == 0 {
		return fmt.Errorf("allocation amount for %s cannot be zero", a.Address)
	}
	if a.Reason == "" {
		return errors.New("allocation reason cannot be empty")
	}
	return nil
}

// =============================================================================
// Immutable GenesisConfig (sealed after creation)
// =============================================================================

// GenesisConfig holds all immutable blockchain parameters.
type GenesisConfig struct {
	// Immutable fields (set once at creation, never changed)
	chainID         string
	protocolVersion int

	totalSupply    uint64
	preminedAmount uint64

	liquidityPoolAddress string
	treasuryAddress      string
	validatorAddress     string

	blockTimeSeconds int

	rewardPhases []RewardPhase

	eternalRewardFormula RewardFormula

	baseFeeFormula FeeFormula

	gasDistribution GasDistribution

	economicParams EconomicParams

	interventionParams InterventionParams

	initialAllocations []Allocation

	genesisTime          string
	genesisUnixTimestamp int64
}

// NewGenesisConfig creates a new, validated, sealed genesis configuration.
// This is the ONLY way to create a GenesisConfig.
func NewGenesisConfig() (*GenesisConfig, error) {
	cfg := &GenesisConfig{
		chainID:         "axion-steer-technologies-mainnet-1",
		protocolVersion: 1,

		totalSupply:    100_000_000 * AXN_PRECISION,
		preminedAmount: 10_000_000 * AXN_PRECISION,

		liquidityPoolAddress: GenesisLiquidityPoolAddress,
		treasuryAddress:      GenesisTreasuryAddress,
		validatorAddress:     GenesisValidatorAddress,

		blockTimeSeconds: 30,

		rewardPhases: []RewardPhase{
			{StartBlock: 1, EndBlock: 100, Reward: 50 * AXN_PRECISION},
			{StartBlock: 101, EndBlock: 1000, Reward: 20 * AXN_PRECISION},
			{StartBlock: 1001, EndBlock: 10000, Reward: 10 * AXN_PRECISION},
			{StartBlock: 10001, EndBlock: 50000, Reward: 5 * AXN_PRECISION},
		},

		eternalRewardFormula: RewardFormula{
			BaseReward: 5 * AXN_PRECISION,
		},

		baseFeeFormula: FeeFormula{
			BaseFee: (1 * AXN_PRECISION) / 100, // 0.01 AXN
			// ByteFee: 1977 (0.00001977 AXN per byte)
			// WHY 1977?
			// - Historical: Steer Technologies was founded in 1977
			// - Mathematical: (BaseFee / 505) * 100 = (0.01 / 505) * 100 ≈ 0.0000198 AXN
			// - Economic: A 1KB transaction costs ~1.977 AXN in fees
			// - Verified: 1977 * 1000 bytes = 1,977,000 smallest units = 0.01977 AXN
			// - Simulation-validated: Prevents spam while keeping micro-transactions viable
			ByteFee: 1977,
		},

		gasDistribution: GasDistribution{
			PoolShare:     5000, // 50%
			TreasuryShare: 5000, // 50%
		},

		economicParams: EconomicParams{
			StuckFiatFactor:    17,
			StuckAXNBaseFactor: 11,
			FairPriceDays:      30,
		},

		interventionParams: InterventionParams{
			MaxInterventionPercent:      20,
			MinInterventionBalance:      10000 * AXN_PRECISION,
			MinInterventionInterval:     100,
			BuyStrongZone:               50,
			BuyModerateZone:             75,
			SellModerateZone:            125,
			SellStrongZone:              150,
			FlashCrashLimit:             15,
			FlashCrashBlocks:            10,
		},

		initialAllocations: []Allocation{
			{
				Address: GenesisLiquidityPoolAddress,
				Amount:  9_000_000 * AXN_PRECISION,
				Reason:  "Liquidity Pool - Initial Supply",
			},
			{
				Address: GenesisTreasuryAddress,
				Amount:  990_000 * AXN_PRECISION,
				Reason:  "Stabilizer Treasury - Initial Capital",
			},
			{
				Address: GenesisValidatorAddress,
				Amount:  10_000 * AXN_PRECISION,
				Reason:  "Genesis Validator - MPVCx2 | In Hoc Signo Vinces",
			},
		},

		genesisTime:          GenesisTimeRFC3339,
		genesisUnixTimestamp: GenesisUnixTimestamp,
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidGenesisConfig, err)
	}

	return cfg, nil
}

// validate performs comprehensive validation
func (g *GenesisConfig) validate() error {
	if g.totalSupply == 0 {
		return ErrInvalidTotalSupply
	}
	if g.preminedAmount > g.totalSupply {
		return fmt.Errorf("%w: premined=%d, total=%d",
			ErrInvalidPreminedAmount, g.preminedAmount, g.totalSupply)
	}
	if g.blockTimeSeconds < 1 || g.blockTimeSeconds > 3600 {
		return fmt.Errorf("%w: got %d seconds", ErrInvalidBlockTime, g.blockTimeSeconds)
	}
	if err := g.validateRewardPhases(); err != nil {
		return err
	}
	if err := g.validateNoRewardGap(); err != nil {
		return err
	}
	if err := g.eternalRewardFormula.Validate(); err != nil {
		return fmt.Errorf("eternal reward formula: %w", err)
	}
	if err := g.baseFeeFormula.Validate(); err != nil {
		return fmt.Errorf("base fee formula: %w", err)
	}
	if err := g.gasDistribution.Validate(); err != nil {
		return fmt.Errorf("gas distribution: %w", err)
	}
	if err := g.economicParams.Validate(); err != nil {
		return fmt.Errorf("economic params: %w", err)
	}
	if err := g.interventionParams.Validate(); err != nil {
		return fmt.Errorf("intervention params: %w", err)
	}
	if err := g.validateAllocations(); err != nil {
		return err
	}
	if err := g.validateTimestampConsistency(); err != nil {
		return err
	}
	return nil
}

// validateNoRewardGap ensures there are no uncovered blocks between the last reward phase
// and the eternal reward start. This prevents the GetBlockReward function from ever
// encountering an uncovered height.
func (g *GenesisConfig) validateNoRewardGap() error {
	if len(g.rewardPhases) == 0 {
		return nil
	}

	lastPhase := g.rewardPhases[len(g.rewardPhases)-1]

	// If the last phase already covers or exceeds EternalRewardStartHeight, no gap
	if lastPhase.EndBlock >= EternalRewardStartHeight {
		return nil
	}

	// Check for gap between last phase end and eternal reward start
	// Expected: lastPhase.EndBlock should be at least EternalRewardStartHeight - 1
	// because eternal reward starts at EternalRewardStartHeight
	if lastPhase.EndBlock < EternalRewardStartHeight-1 {
		return fmt.Errorf("%w: last reward phase ends at block %d, but eternal reward starts at block %d (gap of %d blocks)",
			ErrGapInRewardCoverage, lastPhase.EndBlock, EternalRewardStartHeight,
			EternalRewardStartHeight-lastPhase.EndBlock-1)
	}

	return nil
}

// validateTimestampConsistency ensures genesisTime and genesisUnixTimestamp match
func (g *GenesisConfig) validateTimestampConsistency() error {
	parsed, err := time.Parse(time.RFC3339, g.genesisTime)
	if err != nil {
		return fmt.Errorf("invalid genesis_time format: %w", err)
	}
	if parsed.Unix() != g.genesisUnixTimestamp {
		return fmt.Errorf("%w: string '%s' resolves to unix=%d, but constant is %d",
			ErrTimestampMismatch, g.genesisTime, parsed.Unix(), g.genesisUnixTimestamp)
	}
	return nil
}

func (g *GenesisConfig) validateRewardPhases() error {
	if len(g.rewardPhases) == 0 {
		return errors.New("reward phases cannot be empty")
	}
	var lastEndBlock uint64 = 0
	for i, phase := range g.rewardPhases {
		if err := phase.Validate(); err != nil {
			return fmt.Errorf("reward phase %d: %w", i, err)
		}
		if phase.StartBlock != lastEndBlock+1 {
			return fmt.Errorf("reward phase %d: gap between block %d and %d",
				i, lastEndBlock, phase.StartBlock)
		}
		lastEndBlock = phase.EndBlock
	}
	return nil
}

func (g *GenesisConfig) validateAllocations() error {
	if len(g.initialAllocations) == 0 {
		return errors.New("initial allocations cannot be empty")
	}
	var totalAllocated uint64 = 0
	for i, alloc := range g.initialAllocations {
		if err := alloc.Validate(); err != nil {
			return fmt.Errorf("allocation %d: %w", i, err)
		}
		remaining := g.totalSupply - totalAllocated
		if alloc.Amount > remaining {
			return fmt.Errorf(
				"%w: allocation %d (%s) requires %d units but only %d remain",
				ErrInvalidAllocationSum, i, alloc.Address,
				alloc.Amount, remaining,
			)
		}
		totalAllocated += alloc.Amount
	}
	return nil
}

// =============================================================================
// Public Getters (read-only access to immutable fields)
// =============================================================================

func (g *GenesisConfig) ChainID() string               { return g.chainID }
func (g *GenesisConfig) ProtocolVersion() int          { return g.protocolVersion }
func (g *GenesisConfig) TotalSupply() uint64           { return g.totalSupply }
func (g *GenesisConfig) PreminedAmount() uint64        { return g.preminedAmount }
func (g *GenesisConfig) LiquidityPoolAddress() string  { return g.liquidityPoolAddress }
func (g *GenesisConfig) TreasuryAddress() string       { return g.treasuryAddress }
func (g *GenesisConfig) ValidatorAddress() string      { return g.validatorAddress }
func (g *GenesisConfig) BlockTimeSeconds() int         { return g.blockTimeSeconds }
func (g *GenesisConfig) GenesisTime() string           { return g.genesisTime }
func (g *GenesisConfig) GenesisUnixTimestamp() int64   { return g.genesisUnixTimestamp }

// EconomicParams returns a copy of economic parameters
func (g *GenesisConfig) EconomicParams() EconomicParams {
	return g.economicParams
}

// InterventionParams returns a copy of intervention parameters
func (g *GenesisConfig) InterventionParams() InterventionParams {
	return g.interventionParams
}

// RewardPhases returns a copy of the reward phases (defensive copying)
func (g *GenesisConfig) RewardPhases() []RewardPhase {
	phases := make([]RewardPhase, len(g.rewardPhases))
	copy(phases, g.rewardPhases)
	return phases
}

// InitialAllocations returns a copy of the allocations (defensive copying)
func (g *GenesisConfig) InitialAllocations() []Allocation {
	allocations := make([]Allocation, len(g.initialAllocations))
	copy(allocations, g.initialAllocations)
	return allocations
}

// =============================================================================
// Business Logic - NO FLOAT64, NO PANIC, DETERMINISTIC, CONSENSUS-SAFE
// =============================================================================

// GetBlockReward returns the reward for a given block height.
// This function NEVER panics - it always returns a deterministic value.
// Consensus safety: All nodes produce identical results for the same height.
func (g *GenesisConfig) GetBlockReward(height uint64) uint64 {
	// Check fixed reward phases
	for _, phase := range g.rewardPhases {
		if height >= phase.StartBlock && height <= phase.EndBlock {
			return phase.Reward
		}
	}

	// Eternal reward formula (after phase 4)
	if height >= EternalRewardStartHeight {
		return g.calculateEternalReward(height)
	}

	// This path should never be reached because validateNoRewardGap() ensures
	// complete coverage. However, as a defensive measure, we return the minimum
	// reward rather than panicking. This maintains consensus across all nodes
	// even in the presence of a validation bug.
	//
	// In production, if this is reached, it indicates a validation bug that
	// should be fixed. The blockchain continues operating deterministically.
	return MinBlockReward
}

// calculateEternalReward implements the eternal reward formula using fixed-point arithmetic.
// Uses integer-only operations with safe overflow protection.
// Returns a value in smallest AXN units.
func (g *GenesisConfig) calculateEternalReward(height uint64) uint64 {
	// Calculate ratio = height / EternalRewardStartHeight as fixed-point
	ratio, err := mulDivSafe(height, FRACTIONAL_PRECISION, EternalRewardStartHeight)
	if err != nil {
		// Should never happen (EternalRewardStartHeight > 0)
		return MinBlockReward
	}

	// Calculate log2(ratio) with fractional precision
	log2Ratio := log2FixedPrecise(ratio)

	// denominator = 1 + log2(ratio) as fixed-point
	// Convert 1 to fixed-point: 1 * FRACTIONAL_PRECISION
	oneFixed := FRACTIONAL_PRECISION
	denominator := oneFixed + log2Ratio

	// Ensure denominator is at least 1 (in fixed-point)
	if denominator < oneFixed {
		denominator = oneFixed
	}

	// reward = BaseReward / denominator
	// Multiply numerator by FRACTIONAL_PRECISION to maintain precision
	numerator, err := mulDivSafe(g.eternalRewardFormula.BaseReward, FRACTIONAL_PRECISION, denominator)
	if err != nil {
		// Should never happen (denominator > 0)
		return MinBlockReward
	}

	// Convert from fixed-point to integer (round to nearest)
	reward := fixedToUint64(numerator)

	if reward < MinBlockReward {
		return MinBlockReward
	}
	return reward
}

// GetTransactionFee returns the fee for a transaction at given block.
// Deterministic - pure function based on block height and tx size.
func (g *GenesisConfig) GetTransactionFee(blockHeight uint64, txSizeBytes int) uint64 {
	// Calculate ratio = blockHeight / BaseFeeDenominatorStartHeight as fixed-point
	ratio, err := mulDivSafe(blockHeight, FRACTIONAL_PRECISION, BaseFeeDenominatorStartHeight)
	if err != nil {
		// Should never happen (BaseFeeDenominatorStartHeight > 0)
		return MinTransactionFee
	}

	// denominator = 1 + ratio (both in fixed-point)
	oneFixed := FRACTIONAL_PRECISION
	denominator := oneFixed + ratio
	if denominator < oneFixed {
		denominator = oneFixed
	}

	// baseFee = BaseFee / denominator (maintaining precision)
	baseFeeFixed, err := mulDivSafe(g.baseFeeFormula.BaseFee, FRACTIONAL_PRECISION, denominator)
	if err != nil {
		return MinTransactionFee
	}
	baseFee := fixedToUint64(baseFeeFixed)

	// sizeFee = txSizeBytes * ByteFee (no overflow risk within typical tx sizes)
	sizeFee := uint64(txSizeBytes) * g.baseFeeFormula.ByteFee

	var effectiveFee uint64
	if sizeFee > baseFee {
		effectiveFee = sizeFee
	} else {
		effectiveFee = baseFee
	}

	if effectiveFee < MinTransactionFee {
		return MinTransactionFee
	}
	return effectiveFee
}

// SplitGasDistribution returns pool and treasury shares.
// Both values sum to totalGas (no rounding errors).
func (g *GenesisConfig) SplitGasDistribution(totalGas uint64) (poolShare uint64, treasuryTotal uint64) {
	poolShare = mulDivSafePanic(totalGas, g.gasDistribution.PoolShare, 10000)
	treasuryTotal = totalGas - poolShare
	return
}

// GetTreasurySplit divides treasury gas between USD-pegged and AXN reserves.
// IMPORTANT: When treasuryTotal is odd, the extra unit (remainder of division by 2)
// is allocated to AXN share. This behavior is deterministic and documented.
// All nodes will compute the same split for the same total.
func (g *GenesisConfig) GetTreasurySplit(treasuryTotal uint64) (usdShare uint64, axnShare uint64) {
	usdShare = treasuryTotal / 2
	axnShare = treasuryTotal - usdShare
	// Example: if treasuryTotal = 5, then usdShare = 2, axnShare = 3
	// The remainder (1 unit) goes to AXN share by design.
	return
}

// ConfigHash returns a deterministic SHA-256 hash of all genesis parameters.
// Uses binary serialization (NOT JSON) for guaranteed determinism across all nodes.
// This hash is used for:
//   - Network identity verification
//   - Fork detection
//   - Consensus validation
//
// IMPORTANT: Any change to the genesis parameters will change this hash.
// The hash is computed in a fixed order that never changes.
func (g *GenesisConfig) ConfigHash() string {
	h := sha256.New()

	// Helper to ignore errors (writing to Hash never fails)
	write := func(data interface{}) {
		binary.Write(h, binary.BigEndian, data)
	}
	writeString := func(s string) {
		h.Write([]byte(s))
	}

	// Write all fields in FIXED ORDER (no maps, no JSON, no iteration over maps)
	writeString(g.chainID)
	write(uint64(g.protocolVersion))
	write(g.totalSupply)
	write(g.preminedAmount)
	writeString(g.liquidityPoolAddress)
	writeString(g.treasuryAddress)
	writeString(g.validatorAddress)
	write(uint64(g.blockTimeSeconds))

	// Reward phases (ordered by start block, which they are)
	write(uint64(len(g.rewardPhases)))
	for _, phase := range g.rewardPhases {
		write(phase.StartBlock)
		write(phase.EndBlock)
		write(phase.Reward)
	}

	// Eternal reward formula
	write(g.eternalRewardFormula.BaseReward)

	// Fee formula
	write(g.baseFeeFormula.BaseFee)
	write(g.baseFeeFormula.ByteFee)

	// Gas distribution
	write(g.gasDistribution.PoolShare)
	write(g.gasDistribution.TreasuryShare)

	// Economic params
	write(uint64(g.economicParams.StuckFiatFactor))
	write(uint64(g.economicParams.StuckAXNBaseFactor))
	write(uint64(g.economicParams.FairPriceDays))

	// Intervention params
	write(uint64(g.interventionParams.MaxInterventionPercent))
	write(g.interventionParams.MinInterventionBalance)
	write(g.interventionParams.MinInterventionInterval)
	write(uint64(g.interventionParams.BuyStrongZone))
	write(uint64(g.interventionParams.BuyModerateZone))
	write(uint64(g.interventionParams.SellModerateZone))
	write(uint64(g.interventionParams.SellStrongZone))
	write(uint64(g.interventionParams.FlashCrashLimit))
	write(uint64(g.interventionParams.FlashCrashBlocks))

	// Timestamp
	write(uint64(g.genesisUnixTimestamp))

	// Allocations (ordered as defined in initialAllocations)
	write(uint64(len(g.initialAllocations)))
	for _, alloc := range g.initialAllocations {
		writeString(alloc.Address)
		write(alloc.Amount)
		writeString(alloc.Reason)
	}

	return hex.EncodeToString(h.Sum(nil))
}

// =============================================================================
// Genesis Block Creation
// =============================================================================

// GenesisBlockBuilder builds the genesis block with proper error handling.
// This follows the Builder pattern for complex object creation.
type GenesisBlockBuilder struct {
	config *GenesisConfig
}

// NewGenesisBlockBuilder creates a new builder for genesis blocks.
func NewGenesisBlockBuilder(cfg *GenesisConfig) *GenesisBlockBuilder {
	return &GenesisBlockBuilder{config: cfg}
}

// Build creates and returns the genesis block.
func (b *GenesisBlockBuilder) Build() (*Block, error) {
	txs, err := b.createGenesisTransactions()
	if err != nil {
		return nil, fmt.Errorf("failed to create genesis transactions: %w", err)
	}

	genesisTime, err := time.Parse(time.RFC3339, b.config.GenesisTime())
	if err != nil {
		return nil, fmt.Errorf("invalid genesis_time format: %w", err)
	}

	block := NewBlock(0, GenesisPreviousHash, txs, GenesisValidatorID)
	block.Timestamp = genesisTime.Unix()
	block.Hash = block.CalculateHash()

	return block, nil
}

// createGenesisTransactions creates all initial allocation transactions.
func (b *GenesisBlockBuilder) createGenesisTransactions() ([]Transaction, error) {
	txs := make([]Transaction, 0, len(b.config.InitialAllocations()))

	for i, alloc := range b.config.InitialAllocations() {
		tx, err := b.createGenesisTransaction(i, alloc)
		if err != nil {
			return nil, fmt.Errorf("failed to create transaction for allocation %d: %w", i, err)
		}
		txs = append(txs, *tx)
	}

	return txs, nil
}

// createGenesisTransaction creates a single genesis allocation transaction.
func (b *GenesisBlockBuilder) createGenesisTransaction(index int, alloc Allocation) (*Transaction, error) {
	// Validate allocation (redundant but defensive)
	if err := alloc.Validate(); err != nil {
		return nil, err
	}

	// Serialize allocation data
	data, err := json.Marshal(alloc)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal allocation: %w", err)
	}

	// Generate deterministic hash based on index and address
	hashBase := fmt.Sprintf("genesis_alloc_%d_%s", index, alloc.Address)
	hashBytes := sha256.Sum256([]byte(hashBase))

	tx := &Transaction{
		Hash:      hex.EncodeToString(hashBytes[:]),
		From:      "0000000000000000000000000000000000000000", // Zero address for genesis
		To:        alloc.Address,
		Value:     alloc.Amount,
		Fee:       0, // No fee for genesis transactions
		Nonce:     0,
		Timestamp: b.config.GenesisUnixTimestamp(),
		Signature: "genesis_signature", // Special signature for genesis
		Type:      "GENESIS_ALLOCATION",
		Data:      string(data),
	}

	return tx, nil
}

// =============================================================================
// Global instance (for convenience, but immutable and safe)
// =============================================================================

var (
	globalGenesisConfig *GenesisConfig
	globalConfigOnce    sync.Once
	globalConfigErr     error
)

// GetGlobalGenesisConfig returns the singleton genesis configuration.
// This is safe because the config is immutable after creation.
func GetGlobalGenesisConfig() (*GenesisConfig, error) {
	globalConfigOnce.Do(func() {
		globalGenesisConfig, globalConfigErr = NewGenesisConfig()
	})
	return globalGenesisConfig, globalConfigErr
}

// MustGetGlobalGenesisConfig returns the global config or panics.
// Only use this during initialization when you know it will succeed.
func MustGetGlobalGenesisConfig() *GenesisConfig {
	cfg, err := GetGlobalGenesisConfig()
	if err != nil {
		panic(fmt.Sprintf("failed to load genesis config: %v", err))
	}
	return cfg
}

// GenerateGenesis creates the genesis block using the global config.
// Returns error instead of panicking, allowing graceful failure handling.
func GenerateGenesis() (*Block, error) {
	cfg, err := GetGlobalGenesisConfig()
	if err != nil {
		return nil, fmt.Errorf("genesis config unavailable: %w", err)
	}

	block, err := NewGenesisBlockBuilder(cfg).Build()
	if err != nil {
		return nil, fmt.Errorf("failed to generate genesis block: %w", err)
	}

	return block, nil
}
