<template>
    <div
        class="row medium-padding round large-text"
        style="background-color: var(--surface-container)"
        :data-cy="job.name"
    >
        <div class="max">
            <div>{{ job.name }}</div>
            <small class="m l">{{ job.desc }}</small>
        </div>
        <div class="m l">{{ job.interval }}</div>
        <label class="switch m l">
            <input
                :checked="isActive"
                type="checkbox"
                :disabled="!canAny('write')"
                @click.prevent="toggleIsActive"
            />
            <span></span>
        </label>

        <!-- Start job -->
        <RunJobButton
            v-if="can('run', job.name)"
            :disabled="!isActive"
            :params="job.defaultParams"
            :job-name="job.name"
        />

        <!-- Edit job, or only view it without write -->
        <router-link
            :to="{ name: 'jobEdit', params: { name: job.name } }"
            class="button circle transparent"
            data-cy="edit-job-button"
        >
            <i>{{ canAny("write") ? "edit" : "visibility" }}</i>
            <div class="tooltip bottom">{{ canAny("write") ? "Edit" : "View" }}</div>
        </router-link>

        <!-- Delete job -->
        <button
            v-if="canAny('write')"
            class="circle transparent"
            data-cy="delete-job-button"
            @click.prevent="toggleModalDelete"
        >
            <i>delete</i>
            <div class="tooltip bottom">Delete</div>
        </button>

        <dialog
            :class="{ active: modalDelete }"
            class="padding"
        >
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
import RunJobButton from "@/components/RunJobButton.vue";
import vuex from "vuex";
import { escapeHtml } from "@/store/utils";
import axios from "axios";

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
    computed: {
        ...vuex.mapGetters(["can", "canAny"]),
    },
    methods: {
        deleteJob() {
            const url = `/api/job/${encodeURIComponent(this.job.name)}`;
            axios
                .delete(url)
                .then(() => {
                    this.$notify({
                        text: `${escapeHtml(this.job.name)} has been deleted`,
                        type: "primary",
                    });
                    this.toggleModalDelete();
                    this.$router.go();
                })
                .catch(() => {});
        },
        toggleModalDelete() {
            this.modalDelete = !this.modalDelete;
        },
        toggleIsActive() {
            const url = `/api/job/${encodeURIComponent(this.job.name)}/set_active`;
            const data = new FormData();
            data.append("active", String(!this.isActive));
            axios
                .post(url, data)
                .then((response) => {
                    this.$notify({
                        text: `Job ${escapeHtml(this.job.name)} is ` + (response.data ? "enabled" : "disabled"),
                        type: "primary",
                    });
                    this.isActive = response.data;
                })
                .catch(() => {});
        },
    },
};
</script>

<!-- Add "scoped" attribute to limit CSS to this component only -->
<style scoped></style>
