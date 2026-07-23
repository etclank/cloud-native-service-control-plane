# Cloud-Native Service Control Plane

A portfolio platform for building, deploying, operating, and observing Kubernetes-managed services with Go, Kubernetes operators, GitOps, and OpenTelemetry.

> Current status: H7 platform deployment and H8.3 Collector foundation complete. The Kubebuilder operator, authenticated control-plane API, managed `demo-http` workload, immutable GHCR images, restricted GitOps delivery, production API TLS, and a private, network-restricted OpenTelemetry Collector are live. Later H8 backends and workload OTLP export remain deferred.

## Project Purpose

This project demonstrates an end-to-end cloud-native platform rather than a single isolated application. Its intended capabilities include:

- a Go Kubernetes Operator and `ManagedService` custom resource;
- a Go control-plane API;
- managed Go demonstration workloads;
- GitHub Actions and GitHub Container Registry;
- GitOps delivery through Argo CD;
- metrics, logs, and traces through an OpenTelemetry-based observability stack;
- a synthetic network probe;
- deployment of the existing SmartEnergy API and dashboard;
- documented security, backup, recovery, and operational procedures.

The platform is designed as:

- an interview portfolio project;
- a practical Kubernetes learning environment;
- a cloud-native deployment laboratory;
- a reproducible example of operating workloads on a low-cost public VM.

## Current Live Foundation

The implemented environment currently provides:

- a Hetzner Cloud VM running Ubuntu 24.04 LTS;
- a pinned single-node K3s cluster;
- hardened SSH access through a dedicated key and non-root administrator;
- a Hetzner Cloud Firewall exposing only approved public ports;
- private Kubernetes administration through an SSH tunnel;
- Traefik ingress;
- CoreDNS, Metrics Server, ServiceLB, and local-path persistent storage;
- Kubernetes Secret encryption at rest;
- cert-manager with Let's Encrypt staging and production issuers;
- trusted public HTTPS with automatic renewal;
- permanent HTTP-to-HTTPS redirection;
- a lightweight public validation endpoint;
- a private GitHub repository and linked private GHCR package;
- a SHA-tagged GitHub Actions container build;
- private K3s registry authentication using a read-only credential;
- a digest-pinned internal registry validation workload;
- source-to-runtime image traceability;
- Argo CD with private administration, restricted projects, and manual synchronization;
- a Kubebuilder v4.15.0 operator serving `platform.eoghanclancy.eu/v1alpha1`;
- `ManagedService` reconciliation into owned Deployments and ClusterIP Services;
- a public, bearer-authenticated control-plane API at `https://api.platform.eoghanclancy.eu`;
- production Let's Encrypt TLS, permanent HTTPS redirection, and Traefik rate limiting;
- digest-pinned operator, API, and `demo-http` workloads.
- a GitOps-managed OpenTelemetry Collector with restricted OTLP ingress and a
  `nop` exporter only.

Current validation endpoint:

```text
https://test.platform.eoghanclancy.eu
```

Expected response:

```text
tls-production-validation-ok
```

Current control-plane API:

```text
https://api.platform.eoghanclancy.eu
```

## Architecture

```text
                              Public Internet
                                     |
                           eoghanclancy.eu DNS
                                     |
                         Hetzner Cloud Firewall
                           public TCP 80 / 443
                                     |
                        Ubuntu 24.04 Hetzner VM
                                     |
                         Single-node K3s cluster
                                     |
                    Traefik ingress + cert-manager
                                     |
           +-------------------------+-------------------------+
           |                         |                         |
           v                         v                         v
   Control-plane API       Managed applications       Protected dashboards
           |                         |                         |
           +-------------------------+-------------------------+
                                     |
                         Kubernetes internal network
                                     |
       +-----------------------------+-----------------------------+
       |                             |                             |
       v                             v                             v
 Kubernetes Operator      OpenTelemetry Collector              Argo CD
       |                             |                             |
       v                             v                             v
 ManagedService CRs          Logs / metrics / traces      GitOps reconciliation
                                     |
                       +-------------+-------------+
                       |             |             |
                       v             v             v
                     Loki          Tempo       Prometheus
                                     |
                                  Grafana


Administrative path:

kubectl in WSL
      |
      | https://127.0.0.1:16443
      v
SSH tunnel over restricted TCP 22
      |
      | remote 127.0.0.1:6443
      v
private K3s Kubernetes API
```

