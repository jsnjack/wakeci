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
    WS_MSG_FEED_UPDATE(state, data) {
        state.feed = [
            ...state.feed.filter((el) => el.id !== data.id),
            data,
        ];
    },
    WS_MSG_JOB_UPDATE(state, data) {
        state.jobs = [
            ...state.jobs.filter((el) => el.name !== data.name),
            data,
        ];
    },
};

export default mutations;
