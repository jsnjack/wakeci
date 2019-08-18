<template>
  <span>
    <a href="#" @click.prevent="toggleModal" class="btn btn-success">{{ buttonTitle }}</a>

    <div class="modal" v-bind:class="{active: modalOpen}">
      <a href="#" @click.prevent="toggleModal" class="modal-overlay" aria-label="Close"></a>
      <div class="modal-container">
        <div class="modal-header">
          <a
            href="#"
            @click.prevent="toggleModal"
            class="btn btn-clear float-right"
            aria-label="Close"
          ></a>
          <div class="modal-title text-uppercase">{{ getModalTitle }}</div>
        </div>
        <div class="modal-body">
          <div class="content">
            <form v-show="this.params" ref="form">
              <RunFormItem v-for="item in params" :key="item.name" :params="item"></RunFormItem>
            </form>
            <div class="empty" v-show="!this.params">
              <p class="empty-title h6 text-uppercase">Empty</p>
            </div>
          </div>
        </div>
        <div class="modal-footer">
          <a href="#" @click.prevent="run" class="btn btn-primary float-right">Add to queue</a>
        </div>
      </div>
    </div>
  </span>
</template>

<script>
import RunFormItem from "@/components/RunFormItem";
import axios from "axios";

export default {
    components: {RunFormItem},
    props: {
        params: {
            required: true,
        },
        buttonTitle: {
            type: String,
            required: true,
        },
        jobName: {
            required: true,
        },
    },
    computed: {
        ismodalOpen: function() {
            return this.modalOpen;
        },
        getModalTitle: function() {
            return `${this.jobName} job parameters`;
        },
    },
    methods: {
        run(event) {
            this.toggleModal();
            const url =
        `/api/job/${this.jobName}/run?` +
        new URLSearchParams(
            Array.from(new FormData(this.$refs.form))
        ).toString();
            axios
                .post(url)
                .then((response) => {
                    this.$notify({
                        text: `${this.jobName} has been scheduled (#${response.data})`,
                        type: "success",
                    });
                })
                .catch((error) => {});
        },
        toggleModal(event) {
            this.modalOpen = !this.modalOpen;
        },
    },
    data: function() {
        return {
            modalOpen: false,
        };
    },
};
</script>

<style lang="scss" scoped>
</style>
