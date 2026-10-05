package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/MTG-Thomas/spuro/internal/analyze"
	"github.com/MTG-Thomas/spuro/internal/config"
	"github.com/MTG-Thomas/spuro/internal/model"
	"github.com/MTG-Thomas/spuro/internal/plugin"
	"github.com/MTG-Thomas/spuro/internal/report"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if e := run(ctx, os.Args[1:], os.Stdout, os.Stderr); e != nil {
		fmt.Fprintln(os.Stderr, "spuro:", e)
		os.Exit(1)
	}
}
func run(ctx context.Context, args []string, out, errout io.Writer) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		_, e := fmt.Fprintln(out, "Usage: spuro scan <root...> [--json] [--output PATH] [--plugin NAME] [--no-plugins] [--max-workers N] [--verbose] [--config PATH]\n       spuro sessions [root...] --source FILE [--json]\n       spuro version\n       spuro plugins [--json] [--config PATH]\nSpuro is read-only. Reports must be saved outside source roots and repositories.")
		return e
	}
	if args[0] == "sessions" {
		return runSessions(ctx, args[1:], out, errout)
	}
	if args[0] == "version" {
		if len(args) != 1 {
			return fmt.Errorf("version accepts no arguments")
		}
		_, e := fmt.Fprintln(out, "spuro", model.Version, "schema", model.SchemaVersion)
		return e
	}
	if args[0] != "scan" && args[0] != "plugins" {
		return fmt.Errorf("unknown command %q", args[0])
	}
	command := args[0]
	roots := []string{}
	names := []string(nil)
	output, configPath := "", ""
	asJSON, noPlugins, verbose := false, false, false
	workers := 0
	for i := 1; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			roots = append(roots, args[i+1:]...)
			break
		}
		name, value, has := strings.Cut(a, "=")
		next := func() (string, error) {
			if has {
				return value, nil
			}
			i++
			if i >= len(args) {
				return "", fmt.Errorf("missing value for %s", name)
			}
			return args[i], nil
		}
		switch name {
		case "--json":
			if has {
				return fmt.Errorf("--json accepts no value")
			}
			asJSON = true
		case "--no-plugins":
			noPlugins = true
		case "--verbose":
			verbose = true
		case "--output":
			v, e := next()
			if e != nil {
				return e
			}
			output = v
		case "--config":
			v, e := next()
			if e != nil {
				return e
			}
			configPath = v
		case "--plugin":
			v, e := next()
			if e != nil {
				return e
			}
			names = append(names, v)
		case "--max-workers":
			v, e := next()
			if e != nil {
				return e
			}
			workers, e = strconv.Atoi(v)
			if e != nil || workers < 1 {
				return fmt.Errorf("max-workers must be a positive integer")
			}
		default:
			if strings.HasPrefix(a, "-") {
				return fmt.Errorf("unknown flag %q", a)
			}
			roots = append(roots, a)
		}
	}
	cfg, e := config.Load(configPath)
	if e != nil {
		return e
	}
	if workers > 0 {
		cfg.MaxWorkers = workers
	}
	if command == "plugins" {
		if len(roots) > 0 || output != "" {
			return fmt.Errorf("plugins accepts --json and --config only")
		}
		entries := plugin.Registry(cfg)
		if asJSON {
			return json.NewEncoder(out).Encode(entries)
		}
		for _, p := range entries {
			fmt.Fprintf(out, "%s  enabled=%t available=%t read-only-verified=%t command=%q\n", p.Name, p.Enabled, p.Available, p.ReadOnlyVerified, p.Command)
		}
		return nil
	}
	opts := analyze.Options{Roots: roots, Config: cfg, Plugins: names, NoPlugins: noPlugins}
	if verbose {
		opts.Progress = func(s string) { fmt.Fprintln(errout, s) }
	}
	result, e := analyze.Scan(ctx, opts)
	if e != nil {
		return e
	}
	render := func(w io.Writer) error {
		if asJSON {
			return report.JSON(w, &result)
		}
		return report.Text(w, &result)
	}
	if output != "" {
		if e = report.WriteOutside(output, &result, render); e != nil {
			return e
		}
		fmt.Fprintln(errout, "Report:", output)
		return nil
	}
	return render(out)
}
