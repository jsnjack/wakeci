const mutations = {
    WS_CONNECTED(state, connection) {
        state.ws.obj = connection;
        state.ws.connected = true;
    },
    WS_DISCONNECTED(state) {
        state.ws.connected = false;
    },
    WS_MSG_JOBS_LIST(state, data) {
        state.jobs = data;
    },
};

export default mutations;
