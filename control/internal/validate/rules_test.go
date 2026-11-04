package validate

import (
	"testing"

	"corvus/control/internal/store"
)

func rule(conds ...store.Condition) *store.SecurityRule {
	return &store.SecurityRule{Name: "관리자 보호", Action: "block", Expression: store.Expression{Groups: [][]store.Condition{conds}}}
}

func TestSecurityRuleValidation(t *testing.T) {
	ok := []store.Condition{
		{Field: "ip.src", Operator: "in", Value: []string{"10.0.0.0/8", "192.168.1.1", "list:1"}},
		{Field: "ip.country", Operator: "not_in", Value: []string{"kr"}},
		{Field: "http.path", Operator: "starts_with", Value: []string{"/admin"}, CI: true},
		{Field: "http.header", Name: "X-Api-Key", Operator: "not_exists"},
		{Field: "http.user_agent", Operator: "matches", Value: []string{`(?i)sqlmap|nikto`}},
		{Field: "http.method", Operator: "in", Value: []string{"post", "PUT"}},
		{Field: "http.body", Operator: "matches", Value: []string{`resolved_model`}, CI: true},
	}
	r := rule(ok...)
	if err := SecurityRule(r, func(id int64) string {
		if id == 1 {
			return store.ListKindIP
		}
		return ""
	}, nil); err != nil {
		t.Fatalf("valid rule rejected: %v", err)
	}
	if r.StatusCode != 403 || r.Scope != "all" || r.Expression.Groups[0][1].Value[0] != "KR" || r.Expression.Groups[0][3].Name != "x-api-key" {
		t.Fatalf("not normalised: %+v", r)
	}
	bad := []store.Condition{
		{Field: "ip.src", Operator: "contains", Value: []string{"10."}},
		{Field: "ip.src", Operator: "in", Value: []string{"10.0.0.0/33"}},
		{Field: "ip.src", Operator: "in", Value: []string{"list:9"}},
		{Field: "ip.country", Operator: "in", Value: []string{"KOR"}},
		{Field: "http.path", Operator: "eq", Value: []string{"/a", "/b"}},
		{Field: "http.path", Operator: "matches", Value: []string{`(a)\1`}},
		{Field: "http.path", Operator: "matches", Value: []string{`(*LIMIT_MATCH=1)a`}},
		{Field: "http.header", Operator: "exists"},
		{Field: "http.path", Operator: "eq", Value: []string{"a\nb"}},
		{Field: "http.cookie", Operator: "contains", Value: []string{"x"}},
		{Field: "http.body", Operator: "eq", Value: []string{"x"}},
		{Field: "http.query", Operator: "exists"},
	}
	for i, c := range bad {
		if err := SecurityRule(rule(c), func(int64) string { return "" }, nil); err == nil {
			t.Errorf("bad condition %d accepted: %+v", i, c)
		}
	}
	r = rule(ok[2])
	r.Action, r.StatusCode = "block", 500
	if SecurityRule(r, nil, nil) == nil {
		t.Error("status 500 accepted")
	}
	r = rule(ok[2])
	r.Scope = "hosts"
	if SecurityRule(r, nil, nil) == nil {
		t.Error("empty host scope accepted")
	}
	if SecurityRule(&store.SecurityRule{Name: "x", Action: "block"}, nil, nil) == nil {
		t.Error("empty expression accepted")
	}
}

func TestIPListValidation(t *testing.T) {
	l := &store.IPList{Name: "사무실", Items: []string{"10.0.0.1", " 10.0.0.0/8 ", ""}}
	if err := IPList(l); err != nil || len(l.Items) != 2 || l.Items[0] != "10.0.0.1/32" {
		t.Fatalf("%v %+v", err, l.Items)
	}
	if IPList(&store.IPList{Name: `a$b`}) == nil || IPList(&store.IPList{Name: "x", Items: []string{"nope"}}) == nil {
		t.Error("invalid list accepted")
	}
}
