<template>
    <header class="app-bar">
        <span class="material-icons menu-icon" @click="$emit('menu-clicked')">menu</span>

        <div class="right-side">
            <span class="version">v{{ getVesion }}</span>

            <span class="material-icons cursor-pointer" title="Log out" @click="logOut"
                >exit_to_app</span
            >
        </div>
    </header>
</template>

<script>
import axios from 'axios';

export default {
    name: 'AppBar',
    computed: {
        getVesion: function () {
            return import.meta.env.VITE_VERSION || '0.0.0';
        },
    },
    methods: {
        logOut() {
            axios
                .get('/auth/logout')
                .then((response) => {
                    this.$store.commit('LOG_OUT');
                    this.$router.push('/login');
                })
                .catch((error) => {});
        },
    },
};
</script>

<style lang="scss" scoped>
.app-bar {
    @apply h-12 bg-gray-light dark:bg-primary dark:text-primary text-white flex items-center justify-between py-2 px-6;
    .menu-icon {
        @apply cursor-pointer text-secondary hover:text-secondary-dark dark:text-white dark:hover:text-primary-light;
    }
    .right-side {
        @apply flex items-center text-secondary gap-2 dark:text-white;
        .version {
            @apply text-xs hidden md:block;
        }
    }
}
</style>
