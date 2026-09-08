package store

import (
	"gorm.io/gorm/schema"
	"testing"
)

// Exercise GORM's table naming with the no-DDL PostgreSQL runtime role. The old
// row was inserted at migration 010, before the table was renamed by 012.
func testOAuthTableMigration(t *testing.T, db *Store) {
	t.Helper()
	table := schema.NamingStrategy{}.TableName("OAuthIdentity")
	var identity struct{ ID, UserID, Provider, ProviderUserID, Email, Name string }
	if err := db.db.Table(table).Where("provider = ? AND provider_user_id = ?", "test-provider", "remote-subject").Take(&identity).Error; err != nil {
		t.Fatalf("OAuth runtime lookup: %v", err)
	}
	if identity.ID != "legacy-oauth-link" || identity.UserID != "legacy-oauth-user" || identity.Email != "original@example.test" || identity.Name != "Original" {
		t.Fatal("OAuth migration lost existing account binding")
	}
	duplicate := map[string]any{"id": "duplicate-oauth-link", "user_id": "another-user", "provider": identity.Provider, "provider_user_id": identity.ProviderUserID}
	if err := db.db.Table(table).Create(duplicate).Error; err == nil {
		t.Fatal("migration lost unique provider identity constraint")
	}
	other := map[string]any{"id": "other-provider-link", "user_id": "another-user", "provider": "other-provider", "provider_user_id": identity.ProviderUserID}
	if err := db.db.Table(table).Create(other).Error; err != nil {
		t.Fatalf("independent provider identity: %v", err)
	}
	result := db.db.Table(table).Where("id = ?", identity.ID).Update("name", "Updated")
	if result.Error != nil || result.RowsAffected != 1 {
		t.Fatalf("runtime OAuth update: %v", result.Error)
	}
	if err := db.db.Table(table).Where("id = ?", identity.ID).Take(&identity).Error; err != nil || identity.Name != "Updated" {
		t.Fatalf("runtime OAuth read after update: %v", err)
	}
	var oldTableCount int64
	if err := db.db.Raw("SELECT count(*) FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = 'oauth_identities'").Scan(&oldTableCount).Error; err != nil || oldTableCount != 0 {
		t.Fatalf("obsolete OAuth table remains: %v", err)
	}
	if err := db.db.Exec("DELETE FROM o_auth_identities WHERE id IN (?, ?)", identity.ID, "other-provider-link").Error; err != nil {
		t.Fatal(err)
	}
}
