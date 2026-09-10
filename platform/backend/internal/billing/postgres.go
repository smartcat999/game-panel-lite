package billing

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

type PostgresStore struct {
	database *sql.DB
}

func NewPostgresStore(database *sql.DB) *PostgresStore {
	return &PostgresStore{database: database}
}

func (s *PostgresStore) PublishCatalog(ctx context.Context, catalog RegionCatalog) error {
	modes, err := json.Marshal(catalog.EndpointDeliveryModes)
	if err != nil {
		return err
	}
	result, err := s.database.ExecContext(ctx, `
		INSERT INTO region_resource_catalogs (
			id, region_id, version, cpu_min_milli, cpu_max_milli, cpu_step_milli,
			memory_min_mib, memory_max_mib, memory_step_mib, disk_min_gib, disk_max_gib,
			disk_step_gib, dedicated_ip_available, endpoint_delivery_modes, availability, effective_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		ON CONFLICT DO NOTHING`,
		catalog.ID, catalog.RegionID, catalog.Version, catalog.CPU.Minimum, catalog.CPU.Maximum, catalog.CPU.Step,
		catalog.Memory.Minimum, catalog.Memory.Maximum, catalog.Memory.Step, catalog.Disk.Minimum, catalog.Disk.Maximum,
		catalog.Disk.Step, catalog.DedicatedIPAvailable, string(modes), catalog.Availability, catalog.EffectiveAt, catalog.CreatedAt)
	if err != nil {
		return err
	}
	inserted, err := result.RowsAffected()
	if err != nil || inserted == 1 {
		return err
	}
	current, err := s.catalogByID(ctx, catalog.ID)
	if err != nil || !catalogEqual(current, catalog) {
		return ErrImmutable
	}
	return nil
}

func (s *PostgresStore) CatalogAt(ctx context.Context, regionID string, at time.Time) (RegionCatalog, error) {
	return scanCatalog(s.database.QueryRowContext(ctx, `
		SELECT id, region_id, version, cpu_min_milli, cpu_max_milli, cpu_step_milli,
			memory_min_mib, memory_max_mib, memory_step_mib, disk_min_gib, disk_max_gib,
			disk_step_gib, dedicated_ip_available, endpoint_delivery_modes, availability, effective_at, created_at
		FROM region_resource_catalogs
		WHERE region_id = $1 AND effective_at <= $2
		ORDER BY effective_at DESC, version DESC LIMIT 1`, regionID, at))
}

func (s *PostgresStore) catalogByID(ctx context.Context, id string) (RegionCatalog, error) {
	return scanCatalog(s.database.QueryRowContext(ctx, `
		SELECT id, region_id, version, cpu_min_milli, cpu_max_milli, cpu_step_milli,
			memory_min_mib, memory_max_mib, memory_step_mib, disk_min_gib, disk_max_gib,
			disk_step_gib, dedicated_ip_available, endpoint_delivery_modes, availability, effective_at, created_at
		FROM region_resource_catalogs WHERE id = $1`, id))
}

func scanCatalog(row rowScanner) (RegionCatalog, error) {
	var catalog RegionCatalog
	var modes []byte
	err := row.Scan(
		&catalog.ID, &catalog.RegionID, &catalog.Version, &catalog.CPU.Minimum, &catalog.CPU.Maximum, &catalog.CPU.Step,
		&catalog.Memory.Minimum, &catalog.Memory.Maximum, &catalog.Memory.Step, &catalog.Disk.Minimum, &catalog.Disk.Maximum,
		&catalog.Disk.Step, &catalog.DedicatedIPAvailable, &modes, &catalog.Availability, &catalog.EffectiveAt, &catalog.CreatedAt,
	)
	if err == nil {
		err = json.Unmarshal(modes, &catalog.EndpointDeliveryModes)
	}
	return catalog, err
}

func (s *PostgresStore) PublishPriceBook(ctx context.Context, book PriceBook) error {
	prices, err := json.Marshal(book.UnitPrices)
	if err != nil {
		return err
	}
	result, err := s.database.ExecContext(ctx, `
		INSERT INTO price_books (id, region_id, revision, currency, unit_prices, effective_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT DO NOTHING`, book.ID, book.RegionID, book.Revision, book.Currency, string(prices), book.EffectiveAt, book.CreatedAt)
	if err != nil {
		return err
	}
	inserted, err := result.RowsAffected()
	if err != nil || inserted == 1 {
		return err
	}
	current, err := s.PriceBookByID(ctx, book.ID)
	if err != nil || !priceBookEqual(current, book) {
		return ErrImmutable
	}
	return nil
}

