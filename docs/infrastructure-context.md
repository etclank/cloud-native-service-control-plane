# Cloud-Native Service Control Plane — Hetzner Infrastructure and Deployment Context

> Historical portfolio record: deployment observations apply to the revisions
> and dates recorded below, not current availability. Review environment-specific
> commands before use; see the [documentation index](README.md).

## 1. Purpose of This Document

This document preserves the authoritative infrastructure and deployment
context for hosting the Cloud-Native Service Control Plane and its portfolio
workloads on a Hetzner Cloud virtual machine.

It records architectural decisions, deployment boundaries, security
requirements, the completed H1–H8 roadmap, validation expectations, and
operational context. Maintainers should review it before proposing or applying
infrastructure changes.

---

## 2. Relationship to the Main Project

This infrastructure supports the following project:

```text
Cloud-Native Service Control Plane
```

Suggested project repository:

```text
cloud-native-service-control-plane
```

Current platform closeout:

```text
docs/project-closeout.md
```

Main project README:

```text
README.md
```

This document owns the infrastructure and deployment concerns.

It does not own the internal implementation of:

* the Go Kubernetes Operator
* the `ManagedService` custom resource
* the Go control-plane REST API
* the Go demo application
* the SmartEnergy application

Those components are developed in their source packages and deployed through
the infrastructure defined here. SmartEnergy and the synthetic probe are
optional improvements, not implemented baseline components.

---

## 3. Infrastructure Objective

The objective was to create a secure, reproducible Kubernetes environment on
a Hetzner Cloud VM with narrowly approved public application access. The
implemented environment hosts:

* the Go Kubernetes Operator
* the Go control-plane API
* the managed Go demo application
* the OpenTelemetry Collector
* Argo CD
* bounded Prometheus and reduced kube-state-metrics
* supporting Kubernetes resources

The environment is designed as:

* a portfolio platform
* an interview demonstration environment
* a practical Kubernetes learning environment
* a cloud-native deployment laboratory

It must not be described as a highly available production platform unless the architecture is later expanded and validated accordingly.

---

## 4. Current Infrastructure Status

**Current hosting provider:** Hetzner Cloud

**Current account status:** Active portfolio project

**Current VM status:** `portfolio-k3s-01`, CX23, 2 vCPU, 4 GB RAM, 40 GB SSD

**Current deployment status:** H1–H8 complete for the current portfolio scope

**Current Kubernetes status:** Single-node K3s `v1.36.2+k3s1`, healthy

**Local development environment:** WSL on Windows

**Local Kubernetes target:** k3d/K3s

**Public Kubernetes target:** Single-node K3s

**Current phase:** baseline closed on 24 July 2026; no mandatory next phase

The live environment includes private Argo CD administration, the
`ManagedService` CRD and operator, the authenticated control-plane API, the
managed `portfolio-demo` workload, and a private, NetworkPolicy-restricted
OpenTelemetry Collector plus persistent Prometheus and reduced
kube-state-metrics. The Collector uses only a `nop` exporter; no production
workload exports OTLP. Operational work must inspect current state before
changing it; closeout records are evidence, not permission to assume that live
state can never drift.

---

## 5. Deployment Principles

The Hetzner environment should follow these principles:

### 5.1 Local First

Application development, controller testing, Kubernetes manifests, Helm packaging, and basic observability should be validated locally before deployment to Hetzner.

The public VM should not become the primary development environment.

### 5.2 Reproducible Infrastructure

Configuration should be documented and automated where practical.

Manual commands are acceptable initially, but they must be recorded clearly enough to reproduce the environment.

### 5.3 Minimal Public Exposure

Only approved user-facing endpoints should be accessible from the public internet.

Administrative and data services should remain private.

### 5.4 GitOps-Oriented Delivery

Once the initial bootstrap is complete, application deployment should be driven primarily through version-controlled configuration and Argo CD.

### 5.5 Security Before Convenience

Administrative dashboards, databases, Kubernetes APIs, telemetry receivers, and internal services must not be made public merely to simplify testing.

### 5.6 Honest Availability Claims

A single-node K3s cluster is suitable for this portfolio environment but does not provide node-level high availability.

The documentation must state this limitation clearly.

### 5.7 Incremental Deployment

Infrastructure should be introduced in small, testable slices.

Do not deploy the complete stack in one large operation.

---

## 6. Implemented Architecture

```text
                              Public Internet
                                     |
                                  DNS records
                                     |
                             Hetzner Cloud Firewall
                                     |
                              Public IPv4 / IPv6
                                     |
                           Hetzner Ubuntu Cloud VM
                                     |
                            K3s Kubernetes Cluster
                                     |
                          Traefik Ingress Controller
                                     |
                              cert-manager / TLS
                                     |
            +------------------------+------------------------+
            |                        |                        |
            v                        v                        v
   Control-Plane API        Managed Applications       Private Observability
            |                        |                        |
            |                        |                        |
            +------------------------+------------------------+
                                     |
                           Kubernetes internal network
                                     |
       +-----------------------------+-----------------------------+
       |                             |                             |
       v                             v                             v
 Kubernetes Operator      OpenTelemetry Collector              Argo CD
       |                             |                             |
       v                             v                             v
 ManagedService CRs       Collector + Prometheus          GitOps Reconciliation
                                 |
                         kube-state-metrics
```

---

## 7. Environment Model

The implemented deployment uses:

