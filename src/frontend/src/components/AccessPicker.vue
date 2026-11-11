<template>
    <h6 class="small">Scopes</h6>
    <ul class="list border">
        <li
            v-for="scope in offeredScopes"
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
                        <small class="muted">{{ allJobsHint }}</small>
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
            <i>filter_alt</i>
            <input
                :id="cyPrefix + '-patterns'"
                v-model="patternText"
                type="text"
                placeholder=" "
                :data-cy="cyPrefix + '-patterns'"
                @input="jobsError = false"
                @keydown.enter.prevent
            />
            <label :for="cyPrefix + '-patterns'">Name patterns</label>
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
                    :data-cy="cyPrefix + '-job-search'"
                    @keydown.enter.prevent
                />
            </div>
            <small
                :class="{ 'error-text': jobsError }"
                aria-live="polite"
                :data-cy="cyPrefix + '-jobs-count'"
            >
                <template v-if="jobsError">Select a job or enter a pattern</template>
                <template v-else>{{ selectedCount }} of {{ jobs.length }} selected</template>
            </small>
        </nav>
        <!-- Scrolls on its own, so a long job list doesn't push Create off screen -->
        <ul
            class="list border jobs"
            :data-cy="cyPrefix + '-jobs'"
        >
            <li
                v-for="job in shownJobs"
                :key="job.name"
                :data-cy="cyPrefix + '-job-' + job.name"
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
</template>

<script>
import { matchJobPattern } from "@/store/utils";
import axios from "axios";

// AccessPicker chooses scopes and the jobs they apply to. v-model is
// { scopes, allJobs, jobs, patterns }, as the API takes them.
export default {
    props: {
        modelValue: {
            type: Object,
            required: true,
        },
        // Prefix of the data-cy attributes and ids
        cyPrefix: {
            type: String,
            default: "key",
        },
        // The scopes offered; the others can't be picked
        scopeNames: {
            type: Array,
            default: () => ["read", "run", "write"],
        },
        allJobsHint: {
            type: String,
            default: "Including jobs added later",
        },
    },
    emits: ["update:modelValue"],
    data: function () {
        return {
            selectedScopes: [...this.modelValue.scopes],
            jobAccess: this.modelValue.allJobs ? "all" : "selected",
            patternText: this.modelValue.patterns.join(", "),
            // Only narrows the list shown, never what is selected
            search: "",
            // Jobs picked one by one
            pickedJobs: [...this.modelValue.jobs],
            jobs: [],
            jobsLoaded: false,
            scopeError: false,
            jobsError: false,
            // Where the job choice goes back to when write is unticked
            jobAccessBeforeWrite: this.modelValue.allJobs ? "all" : "selected",
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
        offeredScopes() {
            return this.scopes.filter((scope) => this.scopeNames.includes(scope.name));
        },
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
        // What the API takes: the job fields only count while they are shown.
        // Copies, so that ticking a job, which changes pickedJobs in place,
        // makes a new grant for v-model.
        grant() {
            const selected = this.jobAccess === "selected";
            return {
                scopes: [...this.selectedScopes],
                allJobs: !selected,
                jobs: selected ? [...this.pickedJobs] : [],
                patterns: selected ? [...this.patterns] : [],
            };
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
        grant(grant) {
            this.$emit("update:modelValue", grant);
        },
    },
    mounted() {
        axios
            .get("/api/jobs/")
            .then((response) => {
                this.jobs = response.data || [];
                this.jobsLoaded = true;
            })
            .catch(() => {
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
        // validate shows what is missing and reports whether the choice is
        // complete
        validate() {
            this.scopeError = this.selectedScopes.length === 0;
            const selected = this.jobAccess === "selected";
            this.jobsError = selected && this.pickedJobs.length === 0 && this.patterns.length === 0;
            return !this.scopeError && !this.jobsError && !(selected && this.patternError);
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
</style>
