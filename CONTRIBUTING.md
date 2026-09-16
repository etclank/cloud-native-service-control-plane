# Contributing

Keep changes scoped to the existing platform. Explain behavior, tests, deployment
impact, and limitations in each change.

## Development checks

Run `make test` and `make lint-config lint`. Run `go test -race ./...` for Go
changes. For chart changes, run `make observability-validate` and also lint/render
with `-f deploy/observability/values-h8.4b-candidate.yaml` to exercise the enabled
Prometheus configuration. Do not update locked dependencies incidentally.

Use `make test-e2e` only with a dedicated local Kind cluster. Deployment commands
use Kubernetes credentials and are separate from repository validation.

## Generated files and scaffolding

Use Kubebuilder to scaffold APIs and webhooks. Preserve project structure and
`+kubebuilder:scaffold` markers. Do not hand-edit `PROJECT`, generated DeepCopy
files, CRD bases, generated RBAC, or webhook manifests.

After changing API types or markers, run `make manifests generate` and review
the generated diff. After Go changes, run `make lint-fix` and `make test`.

## Implementation conventions

Reconciliation must be idempotent, watch owned resources, and use controller
references. Handle conflicts through fresh reads and reconciliation retries.
Only write status when it changes; use Kubernetes conditions and observed
generation. Add finalizers only when external cleanup requires them.

Pass request contexts through API and Kubernetes operations. Keep timeouts and
shutdown bounded. Never log authorization headers, token values, request bodies,
or unbounded user-controlled metric labels. Use structured logging with balanced
key/value pairs and capitalized messages without trailing periods.

## Credentials and delivery

Keep credentials, kubeconfigs, private keys, and local environment files outside
Git. `.gitignore` is an additional safeguard, not a credential boundary.
Use digest-qualified runtime images and review rendered RBAC, policies, and
public ingress before deployment. Repository review does not require modifying
live Kubernetes or Argo CD state.
