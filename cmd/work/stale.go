package main

import (
	"fmt"
	"path"
	"strconv"
	"strings"
)

// planTasks returns each unticked plan task as its box line plus the indented lines
// under it, where the task names the documents it cites.
func planTasks(body string) []string {
	var tasks []string
	in, current := false, -1
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimRight(line, "\r")
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "### ") {
			in, current = t == "### Plan", -1
			continue
		}
		if !in {
			continue
		}
		switch {
		case strings.HasPrefix(t, "- [ ]"):
			tasks = append(tasks, t)
			current = len(tasks) - 1
		case isBox(t):
			current = -1
		case current >= 0 && t != "" && (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")):
			tasks[current] += "\n" + t
		default:
			current = -1
		}
	}
	return tasks
}

// cites reports whether text names the document as a whole word: the name must not
// sit inside a longer one, so control.md never matches inside access-control.md.
func cites(text, doc string) bool {
	for from := 0; ; {
		i := strings.Index(text[from:], doc)
		if i < 0 {
			return false
		}
		start, end := from+i, from+i+len(doc)
		before := start == 0 || strings.ContainsRune(" /(`,\n\t", rune(text[start-1]))
		after := end == len(text) || !isWordByte(text[end])
		if before && after {
			return true
		}
		from = start + 1
	}
}

func isWordByte(b byte) bool {
	return b == '_' || b == '-' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

// staleTasks is the first line of every unticked task that cites a changed document.
func staleTasks(body string, changed []string) []string {
	var out []string
	for _, task := range planTasks(body) {
		for _, p := range changed {
			if cites(task, path.Base(p)) {
				out = append(out, strings.SplitN(task, "\n", 2)[0])
				break
			}
		}
	}
	return out
}

// stale lists, and with --apply blocks, every open plan that a document change has
// overtaken. It never removes blocked: re-planning and resuming are the developer's.
func stale(args []string) error {
	apply, ref := false, "a change"
	var changed []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--apply":
			apply = true
		case "--ref":
			if i+1 >= len(args) {
				usage()
			}
			i++
			ref = args[i]
		default:
			changed = append(changed, args[i])
		}
	}
	if len(changed) == 0 {
		usage()
	}
	if err := refresh(); err != nil {
		return err
	}
	issues, err := loadMirror()
	if err != nil {
		return err
	}
	marker := "stale-check: " + ref
	for _, is := range issues {
		tasks := staleTasks(is.Body, changed)
		if len(tasks) == 0 {
			continue
		}
		fmt.Printf("#%d %s: %d task(s) cite a changed document\n", is.Number, is.Title, len(tasks))
		for _, t := range tasks {
			fmt.Println("  " + t)
		}
		if !apply {
			continue
		}
		s := strconv.Itoa(is.Number)
		if !is.has("blocked") {
			if _, err := gh("", "issue", "edit", s, "--add-label", "blocked"); err != nil {
				return err
			}
		}
		if is.commented(marker) {
			continue
		}
		body := fmt.Sprintf("The plan may be stale: %s changed %s, which these unticked tasks cite.\n\n- %s\n\nRe-plan from the specs as they now stand before the loop resumes; the developer removes `blocked`.\n\n<!-- %s -->",
			ref, strings.Join(changed, ", "), strings.Join(tasks, "\n- "), marker)
		if _, err := gh("", "issue", "comment", s, "--body", body); err != nil {
			return err
		}
	}
	return nil
}

func (is issue) commented(marker string) bool {
	for _, c := range is.Comments {
		if strings.Contains(c.Body, marker) {
			return true
		}
	}
	return false
}
