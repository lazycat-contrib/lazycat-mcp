<template>
  <div class="panel">
    <div v-if="title" class="panel-head">
      <h3>{{ title }}</h3>
      <div v-if="$slots['head-actions']">
        <slot name="head-actions" />
      </div>
    </div>
    <div class="panel-body">
      <slot />
    </div>
  </div>
</template>

<script setup>
defineProps({ title: { type: String, default: '' } })
</script>
