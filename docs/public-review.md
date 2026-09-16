# Public Repository Review

Review date: 2026-09-16. Starting revision:
`72a2cb4272c7a2a76539e2bc14be2c9bf8ba752c` (`Fix CRI container identity proof`).
The worktree was clean on `main`, tracking `origin/main`, with zero commits ahead
or behind after fetching. Scope: repository presentation, configuration review,
and local validation; no portfolio-cluster changes or image publication.

## Verified implementation

- The Go API creates/lists/gets/deletes approved ManagedServices in `applications`.
  Authentication, request limits, error mapping, HTTP lifecycle, and telemetry
  have local tests.
- The operator reconciles owned Deployments and ClusterIP Services through
  `CreateOrUpdate`, watches secondary resources, avoids unchanged status writes,
  and reports Kubernetes conditions. Envtest covers schema and reconciliation;
  Kind E2E verifies runtime image identity, metrics access, and managed workload
  creation. Availability reflects ready replicas rather than rollout completion.
- GitOps projects use explicit repository, destination, and resource-kind
  allowlists. Platform Applications pin commits and require manual sync;
  registry validation follows `main`. Upstream Argo CD administration remains
  privileged.
- The enabled Helm overlay includes Collector, persistent Prometheus, reduced
  kube-state-metrics, and six scrape jobs. OTLP workload export is disabled, and
  Collector pipelines use `nop`. Prometheus scrape storage is separate.
- Deployment configuration includes TLS, middleware, hardened containers, scoped
  API RBAC, and private telemetry. Host/firewall controls remain historical
  evidence. See [security boundaries](security-model.md).

## Validation results

Commands ran from the repository root unless otherwise stated. Installed Go was
`go1.26.5 linux/amd64`; envtest used Kubernetes `1.36.2`.

| Check | Command / scope | Result |
| --- | --- | --- |
| Standard tests | `make test` | Passed on retry, including generation, vet, dependency checksums, controller/envtest and API tests; first attempt hit chart-repository DNS timeout |
| All Go packages | `KUBEBUILDER_ASSETS="$PWD/bin/k8s/1.36.2-linux-amd64" go test ./...` | Passed |
| Race detector | Same assets setting with `go test -race ./...` | Passed |
| Vet/build | `go vet ./...`; `go build ./...` | Passed |
| Formatting | `test -z "$(gofmt -l $(git ls-files '*.go'))"` | Passed |
| Module integrity | `go mod verify` | Passed |
| Generated drift | `git diff --exit-code -- go.mod go.sum api config/crd/bases config/rbac/role.yaml` | Passed |
| Lint | `make -o golangci-lint lint-config lint` | Passed, zero issues; skipped only installation after rebuilding the configured custom linter with logcheck |
| Helm base package | `make -o observability-dependencies observability-validate` | Passed with already-downloaded checksum-verified archives; includes deterministic render and Argo Helm comparison |
| Helm enabled overlay | `bin/helm lint deploy/observability -f deploy/observability/values-h8.4b-candidate.yaml` | Passed |
| Enabled render | `bin/helm template observability deploy/observability --namespace observability -f deploy/observability/values-h8.4b-candidate.yaml` and same command with `bin/helm-argocd`, then `cmp` | Identical output; upstream null/table coalescing warnings with older Helm, no render/test failure |
| Kustomize | `bin/kustomize build` for `config/default`, `config/e2e`, `config/samples`, `kubernetes/platform/control-plane-api`, `kubernetes/validation/registry`, `kubernetes/validation/tls` | All rendered; YAML identity and absence of populated Secrets checked |
| Manifest contracts | `go test ./internal/observability`, deployment/GitOps tests included in full suite | Passed; validates typed resources, security fields, target inventory, and Prometheus configuration |
| Prometheus syntax | `bin/promtool-v3.13.1 check config --syntax-only /tmp/readiness-prometheus.yml` against extracted rendered ConfigMap | Passed; referenced cluster token files deliberately not read |
| Isolated E2E | `KUBECONFIG=/tmp/readiness-kind-kubeconfig make test-e2e KIND_CLUSTER=readiness-review-20260916` | Passed; dedicated Kind cluster deleted by cleanup |
| Operator image | `docker build -t readiness/operator:local .` | Passed, local only |
| API image | `docker build -f images/control-plane-api/Dockerfile -t readiness/control-plane-api:local .` | Passed, local only |
| Demo image | `docker build -f images/demo-http/Dockerfile -t readiness/demo-http:local .` | Passed, local only |
| Registry smoke image | `docker build -t readiness/registry-smoke:local images/registry-smoke` | Passed, local only |
| Independent source build | `go build ./...` from a separate source-only copy in `/tmp/readiness-source` | Passed with existing Go dependency cache, without ignored repository binaries/artifacts |
| Vendored manifest | `sha256sum --check SHA256SUMS` in `kubernetes/bootstrap/argocd/upstream` | Passed |
| Actions pins | `git ls-remote` against each configured action's documented upstream release tag | All seven unique repository/version pins matched |
| Documentation | Local Markdown file-link check, manual command/claim review | Passed |
| Diff | `git diff --check` | Passed |

