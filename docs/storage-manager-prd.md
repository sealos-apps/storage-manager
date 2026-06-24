# Storage Manager 产品需求文档

## 问题陈述

Sealos 用户需要一种安全的自助方式，在 Sealos Desktop 工作流内查看和管理 Kubernetes PersistentVolumeClaim。当前常见的存储操作通常依赖 Kubernetes 知识、直接集群凭据或管理员介入。对于查看 PVC、打开文件、创建卷、扩容容量、理解卷无法打开的原因等日常任务，这会带来明显摩擦。

集群管理员也需要一个受控的存储管理界面。管理员需要跨命名空间可见性、StorageClass 生命周期管理工具，以及足够的诊断信息来支持用户，同时需要避免把 kubeconfig、File Browser token、Kubernetes 内部细节和其他敏感信息暴露到浏览器或提交到仓库。

产品必须适配自托管 Encore 部署，保持本地开发对离线环境友好，并遵守 Kubernetes 授权、存储拓扑和 PVC 访问模式规则。

## 解决方案

Storage Manager 提供一个 Sealos Desktop 存储应用，由 typed Encore API 和 React 前端支撑。用户可以查看自己命名空间内的 PVC，检查容量和挂载状态，在策略允许时创建或扩容卷，并为受支持的 PVC 打开临时 File Browser 会话。

文件访问流程由后端控制。后端验证调用者身份，创建或复用挂载目标 PVC 的短生命周期 viewer pod，通过 hook 校验的登录流程签发短生命周期 File Browser token，通过 heartbeat 刷新会话活跃状态，并清理过期或关闭的会话。对于已被挂载的 ReadWriteOnce 卷，后端通过 Kubernetes pod 挂载检测和调度提示判断 viewer 访问是否可用、是否只读、是否需要绑定到特定节点。

管理员侧提供命名空间选择、聚合 PVC 可见性和 StorageClass 管理能力。管理员可以列出命名空间，按授权跨命名空间管理 PVC，查看 StorageClass 元数据，创建或更新 StorageClass YAML，查看 kubectl 风格的描述信息，并且只能删除由 Storage Manager 管理且未被使用的 StorageClass。

系统通过本地和部署期 YAML 配置，复用现有可观测性组件提供日志、指标和可选 tracing，并保持业务 API 全部 typed，确保 OpenAPI 与 Encore TypeScript client 生成稳定。

## 用户故事

