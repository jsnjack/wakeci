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
                        @input="nameError = false"
                    />
                    <label>Name</label>
                    <output class="invalid">Give the key a name</output>
                </div>
            </div>
            <div class="s12 m4">
                <div class="field label border">
                    <select
                        id="key-expiry"
                        v-model="expiresInDays"
                    >
                        <option value="30">30 days</option>
                        <option value="90">90 days</option>
                        <option value="365">1 year</option>
                        <option value="0">Never</option>
                    </select>
                    <label>Expires after</label>
                </div>
            </div>
        </div>

        <h6 class="small bold">Scopes</h6>
        <ul class="list border">
            <li
                v-for="scope in scopes"
                :key="scope.name"
            >
                <label class="option">
                    <span class="checkbox">
                        <input
                            v-model="selectedScopes"
                            type="checkbox"
                            :value="scope.name"
                            :data-cy="'scope-' + scope.name"
                            @change="scopeError = false"
                        />
                        <span></span>
                    </span>
                    <span class="option-text">
                        {{ scope.name }}
                        <small class="muted">{{ scope.description }}</small>
                    </span>
                </label>
            </li>
        </ul>
        <small
            v-if="scopeError"
            class="error-text"
        >
            Select at least one scope
        </small>

        <h6 class="small bold">Jobs</h6>
        <ul class="list border">
            <li>
                <label class="option">
                    <span class="radio">
                        <input
                            v-model="jobAccess"
                            type="radio"
                            value="all"
                            data-cy="jobs-all"
                        />
                        <span></span>
                    </span>
                    <span class="option-text">
                        All jobs
                        <small class="muted">Including jobs added later</small>
                    </span>
                </label>
            </li>
            <li>
                <label class="option">
                    <span class="radio">
                        <input
                            v-model="jobAccess"
                            type="radio"
                            value="selected"
                            data-cy="jobs-selected"
                            :disabled="selectedScopes.includes('write')"
                            @change="focusPatterns"
                        />
                        <span></span>
                    </span>
                    <span class="option-text">
                        Only selected jobs
                        <small class="muted">Pick jobs below, or match them by name</small>
                    </span>
                </label>
                <p v-if="selectedScopes.includes('write')" class="muted">Write access can run commands on the server, so it covers all jobs.</p>
            </li>
        </ul>

        <template v-if="jobAccess === 'selected'">
            <div
                class="field label prefix border"
                :class="{ invalid: patternError !== '' }"
            >
                <i>checklist</i>
                <input
                    id="key-patterns"
                    ref="patternInput"
                    v-model="patternText"
                    type="text"
                    data-cy="job-patterns"
                    @input="jobsError = false"
                    @keydown.enter.prevent
                />
                <label>Name patterns</label>
                <output>For example test_*, vrec_*. * matches anything, and jobs added later that match are included too.</output>
                <output class="invalid">{{ patternError }}</output>
            </div>

            <nav>
                <div class="field small prefix border round max">
                    <i>search</i>
                    <input
                        v-model="search"
                        type="search"
                        placeholder="Search jobs"
                        data-cy="job-search"
                        @keydown.enter.prevent
                    />
                </div>
                <small
                    class="muted"
                    :class="{ 'error-text': jobsError }"
                    data-cy="jobs-count"
                >
                    <template v-if="jobsError">Select a job or enter a pattern</template>
                    <template v-else>{{ selectedCount }} of {{ jobs.length }} selected</template>
                </small>
            </nav>
            <!-- Scrolls on its own, so long job lists don't push Create off screen -->
            <ul
                class="list border jobs"
                data-cy="jobs-list"
            >
                <li
                    v-for="job in shownJobs"
                    :key="job.name"
                    :data-cy="'job-' + job.name"
                >
                    <label class="option">
                        <span class="checkbox">
                            <input
                                type="checkbox"
                                :checked="isSelected(job.name)"
                                :disabled="matchedBy(job.name) !== ''"
                                @change="toggleJob(job.name, $event.target.checked)"
                            />
                            <span></span>
                        </span>
                        <span class="option-text">
                            {{ job.name }}
                            <small
                                v-if="job.desc"
                                class="muted"
                            >
                                {{ job.desc }}
                            </small>
                        </span>
                        <span
                            v-if="matchedBy(job.name)"
                            class="chip small no-margin"
                            :title="'Matches ' + matchedBy(job.name)"
                        >
                            {{ matchedBy(job.name) }}
                        </span>
                    </label>
                </li>
                <li
                    v-if="jobs.length === 0 && jobsLoaded"
                    class="muted"
                >
                    No jobs yet. Patterns still cover jobs added later.
                </li>
                <li
                    v-else-if="shownJobs.length === 0 && jobsLoaded"
                    class="muted"
                >
                    No jobs match "{{ search.trim() }}"
                </li>
            </ul>
        </template>

        <nav class="right-align">
            <router-link
                to="/keys"
                class="button border"
            >
                Cancel
            </router-link>
            <!-- Only while the request is in flight, so it can't create two keys -->
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
        <p class="muted">Copy it now. For security, it won't be shown again.</p>
        <div class="field label suffix border">
            <input
                ref="tokenInput"
                readonly
                class="token"
                :value="token"
                data-cy="new-key-token"
                @focus="$event.target.select()"
            />
            <label>{{ name.trim() }}</label>
            <button
                type="button"
                class="transparent circle"
                title="Copy"
                aria-label="Copy API key"
                data-cy="copy-key"
                @click="copy"
            >
                <i aria-hidden="true">{{ copied ? "check" : "content_copy" }}</i>
            </button>
        </div>
        <p class="muted">Send it as a Bearer token, or as the basic auth password.</p>
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
            // Only narrows the list shown; it never changes what's selected
            search: "",
            // Jobs picked one by one
            selectedJobs: [],
            jobs: [],
            jobsLoaded: false,
            nameError: false,
            scopeError: false,
            jobsError: false,
            creating: false,
            token: "",
            copied: false,
            scopes: [
                { name: "read", description: "View the feed, jobs, builds, logs and artifacts" },
                { name: "run", description: "Run jobs, start and abort builds" },
                { name: "write", description: "Create and edit jobs that run commands on the server; requires access to all jobs" },
            ],
        };
    },
    computed: {
        patterns() {
            return [...new Set(this.patternText.split(/[\s,]+/).filter((p) => p !== ""))];
        },
        patternError() {
            if (this.patterns.some((p) => p.includes("/") || p.includes("\\"))) {
                return "Patterns can't contain / or \\";
            }
            if (this.patterns.some((p) => p.length > 200)) {
                return "Patterns can be at most 200 characters";
            }
            return "";
        },
        shownJobs() {
            const query = this.search.trim().toLowerCase();
            if (query === "") {
                return this.jobs;
            }
            return this.jobs.filter((job) => job.name.toLowerCase().includes(query));
        },
        selectedCount() {
            return this.jobs.filter((job) => this.isSelected(job.name)).length;
        },
    },
    watch: {
        selectedScopes(scopes) {
            if (scopes.includes("write")) { this.jobAccess = "all"; }
        },
    },
    mounted() {
        this.$store.commit("SET_CURRENT_PAGE", "New API key");
        this.$refs.nameInput.focus();
        axios
            .get("/api/jobs/")
            .then((response) => {
                this.jobs = response.data || [];
            })
            .catch((error) => {})
            .finally(() => {
                this.jobsLoaded = true;
            });
    },
    methods: {
        // matchedBy returns the first pattern that matches job name, or ""
        matchedBy(name) {
            return this.patterns.find((p) => matchJobPattern(p, name)) || "";
        },
        isSelected(name) {
            return this.matchedBy(name) !== "" || this.selectedJobs.includes(name);
        },
        toggleJob(name, checked) {
            this.jobsError = false;
            this.selectedJobs = this.selectedJobs.filter((n) => n !== name);
            if (checked) {
                this.selectedJobs.push(name);
            }
        },
        focusPatterns() {
            this.$nextTick(() => this.$refs.patternInput && this.$refs.patternInput.focus());
        },
        // grantedJobs is what the key is limited to: the patterns, plus picked
        // jobs no pattern already covers. Empty means every job.
        grantedJobs() {
            if (this.jobAccess === "all") {
                return [];
            }
            const picked = this.selectedJobs.filter((name) => !this.patterns.some((p) => matchJobPattern(p, name)));
            return [...this.patterns, ...picked];
        },
        create() {
            if (this.creating) {
                return;
            }
            if (this.selectedScopes.includes("write")) { this.jobAccess = "all"; }
            this.nameError = this.name.trim() === "";
            this.scopeError = this.selectedScopes.length === 0;
            const jobs = this.grantedJobs();
            this.jobsError = this.jobAccess === "selected" && jobs.length === 0;
            if (this.nameError) {
                this.$refs.nameInput.focus();
            }
            // The patterns only count, and only show, with selected jobs
            const patternError = this.jobAccess === "selected" && this.patternError !== "";
            if (patternError && !this.nameError) {
                this.$refs.patternInput.focus();
            }
            if (this.nameError || this.scopeError || this.jobsError || patternError) {
                return;
            }
            const data = new URLSearchParams();
            data.append("name", this.name.trim());
            this.selectedScopes.forEach((scope) => data.append("scope", scope));
            jobs.forEach((job) => data.append("job", job));
            data.append("expiresInDays", this.expiresInDays);
            this.creating = true;
            axios
                .post("/api/keys/", data)
                .then((response) => {
                    this.copied = false;
                    this.token = response.data.token;
                })
                .catch((error) => {})
                .finally(() => {
                    this.creating = false;
                });
        },
        copy() {
            const copied = () => {
                this.copied = true;
                setTimeout(() => {
                    this.copied = false;
                }, 2000);
            };
            const failed = () => {
                this.$refs.tokenInput.select();
                this.$notify({ text: "Copy failed, the key is selected: copy it manually", type: "error" });
            };
            if (navigator.clipboard && navigator.clipboard.writeText) {
                navigator.clipboard.writeText(this.token).then(copied).catch(failed);
                return;
            }
            // Browsers only offer the Clipboard API over HTTPS and on localhost
            this.$refs.tokenInput.select();
            let ok = false;
            try {
                ok = document.execCommand("copy");
            } catch (error) {}
            if (ok) {
                copied();
            } else {
                failed();
            }
        },
    },
};
</script>

<style scoped>
.muted {
    color: var(--on-surface-variant);
}
.token {
    font-variant-numeric: tabular-nums;
}
.option {
    display: flex;
    flex: 1;
    align-items: center;
    gap: 1rem;
    white-space: normal;
    cursor: pointer;
}
.option-text {
    display: flex;
    flex: 1;
    flex-direction: column;
}
.jobs {
    max-block-size: 30rem;
    overflow-y: auto;
    border: 0.0625rem solid var(--outline-variant);
    border-radius: 0.75rem;
}
</style>
