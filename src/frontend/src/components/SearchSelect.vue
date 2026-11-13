<template>
    <div
        class="field suffix border small round no-margin search-select"
        :class="[{ label: !!label }, $attrs.class]"
    >
        <input
            :id="inputID"
            ref="input"
            v-bind="inputAttrs"
            :value="text"
            type="text"
            class="active"
            role="combobox"
            autocomplete="off"
            spellcheck="false"
            aria-autocomplete="both"
            :aria-label="label ? null : ariaLabel"
            :aria-expanded="open ? 'true' : 'false'"
            :aria-controls="listID"
            :aria-activedescendant="open && active >= 0 ? optionID(active) : null"
            @focus="onFocus"
            @click="openList"
            @input="onInput"
            @keydown="onKeydown"
            @blur="onBlur"
        />
        <label
            v-if="label"
            :for="inputID"
            class="active"
            >{{ label }}</label
        >
        <i
            class="toggle"
            aria-hidden="true"
            @mousedown.prevent="toggle"
            >{{ open ? "arrow_drop_up" : "arrow_drop_down" }}</i
        >
        <ul
            v-show="open"
            :id="listID"
            ref="list"
            class="search-list"
            role="listbox"
            :aria-label="label || ariaLabel"
        >
            <li
                v-for="(option, i) in matches"
                :id="optionID(i)"
                :key="option.value"
                role="option"
                :aria-selected="option.value === modelValue ? 'true' : 'false'"
                :class="{ active: i === active, selected: option.value === modelValue }"
                :title="option.label"
                data-cy="search-option"
                @mousedown.prevent="choose(option)"
                @mousemove="active = i"
            >
                <template
                    v-for="(part, p) in highlight(option.label)"
                    :key="p"
                >
                    <b v-if="part.match">{{ part.text }}</b>
                    <template v-else>{{ part.text }}</template>
                </template>
            </li>
            <li
                v-if="!matches.length"
                class="none"
                aria-disabled="true"
            >
                No match
            </li>
        </ul>
    </div>
</template>

<script>
import { uniqueID } from "@/analytics";

