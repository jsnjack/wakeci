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
    notifyOnBuildStatusUpdate: [],
    // Whether queued builds start, see QueueStatusData in the backend
    queue: {
        stopping: false,
        abortAt: null,
        restartVersion: "",
    },
    // How many queue statuses were applied, so an older one can be told apart
    queueStatusCount: 0,
    // The wakeci version this page was loaded from (App.vue), empty until
    // the page asked
    serverVersion: "",
    currentPage: "",
    theme: "light",
};

if (window.localStorage.getItem("theme")) {
    state.theme = window.localStorage.getItem("theme");
}

export default state;
