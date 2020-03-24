<template>
  <progress
    class="progress"
    :value="done"
    :max="total"
  />
</template>

<script>
export default {
    props: {
        done: {
            type: Number,
            required: true,
        },
        total: {
            type: Number,
            required: true,
        },
    },
};
</script>

<style scoped lang="scss">
</style>
