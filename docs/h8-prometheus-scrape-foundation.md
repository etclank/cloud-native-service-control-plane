# H8.4C Prometheus Scrape Configuration Foundation

> Historical portfolio record: deployment observations apply to the revisions
> and dates recorded below, not current availability. Review environment-specific
> commands before use; see the [documentation index](README.md).

H8.4C replaces the inert H8.4B candidate scrape configuration with the six
targets accepted by the authoritative
[`h8-prometheus-target-matrix.json`](h8-prometheus-target-matrix.json). The
candidate remains disabled by default. This slice changes no Argo CD resource,
does not deploy Prometheus, and does not complete H8.4.

> Subsequent status: H8.4E/F completed on 2026-07-23. See
> [`h8-observability-closeout.md`](h8-observability-closeout.md) for the live
> result; the statements below preserve this repository slice's original
> boundary.

## Scrape inventory

The global scrape and evaluation intervals are 30 seconds. Every job uses a
30-second scrape interval, a 10-second timeout, and `/metrics`.

| Job | Discovery and endpoint | Scheme | Retained target labels |
| --- | --- | --- | --- |
| `prometheus-self` | static `127.0.0.1:9090` | HTTP | `job`, `instance` |
| `kube-state-metrics` | EndpointSlice in `observability`; exact `observability-kube-state-metrics` Service labels, `http` port, and ready endpoint; TCP 8080 | HTTP | `job`, `instance`, `namespace`, `service`, `pod` |
| `platform-operator` | EndpointSlice in `platform-system`; exact operator Service and controller Pod labels, `https` port, and ready endpoint; TCP 8443 | HTTPS | `job`, `instance`, `namespace`, `service`, `pod` |
| `opentelemetry-collector` | EndpointSlice in `observability`; exact Collector Service and Pod labels, `metrics` port, and ready endpoint; TCP 8888 | HTTP | `job`, `instance`, `namespace`, `service`, `pod` |
| `control-plane-api` | EndpointSlice in `platform-system`; exact API Service and Pod labels, `metrics` port, and ready endpoint; TCP 9090 | HTTP | `job`, `instance`, `namespace`, `service`, `pod` |
| `managed-demo` | EndpointSlice in `applications`; exact controller-owned `demo-http` Service and Pod label conjunction, `metrics` port, and ready endpoint; TCP 9090 | HTTP | `job`, `instance`, `namespace`, `service`, `pod` |

Each EndpointSlice job uses a fixed namespace list and default-drop relabeling.
Anchored keep rules admit only the reviewed Service identity, Service and Pod
labels, named endpoint port, and ready endpoint. Relabeling copies only the
stable namespace, Service, and Pod names. Annotation discovery, `labelmap`,
arbitrary Kubernetes labels, external targets, request or trace identifiers,
and workload-specific message values are not retained.

The operator job reads the standard projected ServiceAccount token from
`/var/run/secrets/kubernetes.io/serviceaccount/token`. It uses the existing
non-resource `/metrics` authorization. TLS verification remains disabled only
for this self-signed, internal controller-runtime endpoint; the token
authenticates the client but does not establish server identity. No token
literal or manually managed credential is committed.

## Endpoint and policy boundary

H8.4C enables the Collector's internal telemetry listener on its Pod IP at TCP
8888 and adds the named `metrics` port to the existing Collector ClusterIP
Service. It adds internal TCP 9090 metrics ports to the control-plane API
Service and operator-managed demo Services. Existing public API routing still
targets only Service port 80, and no Ingress, NodePort, LoadBalancer, or
Prometheus ingress is added.

The Prometheus discovery ClusterRole is reduced to the resources consumed by
the six jobs:

- core `pods` and `services`: `get`, `list`, `watch`;
- `discovery.k8s.io` `endpointslices`: `get`, `list`, `watch`;
- the existing operator metrics-reader binding for non-resource `/metrics`.

Namespace and Node discovery and `nodes/metrics` and `nodes/proxy` access are
removed. kube-state-metrics RBAC and the proven Kubernetes API egress rule are
unchanged.

The target-policy mapping is exact:

