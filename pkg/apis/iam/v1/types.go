package v1

import (
	"errors"
	"fmt"
	"strings"
	"time"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
)

const (
	GroupName    = "iam.gamepanel.io"
	GroupVersion = "iam.gamepanel.io/v1"

	SubjectKindUser   = "User"
	SubjectKindNode   = "Node"
	SubjectKindGroup  = "Group"
	SubjectKindSystem = "System"

	PrincipalNodeAgent = "system:node-agent"
	PrincipalSystem    = "system"

	VerbAll    = "*"
	VerbGet    = "get"
	VerbList   = "list"
	VerbWatch  = "watch"
	VerbCreate = "create"
	VerbUpdate = "update"
	VerbPatch  = "patch"
	VerbDelete = "delete"

	AnnotationSessionUserAgent      = "iam.gamepanel.io/user-agent"
	AnnotationSessionClientIP       = "iam.gamepanel.io/client-ip"
	AnnotationSessionCurrent        = "iam.gamepanel.io/current"
	AnnotationOwnerUID              = "iam.gamepanel.io/owner-uid"
	AnnotationPersonalTenantUID     = "iam.gamepanel.io/personal-tenant-uid"
	AnnotationDisablePersonalTenant = "iam.gamepanel.io/disable-personal-tenant"
	AnnotationSessionsRevoked       = "iam.gamepanel.io/sessions-revoked-uid"

	CacheKeyPrefixNodeToken = "gp:iam:node-token:"

	FinalizerUserCleanup = "iam.gamepanel.io/user-cleanup"
)

// AllowedVerbs lists the standard Kubernetes declarative verbs.
var AllowedVerbs = map[string]bool{
	VerbAll:    true,
	VerbGet:    true,
	VerbList:   true,
	VerbWatch:  true,
	VerbCreate: true,
	VerbUpdate: true,
	VerbPatch:  true,
	VerbDelete: true,
}

// UserState represents the desired state of a User account.
type UserState string

const (
	UserStateActive   UserState = "Active"
	UserStateDisabled UserState = "Disabled"
)

// User represents a platform identity.
type User struct {
	metav1.TypeMeta `json:",inline"`
	Metadata        metav1.ObjectMeta `json:"metadata"`
	Spec            UserSpec          `json:"spec"`
	Status          UserStatus        `json:"status,omitempty"`
}

type UserSpec struct {
	Email           string    `json:"email"`
	DisplayName     string    `json:"displayName,omitempty"`
	CurrentPassword string    `json:"currentPassword,omitempty"`
	Password        string    `json:"password,omitempty"`
	PasswordHash    string    `json:"passwordHash,omitempty"`
	State           UserState `json:"state,omitempty"`
}

