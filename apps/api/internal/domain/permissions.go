package domain

type Permission string

const (
	PermissionServerView      Permission = "server.view"
	PermissionServerCreate    Permission = "server.create"
	PermissionServerControl   Permission = "server.control"
	PermissionServerConfigure Permission = "server.configure"
	PermissionServerDelete    Permission = "server.delete"
	PermissionBackupManage    Permission = "backup.manage"
	PermissionWorldManage     Permission = "world.manage"
	PermissionModManage       Permission = "mod.manage"
	PermissionPlayerManage    Permission = "player.manage"
	PermissionShareManage     Permission = "share.manage"
	PermissionNodeManage      Permission = "node.manage"
	PermissionTeamManage      Permission = "team.manage"
	PermissionSettingsManage  Permission = "settings.manage"
	PermissionSystemManage    Permission = "system.manage"
)

var viewerPermissions = []Permission{
	PermissionServerView,
}

var memberPermissions = []Permission{
	PermissionServerView,
	PermissionServerCreate,
	PermissionServerControl,
	PermissionServerConfigure,
	PermissionBackupManage,
	PermissionWorldManage,
	PermissionModManage,
	PermissionPlayerManage,
	PermissionShareManage,
}

var adminPermissions = []Permission{
	PermissionServerView,
	PermissionServerCreate,
	PermissionServerControl,
	PermissionServerConfigure,
	PermissionServerDelete,
	PermissionBackupManage,
	PermissionWorldManage,
	PermissionModManage,
	PermissionPlayerManage,
	PermissionShareManage,
	PermissionNodeManage,
	PermissionTeamManage,
	PermissionSettingsManage,
	PermissionSystemManage,
}

func NormalizeAccountRole(role Role) Role {
	if role == "" {
		return RoleAdmin
	}
	return role
}

func PermissionsForRole(role Role) []Permission {
	var permissions []Permission
	switch NormalizeAccountRole(role) {
	case RoleAdmin, RoleOwner:
		permissions = adminPermissions
	case RoleMember:
		permissions = memberPermissions
	default:
		permissions = viewerPermissions
	}
	return append([]Permission(nil), permissions...)
}

func RoleHasPermission(role Role, permission Permission) bool {
	for _, allowed := range PermissionsForRole(role) {
		if allowed == permission {
			return true
		}
	}
	return false
}
