# Session archaeology (unreleased on main)

Spuro 0.2.0-dev adds source-oriented ingestion and conservative per-intent
correlation. Build from main with `make build`; the published 0.1.1 binary does
not include these commands. Normal repository scans remain schema v1.

## Existing artifacts only

```sh
bin/spuro sessions ~/src --source /private/audit/normalized-sessions.json
bin/spuro sessions ~/src --source /private/audit/deja-sync-existing.jsonl \
  --provider deja-vu --deja-export-version 0.21.6 --source-host observed-host-id \
  --host-id observed-host-id --json --no-transcript-text \
  --output /private/audit/session-findings.json
```

`--source` is repeatable. Native scan exclusions and analysis settings use the same
configuration loading as `scan`; `--config` selects an explicit file. Plugins remain
disabled regardless of configured plugin settings. The directories shown must already exist, outside source
roots/repositories. Source files cannot be overwritten through output paths,
symlinks, or hardlinks. Missing/malformed sources produce diagnostics while native
scanning continues. With no source, the provider is skipped; Spuro never runs deja,
including help, query, sync export, refresh, usage, config, notes, or scrub commands.
It does not enumerate transcript directories or create exports. No plugin/semantic
service runs during a session scan.

The binary index is private and rebuild/version-dependent; it is not read. Existing
sync JSONL artifacts produced by **0.21.5 or 0.21.6** can be read using the matching
explicit producer version. Unknown versions and unexpected fields are rejected.
Those versions have identical reviewed SyncRecord fields, but no embedded schema
version or completeness marker. This is a bounded compatibility adapter, not a
claim that upstream promises a permanent public schema. Artifact generation by
an external user is separate from this command; Spuro never initiates it.

Every imported session retains provider, source path/type, reviewed format and
asserted producer version, file hash/mtime, observed record range where available,
staleness, and retention gaps. Provider/later-session coverage remains PARTIAL even
when a batch is recent. Mtime is not an authenticated generation/index-freshness
receipt. Source changes during reading are rejected; this is not an atomic estate
snapshot. Stale artifacts are ingested without modification.

Host identity is explicit for cross-host artifacts. The default scan host is the
local hostname; use `--host-id` for a previously verified identity and `--source-host`
for a verified export-source identity. Export origin labels are retained and never
silently mapped to a different local host. Unknown aliases, foreign host IDs, relative
project names, missing paths, and bare-only session locations remain unassociated.
Association uses canonical absolute checkout containment and native Git identity,
not repository names or provider-supplied RepoID claims. Multi-repository session
association is not implemented yet; ambiguous/spanning intents require review.

## Normalized fixtures and export contract

The provider-agnostic input envelope is `{"schema_version":1,"sessions":[...]}`.
Session and intent fields are defined in `internal/model/session.go`. This artifact
contains normalized observations, not full transcripts. Unknown fields/schema
versions, duplicate IDs within a source, and invalid completion groups are rejected.
No provider-supplied classification is accepted. Source read receipts are rebuilt
from the actual artifact rather than trusting input `sources` claims. Omitted command exits remain
unobserved, not success. The reader bounds file/record/session sizes.

Each intent has a unique ID within its session, a matching `session_id`, and explicit
completion groups: all groups must succeed; a group's `mode` is ALL or ANY. An
empty/unsupported criterion never establishes completion. Supported narrow predicates:

- `blob_at_path`: one repository-relative file, expected blob OID/object format;
  compares exact bytes at that path on captured normal-ref tips.
- `literal_in_file`: one file and bounded exact `literal`; establishes only that
  exact literal exists on a captured normal-ref tip, not semantic correctness.
- `commit_reachable`: expected commit OID/object format; reuses native reachability.
- `tree_represented`: expected tree OID/object format; reuses captured normal-ref
  commit trees and reports snapshot equivalence, not semantic supersession.
- `patch_represented`: expected commit OID/object format; reuses native stable patch
  IDs to recognize rebased/cherry-picked history. Empty/merge patches stay unknown.
- `command_passed`: one exact command; requires a later, explicitly linked session
  in the same checkout, observed zero exit/end time, evidence, and an `intent_refs`
  entry containing the original `session_id` and `intent_id`. Never runs the command.

Command `intent_ids` bind commands to intents within their own session. Cross-session
references are qualified `intent_refs`. An intent's `continues` list similarly uses
qualified intent references. Session `continues` links establish chronology and
checkout continuity; they do not imply every intent was resumed. Missing/overlapping
termination times prevent inferred chains. Later user decisions can supersede a
specific intent only with a linked, explicit `actor:user`, `decision:cancelled`,
target session/intent, and evidence. No semantic similarity chaining is attempted.

Sync exports contain text records, not reliable command exit or termination data.
They yield session inventory and conservative final-assistant TODO/Next/etc candidate
intents, with no invented completion criteria, command success, or actual session start/end
status. Unknown termination/completion stays UNKNOWN. Rules intentionally miss
implicit intent and do not interpret tool text as execution proof.

Repeated batches retain all sightings. Later timestamped sync tails replace earlier
candidate tails; conflicting equal-time tails and conflicting normalized observations
exclude that session from classification with a diagnostic. No transcript archive is
created. Timestamp ordering is provider evidence, not guaranteed clock accuracy.

## Classifications and output

Completion is per intent. Unsupported criteria or association gaps stay UNKNOWN.
Dirty state is linked to native findings without claiming session authorship; staged
changes, conflicts, deleted/renamed paths, and index entries remain native evidence.
Failures with incomplete later coverage are FAILED_UNRESOLVED, never abandoned.
Explicit intent continuations can be RESUMED_LATER without claiming completion.
Derive COMPLETED/PARTIALLY_COMPLETED/UNRESOLVED/MIXED/UNKNOWN summaries only afterward.
Session prose never elevates preservation severity; PRESERVE_FIRST requires matching
native preservation evidence. Matching refs/copies are not verified backups.

Session JSON uses `format:spuro-sessions`, `schema_version:1`, independent of the
native scan schema. It retains sources, normalized sessions, per-intent assessments,
coverage, findings, evidence, diagnostics, related native findings, and complete
associated checkout observations (including captured staged/index/conflict state).
Native observation metadata identifies snapshot timing and coverage. `--include-native`
adds the full native scan and can make output very large; it is off by default.
Missing/null provider fields mean unavailable observations, not negative facts.
Human output hides completed/superseded/resumed intents unless `--include-completed`.

`--no-transcript-text` removes descriptions, command bodies and literal predicates
after correlation; reports still contain paths, identifiers and native Git metadata.
Default reports contain only necessary normalized excerpts, with best-effort obvious
secret redaction. Redaction is not a guarantee; keep artifacts/reports private and
review before sharing. No upload or external LLM is part of this feature.

Limits: no comprehensive historic symbol search, squash/semantic completion,
automatic requirements extraction, live provider discovery, universal parsers,
session resumption, or durable archive/restore proof. Never use a session summary
as permission to clean a checkout or shared common Git directory.