type UserStatus struct {
	ObservedGeneration int64              `json:"observedGeneration"`
	Phase              UserState          `json:"phase"`
	DefaultTenant      string             `json:"defaultTenant,omitempty"`
	LastLoginTime      *time.Time         `json:"lastLoginTime,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
}

func (u *User) GetTypeMeta() *metav1.TypeMeta     { return &u.TypeMeta }
func (u *User) GetObjectMeta() *metav1.ObjectMeta { return &u.Metadata }

func (u *User) DefaultMeta() {
	u.TypeMeta = metav1.TypeMeta{APIVersion: GroupVersion, Kind: "User"}
	if u.Spec.State == "" {
		u.Spec.State = UserStateActive
	}
	if u.Status.Phase == "" {
		u.Status.Phase = u.Spec.State
	}
}

func (u *User) Validate() error {
	if err := metav1.ValidateSingleSegmentName("user metadata.name", u.Metadata.Name); err != nil {
		return err
	}
	if strings.TrimSpace(u.Spec.Email) == "" || !strings.Contains(u.Spec.Email, "@") {
		return errors.New("user spec.email must be a valid email address")
	}
	switch u.Spec.State {
	case "", UserStateActive, UserStateDisabled:
	default:
		return fmt.Errorf("invalid user spec.state %q", u.Spec.State)
	}
	return nil
}

// PolicyRule holds information that describes an additive permission rule,
// supporting '*' wildcards across apiGroups, resources (including subresources like "instances/*" or "*/status"),
// verbs, resourceNames, and optional label selectors.
type PolicyRule struct {
	APIGroups     []string              `json:"apiGroups"`
	Resources     []string              `json:"resources"`
	Verbs         []string              `json:"verbs"`
	ResourceNames []string              `json:"resourceNames,omitempty"`
	Selector      *metav1.LabelSelector `json:"selector,omitempty"`
}

func (r PolicyRule) Validate() error {
	if len(r.APIGroups) == 0 {
		return errors.New("rule.apiGroups must not be empty")
	}
	if len(r.Resources) == 0 {
		return errors.New("rule.resources must not be empty")
	}
	if len(r.Verbs) == 0 {
		return errors.New("rule.verbs must not be empty")
	}
	for _, verb := range r.Verbs {
		if !AllowedVerbs[verb] {
			return fmt.Errorf("unsupported verb %q; allowed verbs are get, list, watch, create, update, patch, delete, *", verb)
		}
	}
	for _, res := range r.Resources {
		if strings.Contains(res, ":") {
			return fmt.Errorf("resource %q must not contain ':'; use '/' for subresources", res)
		}
	}
	return nil
}

// Matches evaluates whether this additive rule grants access to the requested action.
func (r PolicyRule) Matches(apiGroup, resource, verb, resourceName string, labels map[string]string) bool {
	if !matchSlice(r.APIGroups, apiGroup) {
		return false
	}
	if !matchSlice(r.Resources, resource) {
		return false
	}
	if !matchSlice(r.Verbs, verb) {
		return false
	}
	if len(r.ResourceNames) > 0 {
		if resourceName == "" {
			if !hasWildcardName(r.ResourceNames) {
				return false
			}
		} else if !matchSlice(r.ResourceNames, resourceName) {
			return false
		}
	}
	if r.Selector != nil && (len(r.Selector.MatchLabels) > 0 || len(r.Selector.MatchExpressions) > 0) {
		if resourceName == "" || resourceName == "*" || labels == nil {
			return false
		}
		if !r.Selector.Matches(labels) {
			return false
		}
	}
	return true
}

func hasWildcardName(names []string) bool {
	for _, n := range names {
		if n == "*" {
			return true
		}
	}
	return false
}

func matchSlice(patterns []string, target string) bool {
	for _, p := range patterns {
		if metav1.WildcardMatch(p, target) {
			return true
		}
	}
	return false
}

// RoleRef references a GlobalRole or TenantRole.
type RoleRef struct {
	Name string `json:"name"`
}

// Subject references an identity bound to a role.
type Subject struct {
	Kind string `json:"kind"` // "User" (default), "Node", "Group", or "System"
	Name string `json:"name"`
}

// GlobalRole defines cluster-wide permissions.
type GlobalRole struct {
	metav1.TypeMeta `json:",inline"`
	Metadata        metav1.ObjectMeta `json:"metadata"`
	Description     string            `json:"description,omitempty"`
	Rules           []PolicyRule      `json:"rules"`
}

func (r *GlobalRole) GetTypeMeta() *metav1.TypeMeta     { return &r.TypeMeta }
func (r *GlobalRole) GetObjectMeta() *metav1.ObjectMeta { return &r.Metadata }

func (r *GlobalRole) DefaultMeta() {
	r.TypeMeta = metav1.TypeMeta{APIVersion: GroupVersion, Kind: "GlobalRole"}
}

func (r *GlobalRole) Validate() error {
	if strings.TrimSpace(r.Metadata.Name) == "" {
		return errors.New("globalrole metadata.name is required")
	}
	for i, rule := range r.Rules {
		if err := rule.Validate(); err != nil {
			return fmt.Errorf("rules[%d]: %w", i, err)
		}
	}
	return nil
}

// GlobalRoleBinding binds subjects to a GlobalRole across the platform.
type GlobalRoleBinding struct {
	metav1.TypeMeta `json:",inline"`
	Metadata        metav1.ObjectMeta `json:"metadata"`
	RoleRef         RoleRef           `json:"roleRef"`
	Subjects        []Subject         `json:"subjects"`
}

func defaultSubjects(subjects []Subject) {
	for i := range subjects {
		if subjects[i].Kind == "" {
			subjects[i].Kind = SubjectKindUser
		}
	}
}

func validateSubjects(subjects []Subject) error {
	if len(subjects) == 0 {
		return errors.New("subjects must not be empty")
	}
	for i, subject := range subjects {
		switch subject.Kind {
		case "", SubjectKindUser, SubjectKindNode, SubjectKindGroup, SubjectKindSystem:
		default:
			return fmt.Errorf("subjects[%d].kind %q is unsupported; allowed kinds are User, Node, Group, System", i, subject.Kind)
		}
	}
	return nil
}

func (b *GlobalRoleBinding) GetTypeMeta() *metav1.TypeMeta     { return &b.TypeMeta }
func (b *GlobalRoleBinding) GetObjectMeta() *metav1.ObjectMeta { return &b.Metadata }

func (b *GlobalRoleBinding) DefaultMeta() {
	b.TypeMeta = metav1.TypeMeta{APIVersion: GroupVersion, Kind: "GlobalRoleBinding"}
	defaultSubjects(b.Subjects)
}

func (b *GlobalRoleBinding) Validate() error {
	if strings.TrimSpace(b.Metadata.Name) == "" {
		return errors.New("globalrolebinding metadata.name is required")
	}
	if strings.TrimSpace(b.RoleRef.Name) == "" {
		return errors.New("roleRef.name is required")
	}
	return validateSubjects(b.Subjects)
}

// TenantRole defines permissions scoped to a single Tenant.
type TenantRole struct {
	metav1.TypeMeta `json:",inline"`
	Metadata        metav1.ObjectMeta `json:"metadata"`
	Description     string            `json:"description,omitempty"`
	Rules           []PolicyRule      `json:"rules"`
}

func (r *TenantRole) GetTypeMeta() *metav1.TypeMeta     { return &r.TypeMeta }
func (r *TenantRole) GetObjectMeta() *metav1.ObjectMeta { return &r.Metadata }

func (r *TenantRole) DefaultMeta() {
	r.TypeMeta = metav1.TypeMeta{APIVersion: GroupVersion, Kind: "TenantRole"}
}

func (r *TenantRole) Validate() error {
	if strings.TrimSpace(r.Metadata.Name) == "" {
		return errors.New("tenantrole metadata.name is required")
	}
	if strings.TrimSpace(r.Metadata.Tenant) == "" {
		return errors.New("tenantrole metadata.tenant is required")
	}
	for i, rule := range r.Rules {
		if err := rule.Validate(); err != nil {
			return fmt.Errorf("rules[%d]: %w", i, err)
		}
	}
	return nil
}

// TenantRoleBinding binds subjects to a TenantRole within a single Tenant.
type TenantRoleBinding struct {
	metav1.TypeMeta `json:",inline"`
	Metadata        metav1.ObjectMeta `json:"metadata"`
	RoleRef         RoleRef           `json:"roleRef"`
	Subjects        []Subject         `json:"subjects"`
}

func (b *TenantRoleBinding) GetTypeMeta() *metav1.TypeMeta     { return &b.TypeMeta }
func (b *TenantRoleBinding) GetObjectMeta() *metav1.ObjectMeta { return &b.Metadata }

func (b *TenantRoleBinding) DefaultMeta() {
	b.TypeMeta = metav1.TypeMeta{APIVersion: GroupVersion, Kind: "TenantRoleBinding"}
	defaultSubjects(b.Subjects)
}

func (b *TenantRoleBinding) Validate() error {
	if strings.TrimSpace(b.Metadata.Name) == "" {
		return errors.New("tenantrolebinding metadata.name is required")
	}
	if strings.TrimSpace(b.Metadata.Tenant) == "" {
		return errors.New("tenantrolebinding metadata.tenant is required")
	}
	if strings.TrimSpace(b.RoleRef.Name) == "" {
		return errors.New("roleRef.name is required")
	}
	return validateSubjects(b.Subjects)
}

// Session represents a declarative login token request/response.
type Session struct {
	metav1.TypeMeta `json:",inline"`
	Metadata        metav1.ObjectMeta `json:"metadata"`
	Kind            string            `json:"kind,omitempty"`       // "User" (default) or "Node"
	Identifier      string            `json:"identifier,omitempty"` // User name/email or Node name
	Password        string            `json:"password,omitempty"`   // Required when Kind is "User"
	Token           string            `json:"token,omitempty"`
	User            string            `json:"user,omitempty"`
	DefaultTenant   string            `json:"defaultTenant,omitempty"`
	ExpiresAt       time.Time         `json:"expiresAt,omitempty"`
}

func (s *Session) GetTypeMeta() *metav1.TypeMeta     { return &s.TypeMeta }
func (s *Session) GetObjectMeta() *metav1.ObjectMeta { return &s.Metadata }

func (s *Session) DefaultMeta() {
	s.TypeMeta = metav1.TypeMeta{APIVersion: GroupVersion, Kind: "Session"}
}

func (s *Session) Validate() error {
	return nil
}
