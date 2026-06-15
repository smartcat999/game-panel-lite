package terraria

import "testing"

func TestTerrariaProvidersExposePlayingCommand(t *testing.T) {
	if command := NewVanillaProvider().PlayerListCommand(); command != "playing" {
		t.Fatalf("expected vanilla player list command playing, got %q", command)
	}
	if command := NewTModLoaderProvider().PlayerListCommand(); command != "playing" {
		t.Fatalf("expected tModLoader player list command playing, got %q", command)
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

	players = provider.ParsePlayerListOutput([]string{"Server started"})
	if players != nil {
		t.Fatalf("expected unrelated logs to be ignored, got %+v", players)
	}
}
