<script setup>
import { watch, ref, computed } from "vue";
const isCollapsed = ref(true);
const filter = ref("");
const selectedParams = ref([]);

const props = defineProps({
    buildName: {
        type: String,
        required: true,
    },
    paramsList: {
        type: Array,
        required: true,
    },
});

const filteredParams = computed({
    get() {
        return props.paramsList.filter((param) => param.toLowerCase().includes(filter.value.toLowerCase()));
    },
});
</script>

<template>
    <div class="params-accordion">
        <span class="params-accordion-title" @click="isCollapsed = !isCollapsed">{{ buildName }} ({{ paramsList.length }})</span>
        <ul :class="['params-list', { collapsed: isCollapsed }]">
            <li><input placeholder="Filter" v-model="filter" /></li>
            <li v-for="param in filteredParams" :key="param">
                <input type="checkbox" :id="param" v-model="selectedParams" :value="param">
                <label :for="param">
                    {{ param }}
                </label>
            </li>
        </ul>
    </div>
</template>

<style lang="scss" scoped>
.params-accordion {
    @apply flex flex-col gap-4 rounded shadow-md bg-white dark:bg-gray-800 dark:text-white p-2;
    .params-accordion-title {
        @apply font-bold cursor-pointer;
    }
    .params-list {
        @apply flex flex-col gap-2 pl-4 m-0 py-0;
        &.collapsed {
            display: none;
        }
        li {
            @apply flex flex-row items-center gap-2 cursor-pointer;
            label {
                @apply text-sm;
            }
            input {
                @apply mr-2;
            }
        }
    }
}
</style>
