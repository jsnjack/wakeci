<template>
    <div class="ui not-found stack gap-3">
        <i class="icon large">celebration</i>
        <h1>Well, a fine little mess. Easy in, easy out. Another triumph.</h1>
        <p class="muted">...or simply put it, everything went hilariously sideways!</p>
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
                >Feed</router-link
            >
        </div>
    </div>
</template>

<script>
export default {
    methods: {
        back() {
            this.$router.go(-1);
        },
    },
};
</script>

<style scoped></style>
