#!/bin/bash
# 简化版 go test 输出脚本

cd /private/var/www/html/ArtisanCloud/X/PowerX/Core/PowerX/backend

echo "========================================="
echo "  Go 测试执行报告"
echo "========================================="
echo ""

# 运行测试，只显示关键信息
go test ./... 2>&1 | \
  grep -v "\[no test files\]" | \
  grep -v "intent-debug" | \
  grep -v "strategies=\[" | \
  grep -v "clause=" | \
  grep -v "-> rule" | \
  grep -v "keep flow=" | \
  grep -v "FINAL keep:" | \
  grep -E "^(FAIL|ok\s+github)" > /tmp/test_output.log

# 如果失败了，再显示详细错误
FAILED_PACKAGES=$(grep "^FAIL" /tmp/test_output.log | wc -l)
if [ $FAILED_PACKAGES -gt 0 ]; then
    echo "❌ 失败的测试包："
    grep "^FAIL" /tmp/test_output.log | awk '{print "  • " $2}'
    echo ""
fi

# 统计
TOTAL=$(cat /tmp/test_output.log | wc -l)
PASS=$(grep "^ok" /tmp/test_output.log | wc -l)
FAIL=$(grep "^FAIL" /tmp/test_output.log | wc -l)

echo "========================================="
echo "  📊 统计"
echo "========================================="
echo "  总包数：$TOTAL"
echo "  ✅ 通过：$PASS"
echo "  ❌ 失败：$FAIL"
echo ""

# 检查构建错误
if grep -q "\[build failed\]" /tmp/test_output.log; then
    echo "⚠️  构建错误存在！"
else
    echo "✅ 无构建错误"
fi

rm -f /tmp/test_output.log

echo ""
echo "========================================="