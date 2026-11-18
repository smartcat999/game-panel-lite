package pgutil

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"time"

	"gorm.io/gorm"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
)

// ParseVersion parses a strictly positive 64-bit decimal version string.
func ParseVersion(raw string) (int64, error) {
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || v < 1 {
		return 0, db.ErrConflict
	}
	return v, nil
}

// FormatVersion formats a positive integer version for ObjectMeta.
func FormatVersion(v int64) string {
	if v < 1 {
		return ""
	}
	return strconv.FormatInt(v, 10)
}

// NonNilMap returns an empty map when m is nil so JSONB columns serialize as '{}' instead of 'null'.
func NonNilMap(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	cp := make(map[string]string, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}

// NonNilStrings returns a non-nil slice copy so JSONB columns serialize as '[]' instead of 'null'.
func NonNilStrings(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	return append([]string(nil), in...)
}

// NonNilConditions returns a non-nil condition slice copy.
func NonNilConditions(in []metav1.Condition) []metav1.Condition {
	if len(in) == 0 {
		return []metav1.Condition{}
	}
	return append([]metav1.Condition(nil), in...)
}

// MarshalJSONCol marshals v for explicit GORM map[string]any JSONB column updates.
func MarshalJSONCol(v any) ([]byte, error) {
	return json.Marshal(v)
}

// InitCreateMeta populates server-owned UID, Generation=1, ResourceVersion="1", and CreationTimestamp.
func InitCreateMeta(meta *metav1.ObjectMeta) {
	if meta.UID == "" {
		meta.UID = db.NewUID()
	}
	meta.Generation = 1
	meta.ResourceVersion = "1"
	if meta.CreationTimestamp.IsZero() {
		meta.CreationTimestamp = time.Now().UTC()
	}
}

// SortedFieldKeys returns deterministic sorted keys from a FieldSelector map.
func SortedFieldKeys(selectors map[string]string) []string {
	if len(selectors) == 0 {
		return nil
	}
	keys := make([]string, 0, len(selectors))
	for k := range selectors {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Paginate executes optional Count + Order/Limit/Offset + Find for strongly-typed repository List queries.
func Paginate[R any](baseTx *gorm.DB, orderSQL string, opts db.ListOptions) ([]R, int, error) {
	offset, limit := max(opts.Offset, 0), max(opts.Limit, 0)
	var totalCount int64
	if !opts.SkipTotalCount && (limit > 0 || offset > 0) {
		if err := baseTx.Count(&totalCount).Error; err != nil {
			return nil, 0, err
		}
		if totalCount == 0 || offset >= int(totalCount) {
			return []R{}, int(totalCount), nil
		}
	}
	queryTx := baseTx.Session(&gorm.Session{})
	if orderSQL != "" {
		queryTx = queryTx.Order(orderSQL)
	}
	if limit > 0 {
		queryTx = queryTx.Limit(limit)
	}
	if offset > 0 {
		queryTx = queryTx.Offset(offset)
	}
	var rows []R
	if err := queryTx.Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	if limit == 0 && offset == 0 {
		totalCount = int64(len(rows))
	}
	return rows, int(totalCount), nil
}

// NextContinue returns the decimal offset cursor for the next page when additional rows remain.
func NextContinue(offset, count, total int) string {
	if count <= 0 {
		return ""
	}
	next := max(offset, 0) + count
	if next < total {
		return strconv.Itoa(next)
	}
	return ""
}

// DeleteByCondition deletes rows matching whereSQL, or conditionally requires matching UID and zero finalizers when expectedUID != "".
func DeleteByCondition(ctx context.Context, orm *gorm.DB, table, keyWhereSQL string, keyArgs []any, expectedUID string) error {
	if orm == nil {
		return errors.New("db is nil")
	}
	if expectedUID == "" {
		res := orm.WithContext(ctx).Table(table).Where(keyWhereSQL, keyArgs...).Delete(map[string]any{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return db.ErrNotFound
		}
		return nil
	}
	var outcome struct {
		Deleted bool `gorm:"column:deleted"`
		Exists  bool `gorm:"column:exists"`
	}
	sql := `WITH deleted AS (
		DELETE FROM ` + table + `
		WHERE ` + keyWhereSQL + `
		  AND uid = ?
		  AND jsonb_array_length(COALESCE(finalizers, '[]'::jsonb)) = 0
		RETURNING 1
	)
	SELECT EXISTS (SELECT 1 FROM deleted) AS deleted,
	       EXISTS (SELECT 1 FROM ` + table + ` WHERE ` + keyWhereSQL + `) AS exists`
	args := make([]any, 0, len(keyArgs)*2+1)
	args = append(args, keyArgs...)
	args = append(args, expectedUID)
	args = append(args, keyArgs...)
	if err := orm.WithContext(ctx).Raw(sql, args...).Scan(&outcome).Error; err != nil {
		return err
	}
	if outcome.Deleted {
		return nil
	}
	if !outcome.Exists {
		return db.ErrNotFound
	}
	return db.ErrConflict
}
