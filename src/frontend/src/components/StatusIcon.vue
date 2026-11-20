<template>
    <span
        class="status-icon"
        :class="{ small }"
        role="img"
        :aria-label="status"
        :data-cy-status="status"
        :data-tip="tip ? status : null"
        :style="{ '--_c': `var(--status-${slug})` }"
    >
        <span
            v-if="status === 'running'"
            class="status-ring"
        ></span>
        <i
            v-else
            class="icon"
            aria-hidden="true"
            >{{ icon }}</i
        >
    </span>
</template>

<script>
// Each status has its own colour and shape, so colour never carries the
// meaning alone: a filled check, cross or clock, a grey outline for queued
// and not run, a turning ring while running
const ICONS = {
    finished: "check_circle",
    failed: "cancel",
    "timed out": "schedule",
    aborted: "do_not_disturb_on",
    skipped: "remove_circle",
    pending: "radio_button_unchecked",
};

export default {
    props: {
        status: {
            type: String,
            required: true,
        },
        small: {
            type: Boolean,
            default: false,
        },
        // Shows the status as a tooltip, for places that don't say it
        tip: {
            type: Boolean,
            default: false,
        },
    },
    computed: {
        icon() {
            return ICONS[this.status] || ICONS.pending;
        },
        slug() {
            return Object.hasOwn(ICONS, this.status) || this.status === "running" ? this.status.replace(" ", "-") : "pending";
        },
    },
};
</script>
