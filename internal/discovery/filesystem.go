package discovery

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"spuro/internal/config"
	gitbackend "spuro/internal/git"
	"spuro/internal/model"
	"strings"
)

type Estate struct {
	Roots        []string
	Repositories []model.Repository
	Diagnostics  []model.Diagnostic
}

func Canonical(p string) (string, error) {
	a, e := filepath.Abs(p)
	if e != nil {
		return "", e
	}
	return filepath.EvalSymlinks(a)
}
func Identity(ctx context.Context, g *gitbackend.Runner, p string) (gitdir, common string, bare bool, err error) {
	a := g.Run(ctx, p, "rev-parse", "--absolute-git-dir")
	if a.Code != 0 {
		return "", "", false, fmtError(gitbackend.Failure(a))
	}
	b := g.Run(ctx, p, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if b.Code != 0 {
		return "", "", false, fmtError(gitbackend.Failure(b))
	}
	gitdir, err = Canonical(gitbackend.PathOutput(a.Stdout))
	if err != nil {
		return
	}
	common, err = Canonical(gitbackend.PathOutput(b.Stdout))
	if err != nil {
		return
	}
	z := g.Run(ctx, p, "rev-parse", "--is-bare-repository")
	if z.Code != 0 {
		err = fmtError(gitbackend.Failure(z))
		return
	}
	bare = strings.TrimSpace(string(z.Stdout)) == "true"
	return
}

type fmtError string

func (e fmtError) Error() string { return string(e) }
func Scan(ctx context.Context, g *gitbackend.Runner, roots, exclude []string) (Estate, error) {
	out := Estate{Roots: []string{}, Repositories: []model.Repository{}, Diagnostics: []model.Diagnostic{}}
	patterns := []*regexp.Regexp{}
	for _, s := range exclude {
		p, e := config.Glob(s)
		if e != nil {
			return out, e
		}
		patterns = append(patterns, p)
	}
	found := map[string]bool{}
	rootset := map[string]bool{}
	bareDirs := map[string]bool{}
	diag := func(p, m string) {
		out.Diagnostics = append(out.Diagnostics, model.Diagnostic{Provider: "filesystem", Operation: "discovery", Path: p, Message: m, Incomplete: true})
	}
	for _, root := range roots {
		canonical, e := Canonical(root)
		if e != nil {
			diag(root, e.Error())
			continue
		}
		if rootset[canonical] {
			continue
		}
		rootset[canonical] = true
		out.Roots = append(out.Roots, canonical)
		e = filepath.WalkDir(canonical, func(p string, d fs.DirEntry, walkErr error) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if walkErr != nil {
				diag(p, walkErr.Error())
				return nil
			}
			if !d.IsDir() {
				return nil
			}
			if p != canonical {
				if d.Name() == ".git" {
					return filepath.SkipDir
				}
				rel, _ := filepath.Rel(canonical, p)
				rel = filepath.ToSlash(rel) + "/"
				for _, pattern := range patterns {
					if pattern.MatchString(rel) {
						return filepath.SkipDir
					}
				}
				if bareDirs[filepath.Dir(p)] {
					switch d.Name() {
					case "objects", "refs", "logs", "worktrees", "hooks", "info":
						return filepath.SkipDir
					}
				}
			}
			if _, e := os.Lstat(filepath.Join(p, ".git")); e == nil {
				found[p] = true
			}
			if _, e := os.Stat(filepath.Join(p, "HEAD")); e == nil {
				if f, e := os.Stat(filepath.Join(p, "objects")); e == nil && f.IsDir() {
					if _, e := os.Stat(filepath.Join(p, "config")); e == nil {
						found[p] = true
						bareDirs[p] = true
					}
				}
			}
			// Never stop at a repo root: nested repositories and agent worktrees remain visible.
			return nil
		})
		if e != nil {
			return out, e
		}
	}
	sort.Strings(out.Roots)
	paths := []string{}
	for p := range found {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	repos := map[string]*model.Repository{}
	for _, p := range paths {
		gd, cd, bare, e := Identity(ctx, g, p)
		if e != nil {
			diag(p, "Git marker does not resolve: "+e.Error())
			continue
		}
		r := repos[cd]
		if r == nil {
			r = &model.Repository{ID: model.RepoID(cd), CommonGitDir: cd, Path: p, Bare: bare, Complete: true, Checkouts: []model.Checkout{}, Evidence: []model.Evidence{}, Diagnostics: []model.Diagnostic{}}
			repos[cd] = r
		}
		if !bare && p != gd {
			if r.Path == r.CommonGitDir || r.Path == gd {
				r.Path = p
			}
			r.Checkouts = append(r.Checkouts, model.Checkout{Path: p, RepoID: r.ID, GitDir: gd, CommonGitDir: cd, Exists: true, Valid: true})
		}
	}
	for _, r := range repos {
		out.Repositories = append(out.Repositories, *r)
	}
	sort.Slice(out.Repositories, func(i, j int) bool { return out.Repositories[i].CommonGitDir < out.Repositories[j].CommonGitDir })
	return out, nil
}
