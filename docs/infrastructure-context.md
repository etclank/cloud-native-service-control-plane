# Cloud-Native Service Control Plane — Hetzner Infrastructure and Deployment Context

## 1. Purpose of This Document

This document is the authoritative infrastructure and deployment context for hosting the Cloud-Native Service Control Plane and its portfolio workloads on a Hetzner Cloud virtual machine.

It is intended to be supplied to:

* the dedicated Hetzner infrastructure ChatGPT conversation
* Codex infrastructure agents
* deployment agents
* Linux administration agents
* Kubernetes agents
* security review agents
* observability agents
* troubleshooting agents
* documentation agents

Its purpose is to preserve architectural decisions, deployment boundaries, security requirements, implementation phases, validation expectations, and operational context across separate conversations and agent sessions.

Agents working on the Hetzner environment should read this document before recommending or applying infrastructure changes.

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

Main project context:

```text
docs/general-context.md
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
* the Go synthetic network probe
* the SmartEnergy application

Those components are developed in the project-development workflow and deployed through the infrastructure defined here.

---

## 3. Infrastructure Objective

The immediate objective is to create a secure, reproducible, publicly accessible Kubernetes environment on a Hetzner Cloud VM.

The environment will host:

* the Go Kubernetes Operator
* the Go control-plane API
* the managed Go demo application
* the OpenTelemetry Collector
* the Grafana observability environment
* the synthetic network probe
* Argo CD
* the SmartEnergy API and dashboard
* PostgreSQL
* Redis
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

**Current account status:** Existing Hetzner account available

**Current VM status:** No authoritative active VM is assumed

**Current deployment status:** Not deployed

**Current Kubernetes status:** Not installed on Hetzner

**Local development environment:** WSL on Windows

**Local Kubernetes target:** k3d/K3s

**Public Kubernetes target:** Single-node K3s

**Current phase:** Infrastructure planning

No agent should assume that a VM, firewall, domain, DNS record, cluster, registry credential, or deployed workload already exists unless the current infrastructure state has been inspected and documented.

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

## 6. Target Architecture

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
   Control-Plane API        Managed Applications       Protected Grafana
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
 ManagedService CRs          Logs / Metrics / Traces      GitOps Reconciliation
                                     |
                       +-------------+-------------+
                       |             |             |
                       v             v             v
                     Loki          Tempo     Prometheus/Mimir
                                     |
                                  Grafana
```

---

## 7. Initial Environment Model

The initial deployment will use:

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
External DNS records managed outside Kubernetes initially
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

## 8. Proposed Kubernetes Namespace Model

The initial namespace plan is:

