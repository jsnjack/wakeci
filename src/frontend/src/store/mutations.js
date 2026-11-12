const mutations = {
    SET_SERVER_VERSION(state, version) {
        state.serverVersion = version;
    },
    WS_CONNECTED(state, connection) {
        state.ws.obj = connection;
        state.ws.connected = true;
        state.ws.failedAttempts = 0;
        while (state.ws.buffer.length > 0) {
            state.ws.obj.sendMessage(state.ws.buffer.shift());
        }
    },
    WS_RECONNECTED(state) {
        state.ws.reconnects += 1;
    },
    WS_DISCONNECTED(state) {
        state.ws.failedAttempts += 1;
        state.ws.connected = false;
    },
    WS_SEND(state, msg) {
        if (state.ws.connected === true) {
            state.ws.obj.sendMessage(msg);
        } else {
            state.ws.buffer.push(msg);
        }
    },
    // status is the answer of /auth/_isLoggedIn
    LOG_IN(state, status) {
        state.ws.failedAttempts = 0;
        state.auth.isLoggedIn = true;
        state.auth.user = status?.user || "";
        state.auth.admin = status?.admin === true;
        state.auth.grants = status?.grants || [];
    },
    LOG_OUT(state) {
        state.ws.failedAttempts = 0;
        state.auth.isLoggedIn = false;
        state.auth.user = "";
        state.auth.admin = false;
        state.auth.grants = [];
        if (state.ws.obj.close) {
            state.ws.obj.close();
        }
    },
    SET_CURRENT_PAGE(state, value) {
        // value can be a string or object with keys 'title' and 'icon'
        if (typeof value === "object" && value !== null) {
            state.currentPage = value.title;
            document.title = value.icon + " " + value.title + " - wakeci";
        } else {
            state.currentPage = value;
            document.title = value + " - wakeci";
        }
    },
    SET_QUEUE_STATUS(state, status) {
        state.queue = {
            stopping: status.stopping,
            abortAt: status.abortAt || null,
            restartVersion: status.restartVersion || "",
        };
        state.queueStatusCount++;
    },
    SET_THEME(state, value) {
        state.theme = value;
        window.localStorage.setItem("theme", value);
    },
    SUBSCRIBE_NOTIFICATION(state, buildId) {
        if (!state.notifyOnBuildStatusUpdate.includes(buildId)) {
            state.notifyOnBuildStatusUpdate.push(buildId);
        }
    },
    UNSUBSCRIBE_NOTIFICATION(state, buildId) {
        state.notifyOnBuildStatusUpdate = state.notifyOnBuildStatusUpdate.filter((id) => id !== buildId);
    },
};

export default mutations;
