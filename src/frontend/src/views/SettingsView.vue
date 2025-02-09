<template>
    <div>
        <form
            method="post"
            @submit.prevent="save"
        >
            <div class="field border small">
                <input
                    type="password"
                    id="password"
                    v-model="password"
                />
                <span class="helper">Password</span>
            </div>

            <div class="field border small">
                <input
                    type="number"
                    id="concurrent-builds"
                    min="1"
                    v-model="concurrentBuilds"
                />
                <span class="helper">Number of concurrent builds</span>
            </div>

            <div class="field border small">
                <input
                    id="build-history-size"
                    v-model="buildHistorySize"
                    type="number"
                    min="1"
                />
                <span class="helper">Number of builds to preserve</span>
            </div>

            <nav class="no-space">
                <div class="max"></div>
                <button
                    data-cy="save-settings"
                    type="submit"
                >
                    Save
                </button>
            </nav>
        </form>
    </div>
</template>

<script>
import axios from "axios";

export default {
    data: function () {
        return {
            password: "",
            concurrentBuilds: 2,
            buildHistorySize: 200,
        };
    },
    mounted() {
        this.$store.commit("SET_CURRENT_PAGE", "Settings");
        this.fetch();
    },
    methods: {
        save() {
            const data = new FormData();
            data.append("password", this.password);
            data.append("concurrentBuilds", this.concurrentBuilds);
            data.append("buildHistorySize", this.buildHistorySize);
            axios
                .post("/api/settings", data, {
                    headers: {
                        "Content-type": "application/x-www-form-urlencoded",
                    },
                })
                .then((response) => {
                    this.$notify({
                        text: "Saved",
                        type: "primary",
                    });
                })
                .catch((error) => {});
        },
        fetch() {
            axios
                .get("/api/settings")
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
};
</script>

<!-- Add "scoped" attribute to limit CSS to this component only -->
<style scoped ></style>
