"""Bounded-label Prometheus metrics for durable mechanics."""

from __future__ import annotations

from dataclasses import dataclass

from prometheus_client import CollectorRegistry, Counter, Gauge, Histogram


@dataclass(slots=True)
class Metrics:
    """Worker metrics without tenant, document, event, or token labels."""

    registry: CollectorRegistry
    loop_iterations: Counter
    outbox_claims: Counter
    outbox_results: Counter
    job_claims: Counter
    job_results: Counter
    function_seconds: Histogram
    worker_lock: Gauge
    redis_connected: Gauge
    dispatch_enabled: Gauge
    active_job: Gauge

    @classmethod
    def create(cls, registry: CollectorRegistry | None = None) -> Metrics:
        """Create metrics in an isolated or supplied registry."""
        target = registry or CollectorRegistry()
        return cls(
            registry=target,
            loop_iterations=Counter(
                "rag_worker_loop_iterations_total",
                "Worker loop triggers",
                ("kind",),
                registry=target,
            ),
            outbox_claims=Counter(
                "rag_worker_outbox_claims_total",
                "Outbox claim outcomes",
                ("outcome",),
                registry=target,
            ),
            outbox_results=Counter(
                "rag_worker_outbox_results_total",
                "Outbox orchestration outcomes",
                ("outcome",),
                registry=target,
            ),
            job_claims=Counter(
                "rag_worker_job_claims_total",
                "Job claim outcomes",
                ("outcome",),
                registry=target,
            ),
            job_results=Counter(
                "rag_worker_job_results_total",
                "Job orchestration outcomes",
                ("outcome",),
                registry=target,
            ),
            function_seconds=Histogram(
                "rag_worker_db_function_seconds",
                "Stored function latency",
                ("function",),
                registry=target,
                buckets=(0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5),
            ),
            worker_lock=Gauge(
                "rag_worker_advisory_lock",
                "Whether this process owns the single-worker advisory lock",
                registry=target,
            ),
            redis_connected=Gauge(
                "rag_worker_redis_wakeup_connected",
                "Whether the optional Redis wake subscriber is connected",
                registry=target,
            ),
            dispatch_enabled=Gauge(
                "rag_worker_dispatch_enabled",
                "Whether durable dispatch is enabled",
                registry=target,
            ),
            active_job=Gauge(
                "rag_worker_active_job",
                "Whether the globally single job slot is occupied",
                registry=target,
            ),
        )
