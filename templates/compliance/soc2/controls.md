# SOC 2 — Trust Services Criteria control checklist

> Scaffolded by `adb compliance scaffold soc2`. Attest each control (owner, date,
> evidence link). `adb audit security` reports the deterministic controls; the
> rest are manual attestations tracked here.

## Security (Common Criteria)

- [ ] **CC6.1 — Logical access controls.** Least-privilege access to systems and
  data is enforced and reviewed. _Owner: ___  Evidence: ____
- [ ] **CC6.6 — Secret management.** Secrets are never committed; a secret scanner
  gates commits/pushes. _(deterministic: `adb audit security` → `secret-scanning`)_
- [ ] **CC6.7 — Encryption in transit & at rest.** _Owner: ___  Evidence: ____
- [ ] **CC7.2 — Monitoring & alerting.** Anomalies are detected and alerted.
- [ ] **CC7.3 — Incident response.** A documented, tested incident-response plan.
- [ ] **CC8.1 — Change management.** Changes are reviewed, tested, and traceable
  (ADRs via `adb adr`, PR review, CI gates).

## Availability

- [ ] **A1.1 — Capacity & SLOs.** Reliability objectives are defined and tracked.
  _(deterministic: `adb audit security` → `slo-defined`; set via `adb slo set`)_
- [ ] **A1.2 — Backup & recovery.** _Owner: ___  Evidence: ____
