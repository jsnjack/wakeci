const state = {
    ws: {
        obj: {sendMessage: function() {}},
        url: process.env.WS_ENDPOINT || "ws://localhost:8081/ws",
        connected: false,
        reconnectTimeout: 1000, // ms
        buffer: [],
    },
};

export default state;
