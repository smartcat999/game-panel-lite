package v1

import (
	"time"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
)

// UserRecord is the strongly-typed GORM model for the users declarative table.
type UserRecord struct {
	Name            string            `gorm:"column:name;primaryKey"`
	UID             string            `gorm:"column:uid;unique;not null"`
	Email           string            `gorm:"column:email;not null"`
	DisplayName     string            `gorm:"column:display_name;not null;default:''"`
	PasswordHash    string            `gorm:"column:password_hash;not null;default:''"`
	State           string            `gorm:"column:state;not null;default:'Active'"`
	ResourceVersion int64             `gorm:"column:resource_version;not null;default:1"`
	Generation      int64             `gorm:"column:generation;not null;default:1"`
	CreatedAt       time.Time         `gorm:"column:created_at;not null"`
	DeletedAt       *time.Time        `gorm:"column:deleted_at"`
	Finalizers      []string          `gorm:"column:finalizers;serializer:json;type:jsonb;not null"`
	Labels          map[string]string `gorm:"column:labels;serializer:json;type:jsonb;not null"`
	Annotations     map[string]string `gorm:"column:annotations;serializer:json;type:jsonb;not null"`
}

func (UserRecord) TableName() string { return "users" }

// UserStateRecord is the strongly-typed GORM model for dynamic user runtime state (user_states).
type UserStateRecord struct {
	UserUID            string             `gorm:"column:user_uid;primaryKey"`
	Version            int64              `gorm:"column:version;not null;default:1"`
	ObservedGeneration int64              `gorm:"column:observed_generation;not null;default:0"`
	Phase              string             `gorm:"column:phase;not null;default:'Active'"`
	DefaultTenant      string             `gorm:"column:default_tenant;not null;default:''"`
	LastLoginTime      *time.Time         `gorm:"column:last_login_time"`
	Conditions         []metav1.Condition `gorm:"column:conditions;serializer:json;type:jsonb;not null"`
	UpdatedAt          time.Time          `gorm:"column:updated_at;not null"`
}

func (UserStateRecord) TableName() string { return "user_states" }

// GlobalRoleRecord is the strongly-typed GORM model for the global_roles table.
type GlobalRoleRecord struct {
	Name            string            `gorm:"column:name;primaryKey"`
	UID             string            `gorm:"column:uid;unique;not null"`
	Description     string            `gorm:"column:description;not null;default:''"`
	Rules           []PolicyRule      `gorm:"column:rules;serializer:json;type:jsonb;not null"`
	ResourceVersion int64             `gorm:"column:resource_version;not null;default:1"`
	Generation      int64             `gorm:"column:generation;not null;default:1"`
	CreatedAt       time.Time         `gorm:"column:created_at;not null"`
	DeletedAt       *time.Time        `gorm:"column:deleted_at"`
	Finalizers      []string          `gorm:"column:finalizers;serializer:json;type:jsonb;not null"`
	Labels          map[string]string `gorm:"column:labels;serializer:json;type:jsonb;not null"`
	Annotations     map[string]string `gorm:"column:annotations;serializer:json;type:jsonb;not null"`
}

func (GlobalRoleRecord) TableName() string { return "global_roles" }

// GlobalRoleBindingRecord is the strongly-typed GORM model for the global_role_bindings table.
type GlobalRoleBindingRecord struct {
	Name            string            `gorm:"column:name;primaryKey"`
	UID             string            `gorm:"column:uid;unique;not null"`
	RoleName        string            `gorm:"column:role_name;not null"`
	Subjects        []Subject         `gorm:"column:subjects;serializer:json;type:jsonb;not null"`
	ResourceVersion int64             `gorm:"column:resource_version;not null;default:1"`
	Generation      int64             `gorm:"column:generation;not null;default:1"`
	CreatedAt       time.Time         `gorm:"column:created_at;not null"`
	DeletedAt       *time.Time        `gorm:"column:deleted_at"`
	Finalizers      []string          `gorm:"column:finalizers;serializer:json;type:jsonb;not null"`
	Labels          map[string]string `gorm:"column:labels;serializer:json;type:jsonb;not null"`
	Annotations     map[string]string `gorm:"column:annotations;serializer:json;type:jsonb;not null"`
}

func (GlobalRoleBindingRecord) TableName() string { return "global_role_bindings" }

// TenantRoleRecord is the strongly-typed GORM model for the tenant_roles table.
type TenantRoleRecord struct {
	Tenant          string            `gorm:"column:tenant;primaryKey"`
	Name            string            `gorm:"column:name;primaryKey"`
	UID             string            `gorm:"column:uid;unique;not null"`
	Description     string            `gorm:"column:description;not null;default:''"`
	Rules           []PolicyRule      `gorm:"column:rules;serializer:json;type:jsonb;not null"`
	ResourceVersion int64             `gorm:"column:resource_version;not null;default:1"`
	Generation      int64             `gorm:"column:generation;not null;default:1"`
	CreatedAt       time.Time         `gorm:"column:created_at;not null"`
	DeletedAt       *time.Time        `gorm:"column:deleted_at"`
	Finalizers      []string          `gorm:"column:finalizers;serializer:json;type:jsonb;not null"`
	Labels          map[string]string `gorm:"column:labels;serializer:json;type:jsonb;not null"`
	Annotations     map[string]string `gorm:"column:annotations;serializer:json;type:jsonb;not null"`
}

func (TenantRoleRecord) TableName() string { return "tenant_roles" }

// TenantRoleBindingRecord is the strongly-typed GORM model for the tenant_role_bindings table.
type TenantRoleBindingRecord struct {
	Tenant          string            `gorm:"column:tenant;primaryKey"`
	Name            string            `gorm:"column:name;primaryKey"`
	UID             string            `gorm:"column:uid;unique;not null"`
	RoleName        string            `gorm:"column:role_name;not null"`
	Subjects        []Subject         `gorm:"column:subjects;serializer:json;type:jsonb;not null"`
	ResourceVersion int64             `gorm:"column:resource_version;not null;default:1"`
	Generation      int64             `gorm:"column:generation;not null;default:1"`
	CreatedAt       time.Time         `gorm:"column:created_at;not null"`
	DeletedAt       *time.Time        `gorm:"column:deleted_at"`
	Finalizers      []string          `gorm:"column:finalizers;serializer:json;type:jsonb;not null"`
	Labels          map[string]string `gorm:"column:labels;serializer:json;type:jsonb;not null"`
	Annotations     map[string]string `gorm:"column:annotations;serializer:json;type:jsonb;not null"`
}

func (TenantRoleBindingRecord) TableName() string { return "tenant_role_bindings" }
