import { useEffect, useState } from "react";
import { getReputation, listFeedback } from "../api";
import { verifyRecord } from "../adr036";
import type { FeedbackRecord, Reputation } from "../types";

function VerifyBadge({ state }: { state: boolean | undefined }) {
  if (state === undefined) return <span className="badge">verifying…</span>;
  return state ? (
    <span className="badge ok" title="signature re-verified in your browser">
      ✓ verified
    </span>
  ) : (
    <span className="badge bad">✗ invalid signature</span>
  );
}

export function ReputationView({ address }: { address: string }) {
  const [rep, setRep] = useState<Reputation | null>(null);
  const [recs, setRecs] = useState<FeedbackRecord[] | null>(null);
  const [verified, setVerified] = useState<Record<number, boolean>>({});
  const [err, setErr] = useState("");

  useEffect(() => {
    let cancelled = false;
    setErr("");
    setRep(null);
    setRecs(null);
    setVerified({});
    Promise.all([getReputation(address), listFeedback({ subject: address })])
      .then(async ([r, f]) => {
        if (cancelled) return;
        setRep(r);
        setRecs(f);
        const entries = await Promise.all(
          f.map(async (rec) => [rec.id, await verifyRecord(rec)] as const),
        );
        if (!cancelled) setVerified(Object.fromEntries(entries));
      })
      .catch((e) => !cancelled && setErr((e as Error).message));
    return () => {
      cancelled = true;
    };
  }, [address]);

  if (err) return <p className="error">{err}</p>;
  if (!rep) return <p className="muted">Loading…</p>;

  return (
    <>
      <div className="panel">
        <div className="muted mono">{address}</div>
        <div className="row" style={{ marginTop: "0.75rem", gap: "2.5rem" }}>
          <div>
            <div className={`score-big ${rep.net >= 0 ? "pos" : "neg"}`}>
              {rep.net > 0 ? "+" : ""}
              {rep.net}
            </div>
            <div className="muted">net score</div>
          </div>
          <div>
            <div className="score-big pos">{rep.positive}</div>
            <div className="muted">positive</div>
          </div>
          <div>
            <div className="score-big neg">{rep.negative}</div>
            <div className="muted">negative</div>
          </div>
          <div>
            <div className="score-big">{rep.deployments_rated}</div>
            <div className="muted">deployments</div>
          </div>
        </div>
      </div>

      <div className="panel">
        <h3 style={{ marginTop: 0 }}>Ratings</h3>
        {recs && recs.length === 0 && <p className="muted">No ratings yet.</p>}
        {recs?.map((rec) => (
          <div className="rating" key={rec.id}>
            <div className="row" style={{ justifyContent: "space-between" }}>
              <span className={rec.score > 0 ? "pos" : "neg"}>{rec.score > 0 ? "▲ +1" : "▼ −1"}</span>
              <VerifyBadge state={verified[rec.id]} />
            </div>
            {rec.comment && <div style={{ margin: "0.25rem 0" }}>{rec.comment}</div>}
            <div className="muted" style={{ fontSize: "0.8rem" }}>
              by <span className="mono">{rec.author}</span> · as {rec.role} · dseq {rec.dseq} ·{" "}
              {new Date(rec.issued_at).toLocaleDateString()}
            </div>
          </div>
        ))}
      </div>
    </>
  );
}
