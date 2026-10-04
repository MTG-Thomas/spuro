package analyze

import (
	"context"
	"github.com/MTG-Thomas/spuro/internal/config"
	"github.com/MTG-Thomas/spuro/internal/discovery"
	gitbackend "github.com/MTG-Thomas/spuro/internal/git"
	"github.com/MTG-Thomas/spuro/internal/model"
	"github.com/MTG-Thomas/spuro/internal/process"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type repoWork struct {
	Repo             model.Repository
	Commits          map[string]*model.Commit
	RefReach         map[string]map[string]bool
	Normal           map[string]bool
	Objects          map[string]bool
	Roots            map[model.Reachability][]string
	Candidates       []string
	InitialRefs      string
	InitialWorktrees string
	InitialStatus    map[string]string
	MergeBases       map[string]string
}

func newWork(r model.Repository) *repoWork {
	r.Checkouts = []model.Checkout{}
	r.Remotes = []model.Remote{}
	r.Refs = []model.Ref{}
	r.Branches = []model.Branch{}
	r.Stashes = []model.Stash{}
	r.Reflog = []model.ReflogEntry{}
	r.Commits = []model.Commit{}
	r.Lineages = []model.Lineage{}
	return &repoWork{Repo: r, Commits: map[string]*model.Commit{}, RefReach: map[string]map[string]bool{}, Normal: map[string]bool{}, Objects: map[string]bool{}, Roots: map[model.Reachability][]string{}, InitialStatus: map[string]string{}, MergeBases: map[string]string{}}
}
func (w *repoWork) diagnostic(op, path, message string, incomplete bool) {
	w.Repo.Diagnostics = append(w.Repo.Diagnostics, model.Diagnostic{Provider: "git", Operation: op, RepoID: w.Repo.ID, Path: path, Message: message, Incomplete: incomplete})
	if incomplete {
		w.Repo.Complete = false
	}
}
func (w *repoWork) run(ctx context.Context, g *gitbackend.Runner, a ...string) process.Result {
	z := g.Run(ctx, w.Repo.Path, a...)
	if z.Code != 0 {
		w.diagnostic(a[0], w.Repo.Path, gitbackend.Failure(z), true)
	}
	return z
}
func refClass(name string) model.Reachability {
	switch {
	case name == "refs/stash":
		return model.StashRoot
	case strings.HasPrefix(name, "refs/remotes/"):
		return model.RemoteTracking
	case strings.HasPrefix(name, "refs/heads/"), strings.HasPrefix(name, "refs/tags/"), strings.HasPrefix(name, "refs/rescue/"), strings.HasPrefix(name, "refs/archive/"), strings.HasPrefix(name, "refs/notes/"):
		return model.DurableRef
	}
	return model.Unknown
}

const refFormat = "--format=%(refname)%00%(objectname)%00%(objecttype)%00%(*objectname)%00%(*objecttype)%00%(upstream)%00%(symref)%00%(committerdate:unix)%00"

func observe(ctx context.Context, g *gitbackend.Runner, r model.Repository, cfg config.Config) *repoWork {
	w := newWork(r)
	z := w.run(ctx, g, "rev-parse", "--show-object-format")
	w.Repo.ObjectFormat = strings.TrimSpace(string(z.Stdout))
	if w.Repo.ObjectFormat != "sha1" && w.Repo.ObjectFormat != "sha256" {
		w.diagnostic("object-format", r.Path, "unknown object format", true)
	}
	z = w.run(ctx, g, "rev-parse", "--is-shallow-repository")
	w.Repo.Shallow = strings.TrimSpace(string(z.Stdout)) == "true"
	if w.Repo.Shallow {
		w.diagnostic("coverage", r.Path, "shallow history limits ancestry/equivalence conclusions", true)
	}
	z = g.Run(ctx, r.Path, "config", "--get-regexp", `^remote\..*\.(promisor|partialclonefilter)$`)
	if z.Code == 0 && len(z.Stdout) > 0 {
		w.Repo.Partial = true
		w.diagnostic("coverage", r.Path, "partial clone; lazy fetch disabled and missing content remains unknown", true)
	} else if z.Code != 1 && z.Code != 0 {
		w.diagnostic("config", r.Path, gitbackend.Failure(z), true)
	}
	refs := w.run(ctx, g, "for-each-ref", refFormat)
	w.InitialRefs = string(refs.Stdout)
	rows, e := gitbackend.FixedFields(refs.Stdout, 8)
	if e != nil {
		w.diagnostic("refs", r.Path, e.Error(), true)
	}
	for _, f := range rows {
		ref := model.Ref{Name: f[0], OID: f[1], ObjectType: f[2], Upstream: f[5], Symbolic: f[6], Timestamp: gitbackend.Unix(f[7]), Class: refClass(f[0])}
		if f[2] == "commit" {
			ref.CommitOID = f[1]
		} else if f[4] == "commit" {
			ref.CommitOID = f[3]
		} else if f[2] == "tag" {
			p := g.Run(ctx, r.Path, "rev-parse", "--verify", "--quiet", ref.Name+"^{commit}")
			if p.Code == 0 {
				ref.CommitOID = strings.TrimSpace(string(p.Stdout))
			}
		}
		w.Repo.Refs = append(w.Repo.Refs, ref)
		if ref.CommitOID != "" {
			w.Roots[ref.Class] = append(w.Roots[ref.Class], ref.CommitOID)
		}
		if strings.HasPrefix(ref.Name, "refs/replace/") {
			w.diagnostic("coverage", r.Path, "replacement refs present; preservation/equivalence may be incomplete", true)
		}
	}
	remoteNames := w.run(ctx, g, "remote")
	for _, name := range strings.Split(strings.TrimSuffix(string(remoteNames.Stdout), "\n"), "\n") {
		if name == "" {
			continue
		}
		u := g.Run(ctx, r.Path, "config", "--get", "remote."+name+".url")
		remote := model.Remote{Name: name}
		if u.Code == 0 {
			remote.URL = gitbackend.RedactURL(gitbackend.PathOutput(u.Stdout))
		}
		for _, ref := range w.Repo.Refs {
			if ref.Name == "refs/remotes/"+name+"/HEAD" {
				remote.HEAD = ref.Symbolic
			}
		}
		w.Repo.Remotes = append(w.Repo.Remotes, remote)
	}
	wt := w.run(ctx, g, "worktree", "list", "--porcelain", "-z")
	w.InitialWorktrees = string(wt.Stdout)
	checkouts, e := gitbackend.Worktrees(wt.Stdout)
	if e != nil {
		w.diagnostic("worktree-list", r.Path, e.Error(), true)
	}
	known := map[string]bool{}
	observed := []model.Checkout{}
	for _, c := range checkouts {
		if c.Reason == "bare" || c.Path == r.CommonGitDir {
			continue
		}
		p, e := filepath.Abs(c.Path)
		if e != nil {
			w.diagnostic("checkout", c.Path, e.Error(), true)
			continue
		}
		c.Path = filepath.Clean(p)
		if canonical, err := discovery.Canonical(c.Path); err == nil {
			c.Path = canonical
		}
		known[c.Path] = true
		observed = append(observed, c)
	}
	for _, c := range r.Checkouts {
		if !known[c.Path] {
			c.Registered = false
			c.Main = c.GitDir == r.CommonGitDir
			observed = append(observed, c)
			if !c.Main {
				w.diagnostic("worktree-registration", c.Path, "discovered linked checkout is not registered in native worktree list", true)
			}
		}
	}
	for _, c := range observed {
		c.RepoID = r.ID
		c.CommonGitDir = r.CommonGitDir
		c.Modified = []model.PathChange{}
		c.Staged = []model.PathChange{}
		c.Untracked = []string{}
		c.Conflicted = []string{}
		c.Files = []model.FileVersion{}
		c.Index = []model.IndexEntry{}
		fi, e := os.Stat(c.Path)
		c.Exists = e == nil && fi.IsDir()
		if !c.Exists {
			if c.HeadOID != "" {
				w.Roots[model.WorktreeHead] = append(w.Roots[model.WorktreeHead], c.HeadOID)
			}
			w.Repo.Checkouts = append(w.Repo.Checkouts, c)
			continue
		}
		gd, cd, _, e := discovery.Identity(ctx, g, c.Path)
		if e != nil || cd != r.CommonGitDir {
			msg := "checkout common Git directory disagrees with its registration"
			if e != nil {
				msg = e.Error()
			}
			w.diagnostic("checkout-identity", c.Path, msg, true)
			w.Repo.Checkouts = append(w.Repo.Checkouts, c)
			continue
		}
		c.GitDir = gd
		c.Valid = true
		status := g.Run(ctx, c.Path, "status", "--porcelain=v2", "-z", "--branch", "--untracked-files=all")
		w.InitialStatus[c.Path] = string(status.Stdout)
		if status.Code != 0 {
			w.diagnostic("status", c.Path, gitbackend.Failure(status), true)
		} else if e = gitbackend.Status(status.Stdout, &c); e != nil {
			w.diagnostic("status", c.Path, e.Error(), true)
		}
		if allZero(c.HeadOID) {
			c.HeadOID = ""
		}
		if c.HeadOID != "" && !allZero(c.HeadOID) {
			w.Roots[model.WorktreeHead] = append(w.Roots[model.WorktreeHead], c.HeadOID)
		}
		changed := map[string]bool{}
		for _, v := range c.Modified {
			changed[v.Path] = true
		}
		for _, v := range c.Staged {
			changed[v.Path] = true
		}
		for _, v := range c.Conflicted {
			changed[v] = true
		}
		for _, v := range c.Untracked {
			changed[v] = true
		}
		if len(changed) > 0 {
			idx := g.Run(ctx, c.Path, "ls-files", "--stage", "-z")
			if idx.Code != 0 {
				w.diagnostic("index", c.Path, gitbackend.Failure(idx), true)
			} else {
				entries, e := gitbackend.Index(idx.Stdout)
				if e != nil {
					w.diagnostic("index", c.Path, e.Error(), true)
				}
				for _, v := range entries {
					if changed[v.Path] {
						c.Index = append(c.Index, v)
					}
				}
			}
			paths := []string{}
			for path := range changed {
				paths = append(paths, path)
			}
			sort.Strings(paths)
			for _, path := range paths {
				kind := "working"
				for _, v := range c.Untracked {
					if v == path {
						kind = "untracked"
						break
					}
				}
				c.Files = append(c.Files, observeFile(ctx, g, w, c.Path, path, kind))
			}
		}
		w.Repo.Checkouts = append(w.Repo.Checkouts, c)
	}
	// Remove duplicate discoveries, retaining exactly one record per physical path.
	seen := map[string]bool{}
	cs := []model.Checkout{}
	for _, c := range w.Repo.Checkouts {
		if !seen[c.Path] {
			cs = append(cs, c)
			seen[c.Path] = true
		}
	}
	w.Repo.Checkouts = cs
	hasStash := false
	for _, ref := range w.Repo.Refs {
		if ref.Name == "refs/stash" {
			hasStash = true
		}
	}
	if hasStash {
		z = w.run(ctx, g, "reflog", "show", "refs/stash", "--format=%gd%x00%H%x00%ct%x00%s%x00")
		rows, e = gitbackend.FixedFields(z.Stdout, 4)
		if e != nil {
			w.diagnostic("stash-list", r.Path, e.Error(), true)
		}
		for _, f := range rows {
			w.Repo.Stashes = append(w.Repo.Stashes, model.Stash{Ref: f[0], OID: f[1], Timestamp: gitbackend.Unix(f[2]), Message: f[3]})
		}

	}
	z = w.run(ctx, g, "reflog", "show", "--all", "--format=%H%x00%gD%x00%ct%x00%gs%x00")
	rows, e = gitbackend.FixedFields(z.Stdout, 4)
	if e != nil {
		w.diagnostic("reflog", r.Path, e.Error(), true)
	}
	for _, f := range rows {
		w.Repo.Reflog = append(w.Repo.Reflog, model.ReflogEntry{OID: f[0], Ref: f[1], Timestamp: gitbackend.Unix(f[2]), Message: f[3]})
		w.Roots[model.ReflogOnly] = append(w.Roots[model.ReflogOnly], f[0])
	}
	if cfg.UnreachableEnabled() {
		// A no-reflogs fsck plus the native reflog ancestry distinguishes both
		// weak classes without repeating the expensive full object verification.
		for _, args := range [][]string{{"fsck", "--unreachable", "--no-reflogs", "--no-progress"}} {
			z = w.run(ctx, g, args...)
			ids := []string{}
			for _, l := range strings.Split(string(z.Stdout), "\n") {
				f := strings.Fields(l)
				if len(f) == 3 && f[0] == "unreachable" && f[1] == "commit" && validOID(f[2]) {
					ids = append(ids, f[2])
					w.Candidates = append(w.Candidates, f[2])
				}
			}
			w.Repo.Evidence = append(w.Repo.Evidence, model.Evidence{ID: model.ID("ev-", r.ID, strings.Join(args, " ")), Provider: "git", Operation: "fsck", Fact: "unreachable-commit-set", Data: map[string]any{"args": args, "oids": ids, "exit_code": z.Code}})
		}
	}
	roots := map[string]bool{}
	for _, set := range w.Roots {
		for _, oid := range set {
			if validOID(oid) {
				roots[oid] = true
			}
		}
	}
	for _, oid := range w.Candidates {
		roots[oid] = true
	}
	input := sortedKeys(roots)
	if len(input) > 0 {
		z = g.Input(ctx, r.Path, strings.NewReader(strings.Join(input, "\n")+"\n"), "log", "--no-decorate", "--format=%H%x00%T%x00%P%x00%ct%x00%s%x00", "--stdin")
		if z.Code != 0 {
			w.diagnostic("commit-metadata", r.Path, gitbackend.Failure(z), true)
		}
		rows, e = gitbackend.FixedFields(z.Stdout, 5)
		if e != nil {
			w.diagnostic("commit-metadata", r.Path, e.Error(), true)
		}
		for _, f := range rows {
			if !validOID(f[0]) {
				w.diagnostic("commit-metadata", r.Path, "invalid commit OID", true)
				continue
			}
			w.Commits[f[0]] = &model.Commit{OID: f[0], TreeOID: f[1], Parents: strings.Fields(f[2]), Timestamp: gitbackend.Unix(f[3]), Subject: f[4], Classes: []model.Reachability{}}
		}
	}
	for _, ref := range w.Repo.Refs {
		if ref.Class != model.DurableRef && ref.Class != model.RemoteTracking {
			continue
		}
		reach := walk(w, ref.CommitOID)
		w.RefReach[ref.Name] = reach
		if ref.Class == model.DurableRef || ref.Class == model.RemoteTracking {
			for oid := range reach {
				w.Normal[oid] = true
			}
		}
	}
	classes := []model.Reachability{model.DurableRef, model.RemoteTracking, model.WorktreeHead, model.StashRoot, model.ReflogOnly, model.Unknown}
	for _, class := range classes {
		seen := map[string]bool{}
		todo := append([]string{}, w.Roots[class]...)
		for len(todo) > 0 {
			oid := todo[len(todo)-1]
			todo = todo[:len(todo)-1]
			if seen[oid] {
				continue
			}
			seen[oid] = true
			c := w.Commits[oid]
			if c == nil {
				continue
			}
			c.Classes = append(c.Classes, class)
			todo = append(todo, c.Parents...)
		}
	}
	for _, c := range w.Commits {
		c.Reachability = model.Unreachable
		if len(c.Classes) > 0 {
			c.Reachability = c.Classes[0]
		}
	}
	normalRoots := []string{}
	for _, ref := range w.Repo.Refs {
		if ref.Class == model.DurableRef || ref.Class == model.RemoteTracking {
			normalRoots = append(normalRoots, ref.OID)
		}
	}
	if len(normalRoots) > 0 {
		z = g.Input(ctx, r.Path, strings.NewReader(strings.Join(normalRoots, "\n")+"\n"), "rev-list", "--objects", "--no-object-names", "--stdin")
		if z.Code != 0 {
			w.diagnostic("normal-object-index", r.Path, gitbackend.Failure(z), true)
		}
		for _, l := range strings.Fields(string(z.Stdout)) {
			if validOID(l) {
				w.Objects[l] = true
			}
		}
	}
	// Branch counts remain independent of inferred preservation/equivalence.
	refsByName := map[string]model.Ref{}
	for _, ref := range w.Repo.Refs {
		refsByName[ref.Name] = ref
	}
	for _, ref := range w.Repo.Refs {
		if !strings.HasPrefix(ref.Name, "refs/heads/") {
			continue
		}
		b := model.Branch{Name: ref.Name, OID: ref.CommitOID, Upstream: ref.Upstream}
		if b.Upstream != "" {
			up, ok := refsByName[b.Upstream]
			b.UpstreamGone = !ok
			if ok {
				b.UpstreamOID = up.CommitOID
				z = g.Run(ctx, r.Path, "rev-list", "--left-right", "--count", b.UpstreamOID+"..."+b.OID)
				f := strings.Fields(string(z.Stdout))
				if z.Code == 0 && len(f) == 2 {
					behind, e1 := strconv.Atoi(f[0])
					ahead, e2 := strconv.Atoi(f[1])
					if e1 == nil && e2 == nil {
						b.Behind = behind
						b.Ahead = ahead
						b.CountsKnown = true
					}
				}
				if !b.CountsKnown {
					w.diagnostic("upstream-counts", r.Path, gitbackend.Failure(z), true)
				}
			}
		}
		c := w.Commits[b.OID]
		b.RemoteReachable = c != nil && hasClass(c.Classes, model.RemoteTracking)
		w.Repo.Branches = append(w.Repo.Branches, b)
	}
	choosePrimary(w, cfg.Primary)
	return w
}
func observeFile(ctx context.Context, g *gitbackend.Runner, w *repoWork, checkout, path, kind string) model.FileVersion {
	v := model.FileVersion{Path: path, Kind: kind, DurableCopies: []model.Copy{}}
	full := filepath.Join(checkout, path)
	before, e := os.Lstat(full)
	if os.IsNotExist(e) {
		v.Kind = "deleted"
		v.Known = true
		v.Stable = true
		return v
	}
	if e != nil {
		w.diagnostic("file-version", full, e.Error(), true)
		return v
	}
	v.Size = before.Size()
	v.Modified = before.ModTime().UTC()
	var z process.Result
	if before.Mode()&os.ModeSymlink != 0 {
		target, e := os.Readlink(full)
		if e != nil {
			w.diagnostic("file-version", full, e.Error(), true)
			return v
		}
		z = g.Input(ctx, checkout, strings.NewReader(target), "hash-object", "--stdin", "--no-filters")
		v.Kind = kind + "_symlink"
	} else if before.Mode().IsRegular() {
		f, e := os.Open(full)
		if e != nil {
			w.diagnostic("file-version", full, e.Error(), true)
			return v
		}
		z = g.Input(ctx, checkout, f, "hash-object", "--stdin", "--no-filters")
		_ = f.Close()
	} else {
		w.diagnostic("file-version", full, "submodule/directory or nonregular path; contents not hashed", true)
		return v
	}
	if z.Code != 0 {
		w.diagnostic("file-version", full, gitbackend.Failure(z), true)
		return v
	}
	after, e := os.Lstat(full)
	v.OID = strings.TrimSpace(string(z.Stdout))
	v.Known = true
	v.Stable = e == nil && before.ModTime() == after.ModTime() && before.Size() == after.Size() && before.Mode() == after.Mode()
	if !v.Stable {
		w.diagnostic("file-version", full, "file changed while hashing", true)
	}
	return v
}
func choosePrimary(w *repoWork, fallback string) {
	refs := map[string]model.Ref{}
	for _, r := range w.Repo.Refs {
		refs[r.Name] = r
	}
	set := func(name string) bool {
		if r, ok := refs[name]; ok && r.CommitOID != "" {
			w.Repo.Primary = name
			w.Repo.PrimaryOID = r.CommitOID
			return true
		}
		return false
	}
	heads := []string{"refs/remotes/origin/HEAD", "refs/remotes/upstream/HEAD"}
	for _, r := range w.Repo.Refs {
		if strings.HasPrefix(r.Name, "refs/remotes/") && strings.HasSuffix(r.Name, "/HEAD") {
			heads = append(heads, r.Name)
		}
	}
	for _, name := range heads {
		if r, ok := refs[name]; ok && r.Symbolic != "" && set(r.Symbolic) {
			return
		}
	}
	// Prefer an upstream of a primary checkout, then an observed conventional
	// integration upstream, then other configured upstreams.
	for _, c := range w.Repo.Checkouts {
		if c.Main && c.Branch != "" {
			if r, ok := refs["refs/heads/"+c.Branch]; ok && set(r.Upstream) {
				return
			}
		}
	}
	for _, b := range w.Repo.Branches {
		if (strings.HasSuffix(b.Upstream, "/main") || strings.HasSuffix(b.Upstream, "/master")) && set(b.Upstream) {
			return
		}
	}
	for _, b := range w.Repo.Branches {
		if set(b.Upstream) {
			return
		}
	}
	for _, name := range []string{"refs/heads/main", "refs/heads/master", "refs/remotes/origin/main", "refs/remotes/origin/master"} {
		if set(name) {
			return
		}
	}
	if fallback != "" {
		if set(fallback) {
			return
		}
		if set("refs/heads/" + fallback) {
			return
		}
		_ = set("refs/remotes/" + fallback)
	}
}
func walk(w *repoWork, root string) map[string]bool {
	out := map[string]bool{}
	todo := []string{root}
	for len(todo) > 0 {
		oid := todo[len(todo)-1]
		todo = todo[:len(todo)-1]
		if out[oid] {
			continue
		}
		c := w.Commits[oid]
		if c == nil {
			continue
		}
		out[oid] = true
		todo = append(todo, c.Parents...)
	}
	return out
}
func hasClass(cs []model.Reachability, s model.Reachability) bool {
	for _, v := range cs {
		if v == s {
			return true
		}
	}
	return false
}
func sortedKeys(m map[string]bool) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
func validOID(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return !allZero(s)
}
func allZero(s string) bool { return strings.Trim(s, "0") == "" }
func evidence(w *repoWork, op, fact, subject string, data map[string]any) string {
	id := model.ID("ev-", w.Repo.ID, op, fact, subject)
	for _, e := range w.Repo.Evidence {
		if e.ID == id {
			return id
		}
	}
	w.Repo.Evidence = append(w.Repo.Evidence, model.Evidence{ID: id, Provider: "git", Operation: op, Fact: fact, Subject: subject, Data: data})
	return id
}
func verify(ctx context.Context, g *gitbackend.Runner, w *repoWork) {
	z := w.run(ctx, g, "for-each-ref", refFormat)
	wt := w.run(ctx, g, "worktree", "list", "--porcelain", "-z")
	changed := string(z.Stdout) != w.InitialRefs || string(wt.Stdout) != w.InitialWorktrees
	for _, c := range w.Repo.Checkouts {
		if c.Valid && c.Exists {
			z := g.Run(ctx, c.Path, "status", "--porcelain=v2", "-z", "--branch", "--untracked-files=all")
			if z.Code != 0 {
				w.diagnostic("final-status", c.Path, gitbackend.Failure(z), true)
			}
			if string(z.Stdout) != w.InitialStatus[c.Path] {
				changed = true
			}
			for _, v := range c.Files {
				if v.Known && v.Stable && v.OID != "" {
					fi, e := os.Lstat(filepath.Join(c.Path, v.Path))
					if e != nil || !fi.ModTime().UTC().Equal(v.Modified) || fi.Size() != v.Size {
						changed = true
					}
				}
			}
		}
	}
	if changed {
		w.Repo.ChangedDuringScan = true
		w.diagnostic("concurrent-change", w.Repo.Path, "refs, registrations or checkout status changed during scan; rerun before preservation decisions", true)
	}
}
