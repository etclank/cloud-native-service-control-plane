# Application Deployment Guide

This guide describes the application routes the repository actually supports.
Git is the source of truth; long-lived imperative `kubectl` changes are not
the normal deployment workflow.

## Supported Routes

### 1. GitOps Kubernetes package — recommended for a new application

Create an application-specific Kustomize package under `kubernetes/`, add the
smallest required permissions to a restricted AppProject, and register a
manual-sync Argo CD Application under `deploy/gitops/bootstrap/`. This is the
only current route for an arbitrary image or application such as FastAPI.

Use
[`kubernetes/platform/control-plane-api`](../kubernetes/platform/control-plane-api)
as the stateless public-service reference and
[`deploy/gitops/bootstrap/control-plane-api-application.yaml`](../deploy/gitops/bootstrap/control-plane-api-application.yaml)
as the GitOps registration reference.

### 2. `ManagedService` — supported only for `demo-http`

The `platform.eoghanclancy.eu/v1alpha1` CRD accepts:

```yaml
apiVersion: platform.eoghanclancy.eu/v1alpha1
kind: ManagedService
metadata:
  name: portfolio-demo
  namespace: applications
spec:
  template: demo-http
  replicas: 1
  message: Hello from a ManagedService
```

`template` must be `demo-http`; `replicas` defaults to `1` and is limited to
`1–3`; `message` is limited to 120 characters. The operator selects the
approved digest-pinned image and creates the Deployment and ClusterIP Service.
The CRD does not accept an arbitrary image, port, probe, environment map,
Ingress, or Secret.

The authenticated control-plane API exposes the same bounded lifecycle in the
fixed `applications` namespace:

- `POST` and `GET /api/v1/managed-services`
- `GET` and `DELETE /api/v1/managed-services/{name}`

It always creates the `demo-http` template. Authentication and safe token use
are documented in the [operator guide](operator-guide.md).

## Application Readiness Contract

Before repository onboarding, an application should:

- produce an OCI-compatible image with an immutable SHA-256 runtime reference;
- listen on `0.0.0.0` at one declared application port;
- provide lightweight liveness and readiness endpoints;
- handle `SIGTERM` and complete bounded graceful shutdown;
- accept non-secret configuration through environment variables or mounted
  files;
- read Secrets from separately provisioned Kubernetes Secret references;
- run as non-root with dropped capabilities and a read-only root filesystem
  where feasible;
- declare realistic CPU/memory requests and limits;
- avoid writing durable data to the container filesystem;
- optionally expose private application metrics on a separate named port.

The current Go services use `/healthz`, `/readyz`, port 8080 for HTTP, and port
9090 for private metrics. A new application may use different values if its
manifests, probes, Service, policies, and discovery configuration agree.

## Repository Onboarding

1. Choose a DNS-1123 application name and an existing or reviewed namespace.
   Platform components use `platform-system`; operator-managed demos use
   `applications`. A new application namespace needs explicit AppProject and
   policy review.
2. Add a self-contained Kustomize package, normally
   `kubernetes/<area>/<application>/`, with:
   - Namespace only when repository ownership includes it;
   - ServiceAccount and least-privilege RBAC if Kubernetes API access is
     required;
   - digest-pinned Deployment;
   - ClusterIP Service;
   - default-deny and narrowly scoped NetworkPolicies;
   - optional Certificate, Traefik Middleware, and Ingress.
3. Use standard labels consistently:
   `app.kubernetes.io/name`, `app.kubernetes.io/component`, and
   `app.kubernetes.io/part-of`.
4. Keep Deployment selectors immutable and make Service selectors match the
   Pod template.
5. Reference Secret names and keys only. Create Secret values through the
   approved out-of-repository operational process.
6. Add only the required repository, destination, resource groups, and kinds
   to a restricted AppProject. Never use wildcard permissions for
   convenience.
7. Add a manual-sync Argo CD Application with an immutable reviewed revision
   when preparing a controlled rollout. Preserve the repository convention:
   no automated sync, prune, or self-heal.
8. Add structured regression tests that render the package and verify images,
   hardening, exposure, policy, and GitOps restrictions.

Public access is optional. When required, use `ingressClassName: traefik`, an
exact hostname, a cert-manager `Certificate` using
`letsencrypt-production`, a shared TLS Secret reference, and permanent HTTPS
redirection. Do not expose administration, metrics, databases, or telemetry
receivers.

## Build and Immutable Publication

Follow the existing image workflows:

