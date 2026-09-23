# SmartEnergy Secret provisioning

SmartEnergy runtime Secrets are created manually outside Git. Never commit Secret manifests or generated values. Store each value as exact bytes without an unintended trailing newline in a protected local file, use file paths rather than literal values on a command line, and remove the local files after verifying recovery procedures. Command-line literals can be retained in shell history and exposed through process inspection.

The following example assumes an administrator-controlled directory outside this repository:

```bash
secret_dir=/secure/local/path/smartenergy
chmod 700 "$secret_dir"
```

Create or update the three required Secrets without writing rendered Secret YAML to disk:

```bash
kubectl -n smartenergy create secret generic smartenergy-runtime \
  --from-file=JWT_SECRET="$secret_dir/JWT_SECRET" \
  --dry-run=client -o yaml | kubectl apply -f -

kubectl -n smartenergy create secret generic smartenergy-postgres \
  --from-file=POSTGRES_DB="$secret_dir/POSTGRES_DB" \
  --from-file=POSTGRES_USER="$secret_dir/POSTGRES_USER" \
  --from-file=POSTGRES_PASSWORD="$secret_dir/POSTGRES_PASSWORD" \
  --from-file=DATABASE_URL="$secret_dir/DATABASE_URL" \
  --dry-run=client -o yaml | kubectl apply -f -

kubectl -n smartenergy create secret generic smartenergy-redis \
  --from-file=REDIS_PASSWORD="$secret_dir/REDIS_PASSWORD" \
  --from-file=REDIS_URL="$secret_dir/REDIS_URL" \
  --from-file=CELERY_BROKER_URL="$secret_dir/CELERY_BROKER_URL" \
  --from-file=CELERY_RESULT_BACKEND="$secret_dir/CELERY_RESULT_BACKEND" \
  --dry-run=client -o yaml | kubectl apply -f -
```

`DATABASE_URL` must use `smartenergy-postgres:5432`. Redis uses the same password with logical database 0 for `REDIS_URL`, 1 for `CELERY_BROKER_URL`, and 2 for `CELERY_RESULT_BACKEND`.

| Secret | Purpose and consumers | Rotation implications |
| --- | --- | --- |
| `smartenergy-runtime` | `JWT_SECRET` for the API | Rotation invalidates existing JWTs; restart the API after the controlled update. |
| `smartenergy-postgres` | PostgreSQL initialization plus `DATABASE_URL` for migration, API, worker, and Beat | Coordinate the database role password and all application clients. Validate migration and readiness after restart. |
| `smartenergy-redis` | Redis authentication and URLs for API, worker, and Beat | Coordinate the Redis password and all three logical-database URLs, then restart clients and verify Celery. |

`smartenergy-backup` is not required. If off-node backup and disaster recovery are implemented later, define their destination, Secret contract, retention, restore validation, and recovery runbook as a separately reviewed enhancement.

Before deployment, inspect only metadata and key names:

```bash
kubectl -n smartenergy get secret \
  smartenergy-runtime smartenergy-postgres smartenergy-redis
kubectl -n smartenergy describe secret smartenergy-runtime
kubectl -n smartenergy describe secret smartenergy-postgres
kubectl -n smartenergy describe secret smartenergy-redis
```
