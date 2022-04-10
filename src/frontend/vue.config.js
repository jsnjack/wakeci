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
            "^/docs": {
                target: "http://localhost:8081/",
                changeOrigin: true,
            },
        },
    },
};
