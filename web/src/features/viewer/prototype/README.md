# 存储类型配置原型

本原型直接复用并扩展项目现有 `StorageAppShell`、`StorageClassAdminView`、`StorageClassMetadataDialog`、shadcn primitives、TanStack Form / Query 及中英文资源，不维护另一套页面或 CSS。

## 运行

从仓库根目录运行 `make web-install`（使用兼容当前 lockfile 的 PNPM 10），然后在 `web/` 运行 `pnpm dev --host 127.0.0.1 --port 4397`。
打开 http://127.0.0.1:4397/storage-policy-prototype.html 。不需要配置 kubeconfig 或启动后端。若 SDK 尚未生成，先运行 `make web-generate-api`。

## 评审路径

- 原有存储类型表格新增“用户可见性”和原型“存储属性”列；保留名称、展示名、Provisioner、回收策略、绑定模式、扩容、PVC 数量及原有操作入口。
- “nfs-team → 编辑”：沿用多语言展示名表单；选择共享 / 独占并勾选“开放给普通用户使用”，保存后通过现有 mutation / cache 流程更新列表。
- “csi-rbd-sc → 编辑”：取消勾选开放并保存，查看隐藏二次确认。取消确认返回编辑，不修改已保存配置；确认后仅可见性变化。
- “csi-archive → 编辑”：初始为待配置且仅管理员可见，必须补齐展示名称和存储属性才能保存。
- 使用中且已有属性的类型暂时禁用属性选择，这是待产品确认的原型建议。

## 数据与边界

`storage-policy-api.ts` 使用本地内存实现项目的 ViewerAPI 接口。保存仅更改本页示例数据，刷新浏览器恢复初始值；预览不发起集群请求。既有 Describe / YAML 入口显示示例信息，创建、删除和 YAML 写入明确提示不在演示范围，不会伪造成功。

`ViewerAPI.storageModePrototype` 是注入适配器的前端演示标记，默认不启用，仅此入口传入 true；不是环境变量或部署配置。它控制现有列表和弹窗的原型属性字段。正式 Encore API 的字段未改变；真实适配器拒绝带有 `storageMode` 的请求，避免将未接通的属性静默丢弃后报告成功。普通入口保留原有 API 流程。

共享 / 独占不是 PVC accessModes；已有类型初始化、使用中属性变更策略、名称长度/重名以及混合挂载仍待确认。没有修改后端、部署配置或 AppLaunchpad / Devbox。

该 HTML 是 Vite 开发预览入口，默认生产构建只打包原有 index.html，不包含示例数据入口。正式功能落地时应移除演示适配器，按确定后的 API 契约接入。

## 验证

`make web-verify` 覆盖 lint、236 项测试、类型检查和生产构建；新增集成测试验证原页面保存、列表刷新、隐藏确认/取消、必填校验、草稿保留、属性锁定及示例数据隔离。浏览器截图位于工作区 `.context/storage-project-*.png`，需求文档第 10 节已更新为项目页面截图。
