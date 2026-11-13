<template>
    <article
        v-if="queue.stopping"
        class="tertiary-container no-elevate"
        data-cy="queue-stopping"
    >
        <div class="row">
            <i>restart_alt</i>
            <div
                v-if="queue.buildsAborted"
                class="max"
            >
                wakeci aborted the running builds and {{ queue.restartVersion ? `restarts into version ${queue.restartVersion}` : "stops" }} once their
                on_aborted and finally tasks end. New builds wait in the queue and start when wakeci is back.
            </div>
            <div
                v-else
                class="max"
            >
                <span v-if="queue.restartVersion"> wakeci restarts into version {{ queue.restartVersion }} once the running builds end. </span>
                <span v-else>wakeci stops once the running builds end.</span>
                Builds still running at {{ abortTime }} are aborted. New builds wait in the queue and start when wakeci is back.
            </div>
            <button
                v-if="!queue.buildsAborted && canAny('write')"
                class="border"
                data-cy="queue-abort"
                :disabled="aborting"
                @click.prevent="confirming = true"
            >
                {{ actionName }}
            </button>
        </div>
        <dialog
            :class="{ active: confirming }"
            class="padding"
        >
            <div>
                Abort the running builds now? Their on_aborted and finally tasks still run, then wakeci
                {{ queue.restartVersion ? `restarts into version ${queue.restartVersion}` : "stops" }}. Queued builds start when wakeci is back.
            </div>
            <nav class="right-align">
                <button
                    class="border"
                    @click.prevent="confirming = false"
                >
                    Cancel
                </button>
                <button
                    data-cy="queue-abort-confirm"
                    @click.prevent="abortNow"
                >
                    Abort builds and {{ queue.restartVersion ? "restart" : "stop" }}
                </button>
            </nav>
        </dialog>
    </article>
</template>

<script>
import { escapeHtml } from "@/store/utils";
import axios from "axios";
import vuex from "vuex";

export default {
    data: function () {
        return {
            confirming: false,
            aborting: false,
        };
    },
    computed: {
        ...vuex.mapState(["queue"]),
        ...vuex.mapGetters(["canAny"]),
        // A stop that no update asked for is a restart only if a service
        // manager starts wakeci again
        actionName: function () {
            return this.queue.restartVersion ? "Restart now" : "Stop now";
        },
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
article {
    margin: 0 0 1rem;
}
</style>
