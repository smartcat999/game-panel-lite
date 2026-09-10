package billing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

const quoteLifetime = 10 * time.Minute

type Store interface {
	PublishCatalog(context.Context, RegionCatalog) error
	CatalogAt(context.Context, string, time.Time) (RegionCatalog, error)
	PublishPriceBook(context.Context, PriceBook) error
	PriceBookAt(context.Context, string, time.Time) (PriceBook, error)
	PriceBookByID(context.Context, string) (PriceBook, error)
	PutQuote(context.Context, Quote) error
	QuoteByID(context.Context, string) (Quote, error)
	CreateHold(context.Context, string, string, time.Time, int64, string) (Quote, error)
	Grant(context.Context, LedgerEntry) (Wallet, error)
	Correct(context.Context, LedgerEntry) (Wallet, error)
	Wallet(context.Context, string) (Wallet, error)
	PostUsage(context.Context, []UsageRecord, time.Time) (DebitResult, error)
	LedgerEntries(context.Context, string, int) ([]LedgerEntry, error)
}

type Module struct {
	store      Store
	signingKey []byte
	now        func() time.Time
}

func New(store Store, signingKey []byte) *Module {
	return &Module{store: store, signingKey: append([]byte(nil), signingKey...), now: time.Now}
}

func (m *Module) PublishCatalog(ctx context.Context, catalog RegionCatalog) error {
	if err := validateCatalog(catalog); err != nil {
		return err
	}
	return m.store.PublishCatalog(ctx, catalog)
}

func (m *Module) PublishPriceBook(ctx context.Context, book PriceBook) error {
	if err := validatePriceBook(book); err != nil {
		return err
	}
	return m.store.PublishPriceBook(ctx, book)
}

func (m *Module) CreateQuote(ctx context.Context, workspaceID, regionID string, spec ResourceSpec, dedicatedIP bool) (Quote, error) {
	if workspaceID == "" || regionID == "" {
		return Quote{}, ErrInvalidResourceSpec
	}
	now := m.now().UTC()
	catalog, err := m.store.CatalogAt(ctx, regionID, now)
	if err != nil {
		return Quote{}, err
	}
	if catalog.Availability == AvailabilityUnavailable || dedicatedIP && !catalog.DedicatedIPAvailable {
		return Quote{}, ErrRegionUnavailable
	}
	if err := validateSpec(catalog, spec); err != nil {
		return Quote{}, err
	}
	book, err := m.store.PriceBookAt(ctx, regionID, now)
	if err != nil {
		return Quote{}, err
	}
	amount, err := hourlyPrice(book, spec, dedicatedIP)
	if err != nil {
		return Quote{}, err
	}
	id, err := randomID("quo")
	if err != nil {
		return Quote{}, err
	}
	quote := Quote{ID: id, WorkspaceID: workspaceID, RegionID: regionID, PriceBookID: book.ID, ResourceSpec: spec, DedicatedIP: dedicatedIP, EstimatedHourlyMinor: amount, Currency: CurrencyCNY, ExpiresAt: now.Add(quoteLifetime), CreatedAt: now}
	if err := m.store.PutQuote(ctx, quote); err != nil {
		return Quote{}, err
	}
	return quote, nil
}

func (m *Module) AuthorizeCreate(ctx context.Context, workspaceID, quoteID string) (Quote, error) {
	now := m.now().UTC()
	quote, err := m.store.QuoteByID(ctx, quoteID)
	if err != nil {
		return Quote{}, err
	}
	if quote.WorkspaceID != workspaceID {
		return Quote{}, ErrQuoteScope
	}
	if !now.Before(quote.ExpiresAt) {
		return Quote{}, ErrQuoteExpired
	}
	maximumDebit, ok := safeMultiply(quote.EstimatedHourlyMinor, 24)
	if !ok {
		return Quote{}, ErrInvalidPriceBook
	}
	signature := m.signFunding(quote.ID, workspaceID, maximumDebit, quote.ExpiresAt)
	return m.store.CreateHold(ctx, quote.ID, workspaceID, now, maximumDebit, signature)
}

func (m *Module) GrantPromotionalCredit(ctx context.Context, workspaceID, sourceID string, amountMinor int64, reason string) (Wallet, error) {
	if amountMinor <= 0 || sourceID == "" || reason == "" {
		return Wallet{}, ErrNegativeBalance
	}
	id, err := randomID("led")
	if err != nil {
		return Wallet{}, err
	}
	entry := LedgerEntry{ID: id, WorkspaceID: workspaceID, Kind: LedgerPromotionalCredit, Bucket: BucketPromotional, AmountMinor: amountMinor, SourceID: sourceID, Reason: reason, OccurredAt: m.now().UTC()}
	return m.store.Grant(ctx, entry)
}

