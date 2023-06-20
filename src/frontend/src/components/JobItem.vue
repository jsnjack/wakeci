<template>
    <div
        class="row medium-padding round large-text"
        style="background-color: var(--background)"
        :data-cy="job.name"
    >
        <div class="max">
            <div>{{ job.name }}</div>
            <small>{{ job.desc }}</small>
        </div>
        <div>{{ job.interval }}</div>
        <label class="switch">
            <input
                :checked="isActive"
                type="checkbox"
                @click.prevent="toggleIsActive"
            />
            <span></span>
        </label>

        <!-- Start job -->
        <RunJobButton
            :disabled="!isActive"
            :params="job.defaultParams"
            :job-name="job.name"
        />

        <!-- Edit job -->
        <router-link
            :to="{ name: 'jobEdit', params: { name: job.name } }"
            class="button circle transparent"
            data-cy="edit-job-button"
        >
            <i>edit</i>
            <div class="tooltip bottom">Edit</div>
        </router-link>

        <!-- Delete job -->
        <button
            class="circle transparent"
            data-cy="delete-job-button"
            @click.prevent="toggleModalDelete"
        >
            <i>delete</i>
            <div class="tooltip bottom">Delete</div>
        </button>

        <dialog :class="{ active: modalDelete }">
            <div>
                Are you sure you want to delete <b>{{ job.name }}</b
                >?
            </div>
            <nav class="right-align">
                <button
                    class="border"
                    @click="toggleModalDelete"
                >
                    Cancel
                </button>
                <button
                    data-cy="delete-job-confirm"
                    @click.prevent="deleteJob"
                >
                    Confirm
                </button>
            </nav>
        </dialog>
    </div>
</template>

<script>
import axios from "axios";
import RunJobButton from "@/components/RunJobButton.vue";

export default {
    components: { RunJobButton },
    props: {
        job: {
            type: Object,
            required: true,
        },
    },
    data: function () {
        return {
            modalDelete: false,
            isActive: this.job.active === "true",
        };
    },
    computed: {},
    methods: {
        deleteJob(event) {
            const url = `/api/job/${this.job.name}`;
            axios
                .delete(url)
                .then((response) => {
                    this.$notify({
                        text: `${this.job.name} has been deleted`,
                        type: "primary",
                    });
                    this.toggleModalDelete();
                    this.$router.go();
                })
                .catch((error) => {});
        },
        toggleModalDelete() {
            this.modalDelete = !this.modalDelete;
        },
        toggleIsActive() {
            const url = `/api/job/${this.job.name}/set_active`;
            const data = new FormData();
            data.append("active", String(!this.isActive));
            axios
                .post(url, data)
                .then((response) => {
                    this.$notify({
                        text: `Job ${this.job.name} is ` + (response.data ? "enabled" : "disabled"),
                        type: "primary",
                    });
                    this.isActive = response.data;
                })
                .catch((error) => {});
        },
    },
};
</script>

<!-- Add "scoped" attribute to limit CSS to this component only -->
<style scoped lang="scss"></style>
