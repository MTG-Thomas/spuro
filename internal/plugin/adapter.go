package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/MTG-Thomas/spuro/internal/process"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// AdapterMain invokes only the documented JSON reporting modes audited for
// GitWell 0.1.1 and stalewood 0.1.6. No report/triage/prune command is permitted.
func AdapterMain(name string, args []string, in io.Reader, out io.Writer) error {
	if name != "gitwell" && name != "stalewood" {
		return fmt.Errorf("unknown adapter %q", name)
	}
	if len(args) != 1 {
		return fmt.Errorf("usage: spuro-plugin-%s manifest|scan", name)
	}
	enc := json.NewEncoder(out)
	if args[0] == "manifest" {
		return enc.Encode(Manifest{1, name, "0.1.0", []string{"supplemental-observations"}, true})
	}
	if args[0] != "scan" {
		return fmt.Errorf("unsupported operation")
	}
	var req Request
	if e := json.NewDecoder(io.LimitReader(in, 16*1024*1024)).Decode(&req); e != nil {
		return e
	}
	if req.ProtocolVersion != 1 {
		return fmt.Errorf("unsupported protocol")
	}
	response := Response{ProtocolVersion: 1, Observations: []Observation{}, Diagnostics: []string{}}
	paths := []string{}
	if name == "gitwell" {
		for _, r := range req.Repositories {
			paths = append(paths, r.Path)
		}
	} else {
		paths = append(paths, req.Roots...)
	}
	sort.Strings(paths)
	ctx := context.Background()
	for _, path := range unique(paths) {
		if !filepath.IsAbs(path) {
			response.Diagnostics = append(response.Diagnostics, "refused nonabsolute backend path: "+path)
			continue
		}
		argv := []string{path, "--json"}
		if name == "stalewood" {
			argv = []string{"--json", "--quiet", path}
		}
		z := process.Run(ctx, 90*time.Second, 32*1024*1024, name, argv, nil)
		if z.Code != 0 {
			response.Diagnostics = append(response.Diagnostics, path+": "+commandError(z))
			continue
		}
		observations, e := NormalizeAdapter(name, z.Stdout)
		if e != nil {
			response.Diagnostics = append(response.Diagnostics, path+": invalid backend JSON: "+e.Error())
			continue
		}
		response.Observations = append(response.Observations, observations...)
	}
	return enc.Encode(response)
}
func NormalizeAdapter(name string, data []byte) ([]Observation, error) {
	var envelope struct {
		Repos []map[string]json.RawMessage `json:"repos"`
	}
	if e := json.Unmarshal(data, &envelope); e != nil {
		return nil, e
	}
	out := []Observation{}
	str := func(m map[string]any, k string) string { v, _ := m[k].(string); return v }
	for _, raw := range envelope.Repos {
		if name == "gitwell" {
			var path string
			_ = json.Unmarshal(raw["path"], &path)
			var sections map[string][]map[string]any
			if e := json.Unmarshal(raw["sections"], &sections); e != nil {
				return nil, e
			}
			keys := []string{}
			for k := range sections {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				for _, f := range sections[k] {
					subject := str(f, "branch")
					if subject == "" {
						subject = str(f, "sha")
					}
					message := str(f, "message")
					if message == "" {
						message = k + ": " + subject
					}
					out = append(out, Observation{Repository: path, Kind: str(f, "type"), Subject: subject, OID: str(f, "sha"), Message: message, Metadata: f})
				}
			}
		} else {
			var repo string
			_ = json.Unmarshal(raw["repo"], &repo)
			var trees []map[string]any
			if e := json.Unmarshal(raw["worktrees"], &trees); e != nil {
				return nil, e
			}
			for _, w := range trees {
				path := str(w, "path")
				out = append(out, Observation{Repository: repo, Kind: "worktree_hint", Subject: str(w, "branch"), Checkout: path, OID: str(w, "head"), Message: "stalewood worktree observation: " + path, Metadata: w})
			}
		}
	}
	return out, nil
}
func ExitAdapter(name string) {
	if e := AdapterMain(name, os.Args[1:], os.Stdin, os.Stdout); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
