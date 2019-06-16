import Vue from "vue";
import Router from "vue-router";
import FeedView from "./views/FeedView.vue";


Vue.use(Router);

export default new Router({
    mode: "history",
    routes: [
        {
            path: "/",
            name: "feed",
            component: FeedView,
        },
        {
            path: "/jobs",
            name: "jobs",
            // route level code-splitting
            // this generates a separate chunk (about.[hash].js) for this route
            // which is lazy-loaded when the route is visited.
            component() {
                return import(/* webpackChunkName: "jobs" */ "./views/JobsView.vue");
            },
        },
        {
            path: "/build/:id",
            name: "build",
            component() {
                return import(/* webpackChunkName: "build" */ "./views/BuildView.vue");
            },
            props: true,
        },
    ],
});
