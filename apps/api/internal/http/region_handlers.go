package http

import (
	"net/http"
	"strings"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

// Default predefined global cloud regions
var standardRegions = []domain.RegionInfo{
	{ID: "hk", Name: "香港", NameEn: "Hong Kong", Flag: "🇭🇰", Available: true},
	{ID: "tokyo", Name: "东京", NameEn: "Tokyo", Flag: "🇯🇵", Available: true},
	{ID: "silicon-valley", Name: "硅谷", NameEn: "Silicon Valley", Flag: "🇺🇸", Available: true},
	{ID: "frankfurt", Name: "法兰克福", NameEn: "Frankfurt", Flag: "🇩🇪", Available: true},
	{ID: "shanghai", Name: "上海", NameEn: "Shanghai", Flag: "🇨🇳", Available: true},
}

// listRegions returns the cloud regions catalog with live node counts: GET /api/regions
func (h *Handler) listRegions(w http.ResponseWriter, r *http.Request) {
	nodes, err := h.store.ListComputeNodes(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list nodes: "+err.Error())
		return
	}

	// Count nodes per region
	nodeCountMap := make(map[string]int)
	for _, n := range nodes {
		reg := strings.ToLower(strings.TrimSpace(n.Region))
		if reg == "" {
			reg = "hk" // default region if unset
		}
		nodeCountMap[reg]++
	}

	regions := make([]domain.RegionInfo, len(standardRegions))
	for i, reg := range standardRegions {
		regCopy := reg
		regCopy.NodeCount = nodeCountMap[reg.ID]
		// If there are compute nodes registered in this region or it's default hk, it's available
		if regCopy.NodeCount > 0 || reg.ID == "hk" {
			regCopy.Available = true
		}
		regions[i] = regCopy
	}

	writeJSON(w, http.StatusOK, regions)
}
