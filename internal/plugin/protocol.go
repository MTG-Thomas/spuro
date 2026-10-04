// Package plugin normalizes supplemental observations without changing native facts.
package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"spuro/internal/config"
	"spuro/internal/model"
	"spuro/internal/process"
	"strings"
	"time"
)

const ProtocolVersion = 1

// A manifest declaration is a compatibility check, not an executable sandbox.
type Manifest struct {
	ProtocolVersion int      `json:"protocol_version"`
	Name            string   `json:"name"`
	Version         string   `json:"version"`
	Capabilities    []string `json:"capabilities"`
	ReadOnly        bool     `json:"read_only"`
}
type Request struct {
	ProtocolVersion int          `json:"protocol_version"`
	Roots           []string     `json:"roots"`
	Repositories    []Repository `json:"repositories"`
}
type Repository struct {
	ID           string `json:"id"`
	Path         string `json:"path"`
	CommonGitDir string `json:"common_git_dir"`
}
type Observation struct {
	Repository string         `json:"repository"`
	Kind       string         `json:"kind"`
	Subject    string         `json:"subject"`
	Checkout   string         `json:"checkout,omitempty"`
	OID        string         `json:"oid,omitempty"`
	Severity   string         `json:"severity,omitempty"`
	Message    string         `json:"message"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}
type Response struct {
	ProtocolVersion int           `json:"protocol_version"`
	Observations    []Observation `json:"observations"`
	Diagnostics     []string      `json:"diagnostics"`
}
type Entry struct {
	Name             string `json:"name"`
	Command          string `json:"command"`
	Enabled          bool   `json:"enabled"`
	Available        bool   `json:"available"`
	ReadOnlyVerified bool   `json:"read_only_verified"`
	Documentation    string `json:"documentation"`
}

func Registry(c config.Config) []Entry {
	names := map[string]bool{"gitwell": true, "stalewood": true}
	for n := range c.Plugins {
		names[n] = true
	}
	keys := []string{}
	for n := range names {
		keys = append(keys, n)
	}
	sort.Strings(keys)
	out := []Entry{}
	for _, n := range keys {
		p := c.Plugins[n]
		e := Entry{Name: n, Command: p.Command, Enabled: p.Enabled, ReadOnlyVerified: p.ReadOnlyVerified, Documentation: p.Documentation}
		if e.Command == "" && (n == "gitwell" || n == "stalewood") {
			e.Command = "spuro-plugin-" + n
			e.ReadOnlyVerified = true
			e.Documentation = "Spuro bundled adapter: documented report-only argv; docs/plugins.md"
		}
		_, err := exec.LookPath(e.Command)
		e.Available = err == nil
		out = append(out, e)
	}
	return out
}
func Run(ctx context.Context, c config.Config, names []string, result *model.Result) {
	registry := map[string]Entry{}
	for _, e := range Registry(c) {
		registry[e.Name] = e
	}
	if names == nil {
		for n, e := range registry {
			if e.Enabled {
				names = append(names, n)
			}
		}
	}
	sort.Strings(names)
	names = unique(names)
	req := Request{ProtocolVersion: 1, Roots: result.Scan.Roots, Repositories: []Repository{}}
	for _, r := range result.Repositories {
		req.Repositories = append(req.Repositories, Repository{r.ID, r.Path, r.CommonGitDir})
	}
	input, _ := json.Marshal(req)
	for _, name := range names {
		start := time.Now()
		entry, ok := registry[name]
		run := model.PluginRun{Name: name, Status: "failed"}
		fail := func(message string) {
			run.Message = message
			run.DurationSeconds = time.Since(start).Seconds()
			result.Plugins = append(result.Plugins, run)
			result.Diagnostics = append(result.Diagnostics, model.Diagnostic{Provider: name, Operation: "plugin", Message: message})
		}
		if !ok {
			fail("plugin is not configured")
			continue
		}
		if !entry.ReadOnlyVerified || entry.Documentation == "" {
			run.Status = "refused"
			fail("read-only invocation has not been independently verified; executable was not invoked")
			continue
		}
		if !entry.Available {
			run.Status = "unavailable"
			fail("executable not found: " + entry.Command)
			continue
		}
		timeout := c.Plugins[name].Timeout()
		z := process.Run(ctx, timeout, 16*1024*1024, entry.Command, []string{"manifest"}, nil)
		if z.Code != 0 {
			fail(commandError(z))
			continue
		}
		var manifest Manifest
		if e := json.Unmarshal(z.Stdout, &manifest); e != nil {
			fail("invalid manifest JSON: " + e.Error())
			continue
		}
		if manifest.ProtocolVersion != 1 || manifest.Name != name || !manifest.ReadOnly {
			run.Status = "refused"
			fail("incompatible or non-read-only manifest")
			continue
		}
		run.Version = manifest.Version
		z = process.Run(ctx, timeout, 32*1024*1024, entry.Command, []string{"scan"}, strings.NewReader(string(input)))
		if z.Code != 0 {
			fail(commandError(z))
			continue
		}
		var response Response
		if e := json.Unmarshal(z.Stdout, &response); e != nil {
			fail("invalid scan JSON: " + e.Error())
			continue
		}
		if response.ProtocolVersion != 1 {
			fail("incompatible response protocol")
			continue
		}
		for _, message := range response.Diagnostics {
			result.Diagnostics = append(result.Diagnostics, model.Diagnostic{Provider: name, Operation: "plugin", Message: message})
		}
		for _, o := range response.Observations {
			index := -1
			for i, r := range result.Repositories {
				if o.Repository == r.ID || o.Repository == r.Path || o.Repository == r.CommonGitDir {
					index = i
					break
				}
				for _, ct := range r.Checkouts {
					if ct.Path == o.Repository {
						index = i
						break
					}
				}
				if index >= 0 {
					break
				}
			}
			if index < 0 {
				result.Diagnostics = append(result.Diagnostics, model.Diagnostic{Provider: name, Operation: "normalization", Path: o.Repository, Message: "observation does not match a native repository; retained as diagnostic"})
				continue
			}
			r := &result.Repositories[index]
			metadataJSON, _ := json.Marshal(o.Metadata)
			ev := model.Evidence{ID: model.ID("ev-", name, r.ID, o.Kind, o.Subject, o.Checkout, o.OID, o.Message, string(metadataJSON)), Provider: name, ProviderVersion: manifest.Version, Operation: "plugin", Fact: o.Kind, Subject: o.Subject, Data: map[string]any{"message": o.Message, "metadata": o.Metadata, "claimed_severity": o.Severity, "native_authority": false}}
			exists := false
			for _, old := range r.Evidence {
				if old.ID == ev.ID {
					exists = true
					break
				}
			}
			if exists {
				continue
			}
			r.Evidence = append(r.Evidence, ev)
			findingID := model.ID("SPURO-", name, r.ID, o.Kind, o.Subject, o.Checkout, o.OID)
			findingIndex := -1
			for i := range result.Findings {
				if result.Findings[i].ID == findingID {
					findingIndex = i
					break
				}
			}
			if findingIndex >= 0 {
				result.Findings[findingIndex].Evidence = append(result.Findings[findingIndex].Evidence, ev.ID)
			} else {
				result.Findings = append(result.Findings, model.Finding{ID: findingID, Severity: model.Info, Kind: "plugin_observation", RepoID: r.ID, Checkout: o.Checkout, Ref: o.Subject, OID: o.OID, Paths: []string{}, Summary: o.Message, Evidence: []string{ev.ID}, Confidence: model.Low, SuggestedAction: "Verify this supplemental assertion with native evidence; it cannot override native facts."})
			}
			run.Observations++
		}
		run.Status = "ok"
		if len(response.Diagnostics) > 0 {
			run.Status = "partial"
		}
		run.DurationSeconds = time.Since(start).Seconds()
		result.Plugins = append(result.Plugins, run)
	}
}
func commandError(z process.Result) string {
	if z.Err != nil {
		return z.Err.Error()
	}
	return fmt.Sprintf("plugin exited %d (stderr omitted to avoid leaking arbitrary tool output)", z.Code)
}
func unique(in []string) []string {
	out := []string{}
	for _, s := range in {
		if len(out) == 0 || out[len(out)-1] != s {
			out = append(out, s)
		}
	}
	return out
}
