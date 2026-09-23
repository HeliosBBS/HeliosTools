package main

import (
	"strings"
	"testing"
	"time"
)

func mk(n int, labels ...string) issue {
	is := issue{Number: n, CreatedAt: time.Date(2026, 1, n, 0, 0, 0, 0, time.UTC)}
	for _, l := range labels {
		is.Labels = append(is.Labels, struct {
			Name string `json:"name"`
		}{l})
	}
	return is
}

func claimed(is issue, by string) issue {
	is.Labels = append(is.Labels, struct {
		Name string `json:"name"`
	}{"claimed"})
	is.Comments = append(is.Comments, struct {
		Body      string    `json:"body"`
		CreatedAt time.Time `json:"createdAt"`
	}{Body: "Claimed by " + by})
	return is
}

func blocked(is issue, state string) issue {
	is.BlockedBy = append(is.BlockedBy, struct {
		Number int    `json:"number"`
		State  string `json:"state"`
	}{1, state})
	return is
}

func TestSelectNext(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		issues []issue
		loop   bool
		want   int
		ok     bool
	}{
		{"own claim first", []issue{mk(1, "P0"), claimed(mk(2, "P3"), "me")}, false, 2, true},
		{"skip others' claim", []issue{claimed(mk(1, "P0"), "them"), mk(2, "P1")}, false, 2, true},
		{"priority before age", []issue{mk(1, "P2"), mk(2, "P0")}, false, 2, true},
		{"age within priority", []issue{mk(2, "P1"), mk(1, "P1")}, false, 1, true},
		{"unprioritised last", []issue{mk(1), mk(2, "P3")}, false, 2, true},
		{"skip blocked label", []issue{mk(1, "P0", "blocked"), mk(2, "P1")}, false, 2, true},
		{"skip human action", []issue{mk(1, "P0", "human-action-required"), mk(2, "P1")}, false, 2, true},
		{"skip open dependency", []issue{blocked(mk(1, "P0"), "OPEN"), mk(2, "P1")}, false, 2, true},
		{"closed dependency is fine", []issue{blocked(mk(1, "P0"), "CLOSED"), mk(2, "P1")}, false, 1, true},
		{"loop needs its label", []issue{mk(1, "P0"), mk(2, "P1", "unattended-loop")}, true, 2, true},
		{"nothing", []issue{mk(1, "P0", "blocked")}, false, 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := selectNext(tc.issues, "me", tc.loop)
			if ok != tc.ok || got.Number != tc.want {
				t.Fatalf("got #%d ok=%v, want #%d ok=%v", got.Number, ok, tc.want, tc.ok)
			}
		})
	}
}

const body = `### Files
a.go
### Exact change
x
### Reason
y
### Plan
- [ ] 1. first
- [x] 2. second
- [ ] 3. third
### Notes
- [ ] not a task
`

func TestTickBody(t *testing.T) {
	t.Parallel()
	got, err := tickBody(body, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "- [x] 3. third") || !strings.Contains(got, "- [ ] 1. first") {
		t.Fatalf("wrong box ticked:\n%s", got)
	}
	if !strings.Contains(got, "- [ ] not a task") {
		t.Fatal("touched a box outside ### Plan")
	}
	if _, err := tickBody(body, 2); err == nil {
		t.Fatal("ticking an already ticked task must fail")
	}
	if _, err := tickBody(body, 9); err == nil {
		t.Fatal("ticking a missing task must fail")
	}
}

func TestLintIssue(t *testing.T) {
	t.Parallel()
	good := mk(1, "P1", "kind:feature")
	good.Body = body
	if f := lintIssue(good); len(f) != 0 {
		t.Fatalf("good issue has faults: %v", f)
	}
	bad := mk(2, "P1", "P2")
	bad.Body = "### Files\n"
	f := lintIssue(bad)
	for _, want := range []string{"exactly one priority", "exactly one kind", "### Exact change", "### Reason", "### Plan"} {
		if !strings.Contains(strings.Join(f, "\n"), want) {
			t.Errorf("missing fault %q in %v", want, f)
		}
	}
	unplanned := claimed(mk(3, "P0", "kind:bug"), "me")
	unplanned.Body = "### Files\n### Exact change\n### Reason\n### Plan\n"
	if f := lintIssue(unplanned); len(f) != 1 || !strings.Contains(f[0], "no plan") {
		t.Fatalf("claimed-without-plan not caught: %v", f)
	}
}

func TestPlanIgnoresOtherSections(t *testing.T) {
	t.Parallel()
	is := issue{Body: body}
	if got := is.plan(); len(got) != 3 {
		t.Fatalf("want 3 plan lines, got %d: %v", len(got), got)
	}
}
