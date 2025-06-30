#!/bin/bash

# 调试翻译解析错误的脚本
# 使用 verbose 模式来获取详细的日志输出

echo "使用 verbose 模式运行翻译以调试 parse_error 问题..."
echo "这将输出详细的日志信息，包括："
echo "- 发送到翻译服务的确切文本（带节点标记）"
echo "- 从翻译服务接收的原始响应"
echo "- 正则表达式匹配的详细信息"
echo ""

# 设置 verbose 标志
./translator translate \
    --verbose \
    --source-lang en \
    --target-lang zh \
    --model deepseek-chat \
    --concurrency 2 \
    "$@"

echo ""
echo "调试提示："
echo "1. 查找 'sending batch translation request' 以查看发送的请求"
echo "2. 查找 'received translation response' 以查看响应"
echo "3. 查找 'response format check' 以了解响应格式"
echo "4. 查找 'missing node translations' 以查看哪些节点未找到翻译"
echo "5. 查找 'found alternative node marker' 以查看是否使用了不同的标记格式"