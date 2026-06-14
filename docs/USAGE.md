# Using the Akash Reputation Service

This is a plain-language guide for people who just want to **use** the service —
to look up how trustworthy an Akash provider (or client) is, or to leave their
own rating after a deployment. No prior knowledge required.

> **The easiest way is the web app.** If the instance has a web UI (the europlots
> instance serves one at its root URL), you can look up reputation and leave a
> rating from the browser — connect Keplr or Leap and sign with one click; no key
> export, nothing leaves your wallet. See [Use the web app](#9-use-the-web-app).
> The `curl` / CLI flow below is the manual alternative for power users and scripts.

---

## 1. What this is

On the [Akash Network](https://akash.network), **deployers** (also called
tenants or clients) rent compute from **providers**. After they've worked
together on a deployment, each side may want to vouch for — or warn about — the
other:

- a **provider** can say *"this deployer paid on time / was a problem"*,
- a **deployer** can say *"this provider was reliable / had downtime"*.

This service collects those ratings and lets anyone look them up. Two things
make the ratings trustworthy:

1. **Every rating is cryptographically signed** by the rater's Akash account, so
   it can't be faked or put in someone else's mouth. Anyone can re-check the
   signature themselves.
2. **You can only rate someone you actually leased with**, and **only once per
   deployment** — so nobody can spam ratings or rate strangers.

Ratings are kept off the blockchain (so leaving one is free — no gas), but each
one carries a signature that proves who said it.

---

## 2. A few words you'll see

| Term | Meaning |
| ---- | ------- |
| **Provider** | who runs the servers. Identified by an `akash1…` address. |
| **Deployer** | who rents the servers (the client/tenant). Also an `akash1…` address. |
| **Lease** | one agreement between a deployer and a provider for a deployment. |
| **dseq / gseq / oseq** | the numbers that identify a specific deployment/lease on-chain (deployment, group, order sequence). |
| **Score** | a rating is either **+1** (positive) or **-1** (negative). |
| **Reputation** | the running tally of all the +1 / -1 ratings an address has received. |

You don't need an account or a key just to **look up** reputation. You only need
your Akash key to **leave** a rating.

---

## 3. Look up a reputation (no account needed)

Every instance of this service has a base URL. Wherever you see
`<reputation-host>` below, replace it with the one you were given (e.g.
`https://reputation.example.com`).

### How well-regarded is an address?

```bash
curl https://<reputation-host>/reputation/akash1<the-address-you-care-about>
```

Example response:

```json
{
  "address": "akash1...",
  "positive": 12,
  "negative": 1,
  "net": 11,
  "deployments_rated": 13
}
```

- `positive` / `negative` — how many +1 and -1 ratings this address has received.
- `net` — positive minus negative (a quick "score").
- `deployments_rated` — how many distinct deployments these ratings came from.

This works for **both** providers and deployers — anyone who has been rated. If
an address has no ratings yet, you'll get all zeros.

### See the individual ratings (with comments)

```bash
curl "https://<reputation-host>/feedback?subject=akash1<the-address>"
```

Each entry shows who rated them (`author`), the score, an optional `comment`,
the deployment it was about, and the `pubkey` + `signature` so the rating can be
independently verified (see [section 6](#6-verify-a-rating-yourself)).

You can also filter by who *wrote* ratings: `?author=akash1…`, or by direction:
`?role=provider` (providers rating deployers) or `?role=deployer` (deployers
rating providers).

---

## 4. Leave a rating

To leave a rating you need:

1. **`provider-services`** installed (the Akash CLI):
   <https://akash.network/docs/deployments/akash-cli/installation/>
2. **Your Akash key** in its keyring — the *same account* that was the provider
   or the deployer on the lease you're rating. (You can only rate as yourself.)
3. **A real lease** between you and the other party. You can rate while the lease
   is active or after it has closed.

### Step 1 — find the deployment you're rating

You need the other party's address and the lease numbers (`dseq`, `gseq`,
`oseq`). List your leases:

```bash
# If you are the DEPLOYER, list leases you created:
provider-services query market lease list --owner akash1<your-address> -o json

# If you are the PROVIDER, list leases on your provider:
provider-services query market lease list --provider akash1<your-address> -o json
```

Each lease shows an `id` with `owner` (the deployer), `provider`, `dseq`,
`gseq`, `oseq`. Note the ones for the deployment you want to rate.

### Step 2 — get the signing tool

The signer is a small command called `repute-sign`. With Go installed:

```bash
go install github.com/shimpa1/akash-reputation/cmd/repute-sign@latest
# installs to $(go env GOPATH)/bin/repute-sign
```

(Or build it from a clone: `go build -o repute-sign ./cmd/repute-sign`.)

For the full `repute-sign` reference — build options, every flag, how the key is
read, and more examples — see **[repute-sign.md](repute-sign.md)**.

### Step 3 — sign and submit your rating

`repute-sign` signs the rating with your key and sends it. Your key is read once
(piped in) and never written anywhere.

**As a deployer rating a provider:**

```bash
provider-services keys export <your-key-name> --unarmored-hex --unsafe 2>&1 \
  | repute-sign --role deployer \
      --provider akash1<the-provider-address> \
      --dseq <dseq> --gseq <gseq> --oseq <oseq> \
      --up --comment "fast and reliable" \
      --api https://<reputation-host>
```

**As a provider rating a deployer:** use `--role provider` and
`--deployer akash1<the-deployer-address>` instead of `--provider`.

Flags:

- `--up` = +1, `--down` = -1 (or `--score 1` / `--score -1`).
- `--comment` is optional (max 1000 characters).
- Your own side (provider or deployer, whichever you are) is filled in
  automatically from your key — you only name the *other* party.
- Drop `--api` to just **print** the signed rating instead of sending it (useful
  to inspect it first); add it back to submit.

You'll be prompted to type `y` to confirm the key export and then your keyring
passphrase. On success you'll see:

```json
{"id": 42, "author": "akash1<you>", "subject": "akash1<them>", "score": 1}
```

### Step 4 — confirm it landed

```bash
curl https://<reputation-host>/reputation/akash1<the-address-you-rated>
```

The counts should reflect your new rating.

---

## 5. The rules (so nothing surprises you)

- **One rating per deployment, per rater.** Trying to rate the same deployment
  twice returns `409 Conflict`. Ratings are immutable — pick +1 or -1 deliberately.
- **You must have had a lease** with the other party for that deployment, or the
  rating is rejected (`422`).
- **You can only rate as yourself** — the signature must come from the provider
  (for `--role provider`) or the deployer (for `--role deployer`). Otherwise `401`.
- Scores are only **+1** or **-1**.

---

## 6. Verify a rating yourself

Because every rating is signed, you don't have to trust the service — you can
re-check any rating. `GET /feedback?...` returns each rating's `pubkey` and
`signature`. A rating is genuine if:

1. the signature is a valid secp256k1 signature, by that `pubkey`, over the
   ADR-036 message built from the rating's fields, and
2. the Akash address derived from that `pubkey` equals the rating's `author`.

The signing format is the standard Cosmos **ADR-036** off-chain message
(`sign/MsgSignData`), the same one wallets like Keplr produce with
`signArbitrary` — so any ADR-036-aware tooling can verify it. The exact bytes
that get signed are the canonical (sorted-key) JSON of the rating's `feedback`
object.

---

## 7. If something goes wrong

| You see | What it means | Fix |
| ------- | ------------- | --- |
| `422` "no lease found…" | No lease between you and the other party for that `dseq/gseq/oseq`. | Double-check the addresses and sequence numbers from `lease list`. |
| `401` "signature verification failed" | The signer isn't the party you claim to be rating as, or the rating was altered. | Make sure `--role` matches the key you signed with, and that you're naming the *other* party. |
| `409` "already recorded" | You already rated this deployment. | Ratings are one-per-deployment and can't be changed. |
| `400` "invalid akash address" | A malformed `akash1…` address. | Re-copy the address. |
| `no 64-char hex private key found` | The key didn't reach `repute-sign`. | Make sure you piped `keys export … --unarmored-hex --unsafe` into it. |

---

## 8. API reference (for the curious)

| Method | Path | Purpose | Needs a signature? |
| ------ | ---- | ------- | ------------------ |
| `GET`  | `/reputation/{address}` | aggregate score for an address | no |
| `GET`  | `/feedback?subject=&author=&role=` | individual ratings (with signatures) | no |
| `GET`  | `/leases?provider=&owner=` | leases the service knows about | no |
| `GET`  | `/info` | this instance's provider address + chain id | no |
| `GET`  | `/healthz` | service health | no |
| `POST` | `/feedback` | submit a signed rating (use `repute-sign` or the web app) | **yes** |

That's it — looking up reputation is a single `curl`, and leaving one is a single
piped command.

---

## 9. Use the web app

If the instance ships the web UI (served at its root URL — e.g.
`https://reputation.europlots.com/`), it's the easiest way in:

**Look up reputation** — open the site, type any `akash1…` address, and you'll see
the aggregate score and every rating. Each rating shows a **✓ verified** badge once
the browser re-checks its signature (section 6) — so you're trusting the math, not
the server.

**Leave a rating** — click **Connect** and approve in **Keplr** or **Leap**:

- If your wallet is a **deployer** that leased from this provider, the app finds
  your deployments automatically; pick one, choose 👍/👎, add an optional comment,
  and click **Sign & submit**. Your wallet pops up to sign — approve it.
- If your wallet is the **provider**, you get a dashboard: enter a deployer's
  address to see your leases with them and rate each deployment.

The signature is produced **inside your wallet** (`signArbitrary`); your private
key is never exported and nothing but the finished, signed rating is sent. It's the
same ADR-036 signature the CLI produces — the web app is just a friendlier front end
over the exact same API.
