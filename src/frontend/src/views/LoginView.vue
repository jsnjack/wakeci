<template>
    <Card :class="['login-card', { 'loading loading-lg': fetching }]">
        <img class="logo" :src="logo" />
        <form v-show="!fetching" method="post" @submit.prevent="logIn">
            <div class="form-item">
                <label>Password</label>
                <input
                    id="password"
                    v-model="password"
                    class="form-input text-center"
                    type="password"
                />
            </div>
            <button type="submit" class="btn btn-primary login-btn">Log in</button>
        </form>
    </Card>
</template>

<script>
import axios from 'axios';
import vuex from 'vuex';
import Card from '@/components/ui/Card.vue';
import logo from '@/assets/WKCI.svg';

export default {
    components: {
        Card,
    },
    data: function () {
        return {
            fetching: true,
            password: '',
            logo,
        };
    },
    computed: {
        ...vuex.mapState(['auth']),
        getRedirectURL: function () {
            return this.$route.query.redirect || '/';
        },
    },
    mounted() {
        document.title = 'Login - wakeci';
        this.fetch();
    },
    methods: {
        fetch() {
            axios
                .get('/auth/_isLoggedIn')
                .then((response) => {
                    this.$store.commit('LOG_IN');
                    this.$router.replace(this.getRedirectURL);
                })
                .catch((error) => {})
                .finally(() => {
                    this.fetching = false;
                });
        },
        logIn() {
            const data = new FormData();
            if (this.password !== '') {
                data.append('password', this.password);
            }
            axios
                .post('/auth/login', data, {
                    headers: {
                        'Content-type': 'application/x-www-form-urlencoded',
                    },
                })
                .then((response) => {
                    this.$store.commit('LOG_IN');
                    this.$router.replace(this.getRedirectURL);
                })
                .catch((error) => {});
        },
    },
};
</script>

<!-- Add "scoped" attribute to limit CSS to this component only -->
<style scoped lang="scss">
.login-card {
    @apply max-w-md mx-auto mt-8 dark:bg-secondary ring-1 ring-primary-light;
    .logo {
        @apply mx-auto my-4;
    }
    .login-btn {
        @apply ml-auto;
    }
}
</style>
