<template>
    <div class="ui help-page stack gap-4">
        <nav
            class="tabbar"
            role="tablist"
            aria-label="Help"
        >
            <a
                v-for="tab in tabs"
                :key="tab.id"
                href="#"
                role="tab"
                :aria-selected="active === tab.id ? 'true' : 'false'"
                :data-cy="'help-tab-' + tab.id"
                @click.prevent="open(tab.id)"
                >{{ tab.title }}</a
            >
        </nav>
        <div v-if="active === 'job'">
            <SyntaxViewer />
        </div>
        <div v-if="active === 'search'">
            <div class="prose stack gap-3">
                <p>Search for builds by <b>ID</b>, <b>name</b>, <b>parameters</b>, or <b>status</b>. Keywords are separated by spaces.</p>

                <p>
                    Use <code>+</code> to require a keyword, <code>-</code> to exclude it, or quotes for exact phrases. Keywords use OR logic: if any match, the
                    build is returned.
                </p>

                <p>Specific attributes can be targeted using <code>key:value</code> syntax, e.g., <code>status:failed</code> or <code>name:myjob</code>.</p>

                <p>
                    Words match anywhere in a value: <code>name:test_unit</code> also finds <code>test_unit_js</code>. <code>NAME=value</code> matches only an
                    equal value: a parameter named NAME, ignoring case, or the build's <code>id</code>, <code>name</code> or <code>status</code>. A label is
                    found by its name without <code>_LABEL</code>: <code>branch=main</code> finds <code>BRANCH_LABEL=main</code>, but not <code>main-fix</code>.
                    Quote a term with spaces: <code>"status=timed out"</code>. The API's feed also takes exact <code>job</code> and <code>status</code> filters;
                    see REST API.
                </p>

                <h2>Examples</h2>

                <p>Failed builds</p>
                <code>+status:failed</code>

                <p>Failed or timed out builds</p>
                <code>status:failed "status:timed out"</code>

                <p>Failed builds excluding "test_"</p>
                <code>+status:failed -name:test_</code>

                <p>Builds whose parameter "ENV" is "prod"</p>
                <code>+env=prod</code>

                <p>Failed builds of the branch "main", not "main-fix"</p>
                <code>+status=failed +branch=main</code>
            </div>
        </div>
        <div
            v-if="active === 'rest'"
            class="stack gap-3"
        >
            <p>
                Every route of the API, with its parameters, answers and examples. "Test Request" runs as you; scripts send an API key as
                <code>Authorization: Bearer wk_...</code>, or the admin password. Agents can read the same as Markdown at
                <a
                    href="/llms-full.txt"
                    target="_blank"
                    rel="noopener noreferrer"
                    data-cy="api-markdown-link"
                    >/llms-full.txt</a
                >.
            </p>
            <iframe
                src="/docs/api/"
                title="REST API documentation"
                class="api-docs"
                data-cy="api-docs-frame"
            ></iframe>
        </div>
        <div
            v-if="active === 'mcp'"
            class="prose stack gap-3"
            data-cy="help-mcp"
        >
            <p>
                Agents such as Claude Code, Codex or Copilot can use wakeci as an MCP server. It works with an API key, within the key's scopes and jobs; create
                one on the API keys page.
            </p>

            <h2>Connect</h2>
            <div class="form-field mcp-key">
                <label
                    class="label-text"
                    for="mcp-key"
                    >API key (optional)</label
                >
                <input
                    id="mcp-key"
                    v-model="apiKey"
                    class="input"
                    type="password"
                    autocomplete="off"
                    data-cy="mcp-key"
                />
                <span class="hint">Fills in the commands below. It stays in this page and isn't saved.</span>
            </div>
            <div
                class="segmented"
                role="tablist"
                aria-label="MCP client"
            >
                <button
                    v-for="option in clients"
                    :key="option.id"
                    type="button"
                    role="tab"
                    :aria-selected="client === option.id ? 'true' : 'false'"
                    :aria-pressed="client === option.id ? 'true' : 'false'"
                    :data-cy="'mcp-client-' + option.id"
                    @click="client = option.id"
                >
                    {{ option.title }}
                </button>
            </div>
            <p
                class="text-sm muted"
                data-cy="mcp-client-note"
            >
                {{ clientNote }}
            </p>
            <div class="cluster nowrap gap-2 mcp-command">
                <pre
                    class="grow"
                    data-cy="mcp-connect"
                    >{{ connectCommand }}</pre>
                <button
                    type="button"
                    class="btn btn-ghost btn-icon"
                    :aria-label="'Copy the command for ' + clientTitle"
                    :data-tip="copied ? 'Copied' : 'Copy'"
                    data-cy="mcp-copy"
                    @click="copy"
                >
                    <i class="icon">{{ copied ? "check" : "content_copy" }}</i>
                </button>
            </div>
            <div v-if="client === 'vscode'">
                <a
                    class="btn"
                    :href="vscodeInstallURL"
                    data-cy="mcp-vscode-install"
                >
                    <i class="icon small">download</i>
                    <span>Install in VS Code</span>
                </a>
            </div>
            <p
                v-if="copyFailed"
                class="error-text"
                role="alert"
            >
                Copying isn't available here; select the command and copy it.
            </p>
            <p class="text-sm">
                Any other client: the URL <code>{{ endpoint }}</code> (Streamable HTTP) with the header <code>Authorization: Bearer wk_...</code>.
            </p>

            <h2>Tools</h2>
            <p
                v-if="toolsError"
                class="error-text"
            >
                Couldn't load the tools: {{ toolsError }}
            </p>
            <span
                v-else-if="tools === null"
                class="status-ring tools-spinner"
                aria-label="Loading"
            ></span>
            <table
                v-else
                class="tools"
                data-cy="mcp-tools"
            >
                <thead>
                    <tr>
                        <th>Tool</th>
                        <th>What it does</th>
                        <th>Inputs</th>
                        <th>Scope</th>
                    </tr>
                </thead>
                <tbody>
                    <tr
                        v-for="tool in tools"
                        :key="tool.name"
                        :data-cy="'mcp-tool-' + tool.name"
                    >
                        <td>
                            <code>{{ tool.name }}</code>
                        </td>
                        <td>{{ tool.description }}</td>
                        <td>
                            <div
                                v-for="input in tool.inputs"
                                :key="input.name"
                                class="tool-input"
                            >
                                <code>{{ input.name }}</code>
                                <small class="muted">
                                    {{ input.type }}{{ input.required ? ", required" : ""
                                    }}<template v-if="input.description">: {{ input.description }}</template>
                                </small>
                            </div>
                            <small
                                v-if="tool.inputs.length === 0"
                                class="muted"
                                >none</small
                            >
                        </td>
                        <td data-cy="mcp-tool-scope">{{ tool.scope === "any" ? "any key" : tool.scope }}</td>
                    </tr>
                </tbody>
            </table>

            <p>
                Scripts that call the REST API can start with
                <a
                    href="/llms.txt"
                    target="_blank"
                    rel="noopener noreferrer"
                    >/llms.txt</a
                >, a one-page guide.
            </p>
        </div>
    </div>
