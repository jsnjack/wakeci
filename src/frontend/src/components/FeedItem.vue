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
            <a class="duration-block">
                <i class="small small-padding">avg_time</i>
                <SimpleDuration :item="build" />
            </a>
            <div></div>
            <a class="duration-block">
                <i class="small small-padding">schedule</i>
                <SimpleStartedAgo :item="build" />
            </a>
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
    <!-- <tr :data-cy-build="build.id">
        <td>
            <router-link :to="{ name: 'build', params: { id: build.id } }">
                {{ build.id }}
            </router-link>
        </td>
        <td>
            <div class="cell-name">
                {{ build.name }}
            </div>
        </td>
        <td class="hide-xs hide-sm">
            <div
                v-show="build.params"
                class="label param tooltip"
                :data-tooltip="getParamsTooltip"
                data-cy="params-text"
            >
                {{ getParamsText }}
            </div>
        </td>
        <td class="hide-xs hide-sm hide-md">
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
        </td>
        <td>
            <BuildStatus :status="build.status" />
        </td>
        <td class="hide-xs">
            <DurationElement
                v-show="build.status !== 'pending'"
                :item="build"
                :use-global-duration-mode-state="true"
            />
        </td>
        <td class="actions">
            <div class="btn-group">
                <router-link
                    :to="{ name: 'build', params: { id: build.id } }"
                    class="btn btn-primary"
                    data-cy="open-build-button"
                >
                    Open
                </router-link>
                <a
                    v-if="!isDone"
                    :href="getAbortURL"
                    class="btn btn-error btn-action"
                    data-cy="abort-build-button"
                    data-tooltip="Abort build"
                    @click.prevent="abort"
                    ><i class="icon icon-cross"
                /></a>
                <a
                    v-if="build.status === 'pending'"
                    :href="getStartURL"
                    class="btn btn-action tooltip tooltip-bottom"
                    data-cy="start-build-button"
                    data-tooltip="Start now"
                    @click.prevent="start"
                    ><i class="icon icon-forward"
                /></a>
            </div>
        </td>
    </tr> -->
</template>

<script>
import BuildStatus from "@/components/BuildStatus.vue";
import BuildProgress from "@/components/BuildProgress.vue";
import BuildProgressETA from "@/components/BuildProgressETA.vue";
import DurationElement from "@/components/DurationElement.vue";
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
        DurationElement,
        SimpleDuration,
        SimpleStartedAgo,
        SimpleDuration,
        SimpleStartedAgo,
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
        getParamsText() {
            if (this.build.params) {
                let index = 0;
                if (this.build.params.length > this.paramsIndex) {
                    index = this.paramsIndex;
                }
                return Object.values(this.build.params[index])[0].substring(0, 20);
            }
            return "";
        },
        getParamsTooltip() {
            if (this.build.params) {
                return this.build.params.map((v) => v[Object.keys(v)[0]]).join("\n");
            }
            return "";
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
.duration-block {
    cursor: default;
}
.feed-item {
    background-color: var(--background);
}
</style>
