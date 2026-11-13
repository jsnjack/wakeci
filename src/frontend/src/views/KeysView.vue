<template>
    <dialog
        :class="{ active: revoking !== null }"
        class="padding"
    >
        <div v-if="revoking">
            Revoke <b>{{ revoking.name }}</b
            >? Anything using it stops working at once.
        </div>
        <nav class="right-align">
            <button
                class="border"
                @click.prevent="revoking = null"
            >
                Cancel
            </button>
            <button
                data-cy="revoke-key-confirm"
                @click.prevent="revoke"
            >
                Revoke
            </button>
        </nav>
    </dialog>

    <dialog
        :class="{ active: revokingPerson !== null }"
        class="padding"
    >
        <div v-if="revokingPerson">
            Revoke access of <b>{{ revokingPerson.person }}</b
            >? This signs them out and revokes their {{ revokingPerson.keys.length }} keys. Anything using the keys stops working at once.
        </div>
        <nav class="right-align">
            <button
                class="border"
                @click.prevent="revokingPerson = null"
            >
                Cancel
            </button>
            <button
                data-cy="revoke-person-confirm"
                @click.prevent="revokePerson"
            >
                Revoke access
            </button>
        </nav>
    </dialog>

    <nav class="no-margin">
        <div
            v-if="admin && people.length > 4"
            class="field small prefix border round no-margin"
        >
            <i>search</i>
            <input
                v-model="search"
                type="search"
                aria-label="Search people and keys"
                placeholder="Search people and keys"
                data-cy="key-search"
            />
        </div>
        <div class="max"></div>
        <router-link
            to="/keys/new"
            class="button small-round"
            data-cy="create-key-open"
        >
            <i>add_circle</i>
            <span>Create API key</span>
        </router-link>
    </nav>

    <div
        class="article"
        data-cy="keys-list"
    >
        <!-- The admin password sees every key, by who made it; anyone else sees their own -->
        <section
            v-for="section in sections"
            :key="section.id"
            :data-cy="'key-group-' + section.id"
        >
            <nav
                v-if="admin"
                class="section-head"
            >
                <div class="max">
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
                    class="border small-round"
                    data-cy="revoke-person"
                    @click.prevent="revokingPerson = section"
                >
                    <i>person_remove</i>
                    <span>Revoke access</span>
                </button>
            </nav>
            <div
                v-for="key in section.keys"
                :key="key.id"
                class="row medium-padding round small-margin key"
                :data-cy="'key-' + key.name"
            >
                <div class="max details">
                    <div class="row small-space chips">
                        <b>{{ key.name }}</b>
                        <span
                            v-for="scope in key.scopes"
                            :key="scope"
                            class="chip small no-margin"
                        >
                            {{ scope }}
                        </span>
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
                    class="border small-round revoke"
                    :aria-label="'Revoke ' + key.name"
                    data-cy="revoke-key"
                    @click.prevent="revoking = key"
                >
                    <i>key_off</i>
                    <span class="m l">Revoke key</span>
                </button>
            </div>
        </section>

        <div
            v-if="sections.length === 0 && fetchingDone"
            class="medium-height middle-align center-align"
        >
            <div class="center-align">
                <i class="extra">key</i>
                <h5 v-if="search.trim() === ''">No API keys yet</h5>
                <h5 v-else>Nothing matches "{{ search.trim() }}"</h5>
                <p>Keys let scripts and other tools call the wakeci API.</p>
            </div>
        </div>
        <div
            v-if="!fetchingDone"
            class="medium-height middle-align center-align"
        >
            <progress class="circle large"></progress>
        </div>
    </div>
</template>

<script>
import KeyUsageChart from "@/components/KeyUsageChart.vue";
import { escapeHtml } from "@/store/utils";
import axios from "axios";
import TimeAgo from "javascript-time-ago";
import en from "javascript-time-ago/locale/en.json";

TimeAgo.addLocale(en);
const timeAgo = new TimeAgo("en-US");

export default {
    components: { KeyUsageChart },
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
            // Narrows the admin's list by person or key name
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
                (section) => section.title.toLowerCase().includes(search) || section.keys.some((key) => key.name.toLowerCase().includes(search)),
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
.key {
    background-color: var(--surface-container);
}
.section-head {
    margin: 1rem 0 0.25rem;
}
/* So long names and several scopes wrap instead of pushing the button off a
   narrow screen */
.details {
    min-width: 0;
    overflow-wrap: anywhere;
}
.chips {
    flex-wrap: wrap;
}
.revoke {
    flex: none;
}
.stats {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-end;
    gap: 0 0.5rem;
    margin-top: 0.25rem;
}
.muted {
    opacity: 0.7;
}
</style>
