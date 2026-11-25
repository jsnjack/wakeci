<template>
    <div class="ui keys-page stack gap-3">
        <UiDialog
            :open="revoking !== null"
            title="Revoke the key?"
            @update:open="revoking = null"
        >
            <p v-if="revoking">
                Revoke <b>{{ revoking.name }}</b
                >? Anything using it stops working at once.
            </p>
            <template #actions>
                <button
                    type="button"
                    class="btn btn-ghost"
                    @click="revoking = null"
                >
                    Cancel
                </button>
                <button
                    type="button"
                    class="btn btn-danger"
                    data-cy="revoke-key-confirm"
                    @click="revoke"
                >
                    Revoke
                </button>
            </template>
        </UiDialog>

        <UiDialog
            :open="revokingPerson !== null"
            title="Revoke access?"
            data-cy="revoke-person-dialog"
            @update:open="revokingPerson = null"
        >
            <p v-if="revokingPerson">
                Revoke access of <b>{{ revokingPerson.person }}</b
                >? This signs them out and revokes their {{ revokingPerson.keys.length }} keys. Anything using the keys stops working at once.
            </p>
            <template #actions>
                <button
                    type="button"
                    class="btn btn-ghost"
                    @click="revokingPerson = null"
                >
                    Cancel
                </button>
                <button
                    type="button"
                    class="btn btn-danger"
                    data-cy="revoke-person-confirm"
                    @click="revokePerson"
                >
                    Revoke access
                </button>
            </template>
        </UiDialog>

        <div class="cluster between gap-2">
            <label
                v-if="admin && people.length > 4"
                class="input-icon key-search"
            >
                <i class="icon">search</i>
                <input
                    v-model="search"
                    type="search"
                    class="input"
                    aria-label="Search people and keys"
                    placeholder="Search people and keys"
                    data-cy="key-search"
                />
            </label>
            <span v-else></span>
            <router-link
                to="/keys/new"
                class="btn btn-primary"
                data-cy="create-key-open"
            >
                <i class="icon small">add</i>
                <span>Create API key</span>
            </router-link>
        </div>

        <div
            class="stack gap-4"
            data-cy="keys-list"
        >
            <!-- The admin password sees every key, by who made it; anyone else sees their own -->
            <section
                v-for="section in sections"
                :key="section.id"
                class="stack gap-0"
                :data-cy="'key-group-' + section.id"
            >
                <div
                    v-if="admin"
                    class="cluster between gap-2 section-head"
                >
                    <div>
                        <b>{{ section.title }}</b>
                        <small
                            v-if="section.signedIn"
                            class="muted"
                            data-cy="key-group-status"
                        >
                            · {{ section.keys.length === 0 ? "signed in, no keys" : "signed in" }}
                        </small>
                    </div>
                    <button
                        v-if="section.person"
                        type="button"
                        class="btn btn-sm btn-outline btn-ghost"
                        data-cy="revoke-person"
                        @click="revokingPerson = section"
                    >
                        <i class="icon small">person_remove</i>
                        <span>Revoke access</span>
                    </button>
                </div>
                <div class="key-list">
                    <div
                        v-for="key in section.keys"
                        :key="key.id"
                        class="key"
                        :data-cy="'key-' + key.name"
                    >
                        <div class="details stack gap-1">
                            <div class="cluster gap-2">
                                <b>{{ key.name }}</b>
                                <span
                                    v-for="scope in key.scopes"
                                    :key="scope"
                                    class="scope text-sm"
                                >
                                    {{ scope }}
                                </span>
                                <!-- Logs, the audit log and build pages name a key by its ID -->
                                <code
                                    class="muted key-id"
                                    title="Key ID, as logs name the key"
                                    data-cy="key-id"
                                >
                                    {{ key.id }}
                                </code>
                            </div>
                            <small class="muted">
                                <span
                                    :title="jobsSummary(key)"
                                    data-cy="key-jobs"
                                >
                                    {{ jobsSummary(key) }}
                                </span>
                                ·
                                <span :title="formatDate(key.createdAt)">Created {{ ago(key.createdAt) }}</span>
                                ·
                                <span
                                    :title="formatDate(key.expiresAt)"
                                    data-cy="key-expiry"
                                >
                                    {{ expiryText(key) }}
                                </span>
                            </small>
                            <small class="muted stats">
                                <span
                                    :title="formatDate(key.lastUsedAt)"
                                    data-cy="key-last-used"
                                >
                                    {{ key.lastUsedAt ? "Used " + ago(key.lastUsedAt) : "Never used" }}
                                </span>
                                ·
                                <span data-cy="key-uses">{{ (key.uses || 0).toLocaleString() }} {{ key.uses === 1 ? "request" : "requests" }}</span>
                                <KeyUsageChart
                                    v-if="today && key.uses > 0"
                                    :uses-by-day="key.usesByDay || []"
                                    :today="today"
                                    :created="key.createdAt"
                                />
                            </small>
                        </div>
                        <span
                            v-if="isExpired(key)"
                            class="error-text"
                        >
                            Expired
                        </span>
                        <button
                            type="button"
                            class="btn btn-sm btn-outline btn-ghost revoke"
                            :aria-label="'Revoke ' + key.name"
                            data-cy="revoke-key"
                            @click="revoking = key"
                        >
                            <i class="icon small">key_off</i>
                            <span class="wide-label">Revoke key</span>
                        </button>
                    </div>
                </div>
            </section>

            <div
                v-if="sections.length === 0 && fetchingDone"
                class="keys-empty stack gap-2 muted"
            >
                <i class="icon large">key</i>
                <h2 v-if="search.trim() === ''">No API keys yet</h2>
                <h2 v-else>Nothing matches "{{ search.trim() }}"</h2>
                <p>Keys let scripts and other tools call the wakeci API.</p>
            </div>
            <div
                v-if="!fetchingDone"
                class="keys-empty"
            >
                <span class="status-ring keys-spinner"></span>
            </div>
        </div>
    </div>
