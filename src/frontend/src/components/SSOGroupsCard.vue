<template>
    <UiDialog
        :open="removing !== null"
        title="Remove the group?"
        @update:open="removing = null"
    >
        <p v-if="removing">
            Remove <b>{{ removing.name }}</b
            >? Its members lose these permissions at once and can't sign in unless another group lets them.
        </p>
        <template #actions>
            <button
                type="button"
                class="btn btn-ghost"
                @click="removing = null"
            >
                Cancel
            </button>
            <button
                type="button"
                class="btn btn-danger"
                data-cy="remove-group-confirm"
                @click="remove"
            >
                Remove
            </button>
        </template>
    </UiDialog>

    <UiDialog
        v-model:open="turningOff"
        title="Turn off people in no group?"
    >
        <p>Stop people in no group from signing in? Those signed in lose these permissions at once.</p>
        <template #actions>
            <button
                type="button"
                class="btn btn-ghost"
                @click="turningOff = false"
            >
                Cancel
            </button>
            <button
                type="button"
                class="btn btn-danger"
                data-cy="turn-off-no-group-confirm"
                @click="turnOff"
            >
                Turn off
            </button>
        </template>
    </UiDialog>

    <section
        v-if="loaded"
        class="stack gap-3"
        data-cy="sso-groups"
    >
        <div class="cluster between gap-2">
            <h2>SSO groups</h2>
            <router-link
                to="/settings/groups/new"
                class="btn btn-primary"
                data-cy="add-group"
            >
                <i class="icon small">group_add</i>
                <span>Add group</span>
            </router-link>
        </div>
        <p class="muted">
            People sign in with single sign-on if the identity provider says they are in one of these groups. Names are matched ignoring case, and someone in
            several groups gets the permissions of all of them. People in none of them can sign in only if you allow it below.
        </p>
        <p
            v-if="!ssoEnabled"
            class="notice"
            data-cy="sso-not-set-up"
        >
            Single sign-on isn't set up on this server. Groups take effect once <code>sso.idpmetadata</code> is set in Wakefile.yaml.
        </p>

        <div class="group-list">
            <div
                v-for="group in groups"
                :key="group.name"
                class="group"
                :data-cy="'group-' + group.name"
            >
                <i class="icon muted wide-only">group</i>
                <div class="details stack gap-1">
                    <div class="cluster gap-2">
                        <b>{{ group.name }}</b>
                        <span
                            v-for="scope in group.scopes"
                            :key="scope"
                            class="scope text-sm"
                            :data-cy="'group-scope-' + scope"
                        >
                            {{ scope }}
                        </span>
                    </div>
                    <small class="muted">
                        <span
                            :title="jobsSummary(group)"
                            data-cy="group-jobs"
                        >
                            {{ jobsSummary(group) }}
                        </span>
                        ·
                        <router-link
                            v-if="group.signedIn > 0"
                            to="/keys"
                            title="People with a current session in this group; the API keys page signs them out"
                            data-cy="group-signed-in"
                        >
                            {{ group.signedIn }} signed in
                        </router-link>
                        <span
                            v-else
                            data-cy="group-signed-in"
                            >nobody signed in</span
                        >
                        ·
                        <span :title="new Date(group.updatedAt).toLocaleString()">Changed {{ ago(group.updatedAt) }}</span>
                    </small>
                </div>
                <div class="cluster nowrap gap-1">
                    <router-link
                        :to="{ name: 'groupEdit', params: { name: group.name } }"
                        class="btn btn-ghost btn-icon"
                        :aria-label="'Edit ' + group.name"
                        data-tip="Edit"
                        data-cy="edit-group"
                    >
                        <i class="icon">edit</i>
                    </router-link>
                    <button
                        type="button"
                        class="btn btn-ghost btn-icon"
                        :aria-label="'Remove ' + group.name"
                        data-tip="Remove"
                        data-cy="remove-group"
                        @click="removing = group"
                    >
                        <i class="icon">delete</i>
                    </button>
                </div>
            </div>

            <div
                v-if="groups.length === 0"
                class="empty stack gap-1"
                data-cy="no-groups"
            >
                <i class="icon large muted">groups</i>
                <b>No groups yet</b>
                <p class="muted">
                    {{
                        noGroup
                            ? "Everyone who signs in gets the permissions of people in no group."
                            : "Nobody can sign in with single sign-on until you add one or allow people in no group."
                    }}
                </p>
            </div>

            <div
                class="group"
                data-cy="no-group"
            >
                <i class="icon muted wide-only">person</i>
                <div class="details stack gap-1">
                    <div class="cluster gap-2">
                        <b>People in no group</b>
                        <template v-if="noGroup">
                            <span
                                v-for="scope in noGroup.scopes"
                                :key="scope"
                                class="scope text-sm"
                                :data-cy="'no-group-scope-' + scope"
                            >
                                {{ scope }}
                            </span>
                        </template>
                    </div>
                    <small
                        v-if="noGroup"
                        class="muted"
                    >
                        <span data-cy="no-group-jobs">{{ jobsSummary(noGroup) }}</span>
                        ·
                        <router-link
                            v-if="noGroup.signedIn > 0"
                            to="/keys"
                            title="People with a current session in none of the groups; the API keys page signs them out"
                            data-cy="no-group-signed-in"
                        >
                            {{ noGroup.signedIn }} signed in
                        </router-link>
                        <span
                            v-else
                            data-cy="no-group-signed-in"
                            >nobody signed in</span
                        >
                        ·
                        <span :title="new Date(noGroup.updatedAt).toLocaleString()">Changed {{ ago(noGroup.updatedAt) }}</span>
                    </small>
                    <small
                        v-else
                        class="muted"
                        data-cy="no-group-off"
                        >Can't sign in. Allow it to give people whose groups match none above permissions of their own.</small
                    >
                </div>
                <div
                    v-if="noGroup"
                    class="cluster nowrap gap-1"
                >
                    <router-link
                        to="/settings/no-group"
                        class="btn btn-ghost btn-icon"
                        aria-label="Edit people in no group"
                        data-tip="Edit"
                        data-cy="edit-no-group"
                    >
                        <i class="icon">edit</i>
                    </router-link>
                    <button
                        type="button"
                        class="btn btn-ghost btn-icon"
                        aria-label="Turn off people in no group"
                        data-tip="Turn off"
                        data-cy="turn-off-no-group"
                        @click="turningOff = true"
                    >
                        <i class="icon">block</i>
                    </button>
                </div>
                <router-link
                    v-else
                    to="/settings/no-group"
                    class="btn btn-outline btn-ghost"
                    data-cy="allow-no-group"
                >
                    <i class="icon small">login</i>
                    <span>Allow</span>
                </router-link>
            </div>
        </div>
    </section>
