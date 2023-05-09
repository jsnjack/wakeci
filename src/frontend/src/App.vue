<template>
    <div id="app">
        <header>
            <nav>
                <h6>wake</h6>
                <small>{{ getVesion }}</small>
                <div class="max"></div>
                <button class="circle transparent">
                    <div class="tooltip bottom">Feed</div>
                    <router-link to="/">
                        <i>list</i>
                    </router-link>
                </button>
                <button class="circle transparent">
                    <div class="tooltip bottom">Jobs</div>
                    <router-link to="/jobs">
                        <i>task_alt</i>
                    </router-link>
                </button>
                <button class="circle transparent">
                    <div class="tooltip bottom">Settings</div>
                    <router-link to="/settings">
                        <i>settings</i>
                    </router-link>
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
        <router-view />
        <notifications
            classes="my-noty"
            position="bottom right"
        />
    </div>
</template>

<script>
// importing as beercss and materialDynamicColors
import "beercss";
import "material-dynamic-colors";

import vuex from "vuex";
import axios from "axios";
import { getWSURL } from "@/store/communication.js";
import wsMessageHandler from "./store/communication.js";
import DocsMenu from "@/components/DocsMenu.vue";

export default {
    components: { DocsMenu },
    computed: {
        ...vuex.mapState(["ws", "auth"]),
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
    },
};
</script>

<style lang="scss">
@import "@/assets/wakeci.scss";

#app {
    font-family: "Avenir", Helvetica, Arial, sans-serif;
    -webkit-font-smoothing: antialiased;
    -moz-osx-font-smoothing: grayscale;
    text-align: center;
    color: #2c3e50;
}

.navbar {
    transition: background-color 1000ms linear;
}

.header-connected {
    background: tomato;
}

.header-connected[data-hostname="mrt-wake.surfly.com"] {
    background: #333;
}

.header-connected[data-hostname="build.surfly.com"] {
    background: #b8662c;
}

.header-disconnected {
    background: #6f6f94;
}

.my-noty {
    padding: 10px;
    margin: 0 5px 5px;

    color: tomato;
    background: tomato;
    border-left: 5px solid tomato;

    & a {
        color: tomato;
        text-decoration: underline;
    }

    &.warn {
        background: tomato;
        border-left-color: darken(tomato, 10%);
    }

    &.error {
        background: tomato;
        border-left-color: darken(tomato, 10%);
    }

    &.success {
        background: tomato;
        border-left-color: darken(tomato, 10%);
    }
}
</style>
