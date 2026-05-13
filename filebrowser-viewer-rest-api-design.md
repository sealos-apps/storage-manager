# PVC File Browser Viewer REST API 与实现设计

## 1. 目标

后端提供一组 REST API，供前端创建 Viewer 会话、等待 Viewer Pod ready、获取 File Browser token、保持会话活跃、关闭会话或 Pod。

整体流程：

```text
Frontend
  -> Backend: 创建 Viewer Session
  -> Backend: 轮询 Pod/Session 状态
  -> Backend: 请求 File Browser token
  -> File Browser: 使用 token 调文件管理 API
  -> Backend: heartbeat / close
```

概念：

```text
Pod Session
  对应一个 Viewer Pod。
  一个 PVC 同一时间通常只有一个 active Pod Session。

Viewer Session
  对应一个用户窗口或一次前端连接。
  多个 Viewer Session 可以复用同一个 Pod Session。
```

## 2. 资源模型

本设计只使用后端进程内的内存 LRU Cache 作为状态存储，不引入数据库。

这意味着后端是有状态服务：

```text
Pod Session
Viewer Session
Auth Request
File Browser token hash
Pod 与 Session 的映射
```

都保存在内存中。实现简单，但会带来两个约束：

1. 后端实例重启后内存状态丢失。
2. 多副本后端需要 sticky session 或按 PVC/PodSession 路由到同一实例；MVP 建议单副本运行。

因此后端必须能从 Kubernetes 资源反向同步 Viewer Pod 状态，并清理失去内存引用的孤儿 Viewer Pod。

### 2.1 Pod Session

```go
type PodSession struct {
	ID          string
	Namespace   string
	PVCName     string
	PVCUID      string
	AccessMode  string // ReadWriteMany / ReadOnlyMany / ReadWriteOnce
	Mode        string // readwrite / readonly
	PodName     string
	ServiceName string
	ViewerURL   string
	Status      string // creating / ready / failed / terminating / terminated
	Reason      string
	NodeName    string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	LastActiveAt time.Time
	ExpiresAt    time.Time
}
```

### 2.2 Viewer Session

```go
type ViewerSession struct {
	ID            string
	PodSessionID  string
	UserID        string
	Username      string // 传给 File Browser 的 USERNAME，建议直接使用 ViewerSession.ID
	Permission    string // readonly / readwrite
	Status        string // active / closed / expired / failed
	CreatedAt     time.Time
	LastHeartbeat time.Time
	ExpiresAt      time.Time
}
```

### 2.3 Auth Request

```go
type AuthRequest struct {
	ID              string
	ViewerSessionID string
	PodSessionID    string
	Username        string
	PasswordHash    string
	UsedAt          *time.Time
	ExpiresAt       time.Time
	CreatedAt       time.Time
}
```

`AuthRequest` 是后端调用 File Browser 登录前生成的一次性认证请求。

推荐：

```text
USERNAME = viewerSession.ID
PASSWORD = authRequest.ID + "." + randomSecret
```

hook 收到后拆分 `PASSWORD`，把 `authRequest.ID` 和 `randomSecret` 的 hash 发给后端验证。

### 2.4 LRU Cache 分组

后端维护几组独立 LRU Cache：

```go
type StateStore struct {
	PodSessions       *lru.Cache[string, *PodSession]
	ViewerSessions    *lru.Cache[string, *ViewerSession]
	AuthRequests      *lru.Cache[string, *AuthRequest]
	TokenRecords      *lru.Cache[string, *TokenRecord]
	PodSessionByPVC   *lru.Cache[string, string] // pvcUID -> podSessionID
	ViewerByPod       *lru.Cache[string, map[string]struct{}] // podSessionID -> viewerSessionIDs
}
```

`TokenRecord` 只保存 token hash，不保存明文 token：

```go
type TokenRecord struct {
	TokenHash       string
	ViewerSessionID string
	PodSessionID    string
	IssuedAt         time.Time
	ExpiresAt        time.Time
}
```

Cache key 建议：