```text
One Hetzner Cloud VM
One K3s server node
One public IP
One Kubernetes cluster
Multiple Kubernetes namespaces
Traefik ingress
cert-manager
Argo CD
Persistent local volumes
External DNS records managed outside Kubernetes
```

This design prioritises:

* simplicity
* cost control
* fast setup
* demonstrability
* understandable operations

It does not provide:

* node-level redundancy
* control-plane redundancy
* multi-zone resilience
* highly available persistent storage
* automatic disaster recovery
* managed database availability

These limitations must be documented in the public repository.

---

## 8. Kubernetes Namespace Model

The implemented platform uses:

```text
kube-system
cert-manager
argocd
platform-system
applications
observability
```

Possible responsibilities:

### `platform-system`

Contains:

* Kubernetes Operator
* control-plane API
* platform configuration
* service accounts
* platform RBAC
* internal platform Services

### `applications`

Contains:

* Go demo service
* services created through `ManagedService`
* approved validation workloads when explicitly created

### `observability`

Contains:

* OpenTelemetry Collector
* Prometheus
* reduced kube-state-metrics
* observability configuration

### `argocd`

Contains:

* Argo CD components

### `cert-manager`

Contains:

* cert-manager components

Additional namespaces require an explicit ownership, AppProject, RBAC,
NetworkPolicy, and Secret boundary.

---

## 9. VM Capacity

The implemented server is a cost-optimized Hetzner CX23 with 2 vCPU, 4GB RAM,
and a 40GB SSD. H8 closeout measured about 6% CPU and 58% memory with the
accepted workloads Ready. Prometheus is bounded by 100m/400m CPU,
256Mi/512Mi memory, 72-hour and 2GB retention, and a 3Gi local-path PVC.

These figures are point-in-time evidence, not a permanent capacity guarantee.
Before adding a workload, account for its requests, limits, rollout surge,
storage growth, image architecture, and representative traffic. A database,
additional telemetry backends, or a highly available topology needs a new
sizing and failure-domain decision.

---

## 10. Operating System Baseline

The VM should use a supported Ubuntu LTS release unless a stronger reason for another distribution appears.

The initial operating-system setup should cover:

* package updates
* time synchronisation
* hostname
* non-root administration
* SSH key authentication
* disabling password-based SSH
* disabling direct root SSH
* basic firewall coordination
* automatic security updates where appropriate
* log review
* disk usage monitoring
* swap decision
* kernel and networking requirements for K3s
* container and K3s storage paths

Do not disable security controls merely because an installation guide suggests
it; document the consequence and use the narrowest compatible correction.

---

## 11. Administrative Access

### 11.1 SSH

SSH should use:

* public-key authentication
* a non-root administrative user
* `sudo` for privileged operations
* source-IP restrictions where practical
* no password authentication
* no direct root login

The private SSH key must remain on the developer's trusted local machine.

It must not be:

* committed to Git
* copied into the project repository
* stored in Kubernetes
* pasted into chat logs
* embedded in automation scripts

### 11.2 Kubernetes Administration

Kubernetes administration will initially use a local kubeconfig obtained securely from the K3s server.

The kubeconfig must:

* remain outside Git
* use restrictive file permissions
* not be pasted into project documentation
* not be shared publicly
* be rotated or replaced if exposed

The Kubernetes API should not be left accessible to the entire internet.

Preferred approaches include:

* Hetzner firewall source-IP restrictions
* SSH tunnelling
* VPN access
* private access through a secure administrative channel

The simplest secure approach should be chosen for the first deployment.

---

## 12. Network Exposure Policy

### 12.1 Publicly Accessible Ports

Expected public traffic:

```text
80/TCP
443/TCP
```

Port 80 may be used for:

* HTTP-to-HTTPS redirection
* ACME HTTP challenges if selected

Port 443 will serve approved HTTPS applications.

### 12.2 Restricted Administrative Port

```text
22/TCP
```

SSH should be restricted to the developer's current trusted IP range when practical.

If the source IP changes frequently, the chosen access strategy must be documented.

### 12.3 Ports That Must Not Be Public

The following must not be publicly open without an explicit documented exception:

```text
6443     Kubernetes API
5432     PostgreSQL
6379     Redis
3000     Grafana direct service
8080     Argo CD or application administration
9090     Prometheus
3100     Loki
3200     Tempo
4317     OTLP gRPC
4318     OTLP HTTP
10250    Kubelet
NodePort range
```

Applications should be exposed through Ingress rather than by opening arbitrary NodePorts.

---

## 13. Hetzner Cloud Firewall

A Hetzner Cloud Firewall should be created and attached to the VM.

Initial inbound policy:

```text
ALLOW TCP 22 from trusted administrative IP addresses
ALLOW TCP 80 from all required internet sources
ALLOW TCP 443 from all required internet sources
DENY all other unsolicited inbound traffic
```

Outbound traffic may initially remain permitted, but outbound restrictions should be considered later for:

* network probe safety
* application egress control
* air-gapped design exercises
* dependency containment

The effective firewall must be validated externally after every significant change.

Do not assume a configured rule is working without testing it.

---

## 14. Domain and DNS Strategy

A domain or subdomain structure will be used for public HTTPS services.

Illustrative structure:

```text
api.platform.example.com
demo.platform.example.com
grafana.platform.example.com
energy.platform.example.com
```

The real domain remains an open decision.

Initial DNS may be managed manually through the domain provider.

ExternalDNS is not required for the first deployment.

Possible later enhancement:

