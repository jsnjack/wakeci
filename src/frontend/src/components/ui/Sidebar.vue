<template>
    <aside class="sidebar">
        <img :src="logo" alt="logo" class="logo"/>

        <nav class="sidebar-links">
            <router-link to="/" class="sidebar-link" exact-active-class="sidebar-link-active">
                <span class="material-icons">playlist_play</span>
                Feed
            </router-link>
            
            <router-link to="/jobs" class="sidebar-link" exact-active-class="sidebar-link-active">
                <span class="material-icons">build</span>
                Jobs
            </router-link>

            <router-link to="/settings" class="sidebar-link" exact-active-class="sidebar-link-active">
                <span class="material-icons">settings</span>
                Settings
            </router-link>

            <Toggle v-model="darkMode" />
        </nav>
    </aside>
</template>

<script>
import logo from '@/assets/WKCI.svg';
import Toggle from './Toggle';

export default {
    name: 'Sidebar',
    components: {
        Toggle,
    },
    data() {
        return {
            logo,
            darkMode: false,
        };
    },
    mounted() {
        this.darkMode = !JSON.stringify(localStorage.getItem('darkMode'));
    },
    watch: {
        darkMode() {
            document.querySelector('html').classList.toggle('dark');
            localStorage.setItem('darkMode', `${this.darkMode}`);
        },
    },
}
</script>

<style lang="scss" scoped>
.sidebar {
    @apply bg-secondary h-screen w-60 flex flex-col px-8 py-10;
    .logo {
        @apply text-center mb-8;
    }
    .sidebar-links {
        @apply flex flex-col gap-4;
        .sidebar-link {
            @apply text-2xl text-white hover:text-primary-light dark:hover:text-primary-light flex gap-3 items-center;
            &.sidebar-link-active {
                @apply text-primary-light;
            }
        }
    }
}

</style>