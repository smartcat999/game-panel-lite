CREATE TABLE IF NOT EXISTS game_server_instances (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  game_key TEXT NOT NULL,
  provider_key TEXT NOT NULL,
  status TEXT NOT NULL,
  world_name TEXT NOT NULL,
  port INTEGER NOT NULL,
  max_players INTEGER NOT NULL,
  password TEXT,
  data_dir TEXT NOT NULL,
  container_id TEXT,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);
