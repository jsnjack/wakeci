<template>
    <header class="ui app-header">
        <router-link
            to="/"
            class="brand"
            aria-label="wakeci, the feed"
            data-tip="Feed"
        >
            <i class="icon">water</i>
        </router-link>
        <span
            class="version muted text-sm"
            data-cy="app-version"
            >{{ getVesion }}</span
        >
        <h1
            class="page-title ellipsis"
            data-cy="page-title"
        >
            {{ currentPage }}
        </h1>
        <span
            v-if="offline"
            class="reconnecting cluster nowrap gap-1 text-sm"
            role="status"
            data-cy="reconnecting"
        >
            <i
                class="icon small"
                aria-hidden="true"
                >sync</i
            >
            <span class="reconnecting-label">Reconnecting…</span>
        </span>
        <nav
            class="app-nav"
            aria-label="Pages"
        >
            <router-link
                v-for="link in links"
                :key="link.to"
                :to="link.to"
                :data-cy="link.cy"
                :data-tip="link.label"
                :aria-label="link.label"
            >
                <i class="icon">{{ link.icon }}</i>
                <span class="link-label">{{ link.label }}</span>
            </router-link>
        </nav>
        <div class="cluster nowrap gap-1 header-actions">
            <button
                type="button"
                class="btn btn-ghost btn-icon wide-only"
                data-cy="toggle-theme"
                :aria-label="themeLabel"
                :data-tip="themeLabel"
                @click="toggleDarkMode"
            >
                <i class="icon">{{ theme === "light" ? "dark_mode" : "light_mode" }}</i>
            </button>
            <button
                v-if="auth.isLoggedIn"
                type="button"
                class="btn btn-ghost btn-icon"
                data-cy="logout"
                aria-label="Log out"
                data-tip="Log out"
                @click="logOut"
            >
                <i class="icon">logout</i>
            </button>
            <UiMenu
                class="phone-only"
                label="Pages"
                icon="menu"
                cy="nav-menu"
            >
                <router-link
                    v-for="link in links"
                    :key="link.to"
                    :to="link.to"
                    role="menuitem"
                >
                    <i class="icon">{{ link.icon }}</i>
                    {{ link.label }}
                </router-link>
                <button
                    type="button"
                    role="menuitem"
                    @click="toggleDarkMode"
                >
                    <i class="icon">{{ theme === "light" ? "dark_mode" : "light_mode" }}</i>
                    {{ themeLabel }}
                </button>
            </UiMenu>
        </div>
    </header>
    <main class="app-main">
        <QueueBanner v-if="auth.isLoggedIn" />
        <!-- A new path mounts its page again, e.g. another build's -->
        <router-view :key="$route.path" />
    </main>
    <notifications
        class="ui"
        classes="toast"
        position="bottom center"
        :dangerously-set-inner-html="true"
        :max="1"
        :pause-on-hover="true"
    />
</template>

<script>
import "@/assets/styles.css";

import QueueBanner from "@/components/QueueBanner.vue";
import UiMenu from "@/components/ui/UiMenu.vue";
import { getWSURL } from "@/store/communication.js";
import axios from "axios";
import vuex from "vuex";
import wsMessageHandler from "./store/communication.js";

// How long the connection is down before the header says so (L6)
const OFFLINE_AFTER_MS = 3000;

