<template>
    <article
        v-if="queue.stopping"
        class="tertiary-container no-elevate"
        data-cy="queue-stopping"
    >
        <div class="row">
            <i>restart_alt</i>
            <div class="max">
                <span v-if="queue.restartVersion"> wakeci restarts into version {{ queue.restartVersion }} once the running builds end. </span>
                <span v-else>wakeci stops once the running builds end.</span>
                Builds still running at {{ abortTime }} are aborted. New builds wait in the queue and start when wakeci is back.
            </div>
        </div>
    </article>
</template>

<script>
import vuex from "vuex";

export default {
    computed: {
        ...vuex.mapState(["queue"]),
        abortTime: function () {
            if (!this.queue.abortAt) {
                return "";
            }
            const abortAt = new Date(this.queue.abortAt);
            const options = { hour: "2-digit", minute: "2-digit" };
            if (abortAt.toDateString() !== new Date().toDateString()) {
                options.month = "short";
                options.day = "numeric";
            }
            return abortAt.toLocaleString([], options);
        },
    },
};
</script>

<style scoped>
article {
    margin: 0 0 1rem;
}
</style>