The operator, API, Argo CD, managed `demo-http` path, and bounded
OpenTelemetry Collector foundation are implemented. Prometheus, Loki, Tempo,
Grafana, workload OTLP export, SmartEnergy, and the synthetic probe remain
later work.

## Infrastructure Baseline

| Item | Current value |
| --- | --- |
| Provider | Hetzner Cloud |
| Server | `portfolio-k3s-01` |
| Server type | CX23 shared cost-optimized x86 |
| Resources | 2 vCPU, 4 GB RAM, 40 GB SSD |
| Operating system | Ubuntu 24.04 LTS |
| Kubernetes | K3s `v1.36.2+k3s1` |
| Ingress controller | Traefik |
| Certificate controller | cert-manager `v1.21.0` |
| Domain | `eoghanclancy.eu` |
| Public application ports | TCP 80 and 443 |
| Administrative access | Restricted SSH and local API tunnel |

The server is intentionally small for the bootstrap and low-traffic portfolio workload. It can be resized when later phases introduce memory-heavy observability and data services.

## Platform Components

### Kubernetes Operator

The Go operator reconciles a `ManagedService` resource into an owned Deployment and ClusterIP Service. The current API is `platform.eoghanclancy.eu/v1alpha1`, and the approved template is `demo-http`.

Implemented behavior includes:

- replicas defaulting to `1` with an allowed range of `1`–`3`;
- messages up to 120 characters;
- immutable approved-image validation;
- owner references and Kubernetes garbage collection;
- drift correction through idempotent reconciliation;
- `Available`, `Progressing`, and `Degraded` status conditions;
- ready-replica and internal endpoint reporting.

### Control-plane API

The Go control-plane API is available at `https://api.platform.eoghanclancy.eu`. `GET /healthz` and `GET /readyz` are public health endpoints. Routes under `/api/v1` require a bearer token loaded by the API from a mounted Secret file. The lifecycle API can create, list, get, and delete `ManagedService` resources only in the fixed `applications` namespace and only with the fixed `demo-http` template.

The token value is never stored or displayed in this repository.

### Managed Demonstration Workload

A small Go service demonstrates the complete lifecycle:

```text
API request
  -> ManagedService resource
  -> operator reconciliation
  -> Deployment and Service
  -> internal endpoint and status
```

The live `portfolio-demo` resource reports `Available=True`, `readyReplicas=1`, and returns its configured message through the internal Service. Its telemetry instrumentation is present, but production OTLP export remains disabled pending a separately reviewed later H8 slice.

### Observability

The first observability component is live: a private OpenTelemetry Collector
deployed through restricted, manual-sync GitOps. NetworkPolicy admits only the
reviewed OTLP client identities on TCP 4317 and 4318. Both authorized
OTLP/HTTP paths were validated, unauthorized identities and TCP 13133 were
blocked, and the Collector remains configured with only a `nop` exporter. No
production workload currently exports telemetry.

The repository-side Prometheus implementation is also complete and awaiting a
separately authorized manual deployment. It contains a standalone,
resource-bounded Prometheus, reduced kube-state-metrics, a 3Gi local-path PVC,
72-hour/2GB retention, six exact private scrape jobs, least-privilege RBAC,
and restricted NetworkPolicies. Kubelet and cAdvisor are optional post-H8
enhancements rather than H8 completion requirements.

Future post-H8 observability enhancements may include:

- Loki for logs;
- Tempo for traces;
- Grafana dashboards;
- a synthetic Go network probe.

H8 now requires only the controlled Prometheus live deployment/validation and
evidence closeout. No additional backend is an H8 acceptance requirement.

### SmartEnergy

The existing SmartEnergy FastAPI, PostgreSQL, Redis, and dashboard project will be deployed as a second workload. It will demonstrate that the platform can host both Go-native components and an existing Python application stack.

## Repository Structure

