# GoGuide 实际架构

本文件描述当前代码，不把 PRD 中的计划能力视为已交付。项目是独立仓库，未依赖 `go-zero-looklook`。

```mermaid
flowchart LR
    UI[浏览器演示页] --> API[Go / go-zero REST]
    API --> Trade[Go 交易逻辑]
    API --> Guide[Go 导购编排]
    Guide --> Eino[Eino 需求提取与方案解释]
    Guide --> Rules[结构化检索与兼容性规则]
    Trade --> DB[(MySQL 8.4)]
    Rules --> DB
    Worker[Go 超时订单 Worker] --> DB
```

## 代码边界

| 部分 | 路径 | 职责 |
| --- | --- | --- |
| HTTP 服务 | `cmd/api`、`internal/api/base.go` | go-zero REST 注册路由、鉴权、请求校验与响应 |
| 商品和管理 | `internal/api/catalog.go` | SKU 检索、规格、价格库存维护及审计 |
| 交易 | `internal/api/orders.go`、`internal/trade/trade.go` | 预览、幂等下单、库存预占、支付/关单状态转移 |
| 导购 | `internal/api/guide_*.go`、`internal/domain/domain.go` | 意图版本、受控查询、组合规划、兼容性和方案确认 |
| 后台作业 | `cmd/worker` | 到期订单扫描，失败后下次扫描可重试 |
| 数据库 | `migrations` | 表结构及版本化迁移 |
| 演示与回归 | `internal/api/web`、`scripts`、`eval` | 浏览器页面、HTTP 冒烟测试、固定任务评测 |

当前直接使用 go-zero 的 REST 服务和手写路由，没有使用 `goctl .api` 生成代码。商品、导购和交易在同一个 Go 进程内，Worker 是独立进程，共享 MySQL。首版用模块边界和数据库事务处理一致性，不依赖消息队列或拆分微服务。

## 核心不变量

1. 模型提取自然语言意图，并根据已核验的结构化方案解释选择；SKU、价格、库存和兼容性结论由 Go 与 MySQL 决定。无法验证的硬条件不能进入“已验证方案”。
2. 方案记录意图版本、SKU 价格版本、金额和证据；确认时复核版本与金额，写购物车需要用户明确操作。正式下单重新预览并在事务中锁定库存。
3. 库存按 `available / reserved / sold` 转移。下单预占，付款转已售，取消或到期释放；支付与关闭锁同一订单行，重复请求按状态幂等处理。
4. 电脑配件名称参考公开第三方列表；规格、报价、库存、价格曲线与支付是模拟数据。无淘宝官方 API 报价、真实资金或外部订单履约。

## 当前容量边界

导购检索每个品类最多 30 个 SKU，规划器取其中最多 12 个，最多 10 个品类；每轮按预算与已选配件兼容性剪枝，保留最多 1200 个组合。当前共有 73 个演示 SKU，其中电脑配件为 21 款、63 个报价。剪枝是面试演示规模的启发式实现，在大目录中仍可能漏召回；需增加规格索引、分层召回及排序评测。`demo_price_snapshots` 的 30 天曲线纯属合成场景。工具日志记录在 `guide_tool_runs`，但尚无完整的模型调用成本与生产链路追踪。