* Operator-managed DNS records
* ExternalDNS
* provider API integration
* deletion finalizers for external DNS cleanup

DNS documentation should record:

* domain owner
* DNS provider
* record type
* hostname
* destination IP
* TTL
* certificate relationship
* service ownership

No provider API token should be committed to the repository.

---

## 15. TLS and Certificate Management

Public services must use HTTPS.

The intended Kubernetes certificate solution is:

```text
cert-manager
```

The environment should include:

* cert-manager installation
* a test issuer where useful
* a production ACME issuer
* certificate resources
* automatic renewal
* renewal validation
* documented failure diagnosis

The exact ACME challenge should be selected based on the domain environment:

```text
HTTP-01
or
DNS-01
```

HTTP-01 is likely simpler initially.

DNS-01 may be preferred later if:

* wildcard certificates are required
* provider automation is available
* several dynamic subdomains are created

The project must not begin directly with production certificate requests while ingress and DNS are unverified, because repeated failures may encounter issuance limits.

---

## 16. K3s Installation Strategy

The initial Kubernetes distribution will be K3s.

The installation should be:

* pinned to a documented version
* configured through explicit installation parameters
* reproducible
* backed up before major upgrades
* validated after installation

The K3s configuration should consider:

* server TLS SANs
* kubeconfig permissions
* Traefik enablement
* local storage
* service load balancer behaviour
* metrics-server
* secrets encryption
* audit logging where practical
* log retention
* cluster CIDRs
* service CIDRs
* node labels
* upgrade procedure

Do not disable bundled K3s components without a clear architectural reason.

The initial design expects to retain:

* Traefik
* metrics-server
* local storage support

ServiceLB usage should be reviewed because the environment has a single VM and public IP.

---

## 17. Ingress Strategy

Traefik will initially be the ingress controller.

Ingress will handle:

* hostname routing
* HTTPS termination
* HTTP-to-HTTPS redirects
* approved public service exposure
* middleware where useful
* request-size controls
* basic security headers
* optional authentication layers

The platform Operator may create Ingress resources for managed applications.

The Operator must not be granted broader permissions than required.

Ingress definitions must not allow arbitrary unsafe annotations without validation.

---

## 18. Persistent Storage Strategy

The cluster uses K3s local-path storage. Prometheus is the only implemented
portfolio component in this repository with a persistent application PVC.

This is acceptable for the portfolio environment but has important limitations:

* volumes are tied to the node
* node loss can cause data loss
* no multi-node replication
* storage failure recovery is manual
* backups are essential

Any additional persistent workload must determine:

* which data must persist
* which data can be recreated
* retention durations
* volume sizes
* backup destinations
* restore procedures

Observability retention should remain deliberately limited to control disk growth.

---

## 19. Backup and Recovery Boundary

Git provides the reconstruction source for non-secret declarative
configuration. The current baseline does not include automated off-node
backups or a validated full-cluster disaster-recovery exercise.

The strategy should distinguish:

### 19.1 Reproducible Configuration

Recoverable from Git:

* Kubernetes manifests
* Helm values
* Argo CD applications
* Collector configuration
* application configuration without secrets
* Operator and API images

### 19.2 Cluster State

Potential backup targets:

* K3s datastore
* important custom resources
* application Secrets where safely handled
* certificate resources
* Argo CD configuration

### 19.3 Application Data

Prometheus history resides on a 3Gi local-path PVC with a `Delete` reclaim
policy. PostgreSQL, Redis, and SmartEnergy data are not present in the current
baseline. Any future stateful application must define off-node backup,
retention, restore, and deletion behavior before deployment.

### 19.4 Recovery Validation

A backup should not be considered reliable until a restore procedure has been
tested. An optional recovery exercise could validate:

1. rebuilding a replacement VM
2. reinstalling K3s
3. restoring GitOps configuration
4. restoring retained application data
5. restoring public application access
6. validating certificates and DNS

Backup credentials and encrypted archives must not be committed to Git.

---

## 20. Secret Management

Initial secrets may be supplied through manually created Kubernetes Secrets, provided that:

* Secret manifests containing real values are not committed
* commands are documented using placeholders
* local secret files are excluded by `.gitignore`
* file permissions are restrictive
* access is namespace-scoped
* Secrets are not printed in completion reports

Potential later options:

* SOPS with age
* Sealed Secrets
* External Secrets
* cloud secret manager integration

The first solution should remain simple and understandable.

Introducing a secret-management platform should not delay the application MVP unnecessarily.

---

## 21. Container Registry Strategy

The initial container registry is expected to be GitHub Container Registry.

Images may include:

```text
control-plane-api
kubernetes-operator
demo-service
network-probe
smartenergy-api
smartenergy-dashboard
```

Registry requirements:

* immutable or traceable tags
* commit-SHA tags
* version tags for releases
* no reliance on `latest` for stable deployment
* documented image architecture
* private registry authentication where required
* Kubernetes image pull Secrets if necessary
* vulnerability scanning where practical

Deployment manifests should identify exact image versions.

---

## 22. GitOps Strategy

Argo CD is the deployment reconciler for the platform operator and control-plane API. Synchronization is intentionally manual so a reviewed Git revision is selected before cluster state changes.

Initial workflow:

```text
1. Application code is committed.
2. GitHub Actions runs tests.
3. Container images are built.
4. Images are pushed to GHCR.
5. Deployment configuration references the new version.
6. Argo CD detects the Git change.
7. Argo CD synchronises the cluster.
8. Kubernetes rolls out the update.
9. Health and telemetry are validated.
```

