package billing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"
	"sort"
	"sync"
	"time"
)

var errNotFound = errors.New("not found")

type walletMeta struct {
	Sequence int64
	State    WalletState
}

type MemoryStore struct {
	mu       sync.Mutex
	catalogs map[string][]RegionCatalog
	books    map[string][]PriceBook
	bookByID map[string]PriceBook
	quotes   map[string]Quote
	meta     map[string]walletMeta
	entries  map[string][]LedgerEntry
	usage    map[string]UsageRecord
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		catalogs: map[string][]RegionCatalog{},
		books:    map[string][]PriceBook{},
		bookByID: map[string]PriceBook{},
		quotes:   map[string]Quote{},
		meta:     map[string]walletMeta{},
		entries:  map[string][]LedgerEntry{},
		usage:    map[string]UsageRecord{},
	}
}

func (s *MemoryStore) PublishCatalog(_ context.Context, catalog RegionCatalog) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, current := range s.catalogs[catalog.RegionID] {
		if current.ID == catalog.ID || current.Version == catalog.Version {
			if catalogEqual(current, catalog) {
				return nil
			}
			return ErrImmutable
		}
	}
	s.catalogs[catalog.RegionID] = append(s.catalogs[catalog.RegionID], cloneCatalog(catalog))
	return nil
}

func (s *MemoryStore) CatalogAt(_ context.Context, regionID string, at time.Time) (RegionCatalog, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var candidates []RegionCatalog
	for _, catalog := range s.catalogs[regionID] {
		if !catalog.EffectiveAt.After(at) {
			candidates = append(candidates, catalog)
		}
	}
	if len(candidates) == 0 {
		return RegionCatalog{}, errNotFound
	}
	sort.Slice(candidates, func(left, right int) bool { return candidates[left].EffectiveAt.After(candidates[right].EffectiveAt) })
	return cloneCatalog(candidates[0]), nil
}

func (s *MemoryStore) PublishPriceBook(_ context.Context, book PriceBook) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, exists := s.bookByID[book.ID]; exists {
		if priceBookEqual(current, book) {
			return nil
		}
		return ErrImmutable
	}
	for _, current := range s.books[book.RegionID] {
		if current.Revision == book.Revision {
			return ErrImmutable
		}
	}
	stored := clonePriceBook(book)
	s.bookByID[book.ID] = stored
	s.books[book.RegionID] = append(s.books[book.RegionID], stored)
	return nil
}

func (s *MemoryStore) PriceBookAt(_ context.Context, regionID string, at time.Time) (PriceBook, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var candidates []PriceBook
	for _, book := range s.books[regionID] {
		if !book.EffectiveAt.After(at) {
			candidates = append(candidates, book)
		}
	}
	if len(candidates) == 0 {
		return PriceBook{}, errNotFound
	}
	sort.Slice(candidates, func(left, right int) bool { return candidates[left].EffectiveAt.After(candidates[right].EffectiveAt) })
	return clonePriceBook(candidates[0]), nil
}

func (s *MemoryStore) PriceBookByID(_ context.Context, id string) (PriceBook, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	book, ok := s.bookByID[id]
	if !ok {
		return PriceBook{}, errNotFound
	}
	return clonePriceBook(book), nil
}

func (s *MemoryStore) PutQuote(_ context.Context, quote Quote) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, exists := s.quotes[quote.ID]; exists && !quoteEqual(current, quote) {
		return ErrImmutable
	}
	s.quotes[quote.ID] = cloneQuote(quote)
	return nil
}

func (s *MemoryStore) QuoteByID(_ context.Context, id string) (Quote, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	quote, ok := s.quotes[id]
	if !ok {
		return Quote{}, errNotFound
	}
	return cloneQuote(quote), nil
}

func (s *MemoryStore) CreateHold(_ context.Context, quoteID, workspaceID string, now time.Time, maximumDebit int64, signature string) (Quote, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	quote, ok := s.quotes[quoteID]
	if !ok {
		return Quote{}, errNotFound
	}
	if quote.WorkspaceID != workspaceID {
		return Quote{}, ErrQuoteScope
	}
	if !now.Before(quote.ExpiresAt) {
		return Quote{}, ErrQuoteExpired
	}
	if quote.Hold != nil {
		return cloneQuote(quote), nil
	}
	wallet := s.walletLocked(workspaceID)
	if wallet.AvailableMinor < maximumDebit {
		return Quote{}, ErrInsufficientFunds
	}
	if wallet.State != WalletActive {
		return Quote{}, ErrInsufficientFunds
	}
	holdID, err := randomID("hold")
	if err != nil {
		return Quote{}, err
	}
	fundingID, err := randomID("fnd")
	if err != nil {
		return Quote{}, err
	}
	quote.Hold = &CapacityHold{ID: holdID, Status: "held", ExpiresAt: quote.ExpiresAt}
	quote.Funding = &FundingAuthorization{ID: fundingID, MaximumDebitMinor: maximumDebit, Currency: CurrencyCNY, ExpiresAt: quote.ExpiresAt, Signature: signature}
	s.quotes[quoteID] = quote
	return cloneQuote(quote), nil
}

