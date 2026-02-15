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

