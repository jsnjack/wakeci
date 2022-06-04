<template></template>

<script>
export default {
    props: {
        status: {
            type: String,
            required: true,
            validator: (val) => ['running', 'finished', 'failed', 'aborted'].includes(val),
        },
    },
};
</script>

<style></style>
