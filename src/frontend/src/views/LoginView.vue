<template>
  <div
    class="container grid-xs"
    :class="{'loading loading-lg': fetching}"
  >
    <form
      v-show="!fetching"
      class="card"
      method="post"
      @submit.prevent="logIn"
    >
      <div class="card-header">
        <div class="card-title h5">
          Password
        </div>
      </div>
      <div class="card-body">
        <input
          id="password"
          v-model="password"
          class="form-input text-center"
          type="password"
        >
      </div>
      <div class="card-footer">
        <button
          type="submit"
          class="btn btn-primary"
        >
          Log in
        </button>
      </div>
    </form>
  </div>
</template>

<script>
import axios from "axios";
import vuex from "vuex";

export default {
    data: function() {
        return {
            fetching: true,
            password: "",
        };
    },
    computed: {
        ...vuex.mapState(["auth"]),
        getRedirectURL: function() {
            return this.$route.query.redirect || "/";
        },
    },
    mounted() {
        document.title = "Login - wakeci";
        this.fetch();
    },
    methods: {
        fetch() {
            axios
                .get("/auth/_isLoggedIn")
                .then((response) => {
                    this.$store.commit("LOG_IN");
                    this.$router.replace(this.getRedirectURL);
                    this.fetching = false;
                })
                .catch((error) => {
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
<style scoped lang="scss">
.card {
  margin-top: 1em;
}
.loading {
    min-height: 80vh;
}
</style>
