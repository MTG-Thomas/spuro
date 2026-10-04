package config

import (
	"fmt"
	"github.com/pelletier/go-toml/v2"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type Analysis struct {
	PatchEquivalence *bool `toml:"patch_equivalence"`
	Unreachable      *bool `toml:"unreachable"`
}
type Plugin struct {
	Command          string `toml:"command"`
	Enabled          bool   `toml:"enabled"`
	ReadOnlyVerified bool   `toml:"read_only_verified"`
	Documentation    string `toml:"documentation"`
	TimeoutSeconds   int    `toml:"timeout_seconds"`
}
type Config struct {
	Exclude    []string          `toml:"exclude"`
	Analysis   Analysis          `toml:"analysis"`
	Plugins    map[string]Plugin `toml:"plugins"`
	Primary    string            `toml:"primary"`
	MaxWorkers int               `toml:"max_workers"`
}

func Defaults() Config {
	b, u := true, true
	return Config{Exclude: []string{"**/node_modules/**", "**/.cache/**"}, Analysis: Analysis{&b, &u}, Plugins: map[string]Plugin{}, MaxWorkers: 4}
}
func Load(explicit string) (Config, error) {
	c := Defaults()
	paths := []string{}
	if explicit != "" {
		paths = append(paths, explicit)
	} else {
		if p, e := os.UserConfigDir(); e == nil {
			paths = append(paths, filepath.Join(p, "spuro", "config.toml"))
		}
		paths = append(paths, ".spuro.toml")
	}
	for _, p := range paths {
		b, e := os.ReadFile(p)
		if os.IsNotExist(e) && explicit == "" {
			continue
		}
		if e != nil {
			return c, e
		}
		if e = toml.Unmarshal(b, &c); e != nil {
			return c, fmt.Errorf("config %s: %w", p, e)
		}
	}
	return c, nil
}
func (c Config) PatchEnabled() bool {
	return c.Analysis.PatchEquivalence == nil || *c.Analysis.PatchEquivalence
}
func (c Config) UnreachableEnabled() bool {
	return c.Analysis.Unreachable == nil || *c.Analysis.Unreachable
}
func (p Plugin) Timeout() time.Duration {
	if p.TimeoutSeconds > 0 {
		return time.Duration(p.TimeoutSeconds) * time.Second
	}
	return 120 * time.Second
}

// Glob supports * and ** with slash-normalized paths, including zero-directory **/.
func Glob(pattern string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				i++
				if i+1 < len(pattern) && pattern[i+1] == '/' {
					i++
					b.WriteString("(?:.*/)?")
				} else {
					b.WriteString(".*")
				}
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(pattern[i])))
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}