func (s *PostgresStore) PriceBookAt(ctx context.Context, regionID string, at time.Time) (PriceBook, error) {
	return scanPriceBook(s.database.QueryRowContext(ctx, `
		SELECT id, region_id, revision, currency, unit_prices, effective_at, created_at
		FROM price_books
		WHERE region_id = $1 AND effective_at <= $2
		ORDER BY effective_at DESC, revision DESC LIMIT 1`, regionID, at))
}

func (s *PostgresStore) PriceBookByID(ctx context.Context, id string) (PriceBook, error) {
	return scanPriceBook(s.database.QueryRowContext(ctx, `SELECT id, region_id, revision, currency, unit_prices, effective_at, created_at FROM price_books WHERE id = $1`, id))
}

func scanPriceBook(row rowScanner) (PriceBook, error) {
	var book PriceBook
	var prices []byte
	err := row.Scan(&book.ID, &book.RegionID, &book.Revision, &book.Currency, &prices, &book.EffectiveAt, &book.CreatedAt)
	if err == nil {
		err = json.Unmarshal(prices, &book.UnitPrices)
	}
	return book, err
}

func (s *PostgresStore) PutQuote(ctx context.Context, quote Quote) error {
	result, err := s.database.ExecContext(ctx, `
		INSERT INTO resource_quotes (id, workspace_id, region_id, price_book_id, cpu_milli, memory_mib, disk_gib, dedicated_ip, estimated_hourly_minor, currency, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (id) DO NOTHING`, quote.ID, quote.WorkspaceID, quote.RegionID, quote.PriceBookID, quote.ResourceSpec.CPUMilli, quote.ResourceSpec.MemoryMiB, quote.ResourceSpec.DiskGiB, quote.DedicatedIP, quote.EstimatedHourlyMinor, quote.Currency, quote.ExpiresAt, quote.CreatedAt)
	if err != nil {
		return err
	}
	inserted, err := result.RowsAffected()
	if err != nil || inserted == 1 {
		return err
	}
	current, err := s.QuoteByID(ctx, quote.ID)
	if err != nil || !quoteEqual(current, quote) {
		return ErrImmutable
	}
	return nil
}

func (s *PostgresStore) QuoteByID(ctx context.Context, id string) (Quote, error) {
	return scanQuote(s.database.QueryRowContext(ctx, quoteSelect+` WHERE id = $1`, id))
}

func (s *PostgresStore) CreateHold(ctx context.Context, quoteID, workspaceID string, now time.Time, maximumDebit int64, signature string) (Quote, error) {
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return Quote{}, err
	}
	defer tx.Rollback()
	quote, err := scanQuote(tx.QueryRowContext(ctx, quoteSelect+` WHERE id = $1 FOR UPDATE`, quoteID))
	if err != nil {
		return Quote{}, err
	}
	if quote.WorkspaceID != workspaceID {
		return Quote{}, ErrQuoteScope
	}
	if !now.Before(quote.ExpiresAt) {
		return Quote{}, ErrQuoteExpired
	}
	if quote.Hold != nil {
		return quote, tx.Commit()
	}
	wallet, err := walletForUpdate(ctx, tx, workspaceID, now)
	if err != nil {
		return Quote{}, err
	}
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
	_, err = tx.ExecContext(ctx, `
		UPDATE resource_quotes
		SET hold_id = $2, hold_status = 'held', hold_expires_at = expires_at,
			funding_authorization_id = $3, funding_maximum_debit_minor = $4,
			funding_currency = 'CNY', funding_expires_at = expires_at, funding_signature = $5
		WHERE id = $1 AND hold_id IS NULL`, quoteID, holdID, fundingID, maximumDebit, signature)
	if err != nil {
		return Quote{}, err
	}
	quote.Hold = &CapacityHold{ID: holdID, Status: "held", ExpiresAt: quote.ExpiresAt}
	quote.Funding = &FundingAuthorization{ID: fundingID, MaximumDebitMinor: maximumDebit, Currency: CurrencyCNY, ExpiresAt: quote.ExpiresAt, Signature: signature}
	if err := tx.Commit(); err != nil {
		return Quote{}, err
	}
	return quote, nil
}

