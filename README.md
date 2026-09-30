# 硬件逻辑分析工作站

支持从多路人工电平采样中解码 UART（8/9 数据位、none/even/odd/mark/space 校验、1/2 停止位、高低空闲极性可配置）和 SPI 时钟同步串行（位序 MSB/LSB、上升/下降采样边沿、片选通道与有效电平、每字位数可配置）。采样块、解码配置、结果版本分开保存；同一采样记录可挂多份不同协议/参数的配置，修改配置会新增配置版本并生成独立结果，原结果保留可对照，互不覆盖。

## 结构

- `backend/internal/model`：采样、配置、标记和结果模型。
- `backend/internal/decode`：协议注册表；`uart.go`、`spi.go` 为已启用协议。新增协议时在 `registry.go` 注册。
- `backend/internal/store`：内存存储，捕获、采样块、配置版本、结果互不覆盖；可选快照文件持久化。
- `backend/internal/api`：HTTP API 与内置核对波形。
- `frontend/src/components/WaveformTrack.tsx`：轨道、标记、以鼠标时间点为锚点的缩放和平移。

## 解码语义

- 采样块按索引和时间戳拼接；UART/SPI 状态跨块保留，半个帧可在下一块继续。
- 块时间戳不连续且交叠在帧内时，输出 `sampling-gap`，丢弃未完成帧，不补造字节。
- 停止位错误、校验错误和超出容差的时钟边沿异常均输出独立错误标记，不产生正常字节。
- SPI 在片选有效区间内按配置边沿采样；片选中途结束或采样缺口会输出 `incomplete-frame` / `sampling-gap` 不完整报文标记，保留已采原始位，不补出未采到的字节。
- 字节和错误都有纳秒级 `startNs`/`endNs`，叠加在原始波形轨道上核对。

## 运行

```bash
cd backend && go run ./cmd/server -addr :8080 -static ../frontend/dist -data logicstation-data.json
```

`-data` 指定快照文件：每次新增捕获、采样块、配置或重算结果都会落盘，服务重启后自动恢复，字节标记仍定位到原始采样区间。置空则关闭持久化。

开发模式：

```bash
cd backend && go run ./cmd/server -addr :8080
cd frontend && npm install && npm run dev
```

打开页面时优先载入已保存的采样记录；没有记录时自动调用 `POST /api/demo` 生成内置核对波形：

- UART 波形：有效字节以及跨块续接后发现的停止位异常。
- SPI 波形：已知发送内容 0xA5、跨块报文 0x3C、一个片选中途结束的不完整报文；同一份采样挂「上升沿采样（正确）」和「下降沿采样（错误边沿）」两份配置，结果并列对照。

## 验证

```bash
cd backend && go test ./... && go vet ./...
cd frontend && npm run build
```
