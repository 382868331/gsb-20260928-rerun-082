# 硬件逻辑分析工作站

首版支持从多路人工电平采样中解码 UART（8/9 数据位、none/even/odd/mark/space 校验、1/2 停止位、高低空闲极性可配置）。采样块、解码配置、结果版本分开保存；修改配置会新增配置版本并生成独立结果，原结果保留可比较。

## 结构

- `backend/internal/model`：采样、配置、标记和结果模型。
- `backend/internal/decode`：协议注册表；`uart.go` 是当前唯一启用协议。新增协议时在 `registry.go` 注册。
- `backend/internal/store`：内存存储，捕获、采样块、配置版本、结果互不覆盖。
- `backend/internal/api`：HTTP API 与内置核对波形。
- `frontend/src/components/WaveformTrack.tsx`：轨道、标记、以鼠标时间点为锚点的缩放和平移。

## 解码语义

- 采样块按索引和时间戳拼接；UART 状态跨块保留，半个帧可在下一块继续。
- 块时间戳不连续且交叠在帧内时，输出 `sampling-gap`，丢弃未完成帧，不补造字节。
- 停止位错误、校验错误和超出容差的时钟边沿异常均输出独立错误标记，不产生正常字节。
- 字节和错误都有纳秒级 `startNs`/`endNs`，叠加在原始波形轨道上核对。

## 运行

```bash
cd backend && go run ./cmd/server -addr :8080 -static ../frontend/dist
```

开发模式：

```bash
cd backend && go run ./cmd/server -addr :8080
cd frontend && npm install && npm run dev
```

打开页面后会自动调用 `POST /api/demo` 生成内置 UART 波形。该波形包含有效字节以及跨块续接后发现的停止位异常。

## 验证

```bash
cd backend && go test ./... && go vet ./...
cd frontend && npm run build
```
