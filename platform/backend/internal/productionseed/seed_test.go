package productionseed

import (
	"testing"
	"time"
)

func TestProductionCatalogIsEffectiveBeforeRelease(t *testing.T) {
	release := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)
	if !resourceCatalog().EffectiveAt.Before(release) || !priceBook().EffectiveAt.Before(release) {
		t.Fatal("production pricing must be effective before release")
	}
}
