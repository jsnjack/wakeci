<template>
  <div id="app">
    <header
      class="navbar"
      :class="getHeaderClass"
      :data-hostname="getHostname"
    >
      <section class="navbar-section">
        <small class="text-gray">v {{ getVesion }}</small>
      </section>
      <section class="navbar-center">
        <router-link
          to="/"
          class="btn btn-link text-light"
        >
          Feed
        </router-link>
        <router-link
          to="/jobs"
          class="btn btn-link text-light"
        >
          Jobs
        </router-link>
        <router-link
          to="/settings"
          class="btn btn-link text-light"
        >
          Settings
        </router-link>
        <DocsMenu />
      </section>
      <section class="navbar-section">
        <a
          data-cy="logout"
          href="#"
          class="btn btn-link text-light"
          @click.prevent="logOut"
        >Log out</a>
      </section>
    </header>
    <router-view />
    <notifications
      classes="my-noty"
      position="bottom right"
    />
  </div>
</template>

<script>
import vuex from "vuex";
import axios from "axios";
import {getWSURL} from "@/store/communication.js";
import wsMessageHandler from "./store/communication.js";
import DocsMenu from "@/components/DocsMenu.vue";

export default {
    components: {DocsMenu},
    computed: {
        ...vuex.mapState(["ws", "auth", "durationMode"]),
        getVesion: function() {
            return import.meta.env.VITE_VERSION || "0.0.0";
        },
        getHeaderClass: function() {
            if (this.$store.state.ws.connected) {
                return "header-connected";
            }
            return "header-disconnected";
        },
        getHostname: function() {
            return location.hostname;
        },
    },
    mounted() {
        // Restore global duration mode
        if (localStorage.getItem("durationMode")) {
            this.$store.commit("TOGGLE_DURATION_MODE", localStorage.getItem("durationMode"));
        }
        this.connect();
    },
    methods: {
        connect: function() {
            if (!this.$store.state.ws.connected) {
                if (!this.auth.isLoggedIn) {
                    setTimeout(this.connect, this.ws.reconnectTimeout);
                    return;
                }
                const ws = new WebSocket(getWSURL());
                ws.sendMessage = function(obj) {
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
        logOut: function() {
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
  background: $primary-color;
}

.header-connected[data-hostname="mrt-wake.surfly.com"] {
  background:#333;
}

.header-connected[data-hostname="build.surfly.com"] {
  background:#b8662c;
}

.header-disconnected {
  background: #6f6f94;
}

.my-noty {
  padding: 10px;
  margin: 0 5px 5px;

  color: $light-color;
  background: $primary-color;
  border-left: 5px solid $primary-color-dark;

  & a {
    color: $light-color;
    text-decoration: underline;
  }

  &.warn {
    background: $warning-color;
    border-left-color: darken($warning-color, 10%);
  }

  &.error {
    background: $error-color;
    border-left-color: darken($error-color, 10%);
  }

  &.success {
    background: $success-color;
    border-left-color: darken($success-color, 10%);
  }
}
</style>
