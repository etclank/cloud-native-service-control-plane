# H7 Platform Deployment Closeout

> Historical portfolio record: deployment observations apply to the revisions
> and dates recorded below, not current availability. Review environment-specific
> commands before use; see the [documentation index](README.md).

**Closeout date:** 2026-07-20
**Phase:** H7 — Platform deployment
**Result:** Complete

## Purpose

This document records the implemented H7 platform, its live validation evidence, its security boundary, and the conditions under which the phase was closed. It contains resource and Secret names but no credential, bearer-token, private-key, or certificate-key value.

H7 demonstrates a complete cloud-native control loop:

```text
authenticated public API request
  -> constrained ManagedService desired state
  -> Kubernetes API and CRD validation
  -> operator reconciliation
  -> owned Deployment and ClusterIP Service
  -> Pod readiness and internal response
  -> ManagedService status
  -> drift correction and garbage-collected deletion
```

## Implementation Summary

### Operator and custom resource

The operator foundation uses Kubebuilder v4.15.0. It serves:

```text
platform.eoghanclancy.eu/v1alpha1
kind: ManagedService
```

The current contract permits only the `demo-http` template:

- replicas default to `1`;
- replicas are limited to `1`–`3`;
- the optional message is limited to 120 characters;
- the operator creates a Deployment and ClusterIP Service;
- both children receive controller owner references;
- Kubernetes garbage collection removes owned children after deletion;
- idempotent reconciliation restores managed fields after drift;
- status reports the internal endpoint and ready replicas;
- conditions are `Available`, `Progressing`, and `Degraded`.

### Control-plane API

The public API endpoint is:

```text
https://api.platform.eoghanclancy.eu
```

Public endpoints:

```text
GET /healthz
GET /readyz
```

Authenticated endpoints:

```text
POST   /api/v1/managed-services
GET    /api/v1/managed-services
GET    /api/v1/managed-services/{name}
DELETE /api/v1/managed-services/{name}
```

The API always manages the `applications` namespace and always selects the `demo-http` template. Clients cannot supply another namespace, image, template, metadata object, label set, annotation set, status object, or raw Kubernetes manifest.

Authentication uses one bearer token loaded from a mounted file backed by `platform-system/control-plane-api-token`. The token value is intentionally absent from Git and this document.

### GitOps

The restricted AppProject is `platform-control-plane`. It accepts only the private repository and the exact destinations `platform-system` and `applications`. Cluster and namespaced resource kinds are explicitly enumerated rather than wildcarded.

Platform Applications:

| Application | Repository path | Destination | Sync mode |
| --- | --- | --- | --- |
| `platform-operator` | `config/default` | `platform-system` | Manual |
| `control-plane-api` | `kubernetes/platform/control-plane-api` | `platform-system`, with explicit cross-namespace resources | Manual |

Argo CD remains private. Administration uses the Kubernetes SSH tunnel and a loopback-only port-forward.

### Public TLS and routing

cert-manager uses the production `letsencrypt-production` ClusterIssuer. The `control-plane-api` Certificate requests exactly `api.platform.eoghanclancy.eu` and writes the resulting key pair to `platform-system/control-plane-api-tls`.

Traefik provides:

- public HTTP on port 80;
- public HTTPS on port 443;
- permanent HTTP-to-HTTPS redirection;
- TLS termination for the exact API hostname;
- an average rate limit of 5 requests per second with a burst of 10;
- routing to only the internal `control-plane-api` ClusterIP Service.

There is no public Argo CD route, Kubernetes API route, NodePort, application LoadBalancer, database port, or telemetry receiver.

## Immutable Runtime Images

### Operator

```text
ghcr.io/etclank/cloud-native-service-control-plane-operator@sha256:377af6a1fb4df40c52d6be37d4e948ca1990fe4c0f840789da68fc3e449ffc75
```

### Demo workload

```text
ghcr.io/etclank/cloud-native-service-control-plane-demo-http@sha256:2d1fc30e0cf75ba9fbe96af176f770524377ee5349acbee9ed94ae13f1143b2f
```

### Control-plane API

```text
ghcr.io/etclank/cloud-native-service-control-plane-api@sha256:22ffdf07c24af219a1fe493095c3293c480da4f3cdc04ccc167e65ae81d631ad
```

GitHub Actions publishes traceable full-commit tags, SBOMs, and maximum provenance, while Kubernetes selects image content by digest rather than a mutable tag.

## Security Boundary

### Identities and authorization

- The operator and API use separate ServiceAccounts.
- The API Role exists only in `applications`.
- The API can create, get, list, and delete only `ManagedService` resources.
- The API cannot access Secrets, Deployments, Services, other namespaces, status, finalizers, update, or patch.
- The API has no ClusterRole or ClusterRoleBinding.

### Runtime hardening

