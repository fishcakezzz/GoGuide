# GoGuide

Go 后端主导的任务式导购与模拟交易项目。首版使用 go-zero REST、MySQL 和独立 Go Worker；Eino 将自然语言购物需求转成结构化条件，Go 代码检索商品并检查预算、库存与兼容性。模型不可用时，普通商品、购物车和交易链路仍可使用。

> 电脑配件名称参考公开第三方商品列表；规格、报价、库存、30 天价格曲线和支付均为本地模拟数据，不能在淘宝或其他平台真实购买。项目没有接入淘宝官方商品或历史价格 API。

## 项目文档

- [产品需求 PRD](docs/GoGuide_PRD.md)：原始需求与 A01–A16 验收目标。
- [技术方案与实现分析](docs/GoGuide_Technical_Design.md)：按当前代码整理的技术选型、交易一致性和 Agent 边界。
- [实际架构速览](docs/ARCHITECTURE.md)：进程、模块和当前容量边界。
- [电脑配件演示数据说明](docs/PC_PARTS_DEMO.md)：商品来源、模拟字段、兼容性和价格曲线边界。

## 启动

推荐使用 Docker Compose，一次启动 MySQL 8.4、迁移、种子数据、API 和 Worker；也可用 Go 1.26+ 与独立 MySQL 8.4 手动运行。

将 `.env.example` 复制为 `.env`，分别设置 `GOGUIDE_JWT_SECRET` 与 `GOGUIDE_PAYMENT_SECRET`（至少 32 字符）。Docker Compose 会读取 `.env`：

```powershell
docker compose up --build
```

手动运行时，先创建 `goguide` 数据库并导入环境变量；Go 程序不会自动读取 `.env`：

```powershell
go run ./cmd/migrate
go run ./cmd/seed
go run ./cmd/api
```

另开一个终端执行：

```powershell
go run ./cmd/worker
```

访问 `http://localhost:8080/`。`cmd/seed` 可重复运行，创建原有 10 个 SKU，以及 21 款电脑配件的 63 个模拟报价 SKU，并为后者生成各 30 天模拟价格点；若设置 `GOGUIDE_ADMIN_EMAIL` 和 `GOGUIDE_ADMIN_PASSWORD`，同时创建管理员账号。修改种子数据需要通过管理接口，不应反复覆盖正在交易的库存。

如果页面显示“Failed to fetch”或“无法连接 GoGuide API”，先检查 `http://127.0.0.1:8080/healthz` 是否返回 `{"status":"ok"}`。没有响应时，检查 MySQL、迁移和 `cmd/api` 是否已启动，并确认 `GOGUIDE_ADDR` 指向 8080 端口。网页由 Go API 提供；请从 `http://127.0.0.1:8080/` 访问，不要直接打开 `internal/api/web/index.html`。如果使用独立前端服务器，还需要把 `/v1/*` 代理到 Go API。

智能导购必须配置 `DEEPSEEK_API_KEY`，通过 Eino 调用 DeepSeek V4.1 Flash（API 标识 `deepseek-flash`，默认地址 `https://api.deepseek.com`）拆解需求。密钥只放在本机环境变量或不入库的 `.env` 中。也可使用通用的 `OPENAI_API_KEY`、`OPENAI_MODEL` 与 `OPENAI_BASE_URL` 接入其他兼容服务。未配置模型时导购接口返回 503 和配置提示；商品、购物车和交易接口仍可使用。Go 后端负责商品查询、价格库存与兼容性核验。

## 关键业务路径

1. 注册登录，保存地址；浏览或搜索具体 SKU。
2. 加购、选中商品，调用订单预览；提交时在 MySQL 事务中复核价格和库存，整单预占。
3. 触发模拟支付，预占转已售；取消或超时关单，预占释放。支付与关单通过订单行锁竞争同一状态。
4. 导购会话保存版本化意图；大模型拆解需求，Go 执行只读检索、预算和配件兼容性校验，最多输出三套带 SKU 价格版本的方案，再由模型基于核验结果解释差异。
5. 用户确认方案版本、金额与商品清单后幂等加购；正式下单必须再走订单预览。

API 入口见 `internal/api/base.go`。每个商品结果带 `source_kind` 和 `observed_at`；电脑配件的 `public_reference_demo` 只表示名称参考第三方公开页面，价格、规格和库存仍为演示数据。`GET /v1/skus/{id}/price-trend` 返回明确标记为 `simulated_scenario` 的 30 天曲线。商品缺少关键兼容参数时返回 `UNKNOWN`，不会作为已验证兼容的组合输出。

## 验证

```powershell
go test ./...
go vet ./...
```

设置 `GOGUIDE_TEST_DSN` 后，`go test ./... -count=1` 还会运行 MySQL 集成测试；不设置时这些测试会跳过。API 启动后可运行 `./scripts/smoke.ps1` 验证注册、订单、支付和导购加购。PRD 的 A01–A16 是目标验收项，不能把现有测试通过等同于全部验收通过。尚未完成的事项请看 `STATUS.md` 和 `QUESTIONS.md`。

固定导购任务集在 `eval/tasks.json`，使用**独立、仅导入演示种子商品**的数据库启动 API 后运行 `./scripts/eval.ps1`。脚本逐项检查是否应生成方案、预算与品类、方案金额、兼容性状态和商品证据。同库运行集成测试会插入测试夹具商品，影响候选集和评测结果。DeepSeek V4.1 Flash 实测记录在 `eval/results-deepseek.json`，运行时可用 `-Output` 指定文件。此前固定的 30 项演示任务为 30/30；任务集规模有限，不能据此声称真实场景准确率。实际代码路径和事务边界见 `docs/ARCHITECTURE.md`。
