package store

import "gorm.io/gorm"

func migrateSQLiteNodePorts(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("CREATE TABLE IF NOT EXISTS gamepanel_sqlite_migrations (version integer PRIMARY KEY)").Error; err != nil {
			return err
		}
		var applied int64
		if err := tx.Table("gamepanel_sqlite_migrations").Where("version = 2").Count(&applied).Error; err != nil {
			return err
		}
		if applied != 0 {
			return nil
		}
		for _, sql := range []string{
			`CREATE TABLE node_port_pools (node_id text PRIMARY KEY)`,
			`CREATE TABLE node_port_reservations (node_id text NOT NULL, host_port integer NOT NULL CHECK(typeof(host_port) = 'integer' AND host_port BETWEEN 1 AND 65535), server_id text NOT NULL, PRIMARY KEY(node_id,host_port,server_id))`,
			`CREATE INDEX idx_node_port_reservations_server ON node_port_reservations(server_id)`,
		} {
			if err := tx.Exec(sql).Error; err != nil {
				return err
			}
		}
		if err := backfillNodePorts(tx); err != nil {
			return err
		}
		return tx.Exec("INSERT INTO gamepanel_sqlite_migrations(version) VALUES(2)").Error
	})
}
