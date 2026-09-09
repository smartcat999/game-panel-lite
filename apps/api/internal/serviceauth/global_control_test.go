package serviceauth

import "testing"

func TestGlobalControlIdentityConfiguration(t *testing.T) {
	if _, err := NewGlobalControls([]string{"spiffe://gamepanel/global-control"}); err != nil {
		t.Fatal(err)
	}
	for _, identities := range [][]string{nil, {""}, {"spiffe://gamepanel/global?x=1"}, {"spiffe://gamepanel/global", "spiffe://gamepanel/global"}} {
		if _, err := NewGlobalControls(identities); err == nil {
			t.Fatalf("accepted invalid identities: %#v", identities)
		}
	}
}
