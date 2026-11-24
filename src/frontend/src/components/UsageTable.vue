<template>
    <div>
        <div class="card-head">
            <h6 class="small no-margin">{{ title }}</h6>
            <div
                v-if="rows.length > 1"
                class="field small prefix border round no-margin"
            >
                <i>search</i>
                <input
                    v-model="search"
                    type="search"
                    :aria-label="searchLabel"
                    :placeholder="searchLabel"
                    :data-cy="`${cy}-search`"
                />
            </div>
        </div>
        <div class="table-scroll">
            <table class="small-text">
                <thead>
                    <tr>
                        <th
                            v-for="column in columns"
                            :key="column.key"
                            :class="{ 'right-align': column.numeric }"
                            class="sortable"
                            :title="column.title"
                            :aria-sort="sortKey === column.key ? (sortAsc ? 'ascending' : 'descending') : null"
                            @click="sortBy(column.key)"
                        >
                            {{ column.label }}
                            <i
                                v-if="sortKey === column.key"
                                class="tiny"
                                >{{ sortAsc ? "arrow_upward" : "arrow_downward" }}</i
                            >
                        </th>
                    </tr>
                </thead>
                <tbody>
                    <tr
                        v-for="row in shownRows"
                        :key="rowKey(row)"
                        :data-cy="`${cy}-row`"
                    >
                        <td
                            v-for="column in columns"
                            :key="column.key"
                            :class="{ 'right-align': column.numeric, nowrap: column.numeric }"
                        >
                            <code v-if="column.code && row[column.key]">{{ row[column.key] }}</code>
                            <template v-else>{{ column.format(row) }}</template>
                        </td>
                    </tr>
                    <tr v-if="shownRows.length === 0">
                        <td
                            :colspan="columns.length"
                            class="muted"
                        >
                            No row matches "{{ search.trim() }}"
                        </td>
                    </tr>
                </tbody>
            </table>
        </div>
    </div>
</template>

<script>
import { formatLatency, formatRate } from "@/analytics";

// UsageTable lists requests or MCP tool calls by what they went to, with
// their outcomes and times, sortable by every column and filtered by the
// words typed
export default {
    props: {
        title: {
            type: String,
            required: true,
        },
        // The search box's placeholder, such as "Filter routes"
        searchLabel: {
            type: String,
            required: true,
        },
        // RouteUse or ToolUse of the API
        rows: {
            type: Array,
            required: true,
        },
        // The columns that name a row: { key, label, format, code }
        nameColumns: {
            type: Array,
            required: true,
        },
        // The field that counts a row's requests or calls, and its label
        countKey: {
            type: String,
            required: true,
        },
        countLabel: {
            type: String,
            required: true,
        },
        cy: {
            type: String,
            required: true,
        },
    },
    data() {
        return {
            search: "",
            // null keeps the API's order, the most used first
            sortKey: null,
            sortAsc: false,
        };
    },
    computed: {
        columns() {
            const count = (row) => row[this.countKey];
            const timed = (format) => (row) => (row.time.count ? format(row) : "—");
            return [
                ...this.nameColumns.map((column) => ({ ...column, value: (row) => column.format(row) })),
                { key: "count", label: this.countLabel, numeric: true, value: count, format: (row) => count(row).toLocaleString() },
                {
                    key: "success",
                    label: "Success",
                    numeric: true,
                    title: "Share that succeeded: a status below 400",
                    value: (row) => this.rate(row),
                    format: (row) => formatRate(this.rate(row), this.rate(row) < 0.995 && this.rate(row) > 0 ? 1 : 0),
                },
                {
                    key: "refused",
                    label: "Refused",
                    numeric: true,
                    title: "Wrong or missing credentials, no permission, or too many requests: 401, 403, 429",
                    value: (row) => row.refused,
                    format: (row) => row.refused.toLocaleString(),
                },
                {
                    key: "clientError",
                    label: "Client errors",
                    numeric: true,
                    title: "Any other 4xx, such as 400 or 404",
                    value: (row) => row.clientError,
                    format: (row) => row.clientError.toLocaleString(),
                },
                {
                    key: "serverError",
                    label: "Server errors",
                    numeric: true,
                    title: "5xx",
                    value: (row) => row.serverError,
                    format: (row) => row.serverError.toLocaleString(),
                },
                {
                    key: "p50",
                    label: "Median",
                    numeric: true,
                    title: "Estimated from time buckets",
                    value: (row) => (row.time.count ? row.time.p50 : -1),
                    format: timed((row) => formatLatency(row.time.p50)),
                },
                {
                    key: "p95",
                    label: "p95",
                    numeric: true,
                    title: "95% of requests took less; estimated from time buckets",
                    value: (row) => (row.time.count ? row.time.p95 : -1),
                    format: timed((row) => formatLatency(row.time.p95)),
                },
                {
                    key: "max",
                    label: "Slowest",
                    numeric: true,
                    value: (row) => (row.time.count ? row.time.max : -1),
                    format: timed((row) => formatLatency(row.time.max)),
                },
            ];
        },
        sortedRows() {
            if (!this.sortKey) {
                return this.rows;
            }
            const { value } = this.columns.find((column) => column.key === this.sortKey);
            const sorted = [...this.rows].sort((a, b) => {
                const x = value(a);
                const y = value(b);
                return typeof x === "string" ? x.localeCompare(y) : x - y;
            });
            return this.sortAsc ? sorted : sorted.reverse();
        },
        shownRows() {
            const words = this.search.toLowerCase().split(/\s+/).filter(Boolean);
            return this.sortedRows.filter((row) => {
                const text = this.nameColumns
                    .map((column) => column.format(row))
                    .join(" ")
                    .toLowerCase();
                return words.every((word) => text.includes(word));
            });
        },
    },
    methods: {
        rate(row) {
            const count = row[this.countKey];
            return count ? row.success / count : null;
        },
        rowKey(row) {
            return this.nameColumns.map((column) => column.format(row)).join("|");
        },
        sortBy(key) {
            if (this.sortKey === key) {
                this.sortAsc = !this.sortAsc;
            } else {
                this.sortKey = key;
                this.sortAsc = this.nameColumns.some((column) => column.key === key) || key === "success";
            }
        },
    },
};
</script>

<style scoped>
.card-head {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.5rem 1rem;
}
.card-head > h6 {
    flex: 1 1 auto;
}
.card-head > :not(h6) {
    flex: 0 1 16rem;
    min-width: 8rem;
    max-width: 100%;
}
.table-scroll {
    overflow-x: auto;
}
th.sortable {
    cursor: pointer;
    user-select: none;
}
.nowrap {
    white-space: nowrap;
}
.muted {
    color: var(--on-surface-variant);
}
</style>
