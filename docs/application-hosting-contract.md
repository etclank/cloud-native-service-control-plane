# Real-Application Hosting Contract

This document defines how substantial portfolio applications are admitted to
the single-node K3s platform. SmartEnergy is the first application using this
contract. Its planned public host is `energy.platform.eoghanclancy.eu`.

`ManagedService` is not the generic deployment mechanism for real portfolio
applications. The constrained operator and its `applications` namespace remain
a separate platform capability for managed `demo-http` workloads.

## Ownership

### Platform provides

- K3s, Argo CD, Traefik, cert-manager, and Prometheus;
- a dedicated namespace with Pod Security Admission labels;
- a conservative namespace ResourceQuota;
- a dedicated AppProject restricted by source, destination, and resource kind;
- a manual GitOps admission point after the application package is ready; and
- platform-side Prometheus discovery and cross-namespace policy when the final
  application metrics identity is known.

### Application provides

- an immutable OCI image and digest-pinned production Kustomize overlay;
- Deployments, StatefulSets, Services, Ingress, Certificate, Middleware,
  ConfigMaps, PVCs, Jobs, CronJobs, and workload NetworkPolicies;
- process health and dependency-aware readiness endpoints;
- a private metrics Service port with stable labels and a stable port name;
- explicit resource requests, limits, and low-memory rollout strategies;
- versioned database migrations and rollback compatibility; and
- an explicit durability boundary, including any backup and restore capability required by the application's approved scope.

The application repository owns these resources so that runtime configuration
and release behavior change with the code they describe. It must not request
Kubernetes RBAC unless a demonstrated runtime need is reviewed.

### Administrator provides

- DNS records;
- Secret values and reconstruction or rotation procedures;
- a namespace-scoped GHCR pull credential when the package is private;
- off-node backup storage and its credentials when required by the approved application scope;
- review of the Argo diff and manual synchronization; and
- capacity measurements and a VM resize only when later evidence justifies it.

## Namespace strategy

Substantial independently released applications receive dedicated namespaces.
The initial names are:

| Application | Namespace |
| --- | --- |
| SmartEnergy | `smartenergy` |
| Distributed job platform | `distributed-job-platform` |

This isolates Secrets, AppProject destinations, NetworkPolicies, quota, and
resource accounting. The existing `applications` namespace is reserved for
`ManagedService`-managed demo workloads and must not host independently
released applications.

## GitOps admission

Application manifests remain in the application repository. Project 1 contains
the platform-owned namespace policy, AppProject, and eventual bootstrap
Application only.

The release flow is:

```text
application commit
-> CI
-> GHCR image
-> immutable image digest
-> application production overlay
-> reviewed application commit
-> Project 1 adds or advances a full pinned targetRevision
-> Argo diff review
-> manual sync
```

SmartEnergy's current Application contract is:

```yaml
repoURL: https://github.com/etclank/smartenergy-api.git
path: deploy/kubernetes/overlays/production
targetRevision: 6dee39d597ce87620f4f12b17ea0bdc079d029f0
destination: smartenergy
project: smartenergy
sync: manual
prune: disabled initially
selfHeal: disabled initially
```

The checked-in Application passed the admission gate and uses manual synchronization. SmartEnergy is live; advancing the checked-in pin does not by itself authorize another sync.

## Secrets

Secrets are not committed to Git. Documentation and value-free examples may
define expected names and keys, while an administrator provisions actual Secret
objects out of band. The SmartEnergy AppProject intentionally cannot create
Secrets.

Expected categories are JWT signing material, PostgreSQL credentials, Redis
credentials, and a GHCR pull credential when required. Off-node backup
credentials apply only to applications whose approved scope includes that
capability. SendGrid and OTLP credentials are optional and disabled initially.
The administrator owns reconstruction and coordinated rotation.

## Stateful services

The planned SmartEnergy baseline is:

| Service | Form | Initial application sizing | Durability boundary |
| --- | --- | --- | --- |
| PostgreSQL | One StatefulSet replica with local-path PVC | approximately 5 GiB | persistent, non-HA |
| Redis | One StatefulSet replica with local-path PVC and AOF | approximately 1 GiB | persistent, non-HA |

These sizes are starting choices for application validation, not permanent
platform guarantees. SmartEnergy owns the database and Redis manifests and
schema migrations. Project 1 does not provide a general database service.

A PVC preserves data across ordinary Pod replacement. It is not an off-node
backup. A backup provides a recoverable copy; it does not make a single replica
highly available.

## Capacity and rollout strategy

The portfolio will first attempt to remain on the existing approximately 4 GiB
Hetzner VM. The Stage 2B sample recorded 2 vCPU, approximately 3.73 GiB
allocatable memory, and approximately 59% memory use before SmartEnergy. That
single sample is a baseline rather than a peak-capacity guarantee, especially
because several platform Pods have no declared requests or limits.

The operating strategy is:

1. Start API, worker, Beat, PostgreSQL, and Redis with one replica each.
2. Require explicit requests and limits for every container and Job.
3. Avoid sidecars, exporters, SendGrid, OTLP, and automatic demo work initially.
4. Use a private metrics port without adding separate exporter Pods.
5. Avoid rollout surge and measure after every deployment step.
6. Stop and reassess when measurements show unsafe headroom.
7. Resize the VM only when sustained evidence shows the required workload does
   not operate safely on the current node.

