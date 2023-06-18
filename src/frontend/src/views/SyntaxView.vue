<template>
    <div class="container grid-xl">
        <SyntaxViewer />
    </div>
</template>

<script>
import SyntaxViewer from "@/components/SyntaxViewer.vue";

export default {
    components: {
        SyntaxViewer,
    },
    mounted() {
        this.$store.commit("SET_CURRENT_PAGE", "Job syntax");
    },
};
</script>

<!-- Add "scoped" attribute to limit CSS to this component only -->
<style scoped lang="scss"></style>
