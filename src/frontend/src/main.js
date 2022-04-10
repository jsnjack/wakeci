import { createApp } from "vue";
import App from "./App.vue";
import router from "./router";
import { store } from "./store/index";
import Notifications from "@kyvg/vue3-notification";
import { notify } from "@kyvg/vue3-notification";
import axios from "axios";
import mitt from "mitt";

const emitter = mitt();

const app = createApp(App);
app.config.globalProperties.emitter = emitter;
app.use(router);
app.use(store);
app.use(Notifications);
app.mount("#app");

// Global axios handler to show error messages and redirect to the login page
// in case of error is 403
axios.interceptors.response.use(
    function (response) {
        return response;
    },
    function (error) {
        // Exclude special request to check if user is logged in
        if (error.request.responseURL.indexOf("/_isLoggedIn") === -1) {
            notify({
                text: (error.response && error.response.data) || error,
                type: "error",
            });
            if (error.response.status === 403) {
                store.commit("LOG_OUT");
                router.push("/login");
            }
        }
        return Promise.reject(error);
    }
);
