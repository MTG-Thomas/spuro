package report

import (
	"encoding/json"
	"fmt"
	"github.com/MTG-Thomas/spuro/internal/discovery"
	"github.com/MTG-Thomas/spuro/internal/model"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

// Normalize emits empty collections as [] rather than null throughout schema v1.
func Normalize(r *model.Result) { normalize(reflect.ValueOf(r).Elem()) }
func normalize(v reflect.Value) {
	switch v.Kind() {
	case reflect.Map:
		if v.IsNil() && v.CanSet() {
			v.Set(reflect.MakeMap(v.Type()))
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if v.Field(i).CanSet() {
				normalize(v.Field(i))
			}
		}
	case reflect.Slice:
		if v.IsNil() && v.CanSet() {
			v.Set(reflect.MakeSlice(v.Type(), 0, 0))
		}
		for i := 0; i < v.Len(); i++ {
			normalize(v.Index(i))
		}
	}
}
func JSON(w io.Writer, r *model.Result) error {
	Normalize(r)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(r)
}
func Text(w io.Writer, r *model.Result) error {
	checkouts, linked := 0, 0
	repos := map[string]string{}
	counts := map[model.Severity]int{}
	for _, repo := range r.Repositories {
		repos[repo.ID] = repo.Path
		checkouts += len(repo.Checkouts)
		for _, c := range repo.Checkouts {
			if !c.Main && c.Registered {
				linked++
			}
		}
	}
	for _, f := range r.Findings {
		counts[f.Severity]++
	}
	if _, e := fmt.Fprintf(w, "Spuro %s — read-only preservation scan\n%d repositories, %d checkout paths, %d linked worktrees\n", r.Scan.Version, len(r.Repositories), checkouts, linked); e != nil {
		return e
	}
	if !r.Scan.Complete {
		fmt.Fprintln(w, "Coverage is incomplete or state changed during scanning; see diagnostics before relying on absence claims.")
	}
	for _, sev := range []model.Severity{model.PreserveFirst, model.Review, model.Housekeeping, model.Info} {
		fmt.Fprintf(w, "\n%s  %d\n", sev, counts[sev])
		if sev == model.Housekeeping || sev == model.Info {
			continue
		}
		shown := 0
		for _, f := range r.Findings {
			if f.Severity != sev {
				continue
			}
			if sev == model.Review && shown >= 20 {
				continue
			}
			shown++
			fmt.Fprintf(w, "  %s  %s\n    %q\n", f.ID, f.Kind, repos[f.RepoID])
			if f.Checkout != "" {
				fmt.Fprintf(w, "    checkout: %q\n", f.Checkout)
			}
			if f.Ref != "" {
				fmt.Fprintf(w, "    ref: %q\n", f.Ref)
			}
			if f.OID != "" {
				fmt.Fprintf(w, "    commit: %s\n", f.OID)
			}
			fmt.Fprintf(w, "    %s\n", f.Summary)
			for _, path := range f.Paths {
				fmt.Fprintf(w, "    %q\n", path)
			}
		}
		if counts[sev] > shown && sev == model.Review {
			fmt.Fprintf(w, "  %d additional findings in JSON.\n", counts[sev]-shown)
		}
	}
	fmt.Fprintf(w, "\n%d diagnostics; elapsed %.2fs. JSON includes normalized evidence and equivalent history.\n", len(r.Diagnostics), r.Scan.Timings["total"])
	_, e := fmt.Fprintln(w, "Absence is scoped to scanned local refs. No squash-equivalence or semantic-obsolescence claim is made.")
	return e
}
func Within(path, root string) bool {
	rel, e := filepath.Rel(root, path)
	return e == nil && (rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
func resolvedTarget(path string) (string, error) {
	a, e := filepath.Abs(path)
	if e != nil {
		return "", e
	}
	if _, e = os.Lstat(a); e == nil {
		return discovery.Canonical(a)
	} else if !os.IsNotExist(e) {
		return "", e
	}
	parent, e := discovery.Canonical(filepath.Dir(a))
	if e != nil {
		return "", e
	}
	return filepath.Join(parent, filepath.Base(a)), nil
}

// WriteOutside never creates output inside scanned roots, checkouts or Git dirs.
// The parent directory must already exist. An atomic rename prevents partial reports.
func WriteOutside(path string, r *model.Result, write func(io.Writer) error) error {
	target, e := resolvedTarget(path)
	if e != nil {
		return e
	}
	protected := append([]string{}, r.Scan.Roots...)
	for _, repo := range r.Repositories {
		protected = append(protected, repo.CommonGitDir)
		for _, c := range repo.Checkouts {
			protected = append(protected, c.Path, c.GitDir)
		}
	}
	for _, root := range protected {
		if root == "" {
			continue
		}
		canonical, err := discovery.Canonical(root)
		if err != nil {
			if !os.IsNotExist(err) {
				return fmt.Errorf("resolve protected path %q: %w", root, err)
			}
			canonical, err = filepath.Abs(root)
			if err != nil {
				return err
			}
		}
		if Within(target, canonical) {
			return fmt.Errorf("refusing report output inside scanned source/repository: %s", target)
		}
	}
	// Also protect repositories outside the current scan, using filesystem markers.
	for dir := filepath.Dir(target); ; dir = filepath.Dir(dir) {
		if _, e := os.Lstat(filepath.Join(dir, ".git")); e == nil {
			return fmt.Errorf("refusing report output inside another checkout: %s", dir)
		}
		if _, e := os.Stat(filepath.Join(dir, "HEAD")); e == nil {
			if _, e = os.Stat(filepath.Join(dir, "objects")); e == nil {
				return fmt.Errorf("refusing report output inside a Git administrative directory: %s", dir)
			}
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	f, e := os.CreateTemp(filepath.Dir(target), ".spuro-report-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if e = write(f); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(f.Name(), target)
}
