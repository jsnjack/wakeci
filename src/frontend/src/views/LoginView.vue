<template>
    <form
        class="medium-height middle-align center-align"
        method="post"
        @submit.prevent="logIn"
    >
        <progress
            v-show="fetching"
            class="circle large"
        ></progress>
        <div
            v-if="!fetching"
            class="center-align"
        >
            <i class="extra">water</i>
            <h5 class="center-align">Welcome to wake</h5>
            <template v-if="ssoEnabled">
                <p>Sign in with your work account</p>
                <div class="space"></div>
                <a
                    class="button large round icon-label login-option"
                    :href="ssoLoginURL"
                    data-cy="sso-login"
                >
                    <i>account_circle</i>
                    <span>Sign in with SSO</span>
                </a>
                <div class="login-or">or</div>
                <!-- The password field only exists once asked for, so password
                     managers don't offer to fill it on every visit -->
                <button
                    v-if="!showPassword"
                    type="button"
                    class="border large round icon-label login-option"
                    data-cy="admin-login"
                    @click="revealPassword"
                >
                    <i>key</i>
                    <span>Sign in as admin</span>
                </button>
            </template>
            <p v-else>Start by logging in with your password</p>
            <template v-if="!ssoEnabled || showPassword">
                <div class="space"></div>
                <nav class="group connected">
                    <div class="max field border left-round prefix round">
                        <input
                            id="password"
                            ref="password"
                            v-model="password"
                            class="form-input text-center"
                            type="password"
                            aria-label="Admin password"
                        />
                    </div>
                    <button class="large right-round primary">Log in</button>
                </nav>
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
            showPassword: false,
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
        // Wait for both, so the page doesn't show the password field and then
        // swap it for single sign-on
        Promise.allSettled([this.fetch(), this.fetchAuthMethods()]).then(() => {
            this.fetching = false;
        });
    },
    methods: {
        fetch() {
            return axios
                .get("/auth/_isLoggedIn")
                .then(() => {
                    this.$store.commit("LOG_IN");
                    this.$router.replace(this.getRedirectURL);
                })
                .catch(() => {});
        },
        fetchAuthMethods() {
            // The page waits for this, so it can't wait forever: without an
            // answer it offers the password
            return axios
                .get("/auth/methods", { timeout: 5000 })
                .then((response) => {
                    this.ssoEnabled = response.data.sso === true;
                })
                .catch(() => {});
        },
        revealPassword() {
            this.showPassword = true;
            this.$nextTick(() => this.$refs.password.focus());
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
                .then(() => {
                    this.$store.commit("LOG_IN");
                    this.$router.replace(this.getRedirectURL);
                })
                .catch(() => {});
        },
    },
};
</script>

<!-- Add "scoped" attribute to limit CSS to this component only -->
<style scoped>
/* The button centres the icon but leaves the capital letters of the label 2px
   above the middle; lower the label so both sit on the button's middle */
.icon-label > span {
    transform: translateY(2px);
}
/* Single sign-on, "or" and the admin password line up in one column */
.login-option {
    box-sizing: border-box;
    width: 16rem;
}
/* A line on each side of "or", between single sign-on and the admin
   password. Not called a divider: BeerCSS styles [class*=divider] as a line. */
.login-or {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    width: 16rem;
    margin: 1.25rem auto;
    color: var(--outline);
}
.login-or::before,
.login-or::after {
    content: "";
    flex: 1;
    border-top: 1px solid var(--outline-variant);
}
</style>
