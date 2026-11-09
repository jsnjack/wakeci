<template>
    <a
        class="button circle transparent"
        :disabled="status === 'pending' ? null : true"
        :href="getStartURL"
        data-cy="start-build-button"
        @click.prevent="start"
    >
        <i>play_arrow</i>
        <div class="tooltip bottom">Start now</div>
    </a>
</template>

<script>
import axios from "axios";

export default {
    props: {
        status: {
            type: String,
            required: true,
        },
        buildID: {
            type: Number,
            required: true,
        },
    },
    computed: {
        getStartURL: function () {
            return `/api/build/${this.buildID}/start`;
        },
    },
    methods: {
        start(event) {
            if (this.status === "pending") {
                axios
                    .post(event.target.href || event.target.parentElement.href)
                    .then(() => {
                        this.$notify({ text: `${this.buildID} has been started`, type: "primary" });
                    })
                    .catch(() => {});
            }
        },
    },
};
</script>

<style  scoped></style>