The initial GitOps structure may remain inside the main repository:

```text
deploy/gitops/
```

A dedicated deployment repository may be introduced later if it provides clearer separation.

Argo CD should use:

* least-privilege project boundaries
* explicit namespaces
* controlled repositories
* protected administrative access
* clear sync policies
* documented rollback behaviour

The tested H6 rollback procedure is documented in [`argocd-sync-rollback-runbook.md`](argocd-sync-rollback-runbook.md). Automatic sync remains disabled by design.

---

## 23. Argo CD Access Policy

Argo CD is an administrative system and must not be exposed publicly without protection.

Preferred initial access methods:

* `kubectl port-forward`
* SSH tunnel
* private VPN
* ingress restricted by trusted source IP
* strong authentication in addition to HTTPS

A public unrestricted Argo CD login page is not acceptable.

Initial credentials must be changed or rotated immediately.

---

## 24. Optional Grafana Access Policy

Grafana is not implemented in the current baseline. If added as an optional
improvement, access must remain controlled.

Possible approaches:

* private access using port forwarding
* protected public ingress
* read-only anonymous dashboards with carefully selected data
* temporary interview access
* basic authentication or an identity-aware proxy

The selected solution must prevent exposure of:

* internal Kubernetes details
* sensitive application logs
* credentials
* request tokens
* personal data
* environment variables
* administrative controls

Logs and traces must be reviewed for sensitive-data leakage before Grafana is made publicly accessible.

---

## 25. Observability Deployment

The accepted observability stack is deployed. The OpenTelemetry Collector is
managed by a restricted, manually synchronized Argo CD Application in the
`observability` namespace. Its authorized OTLP/HTTP paths from
`platform-system` and `applications` have been validated; unauthorized
identities and policy-excluded TCP 13133 are blocked. It has no durable backend
and uses only the `nop` exporter. Bounded Prometheus and reduced
kube-state-metrics provide six accepted private scrape jobs with a 3Gi
local-path PVC. See
[`h8-collector-deployment-closeout.md`](h8-collector-deployment-closeout.md)
for Collector evidence and
[`h8-observability-closeout.md`](h8-observability-closeout.md) for final H8
acceptance.

Grafana, Loki, Tempo, workload OTLP export, kubelet/cAdvisor metrics, and
expanded alerting are optional. Each would require a new resource, retention,
security, and exposure review.

---

## 26. Optional SmartEnergy Deployment

SmartEnergy is not deployed in the current baseline. A future implementation
could contain:

```text
FastAPI API
Static dashboard
PostgreSQL
Redis
Database migration Job
Ingress
Secrets
PersistentVolumeClaims
OpenTelemetry configuration
```

An initial reviewed implementation could use:

* one PostgreSQL instance
* one Redis instance
* one FastAPI replica
* one dashboard replica

This is acceptable for demonstration purposes.

The documentation must state that this is not a highly available database architecture.

Required checks before accepting such an optional implementation would include:

* database persistence after pod replacement
* migration idempotency
* API readiness
* Redis connectivity
* dashboard-to-API connectivity
* backup and restore
* resource usage
* telemetry flow

---

## 27. Control-Plane Deployment

The deployed H7 control plane includes:

```text
Go Kubernetes Operator
Go control-plane REST API
ManagedService CRD
RBAC resources
ServiceAccount
Services
Ingress
production TLS and Traefik Middleware
digest-pinned GHCR images
```

The tested recreation order is:

```text
1. Install the CRD.
2. Install Operator RBAC.
3. Deploy the Operator.
4. Validate Operator readiness.
5. Deploy the control-plane API.
6. Validate Kubernetes API access.
7. Expose the API through protected HTTPS.
8. Create a sample ManagedService.
9. Validate generated resources.
10. Validate status reporting.
```

The operator and API use separate ServiceAccounts. The API can only create, get, list, and delete `ManagedService` resources in `applications`; it cannot read Secrets, manage Deployments or Services directly, select another namespace, update resources, or patch status.

`ManagedService` uses `platform.eoghanclancy.eu/v1alpha1` and the fixed `demo-http` template. Replicas default to one and are limited to one through three; messages are limited to 120 characters. The operator creates owned Deployments and ClusterIP Services, reports `Available`, `Progressing`, and `Degraded` conditions, reports the internal endpoint, garbage-collects owned children, and corrects drift.

The public API is `https://api.platform.eoghanclancy.eu`. Health and readiness are public, while `/api/v1` lifecycle routes require a bearer token loaded from the mounted `platform-system/control-plane-api-token` Secret. The Secret value is not stored in Git.

Runtime images are pinned to these immutable identities:

```text
ghcr.io/etclank/cloud-native-service-control-plane-operator@sha256:8f166fe9cdcbab093dee0bbef460e96dc1ec612973c48f9a07de35f16f4d0937
ghcr.io/etclank/cloud-native-service-control-plane-demo-http@sha256:bf9a75e48c4cbe2a14be4c61339115b76c2af11a06bfcc1b560f52ff3ed46e9e
ghcr.io/etclank/cloud-native-service-control-plane-api@sha256:604c16f04b00272b7b45072ff0c50c5c2d081fbc4ee795e62a4ed1fc861df36e
```

