package mise

// 运行环境子系统(mise)的持久化路径，单一来源。
//
// 设计目标：把整个 mise 运行时完全自包含在 /opt/easyserver/mise 一个目录下，
// 不碰 /etc、不碰用户 ~、不碰用户 shell PATH。卸载面板 = 删掉该目录，零系统残留。
//
// 关键机制：MISE_CONFIG_DIR 决定 mise 的 global config 查找根（默认
// ~/.config/mise）。指向面板私有目录后，面板的 mise 完全不读用户配置。
const (
	BinPath    = "/opt/easyserver/mise/bin/mise"
	DataDir    = "/opt/easyserver/mise" // MISE_DATA_DIR（installs / shims / cache）
	ConfigDir  = "/opt/easyserver/mise" // MISE_CONFIG_DIR（面板私有 config 根）
	ConfigPath = "/opt/easyserver/mise/config.toml"
)
