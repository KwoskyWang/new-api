# 1. 确保你的分支是最新的（可选但推荐）
git checkout custom-ui
git pull origin main

# 2. 切换到 master 分支
git checkout main

# 3. 更新本地 master（避免冲突）
git pull origin main

# 4. 合并你的分支到 master
git merge custom-ui

# 5. 解决可能的冲突（如有），然后提交冲突解决（如已解决）
# git add .
# git commit -m "Merge branch 'custom-ui' into main"

# 6. 推送合并后的 master 到远程
git push origin main

git checkout custom-ui
