# Editorial Content API

这是面向博客发布流程的 Go 内容服务，配合两个 Next.js 前端使用：

- `Next Blog`：博客前台，负责文章展示、SEO、归档、标签和搜索页面。
- `Next Admin`：后台前端，负责 Markdown 上传、在线编辑、预览、草稿保存和发布。
- `Editorial Content API`：本项目，负责文章元数据、Markdown 渲染、文件存储、发布状态和缓存刷新。

## 整体架构

```text
用户 / 管理员
  ↓
Nginx / Caddy
  ├─ blog.example.com       → Next.js 博客前台
  ├─ admin.example.com      → Next.js 后台前端
  └─ api.example.com        → Editorial Content API
                               ↓
                             MySQL
                               ↓
                    SeaweedFS S3 Gateway
```

建议使用子域名部署，避免 `Next Admin` 使用路径前缀时带来的静态资源和路由配置复杂度。

## 服务职责

### Next Blog

博客前台只读取已发布文章：

- 首页、文章详情、分类、标签、归档页。
- 服务端从 Editorial Content API 获取文章数据。
- 使用 ISR / `revalidatePath` / `revalidateTag` 刷新缓存。
- SEO 信息来自数据库字段，不从 Markdown frontmatter 读取。

### Next Admin

后台前端通过 Editorial Content API 管理内容：

- 上传 `.md` 文件。
- 在线编辑 Markdown。
- 实时预览 Markdown。
- 编辑标题、摘要、slug、封面、SEO 标题和 SEO 描述。
- 保存草稿。
- 一键发布。

标题和摘要是独立字段，不放在 Markdown frontmatter 中。

### Editorial Content API

本项目负责：

- 管理文章元数据。
- 保存 Markdown 原文到 SeaweedFS。
- 使用 `goldmark` 渲染 Markdown。
- 使用 `bluemonday` 清洗 HTML，降低 XSS 风险。
- 保存渲染后的 HTML 到 SeaweedFS。
- 使用 MySQL 保存标题、摘要、slug、状态、SEO 和发布时间。
- 发布成功后调用 Next Blog 的 revalidate 接口。

### MySQL

MySQL 只保存结构化元数据：

```text
posts
- id
- title
- slug
- excerpt
- markdown_path
- rendered_html_path
- cover_image_path
- status: draft / published / archived
- author_id
- seo_title
- seo_description
- published_at
- created_at
- updated_at

users
- id
- email
- password_hash
- role: admin
- is_active
- last_login_at
- created_at
- updated_at

refresh_tokens
- id
- user_id
- token_hash       (sha256, varbinary(32), unique)
- user_agent
- ip_address
- expires_at
- revoked_at
- replaced_by      (旋转链, 指向新 token id)
- created_at
```

`refresh_tokens` 仅保存 SHA-256 哈希，原文只下发到 HttpOnly cookie 一次。每次刷新都会旋转：旧记录写入 `revoked_at` 并把 `replaced_by` 指向新行；如果一个已撤销的 token 再次出现，视为重放，立即把该用户的所有 refresh token 全部撤销。

当前设计已取消文章版本表。

### SeaweedFS

SeaweedFS 通过 S3 Gateway 作为对象存储使用，保存：

```text
posts/{post_id}/source.md
posts/{post_id}/rendered.html
uploads/{year}/{month}/{file}
covers/{post_id}.webp
```

Editorial Content API 使用 S3 SDK 访问 SeaweedFS，未来如果要迁移到 S3、R2、OSS 或 COS，可以尽量减少业务代码改动。

## 发布流程

```text
Next Admin 编辑标题、摘要、slug、SEO 和 Markdown
  ↓
POST /admin/posts
  ↓
Editorial Content API 校验字段
  ↓
Markdown 原文保存到 SeaweedFS
  ↓
Markdown 渲染为 HTML 并清洗
  ↓
渲染 HTML 保存到 SeaweedFS
  ↓
文章元数据保存到 MySQL，状态为 draft
  ↓
POST /admin/posts/{id}/publish
  ↓
Editorial Content API 校验文章可发布
  ↓
更新 status = published，设置 published_at
  ↓
调用 Next Blog revalidate endpoint
  ↓
前台文章页面和列表缓存刷新
```

## API 草案

公开接口：

```text
GET /healthz
GET /posts?limit=20&offset=0
GET /posts/{slug}
```

认证接口：

```text
POST /admin/login      签发 access token + 下发 refresh token cookie
POST /admin/refresh    用 refresh cookie 换新的 access，并旋转 refresh
POST /admin/logout     撤销 refresh，清 cookie
GET  /admin/me         返回当前 JWT 解析出的身份
```

