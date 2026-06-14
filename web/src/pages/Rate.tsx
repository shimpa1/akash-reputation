import { useEffect, useState } from "react";
import { listLeases } from "../api";
import { RateForm } from "../components/RateForm";
import type { Connected } from "../wallet";
import type { Lease } from "../types";

export function Rate({ conn, provider }: { conn: Connected; provider: string }) {
  const [leases, setLeases] = useState<Lease[] | null>(null);
  const [err, setErr] = useState("");

  useEffect(() => {
    setErr("");
    setLeases(null);
    listLeases({ owner: conn.address })
      .then(setLeases)
      .catch((e) => setErr((e as Error).message));
  }, [conn.address]);

  return (
    <div className="panel">
      <h3 style={{ marginTop: 0 }}>Rate this provider</h3>
      <p className="muted">
        Rating <span className="mono">{provider}</span> as deployer{" "}
        <span className="mono">{conn.address}</span>.
      </p>
      {err && <p className="error">{err}</p>}
      {!leases ? (
        <p className="muted">Loading your leases with this provider…</p>
      ) : (
        <RateForm
          conn={conn}
          role="deployer"
          provider={provider}
          deployer={conn.address}
          leases={leases}
        />
      )}
    </div>
  );
}
