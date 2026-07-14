"""Runtime-inert C1.3 native-parser package.

Native artifacts are child-internal inputs to the later C1.4 canonicalizer.
They are not MMCP success bodies and must never be staged as Canonical IR.
Parser modules are intentionally not imported here: the isolated child loads
them lazily only after the process-group and Seccomp handshake.
"""
