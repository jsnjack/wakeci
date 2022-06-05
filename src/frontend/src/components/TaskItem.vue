<template>
    <section class="task-item" v-show="isVisible" :data-cy="getCyText">
        <div class="divider" :data-content="getDividerText" />
        <div :class="['task-title', `task-title-${task.status}`]">
            <TaskStatus :status="task.status" :task-title="task.name" />
            <span class="h5 task-name" @click="reloadLogs">{{ name }} ({{ task.kind }})</span>
            <DurationElement v-show="task.status !== 'pending'" :item="task" class="desktop-only" />
            <div class="task-item-actions">
                <button
                    data-cy="reload"
                    class="btn btn-sm btn-primary small desktop-only"
                    @click="reloadLogs"
                >
                    <span class="material-icons">refresh</span>
                    Reload logs
                </button>

                <a :href="getLogURL" target="_blank">
                    <span class="material-icons">open_in_new</span>
                    Open logs
                </a>

                <span
                    :class="['material-icons', 'accordion-control', { opened: content }]"
                    @click="toggleLogs"
                >
                    chevron_right
                </span>
            </div>
        </div>
        <div class="log-container" v-show="!!content">
            <pre class="logs">{{ content }}</pre>
            <TextSpinner v-show="task.status === 'running'" />
            <button @click.prevent="clearLogs" class="btn btn-secondary small">Hide</button>
        </div>
    </section>
</template>

<script>
import axios from 'axios';
import BuildStatus from '@/components/BuildStatus.vue';
import TextSpinner from '@/components/TextSpinner.vue';
import DurationElement from '@/components/DurationElement.vue';
import TaskStatus from '@/components/TaskStatus.vue';

const FlushContentPeriod = 500;

export default {
    components: { BuildStatus, DurationElement, TextSpinner, TaskStatus },
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
            cachedContent: '',
            content: '',
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
            if (this.task.kind === 'main' || this.task.kind === 'finally') {
                return true;
            }
            return !(this.task.startedAt && this.task.startedAt.indexOf('0001-') === 0);
        },
        getLogURL() {
            return `/storage/build/${this.buildID}/task_${this.task.id}.log`;
        },
        getFlushURL() {
            return `/api/build/${this.buildID}/flush`;
        },
    },
    watch: {
        'task.status': 'onStatusChange',
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
        toggleLogs() {
            this.content ? this.clearLogs() : this.reloadLogs();
        },
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
                    this.$notify({
                        text: 'Log file has been reloaded',
                        type: 'info',
                        duration: 1000,
                    });
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
            if (this.task.status === 'running') {
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
                this.cachedContent = '';
                if (this.follow) {
                    this.$nextTick(() => {
                        this.$el.scrollIntoView({
                            behavior: 'smooth',
                            block: 'end',
                        });
                    });
                }
            }
        },
        clearLogs() {
            this.cachedContent = '';
            this.content = '';
        },
        onStatusChange(value, oldValue) {
            if (value === 'running') {
                this.flushInterval = setInterval(
                    function () {
                        this.flushContent();
                    }.bind(this),
                    FlushContentPeriod,
                );
            } else {
                clearInterval(this.flushInterval);
                this.flushContent();
            }

            if (oldValue === 'running' && value === 'finished') {
                this.clearLogs();
            }
        },
    },
};
</script>

<style lang="scss" scoped>
.task-item {
    margin-top: -2px;
    .task-title {
        @apply flex gap-4 items-center bg-white p-2 border-l-2 rounded-sm;
        &.task-title-finished {
            @apply border-success;
            & + .log-container {
                @apply border-success;
            }
        }
        &.task-title-failed {
            @apply border-danger;
            & + .log-container {
                @apply border-danger;
            }
        }
        &.task-title-running {
            @apply border-info;
            & + .log-container {
                @apply border-info;
            }
        }
        &.task-title-aborted {
            @apply border-warning;
            & + .log-container {
                @apply border-warning;
            }
        }
        .task-item-actions {
            @apply flex-1 w-full flex justify-end items-center gap-2;
        }
        .accordion-control {
            @apply transform rotate-90 cursor-pointer transition-transform duration-200;
            &.opened {
                @apply -rotate-90;
            }
        }
    }
    .log-container {
        @apply p-3 text-secondary bg-white border-l-2 max-w-full;
        margin-top: -1px;
        .logs {
            @apply overflow-x-hidden font-mono p-2 border border-secondary bg-gray-light dark:bg-secondary dark:text-gray-light whitespace-pre-wrap;
        }
        .btn {
            @apply ml-auto my-2;
        }
    }
}
</style>