1. 作为 Sealos 用户，我希望看到由当前凭据解析出的命名空间，以便确认自己正在管理哪些存储资源。
2. 作为 Sealos 用户，我希望列出自己命名空间内的 PVC，以便找到需要操作的卷。
3. 作为 Sealos 用户，我希望每个 PVC 行展示名称、命名空间、容量、访问模式、StorageClass、挂载状态和 viewer 支持状态，以便快速选择正确操作。
4. 作为 Sealos 用户，我希望手动刷新存储数据，以便看到 Kubernetes 或其他工作流产生的最新变化。
5. 作为 Sealos 用户，我希望在可用时看到 PVC 使用量指标，以便了解已用空间和剩余空间。
6. 作为 Sealos 用户，我希望 Storage Manager 说明使用量指标不可用的原因，以便避免把缺失指标误解为空数据。
7. 作为 Sealos 用户，我希望通过 UI 创建 PVC，以便无需编写 Kubernetes YAML 就能申请存储。
8. 作为 Sealos 用户，我希望创建 PVC 时必须填写有效名称、容量、访问模式和 StorageClass，以便无效请求在进入 Kubernetes 前被拦截。
9. 作为 Sealos 用户，我希望创建 PVC 时看到命名空间配额，以便选择符合可用额度的容量。
10. 作为 Sealos 用户，我希望 PVC 创建能力在功能关闭时被隐藏或禁用，以便 UI 与平台策略一致。
11. 作为 Sealos 用户，我希望在 StorageClass 支持扩容时扩容 PVC，以便无需重建数据就能增加容量。
12. 作为 Sealos 用户，我希望扩容请求必须大于当前容量，以便阻止误操作、无效操作和缩容请求。
13. 作为 Sealos 用户，我希望启用配额时扩容请求会检查额度，以便避免提交超过账户限制的请求。
14. 作为 Sealos 用户，我希望删除未使用的 PVC 前需要明确确认，以便降低误删数据的风险。
15. 作为 Sealos 用户，我希望 PVC 被活跃 pod 挂载时阻止删除，以便保护运行中的工作负载。
16. 作为 Sealos 用户，我希望为受支持的 PVC 打开文件，以便在浏览器内查看或管理卷内容。
17. 作为 Sealos 用户，我希望打开 PVC 时 Storage Manager 自动创建临时 viewer 会话，以便无需理解 viewer pod 的实现细节。
18. 作为 Sealos 用户，我希望 viewer pod 启动时看到会话进度，以便确认系统正在处理。
19. 作为 Sealos 用户，我希望只有 viewer 会话 ready 且 token 已签发后才显示文件视图，以便直接文件访问从有效状态开始。
20. 作为 Sealos 用户，我希望 Kubernetes 语义允许时获得读写访问，以便创建、重命名、修改、上传和删除文件。
21. 作为 Sealos 用户，我希望写访问不安全或不受支持时仍可只读访问，以便继续检查文件且避免数据风险。
22. 作为 Sealos 用户，我希望不受支持的 PVC 展示原因，以便理解问题来自访问模式、挂载冲突、pod 调度还是功能策略。
23. 作为 Sealos 用户，我希望已挂载的 ReadWriteOnce 卷遵守节点调度约束，以便 viewer pod 在需要时安全地挂载到同一节点。
24. 作为 Sealos 用户，我希望 Storage Manager 检测挂载冲突，以便避免创建不安全的 viewer pod。
25. 作为 Sealos 用户，我希望 File Browser token 生命周期较短，以便限制文件访问暴露窗口。
26. 作为 Sealos 用户，我希望 heartbeat 保持活跃 viewer 会话，以便正常浏览不会被中断。
27. 作为 Sealos 用户，我希望不活跃 viewer 会话自动过期，以便 viewer pod 和 token 不会无限期存在。
28. 作为 Sealos 用户，我希望手动关闭文件会话，以便完成操作后释放 viewer 资源。
29. 作为 Sealos 用户，我希望离开页面时应用尽量关闭 viewer 会话，以便快速清理未使用会话。
30. 作为 Sealos 用户，我希望存在活跃上传时延迟自动关闭行为，以便上传不被页面生命周期事件中断。
31. 作为 Sealos 用户，我希望轮询或 heartbeat 短暂失败时可以恢复会话，以便临时网络问题不会迫使我重启工作。
32. 作为 Sealos 用户，我希望提供刷新会话操作，以便从过期或失败的 viewer 流程中恢复。
33. 作为 Sealos 用户，我希望文件浏览、下载、上传、创建文件夹、编辑文件、复制、移动和删除都通过活跃 token 后面的 File Browser API 完成，以便文件操作始终被限制在当前挂载 PVC 内。
34. 作为 Sealos 用户，我希望在文件会话支持时访问回收站视图，以便删除文件相关流程可被发现。
35. 作为 Sealos 用户，我希望看到带后端错误详情的本地化错误信息，以便用偏好的语言理解失败原因。
36. 作为 Sealos 用户，我希望 file management 关闭时隐藏文件管理导航，以便被关闭的功能不会显示为可操作入口。
37. 作为 Sealos 管理员，我希望看到自己是否可以管理 PVC、管理 StorageClass、创建 PVC 和使用 file management，以便 UI 只展示已授权工作流。
38. 作为 Sealos 管理员，我希望从可访问命名空间列表中选择命名空间，以便跨命名空间支持用户。
39. 作为 Sealos 管理员，我希望查看全命名空间 PVC 视图，以便在一个界面审计集群存储。
40. 作为 Sealos 管理员，我希望在全命名空间视图中为选定命名空间创建 PVC，以便为目标用户命名空间配置存储。
41. 作为 Sealos 管理员，我希望管理员操作使用 Kubernetes 授权检查，以便 UI 权限不会绕过集群策略。
42. 作为 Sealos 管理员，我希望列出 StorageClass 的 provisioner、默认标记、扩容支持、绑定模式、回收策略、归属、创建时间和使用中的 PVC 数量，以便安全管理存储规格。
43. 作为 Sealos 管理员，我希望从 YAML 创建 StorageClass，以便精确配置高级存储能力。
44. 作为 Sealos 管理员，我希望 StorageClass YAML 校验 Kubernetes kind、version、name 和集群级 metadata，以便清晰拒绝无效 manifest。
45. 作为 Sealos 管理员，我希望由 Storage Manager 创建的 StorageClass 被标记为 Storage Manager 管理，以便资源归属明确。
46. 作为 Sealos 管理员，我希望编辑 StorageClass YAML 时移除服务端管理字段，以便更新聚焦在可编辑配置。
47. 作为 Sealos 管理员，我希望更新冲突提示我重新加载后再重试，以便避免覆盖并发变更。
48. 作为 Sealos 管理员，我希望看到 kubectl 风格的 StorageClass 描述，以便无需打开终端也能诊断参数和行为。
49. 作为 Sealos 管理员，我希望阻止删除非 Storage Manager 管理的 StorageClass，以便 Storage Manager 只移除自己拥有的资源。
50. 作为 Sealos 管理员，我希望阻止删除仍被 PVC 引用的 StorageClass，以便现有卷保持有效。
51. 作为平台运维，我希望 Storage Manager 使用自托管 Encore CLI 和本地配置运行，以便开发和部署不依赖 Encore Cloud 登录。
52. 作为平台运维，我希望提交的示例配置和部署配置展示所有必要业务设置，以便各环境可以一致配置。
53. 作为平台运维，我希望本地 kubeconfig 和业务配置被 git 忽略，以便凭据不会被提交。
54. 作为平台运维，我希望 File Browser hook 校验使用内部 bearer token 和一次性 auth request，以便 File Browser 登录由后端控制。
55. 作为平台运维，我希望 File Browser bearer token 在后端状态中只保存 hash，以便降低内存暴露风险。
56. 作为平台运维，我希望 token 响应设置 no-cache header，以便客户端和中间代理不会缓存文件访问 token。
57. 作为平台运维，我希望会话状态保存在有边界的 TTL cache 中，以便 MVP 保持简单且内存使用受控。
58. 作为平台运维，我希望孤儿 viewer pod 可以被反向同步和清理，以便后端重启或 cache 淘汰后不会永久泄漏资源。
59. 作为平台运维，我希望内存态 MVP 明确要求单副本或 sticky routing，以便多副本部署被诚实配置。
60. 作为平台运维，我希望 Kubernetes 调用、File Browser 调用、会话生命周期、清理和大规模扫描有可观测性，以便生产问题可被调试。
61. 作为前端开发者，我希望所有后端调用都经过 Encore 生成 client 边界和 feature API adapter，以便请求形状保持 typed。
62. 作为后端开发者，我希望所有业务 endpoint 都是 typed Encore API，以便 OpenAPI 和 TypeScript client 生成稳定。
63. 作为后端开发者，我希望 raw endpoint 限定为 Prometheus metrics，以便业务 API 保持 schema。
64. 作为安全审查者，我希望 kubeconfig、token、auth header 和 File Browser token 排除在日志和 trace 之外，以便敏感信息不进入可观测性数据。
65. 作为支持工程师，我希望 PVC、StorageClass、auth、viewer、hook、quota 和内部失败都有结构化错误码，以便客户端行为和支持文档可以依赖稳定分类。

