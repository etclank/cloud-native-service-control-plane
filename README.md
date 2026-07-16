# Cloud-Native Service Control Plane

A portfolio platform for building, deploying, operating, and observing Kubernetes-managed services with Go, Kubernetes operators, GitOps, and OpenTelemetry.

> Current status: infrastructure foundation complete through H4 (DNS and trusted TLS). Application and platform components are planned but are not yet deployed.

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
- a lightweight public validation endpoint.

Current validation endpoint:

```text
https://test.platform.eoghanclancy.eu
```

Expected response:

```text
tls-production-validation-ok
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

Components shown below Traefik that are not listed as implemented in the current status remain planned work.

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

## Target Platform Components

### Kubernetes Operator

The Go operator will reconcile a custom `ManagedService` resource into the Kubernetes resources needed to run a managed workload. Planned responsibilities include:

- desired-state validation;
- Deployment and Service management;
- configuration and Secret references;
- status conditions;
- reconciliation after drift;
- safe update and deletion behavior;
- observability integration.

### Control-plane API

The Go control-plane API will provide a user-facing REST interface over the platform domain. It will translate supported API operations into Kubernetes resources while retaining clear validation, authorization, and error boundaries.

### Managed Demonstration Workload

A small Go service will demonstrate the complete lifecycle:

```text
API request
  -> ManagedService resource
  -> operator reconciliation
  -> Deployment and Service
  -> ingress and TLS
  -> telemetry and operational status
```

### Observability

The target observability environment includes:

- OpenTelemetry Collector;
- Prometheus or a compatible metrics backend;
- Loki for logs;
- Tempo for traces;
- Grafana dashboards;
- Kubernetes and application telemetry;
- a synthetic Go network probe.

The final backend selection and resource limits will be validated against the small VM before the complete stack is enabled.

### SmartEnergy

The existing SmartEnergy FastAPI, PostgreSQL, Redis, and dashboard project will be deployed as a second workload. It will demonstrate that the platform can host both Go-native components and an existing Python application stack.

## Repository Structure

The repository begins with infrastructure and documentation and will expand as application development proceeds:

```text
cloud-native-service-control-plane/
├── .github/
│   └── workflows/
├── api/
├── cmd/
├── configs/
├── deploy/
├── docs/
│   ├── architecture/
│   ├── decisions/
│   ├── runbooks/
│   ├── build-guide.md
│   ├── infrastructure-context.md
│   └── operator-guide.md
├── examples/
├── infrastructure/
├── internal/
├── kubernetes/
│   ├── bootstrap/
│   └── validation/
├── observability/
├── pkg/
├── scripts/
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
| [`docs/build-guide.md`](docs/build-guide.md) | Educational reconstruction guide with tested H-phase commands |
| [`docs/operator-guide.md`](docs/operator-guide.md) | Daily SSH, tunnel, Kubernetes, and troubleshooting runbook |
| [`docs/infrastructure-context.md`](docs/infrastructure-context.md) | Authoritative infrastructure requirements and phase model |

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
| H5 — GHCR and CI access | In progress | Container build, publication, immutable tags, cluster pull validation |
| H6 — Argo CD bootstrap | Not started | GitOps controller and initial Application |
| H7 — Platform deployment | Not started | Operator, control-plane API, and managed workload |
| H8 — Observability deployment | Not started | Metrics, logs, traces, dashboards |
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

Public DNS and TLS smoke test:

```bash
dig +short A test.platform.eoghanclancy.eu

curl -sSI http://test.platform.eoghanclancy.eu/

curl -fsS https://test.platform.eoghanclancy.eu/
```

Expected results:

- DNS resolves to the current Hetzner IPv4;
- HTTP returns a permanent redirect to HTTPS;
- HTTPS returns `tls-production-validation-ok` using a publicly trusted certificate.

## License

A project license has not yet been selected. Until a license file is added, no open-source usage rights should be assumed.
