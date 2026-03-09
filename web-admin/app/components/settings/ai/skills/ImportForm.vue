<template>
  <UCard>
    <template #header>
      <div class="flex items-center justify-between">
        <span class="font-medium text-[var(--text-primary)]">导入 Skill（仅 upload）</span>
        <UBadge color="neutral" variant="soft">US2</UBadge>
      </div>
    </template>

    <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
      <UInput v-model="form.skill_id" label="Skill ID" placeholder="例如：skill.thirdparty.demo" />
      <UInput v-model="form.version" label="Version" placeholder="例如：1.0.0" />
      <USelect v-model="form.source" :items="sourceOptions" label="Source" />
      <UInput v-model="form.bundle_uri" label="Bundle URI" placeholder="例如：s3://skills/demo-1.0.0.tgz" />
      <UInput v-model="form.checksum" label="Checksum" placeholder="例如：sha256:xxxx" />
      <UInput v-model="form.signature" label="Signature（可选）" placeholder="签名摘要" />
      <UInput v-model="form.source_url" label="Source URL（可选）" placeholder="GitHub 地址等" />
      <UInput v-model="form.source_ref" label="Source Ref（可选）" placeholder="commit/tag/branch" />
    </div>

    <div class="mt-4 flex items-center gap-2">
      <UButton :loading="loading" @click="submit">导入</UButton>
      <UButton variant="ghost" :disabled="loading" @click="reset">重置</UButton>
    </div>
  </UCard>
</template>

<script setup lang="ts">
import { useSkillsService } from "~/composables/api/services";

const emit = defineEmits<{
  imported: [];
}>();

const toast = useToast();
const skillsService = useSkillsService();
const loading = ref(false);

const sourceOptions = [
  { label: "plugin", value: "plugin" },
  { label: "third_party", value: "third_party" },
];

const form = reactive({
  skill_id: "",
  version: "",
  source: "third_party" as "plugin" | "third_party",
  bundle_uri: "",
  checksum: "",
  signature: "",
  source_url: "",
  source_ref: "",
});

function reset() {
  form.skill_id = "";
  form.version = "";
  form.source = "third_party";
  form.bundle_uri = "";
  form.checksum = "";
  form.signature = "";
  form.source_url = "";
  form.source_ref = "";
}

async function submit() {
  loading.value = true;
  try {
    await skillsService.importSkill({
      skill_id: form.skill_id.trim(),
      version: form.version.trim(),
      source: form.source,
      bundle_uri: form.bundle_uri.trim(),
      checksum: form.checksum.trim(),
      signature: form.signature.trim() || undefined,
      source_url: form.source_url.trim() || undefined,
      source_ref: form.source_ref.trim() || undefined,
    });
    toast.add({ title: "导入成功", color: "success" });
    emit("imported");
    reset();
  } catch (error: any) {
    toast.add({
      title: "导入失败",
      description: error?.message || "请求失败",
      color: "error",
    });
  } finally {
    loading.value = false;
  }
}
</script>
