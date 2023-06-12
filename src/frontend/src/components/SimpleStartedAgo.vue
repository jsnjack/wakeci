<template>
    <div>{{ startedText }}</div>
</template>

<script>
import TimeAgo from "javascript-time-ago";

import en from "javascript-time-ago/locale/en.json";

TimeAgo.addDefaultLocale(en);
const ago = new TimeAgo("en-US");

export default {
    props: {
        item: {
            required: true,
            type: Object,
        },
    },
    data: function () {
        return {
            startedText: "",
        };
    },
    computed: {},
    watch: {
        "item.status": "updateText",
    },
    mounted() {
        this.updateText();
    },
    methods: {
        updateText() {
            if (this.item.startedAt && this.item.startedAt.indexOf("0001-") === 0) {
                // Go's way of saying it is zero
                this.startedText = "";
                return;
            }
            try {
                this.startedText = ago.format(new Date(this.item.startedAt));
            } catch (e) {}
            return "";
        },
    },
};
</script>

<style scoped lang="scss"></style>
