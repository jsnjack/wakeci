<template>
    <div
        ref="root"
        class="menu"
    >
        <button
            ref="button"
            type="button"
            :class="buttonClass || 'btn btn-ghost btn-icon'"
            :aria-label="label"
            :data-tip="open || $slots.button ? null : label"
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
            ref="list"
            class="menu-list"
            role="menu"
            :style="listStyle"
            @click="closeOnClick && (open = false)"
        >
            <slot />
        </div>
    </div>
</template>

<script>
// UiMenu is a button that opens a list of actions under it; a click on an
// action, outside the menu or Esc closes it. The list has a fixed position,
// so no container that clips its content cuts it off, and stays inside the
// window: aligned with the button's end, or its start where that doesn't
// fit, and above the button where there's more room there

const MARGIN = 8;
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
        // Hidden until placed, so it doesn't show where it doesn't stay
        return { open: false, listStyle: { visibility: "hidden" } };
    },
    watch: {
        open(value) {
            if (value) {
                this.listStyle = { visibility: "hidden" };
                document.addEventListener("click", this.onDocumentClick, true);
                document.addEventListener("keydown", this.onKeyDown);
                window.addEventListener("scroll", this.place, { capture: true, passive: true });
                window.addEventListener("resize", this.place);
                this.$nextTick(this.place);
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
            window.removeEventListener("scroll", this.place, { capture: true });
            window.removeEventListener("resize", this.place);
        },
        place(event) {
            const list = this.$refs.list;
            if (!list || !this.$refs.button || (event?.type === "scroll" && list.contains(event.target))) {
                return;
            }
            const button = this.$refs.button.getBoundingClientRect();
            const vw = document.documentElement.clientWidth;
            const vh = window.innerHeight;
            const width = list.offsetWidth;
            let left = button.right - width;
            if (left < MARGIN) {
                left = button.left;
            }
            left = Math.max(MARGIN, Math.min(left, vw - width - MARGIN));
            const below = vh - button.bottom - 4 - MARGIN;
            const above = button.top - 4 - MARGIN;
            const height = list.scrollHeight;
            const up = height > below && above > below;
            const room = up ? above : below;
            const top = up ? button.top - 4 - Math.min(height, room) : button.bottom + 4;
            this.listStyle = { left: `${left}px`, top: `${top}px`, maxBlockSize: `${room}px` };
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
