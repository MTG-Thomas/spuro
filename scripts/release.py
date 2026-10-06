#!/usr/bin/env python3
"""Spuro release operator: plan, signed-tag publish, and downloaded-asset rehearsal.

This developer tool is separate from the read-only Spuro scanner. Only publish
creates/pushes a signed tag. Packaging writes only its dedicated output directory.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import tarfile
import tempfile
import time
import zipfile

ROOT = Path(__file__).resolve().parents[1]
REPO = "MTG-Thomas/spuro"
COMMANDS = ("spuro", "spuro-plugin-gitwell", "spuro-plugin-stalewood")
TARGETS = [(osname, arch) for osname in ("linux", "darwin", "windows") for arch in ("amd64", "arm64")]


def run(argv, *, env=None, check=True):
    p = subprocess.run(argv, cwd=ROOT, env=env, capture_output=True, text=True)
    if check and p.returncode:
        raise RuntimeError(f"{argv[0]} failed: {p.stderr.strip()} {p.stdout.strip()}")
    return p


def output(argv):
    return run(argv).stdout.strip()


def sha(data):
    return hashlib.sha256(data).hexdigest()


def version():
    text = (ROOT / "internal/model/model.go").read_text()
    return re.search(r'const Version = "([^"]+)"', text)[1]


def stable(v):
    if not re.fullmatch(r"(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)", v):
        raise ValueError("release version must be X.Y.Z, without a dev suffix")
    return v


def verified_commit(commit):
    fact = json.loads(output(["gh", "api", f"repos/{REPO}/commits/{commit}"]))
    if fact["sha"] != commit or not fact["commit"]["verification"]["verified"]:
        raise ValueError("exact commit is not GitHub Verified")


def checks(commit):
    rows = json.loads(output(["gh", "api", f"repos/{REPO}/actions/runs?head_sha={commit}&per_page=100"]))["workflow_runs"]
    for name in ("ci", "govulncheck"):
        selected = [r for r in rows if r["name"] == name and r["head_sha"] == commit and r["event"] == "push" and r["head_branch"] == "main"]
        if not selected:
            raise ValueError(f"no main-push {name} evidence for exact commit")
        latest = max(selected, key=lambda r: r["id"])
        if latest["status"] != "completed" or latest["conclusion"] != "success":
            raise ValueError(f"{name} is not successful for exact commit")


def exists(tag):
    p = run(["gh", "api", f"repos/{REPO}/releases/tags/{tag}", "--include"], check=False)
    if p.returncode == 0:
        return True
    if re.search(r"^HTTP/\S+ 404\b", p.stdout, re.M):
        return False
    raise RuntimeError("release existence lookup failed; not treated as absence")


def preflight(v):
    stable(v)
    if version() != v:
        raise ValueError("commit the version/changelog/release notes first")
    if output(["git", "branch", "--show-current"]) != "main" or output(["git", "status", "--porcelain"]):
        raise ValueError("release requires clean main")
    if not (ROOT / f".github/releases/v{v}.md").is_file():
        raise ValueError("release notes missing")
    commit = output(["git", "rev-parse", "HEAD"])
    run(["git", "verify-commit", commit])
    remote = output(["git", "ls-remote", "origin", "refs/heads/main"]).split()[0]
    if remote != commit:
        raise ValueError("local main must equal origin/main")
    verified_commit(commit)
    checks(commit)
    if exists(f"v{v}"):
        raise ValueError("release exists; use verify-existing, never overwrite")
    return commit


def tag_target(tag):
    ref = json.loads(output(["gh", "api", f"repos/{REPO}/git/ref/tags/{tag}"]))
    if ref["object"]["type"] != "tag":
        raise ValueError("release requires signed annotated tag")
    obj = json.loads(output(["gh", "api", f"repos/{REPO}/git/tags/{ref['object']['sha']}"]))
    if obj["tag"] != tag or obj["object"]["type"] != "commit" or not obj["verification"]["verified"]:
        raise ValueError("tag signature/target is not GitHub Verified")
    return obj["object"]["sha"]


def check_tag(tag):
    stable(tag.removeprefix("v"))
    if tag != "v" + version():
        raise ValueError("tag does not match compiled version")
    commit = output(["git", "rev-parse", "HEAD"])
    if tag_target(tag) != commit:
        raise ValueError("tag is not the exact checkout")
    verified_commit(commit)
    checks(commit)
    run(["git", "merge-base", "--is-ancestor", commit, "origin/main"])
    if exists(tag):
        raise ValueError("release exists; refusing overwrite")
    if output(["git", "status", "--porcelain"]):
        raise ValueError("dirty release checkout")
    return commit


def package(osname, arch, dest):
    dest = Path(dest).resolve()
    if dest.exists():
        raise ValueError("package output must not already exist")
    dest.mkdir(parents=True)
    stage = dest / "payload"
    stage.mkdir()
    commit = output(["git", "rev-parse", "HEAD"])
    env = os.environ.copy()
    env.update(GOOS=osname, GOARCH=arch, CGO_ENABLED="0")
    native = output(["go", "env", "GOHOSTOS"]) == osname and output(["go", "env", "GOHOSTARCH"]) == arch
    manifest = {"schema_version": 1, "version": version(), "source_commit": commit, "go_version": output(["go", "env", "GOVERSION"]), "goos": osname, "goarch": arch, "runtime_smoke": native, "workflow_run_id": os.getenv("GITHUB_RUN_ID"), "workflow_run_attempt": os.getenv("GITHUB_RUN_ATTEMPT"), "binaries": {}}
    for command in COMMANDS:
        filename = command + (".exe" if osname == "windows" else "")
        binary = stage / filename
        run(["go", "build", "-trimpath", "-o", str(binary), f"./cmd/{command}"], env=env)
        info = json.loads(output(["go", "version", "-m", "-json", str(binary)]))
        settings = {s["Key"]: s["Value"] for s in info["Settings"]}
        if settings.get("vcs.revision") != commit or settings.get("vcs.modified") != "false" or settings.get("GOOS") != osname or settings.get("GOARCH") != arch or settings.get("CGO_ENABLED") != "0":
            raise ValueError("embedded build identity mismatch/dirty source")
        # Audit the actual compiled symbols for every target, without executing it.
        run(["govulncheck", "-mode", "binary", str(binary)])
        if native:
            if command == "spuro":
                if output([str(binary), "version"]) != f"spuro {version()} schema 1":
                    raise ValueError("built executable version mismatch")
                run([str(binary), "--help"])
            else:
                fact = json.loads(output([str(binary), "manifest"]))
                if fact.get("protocol_version") != 1 or fact.get("name") != command.removeprefix("spuro-plugin-") or not fact.get("read_only"):
                    raise ValueError("adapter smoke mismatch")
        manifest["binaries"][filename] = {"sha256": sha(binary.read_bytes()), "build_info": info}
    for filename in ("LICENSE", "README.md"):
        (stage / filename).write_bytes((ROOT / filename).read_bytes())
    name = f"spuro-v{version()}-{osname}-{arch}"
    receipt = json.dumps(manifest, indent=2).encode() + b"\n"
    (stage / "BUILD.json").write_bytes(receipt)
    (dest / f"{name}.build.json").write_bytes(receipt)
    if osname == "windows":
        with zipfile.ZipFile(dest / f"{name}.zip", "w", zipfile.ZIP_DEFLATED) as z:
            for file in sorted(stage.iterdir()):
                z.write(file, file.name)
    else:
        with tarfile.open(dest / f"{name}.tar.gz", "w:gz") as t:
            for file in sorted(stage.iterdir()):
                t.add(file, arcname=file.name)
    return manifest


def archive_files(file):
    if file.suffix == ".zip":
        with zipfile.ZipFile(file) as z:
            rows = {i.filename: z.read(i) for i in z.infolist() if not i.is_dir()}
            if len(rows) != len(z.infolist()):
                raise ValueError("duplicate/non-file ZIP entries")
    else:
        with tarfile.open(file) as t:
            entries = t.getmembers()
            if any(not i.isfile() for i in entries):
                raise ValueError("non-file tar entries")
            rows = {i.name: t.extractfile(i).read() for i in entries}
            if len(rows) != len(entries):
                raise ValueError("duplicate tar entries")
    if any("/" in n or "\\" in n or n in (".", "..") for n in rows):
        raise ValueError("unexpected archive paths")
    return rows


def actual_build_info(data):
    with tempfile.TemporaryDirectory(prefix="spuro-build-info-") as temp:
        file = Path(temp) / "binary"
        file.write_bytes(data)
        file.chmod(0o700)
        return json.loads(output(["go", "version", "-m", "-json", str(file)]))


def verify_assets(v, folder, commit=None):
    folder = Path(folder)
    records = {}
    for line in (folder / "SHA256SUMS").read_text().splitlines():
        if not re.fullmatch(r"[0-9a-f]{64}  [A-Za-z0-9_.-]+", line):
            raise ValueError("invalid checksum entry")
        digest, name = line.split("  ")
        if name in records:
            raise ValueError("duplicate checksum entry")
        records[name] = digest
    expected = {"spuro-sbom.cdx.json"}
    for osname, arch in TARGETS:
        name = f"spuro-v{v}-{osname}-{arch}"
        expected |= {name + (".zip" if osname == "windows" else ".tar.gz"), name + ".build.json"}
    if set(records) != expected or {p.name for p in folder.iterdir()} != expected | {"SHA256SUMS"}:
        raise ValueError("release must contain exactly all six target archives/receipts and SBOM")
    for name, digest in records.items():
        if sha((folder / name).read_bytes()) != digest:
            raise ValueError("downloaded checksum mismatch")
    sbom = json.loads((folder / "spuro-sbom.cdx.json").read_text())
    if sbom.get("bomFormat") != "CycloneDX" or not sbom.get("components"):
        raise ValueError("invalid/missing module SBOM")
    commits = set()
    for osname, arch in TARGETS:
        name = f"spuro-v{v}-{osname}-{arch}"
        file = folder / (name + (".zip" if osname == "windows" else ".tar.gz"))
        rows = archive_files(file)
        binaries = {c + (".exe" if osname == "windows" else "") for c in COMMANDS}
        if set(rows) != binaries | {"LICENSE", "README.md", "BUILD.json"}:
            raise ValueError("archive payload mismatch")
        receipt = json.loads(rows["BUILD.json"])
        if rows["BUILD.json"] != (folder / f"{name}.build.json").read_bytes():
            raise ValueError("archive/sidecar receipt mismatch")
        if receipt["version"] != v or receipt["goos"] != osname or receipt["goarch"] != arch:
            raise ValueError("version/platform mismatch")
        commits.add(receipt["source_commit"])
        for binary in binaries:
            info = receipt["binaries"][binary]
            if sha(rows[binary]) != info["sha256"]:
                raise ValueError("binary checksum mismatch")
            if actual_build_info(rows[binary]) != info["build_info"]:
                raise ValueError("downloaded binary build-info differs from receipt")
            settings = {s["Key"]: s["Value"] for s in info["build_info"]["Settings"]}
            if settings.get("vcs.revision") != receipt["source_commit"] or settings.get("vcs.modified") != "false" or settings.get("GOOS") != osname or settings.get("GOARCH") != arch or settings.get("CGO_ENABLED") != "0":
                raise ValueError("binary receipt source/target mismatch")
    if len(commits) != 1 or (commit and commits != {commit}):
        raise ValueError("mixed or unexpected source commits")
    return commits.pop()


def checksum(folder):
    folder = Path(folder)
    files = sorted(p for p in folder.iterdir() if p.name != "SHA256SUMS")
    (folder / "SHA256SUMS").write_text("".join(f"{sha(p.read_bytes())}  {p.name}\n" for p in files))


def verify_existing(v):
    stable(v)
    expected = tag_target(f"v{v}")
    with tempfile.TemporaryDirectory(prefix="spuro-release-verify-") as dest:
        run(["gh", "release", "download", f"v{v}", "--repo", REPO, "--dir", dest])
        commit = verify_assets(v, dest, expected)
    verified_commit(commit)
    print(f"verified downloaded release v{v}, source {commit}")


def release_runs():
    return json.loads(output(["gh", "api", f"repos/{REPO}/actions/workflows/release.yml/runs?per_page=100"]))["workflow_runs"]


def wait_release(tag, commit, known):
    deadline = time.monotonic() + 120
    while time.monotonic() < deadline:
        rows = [r for r in release_runs() if r["id"] not in known and r["event"] == "push" and r["head_branch"] == tag and r["head_sha"] == commit]
        if len(rows) > 1:
            raise ValueError("ambiguous release runs; inspect before retry")
        if rows:
            # Bind the watch to the newly created exact-tag run, never 'latest'.
            run(["gh", "run", "watch", str(rows[0]["id"]), "--repo", REPO, "--interval", "10", "--exit-status"])
            return
        time.sleep(10)
    raise RuntimeError("tag push submitted but no exact release run found; inspect before retry")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="action", required=True)
    for name in ("plan", "publish", "verify-existing"):
        p = sub.add_parser(name)
        p.add_argument("version")
    p = sub.add_parser("check-tag"); p.add_argument("tag")
    p = sub.add_parser("package"); p.add_argument("goos", choices=["linux", "darwin", "windows"]); p.add_argument("goarch", choices=["amd64", "arm64"]); p.add_argument("output")
    p = sub.add_parser("verify-assets"); p.add_argument("version"); p.add_argument("folder"); p.add_argument("--commit")
    p = sub.add_parser("checksum"); p.add_argument("folder")
    args = parser.parse_args()
    if args.action in ("plan", "publish"):
        commit = preflight(args.version)
        print(f"Plan: local gates -> signed tag v{args.version} at {commit} -> CI packaging/audit/SBOM -> downloaded-asset verification")
        if args.action == "publish":
            run(["make", "check", "build"])
            if preflight(args.version) != commit:
                raise ValueError("source changed during local gates")
            known = {r["id"] for r in release_runs()}
            run(["git", "tag", "-s", f"v{args.version}", commit, "-m", f"Spuro {args.version}"])
            run(["git", "verify-tag", f"v{args.version}"])
            run(["git", "push", "origin", f"v{args.version}"])
            print("tag submitted; an uncertain delivery must be inspected before retry", flush=True)
            wait_release(f"v{args.version}", commit, known)
            verify_existing(args.version)
    elif args.action == "check-tag": check_tag(args.tag)
    elif args.action == "package": package(args.goos, args.goarch, args.output)
    elif args.action == "verify-assets": verify_assets(args.version, args.folder, args.commit)
    elif args.action == "checksum": checksum(args.folder)
    else: verify_existing(args.version)


if __name__ == "__main__":
    try:
        main()
    except (ValueError, RuntimeError, KeyError) as error:
        raise SystemExit(str(error))
