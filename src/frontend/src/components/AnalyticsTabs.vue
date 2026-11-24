<template>
    <div
        v-if="canAny('write')"
        class="tabs left-align"
        data-cy="analytics-tabs"
    >
        <router-link
            :to="{ name: 'analytics' }"
            :class="{ active: $route.name === 'analytics' }"
            data-cy="analytics-builds-tab"
        >
            <i>monitoring</i>
            <span>Builds</span>
        </router-link>
        <router-link
            :to="{ name: 'analyticsUsage' }"
            :class="{ active: $route.name === 'analyticsUsage' }"
            data-cy="analytics-usage-tab"
        >
            <i>api</i>
            <span>API usage</span>
        </router-link>
    </div>
</template>

<script>
import vuex from "vuex";

// AnalyticsTabs switches between the analytics of builds and of requests,
// which only write and the admin password see (Access.SeesUsage)
export default {
    computed: {
        ...vuex.mapGetters(["canAny"]),
    },
};
</script>
