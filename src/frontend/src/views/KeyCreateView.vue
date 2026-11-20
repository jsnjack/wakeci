<template>
    <div class="ui key-create stack gap-4">
        <form
            v-if="!token"
            class="stack gap-4"
            novalidate
            @submit.prevent="create"
        >
            <div class="key-fields">
                <label class="form-field">
                    <span class="label-text">Name</span>
                    <input
                        id="key-name"
                        ref="nameInput"
                        v-model="name"
                        class="input"
                        :class="{ invalid: nameError }"
                        type="text"
                        maxlength="100"
                        data-cy="key-name"
                        :aria-invalid="nameError ? 'true' : null"
                        @input="nameError = false"
                    />
                    <span
                        v-if="nameError"
                        class="hint error-text"
                        >Give the key a name</span
                    >
                </label>
                <label class="form-field">
                    <span class="label-text">Expires after</span>
                    <select
                        id="key-expiry"
                        v-model="expiresInDays"
                        class="select"
                        data-cy="key-expiry"
                    >
                        <option value="30">30 days</option>
                        <option value="90">90 days</option>
                        <option value="365">1 year</option>
                        <option value="0">Never</option>
                    </select>
                </label>
            </div>

            <AccessPicker
                ref="access"
                v-model="grant"
                :scope-names="scopeNames"
                :all-jobs-hint="auth.admin ? undefined : 'All jobs your groups allow, including jobs added later'"
            />

            <div class="cluster end gap-2">
                <router-link
                    to="/keys"
                    class="btn btn-ghost"
                >
                    Cancel
                </router-link>
                <button
                    type="submit"
                    class="btn btn-primary"
                    data-cy="create-key"
                    :disabled="creating"
                >
                    Create key
                </button>
            </div>
        </form>

        <template v-else>
            <h1>Key created</h1>
            <p>Copy it now. It won't be shown again.</p>
            <label class="form-field">
                <span class="label-text">{{ createdKey.name }}</span>
                <span class="cluster nowrap gap-1">
                    <input
                        id="key-token"
                        ref="tokenInput"
                        readonly
                        class="input token"
                        :value="token"
                        data-cy="new-key-token"
                        @focus="$event.target.select()"
                    />
                    <button
                        type="button"
                        class="btn btn-ghost btn-icon tip-end"
                        aria-label="Copy API key"
                        :data-tip="copied ? 'Copied' : 'Copy'"
                        data-cy="copy-key"
                        @click="copy"
                    >
                        <i class="icon">{{ copied ? "check" : "content_copy" }}</i>
                    </button>
                </span>
            </label>
            <p
                v-if="copyFailed"
                class="error-text"
                role="alert"
            >
                Copying isn't available here. The key is selected, so press Ctrl+C
            </p>
            <span
                class="sr-only"
                role="status"
            >
                {{ copied ? "Copied" : "" }}
            </span>
            <p>Send it as a Bearer token, for example:</p>
            <pre
                class="example"
                data-cy="new-key-example"
                >{{ example }}</pre>
            <div class="cluster end">
                <router-link
                    to="/keys"
                    class="btn btn-primary"
                    data-cy="new-key-done"
                >
                    Done
                </router-link>
            </div>
        </template>
    </div>
</template>

<script>
import AccessPicker from "@/components/AccessPicker.vue";
import axios from "axios";
import vuex from "vuex";

export default {
    components: { AccessPicker },
    data: function () {
        return {
            name: "",
            expiresInDays: "90",
            grant: {
                // The first scope offered, read for the admin password
                scopes: ["read", "run", "write"].filter((scope) => this.$store.getters.canAny(scope)).slice(0, 1),
                allJobs: true,
                jobs: [],
                patterns: [],
            },
            nameError: false,
            creating: false,
            token: "",
            createdKey: null,
            copied: false,
            copyFailed: false,
            origin: window.location.origin,
        };
    },
    computed: {
        ...vuex.mapState(["auth"]),
        // A key can't do more than the person who creates it
        scopeNames() {
            return ["read", "run", "write"].filter((scope) => this.$store.getters.canAny(scope));
        },
        // A request the new key is allowed to make
        example() {
            const curl = `curl -H "Authorization: Bearer ${this.token}"`;
            // Single-quoted for the shell: encodeURIComponent leaves ( ) and '
            // as they are, and %27 stands in for ' inside the quotes
            const url = (path) => `'${(this.origin + path).replaceAll("'", "%27")}'`;
            const scopes = this.createdKey.scopes;
            if (scopes.includes("read")) {
                return `${curl} ${url("/api/feed")}`;
            }
            if (scopes.includes("run")) {
                const job = this.createdKey.jobs[0] || "JOB_NAME";
                return `${curl} -X POST ${url(`/api/job/${encodeURIComponent(job)}/run`)}`;
            }
            return `${curl} -X POST -d name=NEW_JOB ${url("/api/jobs/create")}`;
        },
    },
    mounted() {
        this.$store.commit("SET_CURRENT_PAGE", "New API key");
        this.$refs.nameInput.focus();
    },
    methods: {
        validate() {
            this.nameError = this.name.trim() === "";
            // Both, so every mistake shows at once
            const accessValid = this.$refs.access.validate();
            return !this.nameError && accessValid;
        },
        create() {
            if (this.creating || !this.validate()) {
                return;
            }
            const data = new URLSearchParams();
            data.append("name", this.name.trim());
            data.append("expiresInDays", this.expiresInDays);
            for (const scope of this.grant.scopes) {
                data.append("scope", scope);
            }
            data.append("allJobs", this.grant.allJobs ? "true" : "false");
            for (const job of this.grant.jobs) {
                data.append("job", job);
            }
            for (const pattern of this.grant.patterns) {
                data.append("pattern", pattern);
            }
            // Until the response arrives, so a double click can't create two keys
            this.creating = true;
            axios
                .post("/api/keys/", data)
                .then((response) => {
                    this.createdKey = response.data.key;
                    this.token = response.data.token;
                })
                .catch(() => {})
                .finally(() => {
                    this.creating = false;
                });
        },
        copy() {
            // The Clipboard API only exists on https and localhost
            if (navigator.clipboard && window.isSecureContext) {
                navigator.clipboard.writeText(this.token).then(
                    () => {
                        this.copied = true;
                    },
                    () => this.copyFallback(),
                );
            } else {
                this.copyFallback();
            }
        },
        copyFallback() {
            this.$refs.tokenInput.select();
            this.copyFailed = true;
        },
    },
};
</script>

<style scoped>
.key-fields {
    display: grid;
    grid-template-columns: minmax(0, 2fr) minmax(10rem, 1fr);
    gap: var(--space-4);
}

.token {
    font-family: var(--font-mono);
}

.input.invalid {
    border-color: var(--error);
}

.error-text {
    color: var(--error);
}

.example {
    padding: var(--space-3);
    border-radius: var(--radius-control);
    background-color: var(--surface-container);
    white-space: pre-wrap;
    word-break: break-all;
    font-size: var(--text-sm);
}

@media (max-width: 600px) {
    .key-fields {
        grid-template-columns: 1fr;
    }
}
</style>
