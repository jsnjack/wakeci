import Vue from "vue";
import Router from "vue-router";
import LoginView from "./views/LoginView.vue";
import {requireAuth} from "./auth";


Vue.use(Router);

export default new Router({
    mode: "history",
    routes: [
        {
            path: "/login",
            name: "login",
            component: LoginView,
        },
        {
            path: "/",
            name: "feed",
            component() {
                return import(/* webpackChunkName: "jobs" */ "./views/FeedView.vue");
            },
            beforeEnter: requireAuth,
        },
        {
            path: "/jobs",
            name: "jobs",
            beforeEnter: requireAuth,
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
            beforeEnter: requireAuth,
            component() {
                return import(/* webpackChunkName: "build" */ "./views/BuildView.vue");
            },
            props: true,
        },
    ],
});
