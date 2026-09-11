package regiondirectory

import (
	"context"
	"errors"
	"sort"

	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
)

var ErrRegionUnavailable = errors.New("region is unavailable")

type Region struct {
	ID        contract.RegionID `json:"id"`
	Code      string            `json:"code"`
	Name      string            `json:"name"`
	Names     map[string]string `json:"names,omitempty"`
	Available bool              `json:"available"`
}

type Module struct {
	regions map[contract.RegionID]Region
}

func New(regions []Region) *Module {
	items := make(map[contract.RegionID]Region, len(regions))
	for _, region := range regions {
		items[region.ID] = region
	}
	return &Module{regions: items}
}

func (m *Module) List(_ context.Context) []Region {
	regions := make([]Region, 0, len(m.regions))
	for _, region := range m.regions {
		regions = append(regions, region)
	}
	sort.Slice(regions, func(i, j int) bool { return regions[i].Code < regions[j].Code })
	return regions
}

func (m *Module) RequireAvailable(_ context.Context, regionID contract.RegionID) (Region, error) {
	region, ok := m.regions[regionID]
	if !ok || !region.Available {
		return Region{}, ErrRegionUnavailable
	}
	return region, nil
}
