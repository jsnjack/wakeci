
const mutations = {
    WS_CONNECTED(state, connection) {
        state.ws.obj = connection;
        state.ws.connected = true;
        state.ws.failedAttempts = 0;
        while (state.ws.buffer.length > 0) {
            state.ws.obj.sendMessage(state.ws.buffer.shift());
        }
    },
    WS_DISCONNECTED(state) {
        state.ws.failedAttempts +=1;
        state.ws.connected = false;
    },
    WS_SEND(state, msg) {
        if (state.ws.connected === true) {
            state.ws.obj.sendMessage(msg);
        } else {
            state.ws.buffer.push(msg);
        }
    },
    LOG_IN(state) {
        state.auth.isLoggedIn = true;
    },
    LOG_OUT(state) {
        state.auth.isLoggedIn = false;
        if (state.ws.obj.close) {
            state.ws.obj.close();
        }
    },
    TOGGLE_DURATION_MODE(state) {
        const modes = ["duration", "started", "started at"];
        let idx = modes.indexOf(state.durationMode) + 1;
        if (idx > modes.length - 1) {
            idx = 0;
        }
        state.durationMode = modes[idx];
    },
};

export default mutations;
