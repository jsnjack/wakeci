import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";

import path from 'path';

// chunkSizeLimit fails the build when a JS chunk is over Vite's
// build.chunkSizeWarningLimit (kB of 1000 bytes, minified), which Vite only
// warns about, so a heavy import can't creep back into the first page unnoticed
function chunkSizeLimit() {
    let limit;
    return {
        name: "wakeci-chunk-size-limit",
        apply: "build",
        configResolved(config) {
            limit = config.build.chunkSizeWarningLimit;
        },
        generateBundle(_options, bundle) {
            const big = Object.values(bundle).filter((file) => file.type === "chunk" && file.code.length / 1000 > limit);
            if (big.length > 0) {
                const sizes = big.map((file) => `${file.fileName} (${Math.round(file.code.length / 1000)} kB)`).join(", ");
                this.error(`JS chunks over ${limit} kB: ${sizes}. Load them on demand, or raise build.chunkSizeWarningLimit`);
            }
        },
    };
}

// https://vitejs.dev/config/
export default defineConfig({
    plugins: [vue(), chunkSizeLimit()],
    resolve: {
        alias: {
            "@": path.resolve(import.meta.dirname, "./src"),
        },
    },
    server: {
        port: 8080,
        proxy: {
            "/api": {
                target: "http://localhost:8081/",
                changeOrigin: true,
            },
            "/auth": {
                target: "http://localhost:8081/",
                changeOrigin: true,
            },
            "/storage": {
                target: "http://localhost:8081/",
                changeOrigin: true,
            },
            "/docs": {
                target: "http://localhost:8081/",
                changeOrigin: true,
            },
            // No changeOrigin here: the backend only upgrades a WebSocket when
            // the Origin header matches the request Host (checkWebSocketOrigin),
            // so both must stay the vite server.
            "/ws": {
                target: "http://localhost:8081/",
                ws: true,
            },
        },
    },
});
