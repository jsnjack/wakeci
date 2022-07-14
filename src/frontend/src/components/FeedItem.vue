<template>
    <tr :data-cy-build="build.id">
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
    </tr>
</template>

<script>
import BuildStatus from "@/components/BuildStatus.vue";
import BuildProgress from "@/components/BuildProgress.vue";
import BuildProgressETA from "@/components/BuildProgressETA.vue";
import DurationElement from "@/components/DurationElement.vue";
import axios from "axios";

export default {
    components: { BuildStatus, BuildProgress, BuildProgressETA, DurationElement },
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
    },
    methods: {
        abort(event) {
            axios
                .post(event.target.href || event.target.parentElement.href)
                .then((response) => {
                    this.$notify({ text: `${this.build.id} has been aborted`, type: "success" });
                })
                .catch((error) => {});
        },
        start(event) {
            axios
                .post(event.target.href || event.target.parentElement.href)
                .then((response) => {
                    this.$notify({ text: `${this.build.id} has been started`, type: "success" });
                })
                .catch((error) => {});
        },
    },
};
</script>

<!-- Add "scoped" attribute to limit CSS to this component only -->
<style scoped lang="scss">
.param {
    margin: 0.25em;
}
.param:hover {
    cursor: default;
}
.cell-name {
    max-width: 20ch;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}
@media (max-width: 480px) {
    .cell-name {
        max-width: 15ch;
    }
}
</style>