```text
PodSessions: podSessionID
ViewerSessions: viewerSessionID
AuthRequests: authRequestID
TokenRecords: tokenHash
PodSessionByPVC: namespace + "/" + pvcUID
ViewerByPod: podSessionID
```

LRU Cache 必须支持：

```text
max entries
per-entry expiration
on-evict callback
periodic purge expired
```

如果使用的 LRU 库不支持 TTL，业务层需要在 value 中保存 `ExpiresAt`，每次读取时判断是否过期，并由后台任务定期清理。

## 3. REST API 总览

面向前端：

```text
GET    /api/pvcs
POST   /api/viewer-sessions
GET    /api/viewer-sessions/{viewerSessionID}
POST   /api/viewer-sessions/{viewerSessionID}/token
POST   /api/viewer-sessions/{viewerSessionID}/heartbeat
DELETE /api/viewer-sessions/{viewerSessionID}
DELETE /api/pod-sessions/{podSessionID}
```

面向 File Browser hook：

```text
POST /internal/filebrowser-hook/verify
```

可选调试/管理：

```text
GET /api/pod-sessions/{podSessionID}
```

## 4. API Envelope 与通用 DTO

### 4.1 Envelope 规则

请求体格式：

```json
{
  "...fields": "..."
}
```

成功响应格式：

```json
{
  "resource_type": {
    "...fields": "..."
  }
}
```

错误响应格式：

```json
{
  "error": {
    "code": "...",
    "message": "...",
    "details": {}
  }
}
```

约定：

```text
resource_type 使用 snake_case 单数或集合名。
列表响应使用 xxx_list。
错误返回的 resource_type 固定为 error，即顶层 key 为 error。
所有时间使用 RFC3339 字符串。
所有 size 同时优先返回 bytes；展示字符串可选。
```

示例：

```json
{
  "viewer_session": {
    "id": "vs_123",
    "status": "ready"
  }
}
```

错误：

```json
{
  "error": {
    "code": "VIEWER_SESSION_NOT_FOUND",
    "message": "Viewer session no longer exists",
    "details": {}
  }
}
```

### 4.2 Error DTO

```json
{
  "code": "VIEWER_POD_FAILED",
  "message": "Viewer pod failed to start",
  "details": {
    "reason": "FailedScheduling"
  }
}
```

### 4.3 MountedPod DTO

```json
{
  "namespace": "default",
  "name": "app-0",
  "node_name": "node-a",
  "phase": "Running",
  "read_only": false
}
```

### 4.4 ViewerScheduling DTO

```json
{
  "requires_node": true,
  "node_name": "node-a",
  "reason": "ReadWriteOnce PVC is already mounted on node-a"
}
```

### 4.5 PVC DTO

```json
{
  "namespace": "default",
  "name": "data",
  "uid": "pvc-uid",
  "capacity_bytes": 10737418240,
  "capacity": "10Gi",
  "access_modes": ["ReadWriteOnce"],
  "mounted": true,
  "mounted_pods": [],
  "viewer_supported": true,
  "viewer_mode": "readwrite",
  "viewer_scheduling": {
    "requires_node": false,
    "node_name": "",
    "reason": ""
  },
  "reason": ""
}
```

### 4.6 PodSession DTO

```json
{
  "id": "ps_456",
  "namespace": "default",
  "pvc_name": "data",
  "pvc_uid": "pvc-uid",
  "access_mode": "ReadWriteMany",
  "mode": "readwrite",
  "pod_name": "viewer-ps-456",
  "service_name": "viewer-ps-456",
  "viewer_url": "https://viewer-ps-456.example.com",
  "status": "ready",
  "reason": "",
  "node_name": "node-a",
  "created_at": "2026-05-13T10:00:00+08:00",
  "updated_at": "2026-05-13T10:00:10+08:00",
  "last_active_at": "2026-05-13T10:02:00+08:00",
  "expires_at": "2026-05-13T10:12:00+08:00"
}
```

### 4.7 ViewerSession DTO

