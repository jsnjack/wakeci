<template>
    <div
        v-if="queue.stopping"
        class="ui queue-banner cluster nowrap gap-3"
        role="status"
        data-cy="queue-stopping"
    >
        <i class="icon">restart_alt</i>
        <div
            v-if="queue.buildsAborted"
            class="grow"
        >
            wakeci aborted the running builds and {{ queue.restartVersion ? `restarts into version ${queue.restartVersion}` : "stops" }} once their on_aborted
            and finally tasks end. New builds wait in the queue and start when wakeci is back.
        </div>
        <div
            v-else
            class="grow"
        >
            <span v-if="queue.restartVersion"> wakeci restarts into version {{ queue.restartVersion }} once the running builds end. </span>
            <span v-else>wakeci stops once the running builds end.</span>
            Builds still running at {{ abortTime }} are aborted. New builds wait in the queue and start when wakeci is back.
        </div>
        <button
            v-if="!queue.buildsAborted && canAny('write')"
            type="button"
            class="btn btn-outline btn-ghost"
            data-cy="queue-abort"
            :disabled="aborting"
            @click="confirming = true"
        >
            Restart now
        </button>
        <UiDialog
            v-model:open="confirming"
            title="Restart now?"
        >
            <p>
                Abort the running builds now? Their on_aborted and finally tasks still run, then wakeci
                {{ queue.restartVersion ? `restarts into version ${queue.restartVersion}` : "stops" }}. Queued builds start when wakeci is back.
            </p>
            <template #actions>
                <button
                    type="button"
                    class="btn btn-ghost"
                    @click="confirming = false"
                >
                    Cancel
                </button>
                <button
                    type="button"
                    class="btn btn-danger"
                    data-cy="queue-abort-confirm"
                    @click="abortNow"
                >
                    Abort builds and restart
                </button>
            </template>
        </UiDialog>
    </div>
</template>

<script>
import UiDialog from "@/components/ui/UiDialog.vue";
import { escapeHtml } from "@/store/utils";
import axios from "axios";
import vuex from "vuex";

export default {
    components: { UiDialog },
    data: function () {
        return {
            confirming: false,
            aborting: false,
        };
    },
    computed: {
        ...vuex.mapState(["queue"]),
        ...vuex.mapGetters(["canAny"]),
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
    methods: {
        abortNow() {
            this.confirming = false;
            this.aborting = true;
            axios
                .post("/api/queue/abort")
                .then((response) => {
                    this.$notify({ text: escapeHtml(response.data), type: "primary" });
                })
                .catch(() => {})
                .finally(() => {
                    this.aborting = false;
                });
        },
    },
};
</script>

<style scoped>
.queue-banner {
    margin-block-end: var(--space-4);
    padding: var(--space-3) var(--space-4);
    border-radius: var(--radius-container);
    background-color: var(--tertiary-container);
    color: var(--on-tertiary-container);
}
</style>
