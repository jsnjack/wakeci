<template>
    <dialog
        :class="{ active: confirmingUpdate }"
        class="padding"
    >
        <div v-if="update">
            Install version <b>{{ update.latest }}</b> and restart? wakeci waits for the running builds and aborts any still running after the drain timeout (30
            minutes by default). New builds wait in the queue and start after the restart.
        </div>
        <nav class="right-align">
            <button
                class="border"
                @click.prevent="confirmingUpdate = false"
            >
                Cancel
            </button>
            <button
                data-cy="update-confirm"
                @click.prevent="install"
            >
                Update and restart
            </button>
        </nav>
    </dialog>

    <div>
        <article
            class="border no-elevate update-card"
            data-cy="update-card"
        >
            <div class="row">
                <div
                    class="update-icon"
                    :class="status.color"
                >
                    <i>{{ status.icon }}</i>
                </div>
                <div class="max">
                    <div
                        class="update-title"
                        data-cy="version"
                    >
                        wakeci {{ currentVersion }}
                    </div>
                    <div
                        class="update-text"
                        data-cy="update-status"
                    >
                        {{ status.text }}
                        <a
                            v-if="status.notes && update.releaseNotes"
                            :href="update.releaseNotes"
                            target="_blank"
                            rel="noopener"
                            class="link"
                            data-cy="release-notes"
                            >Release notes</a
                        >
                    </div>
                </div>
                <button
                    data-cy="update-install"
                    :disabled="!canUpdate || installing"
                    @click.prevent="confirmingUpdate = true"
                >
                    {{ installing ? "Installing..." : canUpdate ? "Update to " + update.latest : "Update" }}
                </button>
            </div>
        </article>

        <form
            method="post"
            @submit.prevent="save"
        >
            <!-- Only the admin password may change the password -->
            <div
                v-if="admin"
                class="field border small"
            >
                <input
                    id="password"
                    v-model="password"
                    type="password"
                />
                <span class="helper">Password</span>
            </div>

            <div class="field border small">
                <input
                    id="concurrent-builds"
                    v-model="concurrentBuilds"
                    type="number"
                    min="1"
                />
                <span class="helper">Number of concurrent builds</span>
            </div>

            <div class="field border small">
                <input
                    id="build-history-size"
                    v-model="buildHistorySize"
                    type="number"
                    min="1"
                />
                <span class="helper">Number of builds to preserve</span>
            </div>

            <nav class="no-space">
                <div class="max"></div>
                <button
                    data-cy="save-settings"
                    type="submit"
                >
                    Save
                </button>
            </nav>
        </form>
    </div>
</template>

<script>
import { escapeHtml } from "@/store/utils";
import axios from "axios";
import vuex from "vuex";

export default {
    data: function () {
        return {
            password: "",
            admin: false,
            concurrentBuilds: 2,
            buildHistorySize: 200,
            update: null,
            installing: false,
            confirmingUpdate: false,
        };
    },
    computed: {
        ...vuex.mapState(["queue"]),
        currentVersion: function () {
            if (this.update) {
                return this.update.current;
            }
            // The build date follows the version
            return (import.meta.env.VITE_VERSION || "").split("-")[0];
        },
        // What the card says, with an icon and its color
        status: function () {
            const update = this.update;
            if (this.queue.stopping) {
                const text = this.queue.restartVersion
                    ? `Restarting into ${this.queue.restartVersion} once the running builds end.`
                    : "wakeci stops once the running builds end.";
                return { icon: "restart_alt", color: "tertiary-container", text };
            }
            if (!update) {
                return { icon: "sync", color: "secondary-container", text: "Checking for updates..." };
            }
            if (update.error) {
                return { icon: "sync_problem", color: "secondary-container", text: `Couldn't check for updates: ${update.error}` };
            }
            if (update.available && update.problem) {
                return {
                    icon: "error",
                    color: "error-container",
                    text: `Version ${update.latest} is available, but wakeci can't update itself: ${update.problem}`,
                    notes: true,
                };
            }
            if (update.available) {
                return { icon: "download", color: "primary-container", text: `Version ${update.latest} is available.`, notes: true };
            }
            const checked = update.checkedAt
                ? ` · checked at ${new Date(update.checkedAt).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}`
                : "";
            return { icon: "check", color: "secondary-container", text: `Up to date${checked}` };
        },
        // Not while stopping: the banner says what happens next
        canUpdate: function () {
            return this.update && this.update.available && !this.update.problem && !this.queue.stopping;
        },
    },
    mounted() {
        this.$store.commit("SET_CURRENT_PAGE", "Settings");
        this.fetch();
        this.checkUpdate();
    },
    methods: {
        save() {
            const data = new FormData();
            data.append("password", this.password);
            data.append("concurrentBuilds", this.concurrentBuilds);
            data.append("buildHistorySize", this.buildHistorySize);
            axios
                .post("/api/settings", data, {
                    headers: {
                        "Content-type": "application/x-www-form-urlencoded",
                    },
                })
                .then((response) => {
                    this.$notify({
                        text: "Saved",
                        type: "primary",
                    });
                })
                .catch((error) => {});
        },
        checkUpdate() {
            axios
                .get("/api/update/")
                .then((response) => {
                    this.update = response.data;
                })
                .catch((error) => {});
        },
        install() {
            this.confirmingUpdate = false;
            this.installing = true;
            const data = new FormData();
            data.append("version", this.update.latest);
            axios
                .post("/api/update/", data, {
                    headers: {
                        "Content-type": "application/x-www-form-urlencoded",
                    },
                })
                .then((response) => {
                    this.$notify({
                        text: escapeHtml(response.data),
                        type: "primary",
                    });
                })
                .catch((error) => {})
                .finally(() => {
                    this.installing = false;
                });
        },
        fetch() {
            axios
                .get("/api/settings")
                .then((response) => {
                    this.admin = response.data.admin;
                    if (response.data.concurrentBuilds) {
                        this.concurrentBuilds = response.data.concurrentBuilds;
                    }
                    if (response.data.buildHistorySize) {
                        this.buildHistorySize = response.data.buildHistorySize;
                    }
                })
                .catch((error) => {});
        },
    },
};
</script>

<!-- Add "scoped" attribute to limit CSS to this component only -->
<style scoped>
.update-card {
    margin: 0 0 1.5rem;
}
.update-icon {
    flex: none;
    width: 2.5rem;
    height: 2.5rem;
    border-radius: 50%;
    display: flex;
    align-items: center;
    justify-content: center;
}
.update-title {
    font-size: 1rem;
    font-weight: 500;
}
.update-text {
    font-size: 0.875rem;
    opacity: 0.8;
}
</style>