</template>

<script>
import SyntaxViewer from "@/components/SyntaxViewer.vue";
import axios from "axios";

export default {
    components: {
        SyntaxViewer,
    },
    data: function () {
        return {
            tabs: [
                { id: "job", title: "Job syntax" },
                { id: "search", title: "Search syntax" },
                { id: "rest", title: "REST API" },
                { id: "mcp", title: "MCP" },
            ],
            active: "job",
            // From /docs/mcp.json, which wakeci builds from the tools themselves
            tools: null,
            toolsError: "",
            endpoint: window.location.origin + "/mcp",
            clients: [
                { id: "claude", title: "Claude Code" },
                { id: "codex", title: "Codex" },
                { id: "copilot", title: "Copilot CLI" },
                { id: "vscode", title: "VS Code" },
            ],
            client: "claude",
            // Only fills in the commands; never sent or saved
            apiKey: "",
            copied: false,
            copyFailed: false,
        };
    },
    computed: {
        key() {
            return this.apiKey.trim() || "wk_...";
        },
        clientTitle() {
            return this.clients.find((c) => c.id === this.client).title;
        },
        // The server as VS Code and its "code --add-mcp" take it
        vscodeServer() {
            return { name: "wakeci", type: "http", url: this.endpoint, headers: { Authorization: `Bearer ${this.key}` } };
        },
        vscodeInstallURL() {
            return "vscode:mcp/install?" + encodeURIComponent(JSON.stringify(this.vscodeServer));
        },
        connectCommand() {
            switch (this.client) {
                case "codex":
                    return `export WAKECI_API_KEY=${this.key}\ncodex mcp add wakeci --url ${this.endpoint} --bearer-token-env-var WAKECI_API_KEY`;
                case "copilot":
                    return `copilot mcp add --transport http wakeci ${this.endpoint} \\\n  --header "Authorization: Bearer ${this.key}"`;
                case "vscode":
                    // Over several lines, which the JSON and the shell's quotes allow
                    return `code --add-mcp '${JSON.stringify(this.vscodeServer, null, 2)}'`;
                default:
                    return `claude mcp add --transport http wakeci ${this.endpoint} \\\n  --header "Authorization: Bearer ${this.key}"`;
            }
        },
        clientNote() {
            switch (this.client) {
                case "codex":
                    return "Codex reads the key from WAKECI_API_KEY whenever it starts, so set it in your shell's profile too.";
                case "copilot":
                    return "Adds the server to ~/.copilot/mcp-config.json.";
                case "vscode":
                    return "Adds the server to your VS Code profile; Copilot Chat in agent mode can then use it.";
                default:
                    return "Adds the server to Claude Code for this project; add --scope user for all projects.";
            }
        },
    },
    watch: {
        connectCommand() {
            this.copied = false;
        },
    },
    mounted() {
        this.$store.commit("SET_CURRENT_PAGE", "Help");
        // A link such as /help#mcp opens that tab
        const tab = this.$route.hash.slice(1);
        this.open(this.tabs.some((t) => t.id === tab) ? tab : "job");
    },
    methods: {
        open(tab) {
            this.active = tab;
            if (this.$route.hash !== "#" + tab) {
                this.$router.replace({ hash: "#" + tab });
            }
            if (tab === "mcp" && this.tools === null && this.toolsError === "") {
                this.fetchTools();
            }
        },
        copy() {
            // The Clipboard API only exists on https and localhost
            if (!navigator.clipboard || !window.isSecureContext) {
                this.copyFailed = true;
                return;
            }
            navigator.clipboard.writeText(this.connectCommand).then(
                () => {
                    this.copied = true;
                    this.copyFailed = false;
                },
                () => {
                    this.copyFailed = true;
                },
            );
        },
        fetchTools() {
            axios
                .get("/docs/mcp.json")
                .then((response) => {
                    this.tools = response.data.tools || [];
                })
                .catch((error) => {
                    this.toolsError = error.message;
                });
        },
    },
};
</script>

<style scoped>
.prose {
    max-inline-size: 60rem;
}

.prose code {
    padding: 1px 5px;
    border-radius: 4px;
    background-color: var(--surface-container-high);
}

.api-docs {
    inline-size: 100%;
    block-size: 75vh;
    border: 0;
    border-radius: var(--radius-container);
}

.mcp-key {
    max-inline-size: 28rem;
}

.mcp-command pre {
    padding: var(--space-3);
    border-radius: var(--radius-control);
    background-color: var(--surface-container);
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    font-size: var(--text-sm);
}

.error-text {
    color: var(--error);
}

.tools-spinner {
    inline-size: 20px;
    block-size: 20px;
    --_c: var(--primary);
}

.tools {
    inline-size: 100%;
    border-collapse: collapse;
    font-size: var(--text-sm);
}

.tools th,
.tools td {
    padding: var(--space-2);
    border-block-end: 1px solid var(--outline-variant);
    text-align: start;
    vertical-align: top;
}

.tools th {
    color: var(--on-surface-variant);
}

.tool-input {
    margin-block-end: var(--space-1);
}
</style>
