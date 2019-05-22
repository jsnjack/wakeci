<template>
    <tr>
        <td>
            {{ feed.name }}
        </td>
        <td>
            {{ feed.count }}
        </td>
        <td>
            {{ feed.done_tasks }} / {{ feed.total_tasks }}
        </td>
        <td>
            {{ feed.status }}
        </td>
    </tr>
</template>

<script>
// import vuex from "vuex";

export default {
    props: {
        feed: {
            type: Object,
            required: true,
        },
    },
    methods: {
    },
    computed: {
    },
};
</script>

<!-- Add "scoped" attribute to limit CSS to this component only -->
<style scoped lang="scss">

</style>
