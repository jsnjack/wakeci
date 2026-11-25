<template>
    <div class="ui settings-page stack gap-6">
        <UiDialog
            v-model:open="confirmingUpdate"
            title="Update and restart?"
        >
            <p v-if="update">
                Install version <b>{{ update.latest }}</b> and restart? wakeci waits for the running builds and aborts any still running after the drain timeout
                (30 minutes by default). New builds wait in the queue and start after the restart.
            </p>
            <template #actions>
                <button
                    type="button"
                    class="btn btn-ghost"
                    @click="confirmingUpdate = false"
                >
                    Cancel
                </button>
                <button
                    type="button"
                    class="btn btn-primary"
                    data-cy="update-confirm"
                    @click="install"
                >
                    Update and restart
                </button>
            </template>
        </UiDialog>

        <section
            class="cluster gap-3 update-row"
            data-cy="update-card"
            aria-label="Updates"
        >
            <div
                class="update-icon"
                :style="{ backgroundColor: `var(--${status.color})`, color: `var(--on-${status.color})` }"
            >
                <i class="icon">{{ status.icon }}</i>
            </div>
            <div class="grow">
                <div
                    class="update-title"
                    data-cy="version"
                >
                    wakeci {{ currentVersion }}
                </div>
                <div
                    class="muted text-sm"
                    data-cy="update-status"
                >
                    {{ status.text }}
                    <a
                        v-if="status.notes && update.releaseNotes"
                        :href="update.releaseNotes"
                        target="_blank"
                        rel="noopener"
                        data-cy="release-notes"
                        >Release notes</a
                    >
                </div>
            </div>
            <button
                type="button"
                class="btn btn-primary"
                data-cy="update-install"
                :disabled="!canUpdate || installing"
                @click="confirmingUpdate = true"
            >
                {{ installing ? "Installing..." : canUpdate ? "Update to " + update.latest : "Update" }}
            </button>
        </section>
        <hr class="rule" />
        <form
            class="stack gap-6"
            method="post"
            @submit.prevent="save"
        >
            <section
                class="stack gap-3"
                aria-labelledby="builds-heading"
            >
                <h2 id="builds-heading">Builds</h2>
                <div class="cols">
                    <div class="form-field">
                        <label
                            class="label-text"
                            for="concurrent-builds"
                            >Concurrent builds</label
                        >
                        <input
                            id="concurrent-builds"
                            v-model="concurrentBuilds"
                            class="input"
                            type="number"
                            min="1"
                            :aria-describedby="'concurrent-builds-help'"
                        />
                        <span
                            id="concurrent-builds-help"
                            class="hint"
                            >How many builds may run at the same time</span
                        >
                    </div>
                    <div class="form-field">
                        <label
                            class="label-text"
                            for="build-history-size"
                            >Builds to keep</label
                        >
                        <input
                            id="build-history-size"
                            v-model="buildHistorySize"
                            class="input"
                            type="number"
                            min="1"
                            :aria-describedby="'build-history-help'"
                        />
                        <span
                            id="build-history-help"
                            class="hint"
                            >The latest builds of all jobs, with logs and files. Analytics keeps 400 days</span
                        >
                    </div>
                </div>
            </section>
            <hr class="rule" />
            <section
                class="stack gap-3"
                aria-labelledby="api-limits-heading"
            >
                <div class="cluster between gap-2">
                    <h2 id="api-limits-heading">API key limits</h2>
                    <router-link
                        to="/keys"
                        class="btn btn-outline btn-ghost"
                        ><i class="icon small">key</i><span>Manage API keys</span></router-link
                    >
                </div>
                <p class="muted">Each key has its own limits. A key that goes over them gets 429 and waits as long as Retry-After says.</p>
                <div class="cols">
                    <div class="form-field">
                        <label
                            class="label-text"
                            for="api-key-burst"
                            >Burst size</label
                        >
                        <input
                            id="api-key-burst"
                            v-model="apiKeyBurst"
                            class="input"
                            type="number"
                            min="1"
                            max="100000"
                            data-cy="api-key-burst"
                            :aria-describedby="'api-key-burst-help'"
                        />
                        <span
                            id="api-key-burst-help"
                            class="hint"
                            >How many requests a key may send at full speed before it has to slow down</span
                        >
                    </div>
                    <div class="form-field">
                        <label
                            class="label-text"
                            for="api-key-rate"
                            >Steady rate</label
                        >
                        <input
                            id="api-key-rate"
                            v-model="apiKeyRate"
                            class="input"
                            type="number"
                            min="1"
                            max="100000"
                            data-cy="api-key-rate"
                            :aria-describedby="'api-key-rate-help'"
                        />
                        <span
                            id="api-key-rate-help"
                            class="hint"
                            >Requests a second after the burst is used</span
                        >
                    </div>
                </div>
                <p
                    v-if="apiKeyExample"
                    class="muted text-sm"
                    data-cy="api-key-example"
                >
                    {{ apiKeyExample }}
                </p>
            </section>
            <!-- Only the admin password may change the password -->
            <template v-if="admin">
                <hr class="rule" />
                <section
                    class="stack gap-3"
                    aria-labelledby="password-heading"
                >
                    <h2 id="password-heading">Admin password</h2>
                    <div class="cols">
                        <div class="form-field">
                            <label
                                class="label-text"
                                for="password"
                                >New password</label
                            >
                            <input
                                id="password"
                                v-model="password"
                                class="input"
                                type="password"
                                autocomplete="new-password"
                                :aria-describedby="'password-help'"
                            />
                            <span
                                id="password-help"
                                class="hint"
                                >For password login and Basic auth. Leave it empty to keep the current one</span
                            >
                        </div>
                    </div>
                </section>
            </template>
            <div class="cluster end">
                <button
                    type="submit"
                    class="btn btn-primary"
                    data-cy="save-settings"
                >
                    Save
                </button>
            </div>
        </form>
        <!-- Only the admin password sees or changes the groups -->
        <template v-if="auth.admin">
            <hr class="rule" />
            <SSOGroupsCard :key="groupsKey" />
            <hr class="rule" />
            <SettingsFileCard @imported="reload" />
        </template>
    </div>
