# Documentation

Start with the [project overview](../README.md), then choose a guide:

| Document | Purpose |
| --- | --- |
| [Final portfolio closeout](project-final-closeout.md) | Current September 2026 platform state, limitations, and Project 2 handoff |
| [Security model](security-model.md) | API contract, trust boundaries, and hardening limits |
| [Application deployment](application-deployment-guide.md) | ManagedService and separate GitOps application onboarding |
| [Real-application hosting contract](application-hosting-contract.md) | Platform, application, and administrator ownership for dedicated application onboarding |
| [Operator guide](operator-guide.md) | Administrative access and operational procedures |
| [Argo CD runbook](argocd-sync-rollback-runbook.md) | Manual sync, rollback, and credential rotation |
| [Build guide](cloud-native-service-control-plane-build-guide.md) | Historical environment reconstruction |
| [Infrastructure context](infrastructure-context.md) | Decisions and single-node trade-offs |
| [Optional improvements](optional-improvements.md) | Unimplemented expansion and hardening work |
| [Public review](public-review.md) | Validation commands, outcomes, and review limits |
| [Third-party provenance](third-party.md) | Vendored manifests, dependencies, and licensing scope |

## Historical evidence

[Project baseline closeout](project-closeout.md),
[platform deployment](h7-platform-deployment-closeout.md), and
[observability closeout](h8-observability-closeout.md) are older point-in-time
records. Other `h8-*` files preserve design decisions, rejected approaches, and
bounded network proofs. Earlier slice statements must be read in that historical
context; current status is recorded in the final portfolio closeout, while
current configuration is defined by source and manifests.

These records do not establish current uptime, certificate validity, firewall
state, or disaster recovery. Environment-specific commands require your own
credentials and review. Example hostnames are deployment identities, not a
promise of a publicly available demo. Sanitized addresses and usernames in
Markdown are placeholders; never copy them into a real deployment unchanged.
