<template>
  <div class="card">
      <slot/>
  </div>
</template>

<script>
export default {
    name: 'Card',
}
</script>

<style>
.card {
    @apply bg-white shadow-sm rounded;
}
</style>