import { saveWatches } from "./watches.js";

const mutations = {
    SET_SERVER_VERSION(state, version) {
        state.serverVersion = version;
    },
    WS_CONNECTED(state, connection) {
        state.ws.obj = connection;
        state.ws.connected = true;
        state.ws.failedAttempts = 0;
        // A new connection has no subscriptions
        const topics = Object.keys(state.ws.topics).filter((topic) => state.ws.topics[topic] > 0);
        if (topics.length) {
            state.ws.obj.sendMessage({ type: "in:subscribe", data: { to: topics } });
        }
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
            buildsAborted: status.buildsAborted === true,
            queuedIDs: status.queuedIDs || [],
        };
        state.queueStatusCount++;
    },
    // SET_THEME sets the theme; picked, the user's choice, is kept, while a
    // theme that follows the system is not
    SET_THEME(state, { theme, picked = true }) {
        state.theme = theme;
        if (picked) {
            window.localStorage.setItem("theme", theme);
        }
    },
    // SUBSCRIBE_TOPIC and UNSUBSCRIBE_TOPIC count the parts of the app that
    // use a topic, e.g. the feed and the notifications both need every
    // build's updates, so one that stops doesn't stop the other's.
    // WS_CONNECTED subscribes again to those in use
    SUBSCRIBE_TOPIC(state, topic) {
        const count = state.ws.topics[topic] || 0;
        state.ws.topics[topic] = count + 1;
        if (count === 0 && state.ws.connected) {
            state.ws.obj.sendMessage({ type: "in:subscribe", data: { to: [topic] } });
        }
    },
    UNSUBSCRIBE_TOPIC(state, topic) {
        const count = state.ws.topics[topic] || 0;
        state.ws.topics[topic] = Math.max(0, count - 1);
        if (count === 1 && state.ws.connected) {
            state.ws.obj.sendMessage({ type: "in:unsubscribe", data: { to: [topic] } });
        }
    },
    WATCH_BUILD(state, id) {
        if (!state.watches.builds.includes(id)) {
            // One that ended while the app was closed never says so: the
            // oldest go
            state.watches.builds = [...state.watches.builds, id].slice(-50);
            saveWatches(state.watches);
        }
    },
    UNWATCH_BUILD(state, id) {
        state.watches.builds = state.watches.builds.filter((build) => build !== id);
        saveWatches(state.watches);
    },
    WATCH_SEARCH(state, query) {
        if (query && !state.watches.searches.includes(query)) {
            state.watches.searches = [...state.watches.searches, query];
            saveWatches(state.watches);
        }
    },
    UNWATCH_SEARCH(state, query) {
        state.watches.searches = state.watches.searches.filter((search) => search !== query);
        saveWatches(state.watches);
    },
};

export default mutations;