后台业务接口需要携带 `Authorization: Bearer <accessToken>`：

```text
GET  /admin/posts?status=draft&limit=20&offset=0
POST /admin/posts
POST /admin/posts/{id}/publish
POST /admin/uploads/images
```

`POST /admin/login` 请求：

```json
{
  "email": "admin@example.com",
  "password": "change-me"
}
```

成功响应：

```json
{
  "accessToken": "eyJhbGciOi...",
  "tokenType": "Bearer",
  "expiresAt": "2026-05-12T03:18:00Z",
  "user": {
    "id": "0123456789abcdef0123456789abcdef",
    "email": "admin@example.com",
    "role": "admin",
    "isActive": true,
    "lastLoginAt": "2026-05-11T03:18:00Z",
    "createdAt": "2026-05-01T00:00:00Z",
    "updatedAt": "2026-05-11T03:18:00Z"
  }
}
```

同时返回：

```text
Set-Cookie: refresh_token=<raw>; HttpOnly; Secure; SameSite=Lax;
            Path=/admin; Max-Age=2592000
```

`POST /admin/refresh` 不需要 body，浏览器自动带上 cookie，响应：

```json
{
  "accessToken": "eyJhbGciOi...",
  "tokenType": "Bearer",
  "expiresAt": "2026-05-12T03:18:00Z"
}
```

`POST /admin/logout` 成功时返回 `204 No Content` 并下发 `Max-Age=0` 的同名 cookie。
`/admin/login` / `/admin/refresh` / `/admin/logout` 都受 `LOGIN_RATE_LIMIT` 限速保护。

文章列表（公开和后台）使用同一种分页响应结构：

```json
{
  "items": [
    {
      "id": "f8a2...",
      "title": "第一篇文章",
      "slug": "first-post",
      "status": "draft",
      "publishedAt": null,
      "createdAt": "2026-05-11T03:18:00Z",
      "updatedAt": "2026-05-11T03:18:00Z"
    }
  ],
  "total": 42,
  "limit": 20,
  "offset": 0
}
```

图片上传请求使用 `multipart/form-data`，文件字段名为 `file`。服务不调整图片尺寸和格式，只保存原文件并返回访问路径：

```json
{
  "id": "f8a2...",
  "key": "uploads/images/2026/05/f8a2.../original.jpg",
  "url": "http://localhost:8333/blog/uploads/images/2026/05/f8a2.../original.jpg",
  "contentType": "image/jpeg"
}
```

保存草稿请求示例（`authorId` 由服务端从 JWT 注入，前端不需要传）：

```json
{
  "title": "第一篇文章",
  "slug": "first-post",
  "excerpt": "这是摘要，单独由后台字段管理。",
  "markdown": "# 正文标题\n\n这里是 Markdown 正文。",
  "coverImagePath": "covers/first-post.webp",
  "seoTitle": "第一篇文章 SEO 标题",
  "seoDescription": "第一篇文章 SEO 描述"
}
```

公开文章响应示例（`GET /posts/{slug}`）：

```json
{
  "id": "f8a2...",
  "title": "第一篇文章",
  "slug": "first-post",
  "excerpt": "这是摘要，单独由后台字段管理。",
  "html": "<h1 id=\"正文标题\">正文标题</h1>\n<p>这里是 Markdown 正文。</p>",
  "status": "published",
  "seoTitle": "第一篇文章 SEO 标题",
  "seoDescription": "第一篇文章 SEO 描述",
  "publishedAt": "2026-05-07T03:18:00Z"
}
```

## 本地运行

1. 复制环境变量：

```bash
cp .env.example .env
```

2. 启动 MySQL 和 SeaweedFS：

```bash
docker compose -f deployments/docker-compose.yml up -d mysql seaweed
```

3. 创建 SeaweedFS bucket：

```bash
aws --endpoint-url http://localhost:8333 s3 mb s3://blog
```

4. 运行 API：

```bash
go run ./cmd/server
```

也可以直接启动完整后端栈：

```bash
docker compose -f deployments/docker-compose.yml up --build
```

API 启动后会通过 GORM `AutoMigrate` 自动创建或更新 `posts`、`users` 等表结构。

## 创建管理员账号

应用不会自动创建首个管理员。API 首次启动并自动建表后，手动插入一个 `admin` 用户，并把 `password_hash` 替换为你生成的 bcrypt 哈希：

```sql
insert into users (
  id, email, password_hash, role, is_active, created_at, updated_at
) values (
  '0123456789abcdef0123456789abcdef',
  'admin@example.com',
  '$2a$12$replace_with_your_bcrypt_hash',
  'admin',
  true,
  utc_timestamp(6),
  utc_timestamp(6)
);
```