Registry credentials remain namespace-scoped. `platform-system/ghcr-pull` authenticates operator and API Pods; `applications/ghcr-pull` authenticates managed workload Pods. The production certificate writes its generated key pair to `platform-system/control-plane-api-tls`.

---

## 28. Security Boundaries

### Public Boundary

May include:

* control-plane API
* the lightweight TLS validation endpoint

### Cluster-Internal Boundary

Includes:

* Operator metrics
* OpenTelemetry receivers
* internal application Services
* Prometheus
* kube-state-metrics
* Argo CD server
* Kubernetes API access

### Administrative Boundary

Includes:

* SSH
* `kubectl`
* Argo CD administration
* registry credentials
* DNS-provider credentials

These boundaries must be reflected in:

* firewall rules
* ingress resources
* Kubernetes RBAC
* namespace permissions
* Secrets
* documentation

---

## 29. Required Security Review Areas

Before public deployment, review:

* SSH configuration
* Hetzner firewall
* Kubernetes API exposure
* RBAC permissions
* service accounts
* host-network usage
* privileged containers
* container capabilities
* root container users
* image provenance
* image vulnerabilities
* Secret handling
* ingress annotations
* TLS
* public dashboard access
* application authentication
* rate limiting
* request-size limits
* SSRF exposure
* database exposure
* Redis exposure
* telemetry data leakage
* backup encryption

A security review should report both implemented controls and remaining risks.

---

## 30. Resource Management

All long-running workloads should eventually define:

```text
resources.requests.cpu
resources.requests.memory
resources.limits.cpu
resources.limits.memory
```

Initial values may be conservative and tuned from observations.

Resource review should identify:

* pods being OOM-killed
* CPU throttling
* excessive storage growth
* unbounded logs
* high telemetry volume
* unsuitable retention periods
* unnecessary replicas
* overcommitted VM resources

The cluster should retain enough unused capacity for:

* rollouts
* Jobs
* controller reconciliation
* temporary debugging
* recovery operations

---

## 31. Deployment Environments

The project initially has two environments:

### Local

```text
WSL
Docker
k3d
local ingress
development observability
```

### Public Portfolio

```text
Hetzner VM
Ubuntu
K3s
public DNS
HTTPS
GitOps
persistent storage
protected administration
```

The configurations should share common packaging where practical but may use separate values.

Suggested environment structure:

```text
deploy/
├── base/
├── local/
├── hetzner/
├── gitops/
└── observability/
```

or:

```text
deploy/helm/
├── platform/
├── demo-service/
├── smartenergy/
└── observability/

deploy/environments/
├── local/
└── hetzner/
```

The final structure should be chosen after the main project repository is scaffolded.

---

## 32. Infrastructure Development Phases

The construction roadmap is closed as of 24 July 2026:

| Phase | Status | Accepted result |
| --- | --- | --- |
| H0 | Complete | Architecture, boundaries, and delivery sequence |
| H1 | Complete | Hetzner VM, restricted SSH, and firewall |
| H2 | Complete | Hardened Ubuntu host |
| H3 | Complete | Pinned single-node K3s and private administration |
| H4 | Complete | DNS, cert-manager, production TLS, and redirect |
| H5 | Complete | Immutable GHCR publication and digest-pinned runtime |
| H6 | Complete | Private restricted Argo CD and rollback |
| H7 | Complete | Operator, API, managed demo, GitOps, and public API TLS |
| H8 | Complete | Restricted Collector and bounded six-job Prometheus |

H9–H12 below preserve the original roadmap rationale but are optional
improvements, not unfinished baseline phases.

### Phase H0 — Planning and Decisions

Deliverables:

* this context document
* initial architecture
* VM sizing estimate
* domain strategy
* security boundaries
* deployment sequence

Exit criteria:

* infrastructure responsibilities are clear
* no unresolved decision blocks initial VM provisioning
* cost expectations are understood

Current status:

```text
COMPLETE
```

---

### Phase H1 — Hetzner Project and VM Provisioning

Deliverables:

* Hetzner Cloud project
* VM
* SSH key registration
* firewall
* hostname
* infrastructure inventory
* cost record

Exit criteria:

* VM is reachable through restricted SSH
* root password access is not required
* firewall rules are validated externally
* VM details are documented without exposing secrets

---

### Phase H2 — Operating-System Hardening

Deliverables:

* system updates
* non-root administrator
* SSH hardening
* time synchronisation
* security updates
* base diagnostic tools
* storage review
* host-level monitoring baseline

Exit criteria:

* password SSH is disabled
* direct root SSH is disabled
* administrative access is validated
* no accidental lockout remains
* baseline configuration is documented

---

### Phase H3 — K3s Installation

Deliverables:

* pinned K3s installation
* kubeconfig access
* node validation
* cluster networking validation
* Traefik validation
* storage-class validation
* metrics-server validation

Exit criteria:

* node is `Ready`
* core pods are healthy
* test workloads can be scheduled
* Services resolve internally
* ingress can route a test application
* kubeconfig is protected

---

### Phase H4 — DNS and TLS Foundation

Deliverables:

* DNS records
* cert-manager
* ACME issuers
* test certificate
* production certificate
* HTTPS test ingress

Exit criteria:

* DNS resolves correctly
* HTTP routing works
* HTTPS routing works
* certificate renewal path is understood
* no administrative service is accidentally public

---

### Phase H5 — GitHub Registry and CI Access

Deliverables:

* GHCR image workflow
* registry authentication if required
* Kubernetes image pull Secret
* immutable deployment tag strategy
* test-image deployment

