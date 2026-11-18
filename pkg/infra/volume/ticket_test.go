package volume

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestVerifyFileTicketRejectsTrailingBitTampering(t *testing.T) {
	const secret = "0123456789abcdef0123456789abcdef"
	now := time.Unix(1700000000, 0).UTC()
	var canonical string
	for i := range 1024 {
		tok, err := SignFileTicket(secret, FileTicketClaims{
			InstanceUID: fmt.Sprintf("uid-%d", i),
			Tenant:      "default",
			Instance:    "mc-01",
			NodeName:    "node-01",
			Action:      "read",
			Path:        "ticket-probe.txt",
			ExpiresAt:   now.Add(time.Hour).Unix(),
		})
		if err != nil {
			t.Fatalf("SignFileTicket: %v", err)
		}
		if strings.HasSuffix(tok, "A") {
			canonical = tok
			break
		}
	}
	if canonical == "" {
		t.Fatal("failed to find signed file ticket ending with 'A' within 1024 iterations")
	}
	if _, err := VerifyFileTicket(secret, canonical, now); err != nil {
		t.Fatalf("VerifyFileTicket rejected canonical ticket: %v", err)
	}
	tampered := []string{
		canonical[:len(canonical)-1] + "B",
		canonical[:len(canonical)-1] + "C",
		canonical[:len(canonical)-1] + "D",
		canonical[:len(canonical)-1] + "Q",
		canonical + "=",
	}
	for _, tok := range tampered {
		if _, err := VerifyFileTicket(secret, tok, now); err == nil {
			t.Fatalf("VerifyFileTicket accepted non-canonical tampered ticket %q", tok)
		}
	}
}
