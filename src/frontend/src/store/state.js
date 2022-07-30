const state = {
    ws: {
        obj: { sendMessage: function () {} },
        connected: false,
        reconnectTimeout: 1000, // ms
        buffer: [],
        failedAttempts: 0,
        maxFailedAttempts: 10,
    },
    auth: {
        isLoggedIn: false,
    },
    durationMode: "duration", // See Duration component for details
    selectedParams: {},
};

export default state;
