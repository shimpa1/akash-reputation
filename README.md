# akash-reputation

A signed reputation service for Akash providers and clients. Providers rate the
deployers they leased to, and deployers rate the provider — each rating is an
off-chain **ADR-036 signed** statement, accepted **only once per deployment** and
only when the two parties actually had a lease on-chain.

## Why signatures + lease checks

- **Attributable** — every rating carries a secp256k1 signature that recovers to
  the rater's `akash…` address. It can't be forged or repudiated, and anyone can
  re-verify a stored rating from its `pubkey` + `signature`.
- **Earned** — a rating is rejected unless a lease `(deployer, dseq, gseq, oseq,
  provider)` is known (synced from the chain via `provider-services`, with a live
  fallback). You can't rate someone you never transacted with.
- **One per deployment** — a unique index on `(author, deployer, dseq)` blocks
  duplicate/spam ratings.

Reputation is kept off-chain (no gas); the signatures make each entry portable
and independently verifiable.

## Layout

| Path | What |
| ---- | ---- |
| `cmd/reputation` | the HTTP API server |
| `cmd/repute-sign` | the signing CLI |
| `internal/adr036` | canonical message + ADR-036 sign/verify + akash address derivation (shared by server & CLI) |
| `internal/store` | PostgreSQL store, schema, aggregate queries |
| `internal/leases` | background poller that ingests on-chain leases |
| `internal/api` | HTTP handlers |
| `deploy/` | k8s manifests (namespace, postgres StatefulSet on a resizable `beta3` PVC, deployment, service) |
| `examples/` | example ConfigMap and Secret — copy, fill in your values, and apply (do not commit real values here) |
| `Dockerfile` | multi-stage build; bundles a matching `provider-services` |

The public ingress/route for your host (e.g. an HTTPRoute or Ingress) is
cluster-specific and belongs in your own cluster-config, not in this repo.

## HTTP API

| Method | Path | Purpose |
| ------ | ---- | ------- |
| `GET`  | `/healthz` | liveness/readiness (checks DB) |
| `POST` | `/feedback` | submit a signed rating (verify → lease check → uniqueness) |
| `GET`  | `/reputation/{address}` | aggregate as the rated party: `{positive, negative, net, deployments_rated}` |
| `GET`  | `/feedback?subject=&author=&role=` | raw rows incl. signature, for independent re-verification |
| `GET`  | `/leases?provider=&owner=` | known leases (debug) |

### Signed submission shape

```json
{
  "feedback": {
    "v": "akash-reputation/v1",
    "role": "provider",
    "provider": "akash18ga…",
    "deployer": "akash1…",
    "dseq": "13837770", "gseq": "1", "oseq": "1",
    "score": -1,
    "comment": "slow to pay",
    "issued_at": "2026-06-13T10:00:00Z"
  },
  "pubkey": "<base64 compressed secp256k1>",
  "signature": "<base64 64-byte R||S>"
}
```

`role=provider` ⇒ the signer must be the `provider`; `role=deployer` ⇒ the signer
must be the `deployer`. The signature is over the amino ADR-036 `StdSignDoc`
wrapping the canonical (sorted-key) JSON of the `feedback` object, so a browser
wallet's `signArbitrary` (e.g. Keplr) produces a compatible signature.

## Signing a rating (CLI)

Export the rater's account key from the keyring and pipe it in — the key never
appears in shell args:

```bash
provider-services keys export <key-name> --unarmored-hex --unsafe | \
  go run ./cmd/repute-sign --role provider \
    --deployer akash1<deployer-address> \
    --dseq <dseq> --down --comment "slow to pay" \
    --api https://<your-reputation-host>
```

Without `--api` it prints the signed envelope to stdout. The provider/deployer
"self" side is auto-filled from the key. Use `--up` / `--down` (or `--score`).

## Build & push the image

The cluster is amd64; build with buildx and push in one step. Use an immutable
version tag (not `:latest`) and bump it to roll a new release —
`deploy/deployment.yaml` pins the same tag:

```bash
VERSION=0.1.0
docker buildx build --platform linux/amd64 -t shimpa/akash-reputation:$VERSION --push .
```

The image bundles a matching `provider-services` binary (pinned by the
`PROVIDER_SERVICES_VERSION` build arg) for the lease poller. Note: provider-services
0.11.x needs glibc ≥ 2.39, so the runtime base is `debian:trixie-slim`.

## Deploy

```bash
# 1. Namespace
kubectl apply -f deploy/namespace.yaml

# 2. Create the postgres secret with a real password (do NOT commit it):
kubectl create secret generic reputation-postgres-secret -n reputation \
  --from-literal=POSTGRES_USER=reputation \
  --from-literal=POSTGRES_PASSWORD="$(openssl rand -hex 24)" \
  --from-literal=POSTGRES_DB=reputation

# 3. Create the config from the example (set PROVIDER_ADDR + AKASH_NODE first):
cp examples/configmap.example.yaml /tmp/reputation-config.yaml
$EDITOR /tmp/reputation-config.yaml
kubectl apply -f /tmp/reputation-config.yaml

# 4. Apply the workloads:
kubectl apply -f deploy/          # postgres, deployment, service

# 5. Expose it with your own cluster ingress/route, then verify
kubectl -n reputation get pods,pvc
curl -sSI https://<your-reputation-host>/healthz
```

Exposing the service publicly (Ingress/Gateway HTTPRoute, TLS cert, DNS) is
cluster-specific — wire it up in your own cluster-config, pointing at the
`reputation` Service on port 8080.

### Resizing storage (beta3 is expandable)

```bash
kubectl -n reputation patch pvc postgres-data-reputation-postgres-0 \
  --type merge -p '{"spec":{"resources":{"requests":{"storage":"10Gi"}}}}'
```

Rook Ceph RBD + the filesystem grow online (the `beta3` StorageClass has
`allowVolumeExpansion: true`).

## Configuration (env)

| Var | Default | Notes |
| --- | ------- | ----- |
| `PROVIDER_ADDR` | — (required) | provider whose leases anchor the checks |
| `AKASH_NODE` | — | RPC endpoint for `provider-services` (in-cluster node) |
| `DATABASE_URL` | — | full DSN; or set `POSTGRES_USER/PASSWORD/DB` + `PGHOST/PGPORT` |
| `LEASE_SYNC_INTERVAL` | `10m` | how often to resync leases |
| `LISTEN_ADDR` | `:8080` | HTTP listen address |
| `PROVIDER_SERVICES_BIN` | `provider-services` | poller binary path |

## Scope

Run one instance per provider — it anchors ratings to that provider's leases
(`PROVIDER_ADDR`). The signing/verification format is provider-agnostic, so any
provider can run their own instance and the data could later be federated.
