---
title: Threat Model
owner:
date:
sources: []      # which inputs informed this artifact — the traceability record
lineage: "Public sources this template's structure is drawn from: STRIDE threat categories (spoofing, tampering, repudiation, information disclosure, denial of service, elevation of privilege) and OWASP guidance on data-flow-driven threat modelling and mitigation."
---

# Threat Model

<!-- Model the system as it is designed, not as you wish it were. The value is in
     the boundaries and the per-threat mitigations, not in the diagram. Do this
     while the design can still change — a threat model written after
     implementation only documents what you now have to live with.

     Scope it: a threat model for "the whole platform" produces nothing
     actionable. Model one system or one significant change. -->

## Scope and assumptions

<!-- What is in the model, what is out, and what you are taking on trust (the
     platform's isolation, the identity provider, the managed datastore's
     encryption). State each trusted component explicitly — an unexamined
     "assumed secure" dependency is the most common gap. -->

**In scope:**

**Out of scope:**

**Trusted (not analysed here), and why:**

## Assets and impact

<!-- What an attacker would want: data, credentials, compute, availability,
     reputation. For each, the worst realistic consequence of losing it. "All our
     data" is a weak answer — name the classes and their sensitivity. -->

| Asset | Sensitivity | Worst realistic impact if compromised |
|-------|-------------|---------------------------------------|
| | | |

## Actors

<!-- Legitimate roles plus the adversaries you are modelling: unauthenticated
     internet, authenticated user of another tenant, insider with production
     access, compromised dependency, compromised CI. Note the capability you are
     assuming for each — a model that only considers anonymous outsiders misses
     most real incidents. -->

| Actor | Legitimate or adversarial | Assumed capability |
|-------|---------------------------|--------------------|
| | | |

## Trust boundaries and data flows

<!-- Draw the system with the boundaries marked: every place data or control
     crosses from one trust level to another (internet to service, service to
     datastore, tenant to tenant, control plane to data plane, human to
     production). Threats live on the crossings, so enumerate the crossings
     before analysing anything. -->

```
<!-- data flow diagram with trust boundaries marked -->
```

| # | Flow (from → to) | Crosses boundary | Data carried | Authentication on this flow | Encryption |
|---|------------------|------------------|--------------|-----------------------------|------------|
| DF-1 | | | | | |

## STRIDE analysis

<!-- Work each flow or component against the six categories. Do not leave a
     category blank: either state the threat or state why the category does not
     apply here. "N/A" with no reason is the single most common way a real threat
     gets skipped. Mitigation must name a specific control, not a goal —
     "validate input" is weak; "reject requests whose tenant id does not match
     the token's tenant claim, enforced in middleware" is a mitigation. -->

### Spoofing — can an actor claim an identity that is not theirs?

| ID | Threat | Flow / component | Mitigation | Status (planned / built / accepted) |
|----|--------|------------------|------------|-------------------------------------|
| T-S1 | | | | |

### Tampering — can data or code be modified without authorisation?

| ID | Threat | Flow / component | Mitigation | Status |
|----|--------|------------------|------------|--------|
| T-T1 | | | | |

### Repudiation — can an action be taken without a reliable record of who did it?

| ID | Threat | Flow / component | Mitigation | Status |
|----|--------|------------------|------------|--------|
| T-R1 | | | | |

### Information disclosure — can data reach someone who should not see it?

<!-- Include the quiet channels: logs, error messages, metrics labels, caches,
     backups, support tooling, and cross-tenant leakage through shared keys. -->

| ID | Threat | Flow / component | Mitigation | Status |
|----|--------|------------------|------------|--------|
| T-I1 | | | | |

### Denial of service — can availability be removed cheaply?

| ID | Threat | Flow / component | Mitigation | Status |
|----|--------|------------------|------------|--------|
| T-D1 | | | | |

### Elevation of privilege — can an actor gain rights they were not granted?

| ID | Threat | Flow / component | Mitigation | Status |
|----|--------|------------------|------------|--------|
| T-E1 | | | | |

## Accepted risks

<!-- Threats you are knowingly not mitigating. Each needs the reason, the
     compensating control if any, and a named person who accepted it. An accepted
     risk with no named accepter is an unowned risk. -->

| Threat ID | Why accepted | Compensating control | Accepted by | Date | Revisit by |
|-----------|--------------|----------------------|-------------|------|------------|
| | | | | | |

## Detection and response

<!-- For the highest-severity threats: what signal would show it happening, where
     that signal is collected, who is paged, and what the first response step is.
     A mitigation you cannot detect failing is a hope. -->

| Threat ID | Detection signal | Alert destination | First response step |
|-----------|------------------|-------------------|---------------------|
| | | | |

## Follow-up actions

<!-- Mitigations that are not yet built, each as a tracked item with an owner.
     Link the adb task id so this file does not become the tracker. -->

| Action | Threat ID | Owner | Task id |
|--------|-----------|-------|---------|
| | | | |

## Decisions

<!-- Security posture decisions (an auth model, a tenancy isolation strategy, a
     chosen crypto boundary) belong in the ADR log: run `adb adr new` and
     reference the id here. -->

| ADR | Decision | Date |
|-----|----------|------|
| | | |
