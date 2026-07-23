# H8.4C-K Kubelet and cAdvisor Proof Follow-up

H8.4C-K evaluated whether the deferred `kubelet` and `cadvisor` jobs could be
added safely to the disabled Prometheus candidate. Both remain deferred. No
scrape job, RBAC permission, TCP 10250 policy, dependency, rendered object, or
live resource changed.

The repository baseline was
`164cd70d395e8891bf869137773d229b6669db0f`, and production remained at
Collector Commit A
`01c2d27b2bc671ce76686ce8d60c6a0c5b70b89d`.

## Read-only baseline

The explicit `portfolio-k3s` context reported:

- Kubernetes `/readyz`: healthy;
- Node: `portfolio-k3s-01`, UID
  `f68ef7c8-16ba-493e-9ce0-88586f2a18d4`;
- K3s: `v1.36.2+k3s1`;
- reviewed IPv4 InternalIP: `142.132.178.45`;
- advertised kubelet endpoint: TCP 10250;
- Prometheus ServiceAccount identity: currently denied `nodes`,
  `nodes/metrics`, `nodes/proxy`, `nodes/stats`, and `nodes/log`;
- observability Application: Synced/Healthy at exact Collector Commit A, with
  no active operation or condition;
- no Prometheus Deployment, kube-state-metrics Deployment, or Prometheus PVC.

The Collector retained its seven reviewed resource identities. Its DaemonSet
was 1/1 Ready, and its Pod retained UID
`f7263e53-8ddc-408e-8516-0ee9532a6c08` with zero restarts. The live Collector
Service still exposed only 4317/4318; the API and managed demo Services still
exposed only port 80.

## Authentication and authorization

