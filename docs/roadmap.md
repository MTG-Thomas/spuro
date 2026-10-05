# Remaining work

- Substantiated squash equivalence and bounded large-history incremental caching.
- Agent/process ownership evidence and deeper orphan-directory discovery.
- Broader Windows/macOS real-estate validation and non-POSIX plugin fixtures.
- Richer dropped-stash shape coverage and filesystem content equivalence.
- Explicit human-authorized preservation commands in a future version.
- Opt-in SSH-host orchestration and cross-host report correlation with host-qualified
  identities; the current recommended workflow runs separate local scans per host.

Remote-tracking refs can be stale; this scanner does not fetch or verify backups.
Ignored files and unscanned locations cannot support absence claims. An equivalent
patch is evidence about content, not proof of semantic obsolescence or safe deletion.

Session archaeology follows [ADR 0001](adr/0001-session-archaeology.md): development
main implements existing-artifact ingestion, per-intent predicates, explicit
continuation/cancellation and derived summaries. No provider CLI is invoked.
The binary index remains unsupported; see [usage/limits](sessions.md) and the
[reviewed artifact boundary](session-provider-safety.md). Live acceptance, stable
upstream versioned exports, multi-repository association, richer history predicates,
and semantic review remain future work.

UNKNOWN reconciliation now has an artifact-only pass. Remaining gaps include
validated imports of independent audit schemas, live private-corpus acceptance,
explicit multi-repository intent target binding, richer cached file/blob criteria,
and reviewed tracker evidence ingestion. Fuzzy suggestions remain inspection
leads; semantic completion and durable preservation are not inferred.
