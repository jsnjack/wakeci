<template>
    <dialog
        :class="{ active: removing !== null }"
        class="padding"
    >
        <div v-if="removing">
            Remove <b>{{ removing.name }}</b
            >? Its members lose these permissions at once and can't sign in unless another group lets them.
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
        v-if="loaded"
        class="border no-elevate padding"
        data-cy="sso-groups"
    >
        <nav class="no-margin">
            <h6 class="small max no-margin">SSO groups</h6>
            <router-link
                to="/settings/groups/new"
                class="button small-round"
                data-cy="add-group"
            >
                <i>group_add</i>
                <span>Add group</span>
            </router-link>
        </nav>
        <p class="muted description">
            People sign in with single sign-on if the identity provider says they are in one of these groups. Names are matched ignoring case, and someone in
            several groups gets the permissions of all of them.
        </p>
        <p
            v-if="!ssoEnabled"
            class="notice small-padding small-round"
            data-cy="sso-not-set-up"
        >
            Single sign-on isn't set up on this server. Groups take effect once <code>sso.idpmetadata</code> is set in Wakefile.yaml.
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
                    <router-link
                        v-if="group.signedIn > 0"
                        to="/keys"
                        class="link"
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
            <div class="actions">
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

        <div
            v-if="groups.length === 0"
            class="center-align empty"
            data-cy="no-groups"
        >
            <i class="extra">groups</i>
            <h6 class="small">No groups yet</h6>
            <p class="muted">Nobody can sign in with single sign-on until you add one.</p>
        </div>
    </article>
</template>

<script>
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
    data: function () {
        return {
            groups: [],
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
.notice {
    border-left: 3px solid var(--primary);
    background-color: var(--surface-container);
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
.empty {
    margin: 1.5rem 0 0.5rem;
}
.muted {
    opacity: 0.7;
}
</style>
