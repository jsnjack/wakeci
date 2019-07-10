<template>
  <div id="app">
    <header class="navbar-center bg-primary">
        <section class="navbar-section">
            <router-link to="/" class="btn btn-link text-light">Feed</router-link>
            <router-link to="/jobs" class="btn btn-link text-light">Jobs</router-link>
            <router-link to="/jobs" class="btn btn-link text-light">Settings</router-link>
            <a href="#" @click.prevent="logOut" class="btn btn-link text-light">Log out</a>
        </section>
    </header>
    <router-view/>
    <notifications classes="my-noty" position="bottom right" />
  </div>
</template>

<script>
import vuex from "vuex";
import axios from "axios";
import {AUTHURL} from "@/store/communication";
import wsMessageHandler from "./store/communication";

export default {
    mounted() {
        this.connect();
    },
    computed: {
        ...vuex.mapState(["ws", "auth"]),
    },
    methods: {
        connect: function() {
            if (!this.$store.state.ws.connected) {
                if (!this.auth.isLoggedIn) {
                    setTimeout(this.connect, this.$store.state.ws.reconnectTimeout);
                    return;
                }
                const ws = new WebSocket(this.$store.state.ws.url);
                ws.sendMessage = function(obj) {
                    ws.send(JSON.stringify(obj));
                };
                // Listen for messages
                ws.addEventListener("message", (event) => {
                    wsMessageHandler(this, event.data);
                });

                ws.addEventListener("close", (event) => {
                    this.$store.commit("WS_DISCONNECTED");
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
                .get(AUTHURL + "/logout/")
                .then((response) => {
                    this.$store.commit("LOG_OUT");
                    this.$router.push("/login");
                    this.ws.obj.close();
                })
                .catch((error) => {
                    this.$notify({
                        text: error,
                        type: "error",
                    });
                });
        },
    },
};
</script>

<style lang="scss">
@import "@/assets/wakeci.scss";

#app {
  font-family: 'Avenir', Helvetica, Arial, sans-serif;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
  text-align: center;
  color: #2c3e50;
}

.my-noty {
  padding: 10px;
  margin: 0 5px 5px;

  color: $light-color;
  background: $primary-color;
  border-left: 5px solid $primary-color-dark;

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
