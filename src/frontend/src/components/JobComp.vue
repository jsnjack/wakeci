<template>
    <tr>
        <td>{{ job.name }}</td>
        <td>{{ job.count }}</td>
        <td>
            <a :href="getRunURL" @click.prevent="run" class="btn btn-primary">Run</a>
        </td>
    </tr>
</template>

<script>
import vuex from "vuex";
import axios from "axios";

export default {
    props: {
        job: {
            type: Object,
            required: true,
        },
    },
    methods: {
        run(event) {
            axios.post(event.target.href)
                .then(function(response) {
                    console.log(response);
                })
                .catch(function(error) {
                    console.log(error);
                });
        },
    },
    computed: {
        ...vuex.mapState([
            "api",
        ]),
        getRunURL: function() {
            return `${this.api.baseURL}/job/${this.job.name}/run`;
        },
    },
};
</script>

<!-- Add "scoped" attribute to limit CSS to this component only -->
<style scoped lang="scss">

</style>