Initial memory experiments should remain within these ranges:

| Component | Request | Limit |
| --- | ---: | ---: |
| API | 128-192 MiB | 256-320 MiB |
| Worker | 128-192 MiB | 256-320 MiB |
| Beat | 32-64 MiB | 96-128 MiB |
| PostgreSQL | 192-256 MiB | 384-512 MiB |
| Redis | 32-64 MiB | 96-128 MiB |

These are measurement starting points, not application guarantees. The
namespace quota permits 500m CPU requests, 1500m CPU limits, 896 MiB memory
requests, 1536 MiB memory limits, 12 Pods, three PVCs, and 12 GiB requested
storage. It can be revised after representative measurements.

To avoid temporary duplication on the small node, use:

- API and worker: `RollingUpdate`, `maxSurge: 0`, `maxUnavailable: 1`;
- Beat: `Recreate`, with exactly one scheduler;
- PostgreSQL and Redis: one StatefulSet Pod each.

This accepts brief reduced availability during an update to protect node
headroom. Resize evidence includes `MemoryPressure`, OOM kills, evictions,
scheduler placement failures, impaired platform responsiveness, insufficient
controlled-rollout headroom, or memory remaining around 80-85% under sustained
representative load. A single threshold crossing triggers investigation rather
than an automatic resize decision.

## Networking

Application packages use namespace default-deny plus explicit allows:

```text
Traefik -> public API HTTP port
Prometheus -> private metrics port
API and worker -> PostgreSQL
API, worker, and Beat -> Redis where required
relevant workloads -> CoreDNS
```

NetworkPolicies are additive. Once a policy selects a Pod for ingress or
egress, traffic in that direction must be explicitly allowed by the applicable
policies. Stage 1 demonstrated that adding a selecting policy can break an
existing path unless its real source identity, namespace, labels, protocol, and
port are allowed. SmartEnergy owns the final policies because Project 2 will
finalize those identities and ports.

## Observability

Project 1 uses Prometheus EndpointSlice discovery, not Prometheus Operator.
Applications provide a private metrics Service port, stable Service labels, a
stable named metrics port, bounded metric labels, and a policy allowing only
Prometheus. Project 1 adds the corresponding discovery target and
cross-namespace egress after those values are final.

Do not introduce a `ServiceMonitor`. OTLP remains disabled while the platform
Collector exports to `nop`.

## TLS and DNS

For SmartEnergy, the planned host is `energy.platform.eoghanclancy.eu`.

| Concern | Owner |
| --- | --- |
| Ingress and Certificate | SmartEnergy |
| Generated TLS Secret | cert-manager |
| ClusterIssuer and Traefik | Project 1 |
| DNS record | Administrator |

## Rollback

Git or Argo rollback is not database rollback. Project 1 owns the pinned source
revision, Argo diff, manual synchronization, and platform rollback operation.
The application owns image and configuration compatibility, schema
compatibility, migration downgrade or forward-fix strategy, and data restore.

## SmartEnergy deployment gate

### Project 1 ready before admission

- [x] Dedicated namespace manifest with restricted Pod Security Admission.
- [x] Conservative ResourceQuota for the current node.
- [x] Dedicated source, destination, and resource-restricted AppProject.
- [x] Manual GitOps, Secret, stateful, networking, observability, and rollback
      contracts documented.
- [x] Application bootstrap deployed at an immutable revision with manual synchronization.

### SmartEnergy must implement

- [x] Versioned migrations and database-backed readiness.
- [x] Private metrics port.
- [x] Immutable GHCR publication and digest-pinned production overlay.
- [x] Separate API, worker, and single Beat workloads.
- [x] PostgreSQL and Redis StatefulSets.
- [x] Explicit measured resources and low-memory rollout strategies.
- [x] Default-deny and exact allow NetworkPolicies.
- [x] Structured stdout-only logs.
- [x] Demo seeding and scheduled demo mutation disabled by default.
- [x] Pod-restart persistence validated for PostgreSQL and Redis.
- [x] Off-node backup and node/PVC-loss recovery recorded as outside the portfolio/demo scope.

### Deployment-time actions

- [x] Provision Secrets and optional GHCR access out of band.
- [x] Add the pinned Argo Application and review its diff.
- [x] Create DNS and obtain the cert-manager Certificate.
- [x] Activate the final Prometheus target.
- [x] Deploy incrementally and measure at every capacity checkpoint.
- [x] Retain the existing VM after capacity validation showed no pressure condition.

## Incremental capacity checkpoints

Record the platform baseline, then deploy and measure PostgreSQL, Redis, API,
worker, and Beat in that order. After the final steady-state measurement,
perform one controlled application rollout and record its temporary use.

At every checkpoint inspect:

```bash
kubectl top nodes
kubectl top pods -A
kubectl describe node portfolio-k3s-01
kubectl get pods -A
kubectl get events -A --sort-by='.lastTimestamp'
```

Review Pod restarts, `OOMKilled` states, `MemoryPressure`, evictions, scheduler
failures, CPU throttling, and platform responsiveness. If required Pods cannot
be placed or safe rollout headroom cannot be demonstrated, stop and reassess
resource settings, optional components, and workload behavior before deciding
whether to resize.
