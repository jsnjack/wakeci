<template>
    <div class="modal-bg">
        <Card class="modal" role="alertdialog" aria-modal="true">
            <header v-if="title">
                <div class="modal-title">
                    <h3>{{ title }}</h3>
                    <span @click.prevent="$emit('close')" class="material-icons close-icon">
                        close
                    </span>
                </div>
            </header>
            <main class="modal-body">
                <slot />
            </main>
        </Card>
    </div>
</template>

<script>
import Card from './Card.vue';

export default {
    components: {
        Card,
    },
    props: {
        title: {
            type: String,
            default: null,
        },
        closeIcon: {
            type: Boolean,
            default: true,
        },
    },
};
</script>

<style lang="scss" scoped>
.modal-bg {
    @apply fixed z-40 top-0 left-0 right-0 bottom-0 bg-black/[.4] h-screen w-screen flex items-center justify-center;
    .modal {
        @apply z-50 opacity-100 dark:bg-secondary dark:text-gray-light text-secondary;
        .modal-title {
            @apply my-2 grid items-center gap-4;
            grid-template-columns: auto auto;
            .close-icon {
                @apply ml-8 cursor-pointer text-gray-border hover:text-gray-border-dark;
            }
        }
        .modal-body {
            @apply max-h-96 overflow-y-auto p-2;
        }
    }
}
</style>
