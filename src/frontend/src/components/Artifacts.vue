<template>
  <section>
    <div class="divider" data-content="artifacts"></div>
    <div class="chip" v-for="item in artifacts" :key="item">
        <a :href="downloadURL(item)">{{ item }}</a>
    </div>
  </section>
</template>

<script>

export default {
    props: {
        artifacts: {
            required: true,
            type: Array,
        },
        buildID: {
            required: true,
            type: Number,
        },
    },
    computed: {
    },
    methods: {
        downloadURL(filename) {
            return `/storage/build/${this.buildID}/artifacts/${filename}`;
        },
    },
};
</script>

<style lang="scss" scoped>
</style>