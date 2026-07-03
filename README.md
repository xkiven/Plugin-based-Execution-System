# 插件化执行系统

这是一个使用 Go 实现的插件化执行系统。主程序不直接依赖具体业务逻辑，而是通过扫描插件目录、读取插件清单、按照统一协议调用插件进程，并最终汇总执行结果。

该实现覆盖了题目中的必选要求，并补充了若干进阶能力：

- 插件依赖与最低版本约束
- Host / Plugin API 版本兼容性校验
- 失败隔离与降级兜底
- 面向热加载 / 热卸载的轮询监控模式
- 对多语言插件友好的运行时模型
- 单插件超时控制

## 一、整体架构设计

系统由以下几个核心部分组成：

- `Host CLI`
- `Plugin Manifest`
- `External Process Execution`
- `stdin/stdout JSON Protocol`

### 为什么这样设计

- `Go plugin` 原生方案在 Windows 上支持较差，不适合作为通用交付方案。
- 插件以独立进程运行，异常、崩溃不会直接拖垮主程序。
- 插件可以独立开发、独立发布，主程序与业务实现彻底解耦。
- 统一的 JSON 协议便于后续接入 Python、Node.js 等其他语言插件。

换句话说，这个系统不是把“业务代码动态 import 到主程序里”，而是把“插件当作可独立运行的能力单元”来管理和调度。这样更符合长期演进和工程隔离的目标。

## 二、目录结构

