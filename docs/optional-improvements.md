# Optional Platform Improvements

> Historical portfolio record: deployment observations apply to the revisions
> and dates recorded below, not current availability. Review environment-specific
> commands before use; see the [documentation index](README.md).

**Baseline closed:** 24 July 2026

The implemented platform baseline is complete for the current portfolio scope.
The ideas below are not a mandatory H9 or later delivery plan. Each would need
its own capacity, security, persistence, exposure, and rollback review before
implementation.

Status terms are intentionally bounded:

- **Designed:** useful architecture or constraints exist, but the capability
  is not implemented.
- **Partially prepared:** a prerequisite or related control exists, but the
  capability is not enabled end to end.
- **Entirely unimplemented:** no repository or live implementation exists.

## Observability

### Grafana dashboards

- **Adds:** private dashboards for the six accepted Prometheus jobs and future
  application signals.
- **Why optional:** Prometheus collection, querying, persistence, and target
  health are already validated without a dashboard.
- **Prerequisites and risks:** authentication and access design, bounded
  resources, data-source provisioning, and no public administrative exposure.
- **Status:** designed.

### Loki log aggregation

- **Adds:** centralized, searchable application and platform logs.
- **Why optional:** current operations can use bounded host journals and
  Kubernetes container logs.
- **Prerequisites and risks:** storage and retention budgets, low-cardinality
  labels, sensitive-data review, and recovery behavior for local-path data.
- **Status:** designed.

### Tempo trace storage

- **Adds:** retained distributed traces and cross-service request analysis.
- **Why optional:** the current workloads do not export production OTLP, and
  the baseline does not depend on trace storage.
- **Prerequisites and risks:** workload export, sampling, storage retention,
  trace-data privacy, and measured node capacity.
- **Status:** designed.

### Workload OTLP export

- **Adds:** production metric, log, or trace delivery from the API and managed
  demo to the Collector.
- **Why optional:** shared instrumentation and selector-safe Collector ingress
  are prepared, while the accepted baseline deliberately keeps the Collector
  on its `nop` exporter.
- **Prerequisites and risks:** signal-specific backends, retry and queue
  bounds, endpoint configuration, cardinality review, and failure testing.
- **Status:** partially prepared; export is disabled.

### Kubelet and cAdvisor metrics

- **Adds:** node and container runtime metrics.
- **Why optional:** the six accepted jobs cover the current platform scope.
- **Prerequisites and risks:** the live kubelet certificate lacks the required
  IP subject alternative name; TCP 10250, node RBAC, certificate handling, and
  NetworkPolicy would expand the security boundary.
- **Status:** designed and deliberately unimplemented.

### Alerting, SLOs, and recording rules

- **Adds:** actionable notification, service-level indicators, and efficient
  precomputed queries.
- **Why optional:** current health is inspected directly and no paging
  commitment is part of the portfolio baseline.
- **Prerequisites and risks:** ownership, notification credentials, tested
  thresholds, noise control, and resource/cardinality budgets.
- **Status:** designed.

### Synthetic HTTP, TCP, and DNS probes

- **Adds:** bounded external and internal reachability measurements.
- **Why optional:** Kubernetes health checks and direct operational validation
  cover the current endpoints.
- **Prerequisites and risks:** a strict target allowlist, bounded concurrency
  and timeouts, abuse prevention, telemetry storage, and alert ownership.
- **Status:** designed.

## Applications and Data

### SmartEnergy stack

- **Adds:** the existing FastAPI and dashboard application with PostgreSQL and
  Redis, demonstrating a larger Python and data-service workload.
- **Why optional:** the operator, API, managed demo, GitOps, TLS, and
  observability already demonstrate the accepted platform lifecycle.
- **Prerequisites and risks:** new immutable images and packages, database
  lifecycle ownership, migrations, backups, Secret delivery, capacity review,
  and separate NetworkPolicies.
- **Status:** designed; entirely unimplemented on this platform.

### Additional stateful workloads

- **Adds:** broader storage and application patterns.
- **Why optional:** only Prometheus currently requires persistent storage; no
  application database is part of the baseline.
- **Prerequisites and risks:** local-path is single-node and uses a local
  failure domain. Each workload needs backup, restore, upgrade, retention, and
  data-loss decisions.
- **Status:** entirely unimplemented.

### Application onboarding automation

- **Adds:** scaffolding or validation that generates the repeated Kustomize,
  policy, and Argo CD registration structure.
- **Why optional:** the documented GitOps route is reproducible at the current
  application count.
- **Prerequisites and risks:** templates must preserve restricted AppProject
  permissions, immutable images, Secret boundaries, and application-specific
  policy decisions.
- **Status:** entirely unimplemented.

## Resilience and Scale

### Multi-node or highly available Kubernetes

- **Adds:** node redundancy, safer maintenance, and a foundation for highly
  available control-plane and workload placement.
- **Why optional:** the current low-traffic portfolio scope accepts one node.
- **Prerequisites and risks:** additional cost, datastore topology, storage,
  load balancing, failure-domain design, and migration testing.
- **Status:** designed.

### Multi-cluster management

- **Adds:** workload isolation, failure-domain separation, and per-environment
  GitOps delivery.
- **Why optional:** one small cluster satisfies the baseline.
- **Prerequisites and risks:** cluster identity, credential boundaries,
  repository/environment promotion, centralized observability, and cost.
- **Status:** designed; entirely unimplemented.

### Air-gapped deployment

- **Adds:** operation without public package or image access.
- **Why optional:** the current environment deliberately uses private GitHub
  and GHCR access.
- **Prerequisites and risks:** mirrored registries and packages, offline
  update media, internal trust, SBOM/signature verification, and operational
  complexity.
- **Status:** designed; entirely unimplemented.

### Automated backup and disaster-recovery exercises

- **Adds:** repeatable recovery of cluster state, credentials, configuration,
  and application data with measured recovery objectives.
- **Why optional:** Git reconstructs non-secret configuration and the current
  baseline does not claim automatic disaster recovery.
- **Prerequisites and risks:** encrypted off-node storage, Secret custody,
  K3s and PVC restore procedures, destructive-test isolation, and retention.
- **Status:** partially prepared; recovery boundaries are documented, while
  automation and full restore exercises are unimplemented.

### Deeper capacity, load, and resilience testing

- **Adds:** measured concurrency limits, saturation behavior, and failure
  recovery evidence.
- **Why optional:** current live evidence is appropriate to a low-traffic
  demonstration, not an unbounded workload claim.
- **Prerequisites and risks:** representative workloads, explicit stop
  thresholds, observability, and protection of the single live node.
- **Status:** partially prepared; limited component and recovery checks exist,
  while broad testing is unimplemented.

### Public demonstration hardening

- **Adds:** a repeatable presentation workflow, final public-surface review,
  abuse controls, and demonstration-specific reliability checks.
- **Why optional:** the authenticated API already has production TLS,
  redirection, and rate limiting; public portfolio operation does not require
  a formal service-level commitment.
- **Prerequisites and risks:** threat review, load limits, certificate and DNS
  checks, incident ownership, cost monitoring, and data sanitization.
- **Status:** partially prepared; core public controls are implemented, while
  final demonstration exercises remain optional.

These items preserve the useful intent of the former H9–H12 roadmap without
making them completion requirements. Adoption should be driven by a concrete
need rather than a phase number.
