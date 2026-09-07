package modruntime

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestDependencyGraphResolvesDiamondAndCycleOnce(t *testing.T) {
	graph := map[string]domain.ModFile{
		"A": {ModName: "A", Dependencies: []string{"B", "C"}},
		"B": {ModName: "B", Dependencies: []string{"D"}},
		"C": {ModName: "C", Dependencies: []string{"D"}},
		"D": {ModName: "D", Dependencies: []string{"A"}},
	}
	var calls []string
	got, err := ResolveDependencies(context.Background(), []domain.ModFile{graph["A"]}, func(_ context.Context, name string) (domain.ModFile, bool, error) {
		calls = append(calls, name)
		return graph[name], true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calls, []string{"B", "C", "D"}) || len(got) != 3 {
		t.Fatalf("calls=%v added=%v", calls, got)
	}
}

func TestDependencyGraphStopsOnErrorOrCancellation(t *testing.T) {
	failure := errors.New("dependency unavailable")
	for _, cancelAfterFirst := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		calls := 0
		_, err := ResolveDependencies(ctx, []domain.ModFile{{ModName: "root", Dependencies: []string{"first", "second"}}}, func(context.Context, string) (domain.ModFile, bool, error) {
			calls++
			if cancelAfterFirst {
				cancel()
				return domain.ModFile{ModName: "first"}, true, nil
			}
			return domain.ModFile{}, false, failure
		})
		cancel()
		want := failure
		if cancelAfterFirst {
			want = context.Canceled
		}
		if !errors.Is(err, want) || calls != 1 {
			t.Fatalf("err=%v calls=%d", err, calls)
		}
	}
}

func TestDependencyGraphTraversesExistingRecordsWithoutReportingThemAdded(t *testing.T) {
	got, err := ResolveDependencies(context.Background(), []domain.ModFile{{ModName: "root", Dependencies: []string{"existing"}}}, func(_ context.Context, name string) (domain.ModFile, bool, error) {
		if name == "existing" {
			return domain.ModFile{ModName: name, Dependencies: []string{"new"}}, false, nil
		}
		return domain.ModFile{ModName: name}, true, nil
	})
	if err != nil || len(got) != 1 || got[0].ModName != "new" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}
