<template>
  <div class="container">
    <form class="card col-4 col-mx-auto" method="post" @submit.prevent="logIn">
      <div class="card-header">
        <div class="card-title h5">Password</div>
      </div>
      <div class="card-body">
        <input class="form-input text-center" type="password" id="password" v-model="password" />
      </div>
      <div class="card-footer">
        <button type="submit" class="btn btn-primary">Log in</button>
      </div>
    </form>
  </div>
</template>

<script>
import {AUTHURL} from "@/store/communication";
import axios from "axios";
import vuex from "vuex";

export default {
    mounted() {
        this.fetch();
    },
    computed: {
        ...vuex.mapState(["auth"]),
    },
    methods: {
        fetch() {
            axios
                .get(AUTHURL + "/_isLoggedIn/")
                .then((response) => {
                    this.$store.commit("LOG_IN");
                    this.$router.push("/");
                })
                .catch((error) => {});
        },
        logIn() {
            const data = new FormData();
            data.append("password", this.password);
            axios
                .post(AUTHURL + "/login/", data, {
                    headers: {
                        "Content-type": "application/x-www-form-urlencoded",
                    },
                })
                .then((response) => {
                    this.$store.commit("LOG_IN");
                    this.$router.push("/");
                })
                .catch((error) => {
                    this.$notify({
                        text: error,
                        type: "error",
                    });
                });
        },
    },
    data: function() {
        return {
            password: "",
        };
    },
};
</script>

<!-- Add "scoped" attribute to limit CSS to this component only -->
<style scoped lang="scss">
.card {
  margin-top: 1em;
}
</style>
