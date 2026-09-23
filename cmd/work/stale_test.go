package main

import (
	"reflect"
	"testing"
)

const stalePlan = "### Files\nx\n### Plan\n**Tasks**\n\n" +
	"- [x] 1. Done  [tier: haiku]\n      Acceptance: cluster.md, Behaviour\n" +
	"- [ ] 2. Lease  [tier: opus]\n      Files: internal/cluster/lease.go\n      Acceptance: cluster.md, Data model, operation 1\n      Serves: ADV-001\n" +
	"- [x] ~~3. Struck~~ struck: gone\n      Acceptance: stack.md\n" +
	"- [ ] 4. Gate  [tier: sonnet]\r\n      Acceptance: access-control.md, Behaviour\r\n" +
	"- [ ] 5. Setup  [tier: sonnet]\n      Acceptance: docs/stack.md, the database; database-access.md, Contracts\n" +
	"### Notes\n- [ ] 9. Not a task\n      Acceptance: cluster.md\n"

func TestStaleTasks(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		changed []string
		want    []string
	}{
		{"an indented acceptance line is matched; ticked tasks are not", []string{"docs/spec/cluster.md"}, []string{"- [ ] 2. Lease  [tier: opus]"}},
		{"a struck task is not stale", []string{"docs/stack.md"}, []string{"- [ ] 5. Setup  [tier: sonnet]"}},
		{"a shorter name never matches inside a longer one", []string{"docs/spec/control.md"}, nil},
		{"the longer name matches, across CRLF", []string{"docs/spec/access-control.md"}, []string{"- [ ] 4. Gate  [tier: sonnet]"}},
		{"several changed documents", []string{"docs/spec/cluster.md", "docs/spec/database-access.md"}, []string{"- [ ] 2. Lease  [tier: opus]", "- [ ] 5. Setup  [tier: sonnet]"}},
		{"a document no task cites", []string{"docs/spec/http.md"}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := staleTasks(stalePlan, tc.changed); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
