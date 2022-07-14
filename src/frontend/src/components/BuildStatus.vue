<template>
    <span
        class="label"
        :class="getStatusClass"
        data-cy="build-status-label"
        >{{ status }}</span
    >
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
                    return "label-warning";
                case "failed":
                    return "label-error";
                case "skipped":
                    return "label-skipped";
                case "finished":
                    return "label-success";
                case "aborted":
                case "timed out":
                    return "label-primary";
            }
            // pending
            return "";
        },
    },
};
</script>

<style scoped lang="scss">
span {
    margin: 0.25em;
}
span:hover {
    cursor: default;
}
.label-skipped {
    background-color: royalblue;
    color: white;
}
</style>
