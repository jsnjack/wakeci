const lastStatuses = new Map();

const handleSystemNotification = function (app, data) {
    if (app.$store.state.notifyOnBuildStatusUpdate.includes(data.id)) {
        const lastStatus = lastStatuses.get(data.id);
        if (lastStatus !== data.status && lastStatus !== undefined) {
            if ("Notification" in window && Notification.permission === "granted") {
                new Notification(`Build #${data.id} ${data.status}`, {
                    body: `Job: ${data.name}`,
                    icon: "/favicon.ico",
                });
            }
            lastStatuses.set(data.id, data.status);
        }
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
        }
        console.warn("Unhandled message", msg);
    }
};

export const getWSURL = function () {
    let protocol;
    let hostname;
    if (location.protocol === "https:") {
        protocol = "wss://";
    } else {
        protocol = "ws://";
    }

    if (import.meta.env.PROD) {
        hostname = location.host;
    } else {
        hostname = "localhost:8081";
    }
    return `${protocol}${hostname}/ws`;
};

export default wsMessageHandler;
