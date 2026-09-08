package store

import "gorm.io/gorm"

func migrateSQLiteGlobalInstances(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var applied int64
		if err := tx.Table("gamepanel_sqlite_migrations").Where("version = 3").Count(&applied).Error; err != nil {
			return err
		}
		if applied != 0 {
			return nil
		}
		if err := tx.Exec(globalInstanceSchemaSQL).Error; err != nil {
			return err
		}
		for _, statement := range []string{
			`CREATE TRIGGER server_revisions_no_replace BEFORE INSERT ON server_revisions WHEN EXISTS (SELECT 1 FROM server_revisions WHERE id = NEW.id OR (server_id = NEW.server_id AND spec_generation = NEW.spec_generation)) BEGIN SELECT RAISE(ABORT,'server revisions are immutable'); END`,
			`CREATE TRIGGER server_revisions_no_update BEFORE UPDATE ON server_revisions BEGIN SELECT RAISE(ABORT,'server revisions are immutable'); END`,
			`CREATE TRIGGER server_revisions_no_delete BEFORE DELETE ON server_revisions BEGIN SELECT RAISE(ABORT,'server revisions are immutable'); END`,
			`INSERT INTO gamepanel_sqlite_migrations(version) VALUES(3)`,
		} {
			if err := tx.Exec(statement).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
