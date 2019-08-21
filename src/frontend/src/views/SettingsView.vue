<template>
  <div class="container grid-sm">
    <form class="card" method="post" @submit.prevent="save">
      <div class="card-header">
        <div class="card-title h5">Settings</div>
      </div>
      <div class="card-body text-left">
        <div class="form-group">
          <label class="form-label" for="password">Password</label>
          <input class="form-input" type="password" id="password" v-model="password" />
        </div>
        <div class="form-group">
          <label class="form-label" for="concurrent-builds">Number of concurrent builds</label>
          <input class="form-input" type="number" min="1" id="concurrent-builds" v-model="concurrentBuilds" />
        </div>
        <div class="form-group">
          <label class="form-label" for="build-history-size">Number of builds to preserve</label>
          <input class="form-input" type="number" min="1" id="build-history-size" v-model="buildHistorySize" />
        </div>
      </div>
      <div class="card-footer">
        <button type="submit" class="btn btn-primary">Save</button>
      </div>
    </form>
  </div>
</template>

<script>
import axios from "axios";

export default {
    mounted() {
        document.title = "Settings - wakeci";
        this.fetch();
    },
    methods: {
        save() {
            const data = new FormData();
            data.append("password", this.password);
            data.append("concurrentBuilds", this.concurrentBuilds);
            data.append("buildHistorySize", this.buildHistorySize);
            axios
                .post("/api/settings/", data, {
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
                .catch((error) => {});
        },
        fetch() {
            axios.get("/api/settings/")
                .then((response) => {
                    if (response.data.concurrentBuilds) {
                        this.concurrentBuilds = response.data.concurrentBuilds;
                    }
                    if (response.data.buildHistorySize) {
                        this.buildHistorySize = response.data.buildHistorySize;
                    }
                })
                .catch((error) => {});
        },
    },
    data: function() {
        return {
            password: "",
            concurrentBuilds: 2,
            buildHistorySize: 200,
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
