import { createRouter, createWebHistory } from "vue-router";

import { requireAuth } from "./auth";
// Every full page load passes through /login while the session is checked,
// so the login page is in the main chunk instead of one more request
import LoginView from "./views/LoginView.vue";

const router = createRouter({
    history: createWebHistory(),
    // Each page loads its code when it is first opened, so the first page
    // doesn't download the job editor (CodeMirror) and the rest
    routes: [
        {
            path: "/login",
            name: "login",
            component: LoginView,
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

// A page's chunk that fails to load is most likely one that only the build
// before a self-update had. The page then loads the URL in full, once, to get
// the new build; a chunk this build lacks as well keeps its error, instead of
// reloading forever
const chunkReloadKey = "wakeci-chunk-reload";
const chunkLoadError =
    /Failed to fetch dynamically imported module|error loading dynamically imported module|Importing a module script failed|Unable to preload CSS/i;

router.onError((error, to) => {
    if (!chunkLoadError.test(String(error?.message))) {
        return;
    }
    try {
        if (sessionStorage.getItem(chunkReloadKey) === to.fullPath) {
            sessionStorage.removeItem(chunkReloadKey);
            return;
        }
        sessionStorage.setItem(chunkReloadKey, to.fullPath);
    } catch {
        // Without storage the reload can't be limited to once
        return;
    }
    window.location.assign(to.fullPath);
});

// Only a successful load of that URL ends it: a full load passes through
// /login first, while the session is checked
router.afterEach((to, from, failure) => {
    if (failure) {
        return;
    }
    try {
        if (sessionStorage.getItem(chunkReloadKey) === to.fullPath) {
            sessionStorage.removeItem(chunkReloadKey);
        }
    } catch {
        // Nothing was stored then
    }
});

export default router;
