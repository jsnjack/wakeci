<template>
  <div class="container">
  </div>
</template>

<script>
import vuex from "vuex";

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
    mounted() {
        this.subscribe();
    },
    destroyed() {
        this.unsubscribe();
    },
    computed: {
        ...vuex.mapState(["ws"]),
    },
    methods: {
        subscribe() {
            this.ws.obj.sendMessage({
                "type": "in:subscribe",
                "data": {
                    "to": `${this.job_name}_${this.count}`,
                },
            });
        },
        unsubscribe() {
            this.ws.obj.sendMessage({
                "type": "in:unsubscribe",
                "data": {
                    "to": `${this.job_name}_${this.count}`,
                },
            });
        },
    },
};
</script>

<style scoped lang="scss">
</style>
