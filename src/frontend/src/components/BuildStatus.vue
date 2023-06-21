<template>
    <button
        class="small circle"
        :class="getStatusClass"
        :data-cy-status="status"
    >
        <i>{{ getStatusIcon }}</i>
        <div class="tooltip right">{{ status }}</div>
    </button>
</template>

<script>
export default {
    props: {
        status: {
            type: null,
            required: true,
        },
    },
    computed: {
        getStatusClass() {
            switch (this.status) {
                case "running":
                    return "yellow7";
                case "failed":
                    return "error";
                case "skipped":
                    return "light-green4";
                case "finished":
                    return "green6";
                case "aborted":
                    return "grey6";
                case "timed out":
                    return "deep-purple6";
            }
            // pending
            return "";
        },
        getStatusIcon() {
            switch (this.status) {
                case "running":
                    return "downloading";
                case "failed":
                    return "cancel";
                case "skipped":
                    return "play_disabled";
                case "finished":
                    return "check_circle";
                case "aborted":
                    return "do_not_disturb_on";
                case "timed out":
                    return "acute";
            }
            // pending
            return "pending";
        },
    },
};
</script>

<style scoped lang="scss">
// Disable cursor which comes from a button
button {
    cursor: auto;
}

// Disable light color on hover
button:hover {
    background-image: none;
}
</style>
