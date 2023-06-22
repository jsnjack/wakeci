<template>
    <div class="row">
        <i class="small">schedule</i>
        <div>{{ startedAgoText }}</div>
        <div
            v-if="startedAtText !== ''"
            class="tooltip bottom"
        >
            {{ startedAtText }}
        </div>
    </div>
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
            startedAgoText: "",
            startedAtText: "",
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
                this.startedAgoText = "";
                this.startedAtText = "";
                return;
            }
            try {
                const dateObj = new Date(this.item.startedAt);
                this.startedAgoText = ago.format(dateObj);
                this.startedAtText = dateObj.toLocaleString();
            } catch (e) {}
            return "";
        },
    },
};
</script>

<style scoped lang="scss"></style>
