package world

import (
	"strings"
	"testing"
)

func TestImportValidatesWorldExtension(t *testing.T) {
	service := NewService(t.TempDir())
	if _, _, err := service.Import("srv", "bad.txt", strings.NewReader("x")); err == nil {
		t.Fatal("expected invalid extension to fail")
	}
	if _, size, err := service.Import("srv", "good.wld", strings.NewReader("world")); err != nil || size != 5 {
		t.Fatalf("expected world import, size=%d err=%v", size, err)
	}
}
