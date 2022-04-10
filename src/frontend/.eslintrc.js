// https://eslint.org/docs/user-guide/configuring

module.exports = {
    env: {
        node: true,
    },
    extends: [
        // https://github.com/vuejs/eslint-plugin-vue#priority-a-essential-error-prevention
        // consider switching to `plugin:vue/strongly-recommended` or `plugin:vue/recommended` for stricter rules.
        "eslint:recommended",
        "plugin:vue/recommended",
        "plugin:prettier/recommended",
    ],
    // add your custom rules here
    rules: {
        // allow async-await
        "generator-star-spacing": "off",
        quotes: ["error", "double"],
        "max-len": ["error", 140],
        "require-jsdoc": "off",
        indent: ["error", 4],
        "vue/html-self-closing": "off",
    },
};
