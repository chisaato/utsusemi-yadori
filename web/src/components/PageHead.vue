<script setup lang="ts">
defineProps<{
  title: string
  desc?: string
}>()
</script>

<template>
  <header class="page-head reveal" style="--i: 0">
    <div class="page-head-text">
      <h1 class="page-title">{{ title }}</h1>
      <p v-if="desc" class="page-desc">{{ desc }}</p>
    </div>
    <div class="page-head-actions">
      <slot name="actions" />
    </div>
  </header>
</template>
