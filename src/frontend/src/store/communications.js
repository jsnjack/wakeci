const wsMessageHandler = function(app, data) {
    const msg = JSON.parse(data);
    switch (msg.type) {
    case "ws.device:discovered":
        app.$store.commit("DEVICE_DISCOVERED", msg.data);
        break;
    case "ws.device:status":
        if (msg.data.status === "disconnected") {
            app.$store.commit("DEVICE_DISCONNECTED", msg.data);
        } else {
            console.warn("Unexpected device status", msg);
        }
        break;
    case "ws.device:measurement":
        if (msg.data.recognizedAs === "hr") {
            app.$store.commit("MEASUREMENT_HR", msg.data);
        } else if (msg.data.recognizedAs === "csc_speed") {
            app.$store.commit("MEASUREMENT_CSC_SPEED", msg.data);
        } else if (msg.data.recognizedAs === "csc_cadence") {
            app.$store.commit("MEASUREMENT_CSC_CADENCE", msg.data);
        } else {
            console.warn("Unrecognized measurement", msg);
        }
        break;
    default:
        console.warn("Unhandled message", msg);
    }
};

export default wsMessageHandler;
