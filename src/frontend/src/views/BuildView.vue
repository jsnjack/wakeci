<template>
  <div class="container">
      <div>
          <LogLineComp v-for="item in logs"
          :key="item.id"
          :log="item"
          />
      </div>
  </div>
</template>

<script>
import vuex from "vuex";
import LogLineComp from "@/components/LogLineComp.vue";

export default {
    props: {
        count: {
            required: true,
        },
        job_name: {
            type: String,
            required: true,
        },
    },
    components: {LogLineComp},
    mounted() {
        this.subscribe();
    },
    destroyed() {
        this.unsubscribe();
    },
    computed: {
        ...vuex.mapState(["ws", "logs"]),
    },
    methods: {
        subscribe() {
            this.$store.commit("ACTIVE_SUBSCRIPTION", this.subscription);
            this.ws.obj.sendMessage({
                "type": "in:subscribe",
                "data": {
                    "to": this.subscription,
                },
            });
        },
        unsubscribe() {
            this.$store.commit("ACTIVE_SUBSCRIPTION", "");
            this.ws.obj.sendMessage({
                "type": "in:unsubscribe",
                "data": {
                    "to": this.subscription,
                },
            });
        },
    },
    data: function() {
        return {
            subscription: `build:log:${this.job_name}_${this.count}`,
        };
    },
};
</script>

<style scoped lang="scss">
</style>
