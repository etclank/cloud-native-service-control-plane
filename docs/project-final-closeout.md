# Cloud-Native Service Control Plane — Final Portfolio Closeout

**Date:** 21 September 2026  
**Current repository revision:** `4be7d5a6122c3f948eb1352c172445a9942a88f2` (`feat: define real application hosting contract`)  
**Environment:** Single-node K3s v1.36.2+k3s1 on one Hetzner VM  
**Status:** Portfolio scope complete

This is the current Project 1 closeout. Earlier H7, H8, project-baseline, and
public-review records remain point-in-time evidence for their recorded dates
and revisions.

## Final status

Project 1 is complete for its intended portfolio scope. It is an operationally
validated portfolio platform with production-like controls: immutable delivery,
reviewed GitOps promotion, scoped authorization, network isolation, TLS, and
private observability.

The platform is intentionally single-node, non-HA, manually promoted, and
capacity-constrained. This closeout does not describe it as production-grade or
claim availability beyond the validation recorded here.

## Current architecture

```text
Hetzner VM
-> K3s
-> Traefik
-> cert-manager
-> Argo CD
-> control-plane API
-> ManagedService CRD and operator
-> managed demo
-> Prometheus, OpenTelemetry Collector, and kube-state-metrics
```

Independently released portfolio applications follow a separate path:

```text
application repository
-> CI and GHCR
-> immutable image digest
-> application-owned Kustomize package
-> restricted AppProject
-> commit-pinned Argo Application
-> dedicated namespace
```

SmartEnergy has only its platform prerequisites. Its Argo Application and
workloads have not been deployed.

## Core platform capabilities

- Go Kubernetes Operator and constrained `ManagedService` CRD.
- Desired-state reconciliation of owned Deployments and Services.
- Status conditions, ready replicas, internal endpoint, and observed generation.
- Bearer-authenticated Go control-plane API with bounded request handling.
- Scoped Kubernetes RBAC and separate workload identities.
- Single-node K3s on Hetzner with private administration through an SSH tunnel.
- Traefik ingress and cert-manager production TLS.
- Workload-specific NetworkPolicies and non-root container hardening.
- GitHub Actions and GHCR image publication.
- Immutable runtime image digests and commit-pinned Argo CD Applications.
- Manual synchronization, reviewed diffs, and restricted AppProjects.
- Persistent Prometheus, reduced kube-state-metrics, and an OpenTelemetry
  Collector foundation.
- Structured stdout logging and documented GitOps, rollback, recovery, and
  stateful-data boundaries.

## Stage 1 networking incident and fix

The public control-plane API returned HTTP 502 while its Pod remained healthy.
The actual cause was a NetworkPolicy selecting the API Pod while allowing only
Prometheus traffic to its metrics port. Traefik could no longer reach API TCP
8080.

The fix was a dedicated policy allowing the real `kube-system` Traefik identity
to reach the control-plane API on TCP 8080. Final live validation established:

```text
public /healthz                  -> HTTP 200
unauthenticated /api/v1          -> HTTP 401
unrelated Pod -> API TCP 8080    -> denied
unrelated Pod -> metrics 9090    -> denied
Prometheus -> metrics 9090       -> target Up
```

This incident demonstrates Kubernetes NetworkPolicy's additive semantics: once
a policy selects a Pod, every required path in that direction must be allowed
using the source identity actually present in the cluster.

## Current GitOps state

Final live validation recorded:

| Application | Sync | Health |
| --- | --- | --- |
| `control-plane-api` | Synced | Healthy |
| `platform-operator` | Synced | Healthy |
| `observability` | Synced | Healthy |
| `registry-smoke` | Synced | Healthy |

Platform Applications use reviewed commit pins and manual synchronization.
Automatic pruning and self-healing remain disabled, and the operator reviews
the Argo diff before synchronization. This is deliberate portfolio change
control rather than an incomplete automation feature.

## Observability state

The six validated Prometheus targets were Up during final platform validation:

1. Prometheus.
2. kube-state-metrics.
3. Platform operator.
4. OpenTelemetry Collector.
5. Control-plane API.
6. Managed demo.

Prometheus uses a 3 GiB local-path PVC with 72-hour and 2 GB retention bounds.
The Collector accepts OTLP, while its current pipelines export to `nop`.
Persistent traces and OTLP logs are therefore not provided. Centralized logging
is not implemented. Grafana, Tempo, Loki, and Alertmanager are not required for
the completed Project 1 scope.

