package analyze

import (
	"context"
	"fmt"
	"github.com/MTG-Thomas/spuro/internal/config"
	"github.com/MTG-Thomas/spuro/internal/discovery"
	gitbackend "github.com/MTG-Thomas/spuro/internal/git"
	"github.com/MTG-Thomas/spuro/internal/model"
	"github.com/MTG-Thomas/spuro/internal/plugin"
	"sort"
	"strings"
	"sync"
	"time"
)

type Options struct {
	Roots     []string
	Config    config.Config
	Plugins   []string
	NoPlugins bool
	Progress  func(string)
}

func Scan(ctx context.Context, opts Options) (model.Result, error) {
	start := time.Now().UTC()
	r := model.Result{SchemaVersion: 1, Scan: model.Scan{Version: model.Version, Started: start, Complete: true, MaxWorkers: opts.Config.MaxWorkers, Timings: map[string]float64{}, Excludes: opts.Config.Exclude, Limitations: []string{"Only currently scanned local refs are compared; no fetch or live remote verification.", "Tree/blob equality preserves snapshots/content, not integration context or semantic intent.", "Stable patch IDs ignore whitespace; squash and semantic equivalence are not established.", "Ignored files, excluded directories, external unregistered checkouts and process ownership are not comprehensively inspected.", "Read-only plugin contracts are verified declarations, not a sandbox.", "Copy records are representative ref witnesses, not exhaustive ref-membership lists."}}, Repositories: []model.Repository{}, Findings: []model.Finding{}, Plugins: []model.PluginRun{}, Diagnostics: []model.Diagnostic{}}
	if len(opts.Roots) == 0 {
		return r, fmt.Errorf("at least one source root is required")
	}
	workers := opts.Config.MaxWorkers
	if workers < 1 {
		return r, fmt.Errorf("max-workers must be positive")
	}
	g, e := gitbackend.New()
	if e != nil {
		return r, e
	}
	v := g.Run(ctx, "", "version")
	if v.Code != 0 {
		return r, fmt.Errorf("git unavailable: %s", gitbackend.Failure(v))
	}
	r.Scan.GitVersion = strings.TrimSpace(string(v.Stdout))
	phase := func(name string, t time.Time) {
		r.Scan.Timings[name] = time.Since(t).Seconds()
		if opts.Progress != nil {
			opts.Progress(fmt.Sprintf("%s: %.3fs", name, r.Scan.Timings[name]))
		}
	}
	t := time.Now()
	estate, e := discovery.Scan(ctx, g, opts.Roots, opts.Config.Exclude)
	if e != nil {
		return r, e
	}
	r.Scan.Roots = estate.Roots
	r.Diagnostics = append(r.Diagnostics, estate.Diagnostics...)
	phase("discovery", t)
	works := make([]*repoWork, len(estate.Repositories))
	jobs := make(chan int)
	var wg sync.WaitGroup
	if workers > len(works) {
		workers = len(works)
	}
	t = time.Now()
	for k := 0; k < workers; k++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				works[i] = observe(ctx, g, estate.Repositories[i], opts.Config)
			}
		}()
	}
	for i := range works {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	phase("observation", t)
	t = time.Now()
	x := correlate(ctx, g, works, opts.Config)
	phase("correlation", t)
	t = time.Now()
	scopeComplete := true
	for _, d := range estate.Diagnostics {
		if d.Incomplete {
			scopeComplete = false
		}
	}
	for _, w := range works {
		if !w.Repo.Complete {
			scopeComplete = false
		}
	}
	for _, w := range works {
		analyzeLineages(ctx, g, x, w, opts.Config)
		verify(ctx, g, w)
		r.Findings = append(r.Findings, classify(w, scopeComplete)...)
		for _, id := range sortedCommitKeys(w) {
			w.Repo.Commits = append(w.Repo.Commits, *w.Commits[id])
		}
		r.Repositories = append(r.Repositories, w.Repo)
		r.Diagnostics = append(r.Diagnostics, w.Repo.Diagnostics...)
		if !w.Repo.Complete {
			r.Scan.Complete = false
		}
	}
	for _, d := range estate.Diagnostics {
		if d.Incomplete {
			r.Scan.Complete = false
		}
		if strings.HasPrefix(d.Message, "Git marker does not resolve:") {
			r.Findings = append(r.Findings, model.Finding{ID: model.ID("SPURO-", "invalid-checkout", d.Path), Severity: model.Review, Kind: "stale_worktree", Checkout: d.Path, Paths: []string{}, Evidence: []string{}, Summary: "Git marker could not be resolved: " + d.Message, Confidence: model.High, SuggestedAction: "Inspect the broken Git indirection and preserve files before cleanup."})
		}
	}
	phase("classification", t)
	t = time.Now()
	if !opts.NoPlugins {
		plugin.Run(ctx, opts.Config, opts.Plugins, &r)
	}
	phase("plugins", t)
	Sort(&r)
	r.Scan.Finished = time.Now().UTC()
	r.Scan.Timings["total"] = time.Since(start).Seconds()
	return r, nil
}
func Sort(r *model.Result) {
	rank := map[model.Severity]int{model.PreserveFirst: 0, model.Review: 1, model.Housekeeping: 2, model.Info: 3}
	sort.Slice(r.Findings, func(i, j int) bool {
		a, b := r.Findings[i], r.Findings[j]
		if rank[a.Severity] != rank[b.Severity] {
			return rank[a.Severity] < rank[b.Severity]
		}
		return a.ID < b.ID
	})
	sort.Slice(r.Repositories, func(i, j int) bool { return r.Repositories[i].CommonGitDir < r.Repositories[j].CommonGitDir })
	for i := range r.Repositories {
		repo := &r.Repositories[i]
		sort.Slice(repo.Checkouts, func(i, j int) bool { return repo.Checkouts[i].Path < repo.Checkouts[j].Path })
		sort.Slice(repo.Evidence, func(i, j int) bool { return repo.Evidence[i].ID < repo.Evidence[j].ID })
		sort.Slice(repo.Lineages, func(i, j int) bool { return repo.Lineages[i].ID < repo.Lineages[j].ID })
	}
}
