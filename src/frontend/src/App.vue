<template>
    <div id="app">
        <Sidebar v-if="auth.isLoggedIn" :collapsed="toggleSidebar" />
        <div class="content">
            <AppBar v-if="auth.isLoggedIn" @menu-clicked="toggleSidebar = !toggleSidebar" />

            <main class="main-content">
                <router-view />
            </main>

            <notifications classes="my-noty" position="bottom right" />
        </div>
    </div>
</template>

<script>
import vuex from 'vuex';
import axios from 'axios';
import { getWSURL } from '@/store/communication';
import wsMessageHandler from './store/communication';

import Sidebar from './components/ui/Sidebar.vue';
import AppBar from './components/ui/AppBar.vue';

export default {
    components: {
        Sidebar,
        AppBar,
    },
    data() {
        return {
            toggleSidebar: false,
        };
    },
    computed: {
        ...vuex.mapState(['ws', 'auth', 'durationMode']),
        getHeaderClass: function () {
            if (this.$store.state.ws.connected) {
                return 'header-connected';
            }
            return 'header-disconnected';
        },
        getHostname: function () {
            return location.hostname;
        },
    },
    mounted() {
        // Restore global duration mode
        if (localStorage.getItem('durationMode')) {
            this.$store.commit('TOGGLE_DURATION_MODE', localStorage.getItem('durationMode'));
        }
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
                ws.addEventListener('message', (event) => {
                    wsMessageHandler(this, event.data);
                });

                ws.addEventListener('close', (event) => {
                    this.$store.commit('WS_DISCONNECTED');
                    if (this.ws.failedAttempts >= this.ws.maxFailedAttempts) {
                        this.$store.commit('LOG_OUT');
                        this.$router.push('/login');
                    }
                    setTimeout(this.connect, this.$store.state.ws.reconnectTimeout);
                });

                ws.addEventListener('error', (event) => {
                    console.warn('WS connection error', event);
                });

                ws.addEventListener('open', (event) => {
                    this.$store.commit('WS_CONNECTED', ws);
                });
            } else {
                console.error('WS already connected');
            }
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
                    ws.addEventListener('message', (event) => {
                        wsMessageHandler(this, event.data);
                    });

                    ws.addEventListener('close', (event) => {
                        this.$store.commit('WS_DISCONNECTED');
                        if (this.ws.failedAttempts >= this.ws.maxFailedAttempts) {
                            this.$store.commit('LOG_OUT');
                            this.$router.push('/login');
                        }
                        setTimeout(this.connect, this.$store.state.ws.reconnectTimeout);
                    });

                    ws.addEventListener('error', (event) => {
                        console.warn('WS connection error', event);
                    });

                    ws.addEventListener('open', (event) => {
                        this.$store.commit('WS_CONNECTED', ws);
                    });
                } else {
                    console.error('WS already connected');
                }
            },
            logOut: function () {
                axios
                    .get('/auth/logout')
                    .then((response) => {
                        this.$store.commit('LOG_OUT');
                        this.$router.push('/login');
                    })
                    .catch((error) => {});
            },
        },
    },
};
</script>

<style lang="scss">
@import '@/assets/wakeci.scss';

#app {
    @apply bg-gray-extra-light dark:bg-secondary-dark flex flex-nowrap;
    & .content {
        @apply flex-1;
    }
    .main-content {
        @apply p-6;
    }
}

.header-connected {
    @apply bg-primary;
}

.header-connected[data-hostname='mrt-wake.surfly.com'] {
    background: #333;
}

.header-connected[data-hostname='build.surfly.com'] {
    background: #b8662c;
}

.header-disconnected {
    background: #6f6f94;
}
</style>