</template>

<script>
import UiDialog from "@/components/ui/UiDialog.vue";
import { escapeHtml } from "@/store/utils";
import axios from "axios";
import TimeAgo from "javascript-time-ago";
import en from "javascript-time-ago/locale/en.json";

TimeAgo.addLocale(en);
const timeAgo = new TimeAgo("en-US");

// SSOGroupsCard lets the admin password add, change and remove the single
// sign-on groups. SettingsView shows it to the admin password only, and the
// API refuses everyone else.
export default {
    components: { UiDialog },
    data: function () {
        return {
            groups: [],
            // The rule for people in none of the groups, null while they
            // can't sign in
            noGroup: null,
            turningOff: false,
            ssoEnabled: true,
            loaded: false,
            removing: null,
        };
    },
    mounted() {
        this.fetch();
    },
    methods: {
        fetch() {
            return axios
                .get("/api/groups/")
                .then((response) => {
                    this.groups = response.data.groups || [];
                    this.noGroup = response.data.noGroup || null;
                    this.ssoEnabled = response.data.ssoEnabled === true;
                    this.loaded = true;
                })
                .catch(() => {});
        },
        remove() {
            const group = this.removing;
            this.removing = null;
            axios
                .delete(`/api/groups/${encodeURIComponent(group.name)}`)
                .then(() => {
                    this.$notify({ text: `${escapeHtml(group.name)} can no longer sign in`, type: "primary" });
                    this.fetch();
                })
                .catch(() => {});
        },
        turnOff() {
            this.turningOff = false;
            axios
                .delete("/api/nogroup")
                .then(() => {
                    this.$notify({ text: "People in no group can no longer sign in", type: "primary" });
                    this.fetch();
                })
                .catch(() => {});
        },
        jobsSummary(group) {
            if (group.allJobs) {
                return "All jobs";
            }
            return [...group.jobs, ...group.patterns].join(", ");
        },
        ago(date) {
            return timeAgo.format(new Date(date));
        },
    },
};
</script>

<style scoped>
.notice {
    padding: var(--space-3);
    border-inline-start: 3px solid var(--primary);
    border-radius: var(--radius-control);
    background-color: var(--surface-container);
}

.notice code {
    padding: 1px 5px;
    border-radius: 4px;
    background-color: var(--surface-container-high);
}

.group-list {
    border-block-start: 1px solid var(--outline-variant);
}

.group {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-3) var(--space-2);
    border-block-end: 1px solid var(--outline-variant);
}

/* So long names and several scopes wrap instead of pushing the buttons off a
   narrow screen */
.details {
    flex: 1 1 auto;
    min-inline-size: 0;
    overflow-wrap: anywhere;
}

.scope {
    padding: 0 6px;
    border: 1px solid var(--outline-variant);
    border-radius: 4px;
}

.empty {
    align-items: center;
    padding: var(--space-6) 0;
    text-align: center;
}

@media (max-width: 600px) {
    .wide-only {
        display: none;
    }
}
</style>
