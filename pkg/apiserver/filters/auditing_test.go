package filters

import "testing"

func TestRedactSensitiveAuditURI(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{
			in:   "/apis/core.gamepanel.io/v1/tenants/studio-alpha/invites/eyJ0Ijoic3R1ZGlv.sig123",
			want: "/apis/core.gamepanel.io/v1/tenants/studio-alpha/invites/[REDACTED]",
		},
		{
			in:   "/apis/core.gamepanel.io/v1/tenants/studio-alpha/invites",
			want: "/apis/core.gamepanel.io/v1/tenants/studio-alpha/invites",
		},
		{
			in:   "/apis/core.gamepanel.io/v1/tenants/studio-alpha/instances/mc-01",
			want: "/apis/core.gamepanel.io/v1/tenants/studio-alpha/instances/mc-01",
		},
	}
	for _, tc := range cases {
		if got := redactSensitiveAuditURI(tc.in); got != tc.want {
			t.Fatalf("redactSensitiveAuditURI(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
