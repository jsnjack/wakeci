<template>
    <span class="label label-rounded" :class="getStatusClass">{{ status }}</span>
</template>

<script>
export default {
    props: {
        status: {
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
            case "finished":
                return "label-success";
            case "aborted":
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
span:hover{
    cursor: default;
}
</style>