</template>

<script>
import KeyUsageChart from "@/components/KeyUsageChart.vue";
import UiDialog from "@/components/ui/UiDialog.vue";
import { escapeHtml } from "@/store/utils";
import axios from "axios";
import TimeAgo from "javascript-time-ago";
import en from "javascript-time-ago/locale/en.json";

TimeAgo.addLocale(en);
const timeAgo = new TimeAgo("en-US");

export default {
    components: { KeyUsageChart, UiDialog },
    data: function () {
        return {
            keys: [],
            // The server's date, where the usage charts end
            today: "",
            // The admin password sees and revokes every key, an SSO user only their own
            admin: false,
            fetchingDone: false,
            revoking: null,
            // The section of the person whose access is being revoked
            revokingPerson: null,
            // Everyone with a key or signed in with SSO, from the server
            people: [],
            // Lowercased emails of the people signed in with SSO
            signedIn: [],
            // Narrows the admin's list by person, key name or key ID
            search: "",
        };
    },
    computed: {
        // The keys by who made them for the admin password: those made with
        // the admin password, then each person, also people signed in
        // without keys, so they can be signed out. Anyone else gets their own
        // keys in one section without a heading.
        sections() {
            if (!this.admin) {
                return this.keys.length > 0 ? [{ id: "mine", person: "", title: "", keys: this.keys }] : [];
            }
            const sections = [];
            const byPassword = this.keys.filter((key) => key.createdBy === "");
            if (byPassword.length > 0) {
                sections.push({ id: "admin-password", person: "", title: "Made with the admin password", keys: byPassword });
            }
            for (const person of this.people) {
                const lower = person.toLowerCase();
                sections.push({
                    id: person,
                    person,
                    title: person,
                    keys: this.keys.filter((key) => key.createdBy.toLowerCase() === lower),
                    signedIn: this.signedIn.includes(lower),
                });
            }
            const search = this.search.trim().toLowerCase();
            if (search === "") {
                return sections;
            }
            return sections.filter(
                (section) =>
                    section.title.toLowerCase().includes(search) ||
                    section.keys.some((key) => key.name.toLowerCase().includes(search) || key.id.includes(search)),
            );
        },
    },
    mounted() {
        this.$store.commit("SET_CURRENT_PAGE", "API keys");
        this.fetch();
    },
    methods: {
        fetch() {
            axios
                .get("/api/keys/")
                .then((response) => {
                    this.keys = response.data.keys || [];
                    this.today = response.data.today || "";
                    this.admin = response.data.admin;
                    this.people = response.data.people || [];
                    this.signedIn = response.data.signedIn || [];
                    this.fetchingDone = true;
                })
                .catch(() => {
                    this.fetchingDone = true;
                });
        },
        revoke() {
            const key = this.revoking;
            this.revoking = null;
            axios
                .delete("/api/keys/" + encodeURIComponent(key.id))
                .then(() => {
                    this.$notify({
                        text: "Key revoked",
                        type: "primary",
                    });
                    this.fetch();
                })
                .catch(() => {});
        },
        revokePerson() {
            const person = this.revokingPerson.person;
            this.revokingPerson = null;
            axios
                .delete("/api/keys/", { params: { createdBy: person } })
                .then((response) => {
                    this.$notify({
                        text: `Signed out ${escapeHtml(person)} and revoked ${response.data.keys} keys`,
                        type: "primary",
                    });
                    this.fetch();
                })
                .catch(() => {});
        },
        jobsSummary(key) {
            if (key.allJobs) {
                return "All jobs";
            }
            return [...key.jobs, ...key.patterns].join(", ");
        },
        isExpired(key) {
            return Boolean(key.expiresAt) && new Date(key.expiresAt) <= new Date();
        },
        expiryText(key) {
            if (!key.expiresAt) {
                return "Never expires";
            }
            return (this.isExpired(key) ? "Expired " : "Expires ") + this.ago(key.expiresAt);
        },
        ago(date) {
            return timeAgo.format(new Date(date));
        },
        formatDate(date) {
            return date ? new Date(date).toLocaleString() : "";
        },
    },
};
</script>

<style scoped>
.key-search {
    flex: 0 1 22rem;
}

.section-head {
    padding: 0 var(--space-2) var(--space-1);
}

.key-list {
    border-block-start: 1px solid var(--outline-variant);
}

.key {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-3) var(--space-2);
    border-block-end: 1px solid var(--outline-variant);
}

/* So long names and several scopes wrap instead of pushing the button off a
   narrow screen */
.details {
    flex: 1 1 auto;
    min-inline-size: 0;
    overflow-wrap: anywhere;
}

.scope {
    padding: 0 6px;
    border: 1px solid var(--outline-variant);
    border-radius: 4px;
}

.revoke {
    flex: none;
}

.stats {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-end;
    gap: 0 var(--space-2);
}

/* One click selects the whole ID, to copy it */
.key-id {
    font-size: 0.85em;
    user-select: all;
}

.error-text {
    color: var(--error);
}

.keys-empty {
    align-items: center;
    padding: var(--space-8) 0;
    display: flex;
    flex-direction: column;
    text-align: center;
}

.keys-spinner {
    inline-size: 28px;
    block-size: 28px;
    --_c: var(--primary);
}

@media (max-width: 600px) {
    .wide-label {
        display: none;
    }
}
</style>
