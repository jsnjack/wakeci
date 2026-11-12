<template>
    <article
        class="border no-elevate padding"
        data-cy="server-card"
    >
        <h6 class="small no-margin">Server</h6>
        <p
            v-if="server.configFound"
            class="muted description"
        >
            Set in <code data-cy="server-config-file">{{ server.configFile }}</code
            >; change them there and restart wakeci.
        </p>
        <p
            v-else
            class="muted description"
        >
            <code data-cy="server-config-file">{{ server.configFile }}</code> wasn't found, so wakeci uses its defaults.
        </p>

        <dl class="config">
            <template
                v-for="row in rows"
                :key="row.label"
            >
                <dt>{{ row.label }}</dt>
                <dd :data-cy="'server-' + row.id">
                    <code v-if="row.code && row.value">{{ row.value }}</code>
                    <span v-else-if="row.value">{{ row.value }}</span>
                    <span
                        v-else
                        class="muted"
                        >{{ row.empty }}</span
                    >
                    <span
                        v-if="row.note"
                        class="muted"
                    >
                        · {{ row.note }}</span
                    >
                </dd>
            </template>
        </dl>

        <h6 class="small">Single sign-on</h6>
        <p
            v-if="!server.sso"
            class="muted no-margin"
            data-cy="server-sso"
        >
            Not set up: <code>sso.idpmetadata</code> isn't set.
        </p>
        <dl
            v-else
            class="config"
            data-cy="server-sso"
        >
            <template
                v-for="row in ssoRows"
                :key="row.label"
            >
                <dt>{{ row.label }}</dt>
                <dd :data-cy="'server-' + row.id">
                    <code v-if="row.value">{{ row.value }}</code>
                    <span
                        v-else
                        class="muted"
                        >{{ row.empty }}</span
                    >
                </dd>
            </template>
            <template
                v-for="(cert, i) in server.sso.certificates"
                :key="i"
            >
                <dt>Signing certificate</dt>
                <dd data-cy="server-cert">
                    <span :title="cert.subject">Valid until {{ new Date(cert.notAfter).toLocaleDateString() }}</span>
                    <span
                        v-if="expiryWarning(cert)"
                        class="error-text"
                        data-cy="server-cert-warning"
                    >
                        · {{ expiryWarning(cert) }}</span
                    >
                </dd>
            </template>
        </dl>
    </article>
</template>

<script>
// ServerConfigCard shows what Wakefile.yaml set, read-only: the settings
// page changes the rest. It names the secrets file and counts its secrets,
// never shows them.
export default {
    props: {
        // ServerConfigData of /api/settings
        server: {
            type: Object,
            required: true,
        },
    },
    computed: {
        rows() {
            const s = this.server;
            return [
                { id: "version", label: "Version", value: s.version },
                { id: "port", label: "Port", value: s.port },
                { id: "hostname", label: "Hostname", value: s.hostname, empty: "Not set" },
                { id: "workdir", label: "Work directory", value: s.workDir, code: true },
                { id: "jobdir", label: "Job directory", value: s.jobDir, code: true },
                {
                    id: "secrets",
                    label: "Secrets file",
                    value: s.secretsFile,
                    code: true,
                    empty: "Not set",
                    note: s.secretsFile ? `${s.secrets} ${s.secrets === 1 ? "secret" : "secrets"}` : "",
                },
                { id: "timezone", label: "Time zone", value: s.timezone, empty: "The system's" },
                { id: "drain", label: "Drain timeout", value: s.drainTimeout },
            ];
        },
        ssoRows() {
            const sso = this.server.sso;
            return [
                { id: "idp", label: "Identity provider metadata", value: sso.idpMetadata },
                { id: "acs", label: "ACS URL", value: sso.acsUrl },
                { id: "entity", label: "Entity ID", value: sso.entityId },
            ];
        },
    },
    methods: {
        expiryWarning(cert) {
            const days = Math.floor((new Date(cert.notAfter) - Date.now()) / 86400000);
            if (days < 0) {
                return "Expired: replace the identity provider's metadata";
            }
            if (days <= 30) {
                return `Expires in ${days} ${days === 1 ? "day" : "days"}: replace the identity provider's metadata`;
            }
            return "";
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
.config {
    display: grid;
    grid-template-columns: 12rem minmax(0, 1fr);
    gap: 0.25rem 1rem;
    margin: 0.5rem 0;
}
dt {
    opacity: 0.7;
}
dd {
    margin: 0;
    overflow-wrap: anywhere;
}
/* One column on a phone */
@media (max-width: 600px) {
    .config {
        grid-template-columns: minmax(0, 1fr);
    }
    dd {
        margin-bottom: 0.5rem;
    }
}
.muted {
    opacity: 0.7;
}
</style>
