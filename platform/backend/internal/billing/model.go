package billing

import (
	"errors"
	"time"
)

const CurrencyCNY = "CNY"

var (
	ErrInvalidCatalog      = errors.New("invalid region resource catalog")
	ErrInvalidPriceBook    = errors.New("invalid price book")
	ErrInvalidResourceSpec = errors.New("invalid resource specification")
	ErrRegionUnavailable   = errors.New("region capacity unavailable")
	ErrQuoteExpired        = errors.New("quote expired")
	ErrQuoteScope          = errors.New("quote belongs to another workspace")
	ErrInsufficientFunds   = errors.New("insufficient funds for 24-hour estimate")
	ErrNegativeBalance     = errors.New("ledger entry would produce a negative balance")
	ErrImmutable           = errors.New("immutable record already exists with different content")
)

type ResourceKind string

const (
	ResourceCPU           ResourceKind = "cpu"
	ResourceMemory        ResourceKind = "memory"
	ResourceInstanceDisk  ResourceKind = "instance-disk"
	ResourceBackupStorage ResourceKind = "backup-storage"
	ResourceDedicatedIP   ResourceKind = "dedicated-ip"
)

type Availability string

const (
	AvailabilityAvailable   Availability = "available"
	AvailabilityLimited     Availability = "limited"
	AvailabilityUnavailable Availability = "unavailable"
)

type Range struct {
	Minimum int64 `json:"minimum"`
	Maximum int64 `json:"maximum"`
	Step    int64 `json:"step"`
}

type ResourceSpec struct {
	CPUMilli  int64 `json:"cpuMilli"`
	MemoryMiB int64 `json:"memoryMiB"`
	DiskGiB   int64 `json:"diskGiB"`
}

type RegionCatalog struct {
	ID                    string       `json:"id"`
	RegionID              string       `json:"regionId"`
	Version               int64        `json:"version"`
	CPU                   Range        `json:"cpu"`
	Memory                Range        `json:"memory"`
	Disk                  Range        `json:"disk"`
	DedicatedIPAvailable  bool         `json:"dedicatedIpAvailable"`
	EndpointDeliveryModes []string     `json:"endpointDeliveryModes"`
	Availability          Availability `json:"availability"`
	EffectiveAt           time.Time    `json:"effectiveAt"`
	CreatedAt             time.Time    `json:"createdAt"`
}

type UnitPrice struct {
	ResourceKind ResourceKind `json:"resourceKind"`
	PriceMinor   int64        `json:"priceMinor"`
	UnitQuantity int64        `json:"unitQuantity"`
	Unit         string       `json:"unit"`
}

type PriceBook struct {
	ID          string      `json:"id"`
	RegionID    string      `json:"regionId"`
	Revision    int64       `json:"revision"`
	Currency    string      `json:"currency"`
	UnitPrices  []UnitPrice `json:"unitPrices"`
	EffectiveAt time.Time   `json:"effectiveAt"`
	CreatedAt   time.Time   `json:"createdAt"`
}

type Quote struct {
	ID                   string       `json:"id"`
	WorkspaceID          string       `json:"workspaceId"`
	RegionID             string       `json:"regionId"`
	PriceBookID          string       `json:"priceBookId"`
	ResourceSpec         ResourceSpec `json:"resourceSpec"`
	DedicatedIP          bool         `json:"dedicatedIp"`
	EstimatedHourlyMinor int64        `json:"estimatedHourlyMinor"`
	Currency             string       `json:"currency"`
	ExpiresAt            time.Time    `json:"expiresAt"`
	CreatedAt            time.Time    `json:"createdAt"`
	Hold                 *CapacityHold
	Funding              *FundingAuthorization
}

type CapacityHold struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type FundingAuthorization struct {
	ID                string    `json:"id"`
	MaximumDebitMinor int64     `json:"maximumDebitMinor"`
	Currency          string    `json:"currency"`
	ExpiresAt         time.Time `json:"expiresAt"`
	Signature         string    `json:"signature"`
}

type WalletState string

const (
	WalletActive    WalletState = "active"
	WalletExhausted WalletState = "exhausted"
	WalletSuspended WalletState = "suspended"
)

type Wallet struct {
	WorkspaceID      string      `json:"workspaceId"`
	Currency         string      `json:"currency"`
	PromotionalMinor int64       `json:"promotionalMinor"`
	CashMinor        int64       `json:"cashMinor"`
	AvailableMinor   int64       `json:"availableMinor"`
	LedgerSequence   int64       `json:"ledgerSequence"`
	State            WalletState `json:"state"`
}

type LedgerBucket string

const (
	BucketPromotional LedgerBucket = "promotional"
	BucketCash        LedgerBucket = "cash"
)

type LedgerKind string

const (
	LedgerPromotionalCredit LedgerKind = "promotional-credit"
	LedgerUsageDebit        LedgerKind = "usage-debit"
	LedgerCorrection        LedgerKind = "correction"
)

type LedgerEntry struct {
	ID          string       `json:"id"`
	WorkspaceID string       `json:"workspaceId"`
	Sequence    int64        `json:"sequence"`
	Kind        LedgerKind   `json:"kind"`
	Bucket      LedgerBucket `json:"bucket"`
	AmountMinor int64        `json:"amountMinor"`
	SourceID    string       `json:"sourceId"`
	Reason      string       `json:"reason"`
	OccurredAt  time.Time    `json:"occurredAt"`
}

type Allocation struct {
	WorkspaceID       string
	LogicalInstanceID string
	RegionID          string
	PriceBookID       string
	CPUMilli          int64
	MemoryMiB         int64
	DiskGiB           int64
	BackupGiB         int64
	DedicatedIP       bool
	ComputeAllocated  bool
}

type UsageRecord struct {
	ID                string       `json:"id"`
	WorkspaceID       string       `json:"workspaceId"`
	LogicalInstanceID string       `json:"logicalInstanceId"`
	RegionID          string       `json:"regionId"`
	PriceBookID       string       `json:"priceBookId"`
	ResourceKind      ResourceKind `json:"resourceKind"`
	Quantity          int64        `json:"quantity"`
	IntervalStart     time.Time    `json:"intervalStart"`
	IntervalEnd       time.Time    `json:"intervalEnd"`
	ChargeMinor       int64        `json:"chargeMinor"`
}

type DebitResult struct {
	Entries          []LedgerEntry
	Wallet           Wallet
	RequestedMinor   int64
	CollectedMinor   int64
	UnfundedMinor    int64
	BalanceExhausted bool
}

type ExhaustionPolicy struct {
	StopRunningInstances bool
	FullDataRetention    time.Duration
	BackupOnlyRetention  time.Duration
}

func DefaultExhaustionPolicy() ExhaustionPolicy {
	return ExhaustionPolicy{StopRunningInstances: true, FullDataRetention: 7 * 24 * time.Hour, BackupOnlyRetention: 30 * 24 * time.Hour}
}
