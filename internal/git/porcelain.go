package git

import (
	"bytes"
	"fmt"
	"spuro/internal/model"
	"strconv"
	"strings"
	"time"
)

func Worktrees(b []byte) ([]model.Checkout, error) {
	out := []model.Checkout{}
	cur := model.Checkout{}
	have := false
	flush := func() {
		if have {
			out = append(out, cur)
		}
		cur = model.Checkout{}
		have = false
	}
	for _, token := range bytes.Split(b, []byte{0}) {
		s := string(token)
		if s == "" {
			flush()
			continue
		}
		k, v, _ := strings.Cut(s, " ")
		switch k {
		case "worktree":
			if have {
				flush()
			}
			cur.Path = v
			cur.Registered = true
			have = true
		case "HEAD":
			cur.HeadOID = v
		case "branch":
			cur.Branch = strings.TrimPrefix(v, "refs/heads/")
		case "detached":
			cur.Detached = true
		case "bare": // This record describes a repository, not a checkout.
			cur.Reason = "bare"
		case "locked":
			cur.Locked = true
			cur.Reason = v
		case "prunable":
			cur.Prunable = true
			cur.Reason = v
		default:
			return nil, fmt.Errorf("unknown worktree field %q", k)
		}
	}
	flush()
	if len(out) > 0 {
		out[0].Main = true
	}
	return out, nil
}
func Status(b []byte, c *model.Checkout) error {
	c.Modified = []model.PathChange{}
	c.Staged = []model.PathChange{}
	c.Untracked = []string{}
	c.Conflicted = []string{}
	records := bytes.Split(b, []byte{0})
	for i := 0; i < len(records); i++ {
		s := string(records[i])
		if s == "" {
			continue
		}
		if strings.HasPrefix(s, "# ") {
			key, val, _ := strings.Cut(s[2:], " ")
			switch key {
			case "branch.oid":
				if val != "(initial)" {
					c.HeadOID = val
				}
			case "branch.head":
				c.Detached = val == "(detached)"
				if !c.Detached {
					c.Branch = val
				} else {
					c.Branch = ""
				}
			}
			continue
		}
		if strings.HasPrefix(s, "? ") {
			c.Untracked = append(c.Untracked, s[2:])
			continue
		}
		if strings.HasPrefix(s, "! ") {
			continue
		}
		switch s[0] {
		case '1', '2':
			fields := 9
			if s[0] == '2' {
				fields = 10
			}
			f := strings.SplitN(s, " ", fields)
			if len(f) != fields || len(f[1]) != 2 {
				return fmt.Errorf("malformed status record")
			}
			change := model.PathChange{Path: f[fields-1], Status: f[1], HeadOID: f[6], IndexOID: f[7], Submodule: f[2]}
			if s[0] == '2' {
				i++
				if i >= len(records) {
					return fmt.Errorf("rename lacks original path")
				}
				change.OriginalPath = string(records[i])
			}
			if f[1][0] != '.' {
				c.Staged = append(c.Staged, change)
			}
			if f[1][1] != '.' || f[2] != "N..." {
				c.Modified = append(c.Modified, change)
			}
		case 'u':
			f := strings.SplitN(s, " ", 11)
			if len(f) != 11 {
				return fmt.Errorf("malformed unmerged status")
			}
			c.Conflicted = append(c.Conflicted, f[10])
		default:
			return fmt.Errorf("unknown porcelain status prefix %q", s[0])
		}
	}
	c.StatusKnown = true
	return nil
}

// FixedFields consumes a NUL-delimited format with Git's optional newline
// separator between records. Field contents (including newlines) are preserved.
func FixedFields(b []byte, n int) ([][]string, error) {
	result := [][]string{}
	fields := bytes.Split(b, []byte{0})
	for len(fields) > 0 {
		fields[0] = bytes.TrimPrefix(fields[0], []byte{'\n'})
		if len(fields) == 1 && len(fields[0]) == 0 {
			break
		}
		if len(fields) < n+1 {
			return nil, fmt.Errorf("incomplete NUL format record")
		}
		row := make([]string, n)
		for i := range row {
			row[i] = string(fields[i])
		}
		result = append(result, row)
		fields = fields[n:]
	}
	return result, nil
}
func Unix(s string) time.Time {
	v, e := strconv.ParseInt(s, 10, 64)
	if e != nil || v == 0 {
		return time.Time{}
	}
	return time.Unix(v, 0).UTC()
}
func Index(b []byte) ([]model.IndexEntry, error) {
	out := []model.IndexEntry{}
	for _, rec := range bytes.Split(b, []byte{0}) {
		if len(rec) == 0 {
			continue
		}
		header, path, ok := strings.Cut(string(rec), "\t")
		f := strings.Fields(header)
		if !ok || len(f) != 3 {
			return nil, fmt.Errorf("invalid index record")
		}
		stage, e := strconv.Atoi(f[2])
		if e != nil {
			return nil, e
		}
		out = append(out, model.IndexEntry{Path: path, Mode: f[0], OID: f[1], Stage: stage, DurableCopies: []model.Copy{}})
	}
	return out, nil
}
