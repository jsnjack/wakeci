<template>
    <article
        class="fill small-margin small-padding medium-text no-elevate"
        style="font-weight: 400; display: inline-block; cursor: pointer; word-break: break-all"
        @click.prevent="copyContent"
    >
        {{ getText }}
        <div
            v-if="!includeKeys"
            class="tooltip bottom"
        >
            {{ getName }}
        </div>
    </article>
</template>

<script>
export default {
    props: {
        param: {
            required: true,
            type: Object,
        },
        includeKeys: {
            required: false,
            type: Boolean,
            default: false,
        },
    },
    computed: {
        getName() {
            return Object.keys(this.param)[0];
        },
        getValue() {
            return this.param[this.getName];
        },
        getText() {
            if (this.includeKeys) {
                return `${this.getName}=${this.getValue}`;
            }
            return this.getValue;
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
