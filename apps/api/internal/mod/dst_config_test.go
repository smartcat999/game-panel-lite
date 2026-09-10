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
