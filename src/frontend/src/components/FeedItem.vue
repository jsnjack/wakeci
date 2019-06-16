<template>
  <tr>
    <td>{{ build.name }}</td>
    <td>
      <router-link :to="{ name: 'build', params: { id: build.id}}">{{ build.id }}</router-link>
    </td>
    <td class="tooltip tooltip-right" :data-tooltip="getProgressTooltip">
      <BuildProgress :done="build.done_tasks" :total="build.total_tasks"/>
    </td>
    <td>
      <BuildStatus :status="build.status"></BuildStatus>
    </td>
  </tr>
</template>

<script>
import BuildStatus from "@/components/BuildStatus";
import BuildProgress from "@/components/BuildProgress";

export default {
    components: {BuildStatus, BuildProgress},
    props: {
        build: {
            type: Object,
            required: true,
        },
    },
    computed: {
        getProgressTooltip() {
            return `${this.build.done_tasks} of ${this.build.total_tasks}`;
        },
    },
};
</script>

<!-- Add "scoped" attribute to limit CSS to this component only -->
<style scoped lang="scss">
</style>
