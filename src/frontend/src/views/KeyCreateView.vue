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

        <h6 class="small">Scopes</h6>
        <ul class="list border">
            <li
                v-for="scope in scopes"
                :key="scope.name"
            >
                <label class="checkbox">
                    <input
                        v-model="selectedScopes"
                        type="checkbox"
                        :value="scope.name"
                        :data-cy="'scope-' + scope.name"
                        @change="scopeError = false"
                    />
                    <span>
                        <span class="option-text">
                            <b>{{ scope.name }}</b>
                            <small class="muted">{{ scope.description }}</small>
                        </span>
                    </span>
                </label>
            </li>
        </ul>
        <p
            v-if="scopeError"
            class="error-text"
            role="alert"
        >
            Select at least one scope
        </p>

        <h6 class="small">Jobs</h6>
        <ul class="list border">
            <li>
                <label class="radio">
                    <input
                        v-model="jobAccess"
                        type="radio"
                        value="all"
                        data-cy="jobs-all"
                    />
                    <span>
                        <span class="option-text">
                            All jobs
                            <small class="muted">Including jobs added later</small>
                        </span>
                    </span>
                </label>
            </li>
            <li>
                <label class="radio">
                    <input
                        v-model="jobAccess"
                        type="radio"
                        value="selected"
                        data-cy="jobs-selected"
                        :disabled="writeSelected"
                    />
                    <span>
                        <span class="option-text">
                            Only selected jobs
                            <small
                                v-if="writeSelected"
                                class="muted"
                            >
                                Write can run any command on the server, so it covers all jobs
                            </small>
                            <small
                                v-else
                                class="muted"
                            >
                                Pick jobs below, or match them by name
                            </small>
                        </span>
                    </span>
                </label>
            </li>
        </ul>

        <template v-if="jobAccess === 'selected'">
            <div
                class="field label prefix border"
                :class="{ invalid: patternError !== '' }"
            >
                <i>pattern</i>
                <input
                    id="key-patterns"
                    v-model="patternText"
                    type="text"
                    placeholder=" "
                    data-cy="key-patterns"
                    @input="jobsError = false"
                    @keydown.enter.prevent
                />
                <label for="key-patterns">Name patterns</label>
                <output
                    v-if="patternError"
                    class="invalid"
                >
                    {{ patternError }}
                </output>
                <output v-else>For example test_*, vrec_*. Jobs added later that match are included</output>
            </div>
            <nav>
                <div class="field small prefix border round max">
                    <i>search</i>
                    <input
                        v-model="search"
                        type="search"
                        aria-label="Search jobs"
                        placeholder="Search jobs"
                        data-cy="key-job-search"
                        @keydown.enter.prevent
                    />
                </div>
                <small
                    :class="{ 'error-text': jobsError }"
                    aria-live="polite"
                    data-cy="key-jobs-count"
                >
                    <template v-if="jobsError">Select a job or enter a pattern</template>
                    <template v-else>{{ selectedCount }} of {{ jobs.length }} selected</template>
                </small>
            </nav>
            <!-- Scrolls on its own, so a long job list doesn't push Create off screen -->
            <ul
                class="list border jobs"
                data-cy="key-jobs"
            >
                <li
                    v-for="job in shownJobs"
                    :key="job.name"
                    :data-cy="'key-job-' + job.name"
                >
                    <label class="checkbox max">
                        <input
                            type="checkbox"
                            :checked="isSelected(job.name)"
                            :disabled="matchedBy(job.name) !== ''"
                            @change="toggleJob(job.name, $event.target.checked)"
                        />
                        <span>
                            <span class="option-text">
                                {{ job.name }}
                                <small
                                    v-if="job.desc"
                                    class="muted"
                                >
                                    {{ job.desc }}
                                </small>
                            </span>
                        </span>
                    </label>
                    <span
                        v-if="matchedBy(job.name)"
                        class="chip small no-margin"
                        :title="'Matches ' + matchedBy(job.name)"
                    >
                        {{ matchedBy(job.name) }}
                    </span>
                </li>
                <li v-if="jobsLoaded && jobs.length === 0">No jobs yet. Patterns also cover jobs added later.</li>
                <li v-else-if="jobsLoaded && shownJobs.length === 0">No jobs match "{{ search.trim() }}"</li>
            </ul>
        </template>

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
import { matchJobPattern } from "@/store/utils";
import axios from "axios";