## 实现决策

- 后端继续作为 Encore.go 服务运行，所有业务操作使用 typed public API。请求和响应结构显式声明 body、path、query 和 header tag。
- raw endpoint 只保留 Prometheus 文本指标。所有产品操作使用 typed Encore API 和生成的 client schema。
- API 面覆盖 context、PVC list/create/delete/expand、storage quota、StorageClass list、admin capabilities、admin namespaces、admin StorageClass CRUD/describe、viewer session create/get/token/heartbeat/close、pod session get/close 和 File Browser hook verification。
- Endpoint handler 保持轻量。授权、Kubernetes 交互、会话生命周期、StorageClass 行为、配额查询和 File Browser 交互放在 service interface 后面。
- 每条请求路径都把传入的 context 传递到 handler 和 service collaborator。
- 用户授权基于调用者的 Sealos 或 Kubernetes bearer token。管理员提权是显式流程，并通过 admin authorization check 执行。
- 管理员命名空间选择使用应用级 all-namespaces token 做聚合，同时保留对调用者的授权检查。
- PVC 可见性包括挂载检测、已挂载 pod 明细、viewer 支持状态、viewer 模式、调度提示和可选文件系统使用量指标。
- PVC 创建和扩容同时接收 Kubernetes quantity 字符串和字节数。后端校验一致性、容量方向、StorageClass 支持和启用配额时的额度。
- PVC 删除会校验调用者可见性，并在活跃 pod 仍挂载该 PVC 时阻止删除。
- Viewer session 是用户侧会话，背后由 pod session 支撑。安全时，同一 PVC 的多个 viewer session 可以共享一个 pod session。
- Pod session 表示 Kubernetes 资源，包括 viewer pod、service、public URL、internal URL、runtime version、节点调度、状态、原因、活跃时间和过期时间。
- Viewer session 表示用户状态，包括 ID、pod session ID、namespace、PVC name、status、pod status、viewer URL、mode、token readiness、heartbeat 和过期时间。
- File Browser 登录流程使用一次性 auth request。后端签发 username 和 password secret，File Browser 运行配置的 hook，hook 调用后端校验，File Browser 根据后端决策授予权限。
- File Browser 权限由 viewer mode 映射为只读或读写能力。命令执行、分享和管理员权限保持关闭。
- Viewer token 响应包含 no-cache header，并且只在当前浏览器会话需要的响应体中暴露 bearer token。
- Token record 只存储 token hash，不保存明文 token。
- 会话状态使用有边界的内存 store，覆盖 pod session、viewer session、auth request、token record 和二级索引。TTL 和 purge interval 可配置。
- MVP 部署假设 viewer/session 操作使用单个后端副本或 sticky routing，因为会话状态在内存中。
- Cleanup 通过 Kubernetes 状态同步过期会话和孤儿 viewer pod，避免后端重启和 cache 淘汰造成长期资源泄漏。
- Runtime 行为通过本地 `viewer.yaml` 和 Helm/ConfigMap 示例中的部署期配置管理。敏感本地文件保持 git ignore。
- 可观测性使用现有 recorder 抽象。关键 Kubernetes、File Browser、auth/session lifecycle、cleanup、reconciliation、quota 和 metrics 操作记录结构化日志、计数器和可选 trace。
- 日志和 trace 只使用有边界且安全的属性，例如 operation name、namespace、resource kind、ID、count、status 和 result label。
- 前端代码保留在 Vite workspace 下。共享 UI primitive 保留在共享组件模块，viewer feature 自己拥有 feature API、hook、store、component 和 test。
- 前端通过 service adapter 和 feature API module 使用 Encore 生成 TypeScript client。深层 UI component 接收 typed feature API，不直接构造后端路径。
- TanStack Query option factory 负责 query key、enabled gate、polling interval、invalidation、乐观 heartbeat 更新和 mutation side effect。
- UI 根据后端 capability 派生导航。PVC 管理、file management、namespace selection 和 StorageClass administration 按 capability response 展示。
- File manager 使用从活跃 viewer token 和 viewer URL 初始化的 File Browser client，并被限定到所选 PVC 会话。
- 应用在可恢复状态下保留最后一个有效 file session，以便用户仍可看到文件列表上下文；手动关闭或不兼容导航时清理该状态。
- 浏览器生命周期处理会尽量关闭 viewer session，并考虑活跃上传。
- 国际化错误渲染把稳定后端错误码映射到本地化消息，并在可用时展示安全的 detail 文本。

