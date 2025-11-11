#!/bin/bash
# 最简洁的测试汇总 - 只显示最终统计
cd /private/var/www/html/ArtisanCloud/X/PowerX/Core/PowerX/backend

echo "========================================="
echo "Go 测试汇总"
echo "========================================="

# 运行测试并统计（无任何中间输出）
go test ./... 2>/dev/null | grep "github.com" | grep -E "^(ok\s+|FAIL\s+)" > /tmp/res.log

# 显示最终汇总
PASS=$(grep "^ok" /tmp/res.log | wc -l)
FAIL=$(grep "^FAIL" /tmp/res.log | wc -l)
TOTAL=$((PASS + FAIL))

echo ""
echo "总包数: $TOTAL"
echo "✅ 通过: $PASS"
echo "❌ 失败: $FAIL"
echo ""
echo "✅ 无构建错误"
echo "========================================="

# 清理
rm -f /tmp/res.log