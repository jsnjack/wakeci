import { createRouter, createWebHistory } from "vue-router";
import { notify } from "@kyvg/vue3-notification";

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
// before a self-update had. If wakeci answers, the page loads the URL in full,
// once, to get the new build; a chunk that fails again shows an error instead
// of reloading forever
const chunkReloadKey = "wakeci-chunk-reload";
// How long a reload counts as the one for its URL, so a mark left by a reload
// that ended elsewhere (say on the login page) doesn't stop a later one
const chunkReloadWindow = 30 * 1000;
const chunkLoadError =
    /Failed to fetch dynamically imported module|error loading dynamically imported module|Importing a module script failed|Unable to preload CSS/i;

// withStorage runs use with sessionStorage and returns its result, or
// undefined when the browser blocks sessionStorage
function withStorage(use) {
    try {
        return use(sessionStorage);
    } catch (error) {
        console.debug("sessionStorage is unavailable:", error);
        return undefined;
    }
}

// reloadedFor reports whether the page was just loaded in full for path
function reloadedFor(storage, path) {
    let mark;
    try {
        mark = JSON.parse(storage.getItem(chunkReloadKey));
    } catch (error) {
        console.debug("Ignoring an unreadable reload mark:", error);
        return false;
    }
    return mark?.path === path && Date.now() - mark.at < chunkReloadWindow;
}

function cantLoad() {
    notify({ type: "error", text: "This page couldn't be loaded. Reload the page to try again." });
}

router.onError((error, to) => {
    // Registering onError stops vue-router from logging it
    console.error(`Navigation to ${to.fullPath} failed:`, error);
    if (!chunkLoadError.test(String(error?.message))) {
        return;
    }
    const reloaded = withStorage((storage) => reloadedFor(storage, to.fullPath));
    if (reloaded !== false) {
        // Already loaded in full for it, or, without storage, the reload
        // can't be limited to once
        withStorage((storage) => storage.removeItem(chunkReloadKey));
        cantLoad();
        return;
    }
    // While wakeci restarts, a reload would replace the app with the
    // browser's error page
    fetch("/", { cache: "no-store" })
        .then((response) => {
            if (!response.ok) {
                throw new Error(`wakeci answered ${response.status}`);
            }
            const marked = withStorage((storage) => {
                storage.setItem(chunkReloadKey, JSON.stringify({ path: to.fullPath, at: Date.now() }));
                return true;
            });
            if (!marked) {
                cantLoad();
                return;
            }
            window.location.assign(to.fullPath);
        })
        .catch((error) => {
            console.warn(`Not loading ${to.fullPath} in full, wakeci doesn't answer:`, error);
            cantLoad();
        });
});

// Only a successful load of that URL ends it: a full load passes through
// /login first, while the session is checked
router.afterEach((to, from, failure) => {
    if (failure) {
        return;
    }
    withStorage((storage) => {
        if (reloadedFor(storage, to.fullPath)) {
            storage.removeItem(chunkReloadKey);
        }
    });
});

export default router;
