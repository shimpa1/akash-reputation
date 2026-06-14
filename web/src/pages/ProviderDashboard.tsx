import { useState, type FormEvent } from "react";
import { listLeases } from "../api";
import { RateForm } from "../components/RateForm";
import type { Connected } from "../wallet";
import type { Lease } from "../types";

export function ProviderDashboard({ conn }: { conn: Connected }) {
  const [input, setInput] = useState("");
  const [deployer, setDeployer] = useState("");
  const [leases, setLeases] = useState<Lease[] | null>(null);
  const [err, setErr] = useState("");

  async function search(e: FormEvent) {
    e.preventDefault();
    const addr = input.trim();
    setErr("");
    setLeases(null);
    setDeployer(addr);
    try {
      setLeases(await listLeases({ provider: conn.address, owner: addr }));
    } catch (ex) {
      setErr((ex as Error).message);
    }
  }

  return (
    <div className="panel">
      <h3 style={{ marginTop: 0 }}>Provider dashboard</h3>
      <p className="muted">
        You’re connected as the provider <span className="mono">{conn.address}</span>. Enter a
        deployer address to see your leases with them and leave a rating.
      </p>
      <form onSubmit={search}>
        <div className="row">
          <input
            placeholder="deployer akash1…"
            value={input}
            onChange={(e) => setInput(e.target.value)}
            style={{ flex: 1, minWidth: "240px" }}
          />
          <button type="submit">Find leases</button>
        </div>
      </form>
      {err && <p className="error">{err}</p>}
      {deployer && leases && (
        <div style={{ marginTop: "1rem" }}>
          <RateForm
            conn={conn}
            role="provider"
            provider={conn.address}
            deployer={deployer}
            leases={leases}
          />
        </div>
      )}
    </div>
  );
}
