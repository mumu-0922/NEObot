#!/usr/bin/env python3
"""Offline tests for the Agent Runner local-test host manager."""

from __future__ import annotations

import hashlib
import io
import json
import tarfile
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest import mock

import agent_runner_local_test as subject

PROJECT_DIR = Path(__file__).resolve().parents[1]
LOCK_PATH = PROJECT_DIR / "config" / "agent-runner" / "local-test-toolchain.lock.json"


class LocalTestManagerTests(unittest.TestCase):
    def test_lock_is_strict_and_local_only(self) -> None:
        lock = subject.load_toolchain_lock(LOCK_PATH)
        self.assertEqual(
            {item.name for item in lock.artifacts}, subject.EXPECTED_ARTIFACTS
        )
        self.assertEqual(set(lock.apt_packages), subject.lock_package_allowlist())
        self.assertTrue(lock.fingerprint.startswith("sha256:"))
        value = json.loads(LOCK_PATH.read_text(encoding="utf-8"))
        value["evidenceClass"] = "production"
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "lock.json"
            path.write_text(json.dumps(value), encoding="utf-8")
            with self.assertRaisesRegex(
                subject.LocalTestError, "TOOLCHAIN_LOCK_INVALID"
            ):
                subject.load_toolchain_lock(path)

    def test_lock_rejects_package_and_artifact_pin_drift(self) -> None:
        baseline = json.loads(LOCK_PATH.read_text(encoding="utf-8"))
        candidates = []
        package_drift = json.loads(json.dumps(baseline))
        package_drift["aptPackages"][-1] = "zlib1g-dev"
        candidates.append(package_drift)
        digest_drift = json.loads(json.dumps(baseline))
        digest_drift["artifacts"][0]["sha256"] = "sha256:" + "0" * 64
        candidates.append(digest_drift)
        version_drift = json.loads(json.dumps(baseline))
        version_drift["artifacts"][2]["version"] = "1.29.0"
        candidates.append(version_drift)
        with tempfile.TemporaryDirectory() as directory:
            for index, value in enumerate(candidates):
                path = Path(directory) / f"lock-{index}.json"
                path.write_text(json.dumps(value), encoding="utf-8")
                with self.assertRaisesRegex(
                    subject.LocalTestError, "TOOLCHAIN_LOCK_INVALID"
                ):
                    subject.load_toolchain_lock(path)

    def test_duplicate_lock_key_is_rejected(self) -> None:
        raw = LOCK_PATH.read_text(encoding="utf-8").replace(
            '"evidenceClass": "local_test",',
            '"evidenceClass": "local_test", "evidenceClass": "local_test",',
            1,
        )
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "lock.json"
            path.write_text(raw, encoding="utf-8")
            with self.assertRaisesRegex(subject.LocalTestError, "DUPLICATE_JSON_KEY"):
                subject.load_toolchain_lock(path)

    def test_wsl_conf_edit_is_preserving_and_idempotent(self) -> None:
        baseline = b'[user]\ndefault=mumu\n\n[boot]\ncommand="cd ~"\n'
        expected = b'[user]\ndefault=mumu\n\n[boot]\ncommand="cd ~"\nsystemd=true\n'
        rendered = subject.render_wsl_conf(baseline)
        self.assertEqual(rendered, expected)
        self.assertEqual(subject.render_wsl_conf(rendered), expected)
        replaced = subject.render_wsl_conf(b"[boot]\nsystemd=false\ncommand=x\n")
        self.assertEqual(replaced, b"[boot]\nsystemd=true\ncommand=x\n")
        with self.assertRaisesRegex(subject.LocalTestError, "WSL_CONF_INVALID"):
            subject.render_wsl_conf(b"[boot]\nsystemd=true\nsystemd=false\n")

    def test_subordinate_id_overlap_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "subuid"
            path.write_text("mumu:100000:65536\nother:120000:65536\n", encoding="utf-8")
            with self.assertRaisesRegex(
                subject.LocalTestError, "SUBORDINATE_IDS_OVERLAP"
            ):
                subject.parse_subid(path, "mumu")
            path.write_text("mumu:100000:65536\nother:165536:65536\n", encoding="utf-8")
            self.assertEqual(subject.parse_subid(path, "mumu"), (100000, 65536))

    def test_install_root_excludes_project_runtime_names(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            home = Path(directory)
            safe = subject.validate_install_root(home / ".local" / "agent", home)
            self.assertTrue(str(safe).startswith(str(home)))
            for name in subject.PROTECTED_PROJECT_NAMES:
                with self.assertRaisesRegex(
                    subject.LocalTestError, "INSTALL_ROOT_UNSAFE"
                ):
                    subject.validate_install_root(home / name / "agent", home)
            with self.assertRaisesRegex(subject.LocalTestError, "INSTALL_ROOT_UNSAFE"):
                subject.validate_install_root(Path("/tmp/outside"), home)

    def test_archive_traversal_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            archive = root / "bad.tar.gz"
            with tarfile.open(archive, "w:gz") as output:
                body = b"escape"
                header = tarfile.TarInfo("../escape")
                header.size = len(body)
                output.addfile(header, io.BytesIO(body))
            with self.assertRaisesRegex(subject.LocalTestError, "ARCHIVE_INVALID"):
                subject.safe_extract_tar(archive, root / "extract")

            valid = root / "valid.tar.gz"
            with tarfile.open(valid, "w:gz") as output:
                body = b"safe"
                header = tarfile.TarInfo("source/file.txt")
                header.size = len(body)
                output.addfile(header, io.BytesIO(body))
            subject.safe_extract_tar(valid, root / "valid-extract")
            self.assertEqual(
                (root / "valid-extract" / "source" / "file.txt").read_bytes(), body
            )

    def test_download_hash_drift_and_redirect_allowlist(self) -> None:
        class Response(io.BytesIO):
            def __init__(self, body: bytes, url: str) -> None:
                super().__init__(body)
                self.url = url

            def geturl(self) -> str:
                return self.url

            def __enter__(self) -> "Response":
                return self

            def __exit__(self, *_: object) -> None:
                self.close()

        body = b"pinned"
        digest = "sha256:" + hashlib.sha256(body).hexdigest()
        artifact = subject.ArtifactLock(
            "podman-source", "6.1.0", "https://github.com/source", len(body), digest
        )

        class Opener:
            def __init__(self, response: Response) -> None:
                self.response = response

            def open(self, *_: object, **__: object) -> Response:
                return self.response

        with tempfile.TemporaryDirectory() as directory:
            destination = Path(directory) / "artifact"
            with mock.patch.object(
                subject.urllib.request,
                "build_opener",
                return_value=Opener(
                    Response(body, "https://codeload.github.com/source")
                ),
            ):
                subject.download_artifact(artifact, destination)
            self.assertEqual(destination.read_bytes(), body)
            drift = subject.ArtifactLock(
                artifact.name,
                artifact.version,
                artifact.url,
                artifact.size,
                "sha256:" + "0" * 64,
            )
            with (
                mock.patch.object(
                    subject.urllib.request,
                    "build_opener",
                    return_value=Opener(
                        Response(body, "https://codeload.github.com/source")
                    ),
                ),
                self.assertRaisesRegex(subject.LocalTestError, "DOWNLOAD_HASH_DRIFT"),
            ):
                subject.download_artifact(drift, Path(directory) / "drift")
        handler = subject.PinnedDownloadRedirectHandler()
        request = subject.urllib.request.Request("https://github.com/source")
        with self.assertRaisesRegex(
            subject.LocalTestError, "DOWNLOAD_REDIRECT_REJECTED"
        ):
            handler.redirect_request(
                request,
                None,
                302,
                "redirect",
                {},
                "http://127.0.0.1/internal",
            )

    def test_release_publish_is_one_complete_directory(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            parent = Path(directory)
            release = parent / "staging" / "release"
            release.parent.mkdir(mode=0o700)
            release.mkdir(mode=0o700)
            (release / "bin").mkdir(mode=0o700)
            (release / "config").mkdir(mode=0o700)
            (release / "bin" / "podman").write_text("wrapper", encoding="utf-8")
            (release / "config" / "containers.conf").write_text(
                "[engine]\n", encoding="utf-8"
            )
            root = parent / "installed"
            subject._publish_toolchain_release(release, root, {"test": True})
            self.assertFalse(release.exists())
            self.assertTrue((root / "bin" / "podman").is_file())
            self.assertTrue((root / "config" / "containers.conf").is_file())
            self.assertTrue((root / "storage").is_dir())
            self.assertTrue((root / "toolchain-receipt.json").is_file())

    def test_toolchain_receipt_rejects_installed_file_drift(self) -> None:
        lock = subject.load_toolchain_lock(LOCK_PATH)
        relative_files = [
            "bin/agent-runtime-local-test-smoke",
            "bin/conmon",
            "bin/crun",
            "bin/neo-skill-local-test-workload",
            "bin/podman",
            "bin/podman-engine",
            "config/containers.conf",
            "config/seccomp-agent-v1.json",
            "config/storage.conf",
        ]
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory) / "installed"
            root.mkdir(mode=0o700)
            (root / "bin").mkdir(mode=0o700)
            (root / "config").mkdir(mode=0o700)
            (root / "storage").mkdir(mode=0o700)
            for relative in relative_files:
                path = root / relative
                path.write_bytes(relative.encode("utf-8"))
                path.chmod(0o755 if relative.startswith("bin/") else 0o600)
            receipt = {
                "schemaVersion": subject.RECEIPT_SCHEMA,
                "evidenceClass": subject.EVIDENCE_CLASS,
                "productionEligible": False,
                "lockFingerprint": lock.fingerprint,
                "host": subject.EXPECTED_HOST,
                "installRootFingerprint": subject.sha256_bytes(
                    str(root).encode("utf-8")
                ),
                "builtAt": "2026-08-15T00:00:00Z",
                "files": subject._file_inventory(root, relative_files),
            }
            subject.atomic_write(
                root / "toolchain-receipt.json",
                json.dumps(receipt).encode("utf-8"),
                0o600,
            )
            subject.validate_toolchain_receipt(lock, root)
            (root / "bin" / "crun").write_bytes(b"drift")
            with self.assertRaisesRegex(subject.LocalTestError, "TOOLCHAIN_FILE_DRIFT"):
                subject.validate_toolchain_receipt(lock, root)

    def test_generated_podman_configuration_has_no_patch_markers(self) -> None:
        root = Path("/home/test/.local/agent")
        containers = subject._containers_config(root)
        storage = subject._storage_config(root, 1000)
        self.assertNotIn("\n+", containers + storage)
        self.assertIn('cgroup_manager = "systemd"', containers)
        self.assertIn(f'graphroot = "{root}/storage"', storage)
        self.assertEqual(
            subject._podman_wrapper(root, "sha256:" + "0" * 64).count(
                "CONTAINERS_CONF="
            ),
            1,
        )

    def test_status_baseline_is_read_only_and_nonproduction(self) -> None:
        lock = subject.load_toolchain_lock(LOCK_PATH)
        before = Path("/etc/wsl.conf").read_bytes()
        status = subject.host_status(lock, subject.default_install_root())
        self.assertIn(
            status["code"],
            {
                "SYSTEM_BOOTSTRAP_REQUIRED",
                "RESTART_REQUIRED",
                "TOOLCHAIN_INSTALL_REQUIRED",
                "LOCAL_SMOKE_READY",
            },
        )
        self.assertEqual(status["evidenceClass"], "local_test")
        self.assertFalse(status["productionEligible"])
        self.assertEqual(Path("/etc/wsl.conf").read_bytes(), before)

    def test_bootstrap_refuses_nonroot_without_running_apt(self) -> None:
        lock = subject.load_toolchain_lock(LOCK_PATH)
        with (
            mock.patch.object(subject.os, "geteuid", return_value=1000),
            mock.patch.object(subject, "run_checked") as command,
        ):
            with self.assertRaisesRegex(
                subject.LocalTestError, "ROOT_BOOTSTRAP_REQUIRED"
            ):
                subject.bootstrap_system(lock, "mumu")
            command.assert_not_called()

    def test_bootstrap_rejects_unsupported_host(self) -> None:
        lock = subject.load_toolchain_lock(LOCK_PATH)
        account = SimpleNamespace(pw_uid=1000, pw_gid=1000)
        with (
            tempfile.TemporaryDirectory() as directory,
            mock.patch.object(subject.os, "geteuid", return_value=0),
            mock.patch.object(subject.pwd, "getpwnam", return_value=account),
            mock.patch.object(
                subject,
                "parse_os_release",
                return_value={"ID": "debian", "VERSION_ID": "12"},
            ),
        ):
            with self.assertRaisesRegex(subject.LocalTestError, "HOST_UNSUPPORTED"):
                subject.bootstrap_system(
                    lock,
                    "mumu",
                    Path(directory) / "state",
                    wsl_conf=Path(directory) / "wsl.conf",
                )

    def test_bootstrap_resumes_after_interrupted_package_install(self) -> None:
        lock = subject.load_toolchain_lock(LOCK_PATH)
        account = SimpleNamespace(pw_uid=1000, pw_gid=1000)
        installed: set[str] = set()

        def package_installed(package: str) -> bool:
            return package in installed

        def complete_command(arguments: list[str], **_: object) -> object:
            if arguments[:2] == ["/usr/bin/apt-get", "install"]:
                installed.update(
                    package
                    for package in arguments
                    if package in subject.EXPECTED_APT_PACKAGES
                )
            return SimpleNamespace(returncode=0, stdout="")

        common = (
            mock.patch.object(subject.os, "geteuid", return_value=0),
            mock.patch.object(subject.pwd, "getpwnam", return_value=account),
            mock.patch.object(
                subject,
                "parse_os_release",
                return_value={"ID": "ubuntu", "VERSION_ID": "22.04"},
            ),
            mock.patch.object(subject, "is_wsl2", return_value=True),
            mock.patch.object(subject, "host_architecture", return_value="amd64"),
            mock.patch.object(subject, "pid1_name", return_value="init"),
            mock.patch.object(
                subject, "package_installed", side_effect=package_installed
            ),
        )
        with tempfile.TemporaryDirectory() as directory:
            base = Path(directory)
            state_root = base / "state"
            wsl_conf = base / "wsl.conf"
            wsl_conf.write_text("[user]\ndefault=mumu\n", encoding="utf-8")
            for patcher in common:
                patcher.start()
            self.addCleanup(lambda: [patcher.stop() for patcher in reversed(common)])
            with (
                mock.patch.object(
                    subject,
                    "run_checked",
                    side_effect=[
                        SimpleNamespace(returncode=0),
                        subject.LocalTestError("COMMAND_FAILED"),
                    ],
                ),
                self.assertRaisesRegex(subject.LocalTestError, "COMMAND_FAILED"),
            ):
                subject.bootstrap_system(lock, "mumu", state_root, wsl_conf=wsl_conf)
            state = json.loads(
                (state_root / "bootstrap.json").read_text(encoding="utf-8")
            )
            self.assertEqual(state["status"], "applying")
            self.assertNotIn("systemd=true", wsl_conf.read_text(encoding="utf-8"))
            with mock.patch.object(
                subject, "run_checked", side_effect=complete_command
            ):
                result = subject.bootstrap_system(
                    lock, "mumu", state_root, wsl_conf=wsl_conf
                )
            self.assertEqual(result, "RESTART_REQUIRED")
            self.assertIn("systemd=true", wsl_conf.read_text(encoding="utf-8"))
            state = json.loads(
                (state_root / "bootstrap.json").read_text(encoding="utf-8")
            )
            self.assertEqual(state["status"], "applied")

    def test_rollback_refuses_wsl_configuration_drift(self) -> None:
        before = b"[user]\ndefault=mumu\n"
        after = subject.render_wsl_conf(before)
        state = {
            "schemaVersion": subject.BOOTSTRAP_SCHEMA,
            "evidenceClass": subject.EVIDENCE_CLASS,
            "productionEligible": False,
            "status": "applied",
            "operatorUser": "mumu",
            "createdAt": "2026-08-15T00:00:00Z",
            "wslConfExisted": True,
            "wslConfBeforeSha256": subject.sha256_bytes(before),
            "wslConfAfterSha256": subject.sha256_bytes(after),
            "packagesRequested": list(subject.EXPECTED_APT_PACKAGES),
            "packagesInitiallyMissing": [],
        }
        with tempfile.TemporaryDirectory() as directory:
            base = Path(directory)
            state_root = base / "state"
            state_root.mkdir(mode=0o700)
            (state_root / "wsl.conf.before").write_bytes(before)
            (state_root / "bootstrap.json").write_text(
                json.dumps(state), encoding="utf-8"
            )
            wsl_conf = base / "wsl.conf"
            wsl_conf.write_text("[boot]\nsystemd=false\n", encoding="utf-8")
            with (
                mock.patch.object(subject.os, "geteuid", return_value=0),
                mock.patch.object(subject, "run_checked") as command,
                self.assertRaisesRegex(
                    subject.LocalTestError, "ROLLBACK_WSL_CONF_DRIFT"
                ),
            ):
                subject.rollback_system(state_root, wsl_conf=wsl_conf)
            command.assert_not_called()

    def test_no_password_transport_or_autoremove_exists(self) -> None:
        source = "\n".join(
            path.read_text(encoding="utf-8")
            for path in (
                Path(subject.__file__),
                PROJECT_DIR / "scripts" / "manage-agent-runner-local-test.py",
                PROJECT_DIR / "scripts" / "agent-runner-local-test.sh",
            )
        )
        self.assertNotIn("sudo -S", source)
        self.assertNotIn("--stdin", source)
        self.assertNotIn("autoremove", source)
        self.assertNotIn("docker", source.lower())


if __name__ == "__main__":
    unittest.main()
