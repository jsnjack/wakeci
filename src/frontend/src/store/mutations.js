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
    WS_MSG_BUILD_UPDATE(state, data) {
        const r = findInContainer(state.builds, "id", data.id);
        if (r[0]) {
            Vue.set(state.builds, r[1], Object.assign({}, state.builds[r[1]], data));
        } else {
            state.builds.unshift(data);
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
    WS_MSG_BUILD_LOG(state, msg) {
        if (state.activeSubscription === msg.type) {
            state.logs.push(msg.data);
        } else {
            console.log("Ignore", msg);
        }
    },

    ACTIVE_SUBSCRIPTION(state, name) {
        state.activeSubscription = name;
        if (name === "") {
            state.logs = [];
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
