import Vue from "vue";

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
        const r = findInContainer(state.feed, "id", data.id);
        if (r[0]) {
            Vue.set(state.feed, r[1], Object.assign({}, state.feed[r[1]], data));
        } else {
            state.feed.unshift(data);
        }
    },
    WS_MSG_JOB_UPDATE(state, data) {
        const r = findInContainer(state.jobs, "name", data.name);
        if (r[0]) {
            Vue.set(state.jobs, r[1], Object.assign({}, state.jobs[r[1]], data));
        } else {
            state.jobs.push(data);
        }
    },
};

function findInContainer(container, key, value) {
    for (let i=0; i<container.length; i++) {
        if (container[i][key] === value) {
            return [container[i], i];
        }
    }
    return [undefined, undefined];
}

export default mutations;
