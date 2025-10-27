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
    LOG_IN(state) {
        state.auth.isLoggedIn = true;
    },
    LOG_OUT(state) {
        state.auth.isLoggedIn = false;
        if (state.ws.obj.close) {
            state.ws.obj.close();
        }
    },
    SET_CURRENT_PAGE(state, value) {
        // value can be a string or object with keys 'title' and 'icon'
        if (typeof value === 'object' && value !== null) {
            state.currentPage = value.title;
            document.title = value.icon + " " + value.title + " - wakeci";
        } else {
            state.currentPage = value;
            document.title = value + " - wakeci";
        }
    },
    SET_THEME(state, value) {
        state.theme = value;
        window.localStorage.setItem("theme", value);
    },
};

export default mutations;