1. Build and test locally without embedding credentials or configuration.
2. Use a digest-pinned builder and minimal non-root runtime image.
3. Build from a reviewed Dockerfile and expected context.
4. Publish only a full-source-SHA tag from GitHub Actions; never publish or
   deploy `latest`.
5. Record the workflow source commit and resulting registry digest.
6. Set the Kubernetes runtime image to
   `REGISTRY/REPOSITORY@sha256:<64-lowercase-hex>`.
7. Run immutable action/image scans and the credential scan before updating
   Git.

The repository's workflows demonstrate this pattern for the operator,
control-plane API, and `demo-http`. Private images also require a
namespace-scoped `ghcr-pull` Secret created separately; Secret data never
belongs in a manifest or documentation example.

## FastAPI Stateless Example

FastAPI is not a `ManagedService` template. Onboard it through the recommended
GitOps package. The following is a shell-rendered starting point; replace the
placeholders before validation:

```bash
export APP_NAME=example-api
export APP_NAMESPACE=applications
export APP_IMAGE='ghcr.io/example/example-api@sha256:REPLACE_WITH_64_HEX_DIGEST'
export APP_PORT=8000
export APP_HOSTNAME=example.platform.example.com
```

The application is expected to listen on `0.0.0.0:8000`, provide `/healthz`,
and use one initial replica. Add a distinct `/readyz` endpoint when database
or dependency readiness differs from process health.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ${APP_NAME}
  namespace: ${APP_NAMESPACE}
  labels:
    app.kubernetes.io/name: ${APP_NAME}
    app.kubernetes.io/component: api
    app.kubernetes.io/part-of: cloud-native-service-control-plane
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: ${APP_NAME}
  template:
    metadata:
      labels:
        app.kubernetes.io/name: ${APP_NAME}
        app.kubernetes.io/component: api
        app.kubernetes.io/part-of: cloud-native-service-control-plane
    spec:
      automountServiceAccountToken: false
      imagePullSecrets:
        - name: ghcr-pull
      securityContext:
        runAsNonRoot: true
        runAsUser: 65532
        runAsGroup: 65532
        seccompProfile:
          type: RuntimeDefault
      containers:
        - name: ${APP_NAME}
          image: ${APP_IMAGE}
          imagePullPolicy: IfNotPresent
          ports:
            - name: http
              containerPort: ${APP_PORT}
          env:
            - name: PORT
              value: "${APP_PORT}"
          livenessProbe:
            httpGet:
              path: /healthz
              port: http
            periodSeconds: 10
            timeoutSeconds: 2
          readinessProbe:
            httpGet:
              path: /healthz
              port: http
            periodSeconds: 5
            timeoutSeconds: 2
          resources:
            requests:
              cpu: 10m
              memory: 32Mi
            limits:
              cpu: 200m
              memory: 256Mi
          securityContext:
            allowPrivilegeEscalation: false
            readOnlyRootFilesystem: true
            capabilities:
              drop:
                - ALL
---
apiVersion: v1
kind: Service
metadata:
  name: ${APP_NAME}
  namespace: ${APP_NAMESPACE}
spec:
  type: ClusterIP
  selector:
    app.kubernetes.io/name: ${APP_NAME}
  ports:
    - name: http
      port: 80
      targetPort: http
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: ${APP_NAME}
  namespace: ${APP_NAMESPACE}
spec:
  secretName: ${APP_NAME}-tls
  dnsNames:
    - ${APP_HOSTNAME}
  issuerRef:
    group: cert-manager.io
    kind: ClusterIssuer
    name: letsencrypt-production
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: ${APP_NAME}
  namespace: ${APP_NAMESPACE}
  annotations:
    traefik.ingress.kubernetes.io/router.middlewares: ${APP_NAMESPACE}-${APP_NAME}-redirect-https@kubernetescrd
spec:
  ingressClassName: traefik
  rules:
    - host: ${APP_HOSTNAME}
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: ${APP_NAME}
                port:
                  number: 80
  tls:
    - hosts:
        - ${APP_HOSTNAME}
      secretName: ${APP_NAME}-tls
---
apiVersion: traefik.io/v1alpha1
kind: Middleware
metadata:
  name: ${APP_NAME}-redirect-https
  namespace: ${APP_NAMESPACE}
spec:
  redirectScheme:
    scheme: https
    permanent: true
```

Render placeholders to a temporary file and inspect it before adding the
equivalent resources to a Kustomize package:

```bash
envsubst < application-template.yaml > /tmp/application-rendered.yaml
kubectl apply --dry-run=client --validate=false \
  -f /tmp/application-rendered.yaml
