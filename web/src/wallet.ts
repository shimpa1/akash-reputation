// Browser-wallet adapter for Keplr and Leap. Both inject the same API; we only
// use enable(), getKey() and signArbitrary() — the latter produces an ADR-036
// signature the server verifies. The private key never leaves the wallet.
import { canonicalFeedbackBytes } from "./adr036";
import type { Feedback } from "./types";

interface AminoSignResponse {
  signature: string; // base64, 64-byte R||S
  pub_key: { type: string; value: string }; // value: base64 compressed secp256k1
}

interface CosmosWallet {
  enable(chainId: string): Promise<void>;
  getKey(chainId: string): Promise<{ bech32Address: string }>;
  signArbitrary(chainId: string, signer: string, data: string | Uint8Array): Promise<AminoSignResponse>;
}

export type WalletKind = "keplr" | "leap";

function getProvider(kind: WalletKind): CosmosWallet | undefined {
  return (window as unknown as Record<string, CosmosWallet | undefined>)[kind];
}

export function availableWallets(): WalletKind[] {
  return (["keplr", "leap"] as WalletKind[]).filter((k) => getProvider(k));
}

export interface Connected {
  kind: WalletKind;
  address: string;
  chainId: string;
}

export async function connect(kind: WalletKind, chainId: string): Promise<Connected> {
  const w = getProvider(kind);
  if (!w) throw new Error(`${kind} wallet not detected — install the extension`);
  await w.enable(chainId);
  const key = await w.getKey(chainId);
  return { kind, address: key.bech32Address, chainId };
}

export interface SignedFeedback {
  feedback: Feedback;
  pubkey: string;
  signature: string;
}

// signFeedback signs the canonical message with the connected wallet and returns
// the submission envelope. The wallet shows the message and asks the user to approve.
export async function signFeedback(conn: Connected, f: Feedback): Promise<SignedFeedback> {
  const w = getProvider(conn.kind);
  if (!w) throw new Error(`${conn.kind} wallet not detected`);
  const res = await w.signArbitrary(conn.chainId, conn.address, canonicalFeedbackBytes(f));
  return { feedback: f, pubkey: res.pub_key.value, signature: res.signature };
}
