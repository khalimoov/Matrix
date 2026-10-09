import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// During development the Matrix server runs on :8080 and Vite proxies to it.
const backend = process.env.MATRIX_BACKEND ?? "http://127.0.0.1:8080";

export default defineConfig({
  plugins: [react()],
  build: { chunkSizeWarningLimit: 700 }, // hls.js is a separate lazily loaded chunk
  server: {
    proxy: {
      "/api": { target: backend, changeOrigin: false },
      "/media": { target: backend, changeOrigin: false },
    },
  },
});