export default {
    data: function () {
        return {
            name: "",
            expiresInDays: "90",
            selectedScopes: ["read"],
            jobAccess: "all",
            patternText: "",
            // Only narrows the list shown, never what is selected
            search: "",
            // Jobs picked one by one
            pickedJobs: [],
            jobs: [],
            jobsLoaded: false,
            nameError: false,
            scopeError: false,
            jobsError: false,
            creating: false,
            token: "",
            createdKey: null,
            // Where the job choice goes back to when write is unticked
            jobAccessBeforeWrite: "all",
            copied: false,
            copyFailed: false,
            origin: window.location.origin,
            scopes: [
                { name: "read", description: "View jobs, the feed, builds, logs and artifacts" },
                { name: "run", description: "Run jobs, start and abort builds" },
                {
                    name: "write",
                    description: "Create and edit jobs. Runs commands on the server, so it covers all jobs",
                },
            ],
        };
    },
    computed: {
        writeSelected() {
            return this.selectedScopes.includes("write");
        },
        patterns() {
            return [...new Set(this.patternText.split(/[\s,]+/).filter((p) => p !== ""))];
        },
        patternError() {
            const bad = this.patterns.find((p) => p.includes("/") || p.includes("\\"));
            return bad ? `Patterns can't contain / or \\: ${bad}` : "";
        },
        shownJobs() {
            const search = this.search.trim().toLowerCase();
            if (search === "") {
                return this.jobs;
            }
            return this.jobs.filter((job) => job.name.toLowerCase().includes(search));
        },
        selectedCount() {
            return this.jobs.filter((job) => this.isSelected(job.name)).length;
        },
        // A request the new key is allowed to make
        example() {
            const curl = `curl -H "Authorization: Bearer ${this.token}"`;
            const scopes = this.createdKey.scopes;
            if (scopes.includes("read")) {
                return `${curl} ${this.origin}/api/feed`;
            }
            if (scopes.includes("run")) {
                const job = this.createdKey.jobs[0] || "JOB_NAME";
                return `${curl} -X POST ${this.origin}/api/job/${encodeURIComponent(job)}/run`;
            }
            return `${curl} -X POST -d name=NEW_JOB ${this.origin}/api/jobs/create`;
        },
    },
    watch: {
        writeSelected(selected) {
            if (selected) {
                this.jobAccessBeforeWrite = this.jobAccess;
                this.jobAccess = "all";
            } else {
                this.jobAccess = this.jobAccessBeforeWrite;
            }
        },
    },
    mounted() {
        this.$store.commit("SET_CURRENT_PAGE", "New API key");
        this.$refs.nameInput.focus();
        axios
            .get("/api/jobs/")
            .then((response) => {
                this.jobs = response.data || [];
                this.jobsLoaded = true;
            })
            .catch((error) => {
                this.jobsLoaded = true;
            });
    },
    methods: {
        matchedBy(name) {
            return this.patterns.find((p) => matchJobPattern(p, name)) || "";
        },
        isSelected(name) {
            return this.pickedJobs.includes(name) || this.matchedBy(name) !== "";
        },
        toggleJob(name, checked) {
            this.jobsError = false;
            if (checked) {
                this.pickedJobs.push(name);
            } else {
                this.pickedJobs = this.pickedJobs.filter((n) => n !== name);
            }
        },
        validate() {
            this.nameError = this.name.trim() === "";
            this.scopeError = this.selectedScopes.length === 0;
            // The job fields only count while they are shown
            const selected = this.jobAccess === "selected";
            this.jobsError = selected && this.pickedJobs.length === 0 && this.patterns.length === 0;
            return !this.nameError && !this.scopeError && !this.jobsError && !(selected && this.patternError);
        },
        create() {
            if (this.creating || !this.validate()) {
                return;
            }
            const data = new URLSearchParams();
            data.append("name", this.name.trim());
            data.append("expiresInDays", this.expiresInDays);
            for (const scope of this.selectedScopes) {
                data.append("scope", scope);
            }
            data.append("allJobs", this.jobAccess === "all" ? "true" : "false");
            if (this.jobAccess === "selected") {
                for (const job of this.pickedJobs) {
                    data.append("job", job);
                }
                for (const pattern of this.patterns) {
                    data.append("pattern", pattern);
                }
            }
            // Until the response arrives, so a double click can't create two keys
            this.creating = true;
            axios
                .post("/api/keys/", data)
                .then((response) => {
                    this.createdKey = response.data.key;
                    this.token = response.data.token;
                })
                .catch((error) => {})
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
.option-text {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    padding-inline-start: 0.5rem;
}
.muted {
    opacity: 0.7;
}
.jobs {
    max-height: 20rem;
    overflow-y: auto;
}
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