## 测试决策

- 测试应断言外部行为和公开契约，包括 API 响应、授权决策、状态转换、query option、UI 行为和生成 schema 假设。除非调用顺序本身是契约，否则不应断言私有 helper 调用顺序。
- 后端 handler 测试应覆盖请求解析、授权模式、命名空间解析、service 输入、响应 envelope、token no-cache header 和错误码映射。
- 后端 service 测试应覆盖 PVC 列表、挂载检测、viewer 支持决策、PVC create/delete/expand 校验、配额检查、viewer session 生命周期、File Browser hook verification、token 签发、heartbeat、关闭行为、cache 过期、cleanup 和孤儿资源同步。
- Kubernetes 相关测试使用 fake clientset 做单元覆盖，真实集群检查通过显式 integration config 执行。
- StorageClass 测试应覆盖 list metadata、YAML sanitization、create/update validation、managed ownership label、conflict handling、describe output、delete restriction 和 in-use PVC count。
- Config 测试应覆盖 default、YAML parsing、validation error、redaction、example config，以及产品用到的 deploy config value。
- 可观测性测试应在可行时通过现有 recorder abstraction 验证 operation boundary 和 metrics 行为，同时确保 secret 字段不会进入断言对象。
- 前端 API 测试应覆盖 query key、enabled gate、polling、mutation input mapping、cache write、invalidation、乐观 heartbeat 更新和 rollback 行为。
- 前端工作流测试应使用带 provider 的 render helper，覆盖 context loading、namespace selection、capability-gated navigation、PVC create/expand/delete dialog、viewer launch、session recovery、manual close 和 StorageClass admin dialog。
- File manager 测试应覆盖 File Browser client request 构造、path encoding、upload strategy selection、error normalization 和 session-scoped behavior。
- 布局和 Chrome 86 兼容性声明需要浏览器或构建产物验证。jsdom 测试只足够覆盖行为，不足以证明视觉兼容性。
- 跨后端和前端的改动在交付前需要运行 `go mod tidy`（仅 imports 或 dependencies 变化时）、`make verify`、`encore check` 和 `git diff --check`。
- 仅后端改动可以先使用后端 Makefile target 做局部验证，再在发布或合并前运行更完整的验证。
- 仅前端改动可以先使用 web Makefile target 做局部验证；涉及 Chrome 兼容性时需要包含 build 和 CSS compatibility check。

