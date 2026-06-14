import { useState } from "react";
import { postFeedback } from "../api";
import { signFeedback, type Connected } from "../wallet";
import { MESSAGE_VERSION, type Feedback, type Lease, type Role } from "../types";

// RFC3339 without fractional seconds (the server parses with time.RFC3339,
// which rejects milliseconds).
function nowRFC3339(): string {
  return new Date().toISOString().replace(/\.\d+Z$/, "Z");
}

export function RateForm({
  conn,
  role,
  provider,
  deployer,
  leases,
}: {
  conn: Connected;
  role: Role;
  provider: string;
  deployer: string;
  leases: Lease[];
}) {
  const [idx, setIdx] = useState(0);
  const [score, setScore] = useState<1 | -1>(1);
  const [comment, setComment] = useState("");
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState("");
  const [err, setErr] = useState("");

  if (leases.length === 0) return <p className="muted">No leases found to rate.</p>;

  async function submit() {
    setBusy(true);
    setErr("");
    setResult("");
    try {
      const l = leases[idx];
      const f: Feedback = {
        v: MESSAGE_VERSION,
        role,
        provider,
        deployer,
        dseq: l.DSeq,
        gseq: l.GSeq,
        oseq: l.OSeq,
        score,
        comment,
        issued_at: nowRFC3339(),
      };
      const signed = await signFeedback(conn, f);
      const res = await postFeedback(signed);
      setResult(`Rating #${res.id} recorded (${res.score > 0 ? "+1" : "−1"}).`);
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div>
      <div className="field">
        <label>Deployment</label>
        <select value={idx} onChange={(e) => setIdx(Number(e.target.value))}>
          {leases.map((l, i) => (
            <option key={`${l.DSeq}/${l.GSeq}/${l.OSeq}`} value={i}>
              dseq {l.DSeq} (g{l.GSeq}/o{l.OSeq}) · {l.State}
            </option>
          ))}
        </select>
      </div>
      <div className="field">
        <label>Rating</label>
        <div className="row">
          <button className={score === 1 ? "" : "secondary"} onClick={() => setScore(1)}>
            ▲ Positive
          </button>
          <button className={score === -1 ? "" : "secondary"} onClick={() => setScore(-1)}>
            ▼ Negative
          </button>
        </div>
      </div>
      <div className="field">
        <label>Comment (optional, max 1000)</label>
        <textarea value={comment} maxLength={1000} rows={2} onChange={(e) => setComment(e.target.value)} />
      </div>
      <button disabled={busy} onClick={submit}>
        {busy ? "Waiting for wallet…" : "Sign & submit rating"}
      </button>
      {result && <p className="success">{result}</p>}
      {err && <p className="error">{err}</p>}
    </div>
  );
}