The repository contains the operator, two Go HTTP binaries, immutable image workflows, Kubernetes packages, GitOps bootstrap resources, tests, and operating documentation:

```text
cloud-native-service-control-plane/
├── .github/
│   └── workflows/
├── api/
├── cmd/
├── config/
├── deploy/
├── docs/
│   ├── cloud-native-service-control-plane-build-guide.md
│   ├── h7-platform-deployment-closeout.md
│   ├── infrastructure-context.md
│   └── operator-guide.md
├── images/
├── internal/
├── kubernetes/
│   ├── bootstrap/
│   ├── platform/
│   └── validation/
├── test/
├── Dockerfile
├── Makefile
├── go.mod
└── README.md
```

Directories for components that have not yet been implemented may be introduced by the development workflow rather than as empty placeholders.

## Documentation

| Document | Purpose |
| --- | --- |
| [`docs/cloud-native-service-control-plane-build-guide.md`](docs/cloud-native-service-control-plane-build-guide.md) | Educational reconstruction guide with tested H-phase commands |
| [`docs/operator-guide.md`](docs/operator-guide.md) | Daily SSH, tunnel, Kubernetes, and troubleshooting runbook |
| [`docs/infrastructure-context.md`](docs/infrastructure-context.md) | Authoritative infrastructure requirements and phase model |
| [`docs/h7-platform-deployment-closeout.md`](docs/h7-platform-deployment-closeout.md) | H7 implementation record, validation evidence, security boundary, and exit criteria |
| [`docs/h8-collector-deployment-closeout.md`](docs/h8-collector-deployment-closeout.md) | H8.3D Collector deployment, health, NetworkPolicy, and Gate 3V-R evidence |
| [`docs/h8-observability-design.md`](docs/h8-observability-design.md) | H8 architecture, resource budget, retention, security, and ordered implementation roadmap |
| [`docs/argocd-sync-rollback-runbook.md`](docs/argocd-sync-rollback-runbook.md) | Manual Argo CD synchronization and rollback procedure |

Planned documentation includes:

- complete system explanation;
- learning and interview revision guide;
- architecture decision records;
- deployment and rollback runbooks;
- certificate, backup, and recovery procedures;
- final threat model and public-demo checklist.

## Administrative Quick Start

The Kubernetes API is not exposed publicly. Start the SSH tunnel from WSL:

```bash
export KUBECONFIG="$HOME/.kube/portfolio-k3s.yaml"

ssh \
  -M \
  -S "$HOME/.ssh/controlmasters/portfolio-k3s-tunnel.sock" \
  -fNT \
  -o ExitOnForwardFailure=yes \
  -L 127.0.0.1:16443:127.0.0.1:6443 \
  portfolio-k3s
```

Validate access:

```bash
kubectl get nodes
kubectl get pods --all-namespaces
```

Stop the tunnel:

```bash
ssh \
  -S "$HOME/.ssh/controlmasters/portfolio-k3s-tunnel.sock" \
  -O exit \
  portfolio-k3s
```

See [`docs/operator-guide.md`](docs/operator-guide.md) for the complete procedure and troubleshooting steps.

## Infrastructure Progress

| Phase | Status | Outcome |
| --- | --- | --- |
| H0 — Planning and decisions | Complete | Initial architecture, security boundaries, and phased delivery model |
| H1 — Hetzner project and VM | Complete | VM, dedicated SSH key, firewall, non-root administration |
| H2 — Operating-system hardening | Complete | Updated Ubuntu, hardened SSH, automatic security updates, bounded logs |
| H3 — K3s installation | Complete | Healthy pinned cluster, private administration, DNS/Ingress/PVC validation |
| H4 — DNS and TLS | Complete | Public domain, cert-manager, production certificate, HTTPS redirect |
| H5 — GHCR and CI access | Complete | Private package, immutable build identity, read-only pull Secret, digest-pinned deployment |
| H6 — Argo CD bootstrap | Complete | Private Argo CD, restricted projects, manual synchronization, tested rollback |
| H7 — Platform deployment | Complete | Operator, authenticated API, managed workload, immutable images, GitOps, TLS |
| H8 — Observability deployment | In progress | H8.3 Collector foundation validated; Prometheus and later slices remain |
| H9 — SmartEnergy deployment | Not started | API, dashboard, PostgreSQL, and Redis |
| H10 — Network probe | Not started | Synthetic connectivity and telemetry workload |
| H11 — Backup and recovery | Not started | Tested backup and restoration procedures |
| H12 — Public demo hardening | Not started | Final exposure, security, reliability, and demonstration checks |

