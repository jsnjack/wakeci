<template>
  <section>
    <h4 class="text-left">{{ task.name }}</h4>
    <div class="log-container text-left">
        <span v-for="item in task.logs" :key="item.id" class="d-block">{{ item.data }}</span>
    </div>
  </section>
</template>

<script>
export default {
    props: {
        task: {
            required: true,
        },
    },
};
</script>

<style lang="scss" scoped>
@import "@/assets/colors.scss";

.log-container {
    background: $bg-color;
    margin-left: 1em;
}
</style>
