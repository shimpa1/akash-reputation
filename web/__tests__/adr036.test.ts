import { describe, expect, it } from "vitest";
import { canonicalFeedbackBytes, buildSignDoc } from "../src/adr036";
import type { Feedback } from "../src/types";

// Vectors generated from the Go server (internal/adr036) for the identical
// input — guarantees the browser signs/verifies byte-identical messages.
const sample: Feedback = {
  v: "akash-reputation/v1",
  role: "deployer",
  provider: "akash1prov",
  deployer: "akash1dep",
  dseq: "12345",
  gseq: "1",
  oseq: "2",
  score: -1,
  comment: 'great & fast <100ms> "quoted"',
  issued_at: "2026-01-02T15:04:05Z",
};

const GO_CANONICAL =
  '{"comment":"great \\u0026 fast \\u003c100ms\\u003e \\"quoted\\"","deployer":"akash1dep","dseq":"12345","gseq":"1","issued_at":"2026-01-02T15:04:05Z","oseq":"2","provider":"akash1prov","role":"deployer","score":-1,"v":"akash-reputation/v1"}';

const GO_SIGNDOC =
  '{"account_number":"0","chain_id":"","fee":{"amount":[],"gas":"0"},"memo":"","msgs":[{"type":"sign/MsgSignData","value":{"data":"eyJjb21tZW50IjoiZ3JlYXQgXHUwMDI2IGZhc3QgXHUwMDNjMTAwbXNcdTAwM2UgXCJxdW90ZWRcIiIsImRlcGxveWVyIjoiYWthc2gxZGVwIiwiZHNlcSI6IjEyMzQ1IiwiZ3NlcSI6IjEiLCJpc3N1ZWRfYXQiOiIyMDI2LTAxLTAyVDE1OjA0OjA1WiIsIm9zZXEiOiIyIiwicHJvdmlkZXIiOiJha2FzaDFwcm92Iiwicm9sZSI6ImRlcGxveWVyIiwic2NvcmUiOi0xLCJ2IjoiYWthc2gtcmVwdXRhdGlvbi92MSJ9","signer":"akash1dep"}}],"sequence":"0"}';

describe("adr036 canonicalization", () => {
  it("matches the Go canonical bytes (incl. HTML escaping of & < > and quotes)", () => {
    expect(canonicalFeedbackBytes(sample)).toBe(GO_CANONICAL);
  });

  it("matches the Go ADR-036 sign doc", () => {
    expect(buildSignDoc(canonicalFeedbackBytes(sample), sample.deployer)).toBe(GO_SIGNDOC);
  });

  it("keeps keys sorted regardless of input field order", () => {
    const reordered: Feedback = {
      issued_at: "2026-01-02T15:04:05Z",
      v: "akash-reputation/v1",
      score: 1,
      comment: "ok",
      oseq: "1",
      gseq: "1",
      dseq: "9",
      deployer: "akash1d",
      provider: "akash1p",
      role: "provider",
    };
    expect(canonicalFeedbackBytes(reordered)).toBe(
      '{"comment":"ok","deployer":"akash1d","dseq":"9","gseq":"1","issued_at":"2026-01-02T15:04:05Z","oseq":"1","provider":"akash1p","role":"provider","score":1,"v":"akash-reputation/v1"}',
    );
  });
});
