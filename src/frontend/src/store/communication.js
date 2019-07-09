export const APIURL = process.env.API_ENDPOINT || "http://localhost:8080/api";
export const AUTHURL = process.env.AUTH_ENDPOINT || "http://localhost:8080/auth";

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

export default wsMessageHandler;