Exit criteria:

* cluster can pull approved images
* credentials are not stored in Git
* failed image pulls can be diagnosed
* deployed image version is traceable to a commit

---

### Phase H6 — Argo CD Bootstrap

Deliverables:

* Argo CD installation
* protected administrative access
* repository registration
* initial Application
* sync and rollback runbook

Exit criteria:

* Argo CD reads the deployment repository
* one test application synchronises successfully
* rollback behaviour is tested
* Argo CD is not unrestricted publicly

Current status:

```text
COMPLETE
```

---

### Phase H7 — Platform Deployment

Deliverables:

* ManagedService CRD
* Operator
* control-plane API
* RBAC
* Ingress
* Secrets
* configuration

Exit criteria:

* Operator becomes ready
* API becomes ready
* API can create a ManagedService
* child resources reconcile correctly
* public API is authenticated and HTTPS-protected
* drift correction works

Current status:

```text
COMPLETE — validated 2026-07-20
```

---

### Phase H8 — Observability Deployment

Deliverables:

* OpenTelemetry Collector
* standalone Prometheus
* reduced kube-state-metrics
* retention configuration

Exit criteria:

* six accepted metrics jobs are healthy
* Prometheus history survives Pod replacement
* disk growth is bounded
* access is protected

Current status:

```text
COMPLETE — restricted Collector and bounded six-job Prometheus validated 2026-07-23
```

The accepted H8 scope is complete. Loki, Tempo, Grafana, workload OTLP export,
kubelet, cAdvisor, dashboards, and additional alerting are optional post-H8
enhancements. The live evidence is recorded in
[`h8-observability-closeout.md`](h8-observability-closeout.md).

---

### Optional Improvement — SmartEnergy Deployment

Deliverables:

* FastAPI deployment
* dashboard deployment
* PostgreSQL
* Redis
* migration Job
* storage
* ingress
* TLS
* observability

Exit criteria:

* dashboard is publicly reachable
* API is operational
* database data persists
* Redis works
* migrations complete safely
* telemetry is visible
* backups are configured

Current status: not implemented; not required by the closed baseline.

---

### Optional Improvement — Network Probe Deployment

Deliverables:

* network probe image
* target configuration
* HTTP, TCP, and DNS checks
* bounded concurrency
* dashboards
* alerts

Exit criteria:

* approved targets are checked
* arbitrary targets cannot be submitted publicly
* failures are observable
* timeouts work
* resource usage is bounded

Current status: not implemented; not required by the closed baseline.

---

### Optional Improvement — Backup and Recovery Validation

Deliverables:

* database backup
* K3s backup
* configuration export
* recovery runbook
* restore test

Exit criteria:

* PostgreSQL restore succeeds
* GitOps rebuild succeeds
* critical Secrets can be restored securely
* recovery steps are timed and documented
* known recovery gaps are explicit

Current status: Git reconstruction and rollback boundaries are documented;
automated backup and a full restore exercise are not implemented.

---

### Optional Improvement — Public Demo Hardening

Deliverables:

* final firewall review
* final ingress review
* rate limits
* authentication review
* public endpoint inventory
* dashboards
* demo runbook
* cost review
* security report

Exit criteria:

* all public endpoints are intentional
* all internal services remain private
* live demonstration is repeatable
* sensitive data is absent from dashboards
* resource use is stable
* infrastructure limitations are documented

Current status: TLS, redirect, API authentication, rate limiting, immutable
images, and exposure checks are implemented. Broader demonstration and
resilience exercises remain optional.

---

## 33. Historical Deployment Priority

The completed build followed this dependency-oriented sequence:

```text
1. Secure VM
2. K3s
3. DNS and HTTPS test service
4. Registry image pull
5. Operator and CRD
6. Demo application
7. Control-plane API
8. GitOps
9. Basic observability
10. Optional application or hardening work, when justified
```

This list explains build ordering; it is not a remaining mandatory plan.

---

## 34. Explicit Infrastructure Non-Goals

The initial deployment will not attempt to implement:

* highly available K3s control planes
* multi-node Kubernetes
* multi-region operation
* multi-cluster runtime orchestration
* Cluster API provisioning
* custom CNI development
* custom CSI development
* managed cloud load-balancer integration
* service mesh
* production-grade database clustering
* production-grade Redis clustering
* automatic horizontal infrastructure scaling
* full zero-trust networking
* enterprise identity federation
* complete air-gapped installation
* public unrestricted administrative interfaces
* unlimited telemetry retention

These may become future design exercises or stretch implementations.

---

## 35. Contributor Responsibilities

Infrastructure work may be divided into the following review responsibilities.

### Infrastructure Lead

Owns:

* phase planning
* architecture consistency
* dependency ordering
* risk review
* completion criteria
* coordination across contributors and systems

### Linux and Security

Owns:

* VM bootstrap
* SSH
* OS hardening
* patching
* firewall validation
* user permissions
* host-level security

### Kubernetes

Owns:

* K3s installation
* cluster configuration
* namespaces
* storage
* ingress
* RBAC
* cluster validation
* upgrades

### Networking and TLS

Owns:

* DNS
* firewall rules
* ingress routing
* cert-manager
* TLS
* public exposure inventory

### GitOps and CI

Owns:

* GHCR
* GitHub Actions coordination
* image tags
* Argo CD
* deployment repositories
* rollback procedures

### Observability

Owns:

