<template>
    <div class="container">
        <div class="card">
            <div class="card-header">
                <div class="card-title h5">show_env</div>
                <div class="card-subtitle text-gray">build # 5</div>
            </div>
            <div class="card-footer">
                <progress
                    class="progress"
                    :value="3"
                    :max="5">
                </progress>
            </div>
        </div>

        <!-- TODO build parameters -->
        <details class="accordion text-left" open>
            <summary class="accordion-header c-hand">
                <i class="icon icon-arrow-right mr-1"></i>
                Build params
            </summary>
            <div class="accordion-body">
                fewef
                we
            </div>
        </details>


    </div>
      <!-- <div style="width=700px;">
          <LogLineComp v-for="item in logs"
          :key="item.id"
          :log="item"
          />
      </div> -->
</template>

<script>
import vuex from "vuex";
import LogLineComp from "@/components/BuildView/LogLineComp.vue";

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
@import "@/assets/colors.scss";

.container {
    width: 80%;
    display: flex;
    flex-direction: column;
    .card {
        width: 100%;
    }
}
.accordion-body {
    background: $gray-color-light;
}
</style>