```json
{
  "id": "vs_123",
  "pod_session_id": "ps_456",
  "status": "ready",
  "pod_status": "ready",
  "viewer_url": "https://viewer-ps-456.example.com",
  "mode": "readwrite",
  "reason": "",
  "token_ready": true,
  "created_at": "2026-05-13T10:00:00+08:00",
  "last_heartbeat_at": "2026-05-13T10:01:00+08:00",
  "expires_at": "2026-05-13T10:03:00+08:00"
}
```

### 4.8 ViewerToken DTO

```json
{
  "viewer_session_id": "vs_123",
  "pod_session_id": "ps_456",
  "viewer_url": "https://viewer-ps-456.example.com",
  "token": "file-browser-token",
  "token_type": "Bearer",
  "expires_at": "2026-05-13T10:30:00+08:00"
}
```

### 4.9 Heartbeat DTO

```json
{
  "viewer_session_id": "vs_123",
  "status": "active",
  "server_time": "2026-05-13T10:00:00+08:00",
  "expires_at": "2026-05-13T10:03:00+08:00"
}
```

### 4.10 FileBrowserPermissions DTO

```json
{
  "admin": false,
  "execute": false,
  "create": true,
  "rename": true,
  "modify": true,
  "delete": true,
  "share": false,
  "download": true
}
```

### 4.11 FileBrowserHookVerification DTO

```json
{
  "allow": true,
  "reason": "",
  "scope": "/",
  "permissions": {
    "admin": false,
    "execute": false,
    "create": true,
    "rename": true,
    "modify": true,
    "delete": true,
    "share": false,
    "download": true
  }
}
```

## 5. 前端 API

### 5.1 获取 PVC 列表

```http
GET /api/pvcs?namespace=default
```

响应 `pvc_list`：

```json
{
  "pvc_list": {
    "items": ["PVC"]
  }
}
```

`PVC` 字段见通用 DTO。

后端根据 PVC accessModes 计算：

```text
ReadWriteMany     -> viewer_supported=true, viewer_mode=readwrite
ReadOnlyMany      -> viewer_supported=true, viewer_mode=readonly
ReadWriteOnce     -> viewer_supported=true, viewer_mode=readwrite, 需要节点约束
ReadWriteOncePod  -> viewer_supported=false
```

同时后端需要扫描当前 namespace 内引用该 PVC 的 Pod，用于判断 PVC 是否已挂载、挂载在哪些节点，以及 ReadWriteOnce 的 Viewer Pod 应如何调度。

挂载检测逻辑：

```text
1. List namespace 下的 Pod。
2. 遍历 pod.spec.volumes。
3. 找到 volume.persistentVolumeClaim.claimName == pvc.name 的 Pod。
4. 记录 pod name、phase、spec.nodeName、volume readOnly。
5. 忽略 Succeeded/Failed 且已终止的 Pod；保留 Running/Pending/Unknown 作为可能占用者。
```

ReadWriteOnce 调度判断：

```text
如果 PVC 未被任何 Pod 挂载：
  Viewer Pod 可以正常创建，不强制 nodeName。

如果 PVC 已被一个或多个 Running Pod 挂载，且这些 Pod 都在同一个 node：
  Viewer Pod 需要调度到该 node。

如果 PVC 已被多个 Pod 挂载，且分布在多个 node：
  返回 conflict，Viewer 不应直接创建，除非存储插件明确支持。

如果 PVC 只被 Pending Pod 引用且没有 nodeName：
  返回 pending_conflict 或 unknown，前端可提示稍后重试。
```

对于 `ReadWriteMany` 和 `ReadOnlyMany`，已挂载信息主要用于展示，不限制 Viewer Pod 调度。

### 5.2 创建 Viewer Session

```http
POST /api/viewer-sessions
Content-Type: application/json
```

请求：

```json
{
  "namespace": "string",
  "pvc_name": "string"
}
```

响应 `viewer_session`：

```json
{
  "viewer_session": "ViewerSession"
}
```

`ViewerSession` 字段见通用 DTO。

处理逻辑：

