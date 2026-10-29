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

    <nav class="no-margin">
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
            v-for="key in keys"
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
            v-if="keys.length === 0 && fetchingDone"
            class="medium-height middle-align center-align"
        >
            <div class="center-align">
                <i class="extra">key</i>
                <h5>No API keys yet</h5>
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
import axios from "axios";
import TimeAgo from "javascript-time-ago";
import en from "javascript-time-ago/locale/en.json";

TimeAgo.addLocale(en);
const timeAgo = new TimeAgo("en-US");

export default {
    data: function () {
        return {
            keys: [],
            fetchingDone: false,
            revoking: null,
        };
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
                    this.keys = response.data || [];
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
