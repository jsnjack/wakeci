<template>
    <div class="more-options-container" @click.stop="popoverToggle = !popoverToggle">
        <span class="material-icons more-options"> more_vert </span>
        <div v-show="popoverToggle" class="options-popover">
            <input
                class="search-options small"
                v-model="search"
                placeholder="Filter..."
                @click.stop
            />
            <ul class="options-list">
                <li
                    class="option"
                    v-for="opt in filteredOptions"
                    :key="opt.name"
                    @click="opt.onClick ? opt.onClick() : null"
                >
                    <span class="material-icons" v-if="opt.icon">{{ opt.icon }}</span>
                    {{ opt.name }}
                </li>
            </ul>
        </div>
    </div>
</template>

<script>
export default {
    name: 'MoreOptions',
    props: {
        optionsList: {
            type: Array,
            required: true,
        },
    },
    data() {
        return {
            popoverToggle: false,
            search: '',
        };
    },
    mounted() {
        window.addEventListener('click', this.closePopover);
    },
    beforeUnmount() {
        window.removeEventListener('click', this.closePopover);
    },
    methods: {
        closePopover() {
            this.popoverToggle = false;
        },
    },
    computed: {
        filteredOptions() {
            if (!this.search) {
                return this.optionsList;
            }
            return this.optionsList.filter((opt) =>
                opt.name.toLowerCase().includes(this.search.toLowerCase()),
            );
        },
    },
};
</script>

<style lang="scss" scoped>
.more-options-container {
    @apply relative;
    .more-options {
        @apply ring-1 ring-gray-border flex items-center justify-center rounded-sm cursor-pointer hover:bg-gray-light px-0.5 z-10;
    }
    .options-popover {
        @apply absolute z-50 bg-white ring-1 ring-gray-border rounded-sm shadow-md top-full p-1 right-0 transform translate-y-2 w-80 max-h-60 overflow-y-auto;
        .search-options {
            @apply w-full mb-2;
        }
        .options-list {
            @apply appearance-none m-0 p-0 border-t border-gray-light pt-2;
            .option {
                @apply py-1 px-2 text-sm cursor-pointer hover:bg-gray-light rounded-md flex justify-start items-center gap-2;
            }
        }
    }
}
</style>
