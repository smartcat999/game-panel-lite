package v1

import (
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
)

// TypeMeta describes an individual object in an API response or request.
type TypeMeta struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
}

// ObjectMeta is metadata that all persisted declarative resources must have.
type ObjectMeta struct {
	Name              string            `json:"name"`
	Tenant            string            `json:"tenant,omitempty"`
	UID               string            `json:"uid"`
	ResourceVersion   string            `json:"resourceVersion"`
	StatusVersion     string            `json:"statusVersion,omitempty"`
	Generation        int64             `json:"generation"`
	CreationTimestamp time.Time         `json:"creationTimestamp"`
	DeletionTimestamp *time.Time        `json:"deletionTimestamp,omitempty"`
	Finalizers        []string          `json:"finalizers,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`
}

// ListMeta describes metadata that synthetic list resources must have.
type ListMeta struct {
	ResourceVersion string `json:"resourceVersion,omitempty"`
	Continue        string `json:"continue,omitempty"`
	TotalCount      int    `json:"totalCount"`
}

// Condition contains details for one aspect of the current state of this API Resource.
type Condition struct {
	Type               string    `json:"type"`
	Status             string    `json:"status"` // "True", "False", "Unknown"
	ObservedGeneration int64     `json:"observedGeneration"`
	LastTransitionTime time.Time `json:"lastTransitionTime"`
	Reason             string    `json:"reason"`
	Message            string    `json:"message"`
}

const (
	ConditionTrue    = "True"
	ConditionFalse   = "False"
	ConditionUnknown = "Unknown"

	ConditionTypeReady             = "Ready"
	ConditionTypeReconciled        = "Reconciled"
	ConditionTypeRuntimeReady      = "RuntimeReady"
	ConditionTypeRuntimeCleanup    = "RuntimeCleanup"
	ConditionTypeClockSynced       = "ClockSynced"
	ConditionTypeScheduled         = "Scheduled"
	ConditionTypeQuotaReserved     = "QuotaReserved"
	ConditionTypeResourcesApproved = "ResourcesApproved"
	ConditionTypeResourcesSettled  = "ResourcesSettled"
)

type LabelSelectorOperator string

const (
	LabelSelectorOpIn           LabelSelectorOperator = "In"
	LabelSelectorOpNotIn        LabelSelectorOperator = "NotIn"
	LabelSelectorOpExists       LabelSelectorOperator = "Exists"
	LabelSelectorOpDoesNotExist LabelSelectorOperator = "DoesNotExist"
)

type LabelSelectorRequirement struct {
	Key      string                `json:"key"`
	Operator LabelSelectorOperator `json:"operator"`
	Values   []string              `json:"values,omitempty"`
}

// LabelSelector is a label query over a set of resources.
type LabelSelector struct {
	MatchLabels      map[string]string          `json:"matchLabels,omitempty"`
	MatchExpressions []LabelSelectorRequirement `json:"matchExpressions,omitempty"`
}

