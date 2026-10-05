package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/MTG-Thomas/spuro/internal/analyze"
	"github.com/MTG-Thomas/spuro/internal/config"
	gitbackend "github.com/MTG-Thomas/spuro/internal/git"
	"github.com/MTG-Thomas/spuro/internal/model"
	"github.com/MTG-Thomas/spuro/internal/report"
	"github.com/MTG-Thomas/spuro/internal/sessions"
)

func runSessions(ctx context.Context, args []string, out, errout io.Writer) error {
	if len(args) > 0 && args[0] == "reconcile" {
		return runSessionReconcile(ctx, args[1:], out)
	}
	roots, paths := []string{}, []string{}
	provider, version, output, sourceHost := "normalized-json", "", "", ""
	host, err := os.Hostname()
	if err != nil {
		return err
	}
	asJSON, noText, includeCompleted, verbose, includeNative := false, false, false, false, false
	cfg := config.Defaults()
	configPath := ""
	workers := 0
	for n := 0; n < len(args); n++ {
		name, value, has := strings.Cut(args[n], "=")
		next := func() (string, error) {
			if has {
				return value, nil
			}
			n++
			if n >= len(args) {
				return "", fmt.Errorf("missing value for %s", name)
			}
			return args[n], nil
		}
		switch name {
		case "--help", "-h":
			_, err := fmt.Fprintln(out, "Usage: spuro sessions [root...] [--source FILE] [--provider normalized-json|deja-vu] [--deja-export-version 0.21.5|0.21.6] [--source-host HOST] [--host-id HOST] [--json] [--output FILE] [--no-transcript-text] [--include-completed] [--include-native] [--config FILE] [--max-workers N] [--verbose]\nOnly existing explicitly supplied artifacts are read; provider commands never run.\nUse spuro sessions reconcile --input FILE to process a saved snapshot without scanning.")
			return err
		case "--source", "--provider", "--deja-export-version", "--source-host", "--host-id", "--output", "--max-workers", "--config":
			v, err := next()
			if err != nil {
				return err
			}
			switch name {
			case "--source":
				paths = append(paths, v)
			case "--provider":
				provider = v
			case "--deja-export-version":
				version = v
			case "--source-host":
				sourceHost = v
			case "--host-id":
				host = v
			case "--output":
				output = v
			case "--config":
				configPath = v
			case "--max-workers":
				workers, err = strconv.Atoi(v)
				if err != nil || workers < 1 {
					return fmt.Errorf("invalid max-workers")
				}
			}
		case "--json", "--no-transcript-text", "--include-completed", "--verbose", "--include-native":
			if has {
				return fmt.Errorf("%s takes no value", name)
			}
			switch name {
			case "--json":
				asJSON = true
			case "--no-transcript-text":
				noText = true
			case "--include-completed":
				includeCompleted = true
			case "--verbose":
				verbose = true
			case "--include-native":
				includeNative = true
			}
		case "--":
			roots = append(roots, args[n+1:]...)
			n = len(args)
		default:
			if strings.HasPrefix(name, "-") {
				return fmt.Errorf("unknown session option %s", name)
			}
			roots = append(roots, args[n])
		}
	}
	loaded, loadErr := config.Load(configPath)
	if loadErr != nil {
		return loadErr
	}
	cfg = loaded
	if workers > 0 {
		cfg.MaxWorkers = workers
	}
	if provider != "normalized-json" && provider != "deja-vu" {
		return fmt.Errorf("unsupported session provider %q", provider)
	}
	if host == "" {
		return fmt.Errorf("host identity must not be empty")
	}
	if len(roots) == 0 {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		roots = []string{cwd}
	}
	opts := analyze.Options{Roots: roots, Config: cfg, NoPlugins: true}
	if verbose {
		opts.Progress = func(s string) { fmt.Fprintln(errout, s) }
	}
	native, err := analyze.Scan(ctx, opts)
	if err != nil {
		return err
	}
	sourceStarted := time.Now()
	if verbose {
		fmt.Fprintln(errout, "session artifact discovery/read started")
	}
	format := "normalized"
	if provider == "deja-vu" {
		format = "deja-sync"
	}
	source := sessions.FileSource{Paths: paths, Format: format, ProducerVersion: version, HostID: sourceHost}
	ss := []model.Session{}
	sources := []sessions.SourceArtifact{}
	diagnostics := []model.Diagnostic{}
	if len(paths) == 0 {
		diagnostics = append(diagnostics, model.Diagnostic{Provider: provider, Operation: "artifact-discovery", Message: "no explicitly supplied verified session artifact; provider skipped; no commands executed", Incomplete: true})
	} else {
		artifacts, err := source.Discover()
		if err != nil {
			diagnostics = append(diagnostics, model.Diagnostic{Provider: provider, Operation: "artifact-discovery", Message: err.Error(), Incomplete: true})
		} else {
			seen := map[string]int{}
			blocked := map[string]bool{}
			for _, a := range artifacts {
				imported, err := source.ReadSessions(ctx, a)
				if err != nil {
					diagnostics = append(diagnostics, model.Diagnostic{Provider: provider, Operation: "artifact-read", Path: a.Path, Message: err.Error(), Incomplete: true})
					continue
				}
				for _, s := range imported {
					for _, src := range s.Sources {
						a.SHA256 = src.SHA256
						if src.Earliest != nil && (a.Earliest == nil || src.Earliest.Before(*a.Earliest)) {
							a.Earliest = src.Earliest
						}
						if src.Latest != nil && (a.Latest == nil || src.Latest.After(*a.Latest)) {
							a.Latest = src.Latest
						}
					}
					if blocked[s.ID] {
						continue
					}
					if index, ok := seen[s.ID]; ok {
						merged, err := sessions.Merge(ss[index], s)
						if err != nil {
							diagnostics = append(diagnostics, model.Diagnostic{Provider: provider, Operation: "session-merge", Message: err.Error() + "; conflicting session excluded from classification", Incomplete: true})
							blocked[s.ID] = true
							ss[index].Intents = nil
							ss[index].Commands = nil
							ss[index].Decisions = nil
							ss[index].Context = nil
						} else {
							ss[index] = merged
						}
						continue
					}
					seen[s.ID] = len(ss)
					ss = append(ss, s)
				}
				sources = append(sources, a)
			}
		}
	}
	g, err := gitbackend.New()
	if err != nil {
		return err
	}
	sourceElapsed := time.Since(sourceStarted)
	if verbose {
		fmt.Fprintf(errout, "session artifact read complete: %d artifacts; %d sessions; %s\n", len(sources), len(ss), sourceElapsed)
	}
	engine := sessions.Engine{Git: g, Native: &native, HostID: host}
	if verbose {
		engine.Progress = func(message string) { fmt.Fprintln(errout, message) }
	}
	result := engine.Correlate(ctx, ss)
	result.Sources = sources
	result.Diagnostics = append(result.Diagnostics, diagnostics...)
	if !includeNative {
		result.Native = nil
	}
	result.Timings["source_read"] = sourceElapsed
	unknownStarted := time.Now()
	queue := sessions.FindUnknowns(ctx, &result)
	result.UnknownQueue = &queue
	result.Timings["unknown_reconciliation"] = time.Since(unknownStarted)
	prepareStarted := time.Now()
	sessions.Sanitize(&result, noText)
	result.Timings["report_preparation"] = time.Since(prepareStarted)
	write := func(w io.Writer) error {
		started := time.Now()
		if verbose {
			fmt.Fprintln(errout, "session report serialization started")
		}
		defer func() {
			if verbose {
				fmt.Fprintf(errout, "session report serialization finished: %s\n", time.Since(started))
			}
		}()
		if asJSON {
			return sessions.JSON(w, &result)
		}
		return sessions.Text(w, &result, includeCompleted)
	}
	if output != "" {
		// Also prohibit overwriting any source artifact, including artifacts outside repos.
		absolute, err := filepath.Abs(output)
		if err != nil {
			return err
		}
		for _, src := range sources {
			if absolute == src.Path {
				return fmt.Errorf("refusing output over source artifact")
			}
		}
		for _, path := range paths {
			input, err := filepath.Abs(path)
			if err == nil && absolute == input {
				return fmt.Errorf("refusing output over source artifact")
			}
		}
		protected := native
		protected.Scan.Roots = append([]string{}, native.Scan.Roots...)
		for _, path := range paths {
			if _, err := os.Stat(path); err == nil {
				protected.Scan.Roots = append(protected.Scan.Roots, path)
				if target, err := os.Stat(output); err == nil {
					input, _ := os.Stat(path)
					if os.SameFile(target, input) {
						return fmt.Errorf("refusing output over source artifact hardlink")
					}
				}
			}
		}
		return report.WriteOutside(output, &protected, write)
	}
	return write(out)
}
