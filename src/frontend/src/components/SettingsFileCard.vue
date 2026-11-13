<template>
    <dialog
        :class="{ active: chosen !== null }"
        class="padding"
    >
        <div v-if="chosen">
            Import <b>{{ chosen.name }}</b
            >? It sets every setting the file has, and its SSO groups replace all groups. A file with a mistake changes nothing.
        </div>
        <nav class="right-align">
            <button
                class="border"
                @click.prevent="chosen = null"
            >
                Cancel
            </button>
            <button
                data-cy="settings-import-confirm"
                :disabled="importing"
                @click.prevent="importFile"
            >
                Import
            </button>
        </nav>
    </dialog>

    <section
        class="no-margin"
        data-cy="settings-file"
    >
        <h6 class="small no-margin">Settings file</h6>
        <p class="muted description">
            The settings on this page, the SSO groups and people in no group as YAML, for a backup, review in git or another server. Importing a file sets what
            it has; the password and API keys are never in it. On a stopped server: <code>wakeci --import-settings file.yaml</code>.
        </p>
        <nav class="no-margin">
            <a
                class="button border small-round"
                href="/api/settings/export"
                download
                data-cy="settings-download"
            >
                <i>download</i>
                <span>Download</span>
            </a>
            <button
                class="border small-round"
                data-cy="settings-import"
                @click.prevent="$refs.file.click()"
            >
                <i>upload</i>
                <span>Import</span>
            </button>
            <input
                ref="file"
                type="file"
                accept=".yaml,.yml,application/yaml,text/yaml"
                hidden
                data-cy="settings-import-file"
                @change="choose"
            />
        </nav>
    </section>
</template>

<script>
import axios from "axios";

// How the notification names what an import set
const names = {
    concurrentBuilds: "concurrent builds",
    buildHistorySize: "builds to preserve",
    apiKeyRate: "API key steady rate",
    apiKeyBurst: "API key burst size",
    ssoGroups: "SSO groups",
    ssoNoGroup: "people in no group",
};

// SettingsFileCard downloads the settings as YAML and imports such a file.
// SettingsView shows it to the admin password only, and the API refuses
// everyone else.
export default {
    emits: ["imported"],
    data: function () {
        return { chosen: null, importing: false };
    },
    methods: {
        choose(event) {
            this.chosen = event.target.files[0] || null;
            // So choosing the same file again opens the dialog again
            event.target.value = "";
        },
        importFile() {
            const file = this.chosen;
            this.importing = true;
            file.text()
                .then((text) => axios.post("/api/settings/import", text, { headers: { "Content-Type": "application/yaml" } }))
                .then((response) => {
                    const applied = (response.data.applied || []).map((name) => names[name] || name);
                    this.$notify({ text: `Imported ${applied.join(", ")}`, type: "primary" });
                    this.$emit("imported");
                })
                .catch(() => {})
                .finally(() => {
                    this.chosen = null;
                    this.importing = false;
                });
        },
    },
};
</script>

<style scoped>
.description {
    margin: 0.5rem 0;
}
.muted {
    opacity: 0.7;
}
</style>
