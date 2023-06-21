<template>
    <span
        class="chip fill small"
        style="margin-bottom: 0.5em; word-break: break-all"
        @click.prevent="copyContent"
    >
        {{ getName }}={{ getValue }}
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
    methods: {
        copyContent() {
            navigator.clipboard.writeText(this.getValue).then(
                () => {
                    this.$notify({ text: "The value has been copied to clipboard", type: "primary" });
                },
                () => {
                    this.$notify({ text: "Unable to copy", type: "error" });
                }
            );
        },
    },
};
</script>

<style scoped></style>
