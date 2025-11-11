#!/bin/bash

# 清理测试输出
echo "========================================="
echo "Go 测试汇总报告"
echo "========================================="
echo ""

# 运行测试并捕获输出
OUTPUT=$(go test ./... 2>&1)
EXIT_CODE=$?

# 提取失败的测试包
echo "❌ 失败的测试包："
echo "----------------------------------------"
FAIL_PACKAGES=$(echo "$OUTPUT" | grep -E "^FAIL\s+" | grep -v "FAIL\s+\.\.\." | awk '{print $2}' | sort)
if [ -z "$FAIL_PACKAGES" ]; then
    echo "  ✅ 无失败的测试包"
else
    echo "$FAIL_PACKAGES" | while read pkg; do
        echo "  • $pkg"
    done
fi
echo ""

# 统计测试包
TOTAL_PACKAGES=$(echo "$OUTPUT" | grep -E "^(FAIL|PASS|\?)\s+" | wc -l)
PASS_PACKAGES=$(echo "$OUTPUT" | grep -E "^\?\s+" | wc -l)
NO_TEST_PACKAGES=$(echo "$OUTPUT" | grep "\[no test files\]" | wc -l)

echo "========================================="
echo "📊 测试统计"
echo "========================================="
echo "  总包数：$TOTAL_PACKAGES"
echo "  通过包数：$(($TOTAL_PACKAGES - $(echo "$FAIL_PACKAGES" | wc -l) - $PASS_PACKAGES))"
echo "  跳过包数：$PASS_PACKAGES"
echo "  无测试包数：$NO_TEST_PACKAGES"
echo ""

# 提取主要错误
echo "========================================="
echo "⚠️  主要错误（显示前5个）"
echo "========================================="
echo "$OUTPUT" | grep -E "^(#|FAIL\t|--- FAIL)" | head -20
echo ""

# 最终状态
if [ $EXIT_CODE -eq 0 ]; then
    echo "✅ 所有测试通过！"
else
    echo "❌ 存在测试失败"
fi
echo "========================================="