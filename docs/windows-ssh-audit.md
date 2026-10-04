# Windows laptop and SSH estate audit

Recommend auditing both the laptop and available, authorized SSH developer hosts.
Git remotes and SSH hosts answer different questions: a Git server's advertised refs
are not an inventory of dirty checkouts, worktrees, stashes, or reflogs on that host.
Do not assume every Git remote is a developer machine or grants shell access.

## Workflow and limits

1. Inspect Windows source-root candidates and relevant WSL distributions, if used.
   Keep Windows and WSL observations separate. Inspect existing SSH configuration
   and its Include files for explicit aliases and already documented developer hosts.
   Wildcard stanzas and known_hosts entries are hints, not authorization or a host list.
   Do not print private keys, credential files, tokens, or credential-bearing URLs.
2. Review effective connection settings and ProxyCommand/LocalCommand behavior before
   invocation. Use existing trusted credentials and host keys; do not disable host-key
   verification, accept unknown keys silently, forward an agent, or change SSH config.
   Use bounded, noninteractive connections to authorized explicit hosts. Report unknown
   host keys, missing credentials, and connection failures as incomplete coverage.
3. Establish host/user/OS identity, native Git and Spuro versions, and source roots.
   Deduplicate aliases for the same machine using observed identity, not names alone.
   A reachable host is not proof that every user account or source volume was scanned.
4. Run Spuro on each host locally, preferably native-only first. For Windows, use
   PowerShell argument arrays/quoted paths; use native Git for Windows roots and Linux
   Git inside WSL. Save JSON outside repositories, such as a dedicated directory under
   the platform temporary directory. For example, after creating that directory:

   ```powershell
   & $SpuroExe scan $SourceRoot --no-plugins --json --output $ReportPath --verbose
   if ($LASTEXITCODE -ne 0) { throw 'Spuro scan failed' }
   ```

   Inspect help before unfamiliar flags. If Spuro is absent, verified official release
   installation or building may write only to a dedicated audit-tools directory; do
   not install into audited repositories or change system configuration. External
   analyzers are optional and require documented read-only invocation.
5. Transfer reports with scp/SFTP to a private laptop audit directory; verify hashes.
   Avoid binary/text redirection through older PowerShell, which can alter encoding.
   Preserve original reports, host identity, roots, versions, timestamps, diagnostics,
   tool invocations, and timings. Reports contain sensitive local paths and Git metadata;
   do not upload them to GitHub or other services.
6. Rank preservation risks per host/repository, then investigate candidate copies across
   hosts. Compare exact OIDs only with matching object formats, tree OIDs, file/blob
   versions, and stable patch IDs where appropriate. Qualify Git-directory identities
   and paths by host: `/home/user/src/foo/.git` on two hosts is not one repository.
   Similar names or remote URLs alone do not prove equal contents or a safe backup.
   Separate JSON reports are not automatically correlated by Spuro today. Retain
   deterministic evidence for any agent-assisted cross-host equivalence conclusion.

No fetch, cleanup, pruning, ref creation, reset, checkout/switch, rebase, merge,
cherry-pick, stash/branch deletion, garbage collection, or repository writes are
part of this workflow. A remote copy may itself be weak, dirty, inaccessible, or
unbacked; report its preservation class rather than declaring work safe to delete.
Squash/semantic equivalence and process ownership remain human review questions.

## Agent prompt

```text
Audit development work on this Windows laptop and its available, authorized SSH
developer hosts using Spuro. Read https://github.com/MTG-Thomas/spuro and its
docs/windows-ssh-audit.md; follow applicable local AGENTS.md. This is a read-only
 preservation audit, not cleanup.

Discover significant Windows source roots and relevant existing WSL source roots.
Inspect existing SSH config/Include files and documented developer-host aliases.
Review connection settings before running SSH. Use only explicitly configured or
 documented hosts for which I have authorized access; do not probe networks or
 turn known_hosts entries/wildcard patterns into targets. Deduplicate aliases by
 observed host identity. Keep unknown hosts, inaccessible accounts, missing tools,
 unsupported Git, and incomplete scans as explicit coverage gaps.

Use existing trusted SSH credentials and host-key verification, noninteractive
 bounded connections, and no agent forwarding. Do not change credentials, SSH
 config, trust records, or system configuration. Never expose keys or tokens.

On the laptop, relevant WSL environments, and each accessible authorized developer
 host, record identity, OS, Git/Spuro versions, and source roots. Run Spuro locally
 on each host, native-only first. If missing, use verified official release binaries
 or build in a dedicated temporary audit-tools directory. Save raw JSON and notes
 outside all audited repositories; transfer remote reports privately to this
 laptop via scp/SFTP and verify hashes. Keep Windows/WSL/host identities separate.

Find dirty/staged/untracked/conflicted work, detached checkouts, unique unpushed
 branches, missing upstreams, stashes, reflog-only/unreachable history, and dropped
 stash candidates. Independently verify high-risk findings with read-only Git.
Investigate exact/tree/blob/patch equivalents across scanned hosts when practical;
 neither matching repository names nor remote URLs establish preservation. Spuro
 does not correlate separate host reports automatically. Distinguish additional
 copies from durable backups and do not claim squash or semantic equivalence.

Do not fetch, delete, prune, reset, checkout/switch, rebase, merge, cherry-pick,
 garbage-collect, change refs, create rescue refs, or modify repository contents.
External analyzers are optional and may run only documented read-only modes.

Produce a concise capsule summary plus a private, evidence-backed risk-ranked
 report: PRESERVE FIRST, REVIEW, HOUSEKEEPING. For high risks include host, repo,
 checkout/ref/OIDs, age, dirty status, upstream status, equivalent-copy evidence,
 and exact checks. State coverage gaps and suggest preservation steps for human
 approval; execute none. Do not publish audit data or contact anyone. Work through
 the authorized audit without stopping for routine choices.
```
