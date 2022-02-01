import {createRouter, createWebHistory} from "vue-router";
import LoginView from "./views/LoginView.vue";
import FeedView from "./views/FeedView.vue";
import JobsView from "./views/JobsView.vue";
import BuildView from "./views/BuildView.vue";
import SettingsView from "./views/SettingsView.vue";
import JobEditView from "./views/JobEditView.vue";
import SyntaxView from "./views/SyntaxView.vue";

import {requireAuth} from "./auth";

const router = createRouter({
    history: createWebHistory(),
    routes: [
        {
            path: "/login",
            name: "login",
            component: LoginView,
        },
        {
            path: "/",
            name: "feed",
            component: FeedView,
            beforeEnter: requireAuth,
        },
        {
            path: "/jobs",
            name: "jobs",
            component: JobsView,
            beforeEnter: requireAuth,
        },
        {
            path: "/job/:name",
            name: "jobEdit",
            component: JobEditView,
            beforeEnter: requireAuth,
            props: true,
        },
        {
            path: "/build/:id",
            name: "build",
            component: BuildView,
            beforeEnter: requireAuth,
            props: function(route) {
                let id = route.params.id;
                if (typeof id !== "number") {
                    id = Number(id);
                }
                return {
                    id: id,
                };
            },
        },
        {
            path: "/settings",
            name: "settings",
            component: SettingsView,
            beforeEnter: requireAuth,
        },
        {
            path: "/syntax",
            name: "syntax",
            component: SyntaxView,
            beforeEnter: requireAuth,
        },
    ],
});

export default router;
