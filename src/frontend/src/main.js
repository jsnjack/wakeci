import Vue from "vue";
import App from "./App.vue";
import router from "./router";
import store from "./store/index";
import Notifications from "vue-notification";


Vue.prototype.$eventHub = new Vue();
Vue.config.productionTip = false;
Vue.use(Notifications);

new Vue({
    router,
    store,
    render(h) {
        return h(App);
    },
}).$mount("#app");

