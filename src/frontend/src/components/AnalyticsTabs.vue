<template>
    <nav
        v-if="canAny('write')"
        class="tabbar"
        aria-label="Analytics"
        data-cy="analytics-tabs"
    >
        <router-link
            :to="{ name: 'analytics' }"
            :class="{ active: $route.name === 'analytics' }"
            :aria-current="$route.name === 'analytics' ? 'page' : null"
            data-cy="analytics-builds-tab"
        >
            <i class="icon small">monitoring</i>
            <span>Builds</span>
        </router-link>
        <router-link
            :to="{ name: 'analyticsUsage' }"
            :class="{ active: $route.name === 'analyticsUsage' }"
            :aria-current="$route.name === 'analyticsUsage' ? 'page' : null"
            data-cy="analytics-usage-tab"
        >
            <i class="icon small">api</i>
            <span>API usage</span>
        </router-link>
    </nav>
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
