<!-- app/components/layout/SidebarMenuItem.vue -->
<script setup lang="ts">
import type { MenuItem } from '~/composables/api/services/menuService';
import SidebarMenuItem from '~/components/layout/SidebarMenuItem.vue';

defineProps<{
  item: MenuItem;
  collapsed: boolean;
  densityClass: string;
  expandedItems: Set<string>;
  isActive: (path?: string) => boolean;
  linkFor: (path?: string) => string;
  resolveIcon: (name?: string) => string;
  toggleExpanded: (id: string) => void;
  hasActiveChild: (children?: MenuItem[]) => boolean;
}>();

const renderPluginVersion = () => false;
</script>

<template>
  <li>
    <!-- 0) 分割线 -->
    <div
      v-if="item.path === '---'"
      class="my-2 -mx-3 h-px bg-gray-200/70 dark:bg-gray-700/70"
      role="separator"
    ></div>

    <!-- 1) 有子菜单 -->
    <div
      v-else-if="item.children && item.children.length > 0"
      class="menu-item group relative w-full"
      role="treeitem"
      :aria-expanded="expandedItems.has(item.id)"
      :aria-controls="`submenu-${item.id}`"
    >
      <button
        @click="toggleExpanded(item.id)"
        :class="[
          'w-full flex items-center rounded-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/40',
          collapsed ? 'justify-center px-2' : 'justify-between px-3',
          densityClass,
          hasActiveChild(item.children)
            ? 'text-blue-700 dark:text-blue-100 bg-blue-500/10 ring-1 ring-blue-500/10'
            : 'text-slate-700 dark:text-slate-200 hover:bg-slate-900/5 dark:hover:bg-white/5',
        ]"
      >
        <div v-if="collapsed" class="flex items-center justify-center">
          <span class="inline-block w-5 h-5">
            <UIcon class="w-5 h-5" :name="resolveIcon(item.icon)" />
          </span>
        </div>
        <div v-else class="flex items-center justify-between w-full">
          <div class="flex items-center gap-3">
            <span class="inline-block w-5 h-5 flex-shrink-0">
              <UIcon class="w-5 h-5" :name="resolveIcon(item.icon)" />
            </span>
            <div class="min-w-0 flex items-center gap-2">
              <span class="truncate">{{ item.title }}</span>
              <UBadge
                v-if="renderPluginVersion(item)"
                size="xs"
                color="neutral"
                variant="soft"
                class="shrink-0 uppercase"
              >
                v{{ item.pluginVersion }}
              </UBadge>
            </div>
          </div>
          <div class="flex items-center gap-2 flex-shrink-0">
            <UBadge v-if="item.badge" size="xs" color="primary">{{
              item.badge
            }}</UBadge>
            <UIcon
              name="i-heroicons-chevron-right"
              class="w-4 h-4 transition-transform"
              :class="{ 'rotate-90': expandedItems.has(item.id) }"
            />
          </div>
        </div>
      </button>

      <Transition
        enter-active-class="transition-[max-height,opacity] duration-200 ease-out"
        enter-from-class="opacity-0 max-h-0"
        enter-to-class="opacity-100 max-h-96"
        leave-active-class="transition-[max-height,opacity] duration-150 ease-in"
        leave-from-class="opacity-100 max-h-96"
        leave-to-class="opacity-0 max-h-0"
      >
        <ul
          v-show="expandedItems.has(item.id) && !collapsed"
          :id="`submenu-${item.id}`"
          class="mt-1 ml-6 space-y-1 overflow-hidden"
          role="group"
        >
          <SidebarMenuItem
            v-for="child in item.children"
            :key="child.id"
            :item="child"
            :collapsed="collapsed"
            :densityClass="densityClass"
            :expandedItems="expandedItems"
            :isActive="isActive"
            :linkFor="linkFor"
            :resolveIcon="resolveIcon"
            :toggleExpanded="toggleExpanded"
            :hasActiveChild="hasActiveChild"
          />
        </ul>
      </Transition>
    </div>

    <!-- 2) 无子菜单（有 path） -->
    <NuxtLink
      v-else-if="item.path"
      :to="linkFor(item.path)"
      :class="[
        'menu-item group relative flex items-center rounded-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500/40',
        collapsed ? 'justify-center px-2' : 'justify-between px-3',
        densityClass,
        isActive(item.path)
          ? 'text-blue-700 dark:text-blue-100 bg-blue-500/10 ring-1 ring-blue-500/20'
          : 'text-slate-700 dark:text-slate-200 hover:bg-slate-900/5 dark:hover:bg-white/5',
      ]"
      :aria-current="isActive(item.path) ? 'page' : undefined"
      role="treeitem"
    >
      <span
        v-if="isActive(item.path)"
        class="absolute left-0 top-1/2 -translate-y-1/2 w-1 h-5 rounded-r bg-blue-500 dark:bg-blue-400"
        aria-hidden="true"
      />
      <div v-if="collapsed" class="flex items-center justify-center">
        <span class="inline-block w-5 h-5">
          <UIcon class="w-5 h-5" :name="resolveIcon(item.icon)" />
        </span>
      </div>
      <div v-else class="flex items-center justify-between w-full">
        <div class="flex items-center gap-3">
          <span class="inline-block w-5 h-5 flex-shrink-0">
            <UIcon class="w-5 h-5" :name="resolveIcon(item.icon)" />
          </span>
          <div class="min-w-0 flex items-center gap-2">
            <span class="truncate">{{ item.title }}</span>
            <UBadge
              v-if="renderPluginVersion(item)"
              size="xs"
              color="neutral"
              variant="soft"
              class="shrink-0 uppercase"
            >
              v{{ item.pluginVersion }}
            </UBadge>
          </div>
        </div>
        <UBadge v-if="item.badge" size="xs" color="primary">{{
          item.badge
        }}</UBadge>
      </div>
    </NuxtLink>

    <!-- 3) 占位（无 path） -->
    <div
      v-else
      :class="[
        'flex items-center text-slate-700 dark:text-slate-200 rounded-md',
        collapsed ? 'justify-center px-2' : 'gap-3 px-3',
        densityClass,
      ]"
      role="treeitem"
    >
      <span class="inline-block w-5 h-5 flex-shrink-0">
        <UIcon class="w-5 h-5" :name="resolveIcon(item.icon)" />
      </span>
      <span v-if="!collapsed" class="truncate">
        {{ item.title }}
      </span>
    </div>
  </li>
</template>