func (s *MemoryStore) Grant(_ context.Context, entry LedgerEntry) (Wallet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.appendCreditLocked(entry)
}

func (s *MemoryStore) Correct(_ context.Context, entry LedgerEntry) (Wallet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.appendCreditLocked(entry)
}

func (s *MemoryStore) appendCreditLocked(entry LedgerEntry) (Wallet, error) {
	for _, current := range s.entries[entry.WorkspaceID] {
		if current.SourceID == entry.SourceID && current.Kind == entry.Kind && current.Bucket == entry.Bucket {
			if current.AmountMinor == entry.AmountMinor && current.Reason == entry.Reason {
				return s.walletLocked(entry.WorkspaceID), nil
			}
			return Wallet{}, ErrImmutable
		}
	}
	wallet := s.walletLocked(entry.WorkspaceID)
	bucketBalance := wallet.CashMinor
	if entry.Bucket == BucketPromotional {
		bucketBalance = wallet.PromotionalMinor
	}
	newBucketBalance, bucketOK := safeAdd(bucketBalance, entry.AmountMinor)
	newAvailable, availableOK := safeAdd(wallet.AvailableMinor, entry.AmountMinor)
	if !bucketOK || newBucketBalance < 0 || !availableOK || newAvailable < 0 {
		return Wallet{}, ErrNegativeBalance
	}
	meta := s.meta[entry.WorkspaceID]
	meta.Sequence++
	if newAvailable == 0 {
		meta.State = WalletExhausted
	} else {
		meta.State = WalletActive
	}
	entry.Sequence = meta.Sequence
	s.meta[entry.WorkspaceID] = meta
	s.entries[entry.WorkspaceID] = append(s.entries[entry.WorkspaceID], entry)
	return s.walletLocked(entry.WorkspaceID), nil
}

func (s *MemoryStore) Wallet(_ context.Context, workspaceID string) (Wallet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.walletLocked(workspaceID), nil
}

func (s *MemoryStore) walletLocked(workspaceID string) Wallet {
	meta := s.meta[workspaceID]
	wallet := Wallet{WorkspaceID: workspaceID, Currency: CurrencyCNY, LedgerSequence: meta.Sequence, State: meta.State}
	if wallet.State == "" {
		wallet.State = WalletActive
	}
	for _, entry := range s.entries[workspaceID] {
		switch entry.Bucket {
		case BucketPromotional:
			wallet.PromotionalMinor += entry.AmountMinor
		case BucketCash:
			wallet.CashMinor += entry.AmountMinor
		}
	}
	wallet.AvailableMinor = wallet.PromotionalMinor + wallet.CashMinor
	return wallet
}

func (s *MemoryStore) PostUsage(_ context.Context, records []UsageRecord, now time.Time) (DebitResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(records) == 0 {
		return DebitResult{}, nil
	}
	workspaceID := records[0].WorkspaceID
	var fresh []UsageRecord
	var requested int64
	for _, record := range records {
		if record.WorkspaceID != workspaceID {
			return DebitResult{}, ErrQuoteScope
		}
		if current, exists := s.usage[record.ID]; exists {
			if !usageEqual(current, record) {
				return DebitResult{}, ErrImmutable
			}
			continue
		}
		fresh = append(fresh, record)
		var ok bool
		requested, ok = safeAdd(requested, record.ChargeMinor)
		if !ok {
			return DebitResult{}, ErrInvalidPriceBook
		}
	}
	wallet := s.walletLocked(workspaceID)
	collected := requested
	if collected > wallet.AvailableMinor {
		collected = wallet.AvailableMinor
	}
	promotionalDebit := collected
	if promotionalDebit > wallet.PromotionalMinor {
		promotionalDebit = wallet.PromotionalMinor
	}
	cashDebit := collected - promotionalDebit
	entries := make([]LedgerEntry, 0, 2)
	meta := s.meta[workspaceID]
	if promotionalDebit > 0 {
		meta.Sequence++
		entries = append(entries, usageDebitEntry(workspaceID, BucketPromotional, -promotionalDebit, fresh, meta.Sequence, now))
	}
	if cashDebit > 0 {
		meta.Sequence++
		entries = append(entries, usageDebitEntry(workspaceID, BucketCash, -cashDebit, fresh, meta.Sequence, now))
	}
	for _, record := range fresh {
		s.usage[record.ID] = record
	}
	s.entries[workspaceID] = append(s.entries[workspaceID], entries...)
	if requested > 0 && collected == wallet.AvailableMinor {
		meta.State = WalletExhausted
	}
	s.meta[workspaceID] = meta
	wallet = s.walletLocked(workspaceID)
	return DebitResult{Entries: entries, Wallet: wallet, RequestedMinor: requested, CollectedMinor: collected, UnfundedMinor: requested - collected, BalanceExhausted: wallet.State == WalletExhausted}, nil
}

