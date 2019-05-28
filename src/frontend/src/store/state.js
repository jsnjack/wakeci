const state = {
    ws: {
        obj: {sendMessage: function() {}},
        url: process.env.WS_ENDPOINT || "ws://localhost:8081/ws",
        connected: false,
        reconnectTimeout: 1000, // ms
    },
    jobs: [], // All registered jobs
    builds: [], // Recent builds on feed view
    api: {
        baseURL: process.env.API_ENDPOINT || "http://localhost:8080/api",
    },
    activeSubscription: "",
    logs: [],
};

export default state;
