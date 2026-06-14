// TypeScript port of the server's internal/adr036 package. It must produce
// byte-identical canonical message + ADR-036 sign-doc to the Go code, so that
// (a) wallet signatures verify server-side and (b) we can re-verify stored
// ratings in the browser. Pinned by __tests__/adr036.test.ts against Go output.

import { Secp256k1, Secp256k1Signature, sha256, ripemd160 } from "@cosmjs/crypto";
import { fromBase64, toBase64, toBech32, toUtf8 } from "@cosmjs/encoding";
import type { Feedback, FeedbackRecord, Role } from "./types";

// goJSONString replicates Go's encoding/json string escaping (HTMLEscape on):
// short escapes for " \ \n \r \t; <, >, & and U+2028/U+2029 as \uXXXX; other
// control chars < 0x20 as \u00XX; everything else verbatim (UTF-8).
function goJSONString(s: string): string {
  let out = '"';
  for (const ch of s) {
    const c = ch.codePointAt(0)!;
    switch (ch) {
      case '"': out += '\\"'; break;
      case "\\": out += "\\\\"; break;
      case "\n": out += "\\n"; break;
      case "\r": out += "\\r"; break;
      case "\t": out += "\\t"; break;
      case "<": out += "\\u003c"; break;
      case ">": out += "\\u003e"; break;
      case "&": out += "\\u0026"; break;
      default:
        if (c === 0x2028 || c === 0x2029 || c < 0x20) {
          out += "\\u" + c.toString(16).padStart(4, "0");
        } else {
          out += ch;
        }
    }
  }
  return out + '"';
}

// canonicalFeedbackBytes returns the deterministic JSON that is actually signed,
// with lexicographically sorted keys and no whitespace (matches Go's
// marshalCanonical: comment, deployer, dseq, gseq, issued_at, oseq, provider,
// role, score, v).
export function canonicalFeedbackBytes(f: Feedback): string {
  return (
    "{" +
    '"comment":' + goJSONString(f.comment) + "," +
    '"deployer":' + goJSONString(f.deployer) + "," +
    '"dseq":' + goJSONString(f.dseq) + "," +
    '"gseq":' + goJSONString(f.gseq) + "," +
    '"issued_at":' + goJSONString(f.issued_at) + "," +
    '"oseq":' + goJSONString(f.oseq) + "," +
    '"provider":' + goJSONString(f.provider) + "," +
    '"role":' + goJSONString(f.role) + "," +
    '"score":' + String(f.score) + "," +
    '"v":' + goJSONString(f.v) +
    "}"
  );
}

// buildSignDoc wraps the canonical message in the amino ADR-036 StdSignDoc, the
// exact bytes a wallet hashes and signs (and what the server reconstructs).
export function buildSignDoc(canonical: string, signer: string): string {
  const data = toBase64(toUtf8(canonical));
  return (
    '{"account_number":"0","chain_id":"","fee":{"amount":[],"gas":"0"},"memo":"",' +
    '"msgs":[{"type":"sign/MsgSignData","value":{"data":"' + data + '","signer":"' + signer + '"}}],' +
    '"sequence":"0"}'
  );
}

// akashAddressFromPubkey derives the bech32 akash address from a compressed
// secp256k1 public key: ripemd160(sha256(pubkey)) -> bech32("akash").
export function akashAddressFromPubkey(pubkey: Uint8Array): string {
  return toBech32("akash", ripemd160(sha256(pubkey)));
}

function feedbackFromRecord(r: FeedbackRecord): Feedback {
  return {
    v: "akash-reputation/v1",
    role: r.role as Role,
    provider: r.provider,
    deployer: r.deployer,
    dseq: r.dseq,
    gseq: r.gseq,
    oseq: r.oseq,
    score: r.score,
    comment: r.comment,
    issued_at: r.issued_at,
  };
}

// verifyRecord re-checks a stored rating client-side: the signature must be a
// valid secp256k1 signature by `pubkey` over the rating's sign-doc, and the
// address derived from `pubkey` must equal the rating's author.
export async function verifyRecord(r: FeedbackRecord): Promise<boolean> {
  try {
    const pubkey = fromBase64(r.pubkey);
    const sig = fromBase64(r.signature);
    if (sig.length !== 64) return false;
    if (akashAddressFromPubkey(pubkey) !== r.author) return false;
    const canonical = canonicalFeedbackBytes(feedbackFromRecord(r));
    const doc = buildSignDoc(canonical, r.author);
    const hash = sha256(toUtf8(doc));
    return await Secp256k1.verifySignature(Secp256k1Signature.fromFixedLength(sig), hash, pubkey);
  } catch {
    return false;
  }
}
