import { createRouter, createWebHistory, isNavigationFailure, NavigationFailureType } from "vue-router";
import { notify } from "@kyvg/vue3-notification";

import { requireAuth } from "./auth";
import { store } from "./store/index";
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

// A page's chunk fails to load when the page came from the build before a
// self-update, which had other chunk names. When wakeci now runs another
// version than the page was loaded from, the page loads the URL in full to get
// the new build; once loaded from it, it can't loop. Otherwise it says the
// page couldn't be loaded
// Each navigation's number, by its target, so that a failure arriving after a
// newer navigation began can be told apart
let navigations = 0;
const navigationNumbers = new WeakMap();
router.beforeEach((to) => {
    navigations++;
    navigationNumbers.set(to, navigations);
});
// A click on the page still shown skips beforeEach, but it is a newer choice
// too: the feed stays shown while Jobs loads, and a click on Feed means stay
router.afterEach((to, from, failure) => {
    if (isNavigationFailure(failure, NavigationFailureType.duplicated)) {
        navigations++;
    }
});

// How long the page waits for wakeci's version before it gives up
const versionCheckTimeout = 5000;

function cantLoad() {
    notify({ type: "error", text: "This page couldn't be loaded. Reload the page to try again." });
}

async function runningVersion() {
    const response = await fetch("/api/queue/", { cache: "no-store", signal: AbortSignal.timeout(versionCheckTimeout) });
    if (!response.ok) {
        throw new Error(`wakeci answered ${response.status}`);
    }
    return (await response.json()).version;
}

router.onError((error, to) => {
    // Registering onError stops vue-router from logging it
    console.error(`Navigation to ${to.fullPath} failed:`, error);
    const navigation = navigationNumbers.get(to);
    runningVersion()
        .then((version) => {
            const loaded = store.state.serverVersion;
            if (!version || !loaded || version === loaded) {
                cantLoad();
                return;
            }
            // Not when another page was opened meanwhile: App.vue reloads that
            // one once its connection sees the new version
            if (navigation === navigations) {
                window.location.assign(to.fullPath);
            }
        })
        .catch((reason) => {
            console.warn("Can't tell which version wakeci runs:", reason);
            cantLoad();
        });
});

export default router;
