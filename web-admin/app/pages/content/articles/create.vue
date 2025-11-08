<template>
  <div class="p-6">
    <div class="mb-6">
      <h1 class="text-2xl font-bold text-gray-900 dark:text-white">创建文章</h1>
      <p class="text-gray-600 dark:text-gray-400">创建一篇新的文章内容</p>
    </div>

    <UCard>
      <UForm :schema="schema" :state="state" @submit="onSubmit">
        <div class="space-y-6">
          <!-- 基本信息 -->
          <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
            <div class="lg:col-span-2 space-y-4">
              <UFormField label="文章标题" name="title">
                <UInput v-model="state.title" placeholder="请输入文章标题" />
              </UFormField>

              <UFormField label="文章内容" name="content">
                <UTextarea
                  v-model="state.content"
                  placeholder="请输入文章内容..."
                  :rows="15"
                />
              </UFormField>

              <UFormField label="摘要" name="excerpt">
                <UTextarea
                  v-model="state.excerpt"
                  placeholder="请输入文章摘要..."
                  :rows="3"
                />
              </UFormField>
            </div>

            <!-- 侧边栏设置 -->
            <div class="space-y-4">
              <UCard>
                <template #header>
                  <h3 class="text-lg font-semibold">发布设置</h3>
                </template>

                <div class="space-y-4">
                  <UFormField label="状态" name="status">
                    <USelect v-model="state.status" :items="statusOptions" />
                  </UFormField>

                  <UFormField label="分类" name="category">
                    <USelect
                      v-model="state.category"
                      :items="categoryOptions"
                    />
                  </UFormField>

                  <UFormField label="标签" name="tags">
                    <UInput
                      v-model="state.tags"
                      placeholder="用逗号分隔多个标签"
                    />
                  </UFormField>

                  <UFormField label="发布时间" name="publishAt">
                    <UInput v-model="state.publishAt" type="datetime-local" />
                  </UFormField>
                </div>
              </UCard>

              <UCard>
                <template #header>
                  <h3 class="text-lg font-semibold">SEO 设置</h3>
                </template>

                <div class="space-y-4">
                  <UFormField label="SEO 标题" name="seoTitle">
                    <UInput v-model="state.seoTitle" placeholder="SEO 标题" />
                  </UFormField>

                  <UFormField label="SEO 描述" name="seoDescription">
                    <UTextarea
                      v-model="state.seoDescription"
                      placeholder="SEO 描述"
                      :rows="3"
                    />
                  </UFormField>

                  <UFormField label="关键词" name="keywords">
                    <UInput
                      v-model="state.keywords"
                      placeholder="用逗号分隔关键词"
                    />
                  </UFormField>
                </div>
              </UCard>
            </div>
          </div>

          <!-- 操作按钮 -->
          <div class="flex justify-between">
            <UButton
              :to="localePath('/content/articles')"
              variant="outline"
              color="gray"
            >
              取消
            </UButton>

            <div class="flex gap-2">
              <UButton
                type="submit"
                variant="outline"
                @click="state.status = 'draft'"
              >
                保存草稿
              </UButton>
              <UButton
                type="submit"
                color="primary"
                @click="state.status = 'published'"
              >
                发布文章
              </UButton>
            </div>
          </div>
        </div>
      </UForm>
    </UCard>
  </div>
</template>

<script setup lang="ts">
import { z } from "zod";

// 页面元数据
definePageMeta({
  title: "创建文章",
  layout: "default",
});

// 国际化
const localePath = useLocalePath();

// 表单验证模式
const schema = z.object({
  title: z.string().min(1, "请输入文章标题"),
  content: z.string().min(1, "请输入文章内容"),
  excerpt: z.string().optional(),
  status: z.string(),
  category: z.string().optional(),
  tags: z.string().optional(),
  publishAt: z.string().optional(),
  seoTitle: z.string().optional(),
  seoDescription: z.string().optional(),
  keywords: z.string().optional(),
});

// 表单状态
const state = reactive({
  title: "",
  content: "",
  excerpt: "",
  status: "draft",
  category: "",
  tags: "",
  publishAt: "",
  seoTitle: "",
  seoDescription: "",
  keywords: "",
});

// 选项数据
const statusOptions = [
  { label: "草稿", value: "draft" },
  { label: "已发布", value: "published" },
  { label: "已下线", value: "archived" },
];

const categoryOptions = [
  { label: "技术分享", value: "tech" },
  { label: "产品更新", value: "product" },
  { label: "公司动态", value: "company" },
  { label: "行业资讯", value: "industry" },
];

// 提交处理
async function onSubmit() {
  try {
    // 这里应该调用 API 保存文章
    console.log("保存文章:", state);

    // 显示成功消息
    const toast = useToast();
    toast.add({
      title: "成功",
      description:
        state.status === "published" ? "文章已发布" : "文章已保存为草稿",
      color: "green",
    });

    // 跳转到文章列表
    await navigateTo(localePath("/content/articles"));
  } catch (error) {
    console.error("保存文章失败:", error);

    const toast = useToast();
    toast.add({
      title: "错误",
      description: "保存文章失败，请重试",
      color: "red",
    });
  }
}
</script>
