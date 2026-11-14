package dst

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestBackupSaveCommandsUseNativeCompletionCallbacksForBothShards(t *testing.T) {
	p := NewProvider()
	s := domain.GameServer{Spec: domain.ServerSpec{Config: p.DefaultConfigPayload()}}
	s.Spec.Config["caves"] = map[string]any{"enabled": true}
	commands, err := p.BackupSaveCommands(s, "unique-backup")
	if err != nil {
		t.Fatal(err)
	}
	if len(commands) != 2 {
		t.Fatalf("want both shards, got %d", len(commands))
	}
	for i, target := range []string{"master", "caves"} {
		c := commands[i]
		prefix := "__GAMEPANEL_DST_CONSOLE__:" + target + ":"
		if !strings.HasPrefix(c.Command, prefix) {
			t.Fatal("incorrect shard routing")
		}
		lua, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(c.Command, prefix))
		if err != nil {
			t.Fatal(err)
		}
		for _, required := range []string{"ShardGameIndex:SaveCurrent(function(success)", "success == false", "TheWorld.ismastersim", "TheNet:GetIsServer()", c.Confirmation, c.Failure} {
			if !strings.Contains(string(lua), required) {
				t.Fatalf("missing %q in save command", required)
			}
		}
		if strings.Contains(string(lua), "c_save(); print") {
			t.Fatal("must not acknowledge command submission")
		}
	}
	s.Spec.Config["caves"] = map[string]any{"enabled": false}
	commands, err = p.BackupSaveCommands(s, "new-backup")
	if err != nil || len(commands) != 1 || commands[0].Name != "master" {
		t.Fatalf("disabled caves still required: %+v %v", commands, err)
	}
	s.Spec.Config["gameplay"] = map[string]any{"consoleEnabled": false}
	if _, err := p.BackupSaveCommands(s, "new-backup"); err == nil {
		t.Fatal("disabled console must not yield a false successful backup")
	}
}

func TestBackupDirectoryAndNonceValidation(t *testing.T) {
	p := NewProvider()
	s := domain.GameServer{Spec: domain.ServerSpec{Config: p.DefaultConfigPayload()}}
	s.Spec.Config["identity"] = map[string]any{"clusterName": "原生集群"}
	if got, err := p.BackupDirectory(s); err != nil || got != "dst/原生集群" {
		t.Fatalf("got %q %v", got, err)
	}
	for _, name := range []string{"../escape", "a/b", "a\\b", "name\x00"} {
		s.Spec.Config["identity"] = map[string]any{"clusterName": name}
		if _, err := p.BackupDirectory(s); err == nil {
			t.Fatalf("accepted unsafe name %q", name)
		}
	}
	for _, nonce := range []string{"", "\"injected", "line\nbreak"} {
		if _, err := p.BackupSaveCommands(s, nonce); err == nil {
			t.Fatal("accepted invalid nonce")
		}
	}
}
