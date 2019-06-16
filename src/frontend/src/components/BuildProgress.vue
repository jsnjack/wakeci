<template>
  <progress class="progress" :value="done" :max="total"></progress>
</template>

<script>
export default {
    props: {
        done: {
            required: true,
        },
        total: {
            required: true,
        },
    },
};
</script>

<style scoped lang="scss">
</style>
