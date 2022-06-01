<template>
    <aside :class="['sidebar', { collapsed }]">
        <img :src="logo" alt="logo" class="logo" />

        <nav class="sidebar-links">
            <router-link to="/" class="sidebar-link" exact-active-class="sidebar-link-active">
                <span class="material-icons">playlist_play</span>
                <span class="sidebar-link-label">Feed</span>
            </router-link>

            <router-link to="/jobs" class="sidebar-link" exact-active-class="sidebar-link-active">
                <span class="material-icons">build</span>
                <span class="sidebar-link-label">Jobs</span>
            </router-link>

            <router-link
                to="/settings"
                class="sidebar-link"
                exact-active-class="sidebar-link-active"
            >
                <span class="material-icons">settings</span>
                <span class="sidebar-link-label">Settings</span>
            </router-link>

            <Toggle v-model="darkMode" />
        </nav>
    </aside>
</template>

<script>
import logo from '@/assets/WKCI.svg';
import Toggle from './Toggle.vue';

export default {
    name: 'Sidebar',
    components: {
        Toggle,
    },
    props: {
        collapsed: {
            type: Boolean,
            required: false,
            default: true,
        },
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
};
</script>

<style lang="scss" scoped>
.sidebar {
    @apply bg-secondary h-screen w-60 flex flex-col px-8 py-10;
    transition-property: width, display, margin;
    transition-duration: 0.2s;
    transition-timing-function: ease-in-out;
    &.collapsed {
        @apply w-6 items-center;
        .logo,
        .sidebar-link-label {
            @apply hidden;
        }
    }
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
