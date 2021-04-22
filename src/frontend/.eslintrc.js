// https://eslint.org/docs/user-guide/configuring

module.exports = {
    root: true,
    parser: "vue-eslint-parser",
    parserOptions: {
        parser: "babel-eslint",
    },
    env: {
        browser: true,
    },
    extends: [
    // https://github.com/vuejs/eslint-plugin-vue#priority-a-essential-error-prevention
    // consider switching to `plugin:vue/strongly-recommended` or `plugin:vue/recommended` for stricter rules.
        "plugin:vue/recommended",
        "google",
    ],
    // required to lint *.vue files
    plugins: [
        "vue",
    ],
    // add your custom rules here
    rules: {
    // allow async-await
        "generator-star-spacing": "off",
        "quotes": ["error", "double"],
        "max-len": ["error", 140],
        // allow debugger during development
        "no-debugger": process.env.NODE_ENV === "production" ? "error" : "off",
        "require-jsdoc": "off",
        "indent": ["error", 4],
        "vue/html-self-closing": "off",
    },
};