```text
1. 校验用户登录态。
2. 校验用户是否有权限访问该 PVC。
3. 读取 PVC。
4. 扫描 Pod，判断该 PVC 是否已被挂载以及挂载节点。
5. 如果是 ReadWriteOncePod，返回 400。
6. 如果是 ReadWriteOnce 且已有挂载节点，记录 Viewer Pod node constraint。
7. 如果是 ReadWriteOnce 且挂载状态冲突，返回 409。
8. 使用 pvcUID 在 PodSessionByPVC cache 查找 Pod Session。
9. 如果 cache 命中，校验 PodSession 未过期且 Kubernetes Pod 仍存在。
10. 如果 cache 未命中，从 Kubernetes label 反查已有 Viewer Pod。
11. 如存在可复用 Pod，重建内存 Pod Session。
12. 如不存在，创建 Pod Session 和 Viewer Pod。
13. 创建新的 Viewer Session。
14. 更新 ViewerByPod 映射。
15. 返回 Viewer Session 和 Pod Session 信息。
```

Pod 复用查询 label：

```text
storage-management.sealos.io/component=viewer
storage-management.sealos.io/pvc-name=<pvc-name>
storage-management.sealos.io/pvc-uid=<pvc-uid>
```

建议使用 `pvc-uid`，避免 PVC 同名重建后误复用。

错误 `error`：

```json
{
  "error": "Error"
}
```

`Error.details` 可包含 `mounted_pods: ["MountedPod"]` 等上下文。

### 5.3 查询 Viewer Session 状态

```http
GET /api/viewer-sessions/{viewerSessionID}
```

响应 `viewer_session`：

```json
{
  "viewer_session": "ViewerSession"
}
```

状态：

```text
creating
ready
failed
closed
expired
```

前端轮询到 `status=ready` 后，请求 token。

如果后端重启导致内存中的 Viewer Session 丢失，返回：

```json
{
  "error": "Error"
}
```

前端应重新调用 `POST /api/viewer-sessions`。后端会通过 Kubernetes label 复用已有 Viewer Pod，或创建新的 Viewer Pod。

### 5.4 获取 File Browser Token

```http
POST /api/viewer-sessions/{viewerSessionID}/token
```

响应 `viewer_token`：

```json
{
  "viewer_token": "ViewerToken"
}
```

后端处理：

```text
1. 校验用户登录态。
2. 校验 Viewer Session 属于当前用户。
3. 校验 Viewer Session active。
4. 校验 Pod Session ready。
5. 生成 Auth Request。
6. 调用 File Browser /api/login。
7. File Browser 触发 hook。
8. hook 调后端验证 Auth Request。
9. File Browser 返回 token 给后端。
10. 后端返回 token 给前端。
```

响应头：

```http
Cache-Control: no-store
Pragma: no-cache
```

### 5.5 Viewer Session Heartbeat

```http
POST /api/viewer-sessions/{viewerSessionID}/heartbeat
```

响应 `heartbeat`：

```json
{
  "heartbeat": "Heartbeat"
}
```

前端建议：

```text
每 15-30 秒发送一次 heartbeat。
窗口关闭时尽量调用 DELETE /api/viewer-sessions/{id}。
```

后端处理：

```text
更新 viewer_session.last_heartbeat
更新 pod_session.last_active_at
刷新 ViewerSession cache TTL
刷新 PodSession cache TTL
```

### 5.6 关闭 Viewer Session

```http
DELETE /api/viewer-sessions/{viewerSessionID}
```

响应 `viewer_session`：

```json
{
  "viewer_session": "ViewerSession"
}
```

处理逻辑：

```text
1. 标记 Viewer Session closed。
2. 如果该 Pod Session 下没有 active Viewer Session，不立即删除 Pod。
3. 等待 keepalive grace period。
4. grace period 后仍没有新 session，则清理 Pod。
```

### 5.7 主动关闭 Pod Session

```http
DELETE /api/pod-sessions/{podSessionID}
```

响应 `pod_session`：

```json
{
  "pod_session": "PodSession"
}
```

处理逻辑：

