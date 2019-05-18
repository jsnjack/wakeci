const state = {
    ws: {
        obj: {sendMessage: function() {}},
        url: process.env.WS_ENDPOINT || "ws://localhost:8081/ws",
        connected: false,
        reconnectTimeout: 1000, // ms
    },
    jobs: [],
    feed: [],
    api: {
        baseURL: process.env.API_ENDPOINT || "http://localhost:8080/api",
    },
};

export default state;
