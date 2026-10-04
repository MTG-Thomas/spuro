# Executable plugin protocol v1

A plugin accepts exactly `manifest` or `scan` as its argv operation. Manifest is
JSON on stdout. Scan reads one JSON request from stdin and returns one JSON
response on stdout; stderr is not treated as evidence. Output and runtime are
bounded. No shell is used to launch plugins. Config `command` is an executable
path/name, not a command string with arguments.

```json
{"protocol_version":1,"name":"example","version":"0.1.0","capabilities":["repository-findings"],"read_only":true}
```

Request:

```json
{"protocol_version":1,"roots":["/src"],"repositories":[{"id":"repo-...","path":"/src/repo","common_git_dir":"/src/repo/.git"}]}
```

Response:

```json
{"protocol_version":1,"observations":[{"repository":"/src/repo","kind":"stale_branch","subject":"feature/example","severity":"warning","message":"Old branch","metadata":{"age_days":120}}],"diagnostics":[]}
```

`repository` must match a native repository ID, observation path, checkout path
or common Git directory. Unmatched observations become diagnostics. Additional
optional observation fields are `checkout` and `oid`. Metadata is supplemental;
Spuro never interprets arbitrary plugin metadata as authoritative Git facts.

Bundled adapters expose manifest version `0.1.0`. This is the adapter version,
not a claim about the installed backend version. Their reporting argv was checked
against the following upstream documentation/source:

* [GitWell](https://github.com/DavidCanHelp/GitWell), v0.1.1,
  commit `77f09038ad849b58ba8cff0e1ca402026c1d7090`: default scan with `--json`.
  Its `report` subcommand writes files and is deliberately excluded.
* [stalewood](https://github.com/retif/stalewood), v0.1.6,
  commit `1a9a61742efd45c7f9ceaf70a1304c34bbd16169`: `--json --quiet PATH`,
  with no prune/force flags. JSON is normalized per worktree.

For a custom plugin, configuration must record an independent safety review:

```toml
[plugins.example]
command = "/opt/plugins/spuro-plugin-example"
enabled = true
read_only_verified = true
documentation = "Reviewed v1 report-only invocation; https://example.test/docs"
timeout_seconds = 30
```

Without `read_only_verified` and documentation, Spuro refuses to execute even the
manifest operation. After that review, it still rejects non-read-only manifests,
name mismatches and incompatible protocol versions. These checks are an execution
contract, **not an OS sandbox**. Install only trusted executables. Plugin failure,
malformed JSON, timeout, partial backend results or unavailable tools cannot erase
or modify native observations. Native incomplete coverage and plugin failure are
reported separately.

The MVP adapters are supplemental prototypes. They expose backend metadata as
observations, not preservation decisions. Deadwood and other code analyzers may
later use this protocol, but semantic-obsolescence judgments are outside MVP.
