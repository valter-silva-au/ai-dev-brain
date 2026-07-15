# HIPAA — Security Rule safeguard checklist

> Scaffolded by `adb compliance scaffold hipaa`. Applies when handling ePHI.
> Attest each safeguard (owner, date, evidence link). Deterministic hygiene
> controls are checked by `adb audit security`; the rest are manual.

## Administrative safeguards (§164.308)

- [ ] **Risk analysis & management.** A documented ePHI risk assessment.
- [ ] **Workforce access management.** Least-privilege access to ePHI, reviewed.
- [ ] **Contingency plan.** Backup, disaster-recovery, and emergency-mode plans.

## Technical safeguards (§164.312)

- [ ] **Access control.** Unique user IDs, automatic logoff, encryption/decryption.
- [ ] **Audit controls.** Activity on systems with ePHI is logged and reviewed.
- [ ] **Integrity.** ePHI is protected from improper alteration/destruction.
- [ ] **Transmission security.** Encryption in transit; no secrets in the repo.
  _(deterministic: `adb audit security` → `secret-scanning`, `env-gitignored`)_

## Organizational

- [ ] **Business Associate Agreements (BAAs)** are in place with all processors.
- [ ] **Reliability objectives (SLOs)** for ePHI systems are defined.
  _(deterministic: `adb audit security` → `slo-defined`)_
