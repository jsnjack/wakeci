module.exports = {
    devServer: {
        proxy: {
            "^/api": {
                target: "http://localhost:8081/",
                changeOrigin: true,
            },
            "^/auth": {
                target: "http://localhost:8081/",
                changeOrigin: true,
            },
            "^/storage": {
                target: "http://localhost:8081/",
                changeOrigin: true,
            },
        },
    },
    chainWebpack: config => {
        config.resolve.alias.set('vue', '@vue/compat');
        config.module
        .rule('vue')
        .use('vue-loader')
        .tap(options => {
            return {
                ...options,
                compilerOptions: {
                    compatConfig: {
                        MODE: 2
                    },
                },
            };
        });
    },
};

