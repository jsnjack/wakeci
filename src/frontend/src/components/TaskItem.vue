<template>
    <section v-show="isVisible" :data-cy="getCyText">
        <div class="divider" :data-content="getDividerText" />
        <div class="columns">
            <div class="column">
                <div class="text-left">
                    <BuildStatus :status="task.status" />
                    <span class="h5 task-name" @click="reloadLogs">{{ name }}</span>
                    <DurationElement
                        v-show="task.status !== 'pending'"
                        :item="task"
                        class="text-small m-1"
                    />
                </div>
            </div>
            <div class="column text-right">
                <div class="dropdown dropdown-right text-left">
                    <div class="btn-group">
                        <button data-cy="reload" class="btn btn-sm btn-primary" @click="reloadLogs">
                            Reload
                        </button>
                        <a class="btn btn-sm dropdown-toggle" tabindex="0">
                            <i class="icon icon-caret" />
                        </a>
                        <ul class="menu">
                            <li class="divider" data-content="ACTIONS" />
                            <li class="menu-item">
                                <a :href="getLogURL" target="_blank">Open</a>
                            </li>
                            <li class="menu-item">
                                <a href="#" @click="clearLogs">Hide</a>
                            </li>
                        </ul>
                    </div>
                </div>
            </div>
        </div>
        <div class="log-container text-left code status-border" :class="getBorderClass">
            <pre v-if="content" class="d-block log-line">{{ content }}</pre>
            <TextSpinner v-show="task.status === 'running'" />
        </div>
    </section>
</template>

<script>
import BuildStatus from '@/components/BuildStatus.vue';
import TextSpinner from '@/components/TextSpinner.vue';
import DurationElement from '@/components/DurationElement.vue';
import axios from 'axios';

const FlushContentPeriod = 500;

export default {
    components: { BuildStatus, DurationElement, TextSpinner },
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
        getBorderClass() {
            return `border-${this.task.status}`.replaceAll(' ', '');
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
                        type: 'success',
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
                        this.$el.scrollIntoView(false);
                    });
                }
            }
        },
        clearLogs() {
            this.cachedContent = '';
            this.content = '';
        },
        onStatusChange(value) {
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
        },
    },
};
</script>
