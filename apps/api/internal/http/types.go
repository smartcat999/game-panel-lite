package http

import "github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"

type terrariaPreviewConfig terraria.Config

func (c terrariaPreviewConfig) ToDomain() terraria.Config {
	return terraria.Config(c)
}
