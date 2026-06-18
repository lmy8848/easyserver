# EasyServer v1.0.0 Linux 部署包

## 📦 文件清单

| 文件 | 大小 | 说明 |
|------|------|------|
| `easyserver` | 31MB | 主程序 (Linux amd64, 静态链接) |
| `config.yaml` | 932B | 配置文件模板 |
| `install.sh` | 4.4KB | 一键安装脚本 |
| `linux-deploy.md` | 9.4KB | 详细部署文档 |
| `README.md` | 1.8KB | 本文件 |

## ✅ 二进制信息

```
ELF 64-bit LSB executable, x86-64, version 1 (SYSV), statically linked
```

- 无需额外依赖
- 支持所有 x86_64 Linux 发行版
- 包含嵌入式前端资源

## 🚀 快速部署

### 方式一：一键安装 (推荐)

```bash
# 1. 上传部署包到服务器
scp easyserver-v1.0.0-linux-amd64.tar.gz user@server:/tmp/

# 2. 登录服务器
ssh user@server

# 3. 解压并安装
cd /tmp
tar -xzf easyserver-v1.0.0-linux-amd64.tar.gz
sudo bash install.sh

# 4. 访问
http://server-ip:8080
```

### 方式二：手动安装

```bash
# 1. 解压
tar -xzf easyserver-v1.0.0-linux-amd64.tar.gz

# 2. 创建目录
sudo mkdir -p /opt/easyserver/data

# 3. 复制文件
sudo cp easyserver /opt/easyserver/
sudo cp config.yaml /opt/easyserver/
sudo chmod +x /opt/easyserver/easyserver

# 4. 修改配置 (必须修改 jwt_secret)
sudo vi /opt/easyserver/config.yaml

# 5. 测试运行
/opt/easyserver/easyserver -config /opt/easyserver/config.yaml

# 6. 创建 systemd 服务 (参考 linux-deploy.md)
```

## 🔐 首次登录

| 项目 | 值 |
|------|-----|
| 地址 | `http://server-ip:8080` |
| 用户名 | `admin` |
| 密码 | `admin` |

**⚠️ 请立即修改默认密码！**

## 📋 常用命令

```bash
# 启动
sudo systemctl start easyserver

# 停止
sudo systemctl stop easyserver

# 重启
sudo systemctl restart easyserver

# 查看状态
sudo systemctl status easyserver

# 查看日志
sudo journalctl -u easyserver -f
```

## 📖 详细文档

请参考 `linux-deploy.md` 获取完整部署指南，包括：
- Nginx 反向代理配置
- HTTPS 配置
- 防火墙配置
- 备份与恢复
- 故障排查

## ⚠️ 注意事项

1. **必须修改 JWT Secret** - 否则存在安全风险
2. **建议配置 HTTPS** - 生产环境必须
3. **定期备份数据** - `/opt/easyserver/data/`
4. **修改默认密码** - 首次登录后立即修改

---

**EasyServer v1.0.0** - Linux 服务器管理面板
