"""Static promotion fences for the C1.2 Compose/image boundary."""

from __future__ import annotations

import tomllib
from pathlib import Path

from mm_chat_rag import handlers

_RAG_ROOT = Path(__file__).parents[2]
_MM_CHAT_ROOT = _RAG_ROOT.parent


def test_parser_console_scripts_are_isolated_from_rag_dispatch() -> None:
    project = tomllib.loads((_RAG_ROOT / "pyproject.toml").read_text(encoding="utf-8"))
    scripts = project["project"]["scripts"]

    assert scripts["parser-sidecar"] == "mm_chat_rag.offline_parser.sidecar:main"
    assert scripts["parser-harness-smoke"] == "mm_chat_rag.offline_parser.smoke:main"
    assert handlers.DISPATCH_REGISTRY == {}
    assert handlers.JOB_HANDLER_REGISTRY == {}


def test_parser_image_and_compose_keep_uid_network_and_resource_fences() -> None:
    dockerfile = (_RAG_ROOT / "Dockerfile").read_text(encoding="utf-8")
    compose = (_MM_CHAT_ROOT / "compose.single-server.yml").read_text(encoding="utf-8")

    assert "--uid 10002 --gid rag" in dockerfile
    assert "parser-sidecar:" in compose
    assert 'profiles: ["parser-c1"]' in compose
    assert 'user: "10002:10001"' in compose
    assert "seccomp=./rag/src/mm_chat_rag/offline_parser/profiles/" in compose
    assert "parser-sidecar.json" in compose
    assert "mem_limit: 768m" in compose
    assert "pids_limit: 64" in compose
    assert "network_mode: none" in compose
    assert (
        "/run/mm-chat-parser-harness:rw,noexec,nosuid,nodev,"
        "size=512m,nr_inodes=20000,mode=0700,uid=10001,gid=10001"
    ) in compose


def test_parser_sidecar_service_does_not_delegate_pid_one_to_an_init() -> None:
    compose = (_MM_CHAT_ROOT / "compose.single-server.yml").read_text(encoding="utf-8")
    sidecar = compose.split("  parser-sidecar:\n", 1)[1].split(
        "\n  parser-harness-smoke:",
        1,
    )[0]

    assert "init: true" not in sidecar
    assert 'entrypoint: ["parser-sidecar"]' in sidecar
    assert "network_mode: none" in sidecar