```text
kube-system
cert-manager
argocd
platform-system
applications
smartenergy
observability
monitoring
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
* synthetic test workloads

### `smartenergy`

Contains:

* SmartEnergy FastAPI application
* dashboard
* PostgreSQL
* Redis
* migration Jobs
* SmartEnergy-specific Secrets and ConfigMaps

### `observability`

Contains:

* OpenTelemetry Collector
* Loki
* Tempo
* Prometheus or Mimir
* Grafana
* observability configuration

### `argocd`

Contains:

* Argo CD components

### `cert-manager`

Contains:

* cert-manager components

The final namespace structure may change when deployment packaging is implemented, but namespace ownership should remain explicit.

---

## 9. Initial VM Requirements

The final Hetzner VM type must be selected based on:

* expected K3s overhead
* observability-stack memory usage
* PostgreSQL requirements
* Redis requirements
* application workload count
* budget
* x86 versus ARM compatibility
* container-image architecture availability

A reasonable initial target is expected to require approximately:

```text
4–8 vCPUs
8–16 GB RAM
80 GB or more local storage
Ubuntu LTS
```

A smaller VM may be used for the earliest bootstrap, but the complete platform should not be deployed onto an undersized machine merely to minimise cost.

Before selecting the VM, the infrastructure conversation should estimate resource usage for:

* K3s
* Traefik
* cert-manager
* Argo CD
* Operator
* control-plane API
* demo applications
* OpenTelemetry Collector
* Grafana
* Loki
* Tempo
* Prometheus or Mimir
* SmartEnergy API
* PostgreSQL
* Redis

The exact server type remains an open decision until this sizing review is completed.

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

Agents must not disable security controls simply because an installation guide suggests doing so without explaining the consequence.

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

The first cluster will likely use K3s local-path storage.

This is acceptable for the portfolio environment but has important limitations:

* volumes are tied to the node
* node loss can cause data loss
* no multi-node replication
* storage failure recovery is manual
* backups are essential

Persistent workloads include:

* PostgreSQL
* Grafana state if not provisioned declaratively
* Loki data
* Tempo data
* Prometheus or Mimir data
* Argo CD state
* SmartEnergy data

The deployment plan must determine:

* which data must persist
* which data can be recreated
* retention durations
* volume sizes
* backup destinations
* restore procedures

Observability retention should remain deliberately limited to control disk growth.

---

## 19. Backup and Recovery Strategy

A portfolio environment still requires a documented recovery plan.

The strategy should distinguish:

### 19.1 Reproducible Configuration

Recoverable from Git:

* Kubernetes manifests
* Helm values
* Argo CD applications
* dashboards
* alert rules
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

Must include:

* PostgreSQL logical backups
* SmartEnergy application data
* any irreplaceable generated data

### 19.4 Recovery Validation

A backup is not considered reliable until a restore procedure has been tested.

The first recovery exercise should validate at least:

1. rebuilding a replacement VM
2. reinstalling K3s
3. restoring GitOps configuration
4. restoring PostgreSQL
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

Argo CD will become the primary deployment reconciler after the environment bootstrap.

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

Automatic sync should not be enabled until failed-deployment recovery is understood.

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

## 24. Grafana Access Policy

Grafana may eventually be shown during interviews, but access must remain controlled.

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

The first observability deployment may use a simplified stack.

Expected components:

* OpenTelemetry Collector
* Grafana
* Loki
* Tempo
* Prometheus or Mimir

The infrastructure plan must consider:

* memory limits
* CPU limits
* persistent storage
* retention
* compaction
* log volume
* trace sampling
* metric cardinality
* dashboard provisioning
* alert-rule provisioning
* authentication
* public exposure

The observability stack is likely the most resource-intensive component.

It should be deployed after the base cluster and application workloads are stable.

---

## 26. SmartEnergy Deployment

The SmartEnergy deployment will contain:

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

The initial environment may use:

* one PostgreSQL instance
* one Redis instance
* one FastAPI replica
* one dashboard replica

This is acceptable for demonstration purposes.

The documentation must state that this is not a highly available database architecture.

Required operational checks:

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

The control-plane platform includes:

```text
Go Kubernetes Operator
Go control-plane REST API
ManagedService CRD
RBAC resources
ServiceAccount
Services
Ingress
ConfigMaps
Secrets
Observability configuration
```

The deployment order should be:

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

The API and Operator should use separate service accounts unless there is a strong reason to combine them.

Each component should receive only the permissions it requires.

---

## 28. Security Boundaries

### Public Boundary

May include:

* control-plane API
* demo application
* SmartEnergy dashboard
* selected SmartEnergy API endpoints
* protected Grafana dashboards

### Cluster-Internal Boundary

Includes:

* Operator metrics
* OpenTelemetry receivers
* PostgreSQL
* Redis
* internal application Services
* Prometheus
* Loki
* Tempo
* Argo CD server
* Kubernetes API access

### Administrative Boundary

Includes:

* SSH
* `kubectl`
* Argo CD administration
* Grafana administration
* database administration
* backup access
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
IN PROGRESS
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

---

### Phase H8 — Observability Deployment

Deliverables:

* OpenTelemetry Collector
* Grafana
* Loki
* Tempo
* Prometheus or Mimir
* dashboards
* alert rule
* retention configuration

Exit criteria:

* logs are searchable
* metrics are visible
* traces are visible
* Operator reconciliation telemetry is visible
* application requests can be investigated
* disk growth is bounded
* access is protected

---

### Phase H9 — SmartEnergy Deployment

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

---

### Phase H10 — Network Probe Deployment

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

---

### Phase H11 — Backup and Recovery Validation

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

---

### Phase H12 — Public Demo Hardening

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

---

## 33. Initial Deployment Priority

When time is limited, follow this order:

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
10. SmartEnergy
11. Network probe
12. Advanced hardening
```

A secure working platform demonstration is more valuable than a complete but unstable stack.

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

## 35. Agent Roles

A coordinated infrastructure agent team may divide work into the following roles.

### Infrastructure Lead

Owns:

* phase planning
* architecture consistency
* dependency ordering
* risk review
* completion criteria
* coordination across agents

### Linux and Security Agent

Owns:

* VM bootstrap
* SSH
* OS hardening
* patching
* firewall validation
* user permissions
* host-level security

### Kubernetes Agent

Owns:

* K3s installation
* cluster configuration
* namespaces
* storage
* ingress
* RBAC
* cluster validation
* upgrades

### Networking and TLS Agent

Owns:

* DNS
* firewall rules
* ingress routing
* cert-manager
* TLS
* public exposure inventory

### GitOps and CI Agent

Owns:

* GHCR
* GitHub Actions coordination
* image tags
* Argo CD
* deployment repositories
* rollback procedures

### Observability Agent

Owns:

