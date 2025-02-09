<template>
    <div
        class="row round large-text feed-item"
        :data-cy-build="build.id"
    >
        <BuildStatus :status="build.status" />
        <div class="max">
            <a
                class="small-padding"
                style="padding-bottom: 0 !important; padding-top: 0 !important"
            >
                <router-link :to="{ name: 'build', params: { id: build.id } }"> #{{ build.id }} {{ build.name }}</router-link>
            </a>
            <div
                data-cy="params-container"
                class="l"
            >
                <div></div>
                <ParamItem
                    v-for="(item, index) in this.getFilteredParams"
                    :key="index + 'param'"
                    :param="item"
                    @setFilter="handleSetFilter"
                />
            </div>
        </div>
        <div class="l">
            <button
                v-if="build.params && build.params.length > getFilteredParams.length"
                class="circle transparent"
                @click.prevent="toggleExpandParams"
                data-cy="expand-more-params-button"
            >
                <i>expand_more</i>
                <div class="tooltip bottom">More</div>
            </button>
            <button
                v-if="expandParams"
                class="circle transparent"
                @click.prevent="toggleExpandParams"
                data-cy="expand-less-params-button"
            >
                <i>expand_less</i>
                <div class="tooltip bottom">Less</div>
            </button>
        </div>
        <div
            style="min-width: 200px"
            class="m l"
        >
            <SimpleDuration :item="build" />
            <SimpleStartedAgo :item="build" />
        </div>
        <!-- Open build view -->
        <router-link
            :to="{ name: 'build', params: { id: build.id } }"
            class="button circle transparent m l"
            data-cy="open-build-button"
        >
            <i>arrow_forward</i>
            <div class="tooltip bottom">Open</div>
        </router-link>

        <!-- Abort build -->
        <a
            :disabled="isDone ? true : null"
            :href="getAbortURL"
            class="button circle transparent"
            data-cy="abort-build-button"
            @click.prevent="abort"
        >
            <i>stop</i>
            <div class="tooltip bottom">Abort</div>
        </a>

        <!-- Start build -->
        <StartBuildNowButton
            v-if="build.id"
            :status="build.status"
            :build-i-d="build.id"
        />
    </div>
</template>

<script>
import BuildStatus from "@/components/BuildStatus.vue";
import ParamItem from "@/components/ParamItem.vue";
import SimpleDuration from "@/components/SimpleDuration.vue";
import SimpleStartedAgo from "@/components/SimpleStartedAgo.vue";
import axios from "axios";
import StartBuildNowButton from "./StartBuildNowButton.vue";

const MAX_DEFAULT_NUMBER_OF_PARAMS = 3;

export default {
    components: {
        ParamItem,
        BuildStatus,
        SimpleStartedAgo,
        SimpleDuration,
        StartBuildNowButton,
    },
    props: {
        build: {
            type: Object,
            required: true,
        },
    },
    computed: {
        getAbortURL: function () {
            return `/api/build/${this.build.id}/abort`;
        },
        getStartURL: function () {
            return `/api/build/${this.build.id}/start`;
        },
        isDone() {
            switch (this.build.status) {
                case "failed":
                case "finished":
                case "aborted":
                case "skipped":
                case "timed out":
                    return true;
            }
            return false;
        },
        getFilteredParams() {
            // User asked to show all params
            if (this.expandParams) {
                return this.build.params;
            }

            // Limit number of params and skip empty ones
            if (this.build.params) {
                return this.build.params
                    .filter((v) => {
                        return Object.values(v)[0] !== ""; // skip empty params
                    })
                    .slice(0, MAX_DEFAULT_NUMBER_OF_PARAMS);
            }
            return [];
        },
    },
    methods: {
        abort(event) {
            if (!this.isDone) {
                axios
                    .post(event.target.href || event.target.parentElement.href)
                    .then((response) => {
                        this.$notify({ text: `${this.build.id} has been aborted`, type: "primary" });
                    })
                    .catch((error) => {});
            }
        },
        toggleExpandParams() {
            this.expandParams = !this.expandParams;
        },
        handleSetFilter(filterText) {
            this.$emit("setFilter", filterText);
        },
    },
    data: function () {
        return {
            expandParams: false,
        };
    },
};
</script>

<!-- Add "scoped" attribute to limit CSS to this component only -->
<style scoped >
.feed-item {
    background-color: var(--surface-container);
    padding: 0.5rem 0.5rem !important;
    margin-top: 0.5rem !important;
}
</style>
