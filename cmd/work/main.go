// Command work is the estate's token-cheap view of GitHub Issues: one API call
// refreshes a local mirror, selection reads the mirror, and the few writes go
// through gh. Run it inside a repository checkout; gh infers the repository.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

const mirrorFile = "issues.jsonl"

type issue struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Labels    []struct {
		Name string `json:"name"`
	} `json:"labels"`
	BlockedBy blockedBy `json:"blockedBy"`
	Comments  []struct {
		Body      string    `json:"body"`
		CreatedAt time.Time `json:"createdAt"`
	} `json:"comments"`
}

func usage() {
	fmt.Fprint(os.Stderr, `usage:
  work refresh                       mirror this repository's open issues into issues.jsonl
  work next [--loop]                 print the one issue to work on as JSON (exit 3 if none)
  work claim <n>                     label the issue claimed and say by whom
  work tick <n> <task> <comment>     tick plan box <task> and post the comment
  work close <n> <pr> [writeup-file] close when every box is ticked and the PR is merged
  work lint                          check every open issue's shape (exit 1 on any fault)
  work unclaim-stale <days>          release claims with no activity for <days>
`)
	os.Exit(2)
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "refresh":
		err = refresh()
	case "next":
		err = next(len(os.Args) > 2 && os.Args[2] == "--loop")
	case "claim":
		err = claim(arg(2))
	case "tick":
		if len(os.Args) < 5 {
			usage()
		}
		err = tick(arg(2), arg(3), os.Args[4])
	case "close":
		if len(os.Args) < 4 {
			usage()
		}
		writeup := ""
		if len(os.Args) > 4 {
			writeup = os.Args[4]
		}
		err = closeIssue(arg(2), arg(3), writeup)
	case "lint":
		err = lint()
	case "unclaim-stale":
		err = unclaimStale(arg(2))
	default:
		usage()
	}
	if err != nil {
		var ec exitCode
		if errors.As(err, &ec) {
			os.Exit(int(ec))
		}
		fmt.Fprintln(os.Stderr, "work:", err)
		os.Exit(1)
	}
}

type exitCode int

func (e exitCode) Error() string { return "exit " + strconv.Itoa(int(e)) }

func arg(i int) int {
	if len(os.Args) <= i {
		usage()
	}
	n, err := strconv.Atoi(os.Args[i])
	if err != nil {
		usage()
	}
	return n
}

