export const APIURL = "/api";
export const AUTHURL = "/auth";

const wsMessageHandler = function(app, data) {
    const msg = JSON.parse(data);
    console.info("WS msg", msg);
    if (msg.type.startsWith("build:log:")) {
        app.$eventHub.$emit(msg.type, msg.data);
        return;
    } else if (msg.type.startsWith("build:update:")) {
        // For build view
        app.$eventHub.$emit(msg.type, msg.data);
        // For feed view
        app.$eventHub.$emit("build:update:", msg.data);
        return;
    }
    console.warn("Unhandled message", msg);
};

export const getWSURL = function() {
    let protocol; let hostname;
    if (location.protocol === "https:") {
        protocol = "wss://";
    } else {
        protocol = "ws://";
    }

    if (process.env.NODE_ENV === "production") {
        hostname = location.host;
    } else {
        hostname = "localhost:8081";
    }
    return `${protocol}${hostname}/ws`;
};

export default wsMessageHandler;
