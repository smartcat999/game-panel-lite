package dst

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func (Provider) BackupDirectory(server domain.GameServer) (string, error) {
	config := configFromPayload(server.Spec.Config, defaultConfig())
	name := config.Identity.ClusterName
	if strings.TrimSpace(name) == "" || strings.ContainsAny(name, "/\\\x00\r\n") || strings.Contains(name, "..") || name == "." {
		return "", fmt.Errorf("invalid DST cluster name for backup")
	}
	return clusterConfigDir(config), nil
}

var backupNoncePattern = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

func (p Provider) BackupSaveCommands(server domain.GameServer, nonce string) ([]domain.BackupSaveCommand, error) {
	if !backupNoncePattern.MatchString(nonce) {
		return nil, fmt.Errorf("invalid backup save nonce")
	}
	config := configFromPayload(server.Spec.Config, defaultConfig())
	targets := []string{"master"}
	if config.Caves != nil && config.Caves.Enabled {
		targets = append(targets, "caves")
	}
	commands := make([]domain.BackupSaveCommand, 0, len(targets))
	for _, target := range targets {
		marker := "GAMEPANEL_SAVE_" + nonce + "_" + target
		failed := "GAMEPANEL_SAVE_FAILED_" + nonce + "_" + target
		// c_save() only queues an autosave event. Use the same native save routine
		// with its asynchronous completion callback; never acknowledge after enqueue.
		lua := fmt.Sprintf(`if TheWorld ~= nil and TheWorld.ismastersim and TheNet:GetIsServer() and ShardGameIndex ~= nil then local ok = pcall(function() ShardGameIndex:SaveCurrent(function(success) if success == false then print(%q) else print(%q) end end) end); if not ok then print(%q) end else print(%q) end`, failed, marker, failed, failed)
		command, err := p.ConsoleCommand(server, lua, target)
		if err != nil {
			return nil, err
		}
		commands = append(commands, domain.BackupSaveCommand{Name: target, Command: command, Confirmation: marker, Failure: failed})
	}
	return commands, nil
}
