# `repute-sign` — the rating signer

`repute-sign` is the command-line tool that **signs** an Akash reputation rating
with your account key and (optionally) **submits** it to a reputation service. It
produces the exact ADR-036 signature the server verifies, so a rating made with
it is cryptographically attributable to your `akash1…` address.

You only need it to **leave** a rating. Looking up reputation is just `curl`
against the service (see [USAGE.md](USAGE.md)).

---

## Install / build

**With Go (simplest):**

```bash
go install github.com/shimpa1/akash-reputation/cmd/repute-sign@latest
# binary lands in $(go env GOPATH)/bin — make sure that's on your PATH
```

**From a clone:**

```bash
git clone https://github.com/shimpa1/akash-reputation
cd akash-reputation
go build -o repute-sign ./cmd/repute-sign      # produces ./repute-sign
```

**Cross-compile** (e.g. a Linux amd64 binary from a Mac):

```bash
GOOS=linux GOARCH=amd64 go build -o repute-sign-linux-amd64 ./cmd/repute-sign
```

It's a single static binary with no runtime dependencies. (It is **not** included
in the server container image — that image only runs the service.)

Requires Go 1.24+ to build.

---

## How it works (in one breath)

You give it the lease you're rating, a score, and your key. It builds the
canonical rating message, wraps it in the Cosmos **ADR-036** `StdSignDoc`, signs
it with secp256k1, and either prints the signed bundle or `POST`s it to
`<api>/feedback`. The server re-derives your `akash1…` address from the signature
and checks it matches the party you're rating as — so **you can only rate as
yourself**.

---

## Providing your key (it never touches disk)

The signing key is your account's **unarmored hex** private key. `repute-sign`
reads it, in this order of precedence:

1. `--key-hex <hex>` flag (visible in your shell history — avoid for real keys),
2. the `REPUTE_KEY_HEX` environment variable,
3. **standard input** (the recommended way) — pipe it straight from the keyring:

```bash
provider-services keys export <your-key-name> --unarmored-hex --unsafe 2>&1 | repute-sign …
```

`keys export` prints a `WARNING:` line and passphrase prompts on the same stream;
`repute-sign` ignores that noise and extracts the 64-character hex key. The key
is held only in memory for the moment it signs.

> Security: `--unarmored-hex --unsafe` exposes your raw private key. Only pipe it
> directly into `repute-sign`; don't paste it anywhere or leave it in a file.

---

## Flags

| Flag | Required | Default | Meaning |
| ---- | -------- | ------- | ------- |
| `--role` | yes | — | `provider` (you're rating a deployer) or `deployer` (you're rating a provider) |
| `--provider` | see note | — | the provider's `akash1…` address |
| `--deployer` | see note | — | the deployer's `akash1…` address |
| `--dseq` | yes | — | deployment sequence of the lease |
| `--gseq` | no | `1` | group sequence |
| `--oseq` | no | `1` | order sequence |
| `--up` | one of | — | positive rating (+1) |
| `--down` | these | — | negative rating (-1) |
| `--score` | three | `0` | explicit score, `1` or `-1`; overrides `--up`/`--down` |
| `--comment` | no | — | free-text note, max 1000 chars |
| `--api` | no | — | reputation service base URL; if set, the rating is POSTed there |
| `--key-hex` | no | — | provide the key inline instead of via stdin/env |

**Note on `--provider` / `--deployer`:** name only the *other* party. Your own
side is filled in automatically from your key, so:

- `--role deployer` → you must supply `--provider` (the one you're rating); your
  deployer address comes from the key.
- `--role provider` → you must supply `--deployer`; your provider address comes
  from the key.

---

## Examples

**A deployer rates a provider, positively, and submits it:**

```bash
provider-services keys export my-key --unarmored-hex --unsafe 2>&1 \
  | repute-sign --role deployer \
      --provider akash1<provider-address> \
      --dseq 27272026 --gseq 1 --oseq 1 \
      --up --comment "fast, no downtime" \
      --api https://<reputation-host>
```

Success prints the stored record id:

```json
{"id": 42, "author": "akash1<you>", "subject": "akash1<provider>", "score": 1}
```

**A provider rates a deployer, negatively:**

```bash
provider-services keys export provider-key --unarmored-hex --unsafe 2>&1 \
  | repute-sign --role provider \
      --deployer akash1<deployer-address> \
      --dseq 12345 \
      --down --comment "did not pay" \
      --api https://<reputation-host>
```

**Sign only, don't submit** (omit `--api`) — useful to inspect or to submit
out-of-band. It prints the signed envelope:

```json
{
  "feedback": {
    "v": "akash-reputation/v1",
    "role": "deployer",
    "provider": "akash1...",
    "deployer": "akash1...",
    "dseq": "27272026", "gseq": "1", "oseq": "1",
    "score": 1,
    "comment": "fast, no downtime",
    "issued_at": "2026-06-14T10:39:18Z"
  },
  "pubkey": "<base64 compressed secp256k1>",
  "signature": "<base64 64-byte R||S>"
}
```

You can submit that envelope yourself any time:

```bash
curl -X POST https://<reputation-host>/feedback \
  -H 'Content-Type: application/json' --data @envelope.json
```

---

## Exit behaviour

- Prints the signed envelope (no `--api`) or the server's JSON response (`--api`).
- Exits non-zero with an `error: …` message on failure (bad flags, no key found,
  score not ±1, key address not matching the rated party, or an HTTP error from
  the server such as `409` duplicate / `422` no-such-lease / `401` bad signature).

See [USAGE.md](USAGE.md) for the end-to-end walkthrough and what those server
errors mean.
