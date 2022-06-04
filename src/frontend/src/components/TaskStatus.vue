<template>
    <span :class="['task-status', { clickable }]" :title="`${taskTitle} ${status}`">
        <span
            :aria-label="`Task status: ${status}`"
            role="presentation"
            :class="['material-icons', `task-${status}`]"
        >
            {{ statusIcon }}
        </span>
        <label v-if="showLabel" class="task-status-label">{{ taskTitle }}</label>
    </span>
</template>

<script>
export default {
    props: {
        taskTitle: {
            type: String,
            default: '',
        },
        status: {
            type: String,
            required: true,
            validator: (val) =>
                ['running', 'finished', 'failed', 'aborted', 'pending'].includes(val),
        },
        showLabel: {
            type: Boolean,
            required: false,
            default: false,
        },
        clickable: {
            type: Boolean,
            required: false,
            default: false,
        },
    },
    computed: {
        statusIcon() {
            const status = {
                running: 'radio_button_checked',
                finished: 'check_circle_outline',
                failed: 'error_outline',
                aborted: 'warning',
                pending: 'radio_button_unchecked',
            };

            return status[this.status];
        },
    },
};
</script>

<style lang="scss" scoped>
.task-status {
    @apply flex items-center justify-center flex-col cursor-default;
    &.clickable {
        @apply cursor-pointer;
    }
    .task-status-label {
        @apply text-xs text-secondary capitalize;
    }
    .task-running {
        @apply text-info animate-pulse;
    }
    .task-finished {
        @apply text-success;
    }
    .task-failed {
        @apply text-danger;
    }
    .task-aborted {
        @apply text-warning;
    }
    .task-pending {
        @apply text-gray-border;
    }
}
</style>
