<template>
    <div class="medium-margin"></div>
    <div>
        <nav @click="toggleLogs">
            <a class="row wave max">
                <i v-if="this.content === '' && this.task.status !== 'running'">chevron_right</i>
                <i v-else>expand_more</i>
                <BuildStatus :status="task.status" />
                <div class="max large-text">{{ name }}</div>
                <SimpleDuration :item="task" />
            </a>
            <a
                @click="reloadLogs"
                class="button circle transparent"
            >
                <i>sync</i>
            </a>
            <a
                class="button circle transparent"
                :href="getLogURL"
                target="_blank"
            >
                <i>open_in_new</i>
            </a>
        </nav>
        <article class="log-container no-padding">
            <pre
                v-if="content"
                class="log-line fill large-padding no-round"
                >{{ content }}</pre
            >
            <TextSpinner v-show="task.status === 'running'" />
        </article>
    </div>
</template>

<script>
import BuildStatus from "@/components/BuildStatus.vue";
import TextSpinner from "@/components/TextSpinner.vue";
import DurationElement from "@/components/DurationElement.vue";
import SimpleDuration from "@/components/SimpleDuration.vue";
import axios from "axios";

const FlushContentPeriod = 500;

export default {
    components: { BuildStatus, DurationElement, TextSpinner, SimpleDuration },
    props: {
        buildID: {
            type: Number,
            required: true,
        },
        task: {
            type: Object,
            required: true,
        },
        name: {
            type: String,
            required: true,
        },
        follow: {
            type: Boolean,
            required: true,
        },
    },
    data: function () {
        return {
            cachedContent: "",
            content: "",
            flushInterval: null,
        };
    },
    computed: {
        getDividerText: function () {
            return `task #${this.task.id} | ${this.task.kind}`;
        },
        getCyText: function () {
            return `task_section_${this.task.id}`;
        },
        isVisible: function () {
            // Show only "main" and "finally" tasks or tasks that were started. For example,
            // there is no need to show "finished" tasks if build failed because
            // they won't be executed anyway
            if (this.task.kind === "main" || this.task.kind === "finally") {
                return true;
            }
            return !(this.task.startedAt && this.task.startedAt.indexOf("0001-") === 0);
        },
        getBorderClass() {
            return `border-${this.task.status}`.replaceAll(" ", "");
        },
        getLogURL() {
            return `/storage/build/${this.buildID}/task_${this.task.id}.log`;
        },
        getFlushURL() {
            return `/api/build/${this.buildID}/flush`;
        },
    },
    watch: {
        "task.status": "onStatusChange",
    },
    mounted() {
        this.emitter.on(`build:log:${this.buildID}:task-${this.task.id}`, this.addLog);
        this.onStatusChange(this.task.status);
    },
    unmounted() {
        this.emitter.off(`build:log:${this.buildID}:task-${this.task.id}`, this.addLog);
    },
    beforeUnmount: function () {
        clearInterval(this.flushInterval);
    },
    methods: {
        flushLogs() {
            axios
                .post(this.getFlushURL)
                .then((response) => {
                    this._reloadLogs();
                })
                .catch((error) => {});
        },
        _reloadLogs() {
            axios
                .get(this.getLogURL)
                .then((response) => {
                    this.content = response.data;
                    if (this.follow) {
                        this.$nextTick(() => {
                            this.$el.scrollIntoView(false);
                        });
                    }
                })
                .catch((error) => {});
        },
        reloadLogs() {
            if (this.task.status === "running") {
                this.flushLogs();
            } else {
                this._reloadLogs();
            }
        },
        addLog(log) {
            // It is better not to add logs directly as it may cause browser
            // to render changes to often
            this.cachedContent = this.cachedContent + log.data;
        },
        flushContent() {
            if (this.cachedContent.length > 0) {
                this.content = this.content + this.cachedContent;
                this.cachedContent = "";
                if (this.follow) {
                    this.$nextTick(() => {
                        this.$el.scrollIntoView(false);
                    });
                }
            }
        },
        clearLogs() {
            this.cachedContent = "";
            this.content = "";
        },
        onStatusChange(value) {
            if (value === "running") {
                this.flushInterval = setInterval(
                    function () {
                        this.flushContent();
                    }.bind(this),
                    FlushContentPeriod
                );
            } else {
                clearInterval(this.flushInterval);
                this.flushContent();
            }
        },
        toggleLogs() {
            if (this.content.length > 0) {
                this.clearLogs();
                return;
            }
            this.reloadLogs();
        },
    },
};
</script>

<style lang="scss" scoped>
.log-container {
    overflow: auto;
}
.log-line {
    white-space: pre-wrap;
    word-break: break-word;
}
</style>