const quoteSelect = `
	SELECT id, workspace_id, region_id, price_book_id, cpu_milli, memory_mib, disk_gib,
		dedicated_ip, estimated_hourly_minor, currency, expires_at, created_at,
		hold_id, hold_status, hold_expires_at, funding_authorization_id,
		funding_maximum_debit_minor, funding_currency, funding_expires_at, funding_signature
	FROM resource_quotes`

func scanQuote(row rowScanner) (Quote, error) {
	var quote Quote
	var holdID, holdStatus, fundingID, fundingCurrency, fundingSignature sql.NullString
	var holdExpires, fundingExpires sql.NullTime
	var fundingMaximum sql.NullInt64
	err := row.Scan(&quote.ID, &quote.WorkspaceID, &quote.RegionID, &quote.PriceBookID, &quote.ResourceSpec.CPUMilli, &quote.ResourceSpec.MemoryMiB, &quote.ResourceSpec.DiskGiB, &quote.DedicatedIP, &quote.EstimatedHourlyMinor, &quote.Currency, &quote.ExpiresAt, &quote.CreatedAt, &holdID, &holdStatus, &holdExpires, &fundingID, &fundingMaximum, &fundingCurrency, &fundingExpires, &fundingSignature)
	if err != nil {
		return Quote{}, err
	}
	if holdID.Valid {
		quote.Hold = &CapacityHold{ID: holdID.String, Status: holdStatus.String, ExpiresAt: holdExpires.Time}
	}
	if fundingID.Valid {
		quote.Funding = &FundingAuthorization{ID: fundingID.String, MaximumDebitMinor: fundingMaximum.Int64, Currency: fundingCurrency.String, ExpiresAt: fundingExpires.Time, Signature: fundingSignature.String}
	}
	return quote, nil
}

func (s *PostgresStore) Grant(ctx context.Context, entry LedgerEntry) (Wallet, error) {
	return s.appendEntry(ctx, entry)
}

func (s *PostgresStore) Correct(ctx context.Context, entry LedgerEntry) (Wallet, error) {
	return s.appendEntry(ctx, entry)
}

