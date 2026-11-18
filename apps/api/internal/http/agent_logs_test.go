package http

import (
	"reflect"
	"testing"
)

func TestLogSnapshotDeltaReturnsOnlyNewTail(t *testing.T) {
	current := []string{"one", "two", "three"}
	snapshot := []string{"two", "three", "four", "five"}
	want := []string{"four", "five"}
	if got := logSnapshotDelta(current, snapshot); !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %+v, got %+v", want, got)
	}
}

func TestLogSnapshotDeltaSuppressesIdenticalSnapshot(t *testing.T) {
	snapshot := []string{"one", "two"}
	if got := logSnapshotDelta(snapshot, snapshot); len(got) != 0 {
		t.Fatalf("expected no new lines, got %+v", got)
	}
}
