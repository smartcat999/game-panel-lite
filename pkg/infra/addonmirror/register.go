package addonmirror

import (
	// Register built-in upstream addon/server drivers and HTTP sub-drivers cleanly by subpackage.
	_ "github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror/providers/fabricmc"
	_ "github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror/providers/http"
	_ "github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror/providers/modrinth"
	_ "github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror/providers/neoforge"
	_ "github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror/providers/papermc"
	_ "github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror/providers/purpur"
	_ "github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror/providers/steam"
	_ "github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror/providers/terraria"
	_ "github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror/providers/tmodloader"
	_ "github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror/providers/vanilla"
)
