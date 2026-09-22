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
            const stat = statSync(filePath);
            if (!stat.isFile()) return next();
            const contentType = filePath.endsWith(".flv") ? "video/x-flv" : "audio/wav";
            const range = request.headers.range;
            if (range) {
              const parts = range.replace(/bytes=/, "").split("-");
              const start = parseInt(parts[0], 10);
              const end = parts[1] ? parseInt(parts[1], 10) : stat.size - 1;
              response.writeHead(206, {
                "Content-Range": `bytes ${start}-${end}/${stat.size}`,
                "Accept-Ranges": "bytes",
                "Content-Length": end - start + 1,
                "Content-Type": contentType,
                "Cache-Control": "no-cache",
              });
              createReadStream(filePath, { start, end }).pipe(response);
            } else {
              response.setHeader("Accept-Ranges", "bytes");
              response.setHeader("Content-Length", stat.size);
              response.setHeader("Cache-Control", "no-cache");
              response.setHeader("Content-Type", contentType);
              createReadStream(filePath).pipe(response);
            }
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
