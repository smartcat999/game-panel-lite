package dst

import "github.com/smartcat999/game-panel-lite/apps/api/internal/domain"

func (Provider) ModSupport() domain.ModSupport { return domain.ModSupport{Workshop: true} }
