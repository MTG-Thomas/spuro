# Changelog

## 0.2.0

- Offline UNKNOWN reconciliation over saved session JSON, with exact sighting groups,
  cached predicate witnesses, qualified links, coverage gaps, and separate fuzzy suggestions.

- Read existing normalized session artifacts and reviewed deja-vu sync JSONL without provider execution.
- Correlate per-intent Git/content predicates, dirty state, explicit continuations and cancellations.
- Report derived session summaries with independent coverage/confidence and source provenance.
- No live provider refresh, transcript archive, semantic completion, or session resumption.

### Session reconciliation

- Bound the default session report to a substantiated review shortlist; retain
  unverified candidates and ruled-down evidence separately in JSON.
- Show satisfied observable criteria as counter-evidence to blanket unresolved
  claims, without changing per-intent lifecycle conclusions.
- Add session artifact-read, association, correlation, and reporting timings and
  verbose progress. No provider execution or refresh was introduced.


## 0.1.1

- Correct the project license to AGPL-3.0-only.
- Republish platform archives with the AGPL-3.0 license text.

## 0.1.0

- Recursive native-Git discovery and common-directory worktree deduplication.
- Dirty, conflicted, detached, upstream, stash, reflog, and unreachable observations.
- Exact, tree, and stable patch equivalence with evidence-backed preservation findings.
- Versioned JSON and executable read-only GitWell/stalewood adapters.
- Fixture tests, cross-platform CI, vulnerability checks, and release archives.

No cleanup, rescue refs, network fetching, or semantic/squash equivalence.
