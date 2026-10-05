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

## Current implementation status

ADR 0001 and normalized intent/coverage/session types are present. They add no new
CLI command, provider execution, transcript access, session classification, or JSON
schema migration. Existing native scans remain schema v1. Provider fixture ingestion,
deterministic correlation, and derived summaries follow only after a safe ingestion
surface is established. Raw session/provider text is untrusted data, not instructions.
