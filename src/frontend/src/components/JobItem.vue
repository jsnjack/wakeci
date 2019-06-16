<template>
    <tr>
        <td>{{ job.name }}</td>
        <td>
            <a :href="getRunURL" @click.prevent="run" class="btn btn-primary">Run</a>
        </td>
    </tr>
</template>

<script>
import axios from "axios";
import {APIURL} from "@/store/communication";


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
                .then((response) => {
                    console.log(response);
                    this.$notify({
                        text: `${this.job.name} has been scheduled (#${response.data})`,
                        type: "success",
                    });
                })
                .catch((error) => {
                    this.$notify({
                        text: error,
                        type: "error",
                    });
                });
        },
    },
    computed: {
        getRunURL: function() {
            return `${APIURL}/job/${this.job.name}/run`;
        },
    },
};
</script>

<!-- Add "scoped" attribute to limit CSS to this component only -->
<style scoped lang="scss">

</style>
