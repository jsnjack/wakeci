<template>
    <div class="card">
        <slot />
    </div>
</template>

<script>
export default {
    name: 'Card',
};
</script>

<style>
.card {
    @apply bg-white shadow-sm rounded p-2 relative dark:bg-gray-extra-light;
}
</style>
