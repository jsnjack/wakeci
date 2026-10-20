<template>
    <form
        class="medium-height middle-align center-align"
        method="post"
        @submit.prevent="logIn"
    >
    <progress class="circle large" v-show="fetching"></progress>
        <div
            class="center-align"
            v-show="!fetching && methodsLoaded"
        >
            <i class="extra">water</i>
            <h5 class="center-align">Welcome to wake</h5>
            <p
                v-if="loginError"
                class="error-text"
                data-cy="login-error"
            >
                {{ loginError }}
            </p>
            <p v-if="passwordEnabled">Start by logging in with your password</p>
            <p v-else>Sign in with your company Google account</p>
            <div class="space"></div>
            <nav
                v-if="passwordEnabled"
                class="group connected"
            >
                <div class="max field border left-round prefix round">
                    <input
                        id="password"
                        v-model="password"
                        class="form-input text-center"
                        type="password"
                    />
                </div>
                <button class="large right-round primary">Log in</button>
            </nav>
            <!-- google.disable_password turns the password off server-side -->
            <template v-if="googleEnabled">
                <p v-if="passwordEnabled">or</p>
                <a
                    class="button large"
                    :class="{ border: passwordEnabled }"
                    data-cy="google-login"
                    :href="googleLoginURL"
                >
                    <i>account_circle</i>
                    <span>Sign in with Google</span>
                </a>
            </template>
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
            passwordEnabled: true,
            googleEnabled: false,
            methodsLoaded: false,
        };
    },
    computed: {
        ...vuex.mapState(["auth", "currentPage"]),
        getRedirectURL: function () {
            const redirect = this.$route.query.redirect;
            if (typeof redirect !== "string" || !redirect.startsWith("/") || redirect.startsWith("//") || redirect.includes("\\")) {
                return "/";
            }
            return redirect;
        },
        googleLoginURL: function () {
            return "/auth/google/login?redirect=" + encodeURIComponent(this.getRedirectURL);
        },
        loginError: function () {
            switch (this.$route.query.error) {
                case "google_not_allowed":
                    return "Sign in with your company Google Workspace account";
                case "google_failed":
                    return "Google sign-in failed, please try again";
                default:
                    return "";
            }
        },
    },
    mounted() {
        this.$store.commit("SET_CURRENT_PAGE", "Login");
        this.fetch();
        this.fetchMethods();
    },
    methods: {
        fetchMethods() {
            axios
                .get("/auth/methods")
                .then((response) => {
                    this.passwordEnabled = response.data.password !== false;
                    this.googleEnabled = response.data.google === true;
                })
                .catch((error) => {})
                .finally(() => {
                    this.methodsLoaded = true;
                });
        },
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
