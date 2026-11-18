package mod

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseDSTConfigOptionsResolvesLocaleTables(t *testing.T) {
	source := `
local descs = { fast = "Faster generation" }
local options = {
  toggle = {{description = "Disabled", data = false}, {description = "Enabled", data = true}},
  none = {{description = "", data = false}},
}
local configs = { section = "Worldgen", fast = "Fast generation" }
if locale == "zh" or locale == "zht" then
  descs = { fast = "加快地图生成" }
  options = {
    toggle = {{description = "关闭", data = false}, {description = "开启", data = true}},
    none = {{description = "", data = false}},
  }
  configs = { section = "地图生成", fast = "快速生成" }
end
configuration_options = {
  {name = "section", label = configs.section, options = options.none, default = false},
  {name = "biome_fastgen", label = configs.fast, hover = descs.fast, options = options.toggle, default = false},
}
`
	options, err := ParseDSTConfigOptions(source, "zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	if len(options) != 2 {
		t.Fatalf("expected two options, got %+v", options)
	}
	if options[1].Label != "快速生成" || options[1].Description != "加快地图生成" {
		t.Fatalf("expected localized option, got %+v", options[1])
	}
	if len(options[1].Choices) != 2 || options[1].Choices[1].Label != "开启" || options[1].Choices[1].Value != true {
		t.Fatalf("expected localized boolean choices, got %+v", options[1].Choices)
	}
}

func TestParseDSTConfigOptionsFallsBackToDefaultLanguage(t *testing.T) {
	source := `
local options = { values = {{description = "One", data = 1}, {description = "Two", data = 2}} }
local configs = { count = "Count" }
if locale == "ko" then
  configs = { count = "수량" }
end
configuration_options = {{name = "count", label = configs.count, options = options.values, default = 1}}
`
	options, err := ParseDSTConfigOptions(source, "ja")
	if err != nil {
		t.Fatal(err)
	}
	if len(options) != 1 || options[0].Label != "Count" {
		t.Fatalf("expected default-language option, got %+v", options)
	}
}

func TestReadDSTConfigOptionsPrefersLocalizedModInfo(t *testing.T) {
	dir := t.TempDir()
	base := `local options = { toggle = {{description = "Off", data = false}, {description = "On", data = true}} }
configuration_options = {{name = "enabled", label = "Enabled", options = options.toggle, default = true}}`
	localized := `local options = { toggle = {{description = "关闭", data = false}, {description = "开启", data = true}} }
configuration_options = {{name = "enabled", label = "是否启用", options = options.toggle, default = true}}`
	path := filepath.Join(dir, "modinfo.lua")
	if err := os.WriteFile(path, []byte(base), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "modinfo_chs.lua"), []byte(localized), 0o600); err != nil {
		t.Fatal(err)
	}
	options, err := ReadDSTConfigOptions(path, "zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	if len(options) != 1 || options[0].Label != "是否启用" || options[0].Choices[1].Label != "开启" {
		t.Fatalf("expected localized modinfo to win, got %+v", options)
	}
}

func TestParseDSTConfigOptionsSupportsCommonOptionHelpers(t *testing.T) {
	source := `
local L = (locale == "zh") and true or false
local vars = { OPEN = L and "开启" or "Open", CLOSE = L and "关闭" or "Close" }
local function option(description, data, hover) return {description=description, data=data, hover=hover} end
configuration_options = {
  fns.largeLabel(L and "玩法" or "Gameplay"),
  {name = "feature", label = L and "功能" or "Feature", options = {
    option(vars.OPEN, true), option(vars.CLOSE, false),
  }, default = true},
}`
	options, err := ParseDSTConfigOptions(source, "zh")
	if err != nil {
		t.Fatal(err)
	}
	if len(options) != 2 || !options[0].Section || options[0].Label != "玩法" || options[1].Label != "功能" || options[1].Choices[0].Label != "开启" {
		t.Fatalf("expected helper-generated localized choices, got %+v", options)
	}
}

