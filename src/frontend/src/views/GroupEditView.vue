<template>
    <form
        v-if="ready"
        class="ui group-edit stack gap-4"
        novalidate
        @submit.prevent="save"
    >
        <div
            v-if="isNew"
            class="form-field"
        >
            <label
                class="label-text"
                for="group-name"
                >Group name</label
            >
            <input
                id="group-name"
                ref="nameInput"
                v-model="name"
                class="input"
                :class="{ invalid: nameError !== '' }"
                type="text"
                maxlength="200"
                data-cy="group-name"
                @input="nameError = ''"
            />
            <span
                v-if="nameError"
                class="hint error-text"
                >{{ nameError }}</span
            >
            <span
                v-else
                class="hint"
                >As the identity provider names it, for example Service Team. Case doesn't matter.</span
            >
        </div>
        <p v-else-if="noGroup">
            What people in <b data-cy="group-title">none of the groups</b> may do when they sign in with single sign-on. Members of a group get only their
            groups' permissions. A change applies to their next request.
        </p>
        <p v-else>
            What members of <b data-cy="group-title">{{ name }}</b> may do. A change applies to their next request.
        </p>

        <AccessPicker
            ref="access"
            v-model="grant"
            cy-prefix="group"
        />

        <p
            v-if="noGroup && grant.scopes.includes('write')"
            class="warning"
            data-cy="no-group-write-warning"
        >
            With write, anyone the identity provider lets in who is in none of the groups can change jobs, and so run commands on this server.
        </p>

        <div class="cluster end gap-2">
            <router-link
                to="/settings"
                class="btn btn-ghost"
            >
                Cancel
            </router-link>
            <button
                type="submit"
                class="btn btn-primary"
                data-cy="save-group"
                :disabled="saving"
            >
                {{ isNew ? "Add group" : noGroup && !existed ? "Allow" : "Save" }}
            </button>
        </div>
    </form>
</template>

<script>
import AccessPicker from "@/components/AccessPicker.vue";
import { escapeHtml } from "@/store/utils";
import axios from "axios";

// GroupEditView adds a single sign-on group, or changes what one may do
export default {
    components: { AccessPicker },
    props: {
        // Empty for a new group
        groupName: {
            type: String,
            default: "",
        },
        // The rule for people in none of the groups instead of a group
        noGroup: {
            type: Boolean,
            default: false,
        },
    },
    data: function () {
        return {
            name: this.groupName,
            grant: { scopes: ["read"], allJobs: false, jobs: [], patterns: [] },
            // A group being changed is shown once it is loaded, so the
            // picker starts from its permissions
            ready: this.groupName === "" && !this.noGroup,
            nameError: "",
            saving: false,
            // Whether the rule for people in no group was set already
            existed: false,
        };
    },
    computed: {
        isNew() {
            return this.groupName === "" && !this.noGroup;
        },
    },
    mounted() {
        this.$store.commit("SET_CURRENT_PAGE", this.isNew ? "Add single sign-on group" : this.noGroup ? "People in no group" : `Edit ${this.groupName}`);
        if (this.noGroup) {
            axios
                .get("/api/groups/")
                .then((response) => {
                    const rule = response.data.noGroup;
                    this.existed = Boolean(rule);
                    // Read on all jobs is where most start
                    this.grant = rule
                        ? { scopes: rule.scopes, allJobs: rule.allJobs, jobs: rule.jobs, patterns: rule.patterns }
                        : { scopes: ["read"], allJobs: true, jobs: [], patterns: [] };
                    this.ready = true;
                })
                .catch(() => {});
            return;
        }
        if (this.isNew) {
            this.$nextTick(() => this.$refs.nameInput.focus());
            return;
        }
        axios
            .get("/api/groups/")
            .then((response) => {
                const wanted = this.groupName.toLowerCase();
                const group = (response.data.groups || []).find((g) => g.name.toLowerCase() === wanted);
                if (!group) {
                    this.$notify({ text: `There is no group ${escapeHtml(this.groupName)}`, type: "error" });
                    this.$router.replace("/settings");
                    return;
                }
                this.name = group.name;
                this.grant = { scopes: group.scopes, allJobs: group.allJobs, jobs: group.jobs, patterns: group.patterns };
                this.ready = true;
            })
            .catch(() => {});
    },
    methods: {
        save() {
            this.nameError = this.isNew && this.name.trim() === "" ? "Enter the group's name" : "";
            // Both, so every mistake shows at once
            const accessValid = this.$refs.access.validate();
            if (this.saving || this.nameError !== "" || !accessValid) {
                return;
            }
            const data = new URLSearchParams();
            if (this.isNew) {
                data.append("name", this.name.trim());
            }
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
            const url = this.noGroup ? "/api/nogroup" : this.isNew ? "/api/groups/" : `/api/groups/${encodeURIComponent(this.name)}`;
            // Until the response arrives, so a double click can't add it twice
            this.saving = true;
            axios
                .post(url, data)
                .then((response) => {
                    const text = this.noGroup ? "People in no group can sign in" : `${escapeHtml(response.data.name)} saved`;
                    this.$notify({ text, type: "primary" });
                    this.$router.push("/settings");
                })
                .catch(() => {})
                .finally(() => {
                    this.saving = false;
                });
        },
    },
};
</script>

<style scoped>
.warning {
    padding: var(--space-3);
    border-inline-start: 3px solid var(--error);
    border-radius: var(--radius-control);
    background-color: var(--surface-container);
}

.input.invalid {
    border-color: var(--error);
}

.error-text {
    color: var(--error);
}
</style>
