# 站点登录状态与密钥备份

[文档中心](../README.md) · [返回项目首页](../../README.md)

pt-tools 可配合 PT Tools Helper 同步站点 Cookie，并周期探测站点提供的 `last_access` / `last_login` 信息，在账号接近不活跃期限时通过已配置的通知通道提醒。

## 使用流程

1. 在站点管理页面启用目标站点。
2. 安装 [PT Tools Helper](../../tools/browser-extension/README.md)，并在浏览器中登录 PT 站点。
3. 在扩展中同步单个或多个站点的 Cookie；非 Cookie 认证站点仍需在 Web UI 手动配置。
4. 在 Web UI 的 `/sites/login` 页面检查剩余天数，并按站点设置阈值、提醒计划和通知通道。
5. 收到提醒后正常访问站点并重新同步 Cookie，确认登录状态已刷新。

## 每站点配置

| 配置项                   | 默认值          | 说明                                  |
| ------------------------ | --------------- | ------------------------------------- |
| `BanThresholdDays`       | `30`            | 推定的不活跃封号阈值                  |
| `RemindBeforeDays`       | `10`            | 距阈值多少天开始提醒                  |
| `ReminderCron`           | `0 10,22 * * *` | 提醒计划，默认每天 `10:00` 与 `22:00` |
| `NotificationChannelIDs` | 空              | 接收提醒的通知通道 ID                 |

默认探测间隔约为 6 小时并带随机抖动。站点未提供可靠的最后访问字段时，页面结果仅供参考。

## 备份加密密钥

`~/.pt-tools/secret.key` 用于 AES-256-GCM 加密站点 Cookie 等凭证。密钥丢失后，已存储凭证无法解密，只能重新同步。

导出备份：

```bash
pt-tools secret export > ~/secret.key.backup
chmod 600 ~/secret.key.backup
```

恢复备份：

```bash
pt-tools secret import --force < ~/secret.key.backup
```

> [!CAUTION]
> 密钥备份具备与原始密钥相同的敏感性。请存入密码管理器、加密介质或受控备份系统，不要提交到代码仓库或明文云盘。

### Docker 持久化

容器部署必须把 `/app/.pt-tools` 挂载到宿主机或受管卷。该目录同时包含 `secret.key`、数据库、日志和临时种子文件；重建容器前应确认挂载存在且备份可用。

```yaml
volumes:
  - ./data:/app/.pt-tools
```

删除 `./data` 会同时丢失数据库与密钥。备份时请把密钥与数据库作为一个一致性单元保存。

## 合规边界

此功能使用用户自己的 Cookie 或 API Key 周期访问站点页面或 profile API。启用前请确认站点规则允许自动化访问，并根据个人风险偏好设置探测频率：

- 部分站点可能把频繁脚本访问视为违规。
- 登录状态字段由站点提供，可能缺失、延迟或语义不同。
- pt-tools 不保证该功能可以避免封号，也不承担因自动化访问导致的警告、降级或封禁。

认证信息获取与同步方法见[获取 Cookie / API Key](get-cookie-apikey.md)。
