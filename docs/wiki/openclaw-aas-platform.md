# OpenClaw-aaS Platform Architecture

## Overview

OpenClaw-as-a-Service (OpenClaw-aaS) is the multi-tenant platform strategy for transforming the Propella AI real estate assistant from a single-product deployment into a platform where each real estate agency gets their own configurable AI assistant instance. The platform targets the Australian real estate market with a WA beachhead go-to-market strategy (TASK-00036, TASK-00037).

Key metrics: 4-tier pricing model, base case Year 1 WA: 42 paid agencies, $624K ARR, $729K total revenue, 93.6% gross margin. National TAM: approximately 18,000 agencies.

## Key Decisions

- **ECS Fargate SPOT over EC2 Graviton**: Scale-to-zero ($0 idle cost) is the core platform cost advantage. EC2 would waste $2,400/mo on idle compute at 100 tenants (K-00033, ADR-0003)
- **Hybrid container strategy**: One ECS container per agency, shared across users via OpenClaw per-sender session scoping. Cost: ~$150/mo for 20 agencies at 30% utilization (K-00034, ADR-0003)
- **Single Docker image with tier-based skill filtering**: All skills in one image; entrypoint reads tenant billing plan from DynamoDB and copies only allowed skills to EFS workspace at container boot (K-00035, ADR-0003)
- **Hybrid CRM integration**: MCP server inside ECS container for real-time conversational CRM access; Lambda CrmAdapter for async batch operations. VaultRE is the third CRM adapter after Rex and AgentBox (K-00036, ADR-0003)
- **Per-agency base + per-seat scaling pricing**: Agency pays base fee ($299-$3,999/mo) with included seats. Additional seats at volume-discounted rates ($49 to $29). Naturally pulls agencies toward higher tiers (K-00037, ADR-0003)
- **Single region to 200 tenants, multi-region at 200+**: Fargate quota increase is cheaper and simpler than multi-region. Defer multi-region complexity until geo-expansion demand exists (K-00038, ADR-0003)
- **Defer AML/CTF**: AU real estate agents are not designated reporting entities under AML/CTF Act 2006. Tranche 2 is signalled but not enacted. Building AML now is readiness, not a requirement (K-00039)
- **Bedrock Guardrails**: Already deployed and integrated (terraform/bedrock-guardrail.tf, dispatcher.py:890-923). Only need AU-specific PII patterns added (TFN, Medicare, BSB, passport) (K-00040)
- **WA beachhead GTM**: REIWA Technology Partner + PropTech Hub WA + Mandurah Tech Fest (Nov 2026) + direct Top 50 agent outreach + CRM vendor partnerships. REIWA has 1,350 members covering ~90% of WA agencies (K-00041)

## Learnings

- Approximately 70% of the Sovereign Agent research proposals were already implemented in the Propella codebase in more sophisticated form. Genuinely new capabilities needed: VaultRE CRM integration, CMA generation, AU PII guardrails. Deferred: AML/CTF, RAG (K-00042).
- The competitive landscape has no AU RE competitor in the autonomous + omnichannel quadrant. RiTA is predictive but has no conversation capability; Rex AI does content generation but has no channels; ActivePipe is email-only; generic chatbots lack RE context. Five defensible moats: sovereign architecture, Lobster workflows, persistent memory, multi-agent orchestration, 20+ RE skills (K-00044).
- Embeddable channel badges allow real estate website visitors to start conversations with the AI assistant via WhatsApp or Telegram. Each badge generates a deep link carrying property context (tenant ID, property ID) into the messaging session (K-00032).

## Pricing Tiers

| Tier | Monthly | Included Seats | Additional Seat |
|------|---------|---------------|-----------------|
| Solo | $299 | 1 | $49 |
| Professional | $599 | 3 | $39 |
| Agency | $1,499 | 10 | $29 |
| Enterprise | $3,999 | Custom | Negotiated |

## Related

- ADR-0003: OpenClaw-aaS Platform Architecture (full decision record)

---
*Sources: TASK-00036, TASK-00037*