func (m *Module) Correct(ctx context.Context, workspaceID, sourceID string, bucket LedgerBucket, amountMinor int64, reason string) (Wallet, error) {
	if amountMinor == 0 || sourceID == "" || reason == "" || bucket != BucketPromotional && bucket != BucketCash {
		return Wallet{}, ErrNegativeBalance
	}
	id, err := randomID("led")
	if err != nil {
		return Wallet{}, err
	}
	entry := LedgerEntry{ID: id, WorkspaceID: workspaceID, Kind: LedgerCorrection, Bucket: bucket, AmountMinor: amountMinor, SourceID: sourceID, Reason: reason, OccurredAt: m.now().UTC()}
	return m.store.Correct(ctx, entry)
}

func (m *Module) RecordUsage(ctx context.Context, allocation Allocation, intervalStart, intervalEnd time.Time) (DebitResult, error) {
	if allocation.WorkspaceID == "" || allocation.LogicalInstanceID == "" || allocation.RegionID == "" || allocation.PriceBookID == "" || allocation.CPUMilli < 0 || allocation.MemoryMiB < 0 || allocation.DiskGiB < 0 || allocation.BackupGiB < 0 || !intervalEnd.After(intervalStart) || intervalEnd.Sub(intervalStart) > time.Hour {
		return DebitResult{}, fmt.Errorf("invalid usage interval")
	}
	book, err := m.store.PriceBookByID(ctx, allocation.PriceBookID)
	if err != nil {
		return DebitResult{}, err
	}
	if book.RegionID != allocation.RegionID {
		return DebitResult{}, ErrInvalidPriceBook
	}
	quantities := map[ResourceKind]int64{
		ResourceInstanceDisk:  allocation.DiskGiB,
		ResourceBackupStorage: allocation.BackupGiB,
	}
	if allocation.ComputeAllocated {
		quantities[ResourceCPU] = allocation.CPUMilli
		quantities[ResourceMemory] = allocation.MemoryMiB
	}
	if allocation.DedicatedIP {
		quantities[ResourceDedicatedIP] = 1
	}
	var records []UsageRecord
	for _, kind := range allResourceKinds {
		quantity := quantities[kind]
		if quantity <= 0 {
			continue
		}
		charge, err := intervalPrice(book, kind, quantity, intervalEnd.Sub(intervalStart))
		if err != nil {
			return DebitResult{}, err
		}
		recordID := usageRecordID(allocation.LogicalInstanceID, kind, intervalStart, intervalEnd)
		records = append(records, UsageRecord{ID: recordID, WorkspaceID: allocation.WorkspaceID, LogicalInstanceID: allocation.LogicalInstanceID, RegionID: allocation.RegionID, PriceBookID: allocation.PriceBookID, ResourceKind: kind, Quantity: quantity, IntervalStart: intervalStart.UTC(), IntervalEnd: intervalEnd.UTC(), ChargeMinor: charge})
	}
	return m.store.PostUsage(ctx, records, m.now().UTC())
}

func (m *Module) Wallet(ctx context.Context, workspaceID string) (Wallet, error) {
	return m.store.Wallet(ctx, workspaceID)
}

func (m *Module) CatalogAndPriceBook(ctx context.Context, regionID string) (RegionCatalog, PriceBook, error) {
	now := m.now().UTC()
	catalog, err := m.store.CatalogAt(ctx, regionID, now)
	if err != nil {
		return RegionCatalog{}, PriceBook{}, err
	}
	book, err := m.store.PriceBookAt(ctx, regionID, now)
	return catalog, book, err
}

func (m *Module) LedgerEntries(ctx context.Context, workspaceID string, limit int) ([]LedgerEntry, error) {
	if limit < 1 || limit > 100 {
		limit = 100
	}
	return m.store.LedgerEntries(ctx, workspaceID, limit)
}

func (m *Module) signFunding(quoteID, workspaceID string, maximumDebit int64, expiresAt time.Time) string {
	return fundingSignature(m.signingKey, quoteID, workspaceID, maximumDebit, expiresAt)
}

func fundingSignature(signingKey []byte, quoteID, workspaceID string, maximumDebit int64, expiresAt time.Time) string {
	digest := hmac.New(sha256.New, signingKey)
	fmt.Fprintf(digest, "%s\x00%s\x00%d\x00%d", quoteID, workspaceID, maximumDebit, expiresAt.Unix())
	return hex.EncodeToString(digest.Sum(nil))
}
