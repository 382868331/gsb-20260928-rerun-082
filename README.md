# 硬件逻辑分析工作站（Logic Analysis Station）

从人工电平采样中解码串行通信的最小可用工作站。第一项支持的协议是
**UART**；采样配置、解码配置、解码结果三者各自版本化、互不覆盖，后续
增加协议时只需在 `internal/decode` 注册新解码器。

- **Go 后端**：`internal/model`（数据模型）、`internal/decode`（协议解码器，
  目前为流式 UART）、`internal/engine`（重算与结果对比）、`internal/store`
  （三类版本化存储）、`internal/demo`（自带核对波形）、`internal/api`（HTTP）。
- **React 前端**：`frontend/`，页面分“轨道”和“标记/对比”。

## 语义保证

- **缩放不改变时间定位**：`frontend/src/time.ts` 的 `zoomAt` 以指针所在的
  绝对时刻为锚点，缩放后该时刻仍停留在同一屏幕位置（有单元测试）。
- **跨块接续**：解码器是流式的，帧在数据块中间断开时由解码器内部状态
  接续，半个报文跨块与单块解码结果完全一致（见 `TestFrameSplitAcrossBlocks`）。
- **错误保留、绝不修补成字节**：帧中途遇到采样间隙产生 `sample_gap`，
  边沿偏离位格超过容差产生 `clock_anomaly`，停止位/校验失败产生
  `framing`/`parity`；错误项不带字节值。
- **改配置重算且结果可比**：结果版本由 `hash(采样配置版本, 解码配置版本)`
  确定性导出；旧结果保留，`POST /api/compare` 按起始时刻对齐比较。
- **自带波形核对**：`POST /api/demo` 生成内容已知的 UART 波形，可注入
  采样间隙/时钟异常，用于核对解码内容及每帧起止时刻（含逐位采样时刻）。

## 运行

```bash
# 后端（默认 :8080，静态托管 frontend/dist）
go run ./cmd/logicstation

# 前端开发（代理 /api 到 :8080）
cd frontend && npm install && npm run dev

# 构建前端
cd frontend && npm run build

# 测试
go test ./...
cd frontend && npx vitest run && npx tsc --noEmit
```

## HTTP 摘要

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| PUT | `/api/config/sample` | 创建采样配置版本（不可变） |
| PUT | `/api/config/decode` | 创建解码配置版本（不可变） |
| POST | `/api/blocks` | 追加一个采样块；`startIndex` 跳跃即表示采样间隙 |
| POST | `/api/decode` | 用指定的两个配置版本重算，返回版本化结果 |
| POST | `/api/compare` | 按起始时刻对齐比较两个结果版本 |
| POST | `/api/demo` | 生成自带波形（可注入间隙/时钟异常）并载入 |

采样块 `levels` 为 base64 字节，每字节一位对应采样配置中的通道
（通道 i 对应 bit i）。
