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
```

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

登录接口：

```text
POST /admin/login
GET  /admin/me
```

登录请求示例：

```json
{
  "email": "admin@example.com",
  "password": "change-me"
}
```

后台接口需要携带 `Authorization: Bearer <accessToken>`：

```text
GET  /admin/posts?status=draft&limit=20&offset=0
POST /admin/posts
POST /admin/posts/{id}/publish
```

保存草稿请求示例：

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

公开文章响应示例：

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

JWT_SECRET=change-this-to-a-long-random-secret
JWT_ISSUER=editorial-content-api
JWT_ACCESS_TOKEN_TTL=1h
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
```

生产环境必须替换 `JWT_SECRET`、`NEXT_REVALIDATE_SECRET` 和 SeaweedFS S3 凭据。

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
internal/domain             领域模型
internal/markdown           Markdown 渲染和 HTML 清洗
internal/repository         MySQL 仓储
internal/service            文章业务逻辑
internal/storage            对象存储接口和 SeaweedFS S3 实现
internal/transport/http     HTTP 路由和处理器
deployments                 Docker Compose 和 SeaweedFS 配置
```

## 后续建议

- 增加图片上传接口，统一生成 WebP、尺寸和公开访问路径。
- 增加标签、分类和搜索表。
- 增加后台预览接口：只渲染 Markdown，不保存草稿。
- 生产环境增加数据库和 SeaweedFS 数据目录备份任务。
