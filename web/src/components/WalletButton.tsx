import { useState } from "react";
import { availableWallets, connect, type Connected, type WalletKind } from "../wallet";

function short(a: string) {
  return a.length > 16 ? `${a.slice(0, 11)}…${a.slice(-4)}` : a;
}

export function WalletButton({
  chainId,
  conn,
  onConnect,
  onDisconnect,
}: {
  chainId: string;
  conn: Connected | null;
  onConnect: (c: Connected) => void;
  onDisconnect: () => void;
}) {
  const [err, setErr] = useState("");
  const wallets = availableWallets();

  if (conn) {
    return (
      <div className="row">
        <span className="pill mono" title={conn.address}>
          {conn.kind}: {short(conn.address)}
        </span>
        <button className="secondary" onClick={onDisconnect}>
          Disconnect
        </button>
      </div>
    );
  }

  if (wallets.length === 0) {
    return <span className="muted">Install Keplr or Leap to connect</span>;
  }

  async function go(kind: WalletKind) {
    setErr("");
    try {
      onConnect(await connect(kind, chainId));
    } catch (e) {
      setErr((e as Error).message);
    }
  }

  return (
    <div className="row">
      {wallets.map((k) => (
        <button key={k} onClick={() => go(k)}>
          Connect {k[0].toUpperCase() + k.slice(1)}
        </button>
      ))}
      {err && <span className="error">{err}</span>}
    </div>
  );
}