* OpenTelemetry Collector
* Loki
* Tempo
* Prometheus or Mimir
* Grafana
* alerts
* telemetry retention
* incident dashboards

### Database and State

Owns:

* PostgreSQL
* Redis
* persistent volumes
* migrations
* backups
* restores
* stateful workload validation

### Security Review

Owns:

* threat review
* RBAC review
* ingress review
* Secret review
* image review
* public exposure verification
* completion security report

Overlapping destructive changes require explicit coordination and a single
reviewed owner.

---

## 36. Infrastructure Change Rules

Before making infrastructure changes:

1. Read this document.
2. Read the main project context.
3. Inspect the current infrastructure state.
4. Identify the active deployment phase.
5. Review previous completion reports.
6. Confirm whether commands are local, remote, or Kubernetes-scoped.
7. Identify possible destructive consequences.
8. Preserve existing working access.

While making changes:

1. Work in small slices.
2. Explain privileged commands.
3. Avoid exposing secrets.
4. Avoid opening broad firewall rules.
5. Avoid disabling security controls without justification.
6. Avoid deleting stateful resources casually.
7. Avoid using floating image tags.
8. Record changed configuration.
9. Validate after each significant step.
10. Stop and report unexpected state rather than guessing destructively.

Before declaring completion:

1. Validate service health.
2. Validate firewall exposure.
3. Validate Kubernetes resources.
4. Check logs and events.
5. Check disk and memory.
6. Confirm administrative access still works.
7. Update documentation.
8. Report exact commands or automation used.
9. Report unvalidated assumptions.
10. Provide the next recommended slice.

---

## 37. Destructive-Action Policy

The following actions require explicit caution and a documented backup or recovery plan:

* deleting the VM
* rebuilding the VM
* reinstalling K3s
* removing the K3s datastore
* deleting namespaces
* deleting persistent volumes
* deleting PostgreSQL data
* deleting Secrets
* replacing firewall rules
* modifying SSH access
* changing DNS for live endpoints
* rotating certificate issuers
* modifying storage classes
* uninstalling Argo CD
* uninstalling cert-manager
* upgrading K3s
* changing cluster network ranges

Prefer reversible changes.

A destructive operation should state:

```text
Purpose
Affected resources
Data-loss risk
Access-loss risk
Backup status
Rollback procedure
Validation procedure
```

---

## 38. Infrastructure Inventory

