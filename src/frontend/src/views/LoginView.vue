<template>
    <form
        class="ui login-page"
        method="post"
        @submit.prevent="logIn"
    >
        <span
            v-show="fetching"
            class="status-ring login-spinner"
            role="progressbar"
            aria-label="Loading"
            data-cy="login-progress"
        ></span>
        <div
            v-if="!fetching"
            class="login-card stack gap-4"
        >
            <i class="icon login-logo">water</i>
            <h1>Welcome to wake</h1>
            <template v-if="ssoEnabled">
                <p class="muted">Sign in with your work account</p>
                <a
                    class="btn btn-primary login-option"
                    :href="ssoLoginURL"
                    data-cy="sso-login"
                >
                    <i class="icon small">account_circle</i>
                    <span>Sign in with SSO</span>
                </a>
                <div class="login-or">or</div>
                <!-- The password field only exists once asked for, so password
                     managers don't offer to fill it on every visit -->
                <button
                    v-if="!showPassword"
                    type="button"
                    class="btn btn-outline btn-ghost login-option"
                    data-cy="admin-login"
                    @click="revealPassword"
                >
                    <i class="icon small">key</i>
                    <span>Sign in as admin</span>
                </button>
            </template>
            <p
                v-else
                class="muted"
            >
                Start by logging in with your password
            </p>
            <div
                v-if="!ssoEnabled || showPassword"
                class="cluster nowrap gap-2"
            >
                <input
                    id="password"
                    ref="password"
                    v-model="password"
                    class="input grow"
                    type="password"
                    aria-label="Admin password"
                    placeholder="Admin password"
                />
                <button
                    type="submit"
                    class="btn btn-primary"
                >
                    Log in
                </button>
            </div>
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
                .then((response) => {
                    this.$store.commit("LOG_IN", response.data);
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
                // The session's permissions come with the login status
                .then(() => this.fetch())
                .catch(() => {});
        },
    },
};
</script>

<!-- Add "scoped" attribute to limit CSS to this component only -->
<style scoped>
.login-page {
    display: grid;
    place-items: center;
    min-block-size: 60vh;
}

.login-spinner {
    inline-size: 40px;
    block-size: 40px;
    --_c: var(--primary);
}

.login-card {
    inline-size: min(22rem, 100%);
    align-items: center;
    text-align: center;
}

.login-logo {
    font-size: 56px;
    color: var(--primary);
}

.login-option {
    inline-size: 100%;
    block-size: 40px;
}

.login-card .cluster {
    inline-size: 100%;
}

/* "or" between the ways to sign in, with a line on each side */
.login-or {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    inline-size: 100%;
    color: var(--on-surface-variant);
}

.login-or::before,
.login-or::after {
    content: "";
    flex: 1;
    border-block-start: 1px solid var(--outline-variant);
}
</style>