```text
.
|-- cmd/system/               # CLI 入口
|-- internal/plugins/         # 插件发现、校验、状态管理、执行、监控
|-- internal/samplebuild/     # 示例插件构建辅助逻辑
|-- sampleplugins/            # 示例插件源码
|-- sdk/                      # 插件接口与 JSON 协议
|-- plugins/                  # 运行期插件目录，由 build-samples 生成
|-- sample-input.json         # 示例输入
|-- go.mod
`-- README.md
```

## 三、核心模块说明

- `cmd/system`
  - 提供命令：`build-samples`、`list`、`enable`、`disable`、`watch`、`run`
- `internal/plugins/types.go`
  - 定义插件清单、运行时配置、依赖、状态、执行结果等领域模型
- `internal/plugins/manager.go`
  - 负责插件发现、清单校验、依赖检查、执行编排、失败降级
- `internal/plugins/watch.go`
  - 提供基于轮询的监控能力，模拟热加载 / 热卸载
- `internal/plugins/store.go`
  - 负责持久化 `plugins/state.json`
- `sdk/protocol.go`
  - 定义主程序与插件之间共享的协议和 SDK 辅助封装

## 四、插件规范设计

插件实现遵循统一接口：

```go
type Plugin interface {
    Name() string
    Version() string
    Run(ctx context.Context, data map[string]any) (map[string]any, error)
}
```

SDK 会完成以下工作：

- 从 `stdin` 读取请求
- 反序列化输入数据
- 调用插件的 `Run`
- 将结果编码为 JSON 写回 `stdout`

这样主程序无需感知插件内部逻辑，只需要按照约定协议调度插件即可。

## 五、插件加载机制

系统从 `plugins/` 目录发现插件。每个插件目录至少包含一个 `plugin.json` 清单文件，用于描述插件元信息和运行方式。

示例：

```json
{
  "name": "audit",
  "version": "1.0.0",
  "api_version": "1.0.0",
  "entry": "audit.exe",
  "enabled": true,
  "dependencies": [
    {
      "name": "echo",
      "min_version": "1.0.0"
    }
  ],
  "failure_policy": {
    "use_fallback": false
  }
}
```

扩展字段说明：

- `api_version`
  - 插件声明自己遵循的协议版本
- `compatible_host_versions`
  - 可选，限制允许接入的 Host 版本
- `dependencies`
  - 声明插件依赖的其他插件及最低版本
- `failure_policy`
  - 声明执行失败时是否走兜底输出
- `runtime`
  - 可选，支持非二进制插件的运行方式配置

### 加载流程

1. 扫描 `plugins/` 目录
2. 读取每个插件的 `plugin.json`
3. 校验基础字段是否合法
4. 合并运行时状态覆盖项
5. 检查 API 兼容性、依赖关系和版本约束
6. 生成插件运行视图并交给执行器

## 六、插件管理能力

本系统已实现题目要求中的基础管理能力：

- 支持插件启用 / 禁用
- 支持查看插件名称、版本、状态等元信息
- 插件运行异常不会导致主程序崩溃
- 插件执行状态可记录并持久化

### 状态设计

发现 / 列表阶段的状态：

- `enabled`
- `disabled`
- `error`

执行阶段的结果状态：

- `success`
- `error`
- `degraded`

这里将“插件可被发现但当前不可安全执行”和“插件执行后失败”区分开，有利于后续扩展运维能力和问题定位能力。

## 七、执行流程设计

执行 `run` 命令时，系统会：

1. 重新扫描 `plugins/`
2. 从 `plugins/state.json` 读取启用 / 禁用覆盖项
3. 校验元信息、运行配置、API 版本和依赖关系
4. 过滤出已启用且健康的插件
5. 并发启动插件进程执行
6. 对每个插件施加独立超时控制
7. 汇总输出、错误、耗时、时间戳等结果
8. 持久化最近一次执行状态

### 并发策略

当前实现采用“对健康且启用的插件并发执行”的方式。

这样做的优点：

- 插件之间互不阻塞，整体吞吐更高
- 主程序调度逻辑简单清晰
- 适合大多数无强顺序依赖的处理场景

需要说明的是：如果未来要支持严格依赖执行链，可以在当前模型上继续扩展为“按依赖拓扑排序后分层并发执行”。

## 八、关键实现选择与取舍

## 1. 为什么不用 Go 原生 `plugin`

- Windows 支持不足
- 与编译环境、平台耦合较强
- 不利于多语言扩展

采用独立进程 + 协议通信的代价是：

- 进程启动有额外开销
- 需要进行序列化 / 反序列化

但对于这类面试题场景以及长期演进目标，这个取舍更合理。

## 2. 为什么状态不直接写回插件清单

运行时状态存储在 `plugins/state.json`，而不是修改 `plugin.json`。

这样做的好处：

- 插件发布内容和运行期操作分离
- 插件升级不会覆盖启用 / 禁用状态
- 更接近真实生产系统中的配置与状态分层思路

## 3. 为什么 `watch` 采用轮询而不是文件系统事件

- 不引入额外第三方依赖
- 跨环境行为更稳定
- 足以证明热加载 / 热卸载的核心思路

代价是：

- 变更不是即时感知


## 九、已实现的进阶能力

### 1. 插件依赖与版本约束

加载阶段会校验：

- 依赖插件是否存在
- 依赖插件是否启用
- 依赖插件版本是否满足 `min_version`

若不满足，插件状态会被标记为 `error`，避免非法组合进入运行期。

### 2. Host / Plugin API 版本兼容检查

Host 当前暴露 `HostAPIVersion = "1.0.0"`。

插件可通过以下字段声明兼容关系：

- `api_version`
- `compatible_host_versions`

如果协议版本不匹配，插件会在发现阶段被拒绝，而不是拖到运行期失败。

### 3. 失败隔离与降级

插件本身已经运行在独立进程中，本实现进一步增加了失败降级能力：

- 插件执行失败或超时时，Host 会捕获错误
- 若配置了 `failure_policy.use_fallback`，则返回兜底输出
- 此时执行状态记为 `degraded`，而不是 `error`

这类设计适合“允许部分能力降级，但不希望整体请求硬失败”的业务场景。

### 4. 面向热加载 / 热卸载的监控模式

`watch` 命令会按固定间隔重新扫描插件目录，并输出以下事件：

- `loaded`
- `updated`
- `unloaded`

它不是完整的守护进程式热更新系统，但已经体现了插件动态发现与状态刷新机制。

### 5. 多语言插件运行模型

系统支持两种运行模式：

- `executable`
  - 直接执行插件入口文件
- `command`
  - 使用指定命令启动插件，并将入口文件作为参数传入

例如，一个 Python 插件可以这样声明：

```json
{
  "name": "py-transform",
  "version": "1.0.0",
  "entry": "main.py",
  "runtime": {
    "type": "command",
    "command": "python",
    "args": ["-u"]
  }
}
```

Host 最终执行的命令为：

```text
python -u main.py
```

这意味着当前实现虽然主体用 Go 编写，但架构上已经具备跨语言扩展能力。

### 6. 单插件超时控制

每个插件执行时都可以施加独立超时时间。若超时：

- Host 会终止对应插件进程
- 记录超时错误
- 根据失败策略决定是返回 `error` 还是 `degraded`

这可以避免单个异常插件无限阻塞整个执行链路。

## 十、示例插件

仓库中提供了 4 个示例插件：

- `echo`
  - 原样返回输入数据
- `stats`
  - 对输入做简单统计
- `audit`
  - 依赖 `echo`，用于演示依赖校验
- `unstable`
  - 固定失败，用于演示失败降级

## 十一、命令使用方式

构建示例插件：

```bash
go run -buildvcs=false ./cmd/system build-samples
```

查看插件列表：

```bash
go run -buildvcs=false ./cmd/system list
```

启用 / 禁用插件：

```bash
go run -buildvcs=false ./cmd/system disable echo
go run -buildvcs=false ./cmd/system enable echo
```

执行所有已启用插件：

```bash
go run -buildvcs=false ./cmd/system run -input sample-input.json
```

指定超时时间执行：

```bash
go run -buildvcs=false ./cmd/system run -input sample-input.json -timeout 2s
```

监控插件目录变化：

```bash
go run -buildvcs=false ./cmd/system watch -interval 2s
```

## 十二、示例行为说明

- 当 `echo` 被禁用时，依赖它的 `audit` 会变成 `error`
- 当 `unstable` 执行失败时，会返回 fallback 输出，状态为 `degraded`
- 当某个插件执行超时时，Host 会终止该插件并返回超时信息

## 十三、测试说明

当前测试覆盖了以下关键场景：

- 启用 / 禁用覆盖项优先级
- 插件依赖校验
- 失败策略的降级行为

运行测试：

```bash
go test -buildvcs=false ./...
```

### CI

项目已配置 GitHub Actions，在以下场景自动执行测试：

- 向 `main` 或 `master` 分支推送代码时
- 提交 Pull Request 时

CI 文件位置：

```text
.github/workflows/ci.yml
```

## 十四、未完成部分与后续可扩展方向

如果继续向生产级系统演进，下一步可以考虑：

1. 基于事件的真实文件监控，而不是轮询
2. 插件进程的资源隔离，如 CPU / 内存限制
3. 重试机制与熔断策略
4. 依赖图排序与循环依赖检测
5. 多语言 SDK 的进一步抽象与发布
6. 结构化日志、指标采集与可观测性建设

这些内容并不是本次面试题的必做项，但当前架构已经为这些演进方向预留了空间。

## 十五、第三方库说明

本项目未使用第三方库，全部基于 Go 标准库实现。
