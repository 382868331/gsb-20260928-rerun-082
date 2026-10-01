# 硬件逻辑分析工作站

支持从多路人工电平采样中解码 UART（8/9 数据位、none/even/odd/mark/space 校验、1/2 停止位、高低空闲极性可配置）和 SPI 风格时钟同步串行（位序 MSB/LSB、上升/下降采样边沿、片选通道与有效电平、每字位数可配置）。采样块、解码配置、结果版本分开保存；同一采样记录可用不同解码配置对照，修改配置会新增配置版本并生成独立结果，原结果保留可比较。

## 结构

- `backend/internal/model`：采样、配置、标记和结果模型。
- `backend/internal/decode`：协议注册表；`uart.go` 与 `spi.go` 是启用协议。新增协议时在 `registry.go` 注册。
- `backend/internal/store`：内存存储，捕获、采样块、配置版本、结果互不覆盖。
- `backend/internal/api`：HTTP API 与内置核对波形。
- `frontend/src/components/WaveformTrack.tsx`：轨道、标记、以鼠标时间点为锚点的缩放和平移。

## 解码语义

- 采样块按索引和时间戳拼接；UART 状态跨块保留，半个帧可在下一块继续。
- 块时间戳不连续且交叠在帧内时，输出 `sampling-gap`，丢弃未完成帧，不补造字节。
- 停止位错误、校验错误和超出容差的时钟边沿异常均输出独立错误标记，不产生正常字节。
- 字节和错误都有纳秒级 `startNs`/`endNs`，叠加在原始波形轨道上核对。
- SPI 在片选有效区间内按配置边沿采样数据位；片选中途结束或采集结束时输出 `incomplete-frame` 并保留原始位，不补全字节。
- 片选无效期间出现的采样时钟边沿输出 `clock-outside-cs` 错误标记。
- 结果列表中点击任一标记可将波形视口定位到该字节/错误的原始采样区间，缩放平移不影响定位。
- 再次载入内置示例（含刷新页面后）按采样名称复用同一采样身份：重开的是原采样上用户另存过的配置与结果对照，不会生成同名新示例。
- `POST /api/results/diff` 对照同一采样的两个解码结果，定位首个不同标记的波形区间，并按 `bit-order`（位序）、`clock-edge`（采样边沿）、`incomplete-frame`（片段不完整）说明差异根因；前端可从差异条目直接跳到对应波形区间。

## 运行

```bash
cd backend && go run ./cmd/server -addr :8080 -static ../frontend/dist
```

开发模式：

```bash
cd backend && go run ./cmd/server -addr :8080
cd frontend && npm install && npm run dev
```

打开页面后会自动调用 `POST /api/demo` 生成内置核对波形。UART 波形包含有效字节以及跨块续接后发现的停止位异常；SPI 波形包含已知字节 0x48/0x69（跨块续接）、片选中途结束的不完整报文和片选无效期间的错误时钟边沿，并预置 MSB/LSB/错误边沿三组配置用于对照。

## 验证

```bash
cd backend && go test ./... && go vet ./...
cd frontend && npm run build
```
