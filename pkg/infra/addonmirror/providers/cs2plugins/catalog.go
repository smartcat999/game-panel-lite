package cs2plugins

import (
	"strings"

	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror/driver"
)

const (
	PackageMetamod            = "metamod"
	PackageCounterStrikeSharp = "counterstrikesharp"
)

type curatedPlugin struct {
	ID           string
	Owner        string
	Repo         string
	Title        string
	Summary      string
	Author       string
	Tags         []string
	Dependencies []driver.Dependency
}

var curatedCatalog = []curatedPlugin{
	{
		ID:      PackageMetamod,
		Title:   "Metamod:Source 2.0 (CS2 插件加载框架)",
		Summary: "Counter-Strike 2 核心底层加载器，CounterStrikeSharp 与多数服务端插件的必需前置。",
		Author:  "AlliedModders",
		Tags:    []string{"framework", "metamod", "loader"},
	},
	{
		ID:           PackageCounterStrikeSharp,
		Owner:        "roflmuffin",
		Repo:         "CounterStrikeSharp",
		Title:        "CounterStrikeSharp (.NET 运行时插件框架)",
		Summary:      "基于 Metamod:Source 的现代 C# (.NET 8) CS2 服务端插件运行框架（含 Linux 运行时）。",
		Author:       "roflmuffin",
		Tags:         []string{"framework", "csharp", "dotnet"},
		Dependencies: []driver.Dependency{{ID: PackageMetamod, Title: "Metamod:Source 2.0 (CS2 插件加载框架)"}},
	},
	{
		ID:           "shobhit-pathak--MatchZy",
		Owner:        "shobhit-pathak",
		Repo:         "MatchZy",
		Title:        "MatchZy (比赛管理与跑图练习)",
		Summary:      "CS2 流行的 5v5 竞赛、BP 选图、加时赛、数据统计与战队跑图练习综合插件。",
		Author:       "shobhit-pathak",
		Tags:         []string{"match", "competitive", "practice"},
		Dependencies: []driver.Dependency{{ID: PackageCounterStrikeSharp, Title: "CounterStrikeSharp (.NET 运行时插件框架)"}},
	},
	{
		ID:           "daffyyyy--CS2-SimpleAdmin",
		Owner:        "daffyyyy",
		Repo:         "CS2-SimpleAdmin",
		Title:        "CS2-SimpleAdmin (管理员与封禁系统)",
		Summary:      "常用 CS2 管理员插件，支持踢人、封禁、禁言、换图、VIP 与权限管理。",
		Author:       "daffyyyy",
		Tags:         []string{"admin", "moderation"},
		Dependencies: []driver.Dependency{{ID: PackageCounterStrikeSharp, Title: "CounterStrikeSharp (.NET 运行时插件框架)"}},
	},
	{
		ID:           "B3none--cs2-retakes",
		Owner:        "B3none",
		Repo:         "cs2-retakes",
		Title:        "CS2 Retakes (残局回防模式)",
		Summary:      "自动分配攻防阵营、经济发枪与随机包点开局的经典 CS2 回防训练插件。",
		Author:       "B3none",
		Tags:         []string{"retakes", "gamemode", "training"},
		Dependencies: []driver.Dependency{{ID: PackageCounterStrikeSharp, Title: "CounterStrikeSharp (.NET 运行时插件框架)"}},
	},
	{
		ID:           "abnerfs--cs2-rockthevote",
		Owner:        "abnerfs",
		Repo:         "cs2-rockthevote",
		Title:        "CS2 RockTheVote (玩家投票换图 RTV)",
		Summary:      "支持 !rtv 投票换图、!nominate 提名地图及回合末投票切换地图（支持工坊地图）。",
		Author:       "abnerfs",
		Tags:         []string{"rtv", "mapvote", "utility"},
		Dependencies: []driver.Dependency{{ID: PackageCounterStrikeSharp, Title: "CounterStrikeSharp (.NET 运行时插件框架)"}},
	},
	{
		ID:           "K4ryuu--K4-System",
		Owner:        "K4ryuu",
		Repo:         "K4-System",
		Title:        "K4-System (段位积分与数据统计)",
		Summary:      "包含玩家段位积分、在线时长、MVP 与击杀排行榜的模块化 CS2 统计插件。",
		Author:       "K4ryuu",
		Tags:         []string{"ranks", "stats", "vip"},
		Dependencies: []driver.Dependency{{ID: PackageCounterStrikeSharp, Title: "CounterStrikeSharp (.NET 运行时插件框架)"}},
	},
}

func findCurated(id string) (curatedPlugin, bool) {
	trimmed := strings.TrimSpace(id)
	for _, c := range curatedCatalog {
		if strings.EqualFold(c.ID, trimmed) || (c.Repo != "" && strings.EqualFold(c.Repo, trimmed)) {
			return c, true
		}
	}
	return curatedPlugin{}, false
}

func parseGitHubSlug(id string) (owner, repo string, isCSS, ok bool) {
	trimmed := strings.TrimSpace(id)
	if strings.EqualFold(trimmed, PackageCounterStrikeSharp) {
		return "roflmuffin", "CounterStrikeSharp", true, true
	}
	if c, found := findCurated(trimmed); found && c.Owner != "" && c.Repo != "" {
		return c.Owner, c.Repo, c.ID == PackageCounterStrikeSharp, true
	}
	if o, r, splitOK := strings.Cut(trimmed, "--"); splitOK && isValidSeg(o) && isValidSeg(r) {
		isCSS = strings.EqualFold(o, "roflmuffin") && strings.EqualFold(r, "CounterStrikeSharp")
		return o, r, isCSS, true
	}
	if o, r, splitOK := strings.Cut(trimmed, "/"); splitOK && isValidSeg(o) && isValidSeg(r) {
		isCSS = strings.EqualFold(o, "roflmuffin") && strings.EqualFold(r, "CounterStrikeSharp")
		return o, r, isCSS, true
	}
	return "", "", false, false
}

func isValidSeg(s string) bool {
	return s != "" && !strings.ContainsAny(s, "/\\ \t\r\n?&#")
}

func toCanonicalPackageID(owner, repo string) string {
	if strings.EqualFold(owner, "roflmuffin") && strings.EqualFold(repo, "CounterStrikeSharp") {
		return PackageCounterStrikeSharp
	}
	if c, ok := findCurated(owner + "--" + repo); ok {
		return c.ID
	}
	return owner + "--" + repo
}

func curatedBaseItem(c curatedPlugin) driver.Item {
	srcURL := "https://www.sourcemm.net/downloads.php/?branch=master"
	if c.Owner != "" && c.Repo != "" {
		srcURL = "https://github.com/" + c.Owner + "/" + c.Repo
	}
	return driver.Item{
		ID:           c.ID,
		Title:        c.Title,
		Summary:      c.Summary,
		Author:       c.Author,
		SourceURL:    srcURL,
		UpstreamURL:  srcURL,
		Tags:         append([]string(nil), c.Tags...),
		Dependencies: append([]driver.Dependency(nil), c.Dependencies...),
	}
}

func matchesQuery(c curatedPlugin, q string) bool {
	if q == "" {
		return true
	}
	low := strings.ToLower(q)
	hay := strings.ToLower(c.ID + " " + c.Title + " " + c.Summary + " " + strings.Join(c.Tags, " "))
	return strings.Contains(hay, low)
}
