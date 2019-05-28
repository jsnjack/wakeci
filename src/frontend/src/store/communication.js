const wsMessageHandler = function(app, data) {
    const msg = JSON.parse(data);
    console.info("WS msg", msg);
    switch (msg.type) {
    case "jobs:list":
        app.$store.commit("WS_MSG_JOBS_LIST", msg.data);
        break;
    case "job:update":
        app.$store.commit("WS_MSG_JOB_UPDATE", msg.data);
        break;
    case "build:update":
        app.$store.commit("WS_MSG_BUILD_UPDATE", msg.data);
        break;
    default:
        if (msg.type.indexOf("build:log:") === 0) {
            app.$store.commit("WS_MSG_BUILD_LOG", msg);
            return;
        }
        console.warn("Unhandled message", msg);
    }
};

export default wsMessageHandler;
