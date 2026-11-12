<template>
    <div id="helpView">
        <div class="tabs min right-align">
            <a
                v-for="tab in tabs"
                :key="tab.id"
                :class="{ active: active === tab.id }"
                :data-cy="'help-tab-' + tab.id"
                @click.prevent="open(tab.id)"
                >{{ tab.title }}</a
            >
        </div>
        <div
            v-if="active === 'job'"
            class="page padding active"
        >
            <SyntaxViewer />
        </div>
        <div
            v-if="active === 'search'"
            class="page padding active"
        >
            <article>
                <p>Search for builds by <b>ID</b>, <b>name</b>, <b>parameters</b>, or <b>status</b>. Keywords are separated by spaces.</p>

                <p>
                    Use <code>+</code> to require a keyword, <code>-</code> to exclude it, or quotes for exact phrases. Keywords use OR logic: if any match, the
                    build is returned.
                </p>

                <p>Specific attributes can be targeted using <code>key:value</code> syntax, e.g., <code>status:failed</code> or <code>name:myjob</code>.</p>

                <p>
                    Words match anywhere in a value: <code>name:test_unit</code> also finds <code>test_unit_js</code>. The API's feed also takes exact
                    <code>job</code> and <code>status</code> filters; see REST API.
                </p>

                <h6>Examples</h6>

                <p>Failed builds</p>
                <code>+status:failed</code>

                <p>Failed or timed out builds</p>
                <code>status:failed "status:timed out"</code>

                <p>Failed builds excluding "test_"</p>
                <code>+status:failed -name:test_</code>

                <p>Builds with parameter "env=prod"</p>
                <code>+env:prod</code>
            </article>
        </div>
        <div
            v-if="active === 'rest'"
            class="page padding active"
        >
            <nav class="no-margin">
                <p class="max">
                    Every route of the API, with its parameters and answers. Authenticate with an API key ("Authorize", then <code>Bearer wk_...</code>) or the
                    admin password.
                </p>
                <a
                    class="button border small-round"
                    href="/docs/api/"
                    target="_blank"
                    rel="noopener noreferrer"
                >
                    <i>open_in_new</i>
                    <span>Open in a new window</span>
                </a>
            </nav>
            <iframe
                src="/docs/api/"
                title="REST API documentation"
                class="api-docs"
                data-cy="api-docs-frame"
            ></iframe>
        </div>
        <div
            v-if="active === 'mcp'"
            class="page padding active"
            data-cy="help-mcp"
        >
            <article>
                <p>
                    Agents such as Claude Code, Codex or Copilot can use wakeci as an MCP server. It works with an API key, within the key's scopes and jobs;
                    create one on the API keys page.
                </p>

                <h6>Connect</h6>
                <pre
                    class="small-padding round"
                    data-cy="mcp-connect"
                    >{{ connectCommand }}</pre>
                <p class="small-text">
                    Other clients: the URL <code>{{ endpoint }}</code> (Streamable HTTP) and the header <code>Authorization: Bearer wk_...</code>.
                </p>

                <h6>Tools</h6>
                <p
                    v-if="toolsError"
                    class="error-text"
                >
                    Couldn't load the tools: {{ toolsError }}
                </p>
                <progress
                    v-else-if="tools === null"
                    class="circle small"
                ></progress>
                <table
                    v-else
                    class="border tools"
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
                                    class="input"
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
                        class="link"
                        href="/llms.txt"
                        target="_blank"
                        rel="noopener noreferrer"
                        >/llms.txt</a
                    >, a one-page guide.
                </p>
            </article>
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
        };
    },
    computed: {
        connectCommand() {
            return `claude mcp add --transport http wakeci ${this.endpoint} \\\n  --header "Authorization: Bearer wk_..."`;
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
.api-docs {
    width: 100%;
    height: 75vh;
    border: 0;
    border-radius: 0.5rem;
    margin-top: 1rem;
}
pre {
    background-color: var(--surface-container);
    white-space: pre-wrap;
    word-break: break-all;
}
.tools td {
    vertical-align: top;
}
.input {
    margin-bottom: 0.25rem;
}
.muted {
    opacity: 0.7;
}
</style>
