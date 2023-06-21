<template>
    <span
        class="chip fill small"
        style="margin-bottom: 0.5em; word-break: break-all"
        data-cy="params-value"
    >
        {{ getValue }}
        <div class="tooltip bottom">
            {{ getName }}
        </div>
    </span>
</template>

<script>
export default {
    props: {
        param: {
            required: true,
            type: Object,
        },
    },
    computed: {
        getName() {
            return Object.keys(this.param)[0];
        },
        getValue() {
            return this.param[this.getName];
        },
    },
};
</script>

<style scoped></style>
