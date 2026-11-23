# 面板定时自备份

`easyserver-backup.timer` 每天 03:30（本地时间）触发一次 `easyserver backup`，
把配置与 SQLite 一致性快照写到 `/opt/easyserver/backups/auto-backup-<ts>.tar.gz`，
保留 14 天（更早的 `auto-backup-*` 会被清理，手工导出的备份不动）。

## 安装

```bash
install -m 644 deploy/easyserver-backup.service /etc/systemd/system/
install -m 644 deploy/easyserver-backup.timer   /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now easyserver-backup.timer
systemctl list-timers easyserver-backup.timer
```

## 手动触发与校验

```bash
systemctl start easyserver-backup.service
ls -lt /opt/easyserver/backups | head
tar -tzf /opt/easyserver/backups/auto-backup-*.tar.gz | head
```

## 恢复

面板「设置 → 服务器 → 数据备份」里选包 → 预检 → 恢复（需二次确认）。
恢复前会自动生成 `pre-restore-<ts>.tar.gz`，旧配置/数据库保留为 `.bak-<ts>`。
