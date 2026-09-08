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
			`WITH bindings AS (
    SELECT node_id,id AS server_id,COALESCE(NULLIF(json_extract(COALESCE(NULLIF(spec,''),'{}'),'$.network.hostPort'),0),json_extract(COALESCE(NULLIF(spec,''),'{}'),'$.network.port'),0) AS host_port FROM game_servers
    UNION ALL SELECT node_id,server_id,COALESCE(NULLIF(json_extract(COALESCE(NULLIF(spec,''),'{}'),'$.network.hostPort'),0),json_extract(COALESCE(NULLIF(spec,''),'{}'),'$.network.port'),0) FROM workload_assignments
    UNION ALL SELECT a.node_id,a.server_id,COALESCE(NULLIF(json_extract(p.value,'$.hostPort'),0),json_extract(p.value,'$.port'),0) FROM workload_assignments a JOIN json_each(COALESCE(NULLIF(a.spec,''),'{}'),'$.network.additionalPorts') p
   ) INSERT INTO node_port_reservations(node_id,host_port,server_id) SELECT DISTINCT node_id,host_port,server_id FROM bindings WHERE COALESCE(node_id,'')<>'' AND host_port<>0 ON CONFLICT DO NOTHING`,
			`INSERT INTO node_port_pools(node_id) SELECT DISTINCT node_id FROM node_port_reservations WHERE true ON CONFLICT DO NOTHING`,
			`INSERT INTO gamepanel_sqlite_migrations(version) VALUES(2)`,
		} {
			if err := tx.Exec(sql).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