func (s *MemoryStore) LedgerEntries(_ context.Context, workspaceID string, limit int) ([]LedgerEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries := s.entries[workspaceID]
	if len(entries) > limit {
		entries = entries[len(entries)-limit:]
	}
	result := append([]LedgerEntry(nil), entries...)
	sort.Slice(result, func(left, right int) bool { return result[left].Sequence > result[right].Sequence })
	return result, nil
}

func usageEqual(left, right UsageRecord) bool {
	return left.ID == right.ID && left.WorkspaceID == right.WorkspaceID && left.LogicalInstanceID == right.LogicalInstanceID && left.RegionID == right.RegionID && left.PriceBookID == right.PriceBookID && left.ResourceKind == right.ResourceKind && left.Quantity == right.Quantity && left.IntervalStart.Equal(right.IntervalStart) && left.IntervalEnd.Equal(right.IntervalEnd) && left.ChargeMinor == right.ChargeMinor
}

func catalogEqual(left, right RegionCatalog) bool {
	return left.ID == right.ID && left.RegionID == right.RegionID && left.Version == right.Version && left.CPU == right.CPU && left.Memory == right.Memory && left.Disk == right.Disk && left.DedicatedIPAvailable == right.DedicatedIPAvailable && reflect.DeepEqual(left.EndpointDeliveryModes, right.EndpointDeliveryModes) && left.Availability == right.Availability && left.EffectiveAt.Equal(right.EffectiveAt) && left.CreatedAt.Equal(right.CreatedAt)
}

func priceBookEqual(left, right PriceBook) bool {
	return left.ID == right.ID && left.RegionID == right.RegionID && left.Revision == right.Revision && left.Currency == right.Currency && reflect.DeepEqual(left.UnitPrices, right.UnitPrices) && left.EffectiveAt.Equal(right.EffectiveAt) && left.CreatedAt.Equal(right.CreatedAt)
}

func quoteEqual(left, right Quote) bool {
	return left.ID == right.ID && left.WorkspaceID == right.WorkspaceID && left.RegionID == right.RegionID && left.PriceBookID == right.PriceBookID && left.ResourceSpec == right.ResourceSpec && left.DedicatedIP == right.DedicatedIP && left.EstimatedHourlyMinor == right.EstimatedHourlyMinor && left.Currency == right.Currency && left.ExpiresAt.Equal(right.ExpiresAt) && left.CreatedAt.Equal(right.CreatedAt) && reflect.DeepEqual(left.Hold, right.Hold) && reflect.DeepEqual(left.Funding, right.Funding)
}

func usageDebitEntry(workspaceID string, bucket LedgerBucket, amount int64, records []UsageRecord, sequence int64, now time.Time) LedgerEntry {
	hash := sha256.New()
	for _, record := range records {
		hash.Write([]byte(record.ID))
	}
	hash.Write([]byte(bucket))
	sourceID := "usage_" + hex.EncodeToString(hash.Sum(nil)[:12])
	return LedgerEntry{ID: "led_" + hex.EncodeToString(hash.Sum(nil)[12:24]), WorkspaceID: workspaceID, Sequence: sequence, Kind: LedgerUsageDebit, Bucket: bucket, AmountMinor: amount, SourceID: sourceID, Reason: "measured resource usage", OccurredAt: now}
}

func cloneCatalog(catalog RegionCatalog) RegionCatalog {
	catalog.EndpointDeliveryModes = append([]string(nil), catalog.EndpointDeliveryModes...)
	return catalog
}

func clonePriceBook(book PriceBook) PriceBook {
	book.UnitPrices = append([]UnitPrice(nil), book.UnitPrices...)
	return book
}

func cloneQuote(quote Quote) Quote {
	if quote.Hold != nil {
		hold := *quote.Hold
		quote.Hold = &hold
	}
	if quote.Funding != nil {
		funding := *quote.Funding
		quote.Funding = &funding
	}
	return quote
}
