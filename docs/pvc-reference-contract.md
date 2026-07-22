# PVC Reference Contract

## 目标

Storage Manager 在删除 PVC 前需要识别“没有活跃 Pod，但仍被应用或 DevBox 声明引用”的情况。引用关系是多对多关系：

- 一个 PVC 可以被多个 AppLaunchpad 应用或 DevBox 引用。
- 一个 AppLaunchpad 应用或 DevBox 也可以同时引用多个 PVC。

因此引用关系必须由引用方声明，不能写成 PVC 上的单个 owner/consumer 字段。Storage Manager 只聚合这些声明和 Kubernetes 原生 workload 证据，并在删除 PVC 时做保护。

## 引用方声明

在引用 PVC 的资源上加统一 label 和 annotation。引用方可以是 DevBox CR、AppLaunchpad 的 Deployment/StatefulSet，或后续接入的其他资源。

```yaml
metadata:
  labels:
    storage.sealos.io/pvc-reference-source: "true"
    storage.sealos.io/ref-source: "devbox" # devbox | applaunchpad | ...
  annotations:
    storage.sealos.io/ref-name: "demo"
    storage.sealos.io/pvc-references: |
      [
        {"name":"shared-data","relation":"mounted","mountPath":"/data"},
        {"name":"shared-cache","relation":"mounted","mountPath":"/cache"}
      ]
```

字段约定：

- `storage.sealos.io/pvc-reference-source=true` 是 Storage Manager 的扫描入口。没有这个 label 的资源不会参与 PVC 删除保护。
- `storage.sealos.io/ref-source` 表示引用方来源，例如 `devbox` 或 `applaunchpad`。这是引用方类型，不是 `local`、`shared`、`generated` 这类 PVC 类型。
- `storage.sealos.io/ref-name` 是给用户看的引用方名称，缺省时使用 Kubernetes resource name。
- `storage.sealos.io/pvc-references` 是 JSON array。每一项表示当前资源引用的一个 PVC。
- `name` 是 PVC name。本版本只支持同 namespace 引用；声明了其他 namespace 的条目会被忽略。跨 namespace 需要重新设计授权和展示策略后再开启。
- `relation` 缺省为 `mounted`。StatefulSet `volumeClaimTemplates` 生成的 PVC 会被 Storage Manager 标记为 `owned`。
- `mountPath` 可选，用于提示用户这个 PVC 被挂在哪里。

引用变更时，只更新引用方资源的 annotation，不要反向 patch 被引用 PVC。

## Storage Manager 识别能力

Storage Manager 删除 PVC 前会阻止以下情况：

- 活跃 Pod 仍通过 `volumes[].persistentVolumeClaim.claimName` 挂载该 PVC，返回 `PVC_IN_USE`。
- 带 `storage.sealos.io/pvc-reference-source=true` 的 Deployment/StatefulSet/DevBox/App CR 声明了该 PVC，返回 `PVC_REFERENCED`。
- 带上述 label 的 Deployment/StatefulSet 在 pod template 的 `volumes[].persistentVolumeClaim.claimName` 中引用该 PVC，即使没有显式 `pvc-references` annotation，也会被识别。
- 带上述 label 的 StatefulSet 使用 `volumeClaimTemplates` 生成的 PVC，例如 `<templateName>-<statefulSetName>-<ordinal>`，会被识别为 `owned`。

列表页会 best-effort 展示 `references[]`；普通用户只看到自己 namespace 内的引用方资源，管理员可在允许的 namespace 范围内看到引用方详情。如果引用扫描暂时失败，会记录告警并继续展示 PVC。删除接口会 fail closed：引用扫描失败时不会继续删除。

## AppLaunchpad 接入建议

AppLaunchpad 优先在实际持有 PVC 引用的 Deployment/StatefulSet 上加这套 label/annotation。

- 共享/远程 PVC：写入 `storage.sealos.io/pvc-references`，每个挂载的已有 PVC 一条。
- 普通 workload volumes：即使 annotation 漏写，只要 workload 有扫描 label，Storage Manager 也会从 `spec.template.spec.volumes` 识别。
- StatefulSet local storage：保留扫描 label。Storage Manager 会根据 `volumeClaimTemplates` 和现存 PVC 名识别生成 PVC。
- 应用暂停时不要删除承载引用声明的 workload；只要 Deployment/StatefulSet 还在，PVC 删除保护仍然生效。

## DevBox 接入建议

DevBox 支持挂载已有共享 PVC 时，在 DevBox CR 上加这套 label/annotation。

- DevBox 引用多个 PVC 时，把每个 PVC 都写进 `storage.sealos.io/pvc-references`。
- 挂载路径变化、增删 PVC 时，只更新 DevBox CR 的 annotation。
- 不要依赖 PVC 上的 `cloud.sealos.io/devbox-manager` 作为完整引用模型；它可以作为历史 owner hint，但不能表达多个 DevBox/App 对同一个 PVC 的引用。
