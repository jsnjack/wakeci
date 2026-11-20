<template>
    <div
        ref="root"
        class="menu"
    >
        <button
            type="button"
            :class="buttonClass || 'btn btn-ghost btn-icon'"
            :aria-label="label"
            :aria-expanded="open ? 'true' : 'false'"
            :data-cy="cy"
            @click="open = !open"
        >
            <i
                v-if="icon"
                class="icon"
                >{{ icon }}</i
            >
            <slot name="button" />
        </button>
        <div
            v-if="open"
            class="menu-list"
            role="menu"
            @click="closeOnClick && (open = false)"
        >
            <slot />
        </div>
    </div>
</template>

<script>
// UiMenu is a button that opens a list of actions under it; a click on an
// action, outside the menu or Esc closes it
export default {
    props: {
        label: {
            type: String,
            default: "More",
        },
        icon: {
            type: String,
            default: "more_horiz",
        },
        // The button's classes, an icon button's by default
        buttonClass: {
            type: String,
            default: "",
        },
        cy: {
            type: String,
            default: null,
        },
        // Off for a list of choices, such as checkboxes, that stays open
        closeOnClick: {
            type: Boolean,
            default: true,
        },
    },
    data() {
        return { open: false };
    },
    watch: {
        open(value) {
            if (value) {
                document.addEventListener("click", this.onDocumentClick, true);
                document.addEventListener("keydown", this.onKeyDown);
            } else {
                this.stopListening();
            }
        },
    },
    unmounted() {
        this.stopListening();
    },
    methods: {
        stopListening() {
            document.removeEventListener("click", this.onDocumentClick, true);
            document.removeEventListener("keydown", this.onKeyDown);
        },
        onDocumentClick(event) {
            if (!this.$refs.root.contains(event.target)) {
                this.open = false;
            }
        },
        onKeyDown(event) {
            if (event.key === "Escape") {
                this.open = false;
            }
        },
    },
};
</script>
