import { useState, type FormEvent } from "react";
import { ReputationView } from "../components/ReputationView";

export function Lookup() {
  const [input, setInput] = useState("");
  const [addr, setAddr] = useState("");

  function submit(e: FormEvent) {
    e.preventDefault();
    setAddr(input.trim());
  }

  return (
    <div>
      <div className="panel">
        <form onSubmit={submit}>
          <div className="field">
            <label>Look up an Akash address (provider or deployer)</label>
            <div className="row">
              <input
                placeholder="akash1…"
                value={input}
                onChange={(e) => setInput(e.target.value)}
                style={{ flex: 1, minWidth: "240px" }}
              />
              <button type="submit">Look up</button>
            </div>
          </div>
        </form>
        <p className="muted" style={{ margin: 0, fontSize: "0.8rem" }}>
          No wallet needed to look up. Each rating shows a ✓ badge once its signature is
          re-verified in your browser.
        </p>
      </div>
      {addr && <ReputationView address={addr} />}
    </div>
  );
}
