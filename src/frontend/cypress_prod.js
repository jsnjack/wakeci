const { defineConfig } = require("cypress");

module.exports = defineConfig({
    video: false,
    e2e: {
        baseUrl: "http://localhost:8081",
        supportFile: "cypress/support/index.js",
        specPattern: "cypress/integration/*.js",
    },
});