func TestParseDSTConfigOptionsSupportsLocaleConditionalTables(t *testing.T) {
	source := `
local isCh = locale == "zh" or locale == "zhr"
configuration_options = isCh and
{
  {name = "language_switch", label = "选择语言", options = {
    {description = "中文", data = "ch"}, {description = "English", data = "eng"},
  }, default = "ch"},
} or
{
  {name = "language_switch", label = "Language", options = {
    {description = "中文", data = "ch"}, {description = "English", data = "eng"},
  }, default = "eng"},
}`

	zhOptions, err := ParseDSTConfigOptions(source, "zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	if len(zhOptions) != 1 || zhOptions[0].Label != "选择语言" || zhOptions[0].Default != "ch" {
		t.Fatalf("expected Chinese conditional table, got %+v", zhOptions)
	}

	enOptions, err := ParseDSTConfigOptions(source, "en")
	if err != nil {
		t.Fatal(err)
	}
	if len(enOptions) != 1 || enOptions[0].Label != "Language" || enOptions[0].Default != "eng" {
		t.Fatalf("expected English conditional table, got %+v", enOptions)
	}
}

func TestParseDSTConfigOptionsSupportsJingXiHelpers(t *testing.T) {
	source := `
local is_chinese = locale == "zh" or locale == "zht" or locale == "zhr"
local key_info = is_chinese and {{description = "禁用", data = false}} or {{description = "Disable", data = false}}
local key_info2 = {{description = "J", data = "KEY_J"}, {description = "K", data = "KEY_K"}}
for i = 1, #key_info2 do
  key_info[i + 1] = key_info2[i]
end
local str = {[1] = "减缓食物腐烂的速率"}
local switch_ch = {{description = "开启", data = true}, {description = "关闭", data = false}}
local switch_en = {{description = "Enable", data = true}, {description = "Disable", data = false}}
local function AddTitle(title) return {label = title, name = "", hover = "", options = {{description = "", data = 0}}, default = 0} end
local function AddConfig(label, name, options, default, hover) return {label = label, name = name, options = options, default = default, hover = hover or ""} end
local function AddOptions(data, ispercent, opposite_desc) return {} end
if is_chinese then
  configuration_options = {
    AddTitle("-----------------------------------------"),
    AddTitle("小提示:模组默认配置仅作为维基配置"),
    AddTitle("手工编织野餐篮"),
    AddConfig("保鲜率", "jx_basket_preserver", AddOptions({1,.95}, true, true), .95, str[1]),
    AddConfig("传球按键", "jx_football_key1", key_info, "KEY_J", nil),
  }
else
  configuration_options = {
    AddTitle("-----------------------------------------"),
    AddTitle("Configure these options to your preference"),
    AddTitle("Hand Woven Basket"),
    AddConfig("Preservation rate", "jx_basket_preserver", AddOptions({1,.95}, true, true), .95, nil),
  }
end`

	zhOptions, err := ParseDSTConfigOptions(source, "zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	if len(zhOptions) != 3 || !zhOptions[0].Section || zhOptions[0].Label != "手工编织野餐篮" {
		t.Fatalf("expected Chinese helper-generated options, got %+v", zhOptions)
	}
	if zhOptions[1].Description != "减缓食物腐烂的速率" || len(zhOptions[1].Choices) != 2 || zhOptions[1].Choices[1].Label != "5%" {
		t.Fatalf("expected indexed description and generated percentage choices, got %+v", zhOptions[1])
	}
	if len(zhOptions[2].Choices) != 3 || zhOptions[2].Choices[1].Value != "KEY_J" {
		t.Fatalf("expected appended keyboard choices, got %+v", zhOptions[2])
	}

	enOptions, err := ParseDSTConfigOptions(source, "en")
	if err != nil {
		t.Fatal(err)
	}
	if len(enOptions) != 2 || enOptions[0].Label != "Hand Woven Basket" || enOptions[1].Label != "Preservation rate" {
		t.Fatalf("expected English configuration branch, got %+v", enOptions)
	}
}

func TestParseDSTConfigOptionsSupportsConcatenatedDescriptions(t *testing.T) {
	source := `configuration_options = {{
  name = "fires",
  label = "Show Fires",
  hover = "Show fires globally." .. "\nThey will smoke.",
  options = {{description = "Show", data = true}, {description = "Hide", data = false}},
  default = true,
}}`
	options, err := ParseDSTConfigOptions(source, "en")
	if err != nil {
		t.Fatal(err)
	}
	if len(options) != 1 || options[0].Description != "Show fires globally.\nThey will smoke." {
		t.Fatalf("expected concatenated description, got %+v", options)
	}
}
