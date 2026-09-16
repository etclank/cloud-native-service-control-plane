# Third-Party Provenance

No repository-level license is selected by this review. Existing Go files and
Kubebuilder boilerplate contain Apache 2.0 notices; retain those notices. A
maintainer must choose the licensing scope for other original repository content
before describing the entire project as open source.

| Material | Source and identity |
| --- | --- |
| Argo CD installation manifest | `kubernetes/bootstrap/argocd/upstream/install-v3.4.5.yaml`; upstream Argo CD v3.4.5, checksum in adjacent `SHA256SUMS` |
| Collector Helm chart | Official OpenTelemetry Helm repository; version locked in `deploy/observability/Chart.lock` |
| Prometheus and kube-state-metrics charts | Prometheus Community Helm repository; versions locked in the same file |
| Go libraries | Module identities and versions in `go.mod`, integrity hashes in `go.sum` |
| Container bases | Dockerfiles identify Go, distroless, and BusyBox bases; principal Go/runtime bases are digest pinned |

Dependency chart archives are downloaded, checksum-verified by the Makefile,
and ignored by Git. Their upstream notices and licenses remain applicable.
Container images contain their own third-party software; repository licensing
does not replace those obligations. The registry smoke Dockerfile uses a
versioned BusyBox tag rather than a digest, unlike the main Go image bases.

The vendored Argo CD manifest is upstream deployment material, not proprietary
application source. Keep its checksum and provenance when updating it.
