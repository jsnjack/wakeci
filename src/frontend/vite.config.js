import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";

import path from 'path';

// https://vitejs.dev/config/
export default defineConfig({
    plugins: [vue()],
    resolve: {
        alias: {
            "@": path.resolve(__dirname, "./src"),
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
            "/preview": {
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
