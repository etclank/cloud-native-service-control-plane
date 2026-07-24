# Project Baseline Closeout

**Completion date:** 24 July 2026  
**Authoritative branch:** `main`  
**Scope:** complete for the current portfolio, interview-preparation, and
lightweight application-hosting needs

The H8 implementation was integrated into `main` at
`521a929fc6c6d17e36022a737505d37979506f27`. The foreground Kubernetes tunnel
procedure was then recorded at
`9bd925a97093f93000111266a375f3cc494d2b3d`. The `main` commit containing this
closeout record is the authoritative repository baseline; immutable live
revisions remain recorded separately in the H7/H8 evidence.

## Completion Definition

The Cloud-Native Service Control Plane has an implemented, validated baseline
through H8. It is a small single-node portfolio platform emphasizing
reproducibility, security boundaries, immutable delivery, GitOps, and
operational learning.

This closeout does not claim high availability, enterprise production
readiness, multi-cluster operation, automatic disaster recovery, or capacity
for arbitrary workloads. Former H9–H12 ideas are
[optional improvements](optional-improvements.md), not mandatory construction
phases.

## Implemented Capability Inventory

- Ubuntu 24.04 LTS on one Hetzner CX23 VM, running pinned single-node K3s.
- Restricted SSH and a loopback SSH tunnel for private Kubernetes API access.
- Traefik, cert-manager, production HTTPS, and permanent HTTP-to-HTTPS
  redirection for approved public endpoints.
- A Kubebuilder Go operator and `ManagedService` CRD with idempotent child
  reconciliation, immutable image validation, ownership, and status.
- A bearer-authenticated Go control-plane API with a fixed `applications`
  namespace and bounded `demo-http` lifecycle operations.
- A managed Go demonstration workload with probes, resource limits, hardened
  Pod settings, internal metrics, and owner-driven cleanup.
- Immutable GitHub Actions and digest-pinned GHCR runtime images.
- Restricted AppProjects, private Argo CD administration, and manual GitOps
  synchronization.
- Least-privilege ServiceAccounts/RBAC and workload-specific NetworkPolicies.
- A private OpenTelemetry Collector with selector-safe OTLP ingress and only a
  `nop` exporter.
- A bounded standalone Prometheus, reduced kube-state-metrics, 3Gi persistent
  storage, and six accepted healthy scrape jobs.
- Tested manual synchronization, rollback, Prometheus persistence, and target
  rediscovery procedures.

## Evidence and Validation

The authoritative live H8 evidence is
[H8 Observability Closeout](h8-observability-closeout.md). It records all
three Argo CD Applications as `Synced` and `Healthy`, Ready platform and
observability workloads with zero restarts, exactly six healthy Prometheus
jobs, a Bound 3Gi PVC, retained TSDB history after Pod recreation, and the
absence of kubelet/cAdvisor access and node RBAC.

The [H7 closeout](h7-platform-deployment-closeout.md) records API
authentication, production TLS, `ManagedService` lifecycle behavior, drift
correction, garbage collection, and external exposure checks. Repository
tests, lint, builds, deterministic rendering, dependency locks, immutable
reference checks, and credential scans protect the committed configuration.

These records are point-in-time evidence. Routine health and change procedures
remain in the [operator guide](operator-guide.md) and
[Argo CD rollback runbook](argocd-sync-rollback-runbook.md).

## Security and Network Boundaries

- Public ingress is limited to approved HTTP/HTTPS application endpoints.
- SSH is source-restricted; Kubernetes and Argo CD administration remain
  private.
- Runtime images use reviewed SHA-256 digests; workflows use immutable action
  references.
- Credentials, kubeconfigs, registry tokens, API bearer tokens, and TLS
  private keys remain outside Git.
- Operator, API, observability, and managed workloads use bounded identities,
  RBAC, Pod security settings, and NetworkPolicies.
- Prometheus, Collector receivers, metrics endpoints, and administration
  interfaces have no public Ingress.

This is defense-in-depth for a portfolio environment, not a certification or
formal production assurance claim.

## Capacity and Storage Context

The live node has 2 vCPU, 4GB RAM, and a 40GB SSD. H8 closeout measured about
6% node CPU and 58% memory after the accepted observability deployment. The
observability components used about 75Mi at that observation point.

Prometheus has explicit CPU/memory bounds, 72-hour and 2GB retention limits,
and a 3Gi local-path PVC. Local-path data shares the single node's failure
domain and the PVC reclaim policy is `Delete`. Measurements are not permanent
capacity guarantees: every new workload requires requests/limits and
before/after node measurements.

## Known Limitations

- One VM and one Kubernetes node; no node, zone, or control-plane redundancy.
- Local-path storage is not highly available.
- No automated backup/restore system or tested full-cluster disaster recovery.
- No production PostgreSQL, Redis, SmartEnergy stack, or other stateful
  application on this platform.
- No Grafana, Loki, Tempo, kubelet/cAdvisor collection, or production workload
  OTLP export.
- Argo CD synchronization is deliberately manual.
- The `ManagedService` API accepts only the approved `demo-http` template; it
  is not a general-purpose arbitrary-image deployment API.
- Capacity and public-traffic behavior are validated only for the bounded
  portfolio workload.

## Acceptance Matrix

| Capability | Status | Evidence | Limitation |
| --- | --- | --- | --- |
| Kubernetes runtime | Complete | Pinned Ready K3s node in H7/H8 closeouts | Single node |
| Operator and reconciliation | Complete | H7 lifecycle and drift tests; repository envtest | One approved template |
| Control-plane API | Complete | Auth, CRUD, readiness, TLS evidence | Fixed namespace and resource type |
| Managed demonstration workload | Complete | `portfolio-demo` Ready and Available | Demonstration workload only |
| GitOps delivery | Complete | Three restricted manual-sync Applications | No automated sync |
| Ingress and TLS | Complete | Production certificate, redirect, rate limit | Approved hostnames only |
| Private administrative access | Complete | SSH tunnel and private Argo procedures | Depends on one host and trusted admin path |
| RBAC | Complete | Restricted AppProjects and workload roles | Not a multi-tenant authorization system |
| NetworkPolicy | Complete | Default-deny and target-specific live proof | Depends on current CNI enforcement |
| Metrics collection | Complete | Six accepted Prometheus jobs healthy | No kubelet/cAdvisor |
| Prometheus persistence | Complete | Bound PVC and Pod-recreation proof | Local-path, `Delete` reclaim policy |
| Application deployment readiness | Complete | Managed demo plus documented GitOps route | New apps require explicit package/policy review |
| Rollback procedures | Complete | Tested Argo rollback and H8 dry-run boundary | Destructive data recovery remains separate |
| Documentation and runbooks | Complete | Build, operator, deployment, rollback, and closeout guides | Optional improvements are not operating commitments |

## Closeout Decision

H1–H8 are complete and the implemented platform baseline is formally closed
as of 24 July 2026. New work may be accepted as an independent optional
improvement with its own scope and validation; no mandatory H9 or later phase
remains.
