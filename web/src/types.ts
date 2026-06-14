export type Role = "provider" | "deployer";

export const MESSAGE_VERSION = "akash-reputation/v1";

// Feedback mirrors the server's canonical rating message (internal/adr036).
export interface Feedback {
  v: string;
  role: Role;
  provider: string;
  deployer: string;
  dseq: string;
  gseq: string;
  oseq: string;
  score: number; // +1 or -1
  comment: string;
  issued_at: string; // RFC3339 UTC
}

export interface Reputation {
  address: string;
  positive: number;
  negative: number;
  net: number;
  deployments_rated: number;
}

export interface FeedbackRecord {
  id: number;
  role: Role;
  author: string;
  subject: string;
  provider: string;
  deployer: string;
  dseq: string;
  gseq: string;
  oseq: string;
  score: number;
  comment: string;
  issued_at: string;
  pubkey: string;
  signature: string;
  created_at: string;
}

export interface Lease {
  Owner: string;
  DSeq: string;
  GSeq: string;
  OSeq: string;
  Provider: string;
  State: string;
}

export interface Info {
  provider: string;
  chain_id: string;
}
