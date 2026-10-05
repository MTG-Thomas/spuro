# Session archaeology

Spuro 0.2.0 adds source-oriented ingestion and conservative per-intent
correlation. Install the 0.2.0 release or build with `make build`. Earlier 0.1.x
binaries do not include these commands. Normal repository scans remain schema v1.

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

## Reconciliation shortlist

The default human report shows at most 20 substantiated review items, prioritizing
observed dirty state and intent-bound failed commands. UNKNOWN and unsubstantiated
promise candidates are counted separately; these counts are not forgotten-task
counts. Completion, supersession, and explicit continuation records are ruled down
for the shortlist, without treating continuation as completion. Use
`--include-completed` for the full reconciliation listing, including candidates.
JSON always retains all raw sessions and per-intent assessments and adds a
`reconciliation` partition with evidence references and satisfied criteria.
Partially satisfied criteria are counter-evidence to blanket abandonment claims.

Repository association is contextual: working-directory containment does not prove
that an intent targets that repository. Structured completion criteria must be
scoped by their producer; a narrative reference to another repository, a pending
board entry, or a stale blocker is not automatically a substantiated finding.
Spuro does not fetch PR/milestone status or infer cancellation from board prose.

`--verbose` reports artifact-read, correlation progress, and serialization phases.
JSON `phase_timings_ns` contains artifact reading, direct top-level association,
correlation total (including association and continuation lookup), and report
preparation durations. Serialization duration is emitted to stderr after writing,
not embedded retrospectively in its own output. These measurements are not a
profile of the private Windows corpus; repeated path canonicalization remains an
unverified performance hypothesis.

## Proactive UNKNOWN pass without another audit

Run against an existing **Spuro session snapshot**, not a private audit tool's
candidate CSV or undocumented JSON schema:

```sh
spuro sessions reconcile --input existing-sessions.json
spuro sessions reconcile --input existing-sessions.json --json --output reconciliation.json
spuro sessions reconcile --input existing-sessions.json --no-transcript-text --json
```

This command reads one supplied file, invokes no Git or provider executable, and
performs no repository discovery, canonicalization, index refresh, or network
query. Input and represented repository/source paths are protected against output
overwrite. Input SHA-256 is retained in the JSON receipt. Large inputs are bounded
at 256 MiB; incompatible schemas are refused. A native scan need not be present.

The same pass is included after a normal `sessions` scan. It produces:

- **EVIDENCE_READY**: previously satisfied criteria, exact qualified continuation
  or later user cancellation records, or cached normal-ref commit/tree/patch
  witnesses when a complete, unchanged native snapshot was included.
- **FOLLOW_UP_CANDIDATE**: similar later intent descriptions in the same contextual
  host/repository, with at least three discriminating tokens and 0.5 token overlap.
  The later intent's observed state is shown; it does not transfer to the original.
- **CONCRETE_CANDIDATE**: supplied paths or criteria give an inspection starting point.
- **BACKGROUND**: insufficient concrete evidence; retained in JSON with gaps.

Exact payload sightings are grouped only within a host-qualified harness/thread
and the same contextual scope, kind, origin, target and observable payload.
All member session/intent references remain. Missing host identity or text-free
payloads are not merged by an empty description. Distinct threads are never
merged by fuzzy similarity. At most three fuzzy suggestions per group and twenty
non-background groups appear in human output; all groups remain in JSON. If none
have concrete leads, five background groups show their evidence gaps and next
inspection step instead.

Timestamps of available artifacts are chronological clues, not proof a session
ended or resumed. Working-directory association is still contextual, particularly
for multi-repository conversations. Optional/broad board language is flagged for
scope review. Tracker/PR closure is not queried or interpreted as semantic proof.
Cached witnesses describe the original scan, not current state or durable backups.
This pass **never changes per-intent lifecycle states**. Inspect matched evidence
and original bounded user requests before supplying stronger completion criteria.

`--no-transcript-text` removes candidate descriptions after matching, retaining
reference IDs and non-prose match reasons. It does not remove identifying paths or
IDs; keep reports private. The private laptop candidate schema has not been
validated from this host and is not silently inferred by the offline reader.


## Offline context evidence (development main)

0.3.0-dev fixes repeated schema sightings suggesting themselves as later activity.
Different context/kind/payload sightings remain separate. Same-thread exact-description
references appear in `identity_candidates`, not fuzzy independent-activity suggestions.
They are reconciliation leads, not proven duplicates; group IDs may change from 0.2.0.
Genuinely different threads remain eligible for contextual fuzzy suggestions.

Normalized sessions optionally carry `context_evidence` with `schema_version:1`,
`bounded_requests` and `tracker_witnesses` arrays. Unknown versions are refused.
These are supplied offline observations, never instructions to execute or fetch.
The executable does not contact a tracker, provider, or network during reconciliation.
The additive fields require the development reader; 0.2.0 rejects unfamiliar fields.

Every context record has `id`, qualified `target:{session_id,intent_id}`, and
`provenance:{provider,artifact,record_id,observed_at,evidence}`. Evidence must be
nonempty; timestamps are required and observation time cannot precede the event.
The producer must independently check source records. A provenance label or hash
establishes internal consistency, not authenticity.

Before any match, the original intent requires:

- `recorded_at` and `source_event.at` identifying the same actual intent event;
- nonempty original `evidence` and a `source_event` containing `kind`, `at`,
  `match:"exact_payload"`, `description_sha256` and provenance;
- SHA-256 of the exact UTF-8 intent description, lowercase hex, matching
  `description_sha256`. The producer must first match the actual source payload;
- event kind `todo_write`, `user_message`, `assistant_message`, or `tool_result`.
  `origin:"task_board"` requires `todo_write`; `origin:"user_request"` requires
  `user_message`.

A last-assistant timestamp, schema/artifact mtime, or todo-table timestamp borrowed
from a session tail does not meet this contract. Missing event provenance leaves a
gap, even when `recorded_at` is populated. Redacted/text-free snapshots cannot
reestablish a changed or missing description hash binding. Do not rewrite a hash
merely to fit a redacted description; retain the original private source for review.

A bounded request supplies `actor:"user"`, `at`, `host_id`, `repo_id`, exact
repository-relative `allowed_paths`, and `other_work_forbidden`. No globs,
backslashes, absolute paths or parent traversal are accepted. Its event must be
later than the original intent. The original intent must also supply
`target_repo_id` and `target_resolution:{host_id,repo_id,method,provenance}`.
`method` is `explicit_repository` or `verified_user_path`; producer evidence must
resolve relative language such as “this directory.” Session cwd is never substituted.
Host and target must match exactly. The resulting bounded-path candidate does not
resume, cancel, complete or supersede an entire task board, or authorize current work.

A tracker witness supplies `subject:{forge_host,repository,kind,number,action}`,
`outcome`, `event_at`, `checked_by` and provenance. The original intent supplies the
same exact `tracker_target` tuple. Only `kind:"pull_request"`, `action:"merge"`,
`outcome:"merged"` with a later event produces a checked-merge evidence candidate.
A PR number alone cannot match: another repository's PR42 remains unrelated.
Closed issues/PRs, branch-protection observations and other intents are untouched.
Merge evidence is neither deployment proof nor durable-backup proof.

Context evidence only enriches the UNKNOWN inspection queue. It **does not change
per-intent lifecycle states**, coverage or the derived session summary. Conflicting
record IDs are rejected during source merge. Existing lifecycle predicates remain
the authority for their narrow supported conclusions. Synthetic tests are in
`internal/sessions/context_test.go`; no private corpus is included.
