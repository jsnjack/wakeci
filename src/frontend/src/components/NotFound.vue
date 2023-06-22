<template>
    <div class="fill medium-height middle-align center-align">
        <div class="center-align">
            <i class="extra">celebration</i>
            <h5>Well, a fine little mess. Easy in, easy out. Another triumph.</h5>
            <p>...or simply put it, everything went hilariously sideways!</p>
            <nav class="center-align">
                <button
                    @click.prevent="back"
                    class="border round"
                >
                    Go back
                </button>
                <router-link
                    to="/"
                    class="button"
                    >Feed
                </router-link>
            </nav>
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
