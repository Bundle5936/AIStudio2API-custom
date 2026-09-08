# 本地补丁清单与意图注册表 (Local Patch Registry)

本仓库维护的下游私有补丁存放在 `patches/` 目录下。构建与 CI 时按文件名前缀顺序应用。
当上游更新时，工作流会检查各补丁的兼容状态；若上游已原生实现相应功能，根据【退役判定】移除该补丁即可。

---

### 1. `01-quota-cooldown.patch`
- **目标文件**：`internal/aistudio/quota.go`
- **补丁意图**：防长死刑冷却与阶梯退避。将上游默认冷却至次日重置（24小时）优化为：一般配额错误冷却 2 分钟，每日限额错误冷却 15 分钟。
- **退役判定**：若上游原生支持可配置冷却时间、动态指数退避，或缩短了限额惩罚时长，则可废弃此补丁。

### 2. `02-reset-cooldowns.patch`
- **目标文件**：`internal/aistudio/accounts.go`, `internal/app/admin.go`
- **补丁意图**：账号验证联动清空冷却。在 `AccountPool` 增加 `ResetCooldowns` 方法，在管理后台点击“验证账号”（`VerifyAccount`）且认证成功后，自动清空该账号所有模型的冷却状态与全局冷却，无需等待冷却倒计时。
- **退役判定**：若上游在验证账号或重新登录时自带清除冷却逻辑，则可废弃此补丁。

### 3. `03-schema-lenient.patch`
- **目标文件**：`internal/aistudio/schema.go`
- **补丁意图**：JSON Schema 宽容容错。忽略未知字段而不报错（直接从 map 中删除）；对于 `ARRAY` 类型缺失 `items` 字段的情况补齐默认 string 类型（避免 Google Protobuf 严格校验报错）；对非字符串数组尝试转换为字符串。
- **退役判定**：若上游放宽了 Schema 校验、支持预处理清洗，或已做类似 array items 兜底，则可废弃此补丁。

### 4. `04-webui-auth.patch`
- **目标文件**：`internal/api/middleware.go`, `internal/app/app.go`
- **补丁意图**：Web 管理端 API Key / 密码登录保护。解除 `/api/` 仅限本地回环（loopback）访问的硬编码限制，在 `rootHandler` 挂载登录页、Cookie 会话管理和 Bearer Token 验证，使非本地网络也能安全访问 WebUI 且受密码防护。
- **退役判定**：若上游原生实现了 WebUI 用户鉴权或安全登录机制，则可废弃此补丁。

### 5. `05-camoufox-run.patch`
- **目标文件**：`internal/camoufoxnative/worker.go`
- **补丁意图**：Camoufox 官网 Run 按钮兼容与防死锁。放宽 DOM 选择器以兼容官网改版；当 Run 按钮被前端标记为 disabled 时，通过 DOM 操作强行解除 disabled 属性并触发点击。
- **退役判定**：若上游重构了 WAA 引导逻辑、支持更新的选择器或换用了新的触发方式，则可废弃此补丁。
