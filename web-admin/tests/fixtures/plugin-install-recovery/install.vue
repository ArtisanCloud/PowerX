<template>
  <main class="space-y-4 p-4">
    <h1>安装恢复受控验收</h1>
    <div class="flex flex-wrap gap-3">
      <UButton @click="select('com.powerx.plugins.fixture', 'A')">安装 A</UButton>
      <UButton @click="select('com.powerx.plugins.other', 'B')">安装 B</UButton>
    </div>
    <InstallDialog v-model="open" :plugin="selected" />
  </main>
</template>

<script setup lang="ts">
import InstallDialog from '~/components/plugins/InstallDialog.vue';
import type { MarketplacePlugin } from '~/components/plugins/PluginCard.vue';
definePageMeta({ layout: 'default' });
const open = ref(false);
const selected = ref<MarketplacePlugin>();
function select(id: string, name: string) {
  selected.value = { id, name, version: '1.0.0', description: '', author: 'fixture', category: '', installs: 0, tags: [] };
  open.value = true;
}
</script>
