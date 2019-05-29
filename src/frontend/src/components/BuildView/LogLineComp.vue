<template>
    <span>{{ log.data }}</span>
</template>

<script>

export default {
    props: {
        log: {
            type: Object,
            required: true,
        },
    },
};
</script>

<!-- Add "scoped" attribute to limit CSS to this component only -->
<style scoped lang="scss">
    span {
        display: block;
        text-align: left;
    }
</style>