The [Kubernetes v1.36 kubelet authentication and authorization
contract](https://kubernetes.io/docs/reference/access-authn-authz/kubelet-authn-authz/)
defines TokenReview-based bearer authentication when kubelet token webhook
authentication is enabled and SubjectAccessReview authorization when webhook
authorization is enabled. It maps GET requests below `/metrics/*` to:

```text
apiGroup: ""
resource: nodes
subresource: metrics
verb: get
resourceName: portfolio-k3s-01
```

This applies to both `/metrics` and `/metrics/cadvisor`. `nodes/proxy` is not
required and is explicitly rejected because Kubernetes documents that it can
authorize command execution and is not a read-only permission.

The live `metrics-server` provided compatibility evidence:

- it was Ready with zero restarts and published fresh node CPU/memory through
  `metrics.k8s.io`;
- it selected `InternalIP,ExternalIP,Hostname` and the Node-advertised kubelet
  port;
- it did not use a kubelet insecure-TLS flag;
- its ServiceAccount was authorized for `nodes/metrics`.

This establishes that the current K3s kubelet accepts Kubernetes
ServiceAccount bearer authentication and webhook authorization. The future
minimum endpoint permission is `core/nodes/metrics get`. `core/nodes
get,list,watch` would additionally be required only if Kubernetes Node
discovery remains the selected discovery model. No permission was added
because neither job is accepted.

## TLS proof result

Strict TLS is preferred, and the healthy metrics-server gives strong
compatibility evidence that the reviewed InternalIP works with strict
verification. The exact serving certificate could not be inspected:

- direct TCP 10250 from the workstation was blocked by the existing public
  firewall boundary;
- there was no kubelet serving CertificateSigningRequest object;
- the configured read-only SSH attempt failed public-key authentication;
- Pod execution, port-forwarding, private-key access, and creating a diagnostic
  workload were prohibited.

Consequently, the certificate subject, issuer, validity, SANs, trust anchor,
and any required Prometheus `server_name` remain unproven. Neither strict TLS
configuration nor `insecure_skip_verify` is approved for the candidate.

## NetworkPolicy proof result

H8.4B0 proved that kube-router evaluates Kubernetes API egress against the
post-DNAT node address `142.132.178.45/32` on TCP 6443. It did not test direct
Pod-to-host-process traffic on TCP 10250. Reusing the 6443 result for a
different port and host process would be an inference.

Because SSH inspection and a controlled Pod connectivity case were
unavailable, H8.4C-K did not prove:

- that kube-router evaluates direct kubelet traffic against
  `142.132.178.45`;
- that the exact `/32` TCP 10250 rule admits the Prometheus Pod;
- that removal of that rule denies the same path;
- whether host-process handling changes the enforcement boundary.

No TCP 10250 policy was added.

## Bounded schema evidence

Exactly one read-only schema request was made to each endpoint through the
already authorized Kubernetes API proxy. Streaming processing retained only
selected metric-family names and label-key names. No metric value, timestamp,
raw payload, container ID, image ID, Pod UID, or credential was stored.

### Kubelet candidate families

| Family | Operational use | Retained labels | Removed labels and cardinality |
| --- | --- | --- | --- |
| `kubelet_running_pods` | running Pod count and kubelet stability | `node` | no endpoint labels; one series |
| `kubelet_running_containers` | container count by lifecycle state | `node`, `container_state` | state is kubelet-defined and bounded |
| `kubelet_runtime_operations_total` | runtime operation rate | `node`, `operation_type` | operation type is kubelet-defined and bounded |
| `kubelet_runtime_operations_errors_total` | runtime error rate | `node`, `operation_type` | shares the bounded operation dimension |
| `kubelet_pleg_relist_duration_seconds` | PLEG relist latency degradation | `node`, `le` for buckets; sum/count have only `node` | fixed histogram buckets on one node |

The observed histogram series were the `_bucket`, `_sum`, and `_count`
variants. `kubelet_node_name` and PLEG relist interval were observed but not
retained because they add no required initial use case.

### cAdvisor candidate families

| Family | Operational use | Retained labels | Removed labels and cardinality |
| --- | --- | --- | --- |
| `container_cpu_usage_seconds_total` | Pod/container CPU consumption | `node`, `namespace`, `pod`, `container`, `cpu` | remove `id`, `image`, `name`; `cpu` is bounded by the reviewed two-core node |
| `container_memory_working_set_bytes` | Pod/container working-set memory | `node`, `namespace`, `pod`, `container` | remove `id`, `image`, `name` |

Receive and transmit network families were observed with an `interface` label.
They remain outside the candidate allowlist because dropping that label can
collapse distinct series, while retaining arbitrary interface values has not
been proven safe. No filesystem family was selected.

These are candidate allowlists, not active Prometheus configuration. They are
recorded structurally in
[`h8-prometheus-target-matrix.json`](h8-prometheus-target-matrix.json).

## Decision and next gate

Both jobs remain `deferred`. Their exact endpoint authorization and bounded
CPU/memory family candidates are now known, but the following mandatory proof
is unresolved:

1. exact kubelet serving-certificate subject, issuer, validity, SAN, and trust
   behavior;
2. strict Prometheus TLS configuration, including whether `server_name` is
   needed;
3. controlled kube-router allow/deny evidence for exact
   `142.132.178.45/32` TCP 10250;
4. a safe network-interface value boundary before cAdvisor network families
   can be accepted.

H8.4C-KR subsequently confirmed that the intended alias and configured key
were unambiguous, but the SSH agent held only a different public-key
fingerprint. H8.4C-KR2 then verified the exact configured fingerprint and
authenticated successfully to `portfolio-k3s-01` as `eoghan`. Its single
session stopped before host inspection because `sudo -n` required a password.
The exact result and remaining boundary are recorded in
[`h8-kubelet-cadvisor-certificate-networkpolicy-proof.md`](h8-kubelet-cadvisor-certificate-networkpolicy-proof.md).
The explicit future owner is **H8.4C-KR3 non-interactive read-only host
evidence follow-up**, after the user establishes narrowly scoped passwordless
sudo for the reviewed public-metadata commands or supplies equivalent directly
observed evidence through a separately reviewed method. H8.4D remains blocked
until that follow-up either accepts both jobs or the authoritative roadmap
explicitly changes the mandatory H8.4 inventory.

The observability default render remains seven objects with SHA-256
`cb441f08f11f45e914bad39271fa7198e85a21382a598ea37ad4d1e5eebf2900`.
The disabled candidate remains 31 objects with SHA-256
`307f390cedb0f147fdf88c45ae83a184055c4d0d0269e3257591903ea486f0d5`
and exactly the six H8.4C jobs. Prometheus and kube-state-metrics remain
disabled by default. No live scrape was claimed, and H8.4 remains incomplete.
