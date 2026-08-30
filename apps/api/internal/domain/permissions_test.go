package domain

import "testing"

func TestRolePermissions(t *testing.T) {
	tests := []struct {
		name       string
		role       Role
		permission Permission
		want       bool
	}{
		{name: "viewer can read", role: RoleViewer, permission: PermissionServerView, want: true},
		{name: "viewer cannot control", role: RoleViewer, permission: PermissionServerControl, want: false},
		{name: "member can control", role: RoleMember, permission: PermissionServerControl, want: true},
		{name: "member cannot delete", role: RoleMember, permission: PermissionServerDelete, want: false},
		{name: "member cannot manage settings", role: RoleMember, permission: PermissionSettingsManage, want: false},
		{name: "admin can delete", role: RoleAdmin, permission: PermissionServerDelete, want: true},
		{name: "legacy empty role remains admin", role: "", permission: PermissionSystemManage, want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := RoleHasPermission(test.role, test.permission); got != test.want {
				t.Fatalf("RoleHasPermission(%q, %q) = %v, want %v", test.role, test.permission, got, test.want)
			}
		})
	}
}