The current sanitized inventory is maintained in
[the build guide](cloud-native-service-control-plane-build-guide.md#4-current-resource-inventory)
and the [project closeout](project-closeout.md). Sensitive values, Secret
contents, kubeconfigs, and private keys remain outside the repository. Private
recovery metadata may be maintained separately.

---

## 39. Preferred Infrastructure Slice Format

Each infrastructure implementation slice should define:

```text
Objective
Current state
Prerequisites
Scope
Systems affected
Commands or files expected
Security considerations
Rollback plan
Validation commands
Acceptance criteria
Known exclusions
Documentation updates
```

Each completion report should contain:

```text
Summary
Infrastructure changed
Configuration changed
Commands executed
Validation performed
Validation results
Security impact
Public exposure changes
Resource usage
Known limitations
Rollback status
Recommended next slice
```

---

## 40. Validation Categories

Every phase should use relevant checks from the following categories.

### Host Validation

```text
systemctl status
journalctl
ss -tulpn
df -h
free -h
uptime
timedatectl
```

### Network Validation

```text
DNS resolution
TCP connection tests
HTTP response tests
HTTPS certificate inspection
external port checks
firewall rule inspection
```

### Kubernetes Validation

```text
kubectl get nodes
kubectl get pods --all-namespaces
kubectl get events
kubectl describe
kubectl logs
kubectl auth can-i
kubectl get ingress
kubectl get pvc
kubectl top
```

### Application Validation

```text
health endpoint
readiness endpoint
authenticated API call
ManagedService lifecycle
public route
database connection
Redis connection
telemetry export
```

### Security Validation

```text
public port scan from an external host
RBAC permission review
Secret exposure review
container privilege review
TLS review
administrative endpoint review
```

Validation commands should be recorded without exposing secret values.

---

## 41. Troubleshooting Approach

Infrastructure troubleshooting should follow this sequence:

```text
1. Confirm the reported symptom.
2. Identify the affected boundary.
3. Check recent changes.
4. Inspect host health.
5. Inspect Kubernetes events.
6. Inspect pod status.
7. Inspect application logs.
8. Inspect Service and endpoint mappings.
9. Inspect ingress routing.
10. Inspect DNS and TLS.
11. Inspect firewall rules.
12. Inspect resource pressure.
13. Apply the smallest safe correction.
14. Revalidate the complete path.
15. Document the root cause.
```

Avoid restarting or reinstalling systems before understanding the failure.

---

## 42. Incident Scenarios to Demonstrate

The final portfolio environment should support controlled demonstration of:

* deleted Deployment restored by the Operator
* unhealthy pod blocked by readiness
* failed image pull
* incorrect Service selector
* application latency spike
* application error-rate increase
* failed DNS check
* failed TCP check
* full or nearly full disk warning
* PostgreSQL connection failure
* expired or invalid certificate simulation
* Argo CD drift correction

Each scenario should include:

* trigger
* expected symptoms
* diagnostic steps
* telemetry evidence
* resolution
* prevention

Destructive incidents should be simulated safely.

---

## 43. Cost Management

The infrastructure conversation should track:

* VM monthly cost
* backup cost
* volume cost
* snapshot cost
* public IPv4 cost if applicable
* domain cost
* registry cost
* bandwidth considerations

Cost controls should include:

* right-sizing
* limited telemetry retention
* removing unused snapshots
* stopping unused temporary VMs
* avoiding duplicate observability stacks
* monitoring disk growth

The public environment may remain continuously available during interview preparation, but its running cost should be understood.

---

## 44. Optional Production-Hardening Considerations

The project documentation should explain how the environment would evolve beyond the portfolio deployment.

Potential production improvements:

* multi-node K3s or upstream Kubernetes
* highly available control plane
* separate worker nodes
* private networking
* managed load balancer
* highly available PostgreSQL
* replicated object or block storage
* managed Redis or Redis Sentinel
* dedicated observability nodes
* external secret management
* network policies
* pod security standards
* vulnerability scanning
* image signing
* admission policies
* automated DNS
* external backups
* disaster recovery testing
* SSO
* multi-tenancy
* separate staging and production clusters

These are optional improvements, not current claims or baseline completion
requirements. Their consolidated status and prerequisites are in
[`optional-improvements.md`](optional-improvements.md).

---

## 45. Multi-Cluster and Air-Gapped Interview Scope

Possible design exercises include:

### Multi-Cluster

* management cluster
* workload clusters
* GitOps per cluster
* cluster registration
* service placement
* failure isolation
* central observability
* per-cluster credentials
* Cluster API integration

### Air-Gapped Operation

* internal image registry
* mirrored container images
* offline Helm artefacts
* vendored Go modules
* package repository mirrors
* internal certificate authority
* offline installation bundles
* signed artefacts
* software bills of materials
* restricted telemetry
* controlled upgrade media

No implementation should be represented as complete unless it has been validated.

---

## 46. Documentation and Workstream Boundaries

- Application source, controllers, APIs, tests, and image workflows belong to
  their repository packages.
- Hetzner, Ubuntu, K3s, firewall, DNS, TLS, storage, and GitOps decisions
  belong in this infrastructure context and focused runbooks.
- Live evidence belongs in dated closeout or proof records.
- Day-to-day commands belong in the operator guide.
- New application requirements belong in the application deployment guide.
- Optional ideas belong in the optional-improvements register until a bounded
  implementation is accepted.

Material deployment decisions should be reflected in the relevant
authoritative document rather than existing only in transient working notes.

---

## 47. Current Authoritative Decisions

The following decisions are currently authoritative:

* Hetzner Cloud is the intended public hosting provider.
* The first public environment will use one VM.
* Ubuntu LTS is the intended operating system.
* K3s is the intended Kubernetes distribution.
* The initial cluster is single-node.
* Traefik is the intended ingress controller.
* cert-manager is the intended certificate-management system.
* Public applications will use HTTPS.
* The Kubernetes API must not be broadly public.
* SSH will use keys and restricted access.
* PostgreSQL and Redis must not be publicly exposed.
* Argo CD administration must remain protected.
* Grafana access must remain protected or carefully restricted.
* GitHub Container Registry is the initial image registry.
* Argo CD is the intended GitOps tool.
* Local-path storage is acceptable initially if its limitations are documented.
* Automated backup and full restore testing remain optional
  production-hardening work and must not be implied by the current baseline.
* The observability stack must use bounded retention.
* The infrastructure will be built incrementally.
* Local validation should precede public deployment.
* Secrets must never be committed to Git.
* No high-availability claim should be made for the initial environment.
* The operator and API use separate least-privilege ServiceAccounts.
* The control-plane API manages only `ManagedService` resources in `applications`.
* Argo CD synchronization remains manual and Argo CD remains private.
* Public API traffic uses production TLS, HTTPS redirection, and Traefik rate limiting.
* Runtime images are selected by immutable SHA-256 digest.

---

## 48. Decisions for Optional Improvements

The following decisions are needed only if the corresponding optional work is
accepted:

* backup destination
* longer-term Secret-management mechanism beyond manually managed Kubernetes Secrets
* Grafana public-access mechanism
* retention periods for optional future backends
* K3s upgrade strategy
* VM snapshot policy
* whether additional Hetzner volumes are required

Resolve them within the bounded improvement where they become necessary.

The simplest safe and reversible option should be preferred.

---

## 49. Baseline Closeout

The implemented infrastructure baseline is complete:

```text
H1–H8 COMPLETE — closed 24 July 2026
```

There is no mandatory H9. Routine operation follows the
[operator guide](operator-guide.md). A new application follows the
[application deployment guide](application-deployment-guide.md). Any
[optional improvement](optional-improvements.md) starts with a fresh resource
baseline and preserves immutable images, restricted GitOps, namespace
isolation, and explicit persistence boundaries.

---

## 50. Definition of Infrastructure Success

The Hetzner environment is successful when the developer can demonstrate and explain:

* how the VM is secured
* how inbound traffic is restricted
* how K3s is installed and operated
* how DNS reaches the ingress controller
* how HTTPS certificates are issued and renewed
* how applications are delivered through GitOps
* how container images are versioned
* how the Operator and API are deployed
* how the current Prometheus state persists and what local-path cannot provide
* how metrics are collected and why logs/traces are outside the accepted scope
* how administrative systems remain protected
* what Git can reconstruct and where backup/recovery limitations remain
* how failures are diagnosed
* how the single-node design differs from production high availability
* how the architecture could evolve toward multi-cluster and air-gapped deployments
