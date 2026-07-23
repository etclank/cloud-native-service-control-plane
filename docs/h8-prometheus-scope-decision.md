# H8.4 Final Prometheus Scope Decision

H8 uses exactly six Prometheus scrape jobs:

1. `prometheus-self`;
2. `kube-state-metrics`;
3. `platform-operator`;
4. `opentelemetry-collector`;
5. `control-plane-api`;
6. `managed-demo`.

This inventory is sufficient for the personal single-node K3s development and
portfolio environment. The six jobs cover Prometheus health, bounded
Kubernetes object state, the platform operator, the Collector, the
authenticated control-plane API, and the managed demo workload.

## Deliberate node-target deferral

`kubelet` and `cadvisor` are optional post-H8 enhancements. They are not H8
acceptance or completion requirements, and neither job is implemented.

This is a deliberate scope reduction, not proof that the earlier security
questions were resolved. The exact kubelet TCP 10250 serving-certificate,
Prometheus trust anchor and rotation contract remain unproven. kube-router's
enforcement of ordinary Pod-to-local-node traffic for exact
`142.132.178.45/32` TCP 10250 also remains unproven. The historical evidence
is retained in
[`h8-kubelet-cadvisor-certificate-networkpolicy-proof.md`](h8-kubelet-cadvisor-certificate-networkpolicy-proof.md).

Consequently H8 adds none of the following:

- a kubelet or cAdvisor scrape job;
- `nodes/metrics` or `nodes/proxy` permission;
- Prometheus Node discovery permission;
- TCP 10250 NetworkPolicy access;
- a broad node CIDR;
- `insecure_skip_verify`.

H8.4C-KR3 and H8.4C-KI are cancelled as H8 dependencies. Any later attempt to
add either node target requires a new optional enhancement review and must not
reuse this scope decision as security evidence.

## Completion boundary

The six-job H8.4B/C repository implementation is accepted. H8.4D restricted
GitOps deployment preparation is executable without additional node proof.
H8 remains incomplete until H8.4E live synchronization and validation and
H8.4F evidence/documentation closeout finish.
