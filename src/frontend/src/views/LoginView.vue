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
            <p v-if="ssoEnabled">Log in with your password or single sign-on</p>
            <p v-else>Start by logging in with your password</p>
            <div class="space"></div>
            <nav class="group connected">
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
            <template v-if="ssoEnabled">
                <div class="space"></div>
                <a
                    class="button large border round sso-login"
                    :href="ssoLoginURL"
                    data-cy="sso-login"
                >
                    <i>account_circle</i>
                    <span>Sign in with SSO</span>
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
            ssoEnabled: false,
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
        ssoLoginURL: function () {
            return "/auth/sso/login?redirect=" + encodeURIComponent(this.getRedirectURL);
        },
    },
    mounted() {
        this.$store.commit("SET_CURRENT_PAGE", "Login");
        this.showLoginError();
        this.fetch();
        this.fetchAuthMethods();
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
        fetchAuthMethods() {
            axios
                .get("/auth/methods")
                .then((response) => {
                    this.ssoEnabled = response.data.sso === true;
                })
                .catch((error) => {});
        },
        showLoginError() {
            const messages = {
                sso: "Single sign-on failed, please try again",
                sso_not_allowed: "Your account is not in a group that is allowed to log in",
            };
            const { error, ...query } = this.$route.query;
            if (error === undefined) {
                return;
            }
            // Drop the error from the URL so a reload doesn't show it again
            this.$router.replace({ query });
            if (Object.hasOwn(messages, error)) {
                this.$notify({
                    text: messages[error],
                    type: "error",
                });
            }
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
<style scoped>
/* The button centres the icon but leaves the capital letters of the label 2px
   above the middle; lower the label so both sit on the button's middle */
.sso-login > span {
    transform: translateY(2px);
}
</style>
