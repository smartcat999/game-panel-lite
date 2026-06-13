package http

import "github.com/smartcat999/game-panel-lite/apps/api/internal/domain"

type terrariaPreviewConfig domain.TerrariaConfig

func (c terrariaPreviewConfig) ToDomain() domain.TerrariaConfig {
	return domain.TerrariaConfig(c)
}
