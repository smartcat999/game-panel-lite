package store

import "gorm.io/gorm"

func migrateSQLiteOutbox(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var applied int64
		if err := tx.Table("gamepanel_sqlite_migrations").Where("version = 4").Count(&applied).Error; err != nil {
			return err
		}
		if applied != 0 {
			return nil
		}
		if err := tx.Exec(outboxPublicationSQL).Error; err != nil {
			return err
		}
		return tx.Exec("INSERT INTO gamepanel_sqlite_migrations(version) VALUES(4)").Error
	})
}
