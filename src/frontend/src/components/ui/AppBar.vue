<template>
    <header class="app-bar">
        <div class="right-side">
            <MoreOptions data-cy="app-submenu" :optionsList="moreOptions" :showSearch="false" />
        </div>
    </header>
</template>

<script>
import axios from "axios";
import MoreOptions from "./MoreOptions.vue";

export default {
    name: "AppBar",
    components: {
        MoreOptions,
    },
    data() {
        return {
            moreOptions: null,
            darkMode: false,
        };
    },
    computed: {
        getVersion: function () {
            return import.meta.env.VITE_VERSION || "0.0.0";
        },
    },
    created() {
        this.moreOptions = [
            {
                name: "Settings",
                icon: "settings",
                onClick: () => {
                    this.$router.push({ name: "settings" });
                },
                attrs: {
                    "data-cy": "settings-link",
                },
            },
            {
                name: "Log out",
                icon: "exit_to_app",
                onClick: this.logOut,
                attrs: {
                    "data-cy": "logout",
                },
            },
            {
                name: this.darkMode ? `Light Mode` : `Dark Mode`,
                icon: this.darkMode ? "wb_sunny" : "brightness_3",
                onClick: () => (this.darkMode = !this.darkMode),
            },
            {
                name: `Version ${this.getVersion}`,
                disabled: true,
            },
        ];
        this.darkMode = JSON.parse(localStorage.getItem("darkMode")) || false;
    },
    methods: {
        logOut() {
            axios
                .get("/auth/logout")
                .then((response) => {
                    this.$store.commit("LOG_OUT");
                    this.$router.push("/login");
                })
                .catch((error) => {});
        },
    },
    watch: {
        darkMode(val) {
            document.querySelector("html").classList.toggle("dark");
            localStorage.setItem("darkMode", `${val}`);
            // Items on MoreOptions are not reactive
            this.moreOptions[1].name = val ? `Light Mode` : `Dark Mode`;
            this.moreOptions[1].icon = val ? "wb_sunny" : "brightness_3";
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
