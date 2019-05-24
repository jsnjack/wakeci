<template>
  <div id="app">
    <header class="navbar-center">
        <section class="navbar-section">
            <router-link to="/" class="btn btn-link">Feed</router-link>
            <router-link to="/jobs" class="btn btn-link">History</router-link>
            <router-link to="/jobs" class="btn btn-link">Jobs</router-link>
            <router-link to="/jobs" class="btn btn-link">Settings</router-link>
        </section>
    </header>
    <router-view/>
  </div>
</template>

<script>
import vuex from "vuex";
import wsMessageHandler from "./store/communication";

export default {
    mounted() {
        this.connect();
    },
    computed: {
        ...vuex.mapState([]),
    },
    methods: {
        connect: function() {
            if (!this.$store.state.ws.connected) {
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
    },
};
</script>

<style lang="scss">
@import "@/assets/npci.scss";

#app {
  font-family: 'Avenir', Helvetica, Arial, sans-serif;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
  text-align: center;
  color: #2c3e50;
}

.container {
    display: flex;
    justify-content: space-evenly;
}
</style>
