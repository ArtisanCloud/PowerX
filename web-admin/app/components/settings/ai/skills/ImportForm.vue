<template>
  <UModal
    v-model:open="open"
    title="导入 / 安装 Skill"
    description="支持 upload 导入与 install-tasks 仓库安装。推荐优先使用“仓库安装”。"
    :close="{ onClick: onCancel }"
    :dismissible="false"
    :ui="{ content: 'max-w-3xl w-full' }"
  >
    <template #body>
      <UAlert
        class="mb-3"
        color="info"
        variant="soft"
        icon="i-heroicons-information-circle"
        :title="mode === 'install' ? '仓库安装（推荐）' : 'Upload 导入'"
        :description="mode === 'install'
          ? '填写 provider + repo/repo_url + path，系统将创建 install-task 并自动导入。'
          : '仅用于已上传 bundle 的管理员导入场景（需 bundle_uri/checksum）。'"
      />
      <UTabs v-model="mode" :items="modeOptions" class="mb-3" />

      <div v-if="mode === 'install'" class="grid grid-cols-1 gap-3 md:grid-cols-2">
        <UFormField label="Provider" description="代码仓库平台类型">
          <USelect v-model="installForm.provider" :items="providerOptions" />
        </UFormField>
        <UFormField label="Source" description="导入来源类型">
          <USelect v-model="installForm.source" :items="sourceOptions" />
        </UFormField>
        <UFormField label="Repo（可选）" description="短格式 owner/repo；与 Repo URL 二选一">
          <UInput v-model="installForm.repo" placeholder="例如：openai/skills" />
        </UFormField>
        <UFormField label="Repo URL（可选）" description="完整 Git 地址；与 Repo 二选一">
          <UInput v-model="installForm.repo_url" placeholder="例如：https://gitlab.com/group/repo.git" />
        </UFormField>
        <UFormField class="md:col-span-2" label="Path" description="仓库内具体技能目录，必须包含 SKILL.md" required>
          <UInput v-model="installForm.path" placeholder="例如：skills/.curated/gh-address-comments" />
        </UFormField>
        <UFormField label="Ref（可选）" description="分支或标签，默认 main">
          <UInput v-model="installForm.ref" placeholder="默认 main" />
        </UFormField>
        <UFormField label="Skill ID（可选）" description="不填自动使用 path basename">
          <UInput v-model="installForm.skill_id" placeholder="例如：skill.thirdparty.gh-address-comments" />
        </UFormField>
        <UFormField label="Version（可选）" description="不填默认 1.0.0">
          <UInput v-model="installForm.version" placeholder="默认 1.0.0" />
        </UFormField>
      </div>

      <div v-else class="grid grid-cols-1 gap-3 md:grid-cols-2">
        <UFormField label="Skill ID" required>
          <UInput v-model="uploadForm.skill_id" placeholder="例如：skill.thirdparty.demo" />
        </UFormField>
        <UFormField label="Version" required>
          <UInput v-model="uploadForm.version" placeholder="例如：1.0.0" />
        </UFormField>
        <UFormField label="Source" required>
          <USelect v-model="uploadForm.source" :items="sourceOptions" />
        </UFormField>
        <UFormField label="Bundle URI" description="已上传产物地址（s3/minio/file）" required>
          <UInput v-model="uploadForm.bundle_uri" placeholder="例如：s3://skills/demo-1.0.0.tgz" />
        </UFormField>
        <UFormField label="Checksum" description="建议 sha256:..." required>
          <UInput v-model="uploadForm.checksum" placeholder="例如：sha256:xxxx" />
        </UFormField>
        <UFormField label="Signature（可选）">
          <UInput v-model="uploadForm.signature" placeholder="签名摘要" />
        </UFormField>
        <UFormField label="Source URL（可选）">
          <UInput v-model="uploadForm.source_url" placeholder="GitHub 地址等" />
        </UFormField>
        <UFormField label="Source Ref（可选）">
          <UInput v-model="uploadForm.source_ref" placeholder="commit/tag/branch" />
        </UFormField>
      </div>
      <p v-if="mode === 'install' && taskStatusText" class="mt-2 text-xs text-[var(--text-secondary)]">{{ taskStatusText }}</p>
    </template>
    <template #footer>
      <div class="flex items-center justify-end gap-2">
        <UButton color="neutral" variant="soft" :disabled="loading" @click="onCancel">取消</UButton>
        <UButton :loading="loading" @click="submit">{{ mode === "install" ? "安装并导入" : "导入" }}</UButton>
      </div>
    </template>
  </UModal>
</template>

<script setup lang="ts">
import { useSkillsService } from "~/composables/api/services";
import { useWSBus } from "~/composables/useWSBus";

const open = defineModel<boolean>("open", { default: false });
const emit = defineEmits<{
  imported: [];
}>();

const toast = useToast();
const skillsService = useSkillsService();
const wsBus = useWSBus();
const loading = ref(false);
const taskStatusText = ref("");
const mode = ref<"install" | "upload">("install");

const modeOptions = [
  { label: "仓库安装（推荐）", value: "install" },
  { label: "Upload 导入", value: "upload" },
];