| Job | Prometheus egress | Target ingress |
| --- | --- | --- |
| `prometheus-self` | loopback; none | none |
| `kube-state-metrics` | exact same-namespace kube-state-metrics Pod selector, TCP 8080 | existing exact kube-state-metrics policy |
| `platform-operator` | exact `platform-system` namespace and operator Pod selector, TCP 8443 | exact Prometheus identity in `observability`, TCP 8443 |
| `opentelemetry-collector` | exact same-namespace Collector Pod selector, TCP 8888 | exact Prometheus Pod selector, TCP 8888 |
| `control-plane-api` | exact `platform-system` namespace and API Pod selector, TCP 9090 | exact Prometheus identity in `observability`, TCP 9090 |
| `managed-demo` | exact `applications` namespace and managed demo Pod selector conjunction, TCP 9090 | exact Prometheus identity in `observability`, TCP 9090 |

No rule admits TCP 10250, a broad CIDR, a port range, unrestricted namespace
traffic, or Internet egress.

## Deterministic rendering and ownership

The default observability render remains byte-for-byte unchanged at seven
objects with SHA-256
`cb441f08f11f45e914bad39271fa7198e85a21382a598ea37ad4d1e5eebf2900`.

The explicit H8.4C candidate renders 31 objects with SHA-256
`307f390cedb0f147fdf88c45ae83a184055c4d0d0269e3257591903ea486f0d5`:

- seven unchanged Collector-era objects;
- five upstream Prometheus objects;
- three upstream kube-state-metrics objects;
- sixteen repository-owned objects: two ClusterRoles, three
  ClusterRoleBindings, and eleven target/runtime NetworkPolicies.

The operator package renders 16 objects with SHA-256
`c1a10c07ec4e45e0b970d03812e1fbba30815c8e8395cd35015f0edc5204ef91`;
its previous 15-object render was
`8a45314c45987194240996fe51a6ea02d5a8c989bc58301e58f7f60ca9186c31`.
The API package renders 12 objects with SHA-256
`63ff0c3effbfc90f2754d3a7fe6b228a04f69f5d05eb43a60ecb03f26fb876f7`;
its previous 10-object render was
`e1bee691e8c248156d0a5a6fe4e61c94c39d1325833dded94c74df44ebd583cc`.
Those package deltas are the exact metrics ingress policies required for
cross-namespace Prometheus access.

H8.4C changes no Application or AppProject. H8.4D subsequently derives the
exact observability AppProject and inert Application delta required before the
package can be synchronized.

## Validation and deferrals

Local validation covers the committed target matrix, exact job allowlist,
EndpointSlice selectors and relabeling, durations, operator authentication and
TLS, retained labels, RBAC, target policies, endpoint exposure, immutable
images, H8.4B storage/resources/security, deterministic Helm and Kustomize
renders, dependency archives, Go tests, lint, and builds. Pinned Prometheus
3.13.1 `promtool check config --syntax-only` accepts the exact rendered
configuration. Full local `promtool check config` cannot stat the standard
in-cluster projected token path, which is intentionally absent outside a Pod;
no token file was created or accessed.

H8.4C-K subsequently proved the exact `nodes/metrics get` authorization
contract and recorded bounded kubelet and cAdvisor family candidates, but
could not close the exact serving-certificate or Pod-to-node NetworkPolicy
proof. H8.4C-KR found that the configured SSH key was not loaded and correctly
made no retry. H8.4C-KR2 later authenticated with that exact key, then stopped
before host inspection because `sudo -n` required a password. The
[final H8.4 scope decision](h8-prometheus-scope-decision.md) deliberately
removes both node jobs from H8 acceptance for this personal single-node
environment. Their certificate and policy questions remain unresolved, and
both are optional post-H8 enhancements rather than implemented targets.
TCP 10250 remains absent, as recorded in
[`h8-kubelet-cadvisor-certificate-networkpolicy-proof.md`](h8-kubelet-cadvisor-certificate-networkpolicy-proof.md).
H8.4D is therefore executable with the accepted six jobs and is completed
repository-side by the restricted manual-sync preparation in
[`h8-prometheus-gitops-deployment-preparation.md`](h8-prometheus-gitops-deployment-preparation.md).
No live deployment or scrape occurred in H8.4C/D. Prometheus remains disabled
by default, and H8 remains incomplete pending live validation and closeout.
