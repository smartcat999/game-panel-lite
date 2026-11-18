package v1

import (
	"testing"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
)

func TestPolicyRuleMatchesResourceNamesCollectionBounds(t *testing.T) {
	allowedRule := PolicyRule{
		APIGroups:     []string{"core.gamepanel.io"},
		Resources:     []string{"instances"},
		Verbs:         []string{VerbGet, VerbList, VerbWatch},
		ResourceNames: []string{"allowed"},
	}
	if allowedRule.Matches("core.gamepanel.io", "instances", VerbList, "", nil) {
		t.Fatal("resourceNames rule unexpectedly matched unscoped list")
	}
	if allowedRule.Matches("core.gamepanel.io", "instances", VerbWatch, "", nil) {
		t.Fatal("resourceNames rule unexpectedly matched unscoped watch")
	}
	if !allowedRule.Matches("core.gamepanel.io", "instances", VerbGet, "allowed", nil) {
		t.Fatal("expected exact resourceName match to succeed")
	}
	if allowedRule.Matches("core.gamepanel.io", "instances", VerbGet, "victim", nil) {
		t.Fatal("expected non-matching resourceName to be denied")
	}

	prefixRule := allowedRule
	prefixRule.ResourceNames = []string{"prod-*"}
	if prefixRule.Matches("core.gamepanel.io", "instances", VerbList, "", nil) {
		t.Fatal("prefix resourceNames rule unexpectedly matched unscoped list")
	}
	if !prefixRule.Matches("core.gamepanel.io", "instances", VerbGet, "prod-1", nil) {
		t.Fatal("expected prefix resourceName match to succeed")
	}

	wildcardRule := allowedRule
	wildcardRule.ResourceNames = []string{"*"}
	if !wildcardRule.Matches("core.gamepanel.io", "instances", VerbList, "", nil) {
		t.Fatal("expected explicit wildcard resourceNames rule to match list")
	}
}

func TestPolicyRuleMatchesSelectorCollectionBounds(t *testing.T) {
	notInRule := PolicyRule{
		APIGroups: []string{"core.gamepanel.io"},
		Resources: []string{"instances"},
		Verbs:     []string{VerbGet, VerbList, VerbWatch},
		Selector: &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{Key: "env", Operator: metav1.LabelSelectorOpNotIn, Values: []string{"prod"}},
			},
		},
	}
	if notInRule.Matches("core.gamepanel.io", "instances", VerbList, "", nil) {
		t.Fatal("NotIn selector rule unexpectedly matched unscoped list")
	}
	if notInRule.Matches("core.gamepanel.io", "instances", VerbWatch, "", nil) {
		t.Fatal("NotIn selector rule unexpectedly matched unscoped watch")
	}
	if !notInRule.Matches("core.gamepanel.io", "instances", VerbGet, "dev-1", map[string]string{"env": "dev"}) {
		t.Fatal("expected named object with env=dev to match NotIn selector")
	}
	if notInRule.Matches("core.gamepanel.io", "instances", VerbGet, "prod-1", map[string]string{"env": "prod"}) {
		t.Fatal("expected named object with env=prod to be denied")
	}

	doesNotExistRule := notInRule
	doesNotExistRule.Selector = &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{Key: "restricted", Operator: metav1.LabelSelectorOpDoesNotExist},
		},
	}
	if doesNotExistRule.Matches("core.gamepanel.io", "instances", VerbList, "", nil) {
		t.Fatal("DoesNotExist selector rule unexpectedly matched unscoped list")
	}

	emptySelRule := notInRule
	emptySelRule.Selector = &metav1.LabelSelector{}
	if !emptySelRule.Matches("core.gamepanel.io", "instances", VerbList, "", nil) {
		t.Fatal("expected empty selector rule to match collection list")
	}
}

func TestPolicyRuleMatchesSelectorWildcardAndNilLabels(t *testing.T) {
	posRule := PolicyRule{
		APIGroups: []string{"core.gamepanel.io"},
		Resources: []string{"apps"},
		Verbs:     []string{VerbGet},
		Selector:  &metav1.LabelSelector{MatchLabels: map[string]string{"env": "dev"}},
	}
	negRule := posRule
	negRule.Selector = &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{Key: "restricted", Operator: metav1.LabelSelectorOpNotIn, Values: []string{"secret"}},
			{Key: "blocked", Operator: metav1.LabelSelectorOpDoesNotExist},
		},
	}
	if posRule.Matches("core.gamepanel.io", "apps", VerbGet, "*", map[string]string{"env": "dev"}) {
		t.Fatal("positive selector matched wildcard resourceName *")
	}
	if negRule.Matches("core.gamepanel.io", "apps", VerbGet, "*", map[string]string{}) {
		t.Fatal("negative selector matched wildcard resourceName *")
	}
	if posRule.Matches("core.gamepanel.io", "apps", VerbGet, "app-1", nil) || negRule.Matches("core.gamepanel.io", "apps", VerbGet, "app-1", nil) {
		t.Fatal("selector rule matched nil labels")
	}
	if posRule.Matches("core.gamepanel.io", "apps", VerbGet, "app-1", map[string]string{}) {
		t.Fatal("positive selector matched empty non-nil labels")
	}
	if !negRule.Matches("core.gamepanel.io", "apps", VerbGet, "app-1", map[string]string{}) {
		t.Fatal("negative selector failed to match named resource with empty non-nil labels")
	}
}
