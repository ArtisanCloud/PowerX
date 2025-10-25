#!/bin/bash

echo "=== 重构自检脚本 ==="
echo

echo "1. 检查是否还有错误的 ChatMessage 导入："
grep -r "from \"~/types/agent\".*ChatMessage" app/ || echo "✅ 无错误导入"
echo

echo "2. 检查是否还有使用 useAgentChat 的地方："
grep -r "useAgentChat" app/ || echo "✅ 已清理完毕"
echo

echo "3. 检查是否还有从 ~/types/chat 导入的地方："
grep -r "from \"~/types/chat\"" app/ || echo "✅ 已统一到 ~/types/message"
echo

echo "4. 验证关键文件是否存在："
echo -n "- useDualChannelConnection.ts: "
[ -f "app/composables/agent/useDualChannelConnection.ts" ] && echo "✅ 存在" || echo "❌ 缺失"

echo -n "- useAgentManager.ts: "
[ -f "app/composables/agent/useAgentManager.ts" ] && echo "✅ 存在" || echo "❌ 缺失"

echo -n "- ~/types/message.ts: "
[ -f "app/types/message.ts" ] && echo "✅ 存在" || echo "❌ 缺失"

echo -n "- ChatInterface.vue: "
[ -f "app/components/agent/ChatInterface.vue" ] && echo "✅ 存在" || echo "❌ 缺失"

echo

echo "5. 验证已删除的重复文件："
echo -n "- useAgentChat.ts: "
[ ! -f "app/composables/agent/useAgentChat.ts" ] && echo "✅ 已删除" || echo "❌ 仍存在"

echo -n "- AgentChat.vue: "
[ ! -f "app/components/agent/AgentChat.vue" ] && echo "✅ 已删除" || echo "❌ 仍存在"

echo
echo "=== 重构完成检查 ==="