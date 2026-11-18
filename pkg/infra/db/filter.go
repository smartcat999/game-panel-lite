package db

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
)

// ListOptions specifies filtering and pagination for List operations.
type ListOptions struct {
	Tenant         string
	LabelSelector  *metav1.LabelSelector
	FieldSelector  map[string]string
	Limit          int
	Offset         int
	SkipTotalCount bool
}

// JSONFieldPathLiteral compiles a field selector path to a SQL text-array literal.
func JSONFieldPathLiteral(field string) (string, error) {
	parts := strings.Split(field, ".")
	for index, part := range parts {
		for i := range len(part) {
			ch := part[i]
			if ch == '_' || ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z' || i > 0 && ch >= '0' && ch <= '9' {
				continue
			}
			return "", fmt.Errorf("invalid field selector path %q: segments must match [A-Za-z_][A-Za-z0-9_]*", field)
		}
		if part == "" {
			return "", fmt.Errorf("invalid field selector path %q: empty segment", field)
		}
		if strings.EqualFold(part, "null") {
			parts[index] = `"` + part + `"`
		}
	}
	return "'{" + strings.Join(parts, ",") + "}'", nil
}

// WildcardToSQLLike converts wildcard '*' patterns to SQL LIKE patterns with '\' escape.
func WildcardToSQLLike(pattern string) string {
	var sb strings.Builder
	for _, ch := range pattern {
		switch ch {
		case '%', '_':
			sb.WriteRune('\\')
			sb.WriteRune(ch)
		case '*':
			sb.WriteRune('%')
		default:
			sb.WriteRune(ch)
		}
	}
	return sb.String()
}

// ApplyNameInWhere adds an IN clause for comma-separated names on colExpr.
func ApplyNameInWhere(tx *gorm.DB, colExpr, expected string) *gorm.DB {
	var names []string
	for _, part := range strings.Split(expected, ",") {
		if p := strings.TrimSpace(part); p != "" {
			names = append(names, p)
		}
	}
	if len(names) == 0 {
		return tx.Where("FALSE")
	}
	return tx.Where(colExpr+" IN ?", names)
}

// ApplySubjectsWhere matches role bindings whose JSONB subjects column contains any comma-separated subject name.
func ApplySubjectsWhere(tx *gorm.DB, colExpr, expected string) *gorm.DB {
	orCond := tx.Session(&gorm.Session{NewDB: true})
	added := 0
	for _, part := range strings.Split(expected, ",") {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		needle, _ := json.Marshal([]map[string]string{{"name": p}})
		orCond = orCond.Or("("+colExpr+" @> ?::jsonb)", string(needle))
		added++
	}
	if added == 0 {
		return tx.Where("FALSE")
	}
	return tx.Where(orCond)
}

// ApplyLabelSelector applies LabelSelector predicates against the "labels" JSONB column.
func ApplyLabelSelector(tx *gorm.DB, sel *metav1.LabelSelector) *gorm.DB {
	return ApplyLabelSelectorCol(tx, "labels", sel)
}

// ApplyLabelSelectorCol applies LabelSelector predicates against colExpr (e.g. "labels" or "instances.labels").
func ApplyLabelSelectorCol(tx *gorm.DB, colExpr string, sel *metav1.LabelSelector) *gorm.DB {
	if sel == nil {
		return tx
	}
	var labelKeys []string
	for k := range sel.MatchLabels {
		labelKeys = append(labelKeys, k)
	}
	sort.Strings(labelKeys)
	for _, k := range labelKeys {
		expected := sel.MatchLabels[k]
		if expected == "*" {
			tx = tx.Where("jsonb_exists("+colExpr+", ?)", k)
		} else if strings.Contains(expected, "*") {
			tx = tx.Where("("+colExpr+" ->> ? LIKE ? ESCAPE '\\')", k, WildcardToSQLLike(expected))
		} else {
			tx = tx.Where("("+colExpr+" ->> ? = ?)", k, expected)
		}
	}
	for _, expr := range sel.MatchExpressions {
		tx = applyLabelExpressionCol(tx, colExpr, expr)
	}
	return tx
}

func applyLabelExpressionCol(tx *gorm.DB, colExpr string, expr metav1.LabelSelectorRequirement) *gorm.DB {
	switch expr.Operator {
	case metav1.LabelSelectorOpExists, "exists":
		return tx.Where("jsonb_exists("+colExpr+", ?)", expr.Key)
	case metav1.LabelSelectorOpDoesNotExist, "doesnotexist":
		return tx.Where("NOT jsonb_exists("+colExpr+", ?)", expr.Key)
	case metav1.LabelSelectorOpIn, "in", "=":
		return applyLabelInExprCol(tx, colExpr, expr)
	case metav1.LabelSelectorOpNotIn, "notin", "!=":
		return applyLabelNotInExprCol(tx, colExpr, expr)
	default:
		return tx
	}
}

func applyLabelInExprCol(tx *gorm.DB, colExpr string, expr metav1.LabelSelectorRequirement) *gorm.DB {
	var exactVals []string
	orCond := tx.Session(&gorm.Session{NewDB: true})
	added := 0
	for _, v := range expr.Values {
		if v == "*" {
			orCond = orCond.Or("jsonb_exists("+colExpr+", ?)", expr.Key)
			added++
		} else if strings.Contains(v, "*") {
			orCond = orCond.Or("("+colExpr+" ->> ? LIKE ? ESCAPE '\\')", expr.Key, WildcardToSQLLike(v))
			added++
		} else {
			exactVals = append(exactVals, v)
		}
	}
	if len(exactVals) == 1 {
		orCond = orCond.Or("("+colExpr+" ->> ? = ?)", expr.Key, exactVals[0])
		added++
	} else if len(exactVals) > 1 {
		orCond = orCond.Or("("+colExpr+" ->> ? IN ?)", expr.Key, exactVals)
		added++
	}
	if added > 0 {
		return tx.Where(orCond)
	}
	return tx
}

func applyLabelNotInExprCol(tx *gorm.DB, colExpr string, expr metav1.LabelSelectorRequirement) *gorm.DB {
	var exactVals []string
	negCond := tx.Session(&gorm.Session{NewDB: true})
	added := 0
	for _, v := range expr.Values {
		if v == "*" {
			negCond = negCond.Where("NOT jsonb_exists("+colExpr+", ?)", expr.Key)
			added++
		} else if strings.Contains(v, "*") {
			negCond = negCond.Where("("+colExpr+" ->> ? NOT LIKE ? ESCAPE '\\')", expr.Key, WildcardToSQLLike(v))
			added++
		} else {
			exactVals = append(exactVals, v)
		}
	}
	if len(exactVals) == 1 {
		negCond = negCond.Where("("+colExpr+" ->> ? != ?)", expr.Key, exactVals[0])
		added++
	} else if len(exactVals) > 1 {
		negCond = negCond.Where("("+colExpr+" ->> ? NOT IN ?)", expr.Key, exactVals)
		added++
	}
	if added > 0 {
		return tx.Where("NOT jsonb_exists("+colExpr+", ?) OR ?", expr.Key, negCond)
	}
	return tx
}
