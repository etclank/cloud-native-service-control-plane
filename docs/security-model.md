# API and Security Model

## API contract

| Method | Path | Authentication | Result |
| --- | --- | --- | --- |
| GET | `/healthz` | None | Process health |
| GET | `/readyz` | None | Kubernetes list access in `applications`; 503 when unavailable |
| GET | `/api/v1` | Bearer | Service identity |
| POST | `/api/v1/managed-services` | Bearer | Create an approved demo workload; 201 |
| GET | `/api/v1/managed-services` | Bearer | List all ManagedServices in the fixed namespace |
| GET | `/api/v1/managed-services/{name}` | Bearer | Resource and reconciliation status |
| DELETE | `/api/v1/managed-services/{name}` | Bearer | Request deletion; 204, with asynchronous child cleanup |

Create requests accept `name`, optional `replicas`, and optional `message`.
Names must be DNS labels, replicas are 1–3, and messages are at most 120 Unicode
characters. Unknown JSON fields, query parameters on lifecycle routes, trailing
JSON, and oversized bodies are rejected. Duplicate creation returns 409;
missing resources return 404. There is no update route, idempotency-key support,
or pagination. A successful create means the CR exists, not that Pods are ready.

The process loads `API_TOKEN_FILE` once at startup. Provision a random token
outside Git and restart the API after rotating the mounted Secret. Authentication
compares SHA-256 hashes in constant time. Every token holder has the same
namespace-wide lifecycle access; there is no per-user or tenant authorization.

## Trust boundaries

- Public API traffic terminates TLS at Traefik. Health/readiness routes are public;
  lifecycle routes require authentication. Redirect and rate-limit Middleware
  resources depend on Traefik CRDs and configuration.
- API RBAC is limited to ManagedService create/get/list/delete in `applications`.
  The operator has cluster-scoped CR/child-resource permissions; it does not
  accept arbitrary images from API clients. No custom finalizer is implemented.
  Its finalizer update permission supports owner-reference admission checks.
- API and operator ServiceAccounts need Kubernetes tokens. Demo and Collector
  Pods disable automatic token mounting. Runtime tokens and registry credentials
  are not included in manifests.
- Operator/API/demo containers use non-root identities, seccomp, dropped
  capabilities, read-only root filesystems, and resource requests/limits.
- Argo CD projects restrict sources, destinations, and resource kinds. The
  vendored Argo CD installation itself has broad cluster permissions: project
  allowlists do not turn an Argo CD administrator into an unprivileged user.
- Kubernetes and Argo CD administration are described through private tunnels
  and loopback port forwarding. Host firewall and SSH configuration are
  documented historical controls, not enforced by this repository's manifests.

## Networking and observability limits

Collector and Prometheus have no public Ingress, NodePort, or host networking.
Collector namespace policy denies traffic except explicitly allowed identities
and ports. Prometheus discovery requires an exact Kubernetes API backend /32
and port. The checked-in value is environment-specific and intentionally retained
with its regression fixtures; rediscover and override it for another cluster.
It is a routing destination, not authorization or permission to expose the API.

The API has a dedicated ingress policy that allows only Traefik Pods with the
expected identity in `kube-system` to reach its HTTP listener on TCP 8080. A
separate policy allows only the Prometheus identity in `observability` to reach
the API metrics listener on TCP 9090. These additive rules do not allow Traefik
to scrape metrics or Prometheus to use the API HTTP listener.

The managed-demo metrics policy selects the whole workload Pod and grants only
Prometheus ingress on TCP 9090. Managed-demo TCP 8080 currently has no permitted
Pod caller; adding one requires a separate decision about the intended client
identity. Historical K3s ingress checks are not proof of access on a different
CNI or ingress topology. Verify allowed and denied traffic on the target
dataplane. NetworkPolicy does not provide a universal host/node traffic boundary.

Operator metrics require Kubernetes bearer authentication, but the Prometheus
job uses `insecure_skip_verify: true` for its self-signed serving certificate.
This lacks server identity verification and needs trusted serving certificates
before a stronger production security claim. Other internal scrape jobs use
HTTP inside the cluster.

HTTP metrics normalize routes and methods, and do not label by request ID,
message, or arbitrary URL. Logs omit authorization headers and bodies. Workload
OTLP trace export is disabled in deployment configuration. The Collector's
`nop` exporter retains no logs, traces, or OTLP metrics. Prometheus stores its
separate scrape data locally; retention is bounded, but it is not a backup.

## Intentional limits

This is a single-administrator portfolio platform. Token compromise permits
creating many bounded workloads because there are no namespace quotas or
per-user limits. Replica limits are per resource, not total capacity controls.
There is no HA, replicated storage, formal security certification, or production
SLO. `Available` currently tests ready-replica count; it is not a rollout
completion signal. Review these boundaries before using the system beyond its
portfolio scope.