func gh(stdin string, args ...string) ([]byte, error) {
	cmd := exec.Command("gh", args...)
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gh %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

func refresh() error {
	out, err := gh("", "issue", "list", "--state", "open", "--limit", "1000", "--json",
		"number,title,body,url,createdAt,updatedAt,labels,blockedBy,comments")
	if err != nil {
		return err
	}
	var issues []issue
	if err := json.Unmarshal(out, &issues); err != nil {
		return err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, is := range issues {
		if err := enc.Encode(is); err != nil {
			return err
		}
	}
	if err := os.WriteFile(mirrorFile, buf.Bytes(), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "work: %d open issues mirrored\n", len(issues))
	return nil
}

func loadMirror() ([]issue, error) {
	data, err := os.ReadFile(mirrorFile)
	if err != nil {
		return nil, fmt.Errorf("%w (run `work refresh`)", err)
	}
	var issues []issue
	for _, line := range bytes.Split(data, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var is issue
		if err := json.Unmarshal(line, &is); err != nil {
			return nil, err
		}
		issues = append(issues, is)
	}
	return issues, nil
}

func isBox(line string) bool {
	return strings.HasPrefix(line, "- [ ]") || strings.HasPrefix(line, "- [x]")
}

func identity() string {
	if id := os.Getenv("WORK_IDENTITY"); id != "" {
		return id
	}
	out, _ := exec.Command("git", "config", "--get", "user.name").Output()
	return strings.TrimSpace(string(out))
}

func (is issue) has(label string) bool {
	for _, l := range is.Labels {
		if l.Name == label {
			return true
		}
	}
	return false
}

func (is issue) priority() int {
	for p := 0; p <= 3; p++ {
		if is.has("P" + strconv.Itoa(p)) {
			return p
		}
	}
	return 4
}

func (is issue) kind() string {
	for _, l := range is.Labels {
		if k, ok := strings.CutPrefix(l.Name, "kind:"); ok {
			return k
		}
	}
	return ""
}

// gh returns the dependency list as a connection: the blockers under nodes with a
// total count beside them, not a bare array.
type blockedBy struct {
	Nodes []blocker `json:"nodes"`
}

type blocker struct {
	Number int    `json:"number"`
	State  string `json:"state"`
}

func (is issue) blockedByOpen() bool {
	for _, b := range is.BlockedBy.Nodes {
		if strings.EqualFold(b.State, "open") {
			return true
		}
	}
	return false
}

// The claim is the earliest "Claimed by" comment since the last release: two loops
// that claim in the same second both comment, and the server's order of the comments
// decides, so the loser's comment is noise and never a claim.
func (is issue) claimedBy() string {
	who := ""
	for _, c := range is.Comments {
		if strings.HasPrefix(c.Body, "Claim released") {
			who = ""
			continue
		}
		if who != "" {
			continue
		}
		if rest, ok := strings.CutPrefix(c.Body, "Claimed by "); ok {
			who = strings.TrimSpace(strings.SplitN(rest, "\n", 2)[0])
		}
	}
	return who
}

func (is issue) plan() []string {
	var lines []string
	in := false
	for _, line := range strings.Split(is.Body, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "### ") {
			in = t == "### Plan"
			continue
		}
		if in && isBox(t) && len(t) > 5 {
			lines = append(lines, t)
		}
	}
	return lines
}

// selectNext applies the mechanical rule: the caller's own claimed item, else the
// first open item by priority then age, skipping blocked, human-action and other
// people's claims; in loop mode only items labelled for the loop.
func selectNext(issues []issue, me string, loop bool) (issue, bool) {
	for _, is := range issues {
		if is.has("claimed") && is.claimedBy() == me {
			return is, true
		}
	}
	var open []issue
	for _, is := range issues {
		if is.has("claimed") || is.has("blocked") || is.has("human-action-required") || is.blockedByOpen() {
			continue
		}
		if loop && !is.has("unattended-loop") {
			continue
		}
		open = append(open, is)
	}
	sort.SliceStable(open, func(i, j int) bool {
		if open[i].priority() != open[j].priority() {
			return open[i].priority() < open[j].priority()
		}
		return open[i].CreatedAt.Before(open[j].CreatedAt)
	})
	if len(open) == 0 {
		return issue{}, false
	}
	return open[0], true
}

func next(loop bool) error {
	issues, err := loadMirror()
	if err != nil {
		return err
	}
	is, ok := selectNext(issues, identity(), loop)
	if !ok {
		fmt.Fprintln(os.Stderr, "work: nothing to do")
		return exitCode(3)
	}
	plan := is.plan()
	planned := false
	for _, l := range plan {
		if strings.HasPrefix(l, "- [ ]") {
			planned = true
		}
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{
		"number":             is.Number,
		"title":              is.Title,
		"url":                is.URL,
		"priority":           is.priority(),
		"kind":               is.kind(),
		"planned":            planned,
		"security_sensitive": is.has("security-sensitive"),
		"claimed_by":         is.claimedBy(),
	})
}

// claim comments first and labels only after re-reading the issue and finding its own
// comment to be the claim; a loop that lost the race gets an error and picks again.
func claim(n int) error {
	s := strconv.Itoa(n)
	me := identity()
	if _, err := gh("", "issue", "comment", s, "--body", "Claimed by "+me); err != nil {
		return err
	}
	out, err := gh("", "issue", "view", s, "--json", "comments")
	if err != nil {
		return err
	}
	var is issue
	if err := json.Unmarshal(out, &is); err != nil {
		return err
	}
	if who := is.claimedBy(); who != me {
		return fmt.Errorf("#%d was claimed by %s first", n, who)
	}
	_, err = gh("", "issue", "edit", s, "--add-label", "claimed")
	return err
}

// tickBody marks the task-th unticked plan box (1-based) as done.
func tickBody(body string, task int) (string, error) {
	lines := strings.Split(body, "\n")
	in, seen := false, 0
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "### ") {
			in = t == "### Plan"
			continue
		}
		if !in || !isBox(t) {
			continue
		}
		seen++
		if seen != task {
			continue
		}
		if strings.HasPrefix(t, "- [x]") {
			return "", fmt.Errorf("task %d is already ticked", task)
		}
		lines[i] = strings.Replace(line, "- [ ]", "- [x]", 1)
		return strings.Join(lines, "\n"), nil
	}
	return "", fmt.Errorf("task %d not found under ### Plan", task)
}

