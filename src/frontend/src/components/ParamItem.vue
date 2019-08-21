<template>
  <div class="label param">{{ getName }} = {{ getValue }}</div>
</template>

<script>
export default {
    props: {
        param: {
            required: true,
            type: Object,
        },
    },
    computed: {
        getName() {
            return Object.keys(this.param)[0];
        },
        getValue() {
            return this.param[this.getName];
        },
    },
};
</script>

<style scoped lang="scss">
.param {
  margin: 0.25em;
}
.param:hover{
    cursor: default;
}
</style>
