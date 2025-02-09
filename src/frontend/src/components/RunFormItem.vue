<template>
    <div class="field label border">
        <input
            :id="getForValue"
            type="text"
            :name="getParamsKey"
            :value="getParamsValue"
        />
        <label>{{ getParamsKey }}</label>
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

<style  scoped></style>
