// https://eslint.org/docs/latest/use/configure/configuration-files
import js from "@eslint/js";
import pluginVue from "eslint-plugin-vue";
import prettier from "eslint-config-prettier/flat";
import globals from "globals";

export default [
    { ignores: ["dist/", "playwright-report/", "test-results/"] },
    js.configs.recommended,
    ...pluginVue.configs["flat/recommended"],
    {
        languageOptions: { globals: globals.browser },
    },
    {
        // Build configs and the Playwright tests run in Node
        files: ["*.js", "e2e/**/*.js"],
        languageOptions: { globals: globals.node },
    },
    // Prettier formats the code (`npm run format`), so this turns off the
    // rules that would fight it
    prettier,
];
