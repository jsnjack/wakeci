<template>
    <form
        v-if="!token"
        novalidate
        @submit.prevent="create"
    >
        <div class="grid">
            <div class="s12 m8">
                <div
                    class="field label border"
                    :class="{ invalid: nameError }"
                >
                    <input
                        id="key-name"
                        ref="nameInput"
                        v-model="name"
                        type="text"
                        maxlength="100"
                        placeholder=" "
                        data-cy="key-name"
                        @input="nameError = false"
                    />
                    <label for="key-name">Name</label>
                    <output
                        v-if="nameError"
                        class="invalid"
                    >
                        Give the key a name
                    </output>
                </div>
            </div>
            <div class="s12 m4">
                <div class="field label suffix border">
                    <select
                        id="key-expiry"
                        v-model="expiresInDays"
                        data-cy="key-expiry"
                    >
                        <option value="30">30 days</option>
                        <option value="90">90 days</option>
                        <option value="365">1 year</option>
                        <option value="0">Never</option>
                    </select>
                    <label for="key-expiry">Expires after</label>
                    <i>arrow_drop_down</i>
                </div>
            </div>
        </div>

        <AccessPicker
            ref="access"
            v-model="grant"
            :scope-names="scopeNames"
            :all-jobs-hint="auth.admin ? undefined : 'All jobs your groups allow, including jobs added later'"
        />

        <nav class="right-align">
            <router-link
                to="/keys"
                class="button border"
            >
                Cancel
            </router-link>
            <button
                type="submit"
                data-cy="create-key"
                :disabled="creating"
            >
                Create key
            </button>
        </nav>
    </form>

    <template v-else>
        <h5>Key created</h5>
        <p>Copy it now. It won't be shown again.</p>
        <nav>
            <div class="field label border max">
                <input
                    id="key-token"
                    ref="tokenInput"
                    readonly
                    class="token"
                    placeholder=" "
                    :value="token"
                    data-cy="new-key-token"
                    @focus="$event.target.select()"
                />
                <label for="key-token">{{ createdKey.name }}</label>
            </div>
            <button
                type="button"
                class="circle"
                aria-label="Copy API key"
                data-cy="copy-key"
                @click="copy"
            >
                <i>{{ copied ? "check" : "content_copy" }}</i>
                <div class="tooltip bottom">{{ copied ? "Copied" : "Copy" }}</div>
            </button>
        </nav>
        <p
            v-if="copyFailed"
            class="error-text"
            role="alert"
        >
            Copying isn't available here. The key is selected, so press Ctrl+C
        </p>
        <span
            class="visually-hidden"
            role="status"
        >
            {{ copied ? "Copied" : "" }}
        </span>
        <p>Send it as a Bearer token, for example:</p>
        <pre
            class="small-padding round"
            data-cy="new-key-example"
            >{{ example }}</pre>
        <nav class="right-align">
            <router-link
                to="/keys"
                class="button"
                data-cy="new-key-done"
            >
                Done
            </router-link>
        </nav>
    </template>
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
.token {
    font-family: monospace;
}
.visually-hidden {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip-path: inset(50%);
    white-space: nowrap;
}
pre {
    background-color: var(--surface-container);
    white-space: pre-wrap;
    word-break: break-all;
}
</style>