## 环境变量

```text
APP_ENV=development
HTTP_HOST=0.0.0.0
HTTP_PORT=8080

DATABASE_URL=blog:blog_password@tcp(localhost:3306)/blog?parseTime=true&loc=UTC
DATABASE_MAX_OPEN_CONNS=25
DATABASE_MAX_IDLE_CONNS=5
DATABASE_CONN_MAX_LIFETIME=30m

JWT_SECRET=change-this-to-a-long-random-secret
JWT_ISSUER=editorial-content-api
JWT_ACCESS_TOKEN_TTL=24h
JWT_REFRESH_TOKEN_TTL=720h
REFRESH_COOKIE_NAME=refresh_token
REFRESH_COOKIE_PATH=/admin
REFRESH_COOKIE_DOMAIN=
REFRESH_COOKIE_SECURE=false
REFRESH_COOKIE_SAMESITE=lax

IMAGE_UPLOAD_MAX_BYTES=10485760
PUBLIC_BASE_URL=http://localhost:8080

S3_ENDPOINT=http://localhost:8333
S3_REGION=us-east-1
S3_BUCKET=blog
S3_ACCESS_KEY_ID=admin
S3_SECRET_ACCESS_KEY=admin
S3_USE_PATH_STYLE=true
S3_PUBLIC_BASE_URL=http://localhost:8333/blog

NEXT_REVALIDATE_URL=http://localhost:3000/api/revalidate
NEXT_REVALIDATE_SECRET=change-me-too

ALLOWED_ORIGINS=http://localhost:3000,http://localhost:3001
LOGIN_RATE_LIMIT=10
LOGIN_RATE_WINDOW=1m
```

生产环境必须做：

- 替换 `JWT_SECRET`、`NEXT_REVALIDATE_SECRET` 和 SeaweedFS S3 凭据。
- `REFRESH_COOKIE_SECURE=true`。
- 跨子域部署时 `REFRESH_COOKIE_DOMAIN=.example.com`、`REFRESH_COOKIE_SAMESITE=none`，`ALLOWED_ORIGINS` 配置具体的后台域名（不能是 `*`）。

`JWT_REFRESH_TOKEN_TTL` 必须大于 `JWT_ACCESS_TOKEN_TTL`；`REFRESH_COOKIE_SAMESITE=none` 时 `REFRESH_COOKIE_SECURE` 必须为 `true`，否则启动校验失败。

## Next Admin 前端集成

后台采用 **access token (24h) + refresh token (30d)** 的双令牌方案，目标是让用户在 30 天内无需重复输入密码，同时保留服务端单点撤销的能力。

### 关键约定

- access token 只放在前端内存（React state / Zustand），**不要写 `localStorage`**，避免被 XSS 偷走。
- refresh token 通过 HttpOnly cookie 下发，JS 读不到，浏览器自动持久化最多 30 天。
- 所有调用 `/admin/*` 的请求都必须 `credentials: "include"`，否则浏览器不会带上 cookie。
- 同源部署最省事。跨子域要按上一节的说明配 `REFRESH_COOKIE_*` 与 `ALLOWED_ORIGINS`。

### 流程

```text
用户首次访问 /login
  ↓ POST /admin/login { email, password }
  body  : { accessToken, expiresAt, user }
  cookie: Set-Cookie: refresh_token=...  HttpOnly
  前端  : access 存内存，跳转到 /admin

调用业务接口
  Authorization: Bearer <access>
   ↓ 200 → 正常返回
   ↓ 401 → 走 refresh 流程

静默续期（access 过期 或 页面刷新后内存丢失）
  POST /admin/refresh   (浏览器自动带 cookie)
  body  : { accessToken, expiresAt }
  cookie: Set-Cookie: refresh_token=<旋转后的新值>
  前端  : 更新内存中的 access，重试刚才那个 401 请求
  失败  : 跳转 /login

登出
  POST /admin/logout
  204 + 清 cookie；前端清空内存中的 access
```

### 实现步骤

1. **建一个 auth store**，只在内存里维护 `accessToken / expiresAt / user`。
2. **封装 `apiFetch`**，自动加 `Authorization` 头，遇到 401 调一次 `/admin/refresh` 后重试；用单例 `Promise` 防止并发请求触发多次旋转。
3. **登录页**：`POST /admin/login` 成功后把响应里的 `accessToken / user` 写进 store，跳转到 `/admin`。
4. **后台 layout 里挂一个 `AuthBootstrap`**：组件挂载时若内存里没有 access，主动调用 `POST /admin/refresh`。命中则进后台（实现「关闭浏览器后再回来仍在线」），失败则跳 `/login`。
5. **登出按钮**：`POST /admin/logout` → 清 store → 跳 `/login`。
6. （可选）在 store 里加一个轮询，access 剩余 5 分钟时主动调 refresh，避免业务请求遇到 401 的轻微延迟。

