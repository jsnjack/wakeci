<template>
    <div
        class="row medium-padding round large-text feed-item"
        :data-cy-build="build.id"
    >
        <BuildStatus :status="build.status" />
        <div class="max">
            <a class="small-padding">
                <router-link :to="{ name: 'build', params: { id: build.id } }"> #{{ build.id }} {{ build.name }}</router-link>
            </a>
            <div>
                <ParamItem
                    v-for="(item, index) in this.getFilteredParams"
                    :key="index + 'param'"
                    :param="item"
                />
                <button
                    v-if="build.params && build.params.length > getFilteredParams.length"
                    class="circle transparent"
                    @click.prevent="toggleExpandParams"
                >
                    <i>expand_more</i>
                    <div class="tooltip bottom">More</div>
                </button>
                <button
                    v-if="expandParams"
                    class="circle transparent"
                    @click.prevent="toggleExpandParams"
                >
                    <i>expand_less</i>
                    <div class="tooltip bottom">Less</div>
                </button>
            </div>
        </div>
        <div>
            <SimpleDuration :item="build" />
            <SimpleStartedAgo :item="build" />
        </div>
        <!-- Open build view -->
        <router-link
            :to="{ name: 'build', params: { id: build.id } }"
            class="button circle transparent"
            data-cy="open-build-button"
        >
            <i>arrow_forward</i>
            <div class="tooltip bottom">Open</div>
        </router-link>

        <!-- Abort build -->
        <a
            :disabled="isDone"
            :href="getAbortURL"
            class="button circle transparent"
            data-cy="abort-build-button"
            @click.prevent="abort"
        >
            <i>stop</i>
            <div class="tooltip bottom">Abort</div>
        </a>

        <!-- Start build -->
        <a
            :disabled="build.status !== 'pending'"
            :href="getStartURL"
            class="button circle transparent"
            data-cy="start-build-button"
            @click.prevent="start"
        >
            <i>play_arrow</i>
            <div class="tooltip bottom">Start</div>
        </a>
    </div>
    <!-- TODO: Fix progress bar -->
    <BuildProgress
        v-if="!build.eta"
        :done="getDoneTasks"
        :total="getTotalTasks"
    />
    <BuildProgressETA
        v-if="build.eta"
        :eta="build.eta"
        :started-at="build.startedAt"
        :build-duration="build.duration"
    />
</template>

<script>
import BuildStatus from "@/components/BuildStatus.vue";
import BuildProgress from "@/components/BuildProgress.vue";
import BuildProgressETA from "@/components/BuildProgressETA.vue";
import SimpleDuration from "@/components/SimpleDuration.vue";
import SimpleStartedAgo from "@/components/SimpleStartedAgo.vue";
import ParamItem from "@/components/ParamItem.vue";
import axios from "axios";

const MAX_DEFAULT_NUMBER_OF_PARAMS = 3;

export default {
    components: {
        ParamItem,
        BuildStatus,
        BuildProgress,
        BuildProgressETA,
        SimpleStartedAgo,
        SimpleDuration,
    },
    props: {
        build: {
            type: Object,
            required: true,
        },
        paramsIndex: {
            type: Number,
            required: true,
        },
    },
    computed: {
        getMainTasks() {
            return this.build.tasks.filter((item) => {
                return item.kind === "main";
            });
        },
        getDoneTasks() {
            return this.getMainTasks.filter((item) => {
                return item.status !== "pending" && item.status !== "running";
            }).length;
        },
        getTotalTasks() {
            return this.getMainTasks.length;
        },
        getAbortURL: function () {
            return `/api/build/${this.build.id}/abort`;
        },
        getStartURL: function () {
            return `/api/build/${this.build.id}/start`;
        },
        isDone() {
            switch (this.build.status) {
                case "failed":
                case "finished":
                case "aborted":
                case "skipped":
                case "timed out":
                    return true;
            }
            return false;
        },
        getFilteredParams() {
            // User asked to show all params
            if (this.expandParams) {
                return this.build.params;
            }

            // Limit number of params and skip empty ones
            if (this.build.params) {
                return this.build.params
                    .filter((v) => {
                        return Object.values(v)[0] !== ""; // skip empty params
                    })
                    .slice(0, MAX_DEFAULT_NUMBER_OF_PARAMS);
            }
            return [];
        },
    },
    methods: {
        abort(event) {
            if (!this.isDone) {
                axios
                    .post(event.target.href || event.target.parentElement.href)
                    .then((response) => {
                        this.$notify({ text: `${this.build.id} has been aborted`, type: "success" });
                    })
                    .catch((error) => {});
            }
        },
        start(event) {
            if (this.build.status === "pending") {
                axios
                    .post(event.target.href || event.target.parentElement.href)
                    .then((response) => {
                        this.$notify({ text: `${this.build.id} has been started`, type: "success" });
                    })
                    .catch((error) => {});
            }
        },
        toggleExpandParams() {
            this.expandParams = !this.expandParams;
        },
    },
    data: function () {
        return {
            expandParams: false,
        };
    },
};
</script>

<!-- Add "scoped" attribute to limit CSS to this component only -->
<style scoped lang="scss">
.feed-item {
    background-color: var(--background);
}
</style>
