<template>
    <div class="form-group">
        <label
            class="form-label"
            :for="getForValue"
            >{{ getParamsKey }}</label
        >
        <input
            :id="getForValue"
            class="form-input"
            type="text"
            :name="getParamsKey"
            :value="getParamsValue"
        />
    </div>
</template>

<script>
export default {
    props: {
        params: {
            type: Object,
            required: true,
        },
    },
    computed: {
        getParamsKey() {
            return Object.keys(this.params)[0];
        },
        getParamsValue() {
            return this.params[this.getParamsKey];
        },
        getForValue() {
            return `input-${this.getParamsKey}`;
        },
    },
    methods: {},
};
</script>

<style
    lang="scss"
    scoped
></style>
