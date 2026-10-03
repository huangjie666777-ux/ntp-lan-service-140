# ntp-lan-service

LAN 时间服务：并发轮询 3 个 IPv4 NTP 上游，过滤失准时间源，并通过
UDP(NTPv4) 与 HTTP 为局域网设备授时。**只维护进程内虚拟偏移，绝不修改
系统时钟。**

## 范围

- 真实 UDP 并发轮询 48 字节 NTPv4（RFC 5905 子集，无认证、无广播）。
- 响应校验：来源地址、版本 4、mode 4、stratum 1–14、LI ≠ 3、
  Originate 与请求 Transmit 匹配；非法、迟到（超时后到达）、重复包
  均不进入样本。
- 四时间戳计算 offset/delay，拒绝负 delay 与零（无效）时间戳。
- 每源保留最近 8 个有效样本，用单调时钟判断过期；新鲜样本中取
  delay 最小者，同值取最新。
- 三源均有新鲜候选时，以 offset 中位数为中心保留阈值内源，至少两源
  一致才同步；其中 delay 最小者授时，同值按配置顺序。条件失效即
  未同步，不沿用旧偏移。
- UDP 服务接收 mode 3 请求，返回 mode 4，原样复制 Originate；同步时
  收发时间戳 = 本机时间 + 选中 offset，stratum = 选中源 + 1；未同步
  返回 LI=3、stratum=16，不冒充可靠时间。
- HTTP `GET /status` 暴露各源样本、offset、delay、拒绝原因与当前
  选择，与 UDP 行为一致。
- 关闭时回收轮询 goroutine 与 UDP/HTTP 端口。

## 配置（config.json）

```json
{
  "upstreams": [
    {"name": "up1", "address": "192.168.1.10:123"},
    {"name": "up2", "address": "192.168.1.11:123"},
    {"name": "up3", "address": "192.168.1.12:123"}
  ],
  "udp_listen": "0.0.0.0:10123",
  "http_listen": "0.0.0.0:8080",
  "poll_interval": "2s",
  "poll_timeout": "1s",
  "sample_max_age": "30s",
  "agree_threshold": "500ms"
}
```

## 构建与运行

```sh
go build -o ntp-svc .
./ntp-svc -config config.json
```

## 本机演示

```sh
go build -o /tmp/ntp-upstream ./cmd/upstream
go build -o /tmp/ntp-client ./cmd/client
# 两个正常上游 + 一个失准上游（+10s）
/tmp/ntp-upstream -listen 127.0.0.1:11231 -offset 100ms -stratum 1 &
/tmp/ntp-upstream -listen 127.0.0.1:11232 -offset 120ms -stratum 2 &
/tmp/ntp-upstream -listen 127.0.0.1:11233 -offset 10s  -stratum 1 &
./ntp-svc -config config.json &
curl -s http://127.0.0.1:8080/status   # up3 被中位数滤波排除
/tmp/ntp-client -server 127.0.0.1:10123
```

停掉两个正常上游后，样本过期 → 未同步，客户端收到 LI=3/stratum=16。

## 测试

```sh
go test ./...
```

## 代码结构

- `ntp.go` — 48 字节 NTPv4 编解码、响应校验、offset/delay 计算
- `source.go` — 每源 8 样本环形存储、单调时钟过期、候选选择
- `poller.go` — 并发 UDP 轮询，迟到/重复包过滤
- `select.go` — 中位数一致滤波与授时源选择
- `clock.go` — 进程内虚拟时钟（仅内存偏移）
- `server.go` — mode 3 → mode 4 UDP 授时服务
- `http.go` — chi 路由的 /status 状态接口
- `cmd/upstream`、`cmd/client` — 本机演示上游与客户端
