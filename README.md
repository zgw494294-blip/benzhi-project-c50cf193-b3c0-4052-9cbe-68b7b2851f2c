# 种源活力入库核验台

种源活力入库核验台面向种质资源接收员、实验检测员、质量复核员和资源库管理员。系统用一条可追溯流程完成批次建档、活力与纯度检测、规则检查、异常整改复测、独立复核、冻结以及入库凭据签发。

服务不依赖外部数据库。业务事件写入带递增序号和 SHA-256 摘要链的本地 JSON Lines 账本，查询投影通过原子替换的 JSON 快照保存。所有批次写命令同时校验 `expected_version` 和 `idempotency_key`。

## 构建与测试

项目需要 Go 1.23 或更高版本：

```text
go build ./cmd/seedvault
go test ./...
```

## 运行

默认仅监听高位回环地址 `127.0.0.1:19081`：

```text
go run ./cmd/seedvault
```

可通过参数选择其他回环端口：

```text
go run ./cmd/seedvault -addr=127.0.0.1:19181
```

也可以设置 `PORT` 为端口号；此时服务绑定 `127.0.0.1:<PORT>`。`-addr` 优先于 `PORT`。为了避免意外暴露检测资料，服务会拒绝 `0.0.0.0` 等非回环监听地址。持久化目录默认为 `.seedvault-data`，可用 `-data` 调整。

浏览器打开 `http://127.0.0.1:19081/` 即可使用工作台；公开凭据验证页位于 `/verify`。

## 有界自检

下面的命令会在指定地址临时启动 HTTP 服务，通过真实 JSON API 驱动“建档→不合格检测→整改复测→独立复核→冻结签发→凭据验证”全流程，然后自动关停并清理临时数据：

```text
go run ./cmd/seedvault -addr=127.0.0.1:19081 -selfcheck
```

## 主要 API

- `GET /api/batches` 与 `POST /api/batches`：查询和创建批次；查询参数支持 `species`、`status`、`source_region`、`harvest_from`、`harvest_to`、`page`、`page_size`，带参数时返回分页和状态统计。
- `GET /api/batches/{batchID}`：读取批次完整投影。
- `GET /api/batches/{batchID}/evidence-preview`：读取冻结前证据清单、清单摘要和原测/复测指标对比。
- `POST /api/batches/{batchID}/tests`：保存检测并执行物种质量规则。
- `POST /api/batches/{batchID}/remediations`：提交整改说明和替代复测。
- `POST /api/batches/{batchID}/reviews`：提交独立复核决定。
- `POST /api/batches/{batchID}/freeze`：冻结快照并签发凭据。
- `POST /api/credentials/{credentialID}/revoke`：管理员填写原因撤销有效凭据，并追加批次审计事件。
- `POST /api/credentials/verify`：核对凭据编号和冻结摘要。

写入 API 的 JSON 请求应带 `idempotency_key`，也可通过 `Idempotency-Key` 请求头提供。除创建批次外，写入命令还必须提交当前 `expected_version`。
