import { createReadStream, statSync } from "node:fs";
import { relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

const vodRoot = fileURLToPath(new URL("../backend/data/samples", import.meta.url));

export default defineConfig({
  plugins: [
    react(),
    {
      name: "local-vod-files",
      configureServer(server) {
        server.middlewares.use("/vod", (request, response, next) => {
          const requestPath = decodeURIComponent((request.url ?? "").split("?")[0]);
          const filePath = resolve(vodRoot, `.${requestPath}`);
          if (relative(vodRoot, filePath).startsWith("..")) return next();
          try {
            if (!statSync(filePath).isFile()) return next();
            response.setHeader("Cache-Control", "no-cache");
            response.setHeader("Content-Type", filePath.endsWith(".flv") ? "video/x-flv" : "audio/wav");
            createReadStream(filePath).pipe(response);
          } catch {
            next();
          }
        });
      },
    },
  ],
  server: {
    port: 5173,
    proxy: {
      "/api": {
        target: "http://127.0.0.1:8080",
        changeOrigin: true,
      },
    },
  },
});
