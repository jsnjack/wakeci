<template>
    <nav class="no-margin">
        <small
            v-if="user"
            class="muted"
            data-cy="whoami"
        >
            Signed in as <b>{{ user }}</b>
        </small>
        <div class="max"></div>
        <router-link
            to="/keys/new"
            class="button small-round"
            data-cy="create-key-open"
        >
            <i>add_circle</i>
            Create API key
        </router-link>
    </nav>

    <div
        class="article"
        data-cy="keys-list"
    >
        <div
            v-for="key in keys"
            :key="key.id"
            class="row medium-padding round large-text small-margin"
            style="background-color: var(--surface-container)"
            :data-cy="'key-' + key.name"
        >
            <div class="max">
                <div class="key-name">
                    {{ key.name }}
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
                        :title="(key.jobs || []).join(', ')"
                        data-cy="key-jobs"
                    >
                        {{ jobsSummary(key) }}
                    </span>
                    ·
                    <span :title="formatDate(key.createdAt)">Created {{ ago(key.createdAt) }}</span>
                    ·
                    <span :title="formatDate(key.lastUsedAt)">{{ key.lastUsedAt ? "Used " + ago(key.lastUsedAt) : "Never used" }}</span>
                    ·
                    <span :title="formatDate(key.expiresAt)">{{ key.expiresAt ? "Expires " + ago(key.expiresAt) : "Never expires" }}</span>
                </small>
            </div>
            <button
                class="circle transparent"
                data-cy="revoke-key"
                @click.prevent="revoking = key"
            >
                <i>delete</i>
                <div class="tooltip bottom">Revoke</div>
            </button>
        </div>

        <div
            v-if="keys.length === 0 && fetchingDone"
            class="medium-height middle-align center-align"
        >
            <div class="center-align">
                <i class="extra">key</i>
                <h5>No API keys yet</h5>
                <p class="muted">Keys let scripts and other tools call the wakeci API.</p>
            </div>
        </div>
    </div>

    <dialog
        :class="{ active: revoking !== null }"
        class="padding"
    >
        <div>
            Revoke <b>{{ revoking && revoking.name }}</b>? Anything using it stops working immediately.
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
</template>

<script>
import { escapeHtml } from "@/store/utils";
import axios from "axios";
import TimeAgo from "javascript-time-ago";
import en from "javascript-time-ago/locale/en.json";

// addLocale, unlike addDefaultLocale, is safe to repeat across components
TimeAgo.addLocale(en);
const timeAgo = new TimeAgo("en-US");

export default {
    data: function () {
        return {
            user: "",
            keys: [],
            fetchingDone: false,
            revoking: null,
        };
    },
    mounted() {
        this.$store.commit("SET_CURRENT_PAGE", "API keys");
        this.fetchUser();
        this.fetch();
    },
    methods: {
        fetchUser() {
            axios
                .get("/auth/_isLoggedIn")
                .then((response) => {
                    this.user = response.data.user;
                })
                .catch((error) => {});
        },
        fetch() {
            axios
                .get("/api/keys/")
                .then((response) => {
                    this.keys = response.data || [];
                })
                .catch((error) => {})
                .finally(() => {
                    this.fetchingDone = true;
                });
        },
        revoke() {
            const key = this.revoking;
            this.revoking = null;
            axios
                .delete("/api/keys/" + encodeURIComponent(key.id))
                .then((response) => {
                    this.keys = this.keys.filter((k) => k.id !== key.id);
                    this.$notify({
                        text: `${escapeHtml(key.name)} has been revoked`,
                        type: "primary",
                    });
                })
                .catch((error) => {});
        },
        jobsSummary(key) {
            const jobs = key.jobs || [];
            if (jobs.length === 0) {
                return "All jobs";
            }
            const shown = jobs.slice(0, 3).join(", ");
            return jobs.length > 3 ? `${shown} +${jobs.length - 3} more` : shown;
        },
        ago(value) {
            return timeAgo.format(new Date(value));
        },
        formatDate(value) {
            return value ? new Date(value).toLocaleString() : "";
        },
    },
};
</script>

<style scoped>
.muted {
    color: var(--on-surface-variant);
}
dialog {
    max-inline-size: 36rem;
}
.key-name {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.5rem;
}
</style>