### `fetch` 调用最小示例

```ts
// 登录
await fetch(`${API}/admin/login`, {
  method: 'POST',
  credentials: 'include',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ email, password }),
})

// 静默续期
await fetch(`${API}/admin/refresh`, {
  method: 'POST',
  credentials: 'include',
})

// 调用业务接口
await fetch(`${API}/admin/posts?limit=20&offset=0`, {
  credentials: 'include',
  headers: { Authorization: `Bearer ${accessToken}` },
})

// 登出
await fetch(`${API}/admin/logout`, {
  method: 'POST',
  credentials: 'include',
})
```

### 常见坑速查

| 现象 | 原因 | 解决 |
|---|---|---|
| `/admin/refresh` 总是 401 | fetch 没带 `credentials: "include"` | 所有 `/admin/*` 请求都加 |
| 看到 `Set-Cookie` 但下次没带回来 | 跨站 + SameSite=Lax | 后端改 `SAMESITE=none` 且 `SECURE=true` |
| Safari/Chrome 下 cookie 直接不下发 | HTTP + Secure=true | 本地开发 `SECURE=false` + `SAMESITE=lax` |
| CORS 报错 `Allow-Origin cannot be *` | 带 credentials 时不允许通配 | 把 `ALLOWED_ORIGINS` 配成具体域名 |
| 多个并发请求同时触发了 refresh | 没做去重 | `callRefresh` 用单例 Promise |
| 第二天回来需要重新登录 | refresh cookie 没成功落地 | DevTools → Application → Cookies 检查 `refresh_token` 是否存在、Path 是否覆盖请求路径 |
| 前端清了 cookie 但服务端没失效 | 仅清 cookie 不算登出 | 必须调 `POST /admin/logout`，让服务端 revoke 这条 refresh |

### 本地验证清单

1. `.env` 设置：`REFRESH_COOKIE_SECURE=false`、`REFRESH_COOKIE_SAMESITE=lax`、`ALLOWED_ORIGINS=http://localhost:3000`。
2. 启动 API：`go run ./cmd/server`，启动 Next Admin。
3. 浏览器登录后，DevTools → Application → Cookies 应能看到 `refresh_token`，HttpOnly = ✔，Expires 约 30 天后。
4. 关闭整个浏览器再打开 → 进 `/admin` → 应在一瞬间直接进入后台（说明 `AuthBootstrap` 的静默 refresh 生效）。
5. 在 MySQL 查 `select id, user_id, revoked_at, replaced_by, expires_at from refresh_tokens;`，每次刷新页面都能看到旋转链。
6. 点登出 → 浏览器 cookie 被清，数据库对应行 `revoked_at` 有值。

## Next Blog Revalidate 接口约定

Editorial Content API 发布文章后会向 `NEXT_REVALIDATE_URL` 发送：

```json
{
  "secret": "change-me-too",
  "path": "/posts/first-post",
  "tag": "posts"
}
```

Next Blog 可以在 `/api/revalidate` 中校验 `secret`，然后执行：

```ts
revalidatePath(body.path)
revalidateTag(body.tag)
```

## 目录结构

```text
cmd/server                  应用入口
internal/config             环境变量配置
internal/domain             领域模型 (Post / User / RefreshToken)
internal/markdown           Markdown 渲染和 HTML 清洗
internal/repository/mysql   MySQL 仓储 (post / user / refresh_token) + AutoMigrate
internal/service            业务逻辑 (PostService / AuthService / ImageService)
internal/storage            对象存储接口
internal/storage/s3         SeaweedFS / S3 实现
internal/transport/http
  router.go                 路由表与共享 helper
  handlers.go               所有 HTTP handler
  middleware.go             日志 / CORS / requireAdmin / 登录限速 / refresh cookie
  dto.go                    响应 DTO 与 domain → DTO 映射
  revalidator.go            发布后调用 Next Blog 的 HTTPRevalidator
deployments                 Docker Compose 和 SeaweedFS 配置
```

## 后续建议

- 增加标签、分类和搜索表。
- 增加后台预览接口：只渲染 Markdown，不保存草稿。
- 给 `service` 与 `auth` 路径补单元测试（登录 / refresh 旋转 / 重放检测 / logout）。
- 把 `AutoMigrate` 换成 `golang-migrate` 等显式迁移工具，生产环境更安全。
- 接入 OpenTelemetry / Prometheus，补充指标和分布式追踪。
- 生产环境增加数据库和 SeaweedFS 数据目录备份任务。
