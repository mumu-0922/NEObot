# Staged production activation evidence

## Circularity in the final closure record

The G20.10 production-closure record requires the final bounded canary,
rotation, reboot, disaster-recovery and cleanup matrix. Requiring
`PROMOTION_READY` before the first control-plane connection would be circular:
the control-plane wiring is needed to generate later live evidence.

## Recommended contract

Introduce a strict, short-lived G21.0 activation record for the
`control_plane` stage. It binds:

- exact release commit and migration head `090`;
- operations-policy, Runner manifest and Runner binary SHA-256 values;
- target deployment and Runner identities;
- endpoint, client certificate and server CA fingerprints;
- an exact five-check live set: host isolation, clean-copy install, private
  mTLS probe, zero-inventory reconcile and rollback/Kill-Switch readiness;
- an authorization vector that permits control-plane maintenance only and
  explicitly denies Root Runs, Broker read/mutation, Child, Cron and Learning;
- a <=24-hour review window, approved reviewer fingerprint and zero orphan/
  Scratch residue.

A checked-in template remains held. Positive evidence is generated only in
temporary tests or on the target host. The evaluator derives READY/HELD/INVALID;
the record cannot self-assert readiness.

## Runtime revalidation

The control worker must not trust an environment boolean alone. It revalidates
the strict record and hashes of the mounted policy, release manifest, client
certificate and server CA before each cycle. The configured endpoint is bound
through a domain-separated SHA-256. Stale, template, drifted or widened records
stop control activity.