## 范围外

- 把内存态 state store 替换为数据库支撑的多副本 session store。
- 重新引入独立 Go `cmd/` 开发服务器。
- 要求本地开发依赖 Encore Cloud 登录、Encore Cloud app identity 或 Encore MCP。
- 在 Encore SDK 可以表达 endpoint 时手写前端后端请求路径。
- 添加 OTel metrics 或 OTel logs。
- 把 kubeconfig 内容、原始 auth header、File Browser bearer token 或 hook secret 暴露到日志、trace、前端状态快照、文档示例或提交文件中。
- 支持 StorageClass YAML 工作流之外的任意 Kubernetes 对象编辑。
- 对非 Storage Manager 管理的 StorageClass 执行读/describe 可见性和安全删除阻止说明之外的管理操作。
- 支持 Chrome 86 以下浏览器目标。
- 构建持久审计日志、审批工作流或角色管理 UI。
- 构建面向 pod 或非 PVC-backed volume 的通用 Kubernetes 文件管理器。
- 在缺少 sticky routing 或持久状态设计的情况下支持生产多副本运行。

## 补充说明

- 本 PRD 基于当前仓库状态和安装在项目 `.agents/skills` 目录下的本地 `to-prd` skill 生成。
- 仓库未暴露 issue tracker 配置或 triage label vocabulary，因此本 PRD 以本地项目文档形式发布。skill 模板期望的 triage label 是 `ready-for-agent`。
- 当前实现已经包含大量产品能力。后续工作可以把这份 PRD 作为完善、加固或审查 Storage Manager 的统一需求和验收依据。
- 生成和修改本文档时保留了现有用户工作区改动。
