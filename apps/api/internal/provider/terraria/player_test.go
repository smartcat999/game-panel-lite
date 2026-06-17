package terraria

import (
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestTerrariaProvidersExposePlayerListCommand(t *testing.T) {
	chineseConfig := NewVanillaProvider().DefaultConfig()
	chineseConfig.Language = "zh-Hans"
	englishConfig := chineseConfig
	englishConfig.Language = "en-US"

	if command := NewVanillaProvider().PlayerListCommand(chineseConfig); command != "游戏中" {
		t.Fatalf("expected localized vanilla player list command 游戏中, got %q", command)
	}
	if command := NewVanillaProvider().PlayerListCommand(englishConfig); command != "playing" {
		t.Fatalf("expected English vanilla player list command playing, got %q", command)
	}
	if command := NewTModLoaderProvider().PlayerListCommand(chineseConfig); command != "游戏中" {
		t.Fatalf("expected localized tModLoader player list command 游戏中, got %q", command)
	}
	if command := NewTModLoaderProvider().PlayerListCommand(englishConfig); command != "playing" {
		t.Fatalf("expected English tModLoader player list command playing, got %q", command)
	}
}

func TestParsePlayerListOutput(t *testing.T) {
	provider := NewVanillaProvider()

	players := provider.ParsePlayerListOutput([]string{
		"$ playing",
		"Players connected: Alice, Bob",
	})
	if len(players) != 2 || players[0].Name != "Alice" || players[1].Name != "Bob" {
		t.Fatalf("expected named players, got %+v", players)
	}

	players = provider.ParsePlayerListOutput([]string{
		"$ playing",
		"There are 2 players connected.",
	})
	if len(players) != 2 {
		t.Fatalf("expected count-only output to preserve online count, got %+v", players)
	}

	players = provider.ParsePlayerListOutput([]string{
		"[12:34:56] There are 3 players connected.",
	})
	if len(players) != 3 {
		t.Fatalf("expected timestamped count output to preserve online count, got %+v", players)
	}

	players = provider.ParsePlayerListOutput([]string{
		"$ playing",
		"No players connected.",
	})
	if players == nil || len(players) != 0 {
		t.Fatalf("expected recognized empty player list, got %+v", players)
	}

	players = provider.ParsePlayerListOutput([]string{
		": 无玩家连接。",
	})
	if players == nil || len(players) != 0 {
		t.Fatalf("expected recognized localized empty player list, got %+v", players)
	}

	players = provider.ParsePlayerListOutput([]string{
		": yyds (192.168.215.1:32643)",
		"",
		"1个玩家已连接。",
	})
	if len(players) != 1 || players[0].Name != "yyds" {
		t.Fatalf("expected localized named player, got %+v", players)
	}

	players = provider.ParsePlayerListOutput([]string{"Server started"})
	if players != nil {
		t.Fatalf("expected unrelated logs to be ignored, got %+v", players)
	}
}

func TestParsePlayerLogEvent(t *testing.T) {
	p := NewVanillaProvider()

	cases := []struct {
		line string
		want domain.PlayerLogEvent
	}{
		{"Alice has joined.", domain.PlayerJoined},
		{": Bob has left.", domain.PlayerLeft},
		{"222已加入。", domain.PlayerJoined},
		{": yyds已离开。", domain.PlayerLeft},
	}
	for _, item := range cases {
		got, ok := p.ParsePlayerLogEvent(item.line)
		if !ok || got != item.want {
			t.Fatalf("expected %q to parse as %s, got %s ok=%v", item.line, item.want, got, ok)
		}
	}

	if _, ok := p.ParsePlayerLogEvent("Server started"); ok {
		t.Fatal("expected unrelated log line to be ignored")
	}
}