```text
1. 校验用户有权限关闭该 Pod Session。
2. 标记 Pod Session terminating。
3. 标记其下 Viewer Sessions closed。
4. 删除 Viewer Pod / Service / Ingress。
5. 标记 Pod Session terminated。
```

如果这是多用户共享 Pod，是否允许某个用户主动关闭整个 Pod 需要产品定义。简单策略：

```text
只有创建者、管理员、或最后一个活跃 session 的用户可以关闭 Pod。
普通用户关闭窗口只关闭自己的 Viewer Session。
```

### 5.8 查询 Pod Session

```http
GET /api/pod-sessions/{podSessionID}
```

响应 `pod_session`：

```json
{
  "pod_session": "PodSession"
}
```

## 6. Hook API

### 6.1 验证 File Browser 登录

```http
POST /internal/filebrowser-hook/verify
Authorization: Bearer <hook-client-token>
Content-Type: application/json
```

请求：

```json
{
  "pod_session_id": "string",
  "viewer_pod_name": "string",
  "username": "string",
  "auth_request_id": "string",
  "password_hash": "string"
}
```

响应 `filebrowser_hook_verification`：

```json
{
  "filebrowser_hook_verification": "FileBrowserHookVerification"
}
```

拒绝也使用成功响应 envelope，便于 hook 统一解析：

```json
{
  "filebrowser_hook_verification": "FileBrowserHookVerification"
}
```

接口级错误仍返回 `error`，例如 hook token 无效或请求格式错误：

```json
{
  "error": "Error"
}
```

后端验证：

```text
1. 校验 hook-client-token 属于该 Viewer Pod。
2. 查找 Auth Request。
3. 校验 Auth Request 未过期、未使用。
4. 校验 authRequest.viewerSessionID == username。
5. 校验 authRequest.podSessionID == pod_session_id。
6. 常量时间比较 password_hash。
7. 校验 Viewer Session active。
8. 校验 Pod Session ready。
9. 根据 PVC accessMode 和用户权限计算 File Browser 权限。
10. 原子标记 Auth Request used。
```

Auth Request 必须一次性使用。

如果后端重启导致 Auth Request 丢失，hook verify 必须拒绝。前端随后重新请求 token，后端重新创建 Auth Request 并再次登录 File Browser。

## 7. File Browser Hook 脚本

hook 从环境变量读取：

```text
USERNAME
PASSWORD
POD_SESSION_ID
VIEWER_POD_NAME
BACKEND_VERIFY_URL
HOOK_CLIENT_TOKEN
```

`PASSWORD` 格式：

```text
<authRequestID>.<secret>
```

hook 逻辑：

```text
1. 读取 USERNAME/PASSWORD。
2. 拆分 PASSWORD 得到 authRequestID 和 secret。
3. 计算 passwordHash = sha256(secret)。
4. 请求 /internal/filebrowser-hook/verify。
5. 如果 allow=true，输出 hook.action=auth 和 user.perm.*。
6. 否则输出 hook.action=block。
```

成功输出示例：

```text
hook.action=auth
user.perm.admin=false
user.perm.execute=false
user.perm.create=true
user.perm.rename=true
user.perm.modify=true
user.perm.delete=true
user.perm.share=false
user.perm.download=true
user.scope=/
```

失败输出：

```text
hook.action=block
```

要求：

```text
hook 默认失败
后端不可达时失败
不打印 PASSWORD
不记录 token
请求后端超时建议 2 秒
```

## 8. Viewer Pod 创建

### 8.1 Pod Labels

```yaml
storage-management.sealos.io/component: viewer
storage-management.sealos.io/pvc-name: <pvc-name>
storage-management.sealos.io/pvc-uid: <pvc-uid>
storage-management.sealos.io/pod-session-id: <pod-session-id>
```

### 8.2 Pod 环境变量

```yaml
env:
  - name: POD_SESSION_ID
    value: <pod-session-id>
  - name: BACKEND_VERIFY_URL
    value: https://backend/internal/filebrowser-hook/verify
  - name: HOOK_CLIENT_TOKEN
    valueFrom:
      secretKeyRef:
        name: viewer-hook-secret
        key: token
```