// Matches returns true if the provided resource labels satisfy the selector.
func (s *LabelSelector) Matches(labels map[string]string) bool {
	if s == nil || (len(s.MatchLabels) == 0 && len(s.MatchExpressions) == 0) {
		return true
	}
	for k, expected := range s.MatchLabels {
		actual, ok := labels[k]
		if !ok || !WildcardMatch(expected, actual) {
			return false
		}
	}
	for _, expr := range s.MatchExpressions {
		val, exists := labels[expr.Key]
		switch expr.Operator {
		case LabelSelectorOpIn, "in", "=":
			if !exists {
				return false
			}
			matched := false
			for _, v := range expr.Values {
				if WildcardMatch(v, val) {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		case LabelSelectorOpNotIn, "notin", "!=":
			if !exists {
				break
			}
			for _, v := range expr.Values {
				if WildcardMatch(v, val) {
					return false
				}
			}
		case LabelSelectorOpExists, "exists":
			if !exists {
				return false
			}
		case LabelSelectorOpDoesNotExist, "doesnotexist":
			if exists {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// ParseLabelSelector parses a comma-separated label selector query string
// (e.g., "key=value,tier in (web, api),env!=prod,!retired,healthy") into *LabelSelector.
func ParseLabelSelector(s string) (*LabelSelector, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	terms, err := splitSelectorTerms(s)
	if err != nil {
		return nil, err
	}
	selector := &LabelSelector{
		MatchLabels:      make(map[string]string),
		MatchExpressions: make([]LabelSelectorRequirement, 0),
	}
	for _, term := range terms {
		if err := applySelectorTerm(selector, strings.TrimSpace(term)); err != nil {
			return nil, err
		}
	}
	return selector, nil
}

func applySelectorTerm(selector *LabelSelector, term string) error {
	if term == "" {
		return errors.New("empty term in label selector")
	}
	if key, rest, found := findWordOperator(term, "notin"); found {
		vals, err := extractParenthesesValues(rest)
		if err != nil {
			return fmt.Errorf("invalid notin expression %q: %w", term, err)
		}
		selector.MatchExpressions = append(selector.MatchExpressions, LabelSelectorRequirement{Key: key, Operator: LabelSelectorOpNotIn, Values: vals})
		return nil
	}
	if key, rest, found := findWordOperator(term, "in"); found {
		vals, err := extractParenthesesValues(rest)
		if err != nil {
			return fmt.Errorf("invalid in expression %q: %w", term, err)
		}
		selector.MatchExpressions = append(selector.MatchExpressions, LabelSelectorRequirement{Key: key, Operator: LabelSelectorOpIn, Values: vals})
		return nil
	}
	return applyComparisonOrUnaryTerm(selector, term)
}

func applyComparisonOrUnaryTerm(selector *LabelSelector, term string) error {
	if strings.Contains(term, "!=") {
		parts := strings.SplitN(term, "!=", 2)
		key, val := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		if key == "" || val == "" {
			return fmt.Errorf("invalid inequality expression %q: key and value required", term)
		}
		selector.MatchExpressions = append(selector.MatchExpressions, LabelSelectorRequirement{Key: key, Operator: LabelSelectorOpNotIn, Values: []string{val}})
		return nil
	}
	for _, sep := range []string{"==", "="} {
		if strings.Contains(term, sep) {
			parts := strings.SplitN(term, sep, 2)
			key, val := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
			if key == "" || val == "" {
				return fmt.Errorf("invalid equality expression %q: key and value required", term)
			}
			selector.MatchLabels[key] = val
			return nil
		}
	}
	if strings.HasPrefix(term, "!") {
		key := strings.TrimSpace(term[1:])
		if key == "" {
			return fmt.Errorf("missing key after '!' in %q", term)
		}
		selector.MatchExpressions = append(selector.MatchExpressions, LabelSelectorRequirement{Key: key, Operator: LabelSelectorOpDoesNotExist})
		return nil
	}
	selector.MatchExpressions = append(selector.MatchExpressions, LabelSelectorRequirement{Key: term, Operator: LabelSelectorOpExists})
	return nil
}

func splitSelectorTerms(s string) ([]string, error) {
	var terms []string
	var current strings.Builder
	depth := 0
	for _, ch := range s {
		switch ch {
		case '(':
			depth++
			current.WriteRune(ch)
		case ')':
			depth--
			if depth < 0 {
				return nil, errors.New("unmatched closing parenthesis ')'")
			}
			current.WriteRune(ch)
		case ',':
			if depth == 0 {
				raw := current.String()
				t := strings.TrimSpace(raw)
				if t == "" {
					return nil, errors.New("empty term in label selector")
				}
				terms = append(terms, t)
				current.Reset()
			} else {
				current.WriteRune(ch)
			}
		default:
			current.WriteRune(ch)
		}
	}
	if depth > 0 {
		return nil, errors.New("unmatched opening parenthesis '('")
	}
	raw := current.String()
	t := strings.TrimSpace(raw)
	if t == "" {
		return nil, errors.New("empty term in label selector")
	}
	terms = append(terms, t)
	return terms, nil
}

func extractParenthesesValues(s string) ([]string, error) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "(") || !strings.HasSuffix(s, ")") {
		return nil, errors.New("missing enclosing parentheses (...)")
	}
	inner := strings.TrimSpace(s[1 : len(s)-1])
	if inner == "" {
		return nil, errors.New("values list cannot be empty")
	}
	parts := strings.Split(inner, ",")
	var res []string
	for _, p := range parts {
		item := strings.TrimSpace(p)
		if item == "" {
			return nil, errors.New("values list contains empty value")
		}
		res = append(res, item)
	}
	if len(res) == 0 {
		return nil, errors.New("values list cannot be empty")
	}
	return res, nil
}

func isWhitespaceByte(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

func findWordOperator(term, op string) (string, string, bool) {
	opLen := len(op)
	// term must have room for at least 1 key byte, 1 delimiter byte, opLen operator bytes, and 1 delimiter byte
	for i := 1; i+opLen < len(term); i++ {
		// Delimiter check before operator: must be ASCII whitespace
		if !isWhitespaceByte(term[i-1]) {
			continue
		}

		// Delimiter check after operator: must be ASCII whitespace or '('
		next := term[i+opLen]
		if !isWhitespaceByte(next) && next != '(' {
			continue
		}

		// Direct case-insensitive comparison on candidate slice without converting the whole term
		if !strings.EqualFold(term[i:i+opLen], op) {
			continue
		}

		key := strings.TrimSpace(term[:i])
		rest := strings.TrimSpace(term[i+opLen:])
		if key == "" {
			continue
		}
		return key, rest, true
	}
	return "", "", false
}

// WildcardMatch matches a pattern supporting '*' against a value (including '/' segments such as "instances/*").
func WildcardMatch(pattern, value string) bool {
	pattern = strings.TrimSpace(pattern)
	value = strings.TrimSpace(value)
	if pattern == "*" || pattern == value {
		return true
	}
	if ok, err := doublestar.Match(pattern, value); err == nil && ok {
		return true
	}
	if strings.Contains(pattern, "*") {
		if ok, err := doublestar.Match(strings.ReplaceAll(pattern, "*", "**"), value); err == nil && ok {
			return true
		}
	}
	return false
}

// Object is implemented by all declarative resources.
type Object interface {
	GetTypeMeta() *TypeMeta
	GetObjectMeta() *ObjectMeta
	DefaultMeta()
	Validate() error
}

// List is a generic envelope for resource lists.
type List[T any] struct {
	TypeMeta `json:",inline"`
	Metadata ListMeta `json:"metadata"`
	Items    []T      `json:"items"`
}

// WatchEventType defines the possible types of watch events.
type WatchEventType string

const (
	WatchAdded    WatchEventType = "ADDED"
	WatchModified WatchEventType = "MODIFIED"
	WatchDeleted  WatchEventType = "DELETED"
)

// WatchEvent represents a single event to a watched resource.
type WatchEvent[T any] struct {
	Type   WatchEventType `json:"type"`
	Object T              `json:"object"`
}

// GetCondition returns the Condition with the specified type, or nil if not found.
func GetCondition(conditions []Condition, condType string) *Condition {
	for i := range conditions {
		if conditions[i].Type == condType {
			return &conditions[i]
		}
	}
	return nil
}

// SetCondition updates or appends a Condition in a slice.
func SetCondition(conditions *[]Condition, cond Condition) {
	if cond.LastTransitionTime.IsZero() {
		cond.LastTransitionTime = time.Now().UTC()
	}
	for i := range *conditions {
		if (*conditions)[i].Type == cond.Type {
			if (*conditions)[i].Status == cond.Status {
				cond.LastTransitionTime = (*conditions)[i].LastTransitionTime
			}
			(*conditions)[i] = cond
			return
		}
	}
	*conditions = append(*conditions, cond)
}

// ValidateSingleSegmentName verifies that a resource or tenant identifier is a safe,
// non-empty, single-segment identifier without path separators or traversal segments.
func ValidateSingleSegmentName(field, value string) error {
	if value == "" {
		return fmt.Errorf("%s is required", field)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("invalid %s %q: must not have leading or trailing whitespace", field, value)
	}
	if len(value) > 63 {
		return fmt.Errorf("invalid %s %q: must be at most 63 characters", field, value)
	}
	if value == "." || value == ".." || strings.Contains(value, "..") {
		return fmt.Errorf("invalid %s %q: path traversal segments are not allowed", field, value)
	}
	for i := range len(value) {
		ch := value[i]
		isAlphaNum := (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9')
		if (i == 0 || i == len(value)-1) && !isAlphaNum {
			return fmt.Errorf("invalid %s %q: must start and end with an alphanumeric character", field, value)
		}
		if !isAlphaNum && ch != '-' && ch != '_' && ch != '.' {
			return fmt.Errorf("invalid %s %q: contains disallowed character %q", field, value, ch)
		}
	}
	return nil
}

// NormalizeRelativeFilePath validates and normalizes a workspace-relative path for os.Root operations.
// When allowRoot is true, empty, "/", and "." normalize to ".".
func NormalizeRelativeFilePath(rawPath string, allowRoot bool) (string, error) {
	trimmed := strings.TrimSpace(rawPath)
	if strings.ContainsRune(trimmed, '\x00') || strings.ContainsRune(trimmed, '\\') {
		return "", fmt.Errorf("invalid file path %q: contains disallowed characters", rawPath)
	}
	if trimmed == "" || trimmed == "/" || trimmed == "." {
		if allowRoot {
			return ".", nil
		}
		return "", errors.New("invalid file path: path cannot be empty")
	}
	rel := strings.TrimPrefix(trimmed, "/")
	for _, seg := range strings.Split(rel, "/") {
		if seg == ".." {
			return "", fmt.Errorf("invalid file path %q: escapes instance working directory", rawPath)
		}
	}
	clean := path.Clean(rel)
	if clean == "." {
		if allowRoot {
			return ".", nil
		}
		return "", errors.New("invalid file path: path cannot be empty")
	}
	if clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
		return "", fmt.Errorf("invalid file path %q: escapes instance working directory", rawPath)
	}
	return clean, nil
}

// ValidateRelativeFilePath verifies that rawPath is a valid relative file/directory path inside an instance root,
// rejecting leading '/' (absolute paths), '..', '\', and empty paths.
func ValidateRelativeFilePath(rawPath string) error {
	trimmed := strings.TrimSpace(rawPath)
	if strings.HasPrefix(trimmed, "/") {
		return fmt.Errorf("invalid file path %q: escapes instance working directory", rawPath)
	}
	_, err := NormalizeRelativeFilePath(rawPath, false)
	return err
}

func hasWindowsDrivePrefix(p string) bool {
	if len(p) < 2 || p[1] != ':' {
		return false
	}
	return (p[0] >= 'a' && p[0] <= 'z') || (p[0] >= 'A' && p[0] <= 'Z')
}

// NormalizeArchiveEntryPath normalizes archive entry paths by converting '\' to '/' before
// rejecting null bytes, Windows drive prefixes, leading slashes, and '..' traversal segments.
func NormalizeArchiveEntryPath(rawPath string, allowRoot bool) (string, error) {
	trimmed := strings.TrimSpace(rawPath)
	if strings.ContainsRune(trimmed, '\x00') {
		return "", fmt.Errorf("invalid archive path %q: contains null byte", rawPath)
	}
	norm := strings.ReplaceAll(trimmed, "\\", "/")
	if norm == "" || norm == "/" || norm == "." {
		if allowRoot {
			return ".", nil
		}
		return "", errors.New("invalid archive path: path cannot be empty")
	}
	if strings.HasPrefix(norm, "/") || hasWindowsDrivePrefix(norm) {
		return "", fmt.Errorf("invalid archive path %q: absolute paths are not allowed", rawPath)
	}
	for strings.HasPrefix(norm, "./") {
		norm = strings.TrimPrefix(norm, "./")
	}
	for _, seg := range strings.Split(norm, "/") {
		if seg == ".." {
			return "", fmt.Errorf("invalid archive path %q: traversal segments are not allowed", rawPath)
		}
	}
	clean := path.Clean(norm)
	if clean == "." {
		if allowRoot {
			return ".", nil
		}
		return "", errors.New("invalid archive path: path cannot be empty")
	}
	if clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
		return "", fmt.Errorf("invalid archive path %q: traversal segments are not allowed", rawPath)
	}
	return clean, nil
}
