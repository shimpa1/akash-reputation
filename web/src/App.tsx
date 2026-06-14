import { useEffect, useState } from "react";
import { getInfo } from "./api";
import { WalletButton } from "./components/WalletButton";
import { Lookup } from "./pages/Lookup";
import { Rate } from "./pages/Rate";
import { ProviderDashboard } from "./pages/ProviderDashboard";
import type { Connected } from "./wallet";
import type { Info } from "./types";

type View = "lookup" | "rate";

export function App() {
  const [info, setInfo] = useState<Info | null>(null);
  const [infoErr, setInfoErr] = useState("");
  const [conn, setConn] = useState<Connected | null>(null);
  const [view, setView] = useState<View>("lookup");

  useEffect(() => {
    getInfo()
      .then(setInfo)
      .catch((e) => setInfoErr((e as Error).message));
  }, []);

  const isProvider = !!(conn && info && conn.address === info.provider);

  return (
    <>
      <header className="app">
        <h1>Akash Reputation</h1>
        <nav>
          <button
            className={`tab ${view === "lookup" ? "active" : ""}`}
            onClick={() => setView("lookup")}
          >
            Look up
          </button>
          <button
            className={`tab ${view === "rate" ? "active" : ""}`}
            onClick={() => setView("rate")}
            disabled={!conn}
            title={conn ? "" : "Connect a wallet to leave a rating"}
          >
            {isProvider ? "Provider" : "Rate"}
          </button>
        </nav>
        {info && (
          <WalletButton
            chainId={info.chain_id}
            conn={conn}
            onConnect={(c) => {
              setConn(c);
              setView("rate");
            }}
            onDisconnect={() => {
              setConn(null);
              setView("lookup");
            }}
          />
        )}
      </header>
      <main>
        {infoErr && <p className="error">Service unavailable: {infoErr}</p>}
        {view === "lookup" && <Lookup />}
        {view === "rate" && conn && info ? (
          isProvider ? (
            <ProviderDashboard conn={conn} />
          ) : (
            <Rate conn={conn} provider={info.provider} />
          )
        ) : null}
        {view === "rate" && !conn && <p className="muted">Connect a wallet to leave a rating.</p>}
      </main>
    </>
  );
}
