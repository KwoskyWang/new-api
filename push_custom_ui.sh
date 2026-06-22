#!/bin/bash

# 1. 检查是否传入了 commit message 参数
if [ -z "$1" ]; then
  echo "❌ 错误：缺少提交信息！"
  echo "用法: ./push_custom_ui.sh \"你的提交信息\""
  exit 1
fi

# 将命令行传入的第一个参数作为 commit message
COMMIT_MSG="$1"

echo "🚀 开始处理 custom-ui 分支..."

# 2. 添加所有改动
git add -A

# 3. 提交到本地仓库
git commit -m "$COMMIT_MSG"

# 4. 推送到远程并建立关联
git push -u origin custom-ui

echo "✅ 成功！已将改动推送到远程 custom-ui 分支。"
