// Package git exposes only audited native Git read operations.
package git

import (
	"context"
	"fmt"
	"github.com/MTG-Thomas/spuro/internal/process"
	"io"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

type Runner struct {
	Binary  string
	Timeout time.Duration
	Limit   int
}

func New() (*Runner, error) {
	p, e := exec.LookPath("git")
	return &Runner{Binary: p, Timeout: 90 * time.Second, Limit: 128 * 1024 * 1024}, e
}
func Allowed(a []string) bool {
	if len(a) == 0 {
		return false
	}
	for _, s := range a {
		if s == "--lost-found" || s == "-w" || s == "--write" || s == "--batch-all-objects" || s == "--filters" || s == "--textconv" || s == "--output" || strings.HasPrefix(s, "--output=") {
			return false
		}
	}
	switch a[0] {
	case "version":
		return len(a) == 1
	case "rev-parse", "rev-list", "for-each-ref", "log", "merge-base", "ls-tree", "ls-files", "name-rev", "patch-id", "cherry":
		return true
	case "status":
		return true
	case "diff", "show":
		return contains(a, "--no-ext-diff") && contains(a, "--no-textconv")
	case "symbolic-ref":
		n := 0
		for _, s := range a[1:] {
			if !strings.HasPrefix(s, "-") {
				n++
			}
		}
		return n == 1 && !contains(a, "--delete") && !contains(a, "-d")
	case "worktree":
		return len(a) > 1 && a[1] == "list"
	case "stash":
		return len(a) > 1 && (a[1] == "list" || a[1] == "show" && contains(a, "--no-ext-diff") && contains(a, "--no-textconv"))
	case "reflog":
		return len(a) > 1 && a[1] == "show"
	case "fsck":
		return contains(a, "--unreachable")
	case "config":
		return len(a) == 3 && (a[1] == "--get" || a[1] == "--get-regexp")
	case "remote":
		return len(a) == 1
	case "hash-object":
		return contains(a, "--stdin") && contains(a, "--no-filters")
	case "cat-file":
		return contains(a, "--batch-check") || contains(a, "--batch") || contains(a, "-e")
	}
	return false
}
func contains(a []string, s string) bool {
	for _, v := range a {
		if v == s {
			return true
		}
	}
	return false
}
func (r *Runner) args(dir string, a []string) []string {
	prefix := []string{"--no-optional-locks", "-c", "core.fsmonitor=false", "-c", "core.untrackedCache=false", "-c", "gc.auto=0", "-c", "maintenance.auto=false", "-c", "submodule.recurse=false", "-c", "color.ui=false", "-c", "log.showSignature=false"}
	if dir != "" {
		prefix = append(prefix, "-C", dir)
	}
	return append(prefix, a...)
}
func (r *Runner) Run(ctx context.Context, dir string, a ...string) process.Result {
	return r.Input(ctx, dir, nil, a...)
}
func (r *Runner) Input(ctx context.Context, dir string, input io.Reader, a ...string) process.Result {
	if !Allowed(a) {
		return process.Result{Code: -1, Err: fmt.Errorf("read-only Git gate refused %q", a)}
	}
	return process.Run(ctx, r.Timeout, r.Limit, r.Binary, r.args(dir, a), input)
}
func (r *Runner) PatchIDs(ctx context.Context, dir string, input io.Reader, a ...string) process.Result {
	if !Allowed(a) {
		return process.Result{Code: -1, Err: fmt.Errorf("read-only Git gate refused patch source")}
	}
	return process.Pipe(ctx, r.Timeout, r.Limit, r.Binary, r.args(dir, a), r.args(dir, []string{"patch-id", "--stable"}), input)
}
func Failure(z process.Result) string {
	s := strings.TrimSpace(z.Stderr)
	if z.Err != nil {
		s = fmt.Sprintf("%v: %s", z.Err, s)
	}
	if s == "" {
		s = fmt.Sprintf("git exit %d", z.Code)
	}
	return Redact(s)
}
func Redact(s string) string {
	for _, word := range strings.Fields(s) {
		if strings.Contains(word, "://") {
			clean := RedactURL(word)
			s = strings.ReplaceAll(s, word, clean)
		}
	}
	return s
}
func RedactURL(raw string) string {
	u, e := url.Parse(raw)
	if e != nil {
		return "[unparseable remote URL]"
	}
	if u.User != nil {
		u.User = url.User("REDACTED")
	}
	if u.RawQuery != "" {
		u.RawQuery = "REDACTED"
	}
	if u.Fragment != "" {
		u.Fragment = "REDACTED"
	}
	return u.String()
}

// PathOutput removes only Git's terminating newline; embedded/trailing path
// whitespace remains significant.
func PathOutput(b []byte) string { return strings.TrimSuffix(string(b), "\n") }
