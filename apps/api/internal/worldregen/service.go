package worldregen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/safety"
)

type Move struct {
	Original   string
	Quarantine string
}

type Service struct{}

func NewService() *Service { return &Service{} }

func (s *Service) Quarantine(root string, relativePaths []string, jobID string) ([]Move, error) {
	if !safeToken(jobID) {
		return nil, fmt.Errorf("invalid regeneration job id")
	}
	moves := make([]Move, 0, len(relativePaths))
	for _, relativePath := range relativePaths {
		target, exists, err := safeExistingPath(root, relativePath)
		if err != nil {
			_ = s.Restore(moves)
			return nil, err
		}
		if !exists {
			continue
		}
		quarantine := filepath.Join(filepath.Dir(target), ".gamepanel-reset-"+jobID)
		if _, err := os.Lstat(quarantine); err == nil {
			_ = s.Restore(moves)
			return nil, fmt.Errorf("world regeneration quarantine already exists")
		} else if !os.IsNotExist(err) {
			_ = s.Restore(moves)
			return nil, err
		}
		if err := os.Rename(target, quarantine); err != nil {
			_ = s.Restore(moves)
			return nil, fmt.Errorf("quarantine %s: %w", relativePath, err)
		}
		moves = append(moves, Move{Original: target, Quarantine: quarantine})
	}
	return moves, nil
}

func (s *Service) Restore(moves []Move) error {
	for index := len(moves) - 1; index >= 0; index-- {
		move := moves[index]
		if _, err := os.Lstat(move.Quarantine); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return err
		}
		if err := os.RemoveAll(move.Original); err != nil {
			return fmt.Errorf("remove generated world before rollback: %w", err)
		}
		if err := os.Rename(move.Quarantine, move.Original); err != nil {
			return fmt.Errorf("restore previous world: %w", err)
		}
	}
	return nil
}

func (s *Service) Commit(moves []Move) error {
	for _, move := range moves {
		if err := os.RemoveAll(move.Quarantine); err != nil {
			return fmt.Errorf("remove previous world quarantine: %w", err)
		}
	}
	return nil
}

func (s *Service) FindQuarantine(root string, relativePaths []string, jobID string) ([]Move, error) {
	if !safeToken(jobID) {
		return nil, fmt.Errorf("invalid regeneration job id")
	}
	moves := make([]Move, 0, len(relativePaths))
	for _, relativePath := range relativePaths {
		if filepath.IsAbs(relativePath) {
			return nil, fmt.Errorf("world save path must be relative")
		}
		cleanRelative := filepath.Clean(relativePath)
		if cleanRelative == "." || cleanRelative == ".." || strings.HasPrefix(cleanRelative, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("invalid world save path")
		}
		original, err := safety.SafeJoin(root, cleanRelative)
		if err != nil {
			return nil, err
		}
		quarantineRelative := filepath.Join(filepath.Dir(cleanRelative), ".gamepanel-reset-"+jobID)
		quarantine, exists, err := safeExistingPath(root, quarantineRelative)
		if err != nil {
			return nil, err
		}
		if exists {
			moves = append(moves, Move{Original: original, Quarantine: quarantine})
		}
	}
	return moves, nil
}

func safeExistingPath(root string, relativePath string) (string, bool, error) {
	if filepath.IsAbs(relativePath) {
		return "", false, fmt.Errorf("world save path must be relative")
	}
	cleanRelative := filepath.Clean(relativePath)
	if cleanRelative == "." || cleanRelative == ".." || strings.HasPrefix(cleanRelative, ".."+string(filepath.Separator)) {
		return "", false, fmt.Errorf("invalid world save path")
	}
	target, err := safety.SafeJoin(root, cleanRelative)
	if err != nil {
		return "", false, err
	}
	info, err := os.Lstat(target)
	if os.IsNotExist(err) {
		return target, false, nil
	}
	if err != nil {
		return "", false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", false, fmt.Errorf("world save path cannot be a symbolic link")
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", false, fmt.Errorf("resolve server data directory: %w", err)
	}
	realTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", false, fmt.Errorf("resolve world save path: %w", err)
	}
	if realTarget != realRoot && !strings.HasPrefix(realTarget, realRoot+string(filepath.Separator)) {
		return "", false, fmt.Errorf("world save path resolves outside server data directory")
	}
	return target, true, nil
}

func safeToken(value string) bool {
	if value == "" || len(value) > 80 {
		return false
	}
	for _, item := range value {
		if !unicode.IsLetter(item) && !unicode.IsDigit(item) && item != '-' && item != '_' {
			return false
		}
	}
	return true
}
