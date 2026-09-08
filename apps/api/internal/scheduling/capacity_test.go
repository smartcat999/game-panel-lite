package scheduling

import (
	"errors"
	"math"
	"testing"
)

func TestCheckCapacity(t *testing.T) {
	for _, tc := range []struct {
		name           string
		total, request Resources
		reserved       []Resources
		want           Resources
		reject         bool
	}{
		{name: "exact fit", total: Resources{2, 1024}, request: Resources{1, 512}, reserved: []Resources{{1, 512}}},
		{name: "remaining", total: Resources{4, 4096}, request: Resources{1, 512}, reserved: []Resources{{1, 512}}, want: Resources{2, 3072}},
		{name: "CPU exhausted", total: Resources{1, 8192}, request: Resources{1, 512}, reserved: []Resources{{1, 512}}, reject: true},
		{name: "memory exhausted", total: Resources{8, 512}, request: Resources{1, 512}, reserved: []Resources{{1, 512}}, reject: true},
		{name: "unknown node", total: Resources{0, 8192}, request: Resources{1, 512}, reject: true},
		{name: "unbounded request", total: Resources{8, 8192}, request: Resources{0, 512}, reject: true},
		{name: "unbounded reservation", total: Resources{8, 8192}, request: Resources{1, 512}, reserved: []Resources{{1, 0}}, reject: true},
		{name: "NaN", total: Resources{8, 8192}, request: Resources{math.NaN(), 512}, reject: true},
		{name: "infinite node", total: Resources{math.Inf(1), 8192}, request: Resources{1, 512}, reject: true},
		{name: "infinite reservation", total: Resources{8, 8192}, request: Resources{1, 512}, reserved: []Resources{{math.Inf(1), 512}}, reject: true},
		{name: "memory sum overflow", total: Resources{8, math.MaxInt64}, request: Resources{1, 512}, reserved: []Resources{{1, math.MaxInt64}, {1, math.MaxInt64}}, reject: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CheckCapacity(tc.total, tc.request, tc.reserved)
			if tc.reject {
				if !errors.Is(err, ErrCapacityUnavailable) {
					t.Fatalf("accepted invalid allocation: %v", err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("remaining=%+v want=%+v err=%v", got, tc.want, err)
			}
		})
	}
}
