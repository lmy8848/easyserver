package model

// WebServer represents a web server software (Nginx, Tomcat, Apache, Caddy)
type WebServer struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`            // nginx, tomcat, apache, caddy
	DisplayName    string `json:"display_name"`    // Nginx, Tomcat, Apache, Caddy
	Description    string `json:"description"`
	InstallCmd     string `json:"install_cmd"`     // apt install -y nginx
	UninstallCmd   string `json:"uninstall_cmd"`   // apt remove -y nginx
	ConfigPath     string `json:"config_path"`     // /etc/nginx
	ConfigFile     string `json:"config_file"`     // /etc/nginx/nginx.conf
	SitesAvailable string `json:"sites_available"` // /etc/nginx/sites-available
	SitesEnabled   string `json:"sites_enabled"`   // /etc/nginx/sites-enabled
	ServiceName    string `json:"service_name"`    // nginx, tomcat9
	BinaryPath     string `json:"binary_path"`     // /usr/sbin/nginx
	DefaultPort    int    `json:"default_port"`    // 80, 8080
	LogDir         string `json:"log_dir"`         // /var/log/nginx
	// Runtime state (populated by RefreshStatus)
	Status      string `json:"status"`       // not_installed, running, stopped
	Version     string `json:"version"`
	PID         int    `json:"pid"`
	MemoryBytes int64  `json:"memory_bytes"`
	Uptime      string `json:"uptime"`       // human-readable uptime
	AutoStart   bool   `json:"auto_start"`   // systemctl is-enabled
	ConfigOK    bool   `json:"config_ok"`    // config test result
	CreatedAt   string `json:"created_at"`
}

// Website represents a website deployed on a web server
type Website struct {
	ID           int64  `json:"id"`
	WebServerID  int64  `json:"web_server_id"`
	Name         string `json:"name"`
	Domain       string `json:"domain"`
	RootPath     string `json:"root_path"`
	Port         int    `json:"port"`
	ProjectType  string `json:"project_type"` // static, nodejs, php, python, java, proxy
	AppPort      int    `json:"app_port"`      // app listen port (e.g. 3000 for Node.js)
	SSLEnabled   bool   `json:"ssl_enabled"`
	SSLCertPath  string `json:"ssl_cert_path"`
	SSLKeyPath   string `json:"ssl_key_path"`
	ProxyEnabled bool   `json:"proxy_enabled"`
	ProxyPass    string `json:"proxy_pass"`
	CustomConfig string `json:"custom_config"`
	AccessLog    string `json:"access_log"`
	ErrorLog     string `json:"error_log"`
	Status       string `json:"status"` // active, disabled
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

type CreateWebsiteRequest struct {
	Name        string `json:"name" binding:"required"`
	Domain      string `json:"domain" binding:"required"`
	RootPath    string `json:"root_path" binding:"required"`
	Port        int    `json:"port"`
	ProjectType string `json:"project_type"` // static, nodejs, php, python, java, proxy
	AppPort     int    `json:"app_port"`
	CustomConfig string `json:"custom_config"`
}

type UpdateWebsiteRequest struct {
	Name         *string `json:"name"`
	Domain       *string `json:"domain"`
	RootPath     *string `json:"root_path"`
	Port         *int    `json:"port"`
	ProjectType  *string `json:"project_type"`
	AppPort      *int    `json:"app_port"`
	CustomConfig *string `json:"custom_config"`
}

// ProjectTypeConfig defines Nginx config templates per project type
type ProjectTypeConfig struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Description string `json:"description"`
	DefaultPort int    `json:"default_port"`
	Proxy       bool   `json:"proxy"`
}

func GetProjectTypes() []ProjectTypeConfig {
	return []ProjectTypeConfig{
		{Name: "static", Label: "静态网站", Description: "HTML/CSS/JS，直接由 Nginx 提供服务", DefaultPort: 80, Proxy: false},
		{Name: "nodejs", Label: "Node.js", Description: "反向代理到 Node.js 应用（如 Express、Next.js）", DefaultPort: 3000, Proxy: true},
		{Name: "php", Label: "PHP", Description: "通过 php-fpm 处理 PHP 请求", DefaultPort: 9000, Proxy: false},
		{Name: "python", Label: "Python", Description: "反向代理到 Python 应用（如 Flask、Django）", DefaultPort: 8000, Proxy: true},
		{Name: "java", Label: "Java", Description: "反向代理到 Java 应用（如 Spring Boot）", DefaultPort: 8080, Proxy: true},
		{Name: "proxy", Label: "反向代理", Description: "纯反向代理到其他服务", DefaultPort: 8080, Proxy: true},
	}
}

// PredefinedWebServers returns the default web server entries
func PredefinedWebServers() []WebServer {
	return []WebServer{
		{
			Name:           "nginx",
			DisplayName:    "Nginx",
			Description:    "高性能 HTTP 和反向代理服务器，支持负载均衡、缓存、SSL",
			InstallCmd:     "apt-get install -y nginx",
			UninstallCmd:   "apt-get remove -y nginx",
			ConfigPath:     "/etc/nginx",
			ConfigFile:     "/etc/nginx/nginx.conf",
			SitesAvailable: "/etc/nginx/sites-available",
			SitesEnabled:   "/etc/nginx/sites-enabled",
			ServiceName:    "nginx",
			BinaryPath:     "/usr/sbin/nginx",
			DefaultPort:    80,
			LogDir:         "/var/log/nginx",
		},
		{
			Name:           "apache",
			DisplayName:    "Apache",
			Description:    "最流行的 Web 服务器，模块丰富，生态成熟",
			InstallCmd:     "apt-get install -y apache2",
			UninstallCmd:   "apt-get remove -y apache2",
			ConfigPath:     "/etc/apache2",
			ConfigFile:     "/etc/apache2/apache2.conf",
			SitesAvailable: "/etc/apache2/sites-available",
			SitesEnabled:   "/etc/apache2/sites-enabled",
			ServiceName:    "apache2",
			BinaryPath:     "/usr/sbin/apache2",
			DefaultPort:    80,
			LogDir:         "/var/log/apache2",
		},
		{
			Name:           "tomcat",
			DisplayName:    "Tomcat",
			Description:    "Java Servlet 容器，运行 Java Web 应用",
			InstallCmd:     "apt-get install -y tomcat9",
			UninstallCmd:   "apt-get remove -y tomcat9",
			ConfigPath:     "/etc/tomcat9",
			ConfigFile:     "/etc/tomcat9/server.xml",
			SitesAvailable: "/etc/tomcat9/Catalina/localhost",
			SitesEnabled:   "/etc/tomcat9/Catalina/localhost",
			ServiceName:    "tomcat9",
			BinaryPath:     "/usr/share/tomcat9/bin/catalina.sh",
			DefaultPort:    8080,
			LogDir:         "/var/log/tomcat9",
		},
		{
			Name:           "caddy",
			DisplayName:    "Caddy",
			Description:    "自动 HTTPS、零配置的现代 Web 服务器",
			InstallCmd:     "apt-get install -y caddy",
			UninstallCmd:   "apt-get remove -y caddy",
			ConfigPath:     "/etc/caddy",
			ConfigFile:     "/etc/caddy/Caddyfile",
			SitesAvailable: "/etc/caddy",
			SitesEnabled:   "/etc/caddy",
			ServiceName:    "caddy",
			BinaryPath:     "/usr/bin/caddy",
			DefaultPort:    80,
			LogDir:         "/var/log/caddy",
		},
	}
}
