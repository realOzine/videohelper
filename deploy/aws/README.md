# AWS 单用户 Demo 操作手册

这是最低服务数量的测试部署：一台可停止的 CPU EC2 运行所有容器，私有 S3 保存媒体与备份。不是高可用或公开多用户环境。真实模型的内存、速度和检索质量须在目标主机验证；构建成功或 `/healthz` 成功不能代替这项验收。

## 资源与费用

| 操作 / 资源 | 为什么需要 | 计费与停止后的状态 |
|---|---|---|
| 创建 CodeConnections | 让 AWS GitHub App 只读取授权的私有仓库 | 无常驻计算；保留连接方便后续发布 |
| 创建 CloudFormation 栈 | 版本化管理网络、存储、角色、主机和构建配置 | 普通 AWS 资源类型的栈管理不另收资源运行费；实际资源各自计费 |
| VPC、子网、路由、IGW、安全组 | 主机可主动联网；不开放入站端口 | 无 NAT Gateway / ALB 固定费用；动态公网 IPv4 只在实例运行时使用 |
| EC2 `r6a.large`，2 vCPU / 16 GiB | 先验证 WeMM CPU + ASR/CLAP，避免 GPU 配额和费用 | Ohio 查询报价 $0.1134/小时，停止后无实例计算费；16 GiB 容量尚需实测 |
| 60 GiB gp3 EBS | 保存数据库、Redis、鉴权私钥和模型缓存 | 按配置容量持续计费；按 $0.08/GiB/月估算约 $4.80/月，停止也收费 |
| 两个私有 S3 桶 | 媒体；版本包和数据库备份 | 按容量、请求和适用流量收费；停止 EC2 不影响 S3 |
| ECR | 保存四类镜像，实例按 SHA 拉取 | 按镜像存储及适用流量收费；每类保留最近三个推送版本 |
| CodeBuild small | 按 push 临时构建，无常驻构建机 | 按构建分钟计费；当前定价有每月 100 分钟额度，超额约 $0.005/分钟；失败构建也可能计费 |
| IAM / Systems Manager | 服务临时身份、远程命令和加密端口转发 | 不保存 AWS 长期密钥；本方案不启用付费高级实例管理 |
| CloudWatch Logs | 查看构建失败原因 | 日志写入 / 存储按量计费，保留七天 |
| 可选 Budget | 提供邮箱后发送 20 / 40 USD 提醒 | 提醒有延迟，不是硬上限，不能代替停机 |

以上为 2026-10-27 的未抵扣估算，不是账单保证。每月运行 20 小时的 CPU 计算约 $2.27，加 EBS 约 $4.80，再加 IPv4、S3、ECR、构建和流量。CPU 若需长时间运行，小时单价低不代表完成任务总成本一定低。Free 计划抵扣额度和实例资格以项目实际状态为准，不自动升级付费计划。项目当前区域是 `us-east-2`；新 AWS 体验限制在项目区域创建资源。

