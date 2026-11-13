<template>
    <header :class="{ fill: theme === 'light' }">
        <nav>
            <router-link to="/">
                <i class="large fill primary-text">water</i>
            </router-link>
            <small class="m l">{{ getVesion }}</small>
            <h6 class="max center-align">{{ currentPage }}</h6>
            <router-link
                v-if="auth.isLoggedIn"
                to="/"
                class="button circle transparent m l"
            >
                <div class="tooltip bottom">Feed</div>
                <i>list</i>
            </router-link>
            <router-link
                v-if="auth.isLoggedIn"
                to="/jobs"
                class="button circle transparent m l"
            >
                <div class="tooltip bottom">Jobs</div>
                <i>apps</i>
            </router-link>
            <router-link
                v-if="auth.isLoggedIn && canAny('read')"
                to="/analytics"
                class="button circle transparent m l"
                data-cy="nav-analytics"
            >
                <div class="tooltip bottom">Analytics</div>
                <i>monitoring</i>
            </router-link>
            <router-link
                v-if="auth.isLoggedIn"
                to="/keys"
                class="button circle transparent m l"
                data-cy="nav-keys"
            >
                <div class="tooltip bottom">API keys</div>
                <i>key</i>
            </router-link>
            <router-link
                v-if="auth.isLoggedIn && canAny('write')"
                to="/settings"
                class="button circle transparent m l"
                data-cy="nav-settings"
            >
                <div class="tooltip bottom">Settings</div>
                <i>settings</i>
            </router-link>
            <button
                class="circle transparent m l"
                @click.prevent="toggleDarkMode"
            >
                <div class="tooltip bottom">Toggle dark mode</div>
                <i>dark_mode</i>
            </button>
            <router-link
                to="/help"
                class="button circle transparent m l"
            >
                <div class="tooltip bottom">Help</div>
                <i>help</i>
            </router-link>
            <button
                class="circle transparent"
                data-cy="logout"
                href="#"
                @click.prevent="logOut"
            >
                <i>logout</i>
                <div class="tooltip bottom">Log out</div>
            </button>
        </nav>
    </header>
    <main class="responsive no-scroll medium-padding">
        <QueueBanner v-if="auth.isLoggedIn" />
        <router-view />
    </main>
    <notifications
        classes="snackbar active"
        :dangerously-set-inner-html="true"
        :max="1"
        :pause-on-hover="true"
    />
</template>

<script>
// importing as beercss and materialDynamicColors
import "@/assets/main.css";
import "beercss";
import "material-dynamic-colors";

import QueueBanner from "@/components/QueueBanner.vue";
import { getWSURL } from "@/store/communication.js";
import axios from "axios";
import vuex from "vuex";
import wsMessageHandler from "./store/communication.js";

export default {
    components: { QueueBanner },
    data: function () {
        return {
            // How many times the page asked for the queue status
            queueRequests: 0,
            // How many WebSocket connections opened
            connections: 0,
        };
    },
    computed: {
        ...vuex.mapState(["ws", "auth", "currentPage", "theme"]),
        ...vuex.mapGetters(["canAny"]),
        getVesion: function () {
            return import.meta.env.VITE_VERSION || "0.0.0";
        },
    },
    mounted() {
        this.connect();
        this.applyTheme();
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
                .get("/auth/logout")
                .then(() => {
                    this.$store.commit("LOG_OUT");
                    this.$router.push("/login");
                })
                .catch(() => {});
        },
        toggleDarkMode: function () {
            if (this.theme === "light") {
                this.$store.commit("SET_THEME", "dark");
            } else {
                this.$store.commit("SET_THEME", "light");
            }
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

<style></style>
