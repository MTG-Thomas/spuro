# Spuro

[![CI](https://github.com/MTG-Thomas/spuro/actions/workflows/ci.yml/badge.svg)](https://github.com/MTG-Thomas/spuro/actions/workflows/ci.yml)
[![Vulnerability check](https://github.com/MTG-Thomas/spuro/actions/workflows/govulncheck.yml/badge.svg)](https://github.com/MTG-Thomas/spuro/actions/workflows/govulncheck.yml)

Spuro finds traces of development work left behind across Git repositories and
worktrees. Its question is: **what work on this machine lacks an obvious,
durable, equivalent copy in the scanned Git estate?**

It recursively audits source roots for uncommitted changes, detached checkouts,
unpushed branches, reflog-only history, dropped stashes and unreachable commits.
Native Git supplies the facts; exact reachability, tree equality and stable patch
IDs help separate unique work from rebased, cherry-picked or duplicated history.
Spuro is a read-only preservation-risk scanner with optional session
archaeology. It performs no cleanup, rescue, or agent-resumption operations.

[Spuro 0.3.0](https://github.com/MTG-Thomas/spuro/releases/tag/v0.3.0) tightens offline
UNKNOWN identity grouping and adds provenance-bound context, scoped-request and
stash-source evidence.
See the [session contract](docs/sessions.md#offline-context-evidence-development-main).

## Install

Download an archive from [GitHub Releases](https://github.com/MTG-Thomas/spuro/releases),
verify SHA256SUMS, and place the executable on PATH. Native Git is required separately.
Or install with Go: `go install github.com/MTG-Thomas/spuro/cmd/spuro@latest`.
Adapters are optional; `make build` builds all three executables.

Releases use signed tags, CI-built native archives, checksums, build receipts and a
CycloneDX SBOM. See the [release process](docs/releasing.md).

See [maturity](docs/maturity.md), [contributing](CONTRIBUTING.md), and the [AGPL-3.0 license](LICENSE).

## Build and scan

Requires Go 1.26.8+ and a native Git executable supporting porcelain v2 and
`git worktree list --porcelain -z` (tested with Git 2.47.3 on Linux).

```sh
go build -o /tmp/spuro ./cmd/spuro
/tmp/spuro scan ~/src ~/projects --no-plugins
/tmp/spuro scan ~/src --json --output /tmp/spuro-scan.json --verbose
/tmp/spuro version
/tmp/spuro plugins --json
```

Flags can follow roots. `--max-workers N` bounds repository tasks; shared common
Git directories are analyzed once. Registered worktrees outside the roots are
also inspected. Nested repositories, bare repositories and `.git` indirection
files are supported. Symlinked directories are not recursively followed.

JSON is a versioned API (`schema_version: 1`), containing repositories, checkout
state, refs, branches, stashes, commit metadata, lineage equivalence, evidence,
findings, plugin runs, diagnostics and phase timings. Finding IDs are deterministic
for unchanged repository identity and subject. Moving a common Git directory
changes its identity. Reports must be written outside all scanned roots and
repository paths. Without `--output`, results go to stdout.

The human report shows all `PRESERVE_FIRST` findings, the first 20 `REVIEW`
findings, and counts of quieter findings. JSON retains all observations.

## Audit available SSH hosts too

A laptop-only scan can miss development work on SSH-accessible developer hosts.
Include known, authorized SSH hosts in an estate audit: inspect configured aliases,
confirm their identities, discover source roots on each host, and run a separate
read-only native scan there. Keep host-qualified evidence and coverage gaps.
An SSH-accessible checkout can contain the only copy of dirty or stranded work.

This is an operator/agent workflow, not built-in remote-host scanning. Spuro scans
local roots on the host where it runs; it does not enumerate SSH hosts, connect,
or correlate separate host reports automatically. Use the
[Windows and SSH audit guide](docs/windows-ssh-audit.md), which includes an agent prompt.

## Configuration

Spuro loads the platform user configuration (`~/.config/spuro/config.toml` on
Linux), then `.spuro.toml` in the current directory. `--config PATH` instead loads
only that file. CLI flags override configuration. Native-only scans are fully
functional; bundled plugins are disabled unless explicitly selected or enabled.

```toml
max_workers = 4
exclude = ["**/node_modules/**", "**/.cache/**"]

[analysis]
patch_equivalence = true
unreachable = true

[plugins.gitwell]
enabled = true

[plugins.stalewood]
enabled = true
```

Excluded and ignored files are outside complete coverage. `--no-plugins` overrides
plugin selection and configuration. Disabling patch analysis or unreachable
analysis reduces coverage; it does not prove that unknown work is preserved.

## Evidence and interpretation

* Git branches, tags, rescue/archive refs and notes refs are classified as durable
  local refs; remote-tracking refs are recognized separately. Neither class is
  proof of an off-machine backup. Git stash, worktree HEADs and reflogs are weaker
  preservation mechanisms. Other namespaces remain `UNKNOWN`.
* A branch is compared against other refs, excluding its own ref. Copies in
  independent scanned clones count as additional preservation evidence. Upstream
  ahead/behind counts are independent of inferred equivalence.
* Exact commit reachability is strongest. Identical trees establish snapshot
  equality. Stable patch IDs establish Git's whitespace-insensitive patch
  equivalence for nonmerge commits. Empty and merge patches may remain unknown.
  This does **not** establish squash equivalence or semantic obsolescence.
* Reflog ancestry and fsck-discovered commit ancestry are grouped into tips.
  Dropped-stash candidates must have the expected parent structure; all working,
  index and untracked-parent versions are inspected. A matching historic blob
  establishes that file bytes exist under a scanned normal ref, not that the
  stash's intended combination, paths, modes or deletion intent was integrated.
* Dirty working files and changed index stages are compared by raw Git blob hash,
  without invoking clean filters. Unknown directories/submodules and special
  files remain diagnostic coverage gaps. Whole-filesystem deduplication is absent.
* Ref/registration/status changes during a scan produce incomplete-coverage
  diagnostics. A scan is not an atomic snapshot. Inspect evidence and rerun when
  other developers or agents are actively changing the estate.

No network fetch occurs. Remote-tracking refs can be stale. Unregistered checkouts
outside the roots, ignored files, broken or missing object stores, object content
already garbage-collected, and agent/process ownership require separate review.
Old tracked stash versions without equivalence proof remain review items; absent
untracked-parent versions are ranked more urgently.

The scanner disables optional Git locks, fsmonitor, untracked-cache updates,
automatic maintenance, external diff/textconv and lazy fetching. Git subprocesses
use argv arrays, bounded output, timeouts and isolated process groups.

## Session archaeology

Spuro reads existing normalized session artifacts or reviewed deja-vu sync JSONL
batches, then correlates **each concrete intent** with native Git, dirty state,
recorded commands and explicit later-session links. It never executes deja-vu,
refreshes an index, creates an export, or uploads transcripts. Provider freshness
and later-history gaps remain explicit; unresolved does not mean abandoned.

```sh
spuro sessions ~/src --source /private/audit/normalized-sessions.json \
  --json --output /private/audit/sessions.json --verbose

# Reconcile the saved snapshot without repeating the estate audit.
spuro sessions reconcile --input /private/audit/sessions.json
spuro sessions reconcile --input /private/audit/sessions.json \
  --json --no-transcript-text --output /private/audit/reconciliation.json
```

Offline reconciliation groups exact sightings within host-qualified threads,
finds cached witnesses and qualified links, and ranks separate fuzzy suggestions.
The shortlist exposes scope, chronology, criteria and coverage gaps; it never
promotes UNKNOWN to completed or abandoned. JSON retains every candidate.
Private audit CSV/custom JSON schemas are not automatically inferred.

Session reports contain paths and identifying metadata even with
`--no-transcript-text`; keep them private. Session coverage is fixture-tested;
the laptop-private candidate corpus has not been validated by the implementation
host. See [session usage and limits](docs/sessions.md) and
[provider safety](docs/session-provider-safety.md).

## Supplemental analyzers

Build adapters alongside the scanner and place them on PATH:

```sh
go build -o /tmp/spuro-plugin-gitwell ./cmd/spuro-plugin-gitwell
go build -o /tmp/spuro-plugin-stalewood ./cmd/spuro-plugin-stalewood
PATH=/tmp:$PATH /tmp/spuro scan ~/src --plugin gitwell --plugin stalewood
```

The backend tools must also be on PATH. The adapters invoke GitWell's ordinary
`PATH --json` scan and stalewood's `--json --quiet PATH` reporting mode, never their
cleanup, report-writing or triage operations. GitWell is invoked once per logical
repository rather than depending on its shallow source-root discovery.

Plugin errors are recorded and native scanning continues. Plugin assertions have
provenance and low-confidence informational findings. They cannot replace native
facts or promote preservation severity. See [the protocol](docs/plugins.md).

## Validation

```sh
go test ./...
go test -race ./...
go vet ./...
```

Tests build disposable repositories, including deliberate conflicts, reflog and
unreachable history, linked worktrees, upstream states, dropped stashes and patch
equivalents. Scan tests compare checkout files and repository metadata byte for
byte before and after. Fixture construction alone uses Git mutations; production
scanner code has a read-only command gate. See [estate validation](docs/validation.md)
for the acceptance corpus and recorded limitations.

## License

Copyright (c) 2026 Thomas Bray. Spuro is licensed under the GNU Affero General
Public License, version 3 only (`AGPL-3.0-only`). See [LICENSE](LICENSE).
Third-party dependencies retain their respective licenses.
