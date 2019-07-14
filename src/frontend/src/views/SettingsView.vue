<template>
  <div class="container">
    <form class="card col-6 col-mx-auto" method="post" @submit.prevent="save">
      <div class="card-header">
        <div class="card-title h5">Settings</div>
      </div>
      <div class="card-body text-left">
        <div class="form-group">
          <label class="form-label" for="password">Password</label>
          <input class="form-input" type="password" id="password" v-model="password" />
        </div>
      </div>
      <div class="card-footer">
        <button type="submit" class="btn btn-primary">Save</button>
      </div>
    </form>
  </div>
</template>

<script>
import {APIURL} from "@/store/communication";
import axios from "axios";

export default {
    methods: {
        save() {
            const data = new FormData();
            data.append("password", this.password);
            axios
                .post(APIURL + "/settings/", data, {
                    headers: {
                        "Content-type": "application/x-www-form-urlencoded",
                    },
                })
                .then((response) => {
                    this.$notify({
                        text: "Saved",
                        type: "success",
                    });
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
