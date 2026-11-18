package runtime

import (
	"net/http/httptest"
	"testing"
)

func TestParseListOptionsFieldSelectorSetTerms(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  map[string]string
	}{
		{"single equality", "spec.owner=alice", map[string]string{"spec.owner": "alice"}},
		{"set values continue across commas", "metadata.name.in=a,b,c", map[string]string{"metadata.name.in": "a,b,c"}},
		{"set followed by equality", "metadata.name.in=a,b,state=Paid", map[string]string{"metadata.name.in": "a,b", "state": "Paid"}},
		{"equality followed by set", "state=Paid,metadata.name.in=x,y", map[string]string{"state": "Paid", "metadata.name.in": "x,y"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/x?fieldSelector="+tc.query, nil)
			opts, err := ParseListOptions(req)
			if err != nil {
				t.Fatalf("ParseListOptions(%q): %v", tc.query, err)
			}
			if len(opts.FieldSelector) != len(tc.want) {
				t.Fatalf("selector = %v, want %v", opts.FieldSelector, tc.want)
			}
			for k, v := range tc.want {
				if opts.FieldSelector[k] != v {
					t.Fatalf("selector[%q] = %q, want %q (all=%v)", k, opts.FieldSelector[k], v, opts.FieldSelector)
				}
			}
		})
	}
}

func TestParseListOptionsFieldSelectorRejectsDanglingValue(t *testing.T) {
	req := httptest.NewRequest("GET", "/x?fieldSelector=spec.owner=alice,bob", nil)
	if _, err := ParseListOptions(req); err == nil {
		t.Fatalf("expected error for value without key after a non-set term")
	}
}
