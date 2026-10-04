# MVP validation

The test suite constructs disposable Git estates and checks discovery, nested and
bare repositories, indirection, common-directory deduplication, outside-root
registered worktrees, newline/Unicode paths, staged/modified/untracked files,
conflict stages, missing registrations, unborn repositories, upstream counts,
no/deleted upstreams, exact/tree/cherry-pick/rebase equivalence, reflog ancestry,
unreachable commits, current/dropped stashes and cross-clone blob preservation.

Scan fixtures snapshot repository metadata and checkout contents before/after.
A staged-only unique version is tested even when working bytes match a durable
commit. A tracked dropped-stash patch is tested after rebasing changed unrelated
file bytes. Plugin tests cover valid responses, incompatible/non-read-only
manifests, missing executables, invalid scan/manifest JSON, process failures,
timeout, unknown-executable refusal and contradictory native/plugin claims.
CLI and output tests cover interspersed flags, JSON, invalid flags and report
paths resolving into repositories through symlinks.

Production scanning invokes only gated native read commands. Mutations used for
fixture construction are confined to temporary test directories. Modified timestamps are checked as well as bytes/modes. Weak-ref namespaces
(such as legacy keep/snapshot refs) are grouped into tips rather than treated as
thousands of unrelated findings. Repeated plugin observations are deduplicated;
changed assertions are retained as separate evidence on the same finding.
Tests are not
signing/release workflows; project commits are signed separately.

The live acceptance estate uses the source roots from the earlier read-only audit,
plus the supplemental Codex worktree and held-integration roots. That audit found
approximately 59 logical repositories, 117 checkout paths and 85 linked worktrees.
Counts drift because other agents continue working, and Spuro itself adds a repo.
Acceptance compares preservation conclusions rather than hardcoding those counts.

Machine-specific raw reports, baseline OIDs and timing evidence are stored outside
repositories under `/tmp/spuro-mvp-validation`. No private corpus metadata is
committed here. The optional acceptance test validates a saved versioned report:

```sh
SPURO_ACCEPTANCE_REPORT=/tmp/spuro-mvp-validation/estate-final.json \
SPURO_ACCEPTANCE_BASELINE=/tmp/spuro-mvp-validation/acceptance-baseline.json \
go test ./internal/analyze -run TestAcceptanceReport -v
```

The external baseline requires the two registry proposal files, held integration
conflicts, four manually validated dropped-stash tips, shared-directory
deduplication, and some collapsed equivalent history. New concurrent dirty work
can produce additional high-risk findings. Incomplete native observations or
concurrent edits remain explicit diagnostics; this is not an atomic snapshot.

Performance phases are recorded under `scan.timings_seconds` and printed under
`--verbose`. Full `fsck --no-reflogs` verifies objects once per common directory;
reflog membership comes from native reflog ancestry, avoiding a redundant second
full fsck. Patch IDs are calculated once per object format/OID per scan, using a
bounded repository worker pool and streaming Git's patches into Git's patch-ID
implementation. Representative ref witnesses limit duplicate JSON evidence.

Remaining limits: no live remote verification or off-machine-backup proof; no
squash/semantic equivalence; no ignored-file or full-filesystem content scan;
no reliable process/agent ownership; no rescue operation; no persistent cache;
unknown merge/empty-patch equivalence; plugins use trusted contracts, not an OS
sandbox. Old tracked stash snapshots without deterministic equivalent-patch
proof remain review findings; absent untracked-parent content is ranked higher.
