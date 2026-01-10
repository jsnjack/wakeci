<template>
    <article
        class="fill small-margin small-padding medium-text no-elevate"
        style="font-weight: 400; display: inline-block; cursor: pointer"
        data-cy="params-value"
    >
        <div class="row no-space">
            <div
                @click.prevent="setFilter($event)"
                style="word-break: break-all; flex: auto"
            >
                {{ getText }}
            </div>
            <i
                class="tiny"
                @click.prevent="copyContent"
                >{{ copyIcon }}</i
            >
        </div>

        <div
            v-if="!includeKeys"
            class="tooltip bottom"
        >
            {{ getName }}
        </div>
    </article>
</template>

<script>
const LENGTH_LIMIT = 30;

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
                return `${this.getName} = ${this.getValue}`;
            }
            if (this.getValue.length > LENGTH_LIMIT) {
                return this.getValue.substring(0, LENGTH_LIMIT - 3) + "...";
            }
            return this.getValue;
        },
        getValueStyle() {
            if (this.includeKeys) {
                return "cursor: 'default'";
            }
            return "ddd";
        },
    },
    methods: {
        copyContent() {
            navigator.clipboard.writeText(this.getValue).then(
                () => {
                    this.copyIcon = "done";
                    window.setTimeout(() => {
                        this.copyIcon = "content_copy";
                    }, 1500);
                },
                () => {
                    this.copyIcon = "close";
                    window.setTimeout(() => {
                        this.copyIcon = "content_copy";
                    }, 1500);
                }
            );
        },
        setFilter(event) {
            this.$emit("setFilter", {
                value: this.getName + ":" + this.getValue,
                append: event.ctrlKey || event.metaKey,
            });
        },
    },
    data: function () {
        return {
            copyIcon: "content_copy",
        };
    },
};
</script>

<style scoped></style>
