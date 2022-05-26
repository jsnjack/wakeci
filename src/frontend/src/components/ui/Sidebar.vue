<template>
    <aside class="sidebar">
        <img :src="logo" alt="logo" class="logo"/>

        <nav class="sidebar-links">
            <router-link to="/" class="sidebar-link" exact-active-class="sidebar-link-active">
                Feed
            </router-link>
            
            <router-link to="/jobs" class="sidebar-link" exact-active-class="sidebar-link-active">
                Jobs
            </router-link>

            <router-link to="/settings" class="sidebar-link" exact-active-class="sidebar-link-active">
                Settings
            </router-link>
        </nav>
    </aside>
</template>

<script>
import logo from '@/assets/WKCI.svg';

export default {
    name: 'Sidebar',
    data() {
        return {
            logo,
        };
    },
}
</script>

<style lang="scss" scoped>
.sidebar {
    @apply bg-secondary h-screen w-60 flex flex-col px-8 py-10;
    .logo {
        @apply text-center mb-6;
    }
    .sidebar-links {
        @apply flex flex-col gap-4;
        .sidebar-link {
            @apply text-2xl text-white hover:text-primary-light dark:hover:text-primary-light;
            &.sidebar-link-active {
                @apply text-primary-light;
            }
        }
    }
}

</style>