价格来源：[EC2](https://aws.amazon.com/ec2/pricing/on-demand/)、[EBS](https://aws.amazon.com/ebs/pricing/)、[CodeBuild](https://aws.amazon.com/codebuild/pricing/)、[公网 IPv4](https://aws.amazon.com/vpc/pricing/)。

## 第一次创建

1. 在 AWS CLI 登录 `videohelper-demo` profile，并用 `aws sts get-caller-identity` 确认项目。目标 `r6a.large` 不允许在 FREE 计划启动，须先由项目所有者确认升级 Paid；升级涉及其所有项目且不可撤销。CLI 凭据保存在用户的 AWS 凭据管理中，不提交 Git。
2. 创建 CodeConnections GitHub 连接，在 AWS 控制台完成 GitHub App 授权，仅选择 `realOzine/videohelper`，验证状态为 `AVAILABLE`。
3. 审查代码和历史的凭据风险，推送私有 GitHub `main`。AWS Toolkit 的 25 个技能仅在仓库 `.agents/skills/`；不是用户全局技能。
4. 校验 `stack.yaml`：cfn-lint、`stack.guard`、CloudFormation change set。检查 AWS 服务返回的验证事件，再执行变更集。必须传入已验证的 AL2023 x86_64 AMI 和连接 ARN。创建 IAM 服务角色需要 `CAPABILITY_IAM`；这里不创建人的 IAM 用户或长期密钥。
5. EC2 首次启动先安装两小时自动停机 timer，再准备 Docker / SSM。确认 `/opt/videohelper/bootstrap-ready` 和 timer 正常；首次镜像构建期间先停止空闲 EC2。
6. 首次手动触发 `videohelper-demo-build`。后续 `main` 的代码 push 自动触发；仅文档 / 技能变化跳过。构建运行检查，依次推送 Go、Web、WeMM CPU、ASR/CLAP CPU 镜像，最后发布 S3 `latest.json`。
7. 等待首次构建 `buildStatus=SUCCEEDED`（期望版本已发布），再显式启动并部署，准备固定修订权重；用真实短视频检查上传、索引、搜索、播放，再检查停机 / 开机数据保留。

```bash
export AWS_PROFILE=videohelper-demo
export AWS_REGION=us-east-2
aws login --profile videohelper-demo
aws sts get-caller-identity

# 只在尚未构建期望版本时触发；普通 main push 已会自动触发。
aws codebuild start-build --project-name videohelper-demo-build
```

`CodeConnections` + `CodeBuild` 替代 GitHub OIDC，是因为当前项目的 SCP 禁止 IAM Provider 相关操作。GitHub Actions 文件仅执行检查；真正部署在 AWS 内使用服务角色。[官方 GitHub App 配置](https://docs.aws.amazon.com/codebuild/latest/userguide/connections-github-app.html)。

## 每次测试：启动、访问、停止

在仓库根目录运行，需要 AWS CLI、Python 3；连接会话还需要 AWS Session Manager plugin。

```bash
export AWS_PROFILE=videohelper-demo
export AWS_REGION=us-east-2
python3 deploy/aws/demo.py status
python3 deploy/aws/demo.py start
python3 deploy/aws/demo.py connect
```

`start` 从 CloudFormation 获取专用实例 ID，读取已完整发布的 SHA，开机、等待 SSM、部署；不会创建第二台服务器。`connect` 保持前台运行，在浏览器打开 `http://localhost:8080`。公网安全组没有入站规则，不能直接访问实例公网 IP。

另开终端进入管理会话并签发八小时 Demo token；复制到 Web 的登录 / token 输入位置。将登录页的“API 地址”改为 `http://localhost:8080`，不要沿用本地开发默认的 18000 端口。不要把令牌或 `runtime.env` 发到聊天、Git、日志中。

```bash
export AWS_PROFILE=videohelper-demo
export AWS_REGION=us-east-2
python3 deploy/aws/demo.py shell
# 以下在 EC2 会话中运行；输出只给本地操作人员使用。
sudo docker compose -p videohelper-demo \
  --env-file /opt/videohelper/runtime.env \
  -f /opt/videohelper/current/compose.yml \
  exec -T auth node /srv/demo-auth.mjs token
```

测试结束在本地运行：

```bash
python3 deploy/aws/demo.py stop
python3 deploy/aws/demo.py status
```

Stop 先停止写入并尝试 `pg_dump` 上传 S3，再停止容器和 EC2；若备份失败仍停止计算，并输出警告。确认状态为 `stopped`，只关闭浏览器 / 终端不会停止 EC2。每次开机两小时后 timer 会尝试备份并停机；下载与长任务也会被中断，应在这段时间内安排测试。无需测试全天可用。

## 发布、故障与恢复

服务器停止时，push 只发布镜像和版本包，不开机；下次 `start` 应用最新 SHA。服务器运行时，CodeBuild 通过 SSM 更新同一实例，期间服务短暂停止。权限限制使构建服务不能启动 EC2。

```bash
# 使用指定已发布 SHA；必须是完整 40 位小写 commit SHA。
python3 deploy/aws/demo.py deploy --sha COMMIT_SHA
# 查看首次构建结果和日志位置。
aws codebuild list-builds-for-project --project-name videohelper-demo-build
aws codebuild batch-get-builds --ids BUILD_ID \
  --query 'builds[].{Status:buildStatus,Logs:logs.deepLink}'
aws cloudformation describe-stacks --stack-name videohelper-demo
```

主机检查命令（SSM shell 中）：

```bash
sudo systemctl status videohelper-stop.timer
sudo cloud-init status --long
sudo docker compose -p videohelper-demo --env-file /opt/videohelper/runtime.env \
  -f /opt/videohelper/current/compose.yml ps
df -h /var/lib/videohelper
free -h
```

`/var/lib/videohelper` 保存数据库、Redis、权重和备份；`/opt/videohelper/runtime.env` 保存随机数据库密码和内部 ML 令牌，权限 0600；鉴权私钥在 `/var/lib/videohelper/auth/`。不能删除这些目录后期待旧数据库自动恢复。Web health 成功后，还要查 ML `/readyz` / `/v1/runtime`、真实处理状态和检索结果。

数据库备份在 artifacts 桶 `backups/`。S3 当前备份保留 30 天，非当前版本另受七天生命周期控制；本地备份保留以便诊断，需检查容量。恢复应先停止写入，下载并用 `pg_restore --list` 检查备份，再恢复到隔离数据库核实，确认后才替换服务数据；尚未执行的恢复演练不能称为已验证。媒体另在 media 桶，数据库备份不包含视频文件、Redis 或鉴权密钥。

主机保留当前与上一版本的配置及镜像，但 ECR 只保留最近三个推送版本，停止期间多次 push 可能让旧版本过期。回滚前核实四类镜像存在；必要时重新构建目标 commit。数据库迁移只前进，不自动回滚；先判断旧代码是否兼容新结构。禁止用 `docker compose down -v` 清数据，也不要把删除 CloudFormation 栈当作 Stop：实例、EBS、S3 有保留策略，删栈可能失败或留下持续收费的资源。彻底清理须先确认需要保留的媒体、数据库和密钥。