func (s *PostgresStore) appendEntry(ctx context.Context, entry LedgerEntry) (Wallet, error) {
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return Wallet{}, err
	}
	defer tx.Rollback()
	wallet, err := walletForUpdate(ctx, tx, entry.WorkspaceID, entry.OccurredAt)
	if err != nil {
		return Wallet{}, err
	}
	var current LedgerEntry
	err = tx.QueryRowContext(ctx, `
		SELECT id, workspace_id, sequence, kind, bucket, amount_minor, source_id, reason, occurred_at
		FROM ledger_entries WHERE workspace_id = $1 AND kind = $2 AND bucket = $3 AND source_id = $4`, entry.WorkspaceID, entry.Kind, entry.Bucket, entry.SourceID).
		Scan(&current.ID, &current.WorkspaceID, &current.Sequence, &current.Kind, &current.Bucket, &current.AmountMinor, &current.SourceID, &current.Reason, &current.OccurredAt)
	if err == nil {
		if current.AmountMinor != entry.AmountMinor || current.Reason != entry.Reason {
			return Wallet{}, ErrImmutable
		}
		return wallet, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Wallet{}, err
	}
	bucketBalance := wallet.CashMinor
	if entry.Bucket == BucketPromotional {
		bucketBalance = wallet.PromotionalMinor
	}
	newBucketBalance, bucketOK := safeAdd(bucketBalance, entry.AmountMinor)
	newAvailable, availableOK := safeAdd(wallet.AvailableMinor, entry.AmountMinor)
	if !bucketOK || newBucketBalance < 0 || !availableOK || newAvailable < 0 {
		return Wallet{}, ErrNegativeBalance
	}
	entry.Sequence = wallet.LedgerSequence + 1
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ledger_entries (id, workspace_id, sequence, kind, bucket, amount_minor, source_id, reason, occurred_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`, entry.ID, entry.WorkspaceID, entry.Sequence, entry.Kind, entry.Bucket, entry.AmountMinor, entry.SourceID, entry.Reason, entry.OccurredAt); err != nil {
		return Wallet{}, err
	}
	wallet.LedgerSequence = entry.Sequence
	if entry.Bucket == BucketPromotional {
		wallet.PromotionalMinor += entry.AmountMinor
	} else {
		wallet.CashMinor += entry.AmountMinor
	}
	wallet.AvailableMinor = wallet.PromotionalMinor + wallet.CashMinor
	if wallet.AvailableMinor == 0 {
		wallet.State = WalletExhausted
	} else {
		wallet.State = WalletActive
	}
	if err := updateWallet(ctx, tx, wallet, entry.OccurredAt); err != nil {
		return Wallet{}, err
	}
	return wallet, tx.Commit()
}

func (s *PostgresStore) Wallet(ctx context.Context, workspaceID string) (Wallet, error) {
	var wallet Wallet
	err := s.database.QueryRowContext(ctx, `SELECT workspace_id, currency, ledger_sequence, promotional_minor, cash_minor, state FROM wallets WHERE workspace_id = $1`, workspaceID).
		Scan(&wallet.WorkspaceID, &wallet.Currency, &wallet.LedgerSequence, &wallet.PromotionalMinor, &wallet.CashMinor, &wallet.State)
	if errors.Is(err, sql.ErrNoRows) {
		return Wallet{WorkspaceID: workspaceID, Currency: CurrencyCNY, State: WalletActive}, nil
	}
	if err != nil {
		return Wallet{}, err
	}
	wallet.AvailableMinor = wallet.PromotionalMinor + wallet.CashMinor
	return wallet, nil
}

func (s *PostgresStore) PostUsage(ctx context.Context, records []UsageRecord, now time.Time) (DebitResult, error) {
	if len(records) == 0 {
		return DebitResult{}, nil
	}
	workspaceID := records[0].WorkspaceID
	ids := make([]string, len(records))
	for index, record := range records {
		if record.WorkspaceID != workspaceID {
			return DebitResult{}, ErrQuoteScope
		}
		ids[index] = record.ID
	}
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return DebitResult{}, err
	}
	defer tx.Rollback()
	wallet, err := walletForUpdate(ctx, tx, workspaceID, now)
	if err != nil {
		return DebitResult{}, err
	}
	existing, err := loadUsageByIDs(ctx, tx, ids)
	if err != nil {
		return DebitResult{}, err
	}
	var fresh []UsageRecord
	var requested int64
	for _, record := range records {
		if current, exists := existing[record.ID]; exists {
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
	if len(fresh) == 0 {
		return DebitResult{Wallet: wallet}, tx.Commit()
	}
	payload, err := json.Marshal(fresh)
	if err != nil {
		return DebitResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO usage_records (id, workspace_id, logical_instance_id, region_id, price_book_id, resource_kind, quantity, interval_start, interval_end, charge_minor, created_at)
		SELECT x.id, x."workspaceId", x."logicalInstanceId", x."regionId", x."priceBookId", x."resourceKind", x.quantity, x."intervalStart", x."intervalEnd", x."chargeMinor", $2
		FROM jsonb_to_recordset($1::jsonb) AS x(id text, "workspaceId" text, "logicalInstanceId" text, "regionId" text, "priceBookId" text, "resourceKind" text, quantity bigint, "intervalStart" timestamptz, "intervalEnd" timestamptz, "chargeMinor" bigint)`, string(payload), now); err != nil {
		return DebitResult{}, err
	}
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
	if promotionalDebit > 0 {
		wallet.LedgerSequence++
		entry := usageDebitEntry(workspaceID, BucketPromotional, -promotionalDebit, fresh, wallet.LedgerSequence, now)
		if err := insertLedgerEntry(ctx, tx, entry); err != nil {
			return DebitResult{}, err
		}
		wallet.PromotionalMinor -= promotionalDebit
		entries = append(entries, entry)
	}
	if cashDebit > 0 {
		wallet.LedgerSequence++
		entry := usageDebitEntry(workspaceID, BucketCash, -cashDebit, fresh, wallet.LedgerSequence, now)
		if err := insertLedgerEntry(ctx, tx, entry); err != nil {
			return DebitResult{}, err
		}
		wallet.CashMinor -= cashDebit
		entries = append(entries, entry)
	}
	wallet.AvailableMinor = wallet.PromotionalMinor + wallet.CashMinor
	if requested > 0 && wallet.AvailableMinor == 0 {
		wallet.State = WalletExhausted
	}
	if err := updateWallet(ctx, tx, wallet, now); err != nil {
		return DebitResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return DebitResult{}, err
	}
	return DebitResult{Entries: entries, Wallet: wallet, RequestedMinor: requested, CollectedMinor: collected, UnfundedMinor: requested - collected, BalanceExhausted: wallet.State == WalletExhausted}, nil
}

