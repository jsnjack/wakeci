import Vue from "vue";
import App from "./App.vue";
import router from "./router";
import store from "./store/index";
import Notifications from "@kyvg/vue3-notification";
import axios from "axios";


Vue.prototype.$eventHub = new Vue();
Vue.config.productionTip = false;
Vue.use(Notifications);

const app = Vue.createApp(App);
app.use(router);
app.use(store);
app.mount("#app");

// Global axios handler to show error messages and redirect to the login page
// in case of error is 403
axios.interceptors.response.use(function(response) {
    return response;
}, function(error) {
    // Exclude special request to check if user is logged in
    if (error.request.responseURL.indexOf("/_isLoggedIn") === -1) {
        v.$notify({
            text: error.response && error.response.data || error,
            type: "error",
        });
        if (error.response.status === 403) {
            v.$store.commit("LOG_OUT");
            v.$router.push("/login");
        }
    }
    return Promise.reject(error);
});
