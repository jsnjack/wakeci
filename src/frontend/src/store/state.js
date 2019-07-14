const state = {
    ws: {
        obj: {sendMessage: function() {}},
        connected: false,
        reconnectTimeout: 1000, // ms
        buffer: [],
    },
    auth: {
        isLoggedIn: false,
    },
};

export default state;