const sourceOptions = [
  { label: "plugin", value: "plugin" },
  { label: "third_party", value: "third_party" },
];

const providerOptions = [
  { label: "github", value: "github" },
  { label: "gitlab", value: "gitlab" },
  { label: "gitee", value: "gitee" },
  { label: "bitbucket", value: "bitbucket" },
  { label: "generic_git", value: "generic_git" },
];

const uploadForm = reactive({
  skill_id: "",
  version: "",
  source: "third_party" as "plugin" | "third_party",
  bundle_uri: "",
  checksum: "",
  signature: "",
  source_url: "",
  source_ref: "",
});

const installForm = reactive({
  provider: "github" as "github" | "gitlab" | "gitee" | "bitbucket" | "generic_git",
  source: "third_party" as "plugin" | "third_party",
  repo: "",
  repo_url: "",
  path: "",
  ref: "",
  skill_id: "",
  version: "",
});

function reset() {
  mode.value = "install";
  uploadForm.skill_id = "";
  uploadForm.version = "";
  uploadForm.source = "third_party";
  uploadForm.bundle_uri = "";
  uploadForm.checksum = "";
  uploadForm.signature = "";
  uploadForm.source_url = "";
  uploadForm.source_ref = "";
  installForm.provider = "github";
  installForm.source = "third_party";
  installForm.repo = "";
  installForm.repo_url = "";
  installForm.path = "";
  installForm.ref = "";
  installForm.skill_id = "";
  installForm.version = "";
}

function onCancel() {
  if (typeof document !== "undefined" && document.activeElement instanceof HTMLElement) {
    document.activeElement.blur();
  }
  reset();
  open.value = false;
}

function waitInstallTaskByWS(taskId: string, timeoutMs = 90_000): Promise<{ status: string; error?: string } | null> {
  return new Promise((resolve, reject) => {
    const started = Date.now();
    let disposed = false;
    wsBus.connect();
    const unsubscribe = wsBus.subscribe("_topic.system.notification", (payload: any) => {
      if (disposed || !payload) return;
      if (String(payload?.kind || "").trim() !== "skills.install.task") return;
      if (String(payload?.task_id || "").trim() !== taskId) return;
      const status = String(payload?.status || "").trim().toLowerCase();
      if (!status) return;
      taskStatusText.value = `任务 ${taskId} 状态：${status}`;
      if (status === "success") {
        disposed = true;
        unsubscribe();
        resolve({ status: "success" });
      } else if (status === "failed") {
        disposed = true;
        unsubscribe();
        const msg = String(payload?.error_summary || payload?.content || "安装任务失败").trim();
        reject(new Error(msg));
      }
    });
    const timer = setInterval(() => {
      if (disposed) {
        clearInterval(timer);
        return;
      }
      if (Date.now() - started > timeoutMs) {
        disposed = true;
        clearInterval(timer);
        unsubscribe();
        resolve(null);
      }
    }, 500);
  });
}

async function submit() {
  loading.value = true;
  taskStatusText.value = "";
  try {
    if (mode.value === "upload") {
      await skillsService.importSkill({
        skill_id: uploadForm.skill_id.trim(),
        version: uploadForm.version.trim(),
        source: uploadForm.source,
        bundle_uri: uploadForm.bundle_uri.trim(),
        checksum: uploadForm.checksum.trim(),
        signature: uploadForm.signature.trim() || undefined,
        source_url: uploadForm.source_url.trim() || undefined,
        source_ref: uploadForm.source_ref.trim() || undefined,
      });
      toast.add({ title: "导入成功", color: "success" });
      emit("imported");
      onCancel();
      return;
    }

    const repo = installForm.repo.trim();
    const repoURL = installForm.repo_url.trim();
    const path = installForm.path.trim();
    if (!path) {
      throw new Error("path 必填");
    }
    if (!repo && !repoURL) {
      throw new Error("repo 和 repo_url 至少填写一个");
    }

    const task = await skillsService.createInstallTask({
      provider: installForm.provider,
      source: installForm.source,
      repo: repo || undefined,
      repo_url: repoURL || undefined,
      path,
      ref: installForm.ref.trim() || undefined,
      skill_id: installForm.skill_id.trim() || undefined,
      version: installForm.version.trim() || undefined,
      auto_import: true,
    });
    toast.add({ title: "安装任务已创建", description: `task_id: ${task.task_id}`, color: "success" });
    taskStatusText.value = `任务 ${task.task_id} 已创建，等待实时事件...`;
    const done = await waitInstallTaskByWS(task.task_id);
    if (done?.status === "success") {
      toast.add({ title: "安装并导入成功", color: "success" });
      emit("imported");
      onCancel();
      return;
    }
    toast.add({
      title: "安装任务仍在进行中（未收到实时事件）",
      description: `请在列表查询 task_id: ${task.task_id}`,
      color: "warning",
    });
    emit("imported");
  } catch (error: any) {
    toast.add({
      title: mode.value === "install" ? "安装失败" : "导入失败",
      description: error?.message || "请求失败",
      color: "error",
    });
  } finally {
    loading.value = false;
  }
}
</script>
