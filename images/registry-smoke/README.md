# Registry Smoke Image

A minimal HTTP image used to validate:

- GitHub Actions container builds;
- GitHub Container Registry publication;
- immutable commit-based tags;
- Kubernetes image pulling;
- runtime image-to-commit traceability.

The container runs as UID/GID `65534`, listens on port `8080`, and returns:

```json
{
  "service": "registry-smoke",
  "revision": "<full Git commit>",
  "built_at": "<UTC build time>"
}
```
