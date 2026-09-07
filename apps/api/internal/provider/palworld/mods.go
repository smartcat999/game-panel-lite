package palworld

import (
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"path/filepath"
)

func (Provider) RuntimeModFiles(filename string) []string {
	return []string{filepath.Join("Pal", "Content", "Paks", "~mods", filename)}
}

func (Provider) ModSupport() domain.ModSupport {
	return domain.ModSupport{UploadExtensions: []string{".pak"}}
}
