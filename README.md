# VideoHelper Landing Page

VideoHelper 的独立落地页：只有前端静态页面，不连接后端，也没有进入工作台的入口。首屏的迷你工作台是本地演示，示例视频为开放许可片段（出处见页脚）。

源自主仓库 `main` 分支的 `web/`，图标来自 `Design/Icon`。

## 开发

需要 Node 22+。

```sh
npm ci
npm run dev      # http://127.0.0.1:5173
npm test         # 演示时间线测试
npm run build    # 类型检查并输出 dist/，可直接部署到任意静态主机
```

## 部署

线上地址 <https://videohelper.ozine.me>，托管在 Cloudflare Workers（静态资源，无 Worker 脚本）。配置见 `wrangler.jsonc`：只绑定这个自定义域名，`workers.dev` 与预览地址关闭；域名在同一 Cloudflare 账号下，部署时自动创建 DNS 记录与证书。

```sh
npx wrangler login   # 首次：浏览器授权
npm run deploy       # 构建并上传 dist/
```
