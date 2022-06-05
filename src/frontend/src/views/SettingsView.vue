<template>
    <Card class="settings-card">
        <form method="post" @submit.prevent="save">
            <div class="card-header">
                <h3>Settings</h3>
            </div>
            <div>
                <div class="form-item">
                    <label class="form-label" for="password">Password</label>
                    <input id="password" v-model="password" class="form-input" type="password" />
                </div>

                <div class="form-item">
                    <label class="form-label" for="concurrent-builds"
                        >Number of concurrent builds</label
                    >
                    <input
                        id="concurrent-builds"
                        v-model="concurrentBuilds"
                        class="form-input"
                        type="number"
                        min="1"
                    />
                </div>
                <div class="form-item">
                    <label class="form-label" for="build-history-size"
                        >Number of builds to preserve</label
                    >
                    <input
                        id="build-history-size"
                        v-model="buildHistorySize"
                        class="form-input"
                        type="number"
                        min="1"
                    />
                </div>
            </div>
            <button data-cy="save-settings" type="submit" class="btn btn-primary save-btn">
                Save
            </button>
        </form>
    </Card>
</template>

<script>
import axios from 'axios';
import Card from '@/components/ui/Card.vue';

export default {
    components: {
        Card,
    },
    data: function () {
        return {
            password: '',
            concurrentBuilds: 2,
            buildHistorySize: 200,
        };
    },
    mounted() {
        document.title = 'Settings - wakeci';
        this.fetch();
    },
    methods: {
        save() {
            const data = new FormData();
            data.append('password', this.password);
            data.append('concurrentBuilds', this.concurrentBuilds);
            data.append('buildHistorySize', this.buildHistorySize);
            axios
                .post('/api/settings', data, {
                    headers: {
                        'Content-type': 'application/x-www-form-urlencoded',
                    },
                })
                .then((response) => {
                    this.$notify({
                        text: 'Saved',
                        type: 'success',
                    });
                })
                .catch((error) => {});
        },
        fetch() {
            axios
                .get('/api/settings')
                .then((response) => {
                    if (response.data.concurrentBuilds) {
                        this.concurrentBuilds = response.data.concurrentBuilds;
                    }
                    if (response.data.buildHistorySize) {
                        this.buildHistorySize = response.data.buildHistorySize;
                    }
                })
                .catch((error) => {});
        },
    },
};
</script>

<!-- Add "scoped" attribute to limit CSS to this component only -->
<style scoped lang="scss">
.settings-card {
    @apply min-w-max dark:bg-secondary dark:text-gray-extra-light;
    width: 33%;
    .save-btn {
        @apply mt-4 ml-auto;
    }
}
</style>
