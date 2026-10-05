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

Session archaeology follows [ADR 0001](adr/0001-session-archaeology.md): per-intent
completion and explicit coverage precede derived session summaries. Normalized
models exist; CLI ingestion/correlation are not yet available. The deja-vu adapter
is blocked on a [verified read-only query surface](session-provider-safety.md).
