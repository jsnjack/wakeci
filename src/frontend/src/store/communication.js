export const APIURL = process.env.API_ENDPOINT || "http://localhost:8080/api";

const wsMessageHandler = function(app, data) {
    const msg = JSON.parse(data);
    console.info("WS msg", msg);
    switch (msg.type) {
    case "build:update":
        app.$eventHub.$emit(msg.type, msg.data);
        break;
    default:
        if (msg.type.startsWith("build:log:")) {
            app.$eventHub.$emit(msg.type, msg.data);
            return;
        }
        console.warn("Unhandled message", msg);
    }
};

export default wsMessageHandler;
