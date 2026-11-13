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

    <div class="settings-page">
        <section
            data-cy="update-card"
            aria-label="Updates"
        >
            <div class="row update-row">
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
        </section>
        <hr class="large" />
        <form
            method="post"
            @submit.prevent="save"
        >
            <section aria-labelledby="builds-heading">
                <h6
                    id="builds-heading"
                    class="small no-margin"
                >
                    Builds
                </h6>
                <div class="grid">
                    <div class="s12 m6">
                        <div class="field label border">
                            <input
                                id="concurrent-builds"
                                v-model="concurrentBuilds"
                                type="number"
                                min="1"
                                placeholder=" "
                                aria-describedby="concurrent-builds-help"
                            />
                            <label for="concurrent-builds">Concurrent builds</label>
                            <output id="concurrent-builds-help">How many builds may run at the same time</output>
                        </div>
                    </div>
                    <div class="s12 m6">
                        <div class="field label border">
                            <input
                                id="build-history-size"
                                v-model="buildHistorySize"
                                type="number"
                                min="1"
                                placeholder=" "
                                aria-describedby="build-history-help"
                            />
                            <label for="build-history-size">Builds to keep</label>
                            <output id="build-history-help">The latest builds of all jobs, with logs and files. Analytics keeps 400 days</output>
                        </div>
                    </div>
                </div>
            </section>
            <hr class="large" />
            <section aria-labelledby="api-limits-heading">
                <nav class="no-margin">
                    <h6
                        id="api-limits-heading"
                        class="small max no-margin"
                    >
                        API key limits
                    </h6>
                    <router-link
                        to="/keys"
                        class="button border small-round"
                        ><i>key</i><span>Manage API keys</span></router-link
                    >
                </nav>
                <p class="muted description">Each key has its own limits. A key that goes over them gets 429 and waits as long as Retry-After says.</p>
                <div class="grid">
                    <div class="s12 m6">
                        <div class="field label border">
                            <input
                                id="api-key-burst"
                                v-model="apiKeyBurst"
                                type="number"
                                min="1"
                                max="100000"
                                placeholder=" "
                                data-cy="api-key-burst"
                                aria-describedby="api-key-burst-help"
                            />
                            <label for="api-key-burst">Burst size</label>
                            <output id="api-key-burst-help">How many requests a key may send at full speed before it has to slow down</output>
                        </div>
                    </div>
                    <div class="s12 m6">
                        <div class="field label border">
                            <input
                                id="api-key-rate"
                                v-model="apiKeyRate"
                                type="number"
                                min="1"
                                max="100000"
                                placeholder=" "
                                data-cy="api-key-rate"
                                aria-describedby="api-key-rate-help"
                            />
                            <label for="api-key-rate">Steady rate</label>
                            <output id="api-key-rate-help">Requests a second after the burst is used</output>
                        </div>
                    </div>
                </div>
                <p
                    v-if="apiKeyExample"
                    class="muted small-text"
                    data-cy="api-key-example"
                >
                    {{ apiKeyExample }}
                </p>
            </section>
            <!-- Only the admin password may change the password -->
            <template v-if="admin">
                <hr class="large" />
                <section aria-labelledby="password-heading">
                    <h6
                        id="password-heading"
                        class="small no-margin"
                    >
                        Admin password
                    </h6>
                    <div class="grid">
                        <div class="s12 m6">
                            <div class="field label border">
                                <input
                                    id="password"
                                    v-model="password"
                                    type="password"
                                    autocomplete="new-password"
                                    placeholder=" "
                                    aria-describedby="password-help"
                                />
                                <label for="password">New password</label>
                                <output id="password-help">For password login and Basic auth. Leave it empty to keep the current one</output>
                            </div>
                        </div>
                    </div>
                </section>
            </template>
            <nav class="right-align">
                <button
                    data-cy="save-settings"
                    type="submit"
                >
                    Save
                </button>
            </nav>
        </form>
        <!-- Only the admin password sees or changes the groups -->
        <template v-if="auth.admin">
            <hr class="large" />
            <SSOGroupsCard :key="groupsKey" />
            <hr class="large" />
            <SettingsFileCard @imported="reload" />
        </template>
    </div>
</template>

<script>
import SSOGroupsCard from "@/components/SSOGroupsCard.vue";
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
    components: { SSOGroupsCard, SettingsFileCard },
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
.muted {
    color: var(--on-surface-variant);
}
.description {
    margin: 0.5rem 0 0;
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
@media (max-width: 600px) {
    .update-row {
        flex-wrap: wrap;
    }
    .update-row .max {
        min-width: 0;
    }
}
</style>
