#!/bin/bash

cd /private/var/www/html/ArtisanCloud/X/PowerX/Core/PowerX/backend

echo "========================================="
echo "  Go 测试执行报告"
echo "========================================="
echo ""

# 运行测试，过滤掉 [no test files] 和 intent-debug 日志
go test ./... 2>&1 | grep -v "\[no test files\]" | grep -v "\[intent-debug\]" | grep -v "strategies=" | grep -v "clause=" | grep -v "->" | grep -v "FINAL" | grep -v "keep flow=" | tee /tmp/test_raw.log

# 如果上面的管道失败，直接输出
if [ $? -ne 0 ]; then
    echo "运行测试..."
    go test ./... 2>&1 | tee /tmp/test_raw.log
fi

echo ""
echo "========================================="
echo "  汇总统计"
echo "========================================="

# 统计
TOTAL=$(grep -E "^(ok|FAIL)\s+github" /tmp/test_raw.log | wc -l)
PASS=$(grep "^ok\s+github" /tmp/test_raw.log | wc -l)
FAIL=$(grep "^FAIL\s+github" /tmp/test_raw.log | wc -l)

echo "  总包数：$TOTAL"
echo "  ✅ 通过：$PASS"
echo "  ❌ 失败：$FAIL"
echo ""

# 只显示失败的包
if [ $FAIL -gt 0 ]; then
    echo "失败的包："
    grep "^FAIL\s+github" /tmp/test_raw.log | awk '{print "  • " $2}'
    echo ""
fi

# 检查构建错误
BUILD_ERR=$(grep "\[build failed\]" /tmp/test_raw.log | wc -l)
if [ $BUILD_ERR -gt 0 ]; then
    echo "⚠️  构建错误："
    grep "\[build failed\]" /tmp/test_raw.log | awk '{print "  • " $0}'
else
    echo "✅ 无构建错误"
fi

echo ""
echo "========================================="

# 清理
rm -f /tmp/test_raw.log