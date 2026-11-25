<template>
    <div
        class="ui not-found stack gap-3"
        data-cy="not-found"
    >
        <i class="icon large">search_off</i>
        <h1>{{ title }}</h1>
        <p class="muted">{{ detail }}</p>
        <div class="cluster gap-2">
            <button
                type="button"
                class="btn btn-outline btn-ghost"
                @click="back"
            >
                Go back
            </button>
            <router-link
                to="/"
                class="btn btn-primary"
                >Open the feed</router-link
            >
        </div>
    </div>
</template>

<script>
// NotFound says that what the address names isn't there: a page, or a build
// that cleanup removed
export default {
    props: {
        title: {
            type: String,
            default: "There's no such page",
        },
        detail: {
            type: String,
            default: "The address may be mistyped, or what it showed has been removed.",
        },
    },
    mounted() {
        if (this.$route.name === "notFound") {
            this.$store.commit("SET_CURRENT_PAGE", "Not found");
        }
    },
    methods: {
        back() {
            this.$router.go(-1);
        },
    },
};
</script>

<style scoped>
.not-found {
    align-items: center;
    max-inline-size: 36rem;
    margin: var(--space-8) auto;
    text-align: center;
}

.not-found .icon {
    color: var(--on-surface-variant);
}
</style>
