package safety

import "testing"

func TestSafeFileNameRejectsTraversal(t *testing.T) {
	if _, err := SafeFileName("../world.wld", ".wld"); err == nil {
		t.Fatal("expected traversal file name to fail")
	}
	if _, err := SafeFileName("world.exe", ".wld"); err == nil {
		t.Fatal("expected invalid extension to fail")
	}
}

func TestSafeJoinRejectsEscapes(t *testing.T) {
	if _, err := SafeJoin("/tmp/gamepanel", "..", "outside.wld"); err == nil {
		t.Fatal("expected escaped path to fail")
	}
}
