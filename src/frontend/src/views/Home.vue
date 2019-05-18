<template>
  <div class="home">
    <FeedComp />
    <JobsComp />
  </div>
</template>

<script>
// @ is an alias to /src
import FeedComp from "@/components/FeedComp.vue";
import JobsComp from "@/components/JobsComp.vue";

export default {
    name: "home",
    components: {
        FeedComp, JobsComp,
    },
};
</script>

<style scoped lang="scss">
.home {
  display: flex;
  justify-content: space-evenly;
}
</style>