// SearchSelect is a drop-down list you can type in. Typing narrows the list
// to the options that contain the text, those that start with it first, and
// fills in the rest of the first option that starts with it, selected, so
// typing goes on over it and Enter, Tab or leaving the field takes it. Arrow
// keys move through the list and Escape puts the choice back. Attributes
// such as data-cy go to the input, class to the field
export default {
    inheritAttrs: false,
    props: {
        modelValue: {
            type: String,
            default: "",
        },
        // { value, label }
        options: {
            type: Array,
            required: true,
        },
        // The field's floating label; without one, ariaLabel names it
        label: {
            type: String,
            default: "",
        },
        ariaLabel: {
            type: String,
            default: "",
        },
    },
    emits: ["update:modelValue"],
    data() {
        const id = uniqueID("search-select");
        return {
            inputID: `${id}-input`,
            listID: `${id}-list`,
            // What the field shows
            text: "",
            // What the user typed, without the filled-in rest; null until
            // they type, when the list shows every option
            typed: null,
            open: false,
            active: -1,
        };
    },
    computed: {
        // Every attribute but class, which goes to the field
        inputAttrs() {
            return Object.fromEntries(Object.entries(this.$attrs).filter(([name]) => name !== "class"));
        },
        // The chosen option's label; a value without an option, such as a
        // job from the address that no longer exists, shows as itself
        selectedLabel() {
            const selected = this.options.find((o) => o.value === this.modelValue);
            return selected ? selected.label : this.modelValue;
        },
        matches() {
            if (!this.typed) {
                return this.options;
            }
            const q = this.typed.toLowerCase();
            const starting = [];
            const containing = [];
            for (const option of this.options) {
                const label = option.label.toLowerCase();
                if (label.startsWith(q)) {
                    starting.push(option);
                } else if (label.includes(q)) {
                    containing.push(option);
                }
            }
            return [...starting, ...containing];
        },
    },
    watch: {
        selectedLabel: {
            handler(label) {
                if (!this.open) {
                    this.text = label;
                }
            },
            immediate: true,
        },
    },
    methods: {
        optionID(i) {
            return `${this.listID}-${i}`;
        },
        openList() {
            if (this.open) {
                return;
            }
            this.typed = null;
            this.open = true;
            this.active = this.matches.findIndex((o) => o.value === this.modelValue);
            this.$nextTick(() => {
                this.$refs.input.select();
                this.scrollToActive();
            });
        },
        onFocus() {
            this.openList();
        },
        toggle() {
            if (this.open) {
                this.close();
                this.$refs.input.blur();
            } else {
                this.$refs.input.focus();
                this.openList();
            }
        },
        onInput(event) {
            const input = event.target;
            const value = input.value;
            this.typed = value;
            this.open = true;
            this.active = this.matches.length ? 0 : -1;
            const first = this.matches[0];
            // Not while deleting, which would put back what was just deleted
            const deleting = (event.inputType || "").startsWith("delete");
            if (!deleting && value && first && first.label.toLowerCase().startsWith(value.toLowerCase())) {
                // In the option's own spelling, which may differ in case
                this.text = first.label;
                input.value = this.text;
                input.setSelectionRange(value.length, this.text.length);
            } else {
                this.text = value;
            }
            this.$nextTick(this.scrollToActive);
        },
        onKeydown(event) {
            switch (event.key) {
                case "ArrowDown":
                case "ArrowUp": {
                    event.preventDefault();
                    if (!this.open) {
                        this.openList();
                        return;
                    }
                    const n = this.matches.length;
                    if (n) {
                        const step = event.key === "ArrowDown" ? 1 : -1;
                        this.active = this.active < 0 ? (step > 0 ? 0 : n - 1) : (this.active + step + n) % n;
                        this.$nextTick(this.scrollToActive);
                    }
                    return;
                }
                case "Enter":
                    if (this.open) {
                        event.preventDefault();
                        const option = this.matches[this.active];
                        if (option) {
                            this.choose(option);
                        } else {
                            this.close();
                        }
                    }
                    return;
                case "Escape":
                    if (this.open) {
                        event.preventDefault();
                        this.close();
                    }
                    return;
            }
        },
        // Leaving the field takes the option it names, such as one filled
        // in; anything else puts the choice back
        onBlur() {
            if (!this.open) {
                return;
            }
            const text = this.text.trim().toLowerCase();
            const named = this.typed !== null && this.options.find((o) => o.label.toLowerCase() === text);
            if (named) {
                this.choose(named);
            } else {
                this.close();
            }
        },
        choose(option) {
            this.open = false;
            this.typed = null;
            this.text = option.label;
            if (option.value !== this.modelValue) {
                this.$emit("update:modelValue", option.value);
            }
        },
        close() {
            this.open = false;
            this.typed = null;
            this.text = this.selectedLabel;
        },
        scrollToActive() {
            const item = this.$refs.list && this.$refs.list.children[this.active];
            if (item) {
                item.scrollIntoView({ block: "nearest" });
            }
        },
        // highlight splits a label around the first place it contains what
        // was typed
        highlight(label) {
            if (!this.typed) {
                return [{ text: label, match: false }];
            }
            const at = label.toLowerCase().indexOf(this.typed.toLowerCase());
            if (at < 0) {
                return [{ text: label, match: false }];
            }
            const end = at + this.typed.length;
            return [
                { text: label.slice(0, at), match: false },
                { text: label.slice(at, end), match: true },
                { text: label.slice(end), match: false },
            ].filter((part) => part.text);
        },
    },
};
</script>

<style scoped>
.search-select {
    position: relative;
}
.search-select > input {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}
.toggle {
    cursor: pointer;
}
.search-list {
    position: absolute;
    inset-inline: 0;
    top: calc(100% + 0.25rem);
    z-index: 20;
    max-block-size: 18rem;
    overflow-y: auto;
    margin: 0;
    padding: 0.25rem 0;
    list-style: none;
    background-color: var(--surface-container);
    color: var(--on-surface);
    border-radius: 0.75rem;
    box-shadow: var(--elevate2);
}
.search-list li {
    padding: 0.5rem 1rem;
    cursor: pointer;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    line-height: 1.25rem;
}
.search-list li.active {
    background-color: var(--secondary-container);
    color: var(--on-secondary-container);
}
.search-list li.selected {
    font-weight: 600;
}
.search-list li.none {
    cursor: default;
    color: var(--on-surface-variant);
}
.search-list b {
    color: var(--primary);
}
.search-list li.active b {
    color: inherit;
    text-decoration: underline;
}
</style>