export default {
    components: { QueueBanner, UiMenu },
    data: function () {
        return {
            // How many times the page asked for the queue status
            queueRequests: 0,
            // How many WebSocket connections opened
            connections: 0,
            // The connection has been down for a while (L6)
            offline: false,
            offlineTimer: null,
        };
    },
    computed: {
        // Every build's updates are needed while the user watches builds or
        // searches, whatever page is open (L5)
        // Live updates need the connection (L6)
        wsDown() {
            return this.auth.isLoggedIn && !this.ws.connected;
        },
        notifies() {
            const watches = this.$store.state.watches;
            return this.auth.isLoggedIn && (watches.builds.length > 0 || watches.searches.length > 0);
        },
        ...vuex.mapState(["ws", "auth", "currentPage", "theme"]),
        ...vuex.mapGetters(["canAny"]),
        getVesion: function () {
            return import.meta.env.VITE_VERSION || "0.0.0";
        },
        // The pages the header links to, those the user may open
        links() {
            if (!this.auth.isLoggedIn) {
                return [{ to: "/help", label: "Help", icon: "help", cy: "nav-help" }];
            }
            return [
                { to: "/", label: "Feed", icon: "list", cy: "nav-feed" },
                { to: "/jobs", label: "Jobs", icon: "apps", cy: "nav-jobs" },
                ...(this.canAny("read") ? [{ to: "/analytics", label: "Analytics", icon: "monitoring", cy: "nav-analytics" }] : []),
                { to: "/keys", label: "API keys", icon: "key", cy: "nav-keys" },
                ...(this.canAny("write") ? [{ to: "/settings", label: "Settings", icon: "settings", cy: "nav-settings" }] : []),
                { to: "/help", label: "Help", icon: "help", cy: "nav-help" },
            ];
        },
        themeLabel() {
            return this.theme === "light" ? "Dark theme" : "Light theme";
        },
    },
    watch: {
        // A short drop, e.g. a restart, says nothing; one of a few seconds
        // says that the page doesn't update
        wsDown: {
            immediate: true,
            handler(down) {
                clearTimeout(this.offlineTimer);
                if (down) {
                    this.offlineTimer = setTimeout(() => (this.offline = true), OFFLINE_AFTER_MS);
                } else {
                    this.offline = false;
                }
            },
        },
        notifies: {
            immediate: true,
            handler(value, old) {
                if (value !== Boolean(old)) {
                    this.$store.commit(value ? "SUBSCRIBE_TOPIC" : "UNSUBSCRIBE_TOPIC", "build:update:");
                }
            },
        },
    },
    mounted() {
        this.connect();
        this.applyTheme();
        // Until the user picks a theme, it follows the system's
        this.systemTheme = window.matchMedia?.("(prefers-color-scheme: dark)");
        this.systemTheme?.addEventListener("change", this.onSystemTheme);
    },
    unmounted() {
        this.systemTheme?.removeEventListener("change", this.onSystemTheme);
        clearTimeout(this.offlineTimer);
    },
    methods: {
        connect: function () {
            if (!this.$store.state.ws.connected) {
                if (!this.auth.isLoggedIn) {
                    setTimeout(this.connect, this.ws.reconnectTimeout);
                    return;
                }
                const ws = new WebSocket(getWSURL());
                ws.sendMessage = function (obj) {
                    ws.send(JSON.stringify(obj));
                };
                // Listen for messages
                ws.addEventListener("message", (event) => {
                    wsMessageHandler(this, event.data);
                });

                ws.addEventListener("close", () => {
                    this.$store.commit("WS_DISCONNECTED");
                    if (this.ws.failedAttempts >= this.ws.maxFailedAttempts) {
                        this.$store.commit("LOG_OUT");
                        this.$router.push("/login");
                    }
                    setTimeout(this.connect, this.$store.state.ws.reconnectTimeout);
                });

                ws.addEventListener("error", (event) => {
                    console.warn("WS connection error", event);
                });

                ws.addEventListener("open", () => {
                    this.$store.commit("WS_CONNECTED", ws);
                    this.watchQueueStatus();
                    if (this.connections++ > 0) {
                        this.reloadAccess();
                        this.$store.commit("WS_RECONNECTED");
                    }
                });
            } else {
                console.error("WS already connected");
            }
        },
        // The server closes the connections of a user whose permissions
        // changed, so they are read again on every reconnection. Only an
        // answer that the session ended logs out: a reconnection also
        // follows a restart, when a request can fail on its own.
        reloadAccess: function () {
            axios.get("/auth/_isLoggedIn").then(
                (response) => this.$store.commit("LOG_IN", response.data),
                (error) => {
                    if (error.response?.status === 401 || error.response?.status === 403) {
                        this.$store.commit("LOG_OUT");
                        this.$router.push("/login");
                    }
                },
            );
        },
        // Fetched again on every connection, since a restart ends a stop.
        // After an update the server runs a new version, and the page
        // reloads to get its frontend
        watchQueueStatus: function () {
            this.$store.commit("WS_SEND", {
                type: "in:subscribe",
                data: { to: ["queue:status"] },
            });
            // A status that arrives while the request runs is newer, and the
            // server sends a stop only once. So is the answer to the request
            // of a later connection
            const request = ++this.queueRequests;
            const count = this.$store.state.queueStatusCount;
            axios
                .get("/api/queue/")
                .then((response) => {
                    if (request !== this.queueRequests) {
                        return;
                    }
                    const version = response.data.version;
                    const loaded = this.$store.state.serverVersion;
                    if (version && loaded && version !== loaded) {
                        window.location.reload();
                        return;
                    }
                    if (version) {
                        this.$store.commit("SET_SERVER_VERSION", version);
                    }
                    if (this.$store.state.queueStatusCount === count) {
                        this.$store.commit("SET_QUEUE_STATUS", response.data);
                    }
                })
                .catch(() => {});
        },
        logOut: function () {
            axios
                .post("/auth/logout")
                .then(() => {
                    this.$store.commit("LOG_OUT");
                    this.$router.push("/login");
                })
                .catch(() => {});
        },
        toggleDarkMode: function () {
            this.$store.commit("SET_THEME", { theme: this.theme === "light" ? "dark" : "light" });
            this.applyTheme();
        },
        onSystemTheme(event) {
            if (window.localStorage.getItem("theme")) {
                return;
            }
            this.$store.commit("SET_THEME", { theme: event.matches ? "dark" : "light", picked: false });
            this.applyTheme();
        },
        applyTheme: function () {
            if (this.theme === "light") {
                document.body.classList.remove("dark");
                document.body.classList.add("light");
            } else {
                document.body.classList.add("dark");
                document.body.classList.remove("light");
            }
        },
    },
};
</script>

