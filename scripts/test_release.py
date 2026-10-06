import copy
import io
import json
from pathlib import Path
import subprocess
import tarfile
import tempfile
import unittest
from unittest.mock import patch
import zipfile

import release


class ReleaseTests(unittest.TestCase):
    def fixtures(self, root):
        commit = "a" * 40
        info = {"GoVersion": "go1.26.8", "Settings": [{"Key": "vcs.revision", "Value": commit}, {"Key": "vcs.modified", "Value": "false"}, {"Key": "CGO_ENABLED", "Value": "0"}]}
        for osname, arch in release.TARGETS:
            data = copy.deepcopy(info)
            data["Settings"] += [{"Key": "GOOS", "Value": osname}, {"Key": "GOARCH", "Value": arch}]
            binaries = {c + (".exe" if osname == "windows" else ""): b"synthetic binary" for c in release.COMMANDS}
            receipt = {"version": "1.2.3", "source_commit": commit, "goos": osname, "goarch": arch, "binaries": {c: {"sha256": release.sha(b), "build_info": data} for c, b in binaries.items()}}
            encoded = json.dumps(receipt).encode()
            rows = dict(binaries, LICENSE=b"AGPL-3.0-only", **{"README.md": b"fixture", "BUILD.json": encoded})
            name = f"spuro-v1.2.3-{osname}-{arch}"
            (root / f"{name}.build.json").write_bytes(encoded)
            if osname == "windows":
                with zipfile.ZipFile(root / f"{name}.zip", "w") as z:
                    for n, b in rows.items():
                        z.writestr(n, b)
            else:
                with tarfile.open(root / f"{name}.tar.gz", "w:gz") as t:
                    for n, b in rows.items():
                        entry = tarfile.TarInfo(n); entry.size = len(b)
                        t.addfile(entry, io.BytesIO(b))
        (root / "spuro-sbom.cdx.json").write_text(json.dumps({"bomFormat": "CycloneDX", "components": [{"name": "stdlib"}]}))
        release.checksum(root)
        return commit

    def test_complete_asset_set_and_target_receipts(self):
        with tempfile.TemporaryDirectory() as dest:
            root = Path(dest); commit = self.fixtures(root)
            def info(data):
                return next(iter(self.receipt["binaries"].values()))["build_info"]
            original = release.archive_files
            def archive(file):
                rows = original(file); self.receipt = json.loads(rows["BUILD.json"]); return rows
            with patch.object(release, "archive_files", side_effect=archive), patch.object(release, "actual_build_info", side_effect=info):
                self.assertEqual(release.verify_assets("1.2.3", root, commit), commit)
                with self.assertRaises(ValueError): release.verify_assets("1.2.3", root, "b" * 40)
                (root / "unexpected").write_text("extra")
                with self.assertRaises(ValueError): release.verify_assets("1.2.3", root, commit)

    def test_tamper_missing_matrix_and_bad_sbom(self):
        for change in ("tamper", "missing", "sbom"):
            with self.subTest(change=change), tempfile.TemporaryDirectory() as dest:
                root = Path(dest); self.fixtures(root)
                if change == "tamper": (root / "spuro-v1.2.3-windows-arm64.zip").write_bytes(b"changed")
                if change == "missing": (root / "spuro-v1.2.3-linux-arm64.tar.gz").unlink()
                if change == "sbom":
                    (root / "spuro-sbom.cdx.json").write_text('{}'); release.checksum(root)
                with self.assertRaises((ValueError, FileNotFoundError)): release.verify_assets("1.2.3", root)

    def test_archive_paths_and_symlinks_refused_without_extraction(self):
        with tempfile.TemporaryDirectory() as dest:
            file = Path(dest) / "bad.zip"
            with zipfile.ZipFile(file, "w") as z: z.writestr("../escape", b"bad")
            with self.assertRaises(ValueError): release.archive_files(file)
            file = Path(dest) / "bad.tar.gz"
            with tarfile.open(file, "w:gz") as t:
                entry = tarfile.TarInfo("link"); entry.type = tarfile.SYMTYPE; entry.linkname = "/outside"; t.addfile(entry)
            with self.assertRaises(ValueError): release.archive_files(file)

    def test_confirmed_absence_only(self):
        for code, stdout, absent in ((1, "HTTP/2 404 Not Found\n", True), (1, "HTTP/2 403 Forbidden\n", False), (1, "", False)):
            with patch.object(release, "run", return_value=subprocess.CompletedProcess([], code, stdout, "failure")):
                if absent: self.assertFalse(release.exists("v1.2.3"))
                else:
                    with self.assertRaises(RuntimeError): release.exists("v1.2.3")

    def test_exact_main_push_gate_latest_attempt(self):
        commit = "a" * 40
        rows = [{"name": n, "head_sha": commit, "event": "push", "head_branch": "main", "id": i, "status": "completed", "conclusion": "success"} for i, n in enumerate(("ci", "govulncheck"))]
        with patch.object(release, "output", return_value=json.dumps({"workflow_runs": rows})):
            release.checks(commit)
        bad = dict(rows[0], id=99, status="in_progress", conclusion=None)
        for changes in (rows + [bad], [dict(r, event="pull_request") for r in rows], [dict(r, head_sha="b" * 40) for r in rows]):
            with patch.object(release, "output", return_value=json.dumps({"workflow_runs": changes})):
                with self.assertRaises(ValueError): release.checks(commit)

    def test_signed_annotated_tag_required(self):
        commit = "a" * 40
        ref = {"object": {"type": "tag", "sha": "b" * 40}}
        tag = {"tag": "v1.2.3", "object": {"type": "commit", "sha": commit}, "verification": {"verified": True}}
        with patch.object(release, "output", side_effect=[json.dumps(ref), json.dumps(tag)]):
            self.assertEqual(release.tag_target("v1.2.3"), commit)
        for bad in (dict(tag, verification={"verified": False}), dict(tag, tag="v9.9.9"), dict(tag, object={"type": "tree", "sha": commit})):
            with patch.object(release, "output", side_effect=[json.dumps(ref), json.dumps(bad)]):
                with self.assertRaises(ValueError): release.tag_target("v1.2.3")
        with patch.object(release, "output", return_value=json.dumps({"object": {"type": "commit", "sha": commit}})):
            with self.assertRaises(ValueError): release.tag_target("v1.2.3")

    def test_actual_binary_metadata_must_match_receipt(self):
        with tempfile.TemporaryDirectory() as dest:
            root = Path(dest); self.fixtures(root)
            with patch.object(release, "actual_build_info", return_value={"Settings": []}):
                with self.assertRaisesRegex(ValueError, "build-info differs"):
                    release.verify_assets("1.2.3", root)

    def test_no_dev_version_can_be_tagged(self):
        for v in ("1.2.3-dev", "v1.2.3", "../other", "01.2.3"):
            with self.assertRaises(ValueError): release.stable(v)
        self.assertEqual(release.stable("1.2.3"), "1.2.3")


if __name__ == "__main__":
    unittest.main()