## Real-application hosting contract

Project 1 owns:

- dedicated namespaces;
- Pod Security Admission and ResourceQuota;
- restricted AppProjects and GitOps admission; and
- platform-side observability integration.

Application repositories own their images, production Kustomize packages,
Deployments, StatefulSets, Services, Ingresses, Certificates, Middleware,
PVCs, ConfigMaps, Jobs, CronJobs, NetworkPolicies, health and metrics surfaces,
migrations, backup logic, and rollback compatibility.

The administrator owns DNS, Secret values, manual Argo approval and sync,
off-node backup destinations, and capacity decisions.

`ManagedService` is not the generic deployment path for independently released
portfolio applications. It remains a constrained platform capability for the
approved managed demo.

## SmartEnergy prerequisites now deployed

The live platform contains only these SmartEnergy prerequisites:

```text
Namespace/smartenergy
ResourceQuota/smartenergy
AppProject/argocd/smartenergy
```

The namespace enforces the Kubernetes v1.36 `restricted` Pod Security standard
for enforcement, warnings, and audit. Its quota is:

```text
requests.cpu:               500m
limits.cpu:                 1500m
requests.memory:            896Mi
limits.memory:              1536Mi
pods:                       12
persistentvolumeclaims:     3
requests.storage:           12Gi
```

The AppProject permits only
`https://github.com/etclank/smartenergy-api.git` to deploy to the `smartenergy`
namespace. Its namespaced allowlist is limited to ConfigMap, Service,
PersistentVolumeClaim, Deployment, StatefulSet, Job, CronJob, Ingress,
NetworkPolicy, Certificate, and Middleware. It grants no cluster-scoped access
and cannot manage Secrets, namespace policy, ServiceAccounts, or RBAC.

No SmartEnergy Argo Application exists yet. No SmartEnergy workload or
stateful application resource exists yet. This is intentional.

## Capacity baseline

SmartEnergy Checkpoint 1 recorded this pre-application sample:

| Measure | Value |
| --- | ---: |
| Node capacity | 2 vCPU; approximately 3.73 GiB allocatable memory |
| CPU use | 131m / 6% |
| Memory use | 2309 MiB / 60% |
| CPU requests | 385m / 19% |
| Memory requests | 668 MiB / 17% |
| CPU limits | 1450m / 72% |
| Memory limits | 1290 MiB / 33% |
| Nonterminated Pods | 23 |
| MemoryPressure | False |
| DiskPressure | False |
| PIDPressure | False |

This is one sample rather than peak-capacity evidence. The portfolio will
attempt to remain on the existing approximately 4 GiB VM initially.
SmartEnergy deployment must measure after PostgreSQL, Redis, API, worker, Beat,
and a controlled rollout. A resize is evidence-driven, not a prerequisite, and
the current node is not guaranteed to support the final topology.

## Known limitations

The accepted portfolio boundaries are:

- one K3s node on one Hetzner VM, with no HA control plane;
- local-path storage without replication;
- no automatic disaster recovery or production SLO;
- no tenant isolation and one shared control-plane administrative token;
- manually provisioned Secrets and manual Argo promotion;
- a deliberately constrained `ManagedService` template;
- no persistent distributed tracing or centralized logs;
- no highly available database platform; and
- intentionally small platform capacity.

These are documented scope decisions, not unresolved Project 1 bugs.

## Project 1 to Project 2 handoff

Active portfolio development now moves to `smartenergy-api`. Project 2 must
provide:

- versioned database migrations and database-backed readiness;
- a private metrics port;
- immutable GHCR publication and a digest-pinned production Kustomize overlay;
- separate API, worker, and single Beat workloads;
- PostgreSQL and Redis StatefulSets;
- exact NetworkPolicies and measured resource settings;
- structured stdout-only logging;
- backup and restore procedures; and
- incremental deployment with capacity checkpoints.

Only after the production overlay passes validation should Project 1 gain an
`Application/smartenergy` definition and the final Prometheus target
integration. Those resources are outside this closeout.

Project 1 has no remaining feature backlog. Later changes should respond to
requirements demonstrated while hosting SmartEnergy or the distributed job
platform rather than speculative platform expansion.
