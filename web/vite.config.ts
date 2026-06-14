import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// During `npm run dev`, proxy the API paths to a live instance so the UI works
// without running the backend locally. Override with REPUTATION_API.
const apiTarget = process.env.REPUTATION_API ?? "https://reputation.europlots.com";
const apiPaths = ["/reputation", "/feedback", "/leases", "/info", "/healthz"];

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: Object.fromEntries(
      apiPaths.map((p) => [p, { target: apiTarget, changeOrigin: true, secure: true }]),
    ),
  },
  test: {
    environment: "node",
  },
});