<style>
/* The header on top, the page below it filling the window */
#app {
    display: grid;
    grid-template-rows: auto minmax(0, 1fr);
    min-block-size: 100dvh;
}

.app-main {
    position: relative;
    inline-size: 100%;
    max-inline-size: 75rem;
    margin-inline: auto;
    padding: var(--space-4);
    /* Nothing on a page makes it wider than the window */
    overflow: hidden;
}

/* Wider for pages of logs, so their lines wrap less */
.app-main:has(.build-page, .feed-page.split) {
    max-inline-size: 100rem;
}

.app-header {
    position: sticky;
    inset-block-start: 0;
    z-index: 30;
    display: flex;
    align-items: center;
    gap: var(--space-3);
    block-size: 4rem;
    padding: 0 var(--space-4);
    background-color: var(--surface-container);
    border-block-end: 1px solid var(--outline-variant);
}

.app-header .brand {
    display: inline-flex;
    color: var(--primary);
}

.app-header .brand .icon {
    font-size: 28px;
}

.reconnecting {
    padding: 2px var(--space-2);
    border-radius: var(--radius-pill);
    background-color: color-mix(in srgb, var(--warning) 18%, transparent);
    color: var(--on-surface);
    white-space: nowrap;
}

.reconnecting .icon {
    animation: spin 1.2s linear infinite;
}

@keyframes spin {
    to {
        transform: rotate(-360deg);
    }
}

@media (prefers-reduced-motion: reduce) {
    .reconnecting .icon {
        animation: none;
    }
}

.app-header .page-title {
    flex: 1 1 auto;
    font-size: var(--text-md);
    font-weight: 600;
}

.app-nav {
    display: flex;
    gap: var(--space-1);
}

.app-nav > a {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    block-size: 32px;
    padding: 0 var(--space-3);
    border-radius: var(--radius-pill);
    color: var(--on-surface-variant);
    font-size: var(--text-sm);
    font-weight: 500;
    white-space: nowrap;
}

.app-nav > a:hover {
    background-color: color-mix(in srgb, var(--on-surface) 8%, transparent);
    text-decoration: none;
}

.app-nav > a[aria-current="page"] {
    background-color: var(--secondary-container);
    color: var(--on-secondary-container);
}

.app-nav .icon {
    font-size: 18px;
}

.phone-only {
    display: none;
}

/* Labels where they fit; below that, tips name the icons */
@media (min-width: 1025px) {
    .app-nav > a::after {
        content: none;
    }
}

@media (max-width: 1024px) {
    .app-header .link-label {
        display: none;
    }
    .app-nav > a {
        padding: 0 var(--space-2);
    }
}

@media (max-width: 600px) {
    .app-header .version,
    .app-nav,
    .app-header .wide-only {
        display: none;
    }
    .phone-only {
        display: inline-flex;
    }
    /* Only the turning icon; the text stays for screen readers */
    .reconnecting-label {
        position: absolute;
        inline-size: 1px;
        block-size: 1px;
        overflow: hidden;
        clip-path: inset(50%);
        white-space: nowrap;
    }
}
</style>
