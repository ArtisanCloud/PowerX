<script setup lang="ts">
import RoleManager from "~/components/settings/users/RoleManager.vue";

definePageMeta({
  title: "角色管理",
  icon: "i-heroicons-shield-check",
  order: 9,
});
</script>

<template>
  <div class="p-6">
    <RoleManager />
  </div>
</template>