The test workflow now also checks generated/module drift, builds all packages,
runs the race detector, and validates Helm compatibility. This review verified
those commands locally; it does not claim a completed post-push GitHub run.
No application Go code, dependency versions, image pins, or rendered deployment
values changed. Chart edits only correct comments.

## Public safety

Reviewed tracked filenames, working-tree sensitive-file patterns, broad
credential keyword matches, all-ref commit statistics, sensitive-path history,
and 367 historical file blobs with targeted private-key/token patterns.
Gitleaks v8.24.3 was installed outside the repository and run with redacted output:

```bash
gitleaks git . --log-opts=--all --redact --report-format json --report-path /tmp/readiness-history-secrets.json
gitleaks dir /tmp/readiness-source --redact --report-format json --report-path /tmp/readiness-worktree-secrets.json
```

The all-ref scan reported three matches: a synthetic unit-test token, a Dockerfile
assertion, and a public SSH fingerprint in an older design record. Source review
confirmed these were not deployed credentials. The current source scan reported
the first two fixture/assertion matches only. No real credential, populated
Secret, tracked kubeconfig, private key, or `.env` was found. Scanning is evidence
of review, not a mathematical guarantee of absence.

Repeated node addresses and personal login details in Markdown were sanitized;
one unnecessary public-key fingerprint was omitted. Exact routing values in the
observability configuration and regression fixtures remain intentionally tied
to the documented portfolio topology. API group/domain identities, image
references, and ACME configuration are environment-specific and must be reviewed
before reuse. Historical commits were not rewritten.

Contributor documentation now records development and generated-file rules. No
proprietary application source was identified. Licensing scope remains as
described in [third-party provenance](third-party.md).

## Public endpoint checks and limits

Unauthenticated HTTPS checks initially timed out resolving DNS. On retry,
`curl --max-time 20` returned HTTP **502** for the documented API `/healthz` and
HTTP **200** for the TLS validation hostname. The API's current health is therefore
not established. No authenticated request or portfolio Kubernetes access was
made, and the external failure was not diagnosed through live administration.

The README no longer advertises an available live demo. Existing closeouts are
historical records. A source-only build was verified, but anonymous cloning and
private GHCR access after a future visibility change cannot be established by
this local review. Publication readiness does not imply production readiness or
current deployment health. Remaining engineering limits include single-node
storage, shared-token authorization, no quotas, cluster-dependent networking,
operator scrape certificate verification bypass, and no persistent OTLP backend.

## Reviewed file inventory

### Created

- `CONTRIBUTING.md`
- `docs/README.md`
- `docs/public-review.md`
- `docs/security-model.md`
- `docs/third-party.md`

### Modified

- `.github/workflows/test.yml`
- `.gitignore`
- `README.md`
- `deploy/observability/values-h8.4b-candidate.yaml`
- `deploy/observability/values.yaml`
- `docs/application-deployment-guide.md`
- `docs/argocd-sync-rollback-runbook.md`
- `docs/cloud-native-service-control-plane-build-guide.md`
- `docs/h7-platform-deployment-closeout.md`
- `docs/h8-collector-deployment-closeout.md`
- `docs/h8-kubelet-cadvisor-certificate-networkpolicy-proof.md`
- `docs/h8-kubelet-cadvisor-proof-follow-up.md`
- `docs/h8-observability-closeout.md`
- `docs/h8-observability-design.md`
- `docs/h8-prometheus-api-egress-proof.md`
- `docs/h8-prometheus-gitops-deployment-preparation.md`
- `docs/h8-prometheus-runtime-foundation.md`
- `docs/h8-prometheus-scope-decision.md`
- `docs/h8-prometheus-scrape-foundation.md`
- `docs/infrastructure-context.md`
- `docs/operator-guide.md`
- `docs/optional-improvements.md`
- `docs/project-closeout.md`

### Deleted

- `AGENTS.md`
