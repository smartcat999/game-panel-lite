package http

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
)

func (h *Handler) confirmBackupSave(ctx context.Context, server domain.GameServer, saver provider.OnlineBackupProvider) error {
	if server.Status.RuntimeID == "" {
		return fmt.Errorf("cannot confirm game save without an attached runtime")
	}
	commands, err := saver.BackupSaveCommands(server, uuid.NewString())
	if err != nil {
		return err
	}
	if len(commands) == 0 {
		return fmt.Errorf("provider returned no backup save commands")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	// Subscribe before sending: a fast save must not finish before log capture starts.
	stream, err := h.runtime.LogsWorkload(ctx, server.Status.RuntimeID, true)
	if err != nil {
		return fmt.Errorf("subscribe to save confirmations: %w", err)
	}
	defer stream.Close()
	done := make(chan error, 1)
	go func() { done <- awaitBackupSaves(stream, commands) }()
	for _, command := range commands {
		if err := h.runtime.SendCommandWorkload(ctx, server.Status.RuntimeID, command.Command); err != nil {
			return fmt.Errorf("request %s save: %w", command.Name, err)
		}
	}
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return fmt.Errorf("game save confirmation did not complete; backup not created: %w", ctx.Err())
	}
}

func awaitBackupSaves(stream io.Reader, commands []domain.BackupSaveCommand) error {
	pending := make(map[string]string, len(commands))
	for _, command := range commands {
		if command.Confirmation == "" {
			return fmt.Errorf("provider returned an empty save confirmation")
		}
		pending[command.Confirmation] = command.Name
	}
	scanner := bufio.NewScanner(stream)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		for _, command := range commands {
			if command.Failure != "" && strings.HasSuffix(line, command.Failure) {
				return fmt.Errorf("%s game save failed; backup not created", command.Name)
			}
		}
		for marker := range pending {
			if strings.HasSuffix(line, marker) {
				delete(pending, marker)
			}
		}
		if len(pending) == 0 {
			return nil
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read game save confirmation: %w", err)
	}
	missing := make([]string, 0, len(pending))
	for _, name := range pending {
		missing = append(missing, name)
	}
	sort.Strings(missing)
	return fmt.Errorf("game save confirmation stream ended before %s completed", strings.Join(missing, ", "))
}
