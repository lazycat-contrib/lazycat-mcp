<template>
  <div class="view-shell">
    <h2>{{ title }}</h2>
    <div v-if="$slots.actions" class="view-shell-actions">
      <slot name="actions" />
    </div>
  </div>
</template>

<script setup>
defineProps({ title: { type: String, required: true } })
</script>