func liveBody(n int) (string, error) {
	out, err := gh("", "issue", "view", strconv.Itoa(n), "--json", "body", "--jq", ".body")
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(out), "\n"), nil
}

func tick(n, task int, comment string) error {
	body, err := liveBody(n)
	if err != nil {
		return err
	}
	body, err = tickBody(body, task)
	if err != nil {
		return err
	}
	s := strconv.Itoa(n)
	if _, err := gh(body, "issue", "edit", s, "--body-file", "-"); err != nil {
		return err
	}
	_, err = gh("", "issue", "comment", s, "--body", fmt.Sprintf("Task %d: %s", task, comment))
	return err
}

func closeIssue(n, pr int, writeupFile string) error {
	body, err := liveBody(n)
	if err != nil {
		return err
	}
	if strings.Contains(body, "- [ ]") {
		return errors.New("a plan box is still open; tick it or strike it with a reason")
	}
	out, err := gh("", "pr", "view", strconv.Itoa(pr), "--json", "state", "--jq", ".state")
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(out)) != "MERGED" {
		return fmt.Errorf("PR #%d is not merged", pr)
	}
	args := []string{"issue", "close", strconv.Itoa(n)}
	if writeupFile != "" {
		w, err := os.ReadFile(writeupFile)
		if err != nil {
			return err
		}
		args = append(args, "--comment", string(w))
	}
	_, err = gh("", args...)
	return err
}

// lintIssue returns every way an issue departs from the work-order shape.
func lintIssue(is issue) []string {
	var faults []string
	p, k := 0, 0
	for _, l := range is.Labels {
		switch {
		case len(l.Name) == 2 && l.Name[0] == 'P':
			p++
		case strings.HasPrefix(l.Name, "kind:"):
			k++
		}
	}
	if p != 1 {
		faults = append(faults, "needs exactly one priority label")
	}
	if k != 1 {
		faults = append(faults, "needs exactly one kind label")
	}
	for _, h := range []string{"### Files", "### Exact change", "### Reason", "### Plan"} {
		if !strings.Contains(is.Body, h) {
			faults = append(faults, "missing section "+h)
		}
	}
	if is.has("claimed") && len(is.plan()) == 0 {
		faults = append(faults, "claimed with no plan checklist")
	}
	if is.has("claimed") && is.claimedBy() == "" {
		faults = append(faults, "claimed without a 'Claimed by' comment")
	}
	return faults
}

func lint() error {
	if err := refresh(); err != nil {
		return err
	}
	issues, err := loadMirror()
	if err != nil {
		return err
	}
	bad := 0
	for _, is := range issues {
		for _, f := range lintIssue(is) {
			fmt.Printf("#%d: %s\n", is.Number, f)
			bad++
		}
	}
	if bad > 0 {
		return exitCode(1)
	}
	fmt.Fprintf(os.Stderr, "work: %d issues, no faults\n", len(issues))
	return nil
}

func unclaimStale(days int) error {
	if err := refresh(); err != nil {
		return err
	}
	issues, err := loadMirror()
	if err != nil {
		return err
	}
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	for _, is := range issues {
		if !is.has("claimed") || is.UpdatedAt.After(cutoff) {
			continue
		}
		s := strconv.Itoa(is.Number)
		if _, err := gh("", "issue", "edit", s, "--remove-label", "claimed"); err != nil {
			return err
		}
		if _, err := gh("", "issue", "comment", s, "--body",
			fmt.Sprintf("Claim released: no activity for %d days.", days)); err != nil {
			return err
		}
		fmt.Printf("#%d: claim released\n", is.Number)
	}
	return nil
}
