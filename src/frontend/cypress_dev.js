import { defineConfig } from "cypress";

export default defineConfig({
    video: false,
    e2e: {
        baseUrl: "http://localhost:8080",
        supportFile: "cypress/support/index.js",
        specPattern: "cypress/integration/*.js",
    },
});
