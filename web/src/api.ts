// Thin client over the reputation API. Same-origin: the UI is served from the
// same host as the API (path-routed), so all paths are relative.
import type { Feedback, FeedbackRecord, Info, Lease, Reputation } from "./types";

async function getJSON<T>(path: string): Promise<T> {
  const r = await fetch(path);
  if (!r.ok) throw new Error(`${path}: HTTP ${r.status}`);
  return (await r.json()) as T;
}

export const getInfo = () => getJSON<Info>("/info");

export const getReputation = (address: string) =>
  getJSON<Reputation>(`/reputation/${encodeURIComponent(address)}`);

export async function listFeedback(params: {
  subject?: string;
  author?: string;
  role?: string;
}): Promise<FeedbackRecord[]> {
  const q = new URLSearchParams();
  if (params.subject) q.set("subject", params.subject);
  if (params.author) q.set("author", params.author);
  if (params.role) q.set("role", params.role);
  const r = await getJSON<{ feedback: FeedbackRecord[] | null }>(`/feedback?${q.toString()}`);
  return r.feedback ?? [];
}

export async function listLeases(params: { owner?: string; provider?: string }): Promise<Lease[]> {
  const q = new URLSearchParams();
  if (params.owner) q.set("owner", params.owner);
  if (params.provider) q.set("provider", params.provider);
  const r = await getJSON<{ leases: Lease[] | null }>(`/leases?${q.toString()}`);
  return r.leases ?? [];
}

export interface PostResult {
  id: number;
  author: string;
  subject: string;
  score: number;
}

export async function postFeedback(body: {
  feedback: Feedback;
  pubkey: string;
  signature: string;
}): Promise<PostResult> {
  const r = await fetch("/feedback", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  const text = await r.text();
  if (!r.ok) {
    let msg = text;
    try {
      msg = (JSON.parse(text) as { error?: string }).error ?? text;
    } catch {
      /* non-JSON error body */
    }
    throw new Error(msg || `HTTP ${r.status}`);
  }
  return JSON.parse(text) as PostResult;
}