### 8.3 File Browser 启动参数

示例：

```text
filebrowser \
  --root /srv \
  --address 0.0.0.0 \
  --port 8080 \
  --auth.method=hook \
  --auth.header=
  --tokenExpirationTime=15m \
  --database /tmp/filebrowser.db
```

具体参数以当前 File Browser 镜像版本为准。关键要求：

```text
root 指向 PVC 挂载目录
database 不要放在 PVC 目录内
token 过期时间尽量短
auth 使用 hook
```

### 8.4 PVC 挂载

```yaml
volumeMounts:
  - name: pvc
    mountPath: /srv
volumes:
  - name: pvc
    persistentVolumeClaim:
      claimName: <pvc-name>
      readOnly: <true for ReadOnlyMany>
```

ReadOnlyMany：

```text
readOnly=true
hook 返回只读权限
```

ReadWriteMany / ReadWriteOnce：

```text
readOnly=false
hook 返回读写权限
```

### 8.5 ReadWriteOnce 节点限制

如果 PVC 是 ReadWriteOnce：

```text
1. 查询 PVC 是否已被 Pod 使用。
2. 如果已被某个 Pod 挂载，尽量将 Viewer Pod 调度到同一节点。
3. 如果无法确定节点，创建后观察调度事件。
4. 调度失败则标记 Pod Session failed。
```

实现可先简单处理：

```text
如果 PVC 当前没有被其他 Pod 使用，正常创建 Viewer Pod。
如果发现 PVC 已被其他 Pod 使用，读取该 Pod 的 nodeName，并给 Viewer Pod 设置 nodeName 或 nodeAffinity。
```

更明确的调度规则：

```text
mountedPods 为空：
  不设置 nodeName / nodeAffinity。

mountedPods 中存在 Running Pod，且 nodeName 都相同：
  设置 nodeAffinity 到该 node，或直接设置 nodeName。

mountedPods 中存在 Running Pod，但 nodeName 不同：
  返回 PVC_MOUNT_CONFLICT。

mountedPods 中只有 Pending Pod 且 nodeName 为空：
  返回 PVC_MOUNT_PENDING，提示稍后重试。

mountedPods 中包含当前已存在的 Viewer Pod：
  可直接复用该 Viewer Pod，不再新建。
```

优先使用 `nodeAffinity` 而不是直接 `nodeName`，这样调度器仍能处理污点、资源不足、亲和性等约束：

```yaml
affinity:
  nodeAffinity:
    requiredDuringSchedulingIgnoredDuringExecution:
      nodeSelectorTerms:
        - matchExpressions:
            - key: kubernetes.io/hostname
              operator: In
              values:
                - <node-name>
```

如果目标 node 资源不足或有 taint，Viewer Pod 仍可能调度失败。后端需要 watch/list Pod event 或 condition，并把失败原因写入 Pod Session。

## 9. 后端实现拆分

### 9.1 Controller / HTTP Handler

负责：

```text
解析 REST 请求
校验用户登录态
调用 service
返回 JSON
```

主要 handler：

```go
CreateViewerSession
GetViewerSession
IssueViewerToken
HeartbeatViewerSession
CloseViewerSession
ClosePodSession
VerifyFileBrowserHook
ListPVCs
```

### 9.2 ViewerSessionService

负责：

```text
创建 Viewer Session
复用或创建 Pod Session
维护 heartbeat
关闭 session
计算是否需要清理 Pod
维护 ViewerSession LRU Cache
```

### 9.3 PodSessionService

负责：

```text
查找已有 Viewer Pod
创建 Viewer Pod / Service / Ingress
watch Pod 状态
清理 Pod 资源
处理 ReadWriteOnce 调度约束
维护 PodSession LRU Cache
通过 Kubernetes label 反向重建 PodSession
```

### 9.4 AuthService

负责：

```text
创建 Auth Request
调用 File Browser /api/login
验证 hook 请求
生成权限响应
记录 token hash
维护 AuthRequest LRU Cache
维护 TokenRecord LRU Cache
```

### 9.5 KubernetesClient