- Operator, API, and demo workloads run as non-root.
- Linux capabilities are dropped.
- Root filesystems are read-only.
- RuntimeDefault seccomp is enabled.
- Resource requests and limits are set.
- Health and readiness probes bound rollout decisions.
- Managed workload Pods do not receive a Kubernetes ServiceAccount token.

### Secret boundaries

Only these names are part of the H7 record:

| Secret | Namespace | Purpose |
| --- | --- | --- |
| `ghcr-pull` | `platform-system` | Operator and API image pulls |
| `ghcr-pull` | `applications` | Managed workload image pulls |
| `control-plane-api-token` | `platform-system` | API bearer-token file |
| `control-plane-api-tls` | `platform-system` | cert-manager-generated TLS material |

Kubernetes image pull Secrets are namespace-scoped, which is why the same name is created independently in both permitted workload namespaces. No Secret manifest or value is committed.

### Network boundary

- SSH port 22 is restricted to an approved administrative `/32` source.
- Public application traffic uses only ports 80 and 443.
- The Kubernetes API remains private and is reached through an SSH tunnel.
- Ports 6443, 5432, 6379, 8080, and 9090 were externally filtered during closeout validation.
- Argo CD remains reachable only through private port-forwarding.

## Live Validation Summary

The H7 closeout observed:

- `platform-operator` and `control-plane-api` Argo Applications `Synced` and `Healthy`;
- the `ManagedService` CRD installed;
- operator, API, and demo Pods Ready with zero restarts;
- public health succeeding over trusted HTTPS;
- HTTP returning `308` to HTTPS;
- unauthenticated lifecycle access returning `401`;
- authenticated lifecycle access succeeding;
- `portfolio-demo` created through the API;
- its child Deployment and Service becoming Ready;
- the internal workload returning the configured message;
- status reporting `Available=True` and `readyReplicas=1`;
- replica drift from `1` to `2` being restored to `1`;
- `lifecycle-check` deletion returning `204`;
- the deleted custom resource and owned children being removed;
- `portfolio-demo` remaining active;
- no warning events;
- approximately 6% node CPU and 66% node memory use;
- only 22, 80, and 443 open in the external port scan;
- zero failed host services;
- a valid production certificate for `api.platform.eoghanclancy.eu`.

## Exit-Criteria Matrix

| Criterion | Evidence | Result |
| --- | --- | --- |
| CRD installed | Kubernetes accepted `ManagedService` resources | Pass |
| Operator ready | Operator Deployment and Pod Ready with zero restarts | Pass |
| API ready | API Deployment and Pod Ready; `/readyz` succeeded | Pass |
| Immutable images | All three runtime references use approved SHA-256 digests | Pass |
| Restricted API | Fixed namespace/template and least-privilege Role verified | Pass |
| Authentication | Missing bearer credential returned `401`; valid credential succeeded | Pass |
| TLS | Production certificate valid for the exact API hostname | Pass |
| HTTPS enforcement | HTTP returned permanent `308` redirect | Pass |
| GitOps | Both platform Applications `Synced` and `Healthy` | Pass |
| Manual control | No automated sync, pruning, or self-heal configured | Pass |
| Reconciliation | Deployment and Service became Ready | Pass |
| Status | `Available=True`, `readyReplicas=1`, internal endpoint reported | Pass |
| Drift correction | Manual replica drift restored to desired value | Pass |
| Ownership | Deletion removed the CR and owned child resources | Pass |
| Secret hygiene | Values absent from Git and documentation | Pass |
| Public exposure | Only approved 22, 80, and 443 ports open | Pass |
| Resource baseline | CPU and memory recorded before H8 | Pass with H8 headroom note |

## Known Operational Notes

- API authentication currently uses a single bearer token. Rotation is documented, but per-user identity and authorization are not implemented.
- Argo synchronization is intentionally manual. Reviewed revisions must be synchronized explicitly.
- The single-node K3s environment is not highly available. Node failure or maintenance affects the entire platform.
- The current 4 GB node has limited H8 headroom. The approximately 66% memory baseline requires conservative observability sizing and retention.
- A temporary SSH firewall `/32` source added after a network change must be removed when no longer needed.
- `portfolio-demo` intentionally remains active as the stable H7 demonstration workload.

## Reproduction and Operations

- Build sequence and architectural explanation: [`cloud-native-service-control-plane-build-guide.md`](cloud-native-service-control-plane-build-guide.md)
- Daily access, API lifecycle, credential rotation, and diagnosis: [`operator-guide.md`](operator-guide.md)
- Infrastructure decisions and phase model: [`infrastructure-context.md`](infrastructure-context.md)
- Argo CD sync and rollback: [`argocd-sync-rollback-runbook.md`](argocd-sync-rollback-runbook.md)

## Next Phase

H8 will deploy observability. It should begin with a fresh resource baseline and introduce the OpenTelemetry Collector, metrics, logs, traces, and dashboards incrementally with bounded storage and retention.

H7 is closed. H8 is not yet implemented.
