<template>
    <form
        class="medium-height middle-align center-align"
        method="post"
        @submit.prevent="logIn"
    >
    <progress class="circle large" v-show="fetching"></progress>
        <div
            class="center-align"
            v-show="!fetching"
        >
            <i class="extra">water</i>
            <h5 class="center-align">Welcome to wake</h5>
            <p>Start by logging in with your password</p>
            <div class="space"></div>
            <nav class="no-space">
                <div class="max field border left-round">
                    <input
                        id="password"
                        v-model="password"
                        class="form-input text-center"
                        type="password"
                    />
                </div>
                <button class="large right-round">Log in</button>
            </nav>
        </div>
    </form>
</template>

<script>
import axios from "axios";
import vuex from "vuex";

export default {
    data: function () {
        return {
            fetching: true,
            password: "",
        };
    },
    computed: {
        ...vuex.mapState(["auth", "currentPage"]),
        getRedirectURL: function () {
            return this.$route.query.redirect || "/";
        },
    },
    mounted() {
        this.$store.commit("SET_CURRENT_PAGE", "Login");
        this.fetch();
    },
    methods: {
        fetch() {
            axios
                .get("/auth/_isLoggedIn")
                .then((response) => {
                    this.$store.commit("LOG_IN");
                    this.$router.replace(this.getRedirectURL);
                })
                .catch((error) => {})
                .finally(() => {
                    this.fetching = false;
                });
        },
        logIn() {
            const data = new FormData();
            if (this.password !== "") {
                data.append("password", this.password);
            }
            axios
                .post("/auth/login", data, {
                    headers: {
                        "Content-type": "application/x-www-form-urlencoded",
                    },
                })
                .then((response) => {
                    this.$store.commit("LOG_IN");
                    this.$router.replace(this.getRedirectURL);
                })
                .catch((error) => {});
        },
    },
};
</script>

<!-- Add "scoped" attribute to limit CSS to this component only -->
<style scoped ></style>
