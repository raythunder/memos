# 本地语义搜索手测（简版）

适用场景：本地手动测试 AI 语义搜索（PostgreSQL）。

## 1. 测试账号

- 用户名：`admin`
- 密码：`Passw0rd!`
- 数据目录：`.tmp/memos-dev`

## 2. 启动 PostgreSQL

首次启动（不存在容器时）：

```bash
docker run -d --name memos-pg \
  -e POSTGRES_USER=postgres \
  -e POSTGRES_PASSWORD=postgres \
  -e POSTGRES_DB=memos \
  -p 5432:5432 \
  postgres:16
```

已存在容器时：

```bash
docker start memos-pg
```

## 3. 启动后端（仓库根目录）

```bash
MEMOS_DATA="$(pwd)/.tmp/memos-dev" \
MEMOS_DRIVER=postgres \
MEMOS_DSN="postgres://postgres:postgres@127.0.0.1:5432/memos?sslmode=disable" \
go run ./cmd/memos --port 8081
```

预期：日志包含 `Database driver: postgres`。

### 3.1 使用 Supabase Postgres（可选）

如果你希望后端直接连接 Supabase（Session Pooler）：

```bash
MEMOS_DATA="$(pwd)/.tmp/memos-dev" \
MEMOS_DRIVER=postgres \
MEMOS_DSN="postgres://postgres.<project-ref>:<password>@aws-0-<region>.pooler.supabase.com:5432/postgres?sslmode=require" \
MEMOS_SUPABASE_PROJECT_URL="https://<project-ref>.supabase.co" \
go run ./cmd/memos --port 8081
```

说明：

- 建议使用 Session Pooler `5432`。
- 连接串需要 `sslmode=require`。
- 当前为后端直连数据库模式，项目创建时不需要启用 automatic RLS。

## 4. 启动前端（新终端）

```bash
cd web
pnpm dev --host 127.0.0.1 --port 5173
```

访问：`http://127.0.0.1:5173`

## 5. 配置 AI

推荐在页面中配置：

- `Settings -> AI`
- 填写 OpenAI API Key（可选再填 Base URL / Embedding Model）
- Base URL 只填服务根路径，例如：
  - OpenAI: `https://api.openai.com/v1`
  - Jina: `https://api.jina.ai/v1`
  - 不要填写 `.../v1/embeddings`（后端会自动拼接 `/embeddings`）
- Embedding 模型支持“列表保存 + 下拉切换”：
  - 每行一个模型名（如 `text-embedding-3-small`、`jina-embeddings-v4`）
  - 在“Embedding 模型”下拉框切换当前生效模型
- 支持一键后台重建向量（Reindex vectors）：
  - 点击 `Start reindex` 后在后端后台运行
  - 页面展示进度（已处理/总数、失败数、模型、时间）
  - 刷新页面后进度仍会从服务端恢复显示

也可用环境变量（后端启动时传入）：

- `MEMOS_OPENAI_API_KEY`
- `MEMOS_OPENAI_BASE_URL`
- `MEMOS_OPENAI_EMBEDDING_MODEL`

## 6. 快速验证

1. 登录账号 `admin / Passw0rd!`
2. 新建一条 memo
3. 搜索栏切到 `semantic` 模式并输入自然语言查询
4. 若未配 AI，预期报错：`semantic search is not configured`
5. 配好 AI 后，等待几秒异步索引，再次搜索应命中相关 memo
