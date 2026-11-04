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
        :class="{ active: revokingPerson }"
        class="padding"
    >
        <div>
            Revoke access of <b>{{ createdBy }}</b
            >? This signs them out and revokes their {{ shownKeys.length }} keys. Anything using the keys stops working at once.
        </div>
        <nav class="right-align">
            <button
                class="border"
                @click.prevent="revokingPerson = false"
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
        <!-- Only the admin password sees other people's keys -->
        <template v-if="admin">
            <div class="field label suffix border small no-margin">
                <select
                    id="key-created-by"
                    v-model="createdBy"
                    data-cy="key-created-by"
                >
                    <option value="">Everyone</option>
                    <option
                        v-for="person in people"
                        :key="person"
                        :value="person"
                    >
                        {{ person }}
                    </option>
                    <option :value="passwordFilter">Admin password</option>
                </select>
                <label for="key-created-by">Created by</label>
                <i>arrow_drop_down</i>
            </div>
            <button
                v-if="createdBy !== '' && createdBy !== passwordFilter"
                class="border small-round"
                data-cy="revoke-person"
                @click.prevent="revokingPerson = true"
            >
                <i>person_remove</i>
                <span>Revoke access of {{ createdBy }}</span>
            </button>
        </template>
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
        <div
            v-for="key in shownKeys"
            :key="key.id"
            class="row medium-padding round small-margin key"
            :data-cy="'key-' + key.name"
        >
            <div class="max">
                <div class="row small-space">
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
                    <template v-if="admin">
                        <span data-cy="key-created-by-text">by {{ key.createdBy || "admin password" }}</span>
                        ·
                    </template>
                    <span
                        :title="jobsSummary(key)"
                        data-cy="key-jobs"
                    >
                        {{ jobsSummary(key) }}
                    </span>
                    ·
                    <span :title="formatDate(key.createdAt)">Created {{ ago(key.createdAt) }}</span>
                    ·
                    <span :title="formatDate(key.lastUsedAt)">
                        {{ key.lastUsedAt ? "Used " + ago(key.lastUsedAt) : "Never used" }}
                    </span>
                    ·
                    <span
                        :title="formatDate(key.expiresAt)"
                        data-cy="key-expiry"
                    >
                        {{ expiryText(key) }}
                    </span>
                </small>
            </div>
            <span
                v-if="isExpired(key)"
                class="error-text"
            >
                Expired
            </span>
            <button
                class="circle transparent"
                :aria-label="'Revoke ' + key.name"
                data-cy="revoke-key"
                @click.prevent="revoking = key"
            >
                <i>delete</i>
                <div class="tooltip bottom">Revoke</div>
            </button>
        </div>

        <div
            v-if="shownKeys.length === 0 && fetchingDone"
            class="medium-height middle-align center-align"
        >
            <div class="center-align">
                <i class="extra">key</i>
                <h5 v-if="createdBy === ''">No API keys yet</h5>
                <h5 v-else>No API keys here</h5>
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
import { escapeHtml } from "@/store/utils";
import axios from "axios";
import TimeAgo from "javascript-time-ago";
import en from "javascript-time-ago/locale/en.json";

TimeAgo.addLocale(en);
const timeAgo = new TimeAgo("en-US");

export default {
    data: function () {
        return {
            keys: [],
            // The admin password sees and revokes every key, an SSO user only their own
            admin: false,
            createdBy: "",
            // Filters the keys made with the admin password; no email looks like this
            passwordFilter: ":password",
            fetchingDone: false,
            revoking: null,
            revokingPerson: false,
            // Everyone with a key or signed in with SSO, from the server
            people: [],
        };
    },
    computed: {
        shownKeys() {
            if (this.createdBy === "") {
                return this.keys;
            }
            const createdBy = this.createdBy === this.passwordFilter ? "" : this.createdBy.toLowerCase();
            return this.keys.filter((key) => key.createdBy.toLowerCase() === createdBy);
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
                    this.admin = response.data.admin;
                    this.people = response.data.people || [];
                    if (this.createdBy !== this.passwordFilter && !this.people.includes(this.createdBy)) {
                        this.createdBy = "";
                    }
                    this.fetchingDone = true;
                })
                .catch((error) => {
                    this.fetchingDone = true;
                });
        },
        revoke() {
            const key = this.revoking;
            this.revoking = null;
            axios
                .delete("/api/keys/" + encodeURIComponent(key.id))
                .then((response) => {
                    this.$notify({
                        text: "Key revoked",
                        type: "primary",
                    });
                    this.fetch();
                })
                .catch((error) => {});
        },
        revokePerson() {
            const person = this.createdBy;
            this.revokingPerson = false;
            axios
                .delete("/api/keys/", { params: { createdBy: person } })
                .then((response) => {
                    this.$notify({
                        text: `Signed out ${escapeHtml(person)} and revoked ${response.data.keys} keys`,
                        type: "primary",
                    });
                    this.fetch();
                })
                .catch((error) => {});
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
.muted {
    opacity: 0.7;
}
</style>
