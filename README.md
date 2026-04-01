# CC2API

Claude Code to API Gateway — 反检测网关 + 号池管理平台。

## 功能

- **号池管理**：多 Claude 账号轮转，自动生成设备指纹，每号可配独立代理
- **反检测**：Header wire casing 还原、TLS 指纹伪装（Node.js 24.x）、系统提示词改写、硬件指纹伪装等
- **智能路由**：sticky session 保证同一会话绑定同一账号，自动故障转移与限速处理
- **双模式**：同时支持 Claude Code 客户端（替换模式）和直接 API 调用（注入模式）
- **Web 管理**：账号增删改查、连通性测试、用量统计

## 快速部署（Docker）

> 镜像地址：`ghcr.io/mamoworks/cc2api:latest`

```bash
# 1. 创建目录
mkdir cc2api && cd cc2api

# 2. 下载配置模板
curl -O https://raw.githubusercontent.com/MamoWorks/cc2api/main/config.example.json
cp config.example.json config.json

# 3. 编辑配置（修改密码和 API Key）
vim config.json

# 4. 创建数据目录
mkdir data

# 5. 启动
docker run -d \
  --name cc2api \
  -p 8080:8080 \
  -v $(pwd)/config.json:/app/config.json \
  -v $(pwd)/data:/app/data \
  --restart unless-stopped \
  ghcr.io/mamoworks/cc2api:latest
```

或使用 docker-compose：

```yaml
# docker-compose.yml
services:
  cc2api:
    image: ghcr.io/mamoworks/cc2api:latest
    ports:
      - "8080:8080"
    volumes:
      - ./config.json:/app/config.json
      - ./data:/app/data
    restart: unless-stopped
```

```bash
docker compose up -d
```

> **注意**：`data/` 目录必须挂载，SQLite 数据库存放在此目录下。不挂载会导致容器重启后数据丢失。

## 配置文件

`config.json`（不存在则使用默认值）：

```json
{
  "server": { "host": "0.0.0.0", "port": 8080 },
  "database": { "driver": "sqlite", "dsn": "data/cc2api.db" },
  "admin": { "password": "your_password", "api_key": "your_gateway_key" },
  "log_level": "info"
}
```

| 字段 | 说明 |
|------|------|
| `database.driver` | `sqlite`（默认）或 `postgres` |
| `database.dsn` | SQLite 填文件路径，PostgreSQL 填连接串 |
| `redis` | 添加此字段启用 Redis（可选，默认内存缓存） |
| `admin.password` | 管理后台登录密码 |
| `admin.api_key` | 客户端连接网关的 API Key |
| `log_level` | `debug` / `info`（默认）/ `warn` / `error` |

## 使用

### 添加账号

访问管理后台 `http://your-server:8080`，密码为 `config.json` 中的 `admin.password`。

点击「添加账号」，填写：
- **邮箱**（必填）：Claude 账号邮箱
- **Token**（必填）：OAuth token（`sk-ant-oat01-...`）
- **代理地址**（选填）：`http://host:port` 或 `socks5://host:port`

### 客户端配置

**Claude Code：**
```bash
export ANTHROPIC_BASE_URL="http://your-server:8080"
export ANTHROPIC_API_KEY="your_gateway_key"
```

**API 调用：**
```bash
curl http://your-server:8080/v1/messages \
  -H "x-api-key: your_gateway_key" \
  -H "Content-Type: application/json" \
  -d '{"model":"claude-sonnet-4-5-20250929","max_tokens":1024,"messages":[{"role":"user","content":"hello"}]}'
```

## 本地编译

```bash
# 后端
go build -o cc2api ./cmd/server/

# 前端（可选，生产模式由后端 serve）
cd web && npm ci && npm run build
```

## 获取 OAuth Token

1. 在本地完成 Claude OAuth 登录
2. 运行以下命令生成长效 token：
   ```bash
   claude setup-token
   ```
3. 复制输出的 `sk-ant-oat01-...` token，填入账号管理

## PostgreSQL 部署

默认使用 SQLite（零依赖），生产环境可切换 PostgreSQL：

```json
{
  "database": {
    "driver": "postgres",
    "host": "localhost",
    "port": 5432,
    "user": "postgres",
    "password": "your_password",
    "dbname": "cc2api"
  }
}
```

docker-compose 完整版（含 PostgreSQL + Redis）：

```yaml
services:
  cc2api:
    image: ghcr.io/mamoworks/cc2api:latest
    ports:
      - "8080:8080"
    volumes:
      - ./config.json:/app/config.json
    depends_on:
      postgres:
        condition: service_healthy
    restart: unless-stopped

  postgres:
    image: postgres:15-alpine
    environment:
      POSTGRES_DB: cc2api
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:-cc2api_secret}
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postgres"]
      interval: 5s
      timeout: 3s
      retries: 5

volumes:
  pgdata:
```

> PostgreSQL 模式下不需要挂载 `data/` 目录。

## API 端点

| 端点 | 说明 |
|------|------|
| `POST /v1/messages` | Claude API 转发 |
| `GET /v1/models` | 可用模型列表 |
| `GET /_health` | 健康检查 |
| `GET /admin/accounts` | 账号列表 |
| `POST /admin/accounts` | 创建账号 |
| `PUT /admin/accounts/:id` | 更新账号 |
| `DELETE /admin/accounts/:id` | 删除账号 |
| `POST /admin/accounts/:id/test` | 测试账号连通性 |
| `GET /admin/usage?hours=24` | 用量统计 |
| `GET /admin/dashboard` | Dashboard 数据 |
