package deliverycontrol

import (
	"encoding/json"
	"testing"
)

func TestCloneModLockNormalizesNilToEmptyArray(t *testing.T) {
	encoded, err := json.Marshal(cloneModLock(nil))
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != "[]" {
		t.Fatalf("expected an empty JSON array, got %s", encoded)
	}
}