```

The example deliberately omits database credentials, application RBAC, and
NetworkPolicy because those require application-specific decisions. Add
default-deny plus exact DNS, ingress, dependency, and optional metrics paths
before rollout. If PostgreSQL is required, treat it as an external dependency
or a separately reviewed stateful service; provide its connection Secret
outside Git.

## Deployment Lifecycle

### Preflight and review

- Confirm the branch and clean worktree.
- Verify the source commit used for the image and retrieve its immutable
  registry digest.
- Review namespace ownership, AppProject permissions, public exposure, Secret
  references, NetworkPolicies, resources, and storage consequences.
- Render with `kubectl kustomize <package>` and run repository tests/scans.

### Commit, publish configuration, and synchronize

Commit the reviewed source of truth and push through the normal repository
review process. Image publication precedes adopting its digest. Argo CD
synchronization is manual: inspect the exact revision and diff, then follow
the [synchronization and rollback runbook](argocd-sync-rollback-runbook.md).

### Verify

Check:

```bash
kubectl get deployment,pod,service,endpointslice -n APP_NAMESPACE
kubectl describe deployment -n APP_NAMESPACE APP_NAME
kubectl get events -n APP_NAMESPACE --sort-by='.lastTimestamp'
kubectl logs -n APP_NAMESPACE deployment/APP_NAME
```

For a public application, verify DNS, redirect, certificate, TLS, and the
intended route. For private metrics, confirm the exact Prometheus target is
`up`; adding a new target requires a reviewed scrape job and target-specific
NetworkPolicy, not only a metrics label.

### Safe update and rollback

Publish a new immutable image, update the digest in Git, render and validate,
review the Argo diff, then synchronize manually. If health fails, use the
runbook to restore a previously reviewed Git revision. Do not patch the live
Deployment as a durable fix.

### Removal

Remove the Application and package only through a reviewed change and explicit
cleanup plan. Determine whether PVCs, external databases, certificates, DNS,
and manually provisioned Secrets must be retained. Never assume pruning a
stateful resource is reversible.

## Stateful Application Boundary

The cluster supports persistent volumes, and Prometheus demonstrates
local-path persistence. Database schema migration, backup, restore, upgrade,
failover, corruption recovery, and retention remain application
responsibilities. In-cluster PostgreSQL and Redis are not part of the current
baseline. A single-node local-path volume is neither highly available nor an
off-node backup.

## Capacity

A lightweight stateless application can fit the measured baseline without an
immediate resize, but this is not a guarantee. Record node and Pod CPU/memory
before deployment, ensure requests fit while retaining safety margin, and
measure again after rollout and representative traffic. Account for rollout
surge, Argo CD, system components, and local disk growth; do not size from
application Pod metrics alone.

## Troubleshooting

| Symptom | Checks |
| --- | --- |
| `ImagePullBackOff` | Exact digest, GHCR package access, namespace-local `ghcr-pull`, Pod events |
| Readiness failure | Probe path/port, bind address, dependency readiness, logs and events |
| No Service endpoints | Service selector versus Pod labels; Pod readiness; EndpointSlices |
| Ingress/TLS failure | DNS, Ingress class/host, Service port, Certificate/Request/Order/Challenge, redirect middleware |
| Network timeout | Default-deny policy, ingress source, DNS egress, dependency CIDR/port, CNI enforcement |
| Argo `OutOfSync`/`Degraded` | Exact target revision, rendered diff, operation state, resource events; do not enable automatic repair |
| CPU/memory pressure | Requests/limits, `kubectl top`, node allocatable/headroom, OOM or eviction events |
| Missing Prometheus target | Scrape job selectors/relabeling, metrics Service/port, target policy, `/targets` status |

## Completion Checklist

- [ ] Image builds and runs as non-root on `0.0.0.0`.
- [ ] Liveness, readiness, and graceful shutdown are tested.
- [ ] Runtime image is pinned by SHA-256 digest.
- [ ] No credential or Secret value is committed.
- [ ] Kustomize renders deterministically.
- [ ] Selectors, labels, ports, probes, and Services agree.
- [ ] Requests, limits, and rollout capacity are reviewed.
- [ ] Default-deny and exact required network paths are defined.
- [ ] RBAC and AppProject permissions contain no convenience wildcards.
- [ ] Optional Ingress uses exact DNS, production TLS, and HTTPS redirect.
- [ ] GitOps Application remains restricted and manual-sync.
- [ ] Repository tests, lint, build, render, immutable-reference, and credential
      checks pass.
- [ ] Deployment, Pod, Service, endpoint, event, and log health is verified.
- [ ] Metrics discovery is verified when added.
- [ ] A reviewed previous revision and removal/data-retention plan exist.