## Delivery Principles

### Local first

Application code, tests, manifests, and packaging should be validated locally before public deployment. The Hetzner VM is not the primary development environment.

### Incremental validation

Infrastructure is introduced in small slices. Each slice must demonstrate real behavior before the next dependency is added.

### GitOps-oriented operation

After bootstrap, deployment configuration will be version controlled and reconciled through Argo CD rather than maintained as undocumented manual state.

### Minimal public exposure

Only approved application endpoints use public ingress. Kubernetes, databases, telemetry receivers, and administration interfaces remain private unless a later secured design explicitly changes that decision.

### Immutable deployment identity

Container images will be traceable to Git commits. Kubernetes deployments will use immutable image digests rather than relying on a mutable `latest` tag.

### Security before convenience

Credentials remain outside Git. GitHub Actions will use narrowly scoped workflow tokens, and the cluster will avoid long-lived registry credentials when public images permit anonymous pulls.

## Security Posture

Current controls include:

- dedicated passphrase-protected SSH key;
- disabled SSH password authentication;
- disabled direct root SSH;
- restricted SSH source IP at the Hetzner firewall;
- only TCP 80 and 443 public for applications;
- private Kubernetes API accessed through SSH tunnelling;
- Ubuntu automatic security upgrades without automatic reboot;
- AppArmor enabled;
- bounded journal retention;
- K3s Secret encryption;
- trusted HTTPS and automatic certificate renewal;
- separate least-privilege operator and API ServiceAccounts;
- API RBAC limited to create/get/list/delete `ManagedService` resources in `applications`;
- non-root containers, dropped capabilities, read-only root filesystems, and RuntimeDefault seccomp;
- digest-qualified runtime images;
- private Argo CD administration and manual synchronization;
- no kubeconfig, private keys, tokens, or Secret values in Git.

Security validation is continuous. This list describes implemented controls, not a claim of formal certification or production hardening.

## Current Limitations

The initial platform uses one VM and one Kubernetes node. It does not provide:

- node or control-plane redundancy;
- multi-zone availability;
- highly available persistent storage;
- managed database availability;
- automatic disaster recovery;
- zero-downtime node maintenance;
- production service-level guarantees.

These are conscious cost and complexity trade-offs for a portfolio environment. The project documentation will continue to distinguish demonstrated behavior from production-grade claims.

## Current Validation

H7 live validation established that:

- the operator and API Argo Applications are `Synced` and `Healthy`;
- the CRD is installed and operator, API, and demo Pods are Ready with zero restarts;
- HTTP redirects permanently to HTTPS and the production certificate is valid;
- public health succeeds, unauthenticated API access returns `401`, and authenticated access succeeds;
- API-created `ManagedService` resources reconcile into owned child resources;
- status reports `Available=True` and `readyReplicas=1`;
- replica drift from `1` to `2` is restored to `1`;
- deletion removes the custom resource and its owned Deployment and Service;
- the external scan exposes only ports 22, 80, and 443 from the tested source;
- the node baseline was approximately 6% CPU and 66% memory.

Safe public checks that do not require a bearer token:

```bash
dig +short A api.platform.eoghanclancy.eu

curl -sSI http://api.platform.eoghanclancy.eu/healthz

curl -fsS https://api.platform.eoghanclancy.eu/healthz
```

Expected results:

- DNS resolves to the current Hetzner IPv4;
- HTTP returns a permanent redirect to HTTPS;
- HTTPS returns a JSON healthy response using a publicly trusted certificate.

Authenticated calls are documented in the [operator guide](docs/operator-guide.md) and deliberately avoid placing the bearer token in command arguments or documentation.

## License

A project license has not yet been selected. Until a license file is added, no open-source usage rights should be assumed.
