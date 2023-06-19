<template>
    <header>
        <nav>
            <router-link to="/">
                <i class="large fill primary-text">water</i>
            </router-link>
            <small>{{ getVesion }}</small>
            <h6 class="max center-align">{{ currentPage }}</h6>
            <router-link
                to="/"
                class="button circle transparent"
            >
                <div class="tooltip bottom">Feed</div>
                <i>list</i>
            </router-link>
            <router-link
                to="/jobs"
                class="button circle transparent"
            >
                <div class="tooltip bottom">Jobs</div>
                <i>apps</i>
            </router-link>
            <router-link
                to="/settings"
                class="button circle transparent"
            >
                <div class="tooltip bottom">Settings</div>
                <i>settings</i>
            </router-link>
            <button
                class="circle transparent"
                @click.prevent="toggleDarkMode"
            >
                <div class="tooltip bottom">Toggle dark mode</div>
                <i>dark_mode</i>
            </button>
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
    <main class="responsive no-scroll">
        <router-view />
    </main>
    <notifications
        classes="toast active"
        :max="1"
        :pauseOnHover="true"
    />
</template>

<script>
// importing as beercss and materialDynamicColors
import "beercss";
import "material-dynamic-colors";
import "@/assets/main.scss";

import vuex from "vuex";
import axios from "axios";
import { getWSURL } from "@/store/communication.js";
import wsMessageHandler from "./store/communication.js";
import DocsMenu from "@/components/DocsMenu.vue";

export default {
    components: { DocsMenu },
    data: function () {
        return {
            darkMode: false,
        };
    },
    computed: {
        ...vuex.mapState(["ws", "auth", "currentPage"]),
        getVesion: function () {
            return import.meta.env.VITE_VERSION || "0.0.0";
        },
    },
    mounted() {
        this.connect();
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

                ws.addEventListener("close", (event) => {
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

                ws.addEventListener("open", (event) => {
                    this.$store.commit("WS_CONNECTED", ws);
                });
            } else {
                console.error("WS already connected");
            }
        },
        logOut: function () {
            axios
                .get("/auth/logout")
                .then((response) => {
                    this.$store.commit("LOG_OUT");
                    this.$router.push("/login");
                })
                .catch((error) => {});
        },
        toggleDarkMode: function () {
            if (this.darkMode) {
                document.body.classList.remove("dark");
                document.body.classList.add("light");
                this.darkMode = false;
            } else {
                document.body.classList.add("dark");
                document.body.classList.remove("light");
                this.darkMode = true;
            }
        },
    },
};
</script>

<style></style>
