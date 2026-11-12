const state = {
    ws: {
        obj: { sendMessage: function () {} },
        connected: false,
        reconnectTimeout: 1000, // ms
        buffer: [],
        failedAttempts: 0,
        maxFailedAttempts: 10,
        // Counts the connections after the first. The server closes a
        // connection when the user's permissions change, so pages watch it
        // to load their data again, also what was sent while disconnected.
        reconnects: 0,
    },
    auth: {
        isLoggedIn: false,
        // The single sign-on email, empty for the admin password
        user: "",
        // The admin password may do everything
        admin: false,
        // What the user's single sign-on groups allow, see Grant in the
        // backend. The API checks every request; the pages only hide what
        // the user can't use.
        grants: [],
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
