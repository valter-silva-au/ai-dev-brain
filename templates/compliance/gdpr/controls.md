# GDPR — data-protection control checklist

> Scaffolded by `adb compliance scaffold gdpr`. Attest each control (owner, date,
> evidence link). Deterministic hygiene controls are checked by
> `adb audit security`; the rest are manual attestations tracked here.

## Lawfulness & rights

- [ ] **Art. 6 — Lawful basis.** A lawful basis is recorded for each processing
  activity. _Owner: ___  Evidence: ____
- [ ] **Art. 15–22 — Data-subject rights.** Access, rectification, erasure,
  portability, and objection are supported operationally.
- [ ] **Art. 30 — Records of processing (RoPA).** A processing register is
  maintained. _Owner: ___  Evidence: ____

## Security & accountability

- [ ] **Art. 32 — Security of processing.** Encryption/pseudonymisation and access
  control are in place; secrets are never committed. _(deterministic:
  `adb audit security` → `secret-scanning`, `env-gitignored`)_
- [ ] **Art. 33/34 — Breach notification.** A 72-hour breach-notification process.
- [ ] **Art. 35 — DPIA.** A data-protection impact assessment for high-risk
  processing. _Owner: ___  Evidence: ____
- [ ] **Data retention.** Retention periods are defined and enforced.
