<template>
    <dialog
        :class="{ active: removing !== null }"
        class="padding"
    >
        <div v-if="removing">
            Remove <b>{{ removing.name }}</b
            >? Its members lose these permissions at once and can't log in unless another group lets them.
        </div>
        <nav class="right-align">
            <button
                class="border"
                @click.prevent="removing = null"
            >
                Cancel
            </button>
            <button
                data-cy="remove-group-confirm"
                @click.prevent="remove"
            >
                Remove
            </button>
        </nav>
    </dialog>

    <article
        v-if="shown"
        class="border no-elevate padding"
        data-cy="sso-groups"
    >
        <nav class="no-margin">
            <h6 class="small max no-margin">Single sign-on groups</h6>
            <router-link
                v-if="canEdit"
                to="/settings/groups/new"
                class="button small-round"
                data-cy="add-group"
            >
                <i>group_add</i>
                <span>Add group</span>
            </router-link>
        </nav>
        <p class="muted description">
            Members of these groups can log in with single sign-on. Names are matched to the groups the identity provider sends, ignoring case, and someone in
            several groups gets the permissions of all of them.
        </p>

        <div
            v-for="group in groups"
            :key="group.name"
            class="row medium-padding round small-margin group"
            :data-cy="'group-' + group.name"
        >
            <i class="m l">group</i>
            <div class="max details">
                <div class="row small-space chips">
                    <b>{{ group.name }}</b>
                    <span
                        v-for="scope in group.scopes"
                        :key="scope"
                        class="chip small no-margin"
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
                    <span :title="new Date(group.updatedAt).toLocaleString()">Changed {{ ago(group.updatedAt) }}</span>
                </small>
            </div>
            <div
                v-if="canEdit"
                class="actions"
            >
                <router-link
                    :to="{ name: 'groupEdit', params: { name: group.name } }"
                    class="button circle transparent"
                    :aria-label="'Edit ' + group.name"
                    data-cy="edit-group"
                >
                    <i>edit</i>
                    <div class="tooltip bottom">Edit</div>
                </router-link>
                <button
                    class="circle transparent"
                    :aria-label="'Remove ' + group.name"
                    data-cy="remove-group"
                    @click.prevent="removing = group"
                >
                    <i>delete</i>
                    <div class="tooltip bottom">Remove</div>
                </button>
            </div>
        </div>

        <p
            v-if="groups.length === 0"
            data-cy="no-groups"
        >
            No groups yet, so nobody can log in with single sign-on.
        </p>
        <p
            v-if="!canEdit"
            class="muted"
        >
            <small>Only the admin password can change the groups.</small>
        </p>
    </article>
</template>

<script>
import { escapeHtml } from "@/store/utils";
import axios from "axios";
import TimeAgo from "javascript-time-ago";
import en from "javascript-time-ago/locale/en.json";

TimeAgo.addLocale(en);
const timeAgo = new TimeAgo("en-US");

// SSOGroupsCard lists the single sign-on groups and, for the admin password,
// lets them be added, changed and removed
export default {
    data: function () {
        return {
            groups: [],
            canEdit: false,
            ssoEnabled: false,
            loaded: false,
            removing: null,
        };
    },
    computed: {
        // Groups only matter with single sign-on, but are shown while any
        // are left, so they can be removed
        shown() {
            return this.loaded && (this.ssoEnabled || this.groups.length > 0);
        },
    },
    mounted() {
        Promise.allSettled([this.fetch(), this.fetchAuthMethods()]).then(() => {
            this.loaded = true;
        });
    },
    methods: {
        fetch() {
            return axios
                .get("/api/groups/")
                .then((response) => {
                    this.groups = response.data.groups || [];
                    this.canEdit = response.data.canEdit === true;
                })
                .catch(() => {});
        },
        fetchAuthMethods() {
            return axios
                .get("/auth/methods")
                .then((response) => {
                    this.ssoEnabled = response.data.sso === true;
                })
                .catch(() => {});
        },
        remove() {
            const group = this.removing;
            this.removing = null;
            axios
                .delete(`/api/groups/${encodeURIComponent(group.name)}`)
                .then(() => {
                    this.$notify({ text: `${escapeHtml(group.name)} can no longer log in`, type: "primary" });
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
article {
    margin: 1.5rem 0 0;
}
.description {
    margin: 0.5rem 0;
}
.group {
    background-color: var(--surface-container);
}
/* So long names and several scopes wrap instead of pushing the buttons off a
   narrow screen */
.details {
    min-width: 0;
    overflow-wrap: anywhere;
}
.chips {
    flex-wrap: wrap;
}
.actions {
    display: flex;
    flex: none;
}
.muted {
    opacity: 0.7;
}
</style>
