import Vue from "vue";
import App from "./App.vue";
import router from "./router";
import store from "./store/index";
import Notifications from "@kyvg/vue3-notification";
import axios from "axios";
import {createApp} from "vue";

Vue.config.productionTip = false;
Vue.configureCompat({WATCH_ARRAY: false});

const app = createApp(App);
app.config.globalProperties.$eventHub = new Vue();
app.use(router);
app.use(store);
app.use(Notifications);
app.mount("#app");

// Global axios handler to show error messages and redirect to the login page
// in case of error is 403
axios.interceptors.response.use(function(response) {
    return response;
}, function(error) {
    // Exclude special request to check if user is logged in
    if (error.request.responseURL.indexOf("/_isLoggedIn") === -1) {
        app.prototype.$notify({
            text: error.response && error.response.data || error,
            type: "error",
        });
        if (error.response.status === 403) {
            app.prototype.$store.commit("LOG_OUT");
            app.prototype.$router.push("/login");
        }
    }
    return Promise.reject(error);
});
