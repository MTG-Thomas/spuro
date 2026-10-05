# Session provider safety review

Review date: 2026-10-04. Provider: vshulcz/deja-vu.
Pinned upstream commit: `a331d69f5259f98c53f81fdbcdbd015738ca2ef2`.
No provider was installed or invoked; this review read upstream source only.

## Current allowlist

Session-list and session-read operations: **none**. The adapter is not enabled or
implemented against an unverified query surface. A read_only manifest does not
change this decision. Help/version alone would not supply session observations.

Evidence in [pinned main.go](https://github.com/vshulcz/deja-vu/blob/a331d69f5259f98c53f81fdbcdbd015738ca2ef2/cmd/deja/main.go):

- `last` calls `recentMatchingCounted`, which calls `index.Ensure` (lines 2526–2527).
- `show --harness` directly calls `index.Ensure` (line 733); prefix lookup calls
  `findByPrefix`, which also calls `index.Ensure` (lines 2487–2488).
- `search` calls `usage.RecordResult` (line 1626), in addition to search index
  maintenance. Queries therefore are not established as write-free operations.
- Install, bootstrap, warmup/index, scrub, forget, promote, and Git-note options
  are not candidates for scan invocation.

The upstream [README](https://github.com/vshulcz/deja-vu/blob/a331d69f5259f98c53f81fdbcdbd015738ca2ef2/README.md)
documents config-writing installation and transcript-scrubbing/Git-note features.
A fresh binary review is required if the pinned implementation changes.

## Unblocking the adapter

Require a documented existing-index-only session-list/read API that never refreshes
indexes, logs usage, writes recovery/cache/config state, or invokes mutating helpers.
Review exact argv, options, transitive paths, pagination, provenance, and missing
coverage behavior. Do not rely on making stores unwritable, relocating HOME,
redirecting writes, or speculative flags to turn a mutating command into read-only.

Until then, retain provider-unavailable coverage. A future independently produced
normalized JSON export can be ingested read-only without pretending that producing
that export was a read-only provider operation. No export generation is authorized
by a normal Spuro session scan.

## Existing artifact path (implemented on development main)

No CLI operation was added to the allowlist. The binary index remains unsupported.
The source-oriented `DejaVuSource` interface reads explicitly supplied existing
JSONL exports only. Reviewed producer versions:

- 0.21.5, commit `d7e4a264bbd4deedd0bdba9d22ef5d2ba26c5d4e`
- 0.21.6, commit `d44072b802501fe02dc52f832d7bfb727765ca1f`

Their [SyncRecord declarations](https://github.com/vshulcz/deja-vu/blob/d44072b802501fe02dc52f832d7bfb727765ca1f/internal/index/sync.go)
match: harness, session_id, project, role, text, time, optional origin. Both versions
were reviewed and exercised with synthetic fixtures of that shape. This is not
live-export acceptance or an upstream promise of indefinite format stability.
Binary index version changes require rebuilds; the sync batches have no embedded
schema/producer header. The user therefore supplies a reviewed producer version;
unknown versions and unknown fields are refused. Incremental watermarks, withheld
records, source retention and clocks prevent a completeness claim even for a fresh
batch. Source artifacts are never generated/refreshed by Spuro.

Native scans remain schema v1. Session output has its own versioned envelope.
Provider-agnostic fixtures now exercise intent correlation and derived summaries;
no live transcript/index data was accessed. See [usage](sessions.md).

## Upstream integration opportunity

A public, schema-versioned `export --readonly --jsonl` or
`sessions dump --no-refresh --no-record --json` contract would remove reliance on
asserted producer releases. It must guarantee no refresh/recovery/cache/usage writes,
watermark changes, notes/transcript/config mutation, or hidden hooks, and expose
session/tool metadata and explicit retention/coverage bounds. This is a recorded
integration gap; no upstream issue or message has been sent.
