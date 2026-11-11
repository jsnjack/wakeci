import { createApp } from "vue";
import App from "./App.vue";
import router from "./router";
import { store } from "./store/index";
import Notifications from "@kyvg/vue3-notification";
import { notify } from "@kyvg/vue3-notification";
import axios from "axios";
import mitt from "mitt";
import { escapeHtml } from "./store/utils";

const emitter = mitt();

const app = createApp(App);
app.config.globalProperties.emitter = emitter;
app.use(router);
app.use(store);
app.use(Notifications);
app.mount("#app");

// Global axios handler to show error messages and redirect to the login page
// when the session ended
axios.interceptors.response.use(
    function (response) {
        return response;
    },
    function (error) {
        // Exclude special request to check if user is logged in
        const responseURL = error.request?.responseURL || "";
        if (!responseURL.includes("/_isLoggedIn")) {
            notify({
                text: escapeHtml((error.response && error.response.data) || error),
                type: "error",
            });
            if (error.response?.status === 403) {
                // Either the session ended or the user's groups don't allow
                // the request; only the first logs out. The permissions may
                // have changed, so they are read again.
                axios.get("/auth/_isLoggedIn").then(
                    (response) => store.commit("LOG_IN", response.data),
                    () => {
                        store.commit("LOG_OUT");
                        router.push("/login");
                    },
                );
            }
        }
        return Promise.reject(error);
    },
);