负责：

```text
Get PVC
List Pods by label
List Pods by namespace
Create Pod
Create Service
Create Ingress
Delete resources
Watch Pod
```

### 9.6 PVCMountDetector

负责根据 Pod volume 引用判断 PVC 当前挂载情况。

接口示例：

```go
type PVCMountDetector interface {
	DetectPVCMounts(ctx context.Context, namespace string, pvcName string) (*PVCMountInfo, error)
}

type PVCMountInfo struct {
	Mounted     bool
	MountedPods []MountedPod
	Nodes       []string
	Conflict    bool
	Reason      string
}

type MountedPod struct {
	Namespace string
	Name      string
	NodeName  string
	Phase     string
	ReadOnly  bool
}
```

调用场景：

```text
GET /api/pvcs:
  返回 mounted/mountedPods/viewerScheduling。

POST /api/viewer-sessions:
  为 ReadWriteOnce 判断是否需要 nodeAffinity，或是否冲突。
```

### 9.7 StateStore

负责封装内存 LRU Cache，避免业务代码直接操作多个 cache。

接口示例：

```go
type StateStore interface {
	GetPodSession(id string) (*PodSession, bool)
	PutPodSession(session *PodSession)
	DeletePodSession(id string)
	FindPodSessionByPVC(namespace, pvcUID string) (*PodSession, bool)

	GetViewerSession(id string) (*ViewerSession, bool)
	PutViewerSession(session *ViewerSession)
	DeleteViewerSession(id string)
	ListViewerSessionsByPod(podSessionID string) []*ViewerSession

	CreateAuthRequest(req *AuthRequest, secret string)
	ConsumeAuthRequest(id string, passwordHash string) (*AuthRequest, bool)

	PutTokenRecord(record *TokenRecord)
	PurgeExpired(now time.Time) []ExpiredItem
}
```

要求：

```text
ConsumeAuthRequest 必须原子完成：
  校验存在
  校验未过期
  校验未使用
  常量时间比较 hash
  标记 used 或删除

所有 cache 操作需要加锁，或使用线程安全 LRU 实现。
```

## 10. 清理任务

后端后台任务定期运行：

```text
每 30-60 秒扫描 Viewer Session 和 Pod Session。
```

清理 Viewer Session：

```text
如果 last_heartbeat 超过 timeout，标记 expired。
从 ViewerSessions cache 删除。
从 ViewerByPod 映射中删除。
```

清理 Pod Session：

```text
如果 Pod Session 下没有 active Viewer Session；
并且 last_active_at 超过 keepalive grace period；
删除 Viewer Pod。
从 PodSessions cache 删除。
从 PodSessionByPVC cache 删除。
```

清理 Auth Request：

```text
如果 auth_request.expires_at < now，删除。
如果 auth_request.used_at != nil，删除或等待很短 TTL 后删除。
```

清理 TokenRecord：

```text
如果 token_record.expires_at < now，删除。
```

建议参数：

```text
heartbeat interval: 15-30s
viewer session timeout: 60-90s
pod keepalive grace: 3-10min
auth request TTL: 30s
File Browser token TTL: 15min
```

### 10.1 LRU Eviction 处理

LRU 因容量上限淘汰条目时，需要按类型处理：

```text
AuthRequest 被淘汰：
  直接失效，后续 hook verify 返回拒绝。

TokenRecord 被淘汰：
  只影响审计，不影响 File Browser 已签发 token。

ViewerSession 被淘汰：
  视为 session expired。
  从 ViewerByPod 中移除。

PodSession 被淘汰：
  不要立即删除 Kubernetes Pod。
  先通过 K8S 同步任务确认该 Pod 是否仍有 active ViewerSession。
  如果没有内存 session 引用，进入 grace 清理。
```

实际实现中建议不要依赖 LRU eviction 做业务清理。业务清理应主要由 TTL 扫描完成，LRU 只是内存上限保护。

### 10.2 Kubernetes 资源同步

因为后端内存状态可能丢失，必须定期同步 Kubernetes 中的 Viewer Pod。

同步任务：

