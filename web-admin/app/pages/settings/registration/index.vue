<script setup lang="ts">
import RegistrationPolicyPanel from "~/components/settings/RegistrationPolicyPanel.vue";

definePageMeta({
  layout: "default",
});
</script>

<template>
  <div class="mx-auto max-w-6xl px-4 py-6">
    <ClientOnly>
      <RegistrationPolicyPanel />
    </ClientOnly>
  </div>
</template>
