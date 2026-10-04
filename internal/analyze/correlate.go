package analyze

import (
	"context"
	"fmt"
	"github.com/MTG-Thomas/spuro/internal/config"
	gitbackend "github.com/MTG-Thomas/spuro/internal/git"
	"github.com/MTG-Thomas/spuro/internal/model"
	"sort"
	"strconv"
	"strings"
	"sync"
)

type correlation struct {
	works           []*repoWork
	objects         map[string][]model.Copy
	trees           map[string][]location
	patches         map[string][]location
	exactCache      map[string][]model.Copy
	equivalentCache map[string][]model.Copy
}
type location struct {
	w   *repoWork
	oid string
}

func key(w *repoWork, oid string) string { return w.Repo.ObjectFormat + ":" + oid }
func correlate(ctx context.Context, g *gitbackend.Runner, works []*repoWork, cfg config.Config) *correlation {
	x := &correlation{works: works, objects: map[string][]model.Copy{}, trees: map[string][]location{}, patches: map[string][]location{}, exactCache: map[string][]model.Copy{}, equivalentCache: map[string][]model.Copy{}}
	cache := map[string]string{}
	attempted := map[string]bool{}
	idsByRepo := make([][]string, len(works))
	patchesByRepo := make([]map[string]string, len(works))
	for i, w := range works {
		if cfg.PatchEnabled() {
			for _, oid := range sortedCommitKeys(w) {
				c := w.Commits[oid]
				if (len(c.Parents) <= 1 || stashShape(w, c)) && !attempted[key(w, oid)] {
					idsByRepo[i] = append(idsByRepo[i], oid)
					attempted[key(w, oid)] = true
				}
			}
		}
		for oid := range w.Objects {
			x.objects[key(w, oid)] = append(x.objects[key(w, oid)], model.Copy{RepoID: w.Repo.ID, Kind: "normal-ref-object"})
		}
	}
	jobs := make(chan int)
	var wg sync.WaitGroup
	workers := cfg.MaxWorkers
	if workers > len(works) {
		workers = len(works)
	}
	for k := 0; k < workers; k++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				w := works[i]
				ids := idsByRepo[i]
				if len(ids) == 0 {
					continue
				}
				z := g.PatchIDs(ctx, w.Repo.Path, strings.NewReader(strings.Join(ids, "\n")+"\n"), "log", "--no-walk=unsorted", "--root", "--diff-merges=first-parent", "--format=%H", "--patch", "--binary", "--no-ext-diff", "--no-textconv", "--stdin")
				if z.Code != 0 {
					w.diagnostic("patch-id", w.Repo.Path, gitbackend.Failure(z), true)
					continue
				}
				patchesByRepo[i] = map[string]string{}
				for _, line := range strings.Split(string(z.Stdout), "\n") {
					f := strings.Fields(line)
					if len(f) == 2 && validOID(f[1]) {
						patchesByRepo[i][key(w, f[1])] = f[0]
					}
				}
			}
		}()
	}
	for i := range works {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	for _, patches := range patchesByRepo {
		for oid, patch := range patches {
			cache[oid] = patch
		}
	}
	for _, w := range works {
		for _, oid := range sortedCommitKeys(w) {
			c := w.Commits[oid]
			c.PatchID = cache[key(w, oid)]
			if w.Normal[oid] {
				x.trees[key(w, c.TreeOID)] = append(x.trees[key(w, c.TreeOID)], location{w, oid})
				if c.PatchID != "" {
					x.patches[c.PatchID] = append(x.patches[c.PatchID], location{w, oid})
				}
			}
		}
	}
	for _, w := range works {
		for i := range w.Repo.Checkouts {
			c := &w.Repo.Checkouts[i]
			for j := range c.Files {
				v := &c.Files[j]
				v.DurableCopies = x.objectCopies(w, v.OID)
			}
			for j := range c.Index {
				v := &c.Index[j]
				v.DurableCopies = x.objectCopies(w, v.OID)
			}
		}
	}
	return x
}
func sortedCommitKeys(w *repoWork) []string {
	m := map[string]bool{}
	for oid := range w.Commits {
		m[oid] = true
	}
	return sortedKeys(m)
}
func (x *correlation) objectCopies(w *repoWork, oid string) []model.Copy {
	out := append([]model.Copy{}, x.objects[key(w, oid)]...)
	return out
}
func (x *correlation) refs(loc location, subject *repoWork, exclude string) []model.Copy {
	out := []model.Copy{}
	seen := map[model.Reachability]bool{}
	for _, r := range loc.w.Repo.Refs {
		if r.Class != model.DurableRef && r.Class != model.RemoteTracking {
			continue
		}
		if seen[r.Class] || r.Symbolic != "" {
			continue
		}
		if loc.w == subject && r.Name == exclude {
			continue
		}
		if loc.w.RefReach[r.Name][loc.oid] {
			out = append(out, model.Copy{RepoID: loc.w.Repo.ID, Ref: r.Name, OID: loc.oid, Kind: "commit-reachability"})
			seen[r.Class] = true
		}
	}
	return out
}
func (x *correlation) exact(w *repoWork, oid, exclude string) []model.Copy {
	cacheKey := copyCacheKey(w, oid, exclude)
	if copies, ok := x.exactCache[cacheKey]; ok {
		return copies
	}
	out := []model.Copy{}
	for _, other := range x.works {
		if other.Repo.ObjectFormat == w.Repo.ObjectFormat && other.Normal[oid] {
			out = append(out, x.refs(location{other, oid}, w, exclude)...)
		}
	}
	x.exactCache[cacheKey] = out
	return out
}
func (x *correlation) equivalent(w *repoWork, oid, exclude string, tree bool) []model.Copy {
	cacheKey := fmt.Sprint(tree) + copyCacheKey(w, oid, exclude)
	if copies, ok := x.equivalentCache[cacheKey]; ok {
		return copies
	}
	c := w.Commits[oid]
	out := []model.Copy{}
	if c == nil {
		return out
	}
	var locations []location
	if tree {
		locations = x.trees[key(w, c.TreeOID)]
	} else if c.PatchID != "" {
		locations = x.patches[c.PatchID]
	}
	for _, loc := range locations {
		if loc.oid == oid && loc.w.Repo.ObjectFormat == w.Repo.ObjectFormat {
			continue
		}
		copies := x.refs(loc, w, exclude)
		for i := range copies {
			if tree {
				copies[i].Kind = "identical-tree"
			} else {
				copies[i].Kind = "stable-patch-id"
			}
		}
		out = append(out, copies...)
	}
	x.equivalentCache[cacheKey] = out
	return out
}
func analyzeLineages(ctx context.Context, g *gitbackend.Runner, x *correlation, w *repoWork, cfg config.Config) {
	add := func(kind, ref, checkout, oid string) {
		l := lineage(ctx, g, x, w, cfg, kind, ref, checkout, oid)
		w.Repo.Lineages = append(w.Repo.Lineages, l)
	}
	used := map[string]bool{}
	unknownRoots := map[string]bool{}
	unknownRefs := map[string]string{}
	namespaces := map[string]int{}
	for _, r := range w.Repo.Refs {
		if r.Class == model.Unknown && validOID(r.CommitOID) {
			unknownRoots[r.CommitOID] = true
			if unknownRefs[r.CommitOID] == "" {
				unknownRefs[r.CommitOID] = r.Name
			}
			parts := strings.Split(r.Name, "/")
			namespace := r.Name
			if len(parts) > 2 {
				namespace = strings.Join(parts[:3], "/")
			}
			namespaces[namespace]++
		}
	}
	// Group weak-ref ancestry into tips instead of one finding per keep/snapshot
	// ref. Starting with parents preserves roots that have no descendant root.
	todo := []string{}
	for oid := range unknownRoots {
		if c := w.Commits[oid]; c != nil {
			todo = append(todo, c.Parents...)
		}
	}
	seenParents := map[string]bool{}
	for len(todo) > 0 {
		oid := todo[len(todo)-1]
		todo = todo[:len(todo)-1]
		if seenParents[oid] {
			continue
		}
		seenParents[oid] = true
		delete(unknownRoots, oid)
		if c := w.Commits[oid]; c != nil {
			todo = append(todo, c.Parents...)
		}
	}
	for _, oid := range sortedKeys(unknownRoots) {
		add("unknown_ref", unknownRefs[oid], "", oid)
		used[oid] = true
	}
	if len(namespaces) > 0 {
		evidence(w, "refs", "weak-namespace-tip-grouping", w.Repo.ID, map[string]any{"namespaces": namespaces, "tip_oids": sortedKeys(unknownRoots), "raw_refs_retained": true})
	}
	for i := range w.Repo.Branches {
		b := &w.Repo.Branches[i]
		add("branch", b.Name, "", b.OID)
		b.LineageID = w.Repo.Lineages[len(w.Repo.Lineages)-1].ID
	}
	for _, c := range w.Repo.Checkouts {
		if c.Detached && validOID(c.HeadOID) {
			add("detached", "", c.Path, c.HeadOID)
			used[c.HeadOID] = true
		}
	}
	for _, s := range w.Repo.Stashes {
		add("stash", s.Ref, "", s.OID)
		used[s.OID] = true
	}
	candidates := map[string]bool{}
	for oid, c := range w.Commits {
		if c.Reachability == model.ReflogOnly || c.Reachability == model.Unreachable {
			candidates[oid] = true
		}
	}
	// Retain tips; ancestry remains available in commit metadata and lineage membership.
	for _, oid := range sortedKeys(candidates) {
		for _, p := range w.Commits[oid].Parents {
			delete(candidates, p)
		}
	}
	for _, oid := range sortedKeys(candidates) {
		if used[oid] {
			continue
		}
		kind := "unreachable"
		if w.Commits[oid].Reachability == model.ReflogOnly {
			kind = "reflog"
		}
		add(kind, "", "", oid)
	}
}
func lineage(ctx context.Context, g *gitbackend.Runner, x *correlation, w *repoWork, cfg config.Config, kind, ref, checkout, oid string) model.Lineage {
	l := model.Lineage{ID: model.ID("strand-", w.Repo.ID, kind, ref, checkout, oid), Kind: kind, Ref: ref, Checkout: checkout, TipOID: oid, Primary: w.Repo.Primary, Commits: []string{}, EquivalentCopies: []model.Copy{}, RepresentedCommits: []string{}, StashFiles: []model.FileVersion{}, Complete: w.Repo.Complete, Equivalence: model.EquivalenceUnknown}
	c := w.Commits[oid]
	if c == nil {
		l.Complete = false
		return l
	}
	l.Reachability = c.Reachability
	exclude := ""
	if kind == "branch" {
		exclude = ref
	}
	exact := x.exact(w, oid, exclude)
	trees := x.equivalent(w, oid, exclude, true)
	if len(exact) > 0 {
		l.Equivalence = model.ExactlyPreserved
		l.EquivalentCopies = append([]model.Copy{}, exact...)
	} else if len(trees) > 0 {
		l.Equivalence = model.TreeEquivalent
		l.EquivalentCopies = append([]model.Copy{}, trees...)
	}
	var baseSet map[string]bool
	if w.Repo.PrimaryOID != "" && w.Repo.Primary != exclude {
		pair := oid + ":" + w.Repo.PrimaryOID
		if base, ok := w.MergeBases[pair]; ok {
			l.MergeBase = base
			if base != "" {
				baseSet = walk(w, base)
			}
		} else {
			z := g.Run(ctx, w.Repo.Path, "merge-base", oid, w.Repo.PrimaryOID)
			if z.Code == 0 {
				l.MergeBase = strings.TrimSpace(string(z.Stdout))
				baseSet = walk(w, l.MergeBase)
				w.MergeBases[pair] = l.MergeBase
			} else if z.Code == 1 {
				w.MergeBases[pair] = ""
			} else {
				w.diagnostic("merge-base", w.Repo.Path, gitbackend.Failure(z), true)
				l.Complete = false
			}
		}
	}
	represented := 0
	ids := []string{}
	if len(exact) == 0 {
		ids = sortedKeys(walk(w, oid))
	}
	for _, id := range ids {
		if baseSet[id] {
			continue
		}
		if len(x.exact(w, id, exclude)) > 0 {
			continue
		}
		l.Commits = append(l.Commits, id)
		l.SHAUnique++
		patches := x.equivalent(w, id, exclude, false)
		if len(patches) > 0 {
			represented++
			l.RepresentedCommits = append(l.RepresentedCommits, id)
			l.EquivalentCopies = append(l.EquivalentCopies, patches...)
		} else if w.Commits[id].PatchID == "" {
			l.PatchUnknown++
		} else {
			l.PatchUnique++
		}
	}
	if l.Equivalence == model.EquivalenceUnknown {
		switch {
		case l.SHAUnique > 0 && represented == l.SHAUnique:
			l.Equivalence = model.PatchEquivalent
		case represented > 0:
			l.Equivalence = model.PartiallySuperseded
		case l.SHAUnique > 0 && l.PatchUnknown == 0 && cfg.PatchEnabled():
			l.Equivalence = model.Unique
		}
	}
	if l.Equivalence != model.ExactlyPreserved && l.Equivalence != model.TreeEquivalent {
		base := l.MergeBase
		if base == "" && len(c.Parents) > 0 {
			base = c.Parents[0]
		}
		if base != "" {
			z := g.Run(ctx, w.Repo.Path, "diff", "--stat", "--no-ext-diff", "--no-textconv", base, oid, "--")
			if z.Code == 0 {
				l.DiffStat = string(z.Stdout)
			} else {
				w.diagnostic("diff-stat", w.Repo.Path, gitbackend.Failure(z), true)
				l.Complete = false
			}
		}
	}
	if kind == "stash" || kind == "unreachable" || kind == "reflog" {
		stashContents(ctx, g, x, w, &l, c)
	}
	l.Complete = l.Complete && w.Repo.Complete
	return l
}
func stashContents(ctx context.Context, g *gitbackend.Runner, x *correlation, w *repoWork, l *model.Lineage, c *model.Commit) {
	if len(c.Parents) < 2 || len(c.Parents) > 3 {
		return
	}
	index := w.Commits[c.Parents[1]]
	if index == nil || len(index.Parents) != 1 || index.Parents[0] != c.Parents[0] {
		return
	}
	if len(c.Parents) == 3 {
		u := w.Commits[c.Parents[2]]
		if u == nil || len(u.Parents) != 0 {
			return
		}
		l.UntrackedParent = c.Parents[2]
	}
	l.StashLike = true
	l.StashConfidence = model.Medium
	if strings.HasPrefix(c.Subject, "WIP on ") || strings.HasPrefix(c.Subject, "On ") {
		l.StashConfidence = model.High
	}
	for i, tip := range []string{c.OID, c.Parents[1]} {
		z := g.Run(ctx, w.Repo.Path, "diff", "--raw", "-z", "--no-abbrev", "--no-ext-diff", "--no-textconv", c.Parents[0], tip, "--")
		if z.Code != 0 {
			w.diagnostic("stash-diff", w.Repo.Path, gitbackend.Failure(z), true)
			l.Complete = false
			continue
		}
		tokens := strings.Split(string(z.Stdout), "\x00")
		for j := 0; j+1 < len(tokens); j += 2 {
			h := strings.Fields(tokens[j])
			if len(h) != 5 {
				continue
			}
			v := model.FileVersion{Path: tokens[j+1], OID: h[3], Kind: []string{"stash-working", "stash-index"}[i], Known: true, Stable: true, DurableCopies: []model.Copy{}}
			if allZero(v.OID) {
				v.OID = ""
				v.Kind = "stash-deletion"
			}
			v.DurableCopies = x.objectCopies(w, v.OID)
			l.StashFiles = append(l.StashFiles, v)
		}
	}
	if l.UntrackedParent != "" {
		z := g.Run(ctx, w.Repo.Path, "ls-tree", "-r", "-z", "--long", l.UntrackedParent)
		if z.Code != 0 {
			w.diagnostic("stash-untracked", w.Repo.Path, gitbackend.Failure(z), true)
			l.Complete = false
			return
		}
		for _, token := range strings.Split(string(z.Stdout), "\x00") {
			h, path, ok := strings.Cut(token, "\t")
			f := strings.Fields(h)
			if !ok || len(f) != 4 {
				continue
			}
			size, _ := strconv.ParseInt(f[3], 10, 64)
			v := model.FileVersion{Path: path, OID: f[2], Size: size, Kind: "stash-untracked", Known: true, Stable: true, DurableCopies: x.objectCopies(w, f[2])}
			l.StashFiles = append(l.StashFiles, v)
		}
	}
	// Snapshot or aggregate first-parent patch equivalence preserves tracked stash
	// work even when rebasing changed the surrounding file bytes. Third-parent
	// untracked content must be checked independently.
	for i := range l.StashFiles {
		v := &l.StashFiles[i]
		v.RepresentedBy = []model.Copy{}
		if v.Kind == "stash-untracked" {
			continue
		}
		tip := c.OID
		if v.Kind == "stash-index" {
			tip = c.Parents[1]
		}
		v.RepresentedBy = x.exact(w, tip, "")
		if len(v.RepresentedBy) == 0 {
			v.RepresentedBy = x.equivalent(w, tip, "", true)
		}
		if len(v.RepresentedBy) == 0 {
			v.RepresentedBy = x.equivalent(w, tip, "", false)
		}
	}
	// De-duplicate repeated index/working versions, preserving their source kind.
	seen := map[string]bool{}
	files := []model.FileVersion{}
	for _, v := range l.StashFiles {
		k := v.Path + "\x00" + v.OID + "\x00" + v.Kind
		if !seen[k] {
			seen[k] = true
			files = append(files, v)
		}
	}
	l.StashFiles = files
}
func classify(w *repoWork, scopeComplete bool) []model.Finding {
	out := []model.Finding{}
	add := func(sev model.Severity, kind, checkout, ref, oid, summary string, paths []string, ev string, confidence model.Confidence) {
		if paths == nil {
			paths = []string{}
		}
		out = append(out, model.Finding{ID: model.ID("SPURO-", w.Repo.ID, kind, checkout, ref, oid, strings.Join(paths, "\x00")), Severity: sev, Kind: kind, RepoID: w.Repo.ID, Checkout: checkout, Ref: ref, OID: oid, Summary: summary, Paths: paths, Evidence: []string{ev}, Confidence: confidence, SuggestedAction: "Review the evidence and preserve valuable work before any cleanup."})
	}
	for _, c := range w.Repo.Checkouts {
		ev := evidence(w, "status", "checkout-state", c.Path, map[string]any{"head": c.HeadOID, "branch": c.Branch, "status_known": c.StatusKnown, "modified": c.Modified, "staged": c.Staged, "untracked": c.Untracked, "conflicted": c.Conflicted, "file_versions": c.Files, "index_entries": c.Index, "registered": c.Registered, "exists": c.Exists, "valid": c.Valid})
		if !c.Exists || !c.Valid || (!c.Registered && !c.Main) {
			add(model.Review, "stale_worktree", c.Path, c.Branch, c.HeadOID, "Checkout registration, filesystem path or Git identity is inconsistent; recoverable state needs review.", nil, ev, model.High)
		}
		if len(c.Conflicted) > 0 {
			add(model.PreserveFirst, "conflicted_checkout", c.Path, c.Branch, c.HeadOID, fmt.Sprintf("%d unresolved paths retain working files and multiple index stages.", len(c.Conflicted)), c.Conflicted, ev, model.High)
		}
		untracked := []string{}
		unique := []string{}
		uncertain := false
		for _, v := range c.Files {
			if !v.Known || !v.Stable {
				uncertain = true
			}
			if v.Known && v.Stable && v.OID != "" && len(v.DurableCopies) == 0 {
				unique = append(unique, v.Path)
				if strings.HasPrefix(v.Kind, "untracked") {
					untracked = append(untracked, v.Path)
				}
			}
		}
		for _, entry := range c.Index {
			if validOID(entry.OID) && len(entry.DurableCopies) == 0 {
				unique = append(unique, entry.Path)
			}
		}
		sort.Strings(unique)
		unique = dedupStrings(unique)
		confidence := model.High
		if !w.Repo.Complete || !scopeComplete {
			confidence = model.Medium
		}
		if len(untracked) > 0 {
			add(model.PreserveFirst, "untracked_unique", c.Path, c.Branch, c.HeadOID, fmt.Sprintf("%d untracked file versions have no identical blob found among scanned normal refs.", len(untracked)), untracked, ev, confidence)
		}
		if len(c.Modified)+len(c.Staged) > 0 || uncertain {
			sev := model.Review
			if len(unique) > 0 {
				sev = model.PreserveFirst
			}
			add(sev, "dirty_checkout", c.Path, c.Branch, c.HeadOID, fmt.Sprintf("%d modified, %d staged paths; %d working/index paths with versions absent from scanned normal refs. Index and deletions also require review.", len(c.Modified), len(c.Staged), len(unique)), unique, ev, confidence)
		}
	}
	groups := map[string][]string{}
	for _, c := range w.Repo.Checkouts {
		if c.StatusKnown && len(c.Modified)+len(c.Staged)+len(c.Untracked)+len(c.Conflicted) == 0 && validOID(c.HeadOID) {
			groups[c.HeadOID] = append(groups[c.HeadOID], c.Path)
		}
	}
	for _, oid := range sortedGroupKeys(groups) {
		paths := groups[oid]
		if len(paths) < 2 {
			continue
		}
		sort.Strings(paths)
		ev := evidence(w, "worktree-correlation", "shared-clean-head", oid, map[string]any{"checkout_paths": paths, "ownership": "not established", "common_git_dir": w.Repo.CommonGitDir})
		add(model.Info, "duplicate_worktree", "", "", oid, fmt.Sprintf("%d clean checkout paths share this HEAD and common Git directory; ownership and ongoing use are unknown.", len(paths)), paths, ev, model.High)
	}
	for _, l := range w.Repo.Lineages {
		ev := evidence(w, "correlation", "lineage-preservation", l.ID, map[string]any{"lineage": l, "scope": "currently scanned local normal refs; excludes candidate branch itself", "squash_equivalence": "not established"})
		preserved := l.Equivalence == model.ExactlyPreserved || l.Equivalence == model.TreeEquivalent || l.Equivalence == model.PatchEquivalent
		confidence := model.Medium
		if l.Equivalence == model.ExactlyPreserved || l.Equivalence == model.TreeEquivalent {
			confidence = model.High
		}
		if l.StashLike {
			absent := []string{}
			for _, v := range l.StashFiles {
				if v.OID != "" && len(v.DurableCopies) == 0 && len(v.RepresentedBy) == 0 {
					absent = append(absent, v.Path)
				}
			}
			sort.Strings(absent)
			absent = dedupStrings(absent)
			kind := "stash"
			if l.Kind != "stash" {
				kind = "dropped_stash_candidate"
			}
			sev := model.Review
			if len(absent) > 0 {
				// Missing old tracked snapshots alone do not establish novel patches.
				// Untracked-parent bytes never present under normal refs are stronger.
				for _, v := range l.StashFiles {
					if v.Kind == "stash-untracked" && v.OID != "" && len(v.DurableCopies) == 0 {
						sev = model.PreserveFirst
					}
				}
			} else if len(l.StashFiles) > 0 && l.Complete {
				sev = model.Housekeeping
			}
			stashConfidence := l.StashConfidence
			if len(absent) > 0 && !scopeComplete {
				stashConfidence = model.Medium
			}
			add(sev, kind, l.Checkout, l.Ref, l.TipOID, fmt.Sprintf("Stash-shaped history contains %d file paths lacking exact blob or tracked snapshot/patch preservation in scanned normal refs; older tracked snapshots need contextual review.", len(absent)), absent, ev, stashConfidence)
			continue
		}
		kind := ""
		sev := model.Review
		switch l.Kind {
		case "branch":
			var b model.Branch
			for _, v := range w.Repo.Branches {
				if v.LineageID == l.ID {
					b = v
					break
				}
			}
			if b.Upstream == "" || b.UpstreamGone {
				missingev := evidence(w, "upstream", "branch-state", b.Name, map[string]any{"branch": b})
				s := model.Info
				if b.UpstreamGone {
					s = model.Review
				}
				if preserved {
					s = model.Housekeeping
				}
				add(s, "missing_upstream", "", b.Name, b.OID, "Branch has no existing configured upstream; preservation is assessed separately.", nil, missingev, model.High)
			}
			if !b.RemoteReachable && !preserved {
				kind = "unpushed_unique_branch"
			} else if preserved {
				kind = equivalenceKind(l.Equivalence)
				sev = model.Housekeeping
			} else {
				continue
			}
		case "detached":
			kind = "detached_unique_head"
			if !preserved {
				sev = model.PreserveFirst
			}
		case "reflog":
			kind = "reflog_only_history"
		case "unreachable":
			kind = "unreachable_history"
		case "stash":
			kind = "stash"
		case "unknown_ref":
			kind = "weak_ref_history"
		}
		if preserved {
			sev = model.Housekeeping
			if l.Kind != "branch" {
				kind = equivalenceKind(l.Equivalence)
			}
		}
		if l.Equivalence == model.PartiallySuperseded {
			kind = "partially_superseded"
		}
		if kind != "" {
			add(sev, kind, l.Checkout, l.Ref, l.TipOID, fmt.Sprintf("%s: %d commits unique by SHA, %d without patch equivalents, %d with unknown patch equivalence.", l.Equivalence, l.SHAUnique, l.PatchUnique, l.PatchUnknown), nil, ev, confidence)
		}
	}
	return out
}
func equivalenceKind(e model.Equivalence) string {
	switch e {
	case model.ExactlyPreserved:
		return "exactly_preserved"
	case model.TreeEquivalent:
		return "tree_equivalent"
	case model.PatchEquivalent:
		return "patch_equivalent"
	}
	return "plugin_observation"
}
func dedupStrings(in []string) []string {
	out := []string{}
	for _, s := range in {
		if len(out) == 0 || out[len(out)-1] != s {
			out = append(out, s)
		}
	}
	return out
}

func stashShape(w *repoWork, c *model.Commit) bool {
	if len(c.Parents) < 2 || len(c.Parents) > 3 {
		return false
	}
	p := w.Commits[c.Parents[1]]
	if p == nil || len(p.Parents) != 1 || p.Parents[0] != c.Parents[0] {
		return false
	}
	if len(c.Parents) == 3 {
		u := w.Commits[c.Parents[2]]
		return u != nil && len(u.Parents) == 0
	}
	return true
}

func sortedGroupKeys(m map[string][]string) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func copyCacheKey(w *repoWork, oid, exclude string) string {
	k := key(w, oid)
	if exclude != "" {
		k += "\x00" + w.Repo.ID + "\x00" + exclude
	}
	return k
}