```text
每 30-60 秒 list label storage-management.sealos.io/component=viewer 的 Pod。
```

对每个 Viewer Pod：

```text
1. 读取 label:
   - pvc-name
   - pvc-uid
   - pod-session-id

2. 如果内存中没有对应 PodSession：
   - 如果 Pod age 小于 recovery grace，重建 PodSession。
   - 如果 Pod age 超过 orphan grace，删除该 Pod。

3. 如果内存中有 PodSession：
   - 同步 Pod phase / ready condition。
   - Pod Failed/Unknown 时标记 PodSession failed 并清理。
   - Pod Ready 时标记 ready。
```

建议 grace：

```text
recovery grace: 2-5min
orphan grace: 5-10min
```

后端重启后的行为：

```text
1. 内存 cache 为空。
2. 启动同步任务 list 现有 Viewer Pod。
3. 对较新的 Viewer Pod 重建 PodSession。
4. 原有 ViewerSession 无法恢复，前端需要重新创建 ViewerSession。
5. 没有新 ViewerSession 接管的旧 Pod，超过 orphan grace 后删除。
```

### 10.3 Watch 与 List 的关系

可以同时使用：

```text
watch: 实时更新 Pod 状态。
periodic list: 防止 watch 断线或内存状态丢失。
```

MVP 可以只做 periodic list，简单可靠。

## 11. 前端实现

### 11.1 页面流程

```text
1. 展示 PVC 列表。
2. 用户点击管理。
3. 调 POST /api/viewer-sessions。
4. 进入 loading 状态。
5. 轮询 GET /api/viewer-sessions/{id}。
6. ready 后调用 POST /token。
7. 保存 token 到内存。
8. 使用 token 初始化文件管理 UI。
9. 定时 heartbeat。
10. 页面关闭或用户退出时 DELETE viewer session。
```

### 11.2 Token 使用

```text
token 只存 React state / memory。
不放 localStorage。
不放 URL。
刷新页面后重新走 token 获取流程。
```

### 11.3 文件管理 API

如果前端直接使用 File Browser 原生 UI，可以把 token 注入 File Browser 所需的前端状态。

如果前端自己实现文件管理 UI，则调用 File Browser API：

```http
Authorization: Bearer <token>
```

## 12. 错误码

```text
PVC_NOT_FOUND
PVC_ACCESS_DENIED
UNSUPPORTED_ACCESS_MODE
VIEWER_POD_CREATING
VIEWER_POD_FAILED
VIEWER_SESSION_NOT_FOUND
VIEWER_SESSION_EXPIRED
AUTH_REQUEST_EXPIRED
AUTH_REQUEST_USED
FILEBROWSER_LOGIN_FAILED
HOOK_VERIFY_FAILED
```

错误响应 `error`：

```json
{
  "error": {
    "code": "VIEWER_POD_FAILED",
    "message": "Viewer pod failed to start",
    "details": {
      "reason": "FailedScheduling"
    }
  }
}
```

## 13. 最小实现顺序

建议按这个顺序落地：

1. PVC 列表和 accessMode 判断。
2. 创建 Viewer Pod，挂载 PVC，跑 File Browser。
3. Pod Session / Viewer Session 数据模型。
4. 创建 Viewer Session API。
5. 轮询状态 API。
6. auth request 和 hook verify API。
7. 后端调用 File Browser login 并返回 token。
8. 前端使用 token 访问 File Browser。
9. heartbeat 和空闲清理。
10. ReadWriteOnce 节点约束。
11. Pod 异常 watch 和状态回写。

## 14. 安全要点

必须做到：

```text
/api/login 尽量只允许后端访问
PASSWORD 一次性、短 TTL、只使用一次
hook 在线调用后端验证
hook 根据后端返回权限输出 user.perm.*
File Browser token 只存内存
File Browser database 不放在 PVC 内
ReadOnlyMany 同时使用只读挂载和只读权限
ReadWriteOncePod 直接禁用
```

暂不作为 MVP 必选：

```text
PKCE
DPoP
逐请求代理鉴权
```
