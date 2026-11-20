<template>
    <dialog
        ref="dialog"
        class="modal"
        :class="{ wide }"
        :aria-labelledby="title ? titleID : null"
        @close="onClose"
        @click="onClick"
    >
        <div class="stack gap-4">
            <h2
                v-if="title"
                :id="titleID"
            >
                {{ title }}
            </h2>
            <slot />
            <div
                v-if="$slots.actions"
                class="modal-actions"
            >
                <slot name="actions" />
            </div>
        </div>
    </dialog>
</template>

<script>
let dialogs = 0;

// UiDialog is a modal dialog on the native <dialog>: showModal() keeps focus
// in it and the page under it inert, Esc closes it, and so does a click
// beside it. v-model:open opens and closes it.
export default {
    props: {
        open: {
            type: Boolean,
            default: false,
        },
        title: {
            type: String,
            default: "",
        },
        // Wider, for a form of several fields
        wide: {
            type: Boolean,
            default: false,
        },
    },
    emits: ["update:open"],
    data() {
        return { titleID: `dialog-title-${++dialogs}` };
    },
    watch: {
        open: "sync",
    },
    mounted() {
        this.sync();
    },
    methods: {
        sync() {
            const dialog = this.$refs.dialog;
            if (this.open && !dialog.open) {
                dialog.showModal();
            } else if (!this.open && dialog.open) {
                dialog.close();
            }
        },
        onClose() {
            if (this.open) {
                this.$emit("update:open", false);
            }
        },
        // A click on the backdrop reaches the dialog itself, outside its box
        onClick(event) {
            if (event.target !== this.$refs.dialog) {
                return;
            }
            const box = this.$refs.dialog.getBoundingClientRect();
            const inside = event.clientX >= box.left && event.clientX <= box.right && event.clientY >= box.top && event.clientY <= box.bottom;
            if (!inside) {
                this.$refs.dialog.close();
            }
        },
    },
};
</script>