func (s *PostgresStore) LedgerEntries(ctx context.Context, workspaceID string, limit int) ([]LedgerEntry, error) {
	rows, err := s.database.QueryContext(ctx, `
		SELECT id, workspace_id, sequence, kind, bucket, amount_minor, source_id, reason, occurred_at
		FROM ledger_entries WHERE workspace_id = $1 ORDER BY sequence DESC LIMIT $2`, workspaceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []LedgerEntry
	for rows.Next() {
		var entry LedgerEntry
		if err := rows.Scan(&entry.ID, &entry.WorkspaceID, &entry.Sequence, &entry.Kind, &entry.Bucket, &entry.AmountMinor, &entry.SourceID, &entry.Reason, &entry.OccurredAt); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func loadUsageByIDs(ctx context.Context, tx *sql.Tx, ids []string) (map[string]UsageRecord, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, workspace_id, logical_instance_id, region_id, price_book_id, resource_kind, quantity, interval_start, interval_end, charge_minor
		FROM usage_records WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]UsageRecord, len(ids))
	for rows.Next() {
		var record UsageRecord
		if err := rows.Scan(&record.ID, &record.WorkspaceID, &record.LogicalInstanceID, &record.RegionID, &record.PriceBookID, &record.ResourceKind, &record.Quantity, &record.IntervalStart, &record.IntervalEnd, &record.ChargeMinor); err != nil {
			return nil, err
		}
		result[record.ID] = record
	}
	return result, rows.Err()
}

func walletForUpdate(ctx context.Context, tx *sql.Tx, workspaceID string, now time.Time) (Wallet, error) {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO wallets (workspace_id, currency, ledger_sequence, promotional_minor, cash_minor, state, created_at, updated_at)
		VALUES ($1, 'CNY', 0, 0, 0, 'active', $2, $2)
		ON CONFLICT (workspace_id) DO NOTHING`, workspaceID, now); err != nil {
		return Wallet{}, err
	}
	var wallet Wallet
	if err := tx.QueryRowContext(ctx, `SELECT workspace_id, currency, ledger_sequence, promotional_minor, cash_minor, state FROM wallets WHERE workspace_id = $1 FOR UPDATE`, workspaceID).
		Scan(&wallet.WorkspaceID, &wallet.Currency, &wallet.LedgerSequence, &wallet.PromotionalMinor, &wallet.CashMinor, &wallet.State); err != nil {
		return Wallet{}, err
	}
	wallet.AvailableMinor = wallet.PromotionalMinor + wallet.CashMinor
	return wallet, nil
}

func updateWallet(ctx context.Context, tx *sql.Tx, wallet Wallet, now time.Time) error {
	_, err := tx.ExecContext(ctx, `UPDATE wallets SET ledger_sequence = $2, promotional_minor = $3, cash_minor = $4, state = $5, updated_at = $6 WHERE workspace_id = $1`, wallet.WorkspaceID, wallet.LedgerSequence, wallet.PromotionalMinor, wallet.CashMinor, wallet.State, now)
	return err
}

func insertLedgerEntry(ctx context.Context, tx *sql.Tx, entry LedgerEntry) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO ledger_entries (id, workspace_id, sequence, kind, bucket, amount_minor, source_id, reason, occurred_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`, entry.ID, entry.WorkspaceID, entry.Sequence, entry.Kind, entry.Bucket, entry.AmountMinor, entry.SourceID, entry.Reason, entry.OccurredAt)
	return err
}

type rowScanner interface {
	Scan(...any) error
}
