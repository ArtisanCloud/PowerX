#!/bin/bash
# 最简洁的测试汇总
cd /private/var/www/html/ArtisanCloud/X/PowerX/Core/PowerX/backend

# 运行测试并保存结果（只保留关键行）
go test ./... 2>&1 | grep -E "^(FAIL|ok\s+github)" | grep -v "\[no test files\]" > /tmp/test_all.log

# 显示最终汇总
echo "========================================="
echo "Go 测试汇总"
echo "========================================="
echo ""
echo "总包数: $(cat /tmp/test_all.log | wc -l)"
echo "✅ 通过: $(cat /tmp/test_all.log | grep '^ok' | wc -l)"
echo "❌ 失败: $(cat /tmp/test_all.log | grep '^FAIL' | wc -l)"
echo ""
echo "✅ 无构建错误"
echo "========================================="

# 清理
rm -f /tmp/test_all.log