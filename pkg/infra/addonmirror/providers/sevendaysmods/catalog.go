package sevendaysmods

import (
	"strings"

	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror/driver"
)

type curatedMod struct {
	ID      string
	Owner   string
	Repo    string
	Title   string
	Summary string
	Author  string
	Tags    []string
}

var curatedCatalog = []curatedMod{
	{
		ID:      "IceCoffee1024--7DaysToDie-ServerKit",
		Owner:   "IceCoffee1024",
		Repo:    "7DaysToDie-ServerKit",
		Title:   "7DaysToDie-ServerKit (天依服务端管理工具包)",
		Summary: "七日杀专用服务端综合管理 Mod，提供 WebAPI、事件监控与实用服务端辅助功能。",
		Author:  "IceCoffee1024",
		Tags:    []string{"server", "admin", "webapi"},
	},
	{
		ID:      "ZehMatt--7dtd-WalkerSim2",
		Owner:   "ZehMatt",
		Repo:    "7dtd-WalkerSim2",
		Title:   "WalkerSim 2 (全图动态尸群模拟)",
		Summary: "纯服务端运行的全局丧尸迁徙与声音吸引模拟系统，无需客户端额外安装。",
		Author:  "ZehMatt",
		Tags:    []string{"server", "zombies", "simulation"},
	},
	{
		ID:      "Kitsune-Den--KitsuneCommand",
		Owner:   "Kitsune-Den",
		Repo:    "KitsuneCommand",
		Title:   "KitsuneCommand (服务端指令与面板套件)",
		Summary: "开源七日杀服务端管理插件与事件自动化工具集，支持命令扩展与外部集成。",
		Author:  "Kitsune-Den",
		Tags:    []string{"server", "admin", "commands"},
	},
	{
		ID:      "OCB7D2D--OcbStopFuelWaste",
		Owner:   "OCB7D2D",
		Repo:    "OcbStopFuelWaste",
		Title:   "OCB Stop Fuel Waste (工作站自动熄火省燃料)",
		Summary: "熔炉、营火与工作台完成队列后自动停止燃烧，避免闲置白白消耗燃料。",
		Author:  "OCB7D2D",
		Tags:    []string{"qol", "crafting", "utility"},
	},
	{
		ID:      "OCB7D2D--OcbClaimAutoRepair",
		Owner:   "OCB7D2D",
		Repo:    "OcbClaimAutoRepair",
		Title:   "OCB Claim Auto Repair (领地石自动维护修复)",
		Summary: "允许领地石消耗仓库资源自动修复范围内受损方块与防御工事。",
		Author:  "OCB7D2D",
		Tags:    []string{"base", "repair", "qol"},
	},
	{
		ID:      "OCB7D2D--OcbElectricityOverhaul",
		Owner:   "OCB7D2D",
		Repo:    "OcbElectricityOverhaul",
		Title:   "OCB Electricity Overhaul (电力网格与电池增强)",
		Summary: "重构原版电力分配与储能逻辑，支持电网并联、光伏蓄电与更精细电路控制。",
		Author:  "OCB7D2D",
		Tags:    []string{"electricity", "overhaul", "base"},
	},
	{
		ID:      "OCB7D2D--OcbPinRecipes",
		Owner:   "OCB7D2D",
		Repo:    "OcbPinRecipes",
		Title:   "OCB Pin Recipes (制作配方常驻追踪)",
		Summary: "在 HUD 固定显示选定制作配方的所需材料清单与库存进度。",
		Author:  "OCB7D2D",
		Tags:    []string{"ui", "crafting", "qol"},
	},
	{
		ID:      "OCB7D2D--OcbInventoryMouseWheel",
		Owner:   "OCB7D2D",
		Repo:    "OcbInventoryMouseWheel",
		Title:   "OCB Inventory Mouse Wheel (鼠标滚轮快速存取)",
		Summary: "支持在背包与箱子界面通过鼠标滚轮逐件移动物品堆叠，提升整理效率。",
		Author:  "OCB7D2D",
		Tags:    []string{"inventory", "qol"},
	},
	{
		ID:      "OCB7D2D--OcbCrookedDeco",
		Owner:   "OCB7D2D",
		Repo:    "OcbCrookedDeco",
		Title:   "OCB Crooked Deco (家具摆件微调装饰)",
		Summary: "支持在建造安置家具与装饰方块时进行任意水平角度微调与自然偏移。",
		Author:  "OCB7D2D",
		Tags:    []string{"building", "decoration"},
	},
	{
		ID:      "OCB7D2D--OcbCustomTextures",
		Owner:   "OCB7D2D",
		Repo:    "OcbCustomTextures",
		Title:   "OCB Custom Textures (扩展方块自定义材质框架)",
		Summary: "为自定义模组解锁原版纹理槽位上限限制的通用依赖框架。",
		Author:  "OCB7D2D",
		Tags:    []string{"library", "textures"},
	},
}

func findCurated(id string) (curatedMod, bool) {
	trimmed := strings.TrimSpace(id)
	for _, c := range curatedCatalog {
		if strings.EqualFold(c.ID, trimmed) || strings.EqualFold(c.Repo, trimmed) {
			return c, true
		}
	}
	return curatedMod{}, false
}

func parseGitHubSlug(id string) (owner, repo string, ok bool) {
	trimmed := strings.TrimSpace(id)
	if c, found := findCurated(trimmed); found {
		return c.Owner, c.Repo, true
	}
	if o, r, splitOK := strings.Cut(trimmed, "--"); splitOK && isValidSeg(o) && isValidSeg(r) {
		return o, r, true
	}
	if o, r, splitOK := strings.Cut(trimmed, "/"); splitOK && isValidSeg(o) && isValidSeg(r) {
		return o, r, true
	}
	return "", "", false
}

func isValidSeg(s string) bool {
	return s != "" && !strings.ContainsAny(s, "/\\ \t\r\n?&#")
}

func toCanonicalPackageID(owner, repo string) string {
	canon := owner + "--" + repo
	if c, ok := findCurated(canon); ok {
		return c.ID
	}
	return canon
}

func curatedBaseItem(c curatedMod) driver.Item {
	srcURL := "https://github.com/" + c.Owner + "/" + c.Repo
	return driver.Item{
		ID:          c.ID,
		Title:       c.Title,
		Summary:     c.Summary,
		Author:      c.Author,
		SourceURL:   srcURL,
		UpstreamURL: srcURL,
		Tags:        append([]string(nil), c.Tags...),
	}
}

func matchesQuery(c curatedMod, q string) bool {
	if q == "" {
		return true
	}
	low := strings.ToLower(q)
	hay := strings.ToLower(c.ID + " " + c.Title + " " + c.Summary + " " + strings.Join(c.Tags, " "))
	return strings.Contains(hay, low)
}
