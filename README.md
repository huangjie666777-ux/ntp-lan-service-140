# ntp-lan-service-140

局域网 NTP 过滤与授时服务。从三个 IPv4 上游并发轮询，过滤失准时间源，
以**进程内虚拟偏移**为本机客户端授时——**从不修改系统时钟**。

## 构建与测试

```sh
go build ./...
go test ./...
```

## 运行

```sh
go run ./cmd/ntpd \
  -udp 127.0.0.1:1230 -http 127.0.0.1:8080 \
  -poll 5s -timeout 2s -validity 30s -threshold 500ms \
  -upstream 10.0.0.11:123 -upstream 10.0.0.12:123 -upstream 10.0.0.13:123
```

必须恰好三个 `-upstream`，且为字面 IPv4 `host:port`。参数说明：

| 参数 | 含义 |
|---|---|
| `-udp` | NTP 服务 UDP 监听地址 |
| `-http` | 状态查询 HTTP 监听地址 |
| `-poll` | 上游轮询周期 |
| `-timeout` | 单次 UDP 请求超时 |
| `-validity` | 样本有效期（单调时钟判定） |
| `-threshold` | 一致阈值（相对 offset 中位数） |

## 工作原理

- **轮询与校验**（`internal/ntp/poller.go`）：每周期对三个源并发发送 48 字节
  NTPv4（mode 3）请求。响应必须：版本 4、mode 4、stratum 1–14、LI≠3，
  且 Originate 与本请求 Transmit 完全一致（迟到/重复/外来包直接拒绝并记录原因）。
- **样本过滤**（`internal/ntp/source.go`）：四时间戳计算
  `offset=((t2-t1)+(t3-t4))/2`、`delay=(t4-t1)-(t3-t2)`，拒绝负 delay 与
  零/乱序时间戳。每源保留最近 8 个有效样本；按单调时钟判断过期，
  新鲜样本中取 delay 最小者（同值取最新）作为候选。
- **选源**（`internal/ntp/select.go`）：三源均有新鲜候选时，以 offset 中位数
  为中心保留阈值内的源，至少两源一致才同步；一致源中 delay 最小者授时
  （同值按配置顺序）。条件失效立即转为未同步，**不沿用旧偏移**。
- **虚拟时钟**（`internal/ntp/clock.go`）：仅保存内存偏移，`Now()=本地时间+偏移`。
- **应答服务**（`internal/ntp/server.go`）：接收 mode 3，返回 mode 4，原样复制
  Originate。同步时收发时间戳用虚拟时间，stratum 为选中源 +1；
  未同步返回 LI=3、stratum=16，绝不冒充可靠时间。
- **HTTP 状态**（`internal/ntp/http.go`）：`GET /status` 返回各源样本、offset、
  delay、拒绝原因、当前选择与同步状态，与 UDP 行为一致；`GET /healthz` 探活。

## 本地演示

终端 1–3 启动本机上游（第三个为失准源）：

```sh
go run ./examples/upstream -addr 127.0.0.1:11231 -offset 100ms -stratum 1
go run ./examples/upstream -addr 127.0.0.1:11232 -offset 110ms -stratum 2
go run ./examples/upstream -addr 127.0.0.1:11233 -offset 5s   -stratum 1
```

终端 4 启动服务并查询：

```sh
go run ./cmd/ntpd -udp 127.0.0.1:1230 -http 127.0.0.1:8080 -poll 1s \
  -upstream 127.0.0.1:11231 -upstream 127.0.0.1:11232 -upstream 127.0.0.1:11233
curl -s 127.0.0.1:8080/status        # agreeing:[0,1]，+5s 源被排除
go run ./examples/client -server 127.0.0.1:1230
```

`examples/upstream` 还支持 `-li 3`、`-mode`、`-stratum` 模拟各类非法上游，
其拒绝原因会出现在 /status 的 `rejections` 中。

## 范围说明

- 仅进程内虚拟偏移，不调用任何系统时钟调整接口；容器/无权限环境可安全运行。
- 仅支持 UDP/IPv4 的 NTPv4 客户端/服务器模式（mode 3/4），不实现广播、
  对称模式、认证（Autokey/NTS）与 Kiss-o'-Death。
- 时钟偏移为内存状态，进程重启后需重新同步；未同步期间对客户端明确返回
  LI=3 / stratum 16。
- 关闭时（SIGINT/SIGTERM）回收轮询 goroutine、释放 UDP 端口并优雅关闭 HTTP。
