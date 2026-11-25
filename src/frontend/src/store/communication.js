import { createFilterObj, escapeHtml, matchesFilter } from "./utils.js";

const lastStatuses = new Map();

// The statuses a build ends with
const FINAL = ["finished", "failed", "aborted", "timed out"];

// handleSystemNotification tells the user once a build they watch, or one
// that matches a search they watch, has ended (L5 in docs/ui-roadmap.md),
// whatever page is open: a system notification if the browser allows them,
// otherwise a message on the page
const handleSystemNotification = function (app, data) {
    const last = lastStatuses.get(data.id);
    // Only builds that haven't ended are kept, so a page open for days
    // doesn't keep every build; one that ended tells at most once, since
    // its later updates find no status before them
    if (!FINAL.includes(data.status)) {
        lastStatuses.set(data.id, data.status);
        return;
    }
    lastStatuses.delete(data.id);
    const watches = app.$store.state.watches;
    const watched = watches.builds.includes(data.id);
    // A search tells only of builds this page saw before they ended: one
    // that ended before it was opened still sends updates while its hooks
    // run, and would tell again on every reload. A bell is the user's own,
    // and goes once it told
    const search = last === undefined ? undefined : watches.searches.find((query) => matchesFilter(data, createFilterObj(query)));
    if (!watched && search === undefined) {
        return;
    }
    if (watched) {
        app.$store.commit("UNWATCH_BUILD", data.id);
    }
    const title = `${data.name} ${data.status}`;
    const body = search === undefined ? `Build #${data.id}` : `Build #${data.id}, matching "${search}"`;
    if ("Notification" in window && Notification.permission === "granted") {
        new Notification(title, { body, icon: "/favicon.ico", tag: `build-${data.id}` });
    } else {
        app.$notify({
            text: `${escapeHtml(title)}: <a href="/build/${data.id}/">#${data.id}</a>`,
            type: data.status === "finished" ? "primary" : "error",
            duration: 10000,
        });
    }
};

const wsMessageHandler = function (app, data) {
    const messages = data.split("\n");
    for (let i = 0; i < messages.length; i++) {
        const msg = JSON.parse(messages[i]);
        if (msg.type.startsWith("build:log:")) {
            app.emitter.emit(`${msg.type}:task-${msg.data.taskID}`, msg.data);
            continue;
        } else if (msg.type.startsWith("build:update:")) {
            handleSystemNotification(app, msg.data);

            // For build view
            app.emitter.emit(msg.type, msg.data);
            // For feed view
            app.emitter.emit("build:update:", msg.data);
            continue;
        } else if (msg.type === "queue:status") {
            app.$store.commit("SET_QUEUE_STATUS", msg.data);
            continue;
        }
        console.warn("Unhandled message", msg);
    }
};

export const getWSURL = function () {
    let protocol;
    if (location.protocol === "https:") {
        protocol = "wss://";
    } else {
        protocol = "ws://";
    }

    // Always connect to the page's own origin. In dev that is the vite server,
    // which proxies /ws to the backend; going straight to localhost:8081 would
    // send Origin: localhost:8080 and be rejected by checkWebSocketOrigin.
    return `${protocol}${location.host}/ws`;
};

export default wsMessageHandler;