* OpenTelemetry Collector
* Loki
* Tempo
* Prometheus or Mimir
* Grafana
* alerts
* telemetry retention
* incident dashboards

### Database and State Agent

Owns:

* PostgreSQL
* Redis
* persistent volumes
* migrations
* backups
* restores
* stateful workload validation

### Security Review Agent

Owns:

* threat review
* RBAC review
* ingress review
* Secret review
* image review
* public exposure verification
* completion security report

No agent should make overlapping destructive changes without coordinating through the infrastructure lead.

---

## 36. Agent Working Rules

Before making infrastructure changes, agents must:

1. Read this document.
2. Read the main project context.
3. Inspect the current infrastructure state.
4. Identify the active deployment phase.
5. Review previous completion reports.
6. Confirm whether commands are local, remote, or Kubernetes-scoped.
7. Identify possible destructive consequences.
8. Preserve existing working access.

While making changes, agents must:

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

Before declaring completion, agents must:

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

Agents should prefer reversible changes.

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

The infrastructure conversation should maintain an inventory similar to:

```text
Provider:
Hetzner Cloud

Project:
TBD

Server name:
TBD

Server type:
TBD

Region:
TBD

Operating system:
TBD

Public IPv4:
REDACTED IN PUBLIC DOCS

Public IPv6:
REDACTED OR DOCUMENTED SAFELY

Administrative user:
TBD

SSH source restrictions:
TBD

K3s version:
TBD

Kubernetes version:
TBD

Domain:
TBD

DNS provider:
TBD

Ingress controller:
Traefik

Certificate manager:
cert-manager

Container registry:
GHCR

GitOps:
Argo CD

Storage class:
TBD

Backup location:
TBD
```

Sensitive values must not be added to a public repository.

A sanitised inventory may be committed.

A private inventory may be maintained separately.

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

## 44. Production-Hardening Roadmap

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

These are future improvements, not current claims.

---

## 45. Multi-Cluster and Air-Gapped Interview Scope

The infrastructure team may later prepare design documentation for:

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

## 46. Chat Separation

### Master Interview Preparation Chat

Owns:

* role priorities
* overall schedule
* project prioritisation
* interview strategy
* cross-chat decisions

### Project Development Chat

Owns:

* Go code
* Operator
* REST API
* demo service
* network probe
* SmartEnergy Kubernetes packaging
* tests
* application documentation

### Hetzner Infrastructure Chat

Owns:

* Hetzner project
* VM provisioning
* OS hardening
* K3s
* firewall
* DNS
* TLS
* storage
* GitOps deployment
* backups
* public exposure
* infrastructure runbooks

### Interview Preparation Chat

Owns:

* technical questions
* system-design practice
* behavioural preparation
* live project explanation
* troubleshooting exercises

Important deployment decisions made in the infrastructure chat should be reflected in this document and, where architecturally relevant, in the main project context.

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
* Backups and restore instructions are required.
* The observability stack must use bounded retention.
* The infrastructure will be built incrementally.
* Local validation should precede public deployment.
* Secrets must never be committed to Git.
* No high-availability claim should be made for the initial environment.

---

## 48. Current Open Decisions

The following decisions remain open:

* exact Hetzner server type
* exact Hetzner region
* x86 versus ARM
* exact Ubuntu LTS release
* real domain and subdomain structure
* DNS provider
* HTTP-01 versus DNS-01 ACME validation
* Kubernetes API access strategy
* VPN versus source-IP restriction versus SSH tunnel
* exact persistent-volume sizes
* backup destination
* Secret-management mechanism
* Prometheus versus Mimir for the first public deployment
* Grafana public-access mechanism
* Argo CD public-access mechanism
* GitOps repository structure
* observability retention periods
* K3s upgrade strategy
* VM snapshot policy
* whether additional Hetzner volumes are required

Open decisions should be resolved at the phase where they become necessary.

The simplest safe and reversible option should be preferred.

---

## 49. Immediate Next Infrastructure Step

The next infrastructure planning task is:

```text
Phase H0, Slice H0.1 — Finalise initial VM sizing, region, domain assumptions, access model, and monthly cost.
```

The first implementation task after planning will be:

```text
Phase H1, Slice H1.1 — Create the Hetzner VM, SSH access, and minimal firewall without installing Kubernetes.
```

The VM should not be provisioned until the following are agreed:

* server architecture
* server size
* Hetzner region
* SSH public key
* administrative source-IP strategy
* initial firewall rules
* estimated monthly cost

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
* how stateful services persist data
* how logs, metrics, and traces are collected
* how administrative systems remain protected
* how backups and recovery work
* how failures are diagnosed
* how the single-node design differs from production high availability
* how the architecture could evolve toward multi-cluster and air-gapped deployments
