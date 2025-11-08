#!/bin/bash

cd /private/var/www/html/ArtisanCloud/X/PowerX/Core/PowerX/backend

echo "========================================="
echo "  🔍 构建错误检查报告"
echo "  $(date '+%Y-%m-%d %H:%M:%S')"
echo "========================================="
echo ""

# 运行测试
OUTPUT=$(go test ./... 2>&1)
EXIT_CODE=$?

# 检查构建错误
echo "检查构建错误..."
echo "----------------------------------------"
BUILD_ERRORS=$(echo "$OUTPUT" | grep "\[build failed\]")

if [ -z "$BUILD_ERRORS" ]; then
    echo "  ✅ 无构建错误！"
else
    echo "  ❌ 发现构建错误："
    echo "$BUILD_ERRORS"
fi
echo ""

# 检查具体的编译错误
echo "检查编译错误..."
echo "----------------------------------------"
COMPILE_ERRORS=$(echo "$OUTPUT" | grep -E "^(#|undefined:|unknown field)" | head -10)

if [ -z "$COMPILE_ERRORS" ]; then
    echo "  ✅ 无编译错误！"
else
    echo "  ❌ 发现编译错误："
    echo "$COMPILE_ERRORS"
    echo "  ... (更多错误请查看完整输出)"
fi
echo ""

# 最终结果
echo "========================================="
if [ -z "$BUILD_ERRORS" ] && [ -z "$COMPILE_ERRORS" ]; then
    echo "  ✅ 所有构建错误已修复！"
    echo "     'go test ./...' 可正常执行"
else
    echo "  ⚠️  仍存在构建错误"
    echo "     请检查上述错误信息"
fi
echo "========================================="
echo ""

# 运行我们之前修复的测试
echo "验证之前修复的测试包..."
echo "----------------------------------------"
FIXED_TESTS=(
    "internal/service/event_fabric/authorization"
    "internal/service/media"
    "internal/tests/http/admin/event_fabric"
    "pkg/utils/logger/lib"
    "pkg/auth/jwt_test.go"
    "pkg/dynamic_form/app"
    "tests/contract/integration_gateway"
)

for test in "${FIXED_TESTS[@]}"; do
    result=$(go test ./$test 2>&1 | grep -E "^(PASS|FAIL|ok)" | head -1)
    if [[ $result == *"ok"* ]]; then
        echo "  ✅ $test"
    else
        echo "  ❌ $test"
    fi
done

echo ""
echo "========================================="