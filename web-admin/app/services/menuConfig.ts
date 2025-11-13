/**
 * 菜单配置脚本
 * 用于添加插件发布相关的菜单项
 *
 * 使用方法：
 * 1. 确保后端API可访问
 * 2. 运行此脚本创建菜单
 * 3. 或手动调用后端API创建菜单
 */

export interface MenuConfig {
  parentMenu?: {
    title: string
    path: string
    icon: string
    order: number
  }
  items: Array<{
    title: string
    path: string
    icon: string
    order: number
  }>
}

/**
 * 插件发布菜单配置
 */
export const pluginReleaseMenuConfig: MenuConfig = {
  parentMenu: {
    title: '插件发布',
    path: '/admin/plugin-release',
    icon: 'i-heroicons-cube-transparent',
    order: 50
  },
  items: [
    {
      title: '离线包入库',
      path: '/admin/plugin-release/offline-packages',
      icon: 'i-heroicons-cloud-arrow-up',
      order: 1
    },
    {
      title: 'Marketplace审核',
      path: '/admin/plugin-release/marketplace',
      icon: 'i-heroicons-check-badge',
      order: 2
    }
  ]
}

/**
 * 知识空间菜单配置
 */
export const knowledgeSpaceMenuConfig: MenuConfig = {
  parentMenu: {
    title: '知识空间',
    path: '/knowledge-spaces',
    icon: 'i-heroicons-globe-alt',
    order: 40
  },
  items: [
    {
      title: '空间列表',
      path: '/knowledge-spaces',
      icon: 'i-heroicons-rectangle-stack',
      order: 1
    },
    {
      title: '创建空间',
      path: '/knowledge-spaces/create',
      icon: 'i-heroicons-plus-circle',
      order: 2
    }
  ]
}

/**
 * 通过后端API创建菜单
 */
export async function createMenusViaAPI() {
  const apiBase = '/api/admin'

  try {
    // 创建父菜单
    const parentMenu = await createMenu({
      title: pluginReleaseMenuConfig.parentMenu!.title,
      icon: pluginReleaseMenuConfig.parentMenu!.icon,
      path: pluginReleaseMenuConfig.parentMenu!.path,
      order: pluginReleaseMenuConfig.parentMenu!.order,
      visible: true,
      permissions: ['admin:plugin:release:view'],
      badge: undefined
    })

    console.log('父菜单创建成功:', parentMenu)

    // 创建子菜单
    for (const item of pluginReleaseMenuConfig.items) {
      const childMenu = await createMenu({
        title: item.title,
        icon: item.icon,
        path: item.path,
        parentId: parentMenu.data!.id,
        order: item.order,
        visible: true,
        permissions: ['admin:plugin:release:view'],
        badge: undefined
      })
      console.log(`子菜单创建成功: ${item.title}`, childMenu)
    }

    return { success: true }
  } catch (error) {
    console.error('菜单创建失败:', error)
    return { success: false, error }
  }
}

async function createMenu(params: any) {
  // TODO: 实际调用时需要替换为真实API
  // return await $fetch('/api/admin/menus', {
  //   method: 'POST',
  //   headers: { Authorization: `Bearer ${useCookie('token').value}` },
  //   body: params
  // })

  // 模拟API调用
  await new Promise(resolve => setTimeout(resolve, 500))
  return {
    code: 200,
    message: 'success',
    data: { id: Math.floor(Math.random() * 10000), ...params }
  }
}

/**
 * 手动创建菜单的说明
 */
export const menuCreationInstructions = `
手动创建菜单的步骤：

1. 登录 Web Admin 控制台（root 或 system_admin 权限）

2. 打开浏览器开发者工具，控制台执行：

```javascript
// 创建父菜单
fetch('/api/admin/menus', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    'Authorization': 'Bearer ' + useCookie('token').value
  },
  body: JSON.stringify({
    title: '插件发布',
    icon: 'i-heroicons-cube-transparent',
    path: '/admin/plugin-release',
    order: 50,
    visible: true,
    permissions: ['admin:plugin:release:view']
  })
})
```

3. 复制返回的父菜单ID，然后创建子菜单：

```javascript
// 创建"离线包入库"子菜单
fetch('/api/admin/menus', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    'Authorization': 'Bearer ' + useCookie('token').value
  },
  body: JSON.stringify({
    title: '离线包入库',
    icon: 'i-heroicons-cloud-arrow-up',
    path: '/admin/plugin-release/offline-packages',
    parentId: '复制上一步的ID',
    order: 1,
    visible: true,
    permissions: ['admin:plugin:release:view']
  })
})

// 创建"Marketplace审核"子菜单
fetch('/api/admin/menus', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    'Authorization': 'Bearer ' + useCookie('token').value
  },
  body: JSON.stringify({
    title: 'Marketplace审核',
    icon: 'i-heroicons-check-badge',
    path: '/admin/plugin-release/marketplace',
    parentId: '复制父菜单ID',
    order: 2,
    visible: true,
    permissions: ['admin:plugin:release:view']
  })
})
```

4. 如需新增“知识空间”导航，可复用上述脚本，将 \`title/path/icon\` 调整为 \`knowledgeSpaceMenuConfig\` 中的值；刷新页面即可看到“知识空间 → 空间列表 / 创建空间”入口。
`