</template>

<script>
import SSOGroupsCard from "@/components/SSOGroupsCard.vue";
import UiDialog from "@/components/ui/UiDialog.vue";
import SettingsFileCard from "@/components/SettingsFileCard.vue";
import { escapeHtml } from "@/store/utils";
import axios from "axios";
import vuex from "vuex";

// duration says how long seconds is, rounded for a sentence
function duration(seconds) {
    if (seconds < 1) {
        return "less than a second";
    }
    const round = (value, unit) => {
        const n = Math.round(value);
        return n === 1 ? `1 ${unit}` : `${n} ${unit}s`;
    };
    if (seconds < 90) {
        return round(seconds, "second");
    }
    if (seconds < 90 * 60) {
        return round(seconds / 60, "minute");
    }
    return round(seconds / 3600, "hour");
}

export default {
    components: { SSOGroupsCard, SettingsFileCard, UiDialog },
    data: function () {
        return {
            password: "",
            admin: false,
            concurrentBuilds: 2,
            buildHistorySize: 200,
            apiKeyRate: 10,
            apiKeyBurst: 120,
            update: null,
            installing: false,
            confirmingUpdate: false,
            // Changing it reloads the groups after an import
            groupsKey: 0,
        };
    },
    computed: {
        ...vuex.mapState(["queue", "auth"]),
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
        // What the API key limits mean with the values typed, and how the
        // burst comes back (keyRequestLimiter.take)
        apiKeyExample: function () {
            const burst = Number(this.apiKeyBurst);
            const rate = Number(this.apiKeyRate);
            if (!(burst >= 1 && rate >= 1)) {
                return "";
            }
            const requests = burst === 1 ? "1 request" : `${burst} requests`;
            return (
                `With these values, a script can send ${requests} as fast as it can, even all within one second, then ${rate} a second. ` +
                `The burst isn't reset on a timer: every second gives ${rate} of it back and every request uses one. ` +
                `So ${duration(burst / rate)} without requests fill it again, and a key that keeps sending ${rate} a second never gets any back.`
            );
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
            data.append("apiKeyRate", this.apiKeyRate);
            data.append("apiKeyBurst", this.apiKeyBurst);
            axios
                .post("/api/settings", data, {
                    headers: {
                        "Content-type": "application/x-www-form-urlencoded",
                    },
                })
                .then(() => {
                    this.$notify({
                        text: "Saved",
                        type: "primary",
                    });
                })
                .catch(() => {});
        },
        reload() {
            this.fetch();
            this.groupsKey++;
        },
        checkUpdate() {
            axios
                .get("/api/update/")
                .then((response) => {
                    this.update = response.data;
                })
                .catch(() => {});
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
                .catch(() => {})
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
                    if (response.data.apiKeyRate) {
                        this.apiKeyRate = response.data.apiKeyRate;
                    }
                    if (response.data.apiKeyBurst) {
                        this.apiKeyBurst = response.data.apiKeyBurst;
                    }
                })
                .catch(() => {});
        },
    },
};
</script>

<style scoped>
.update-icon {
    flex: none;
    display: flex;
    align-items: center;
    justify-content: center;
    inline-size: 2.5rem;
    block-size: 2.5rem;
    border-radius: 50%;
}

.update-title {
    font-weight: 600;
}

.cols {
    --min: 18rem;
}
</style>
