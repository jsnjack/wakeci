<template>
    <nav
        class="tabbar"
        aria-label="Job"
    >
        <router-link
            :to="{ name: 'jobEdit', params: { name } }"
            data-cy="job-file-tab"
            :aria-current="current === 'edit' ? 'page' : null"
        >
            <i class="icon small">description</i>
            <span>{{ canAny("write") ? "Edit" : "View" }}</span>
        </router-link>
        <router-link
            :to="{ name: 'jobInsights', params: { name } }"
            data-cy="job-insights-tab"
            :aria-current="current === 'insights' ? 'page' : null"
        >
            <i class="icon small">insights</i>
            <span>Insights</span>
        </router-link>
    </nav>
</template>

<script>
import vuex from "vuex";

// The tabs of a job's pages: its file, and how its builds went
export default {
    props: {
        name: {
            type: String,
            required: true,
        },
        // edit or insights
        current: {
            type: String,
            required: true,
        },
    },
    computed: {
        ...vuex.mapGetters(["canAny"]),
    },
};
</script>
