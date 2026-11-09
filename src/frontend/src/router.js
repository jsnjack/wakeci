import { createRouter, createWebHistory } from "vue-router";

import { requireAuth } from "./auth";

const router = createRouter({
    history: createWebHistory(),
    // Each page loads its code when it is first opened, so the first page
    // doesn't download the job editor (CodeMirror) and the rest
    routes: [
        {
            path: "/login",
            name: "login",
            component: () => import("./views/LoginView.vue"),
        },
        {
            path: "/",
            name: "feed",
            component: () => import("./views/FeedView.vue"),
            beforeEnter: requireAuth,
        },
        {
            path: "/jobs",
            name: "jobs",
            component: () => import("./views/JobsView.vue"),
            beforeEnter: requireAuth,
        },
        {
            path: "/job/:name",
            name: "jobEdit",
            component: () => import("./views/JobEditView.vue"),
            beforeEnter: requireAuth,
            props: true,
        },
        {
            path: "/build/:id",
            name: "build",
            component: () => import("./views/BuildView.vue"),
            beforeEnter: requireAuth,
            props: function (route) {
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
            component: () => import("./views/SettingsView.vue"),
            beforeEnter: requireAuth,
        },
        {
            path: "/keys",
            name: "keys",
            component: () => import("./views/KeysView.vue"),
            beforeEnter: requireAuth,
        },
        {
            path: "/keys/new",
            name: "keyCreate",
            component: () => import("./views/KeyCreateView.vue"),
            beforeEnter: requireAuth,
        },
        {
            path: "/help",
            name: "help",
            component: () => import("./views/HelpView.vue"),
        },
    ],
});

export default router;
