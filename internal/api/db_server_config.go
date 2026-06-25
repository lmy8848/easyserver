package api

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"easyserver/internal/service"

	"github.com/gin-gonic/gin"
)

// ConfigHandler handles MySQL/PostgreSQL/Redis config management endpoints.
// These use package-level functions from the service package and have no service struct dependency.
type ConfigHandler struct{}

func NewConfigHandler() *ConfigHandler {
	return &ConfigHandler{}
}

// --- MySQL Config ---

func (h *ConfigHandler) GetMySQLConfig(c *gin.Context) {
	configPath := service.FindMySQLConfig()
	if configPath == "" {
		Success(c, gin.H{"found": false, "message": "未找到 MySQL 配置文件"})
		return
	}

	config, err := service.ParseMySQLConfig(configPath)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	// Build response with common params metadata
	response := gin.H{
		"found":    true,
		"config":   config,
		"sections": gin.H{},
	}
	sections := response["sections"].(gin.H)
	for _, section := range config.Sections {
		sections[section.Name] = gin.H{
			"params": section.Params,
			"meta":   service.GetCommonParams(section.Name),
		}
	}

	Success(c, response)
}

func (h *ConfigHandler) SaveMySQLConfig(c *gin.Context) {
	var req struct {
		Sections []struct {
			Name   string            `json:"name"`
			Params map[string]string `json:"params"`
		} `json:"sections"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	configPath := service.FindMySQLConfig()
	if configPath == "" {
		BadRequest(c, "未找到 MySQL 配置文件")
		return
	}

	// Handle raw text save from the raw text modal
	if raw, ok := isRawConfigRequest(req.Sections); ok {
		if err := saveRawConfig(configPath, raw); err != nil {
			InternalError(c, err.Error())
			return
		}
		Success(c, gin.H{"message": "配置已保存", "path": configPath})
		return
	}

	config := &service.MySQLConfig{
		FilePath: configPath,
	}
	for _, s := range req.Sections {
		config.Sections = append(config.Sections, service.ConfigSection{
			Name:   s.Name,
			Params: s.Params,
		})
	}

	if err := service.SaveMySQLConfig(config); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{"message": "配置已保存", "path": configPath})
}

func (h *ConfigHandler) GetMySQLCommonParams(c *gin.Context) {
	section := c.DefaultQuery("section", "mysqld")
	params := service.GetCommonParams(section)
	Success(c, params)
}

// --- PostgreSQL Config ---

func (h *ConfigHandler) GetPostgreSQLConfig(c *gin.Context) {
	configPath := service.FindPostgreSQLConfig()
	if configPath == "" {
		Success(c, gin.H{"found": false, "message": "未找到 PostgreSQL 配置文件"})
		return
	}

	config, err := service.ParsePostgreSQLConfig(configPath)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	response := gin.H{
		"found":    true,
		"config":   config,
		"sections": gin.H{},
	}
	sections := response["sections"].(gin.H)
	for _, section := range config.Sections {
		sections[section.Name] = gin.H{
			"params": section.Params,
			"meta":   service.GetPostgreSQLCommonParams(),
		}
	}

	Success(c, response)
}

func (h *ConfigHandler) SavePostgreSQLConfig(c *gin.Context) {
	var req struct {
		Sections []struct {
			Name   string            `json:"name"`
			Params map[string]string `json:"params"`
		} `json:"sections"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	configPath := service.FindPostgreSQLConfig()
	if configPath == "" {
		BadRequest(c, "未找到 PostgreSQL 配置文件")
		return
	}

	// Handle raw text save from the raw text modal
	if raw, ok := isRawConfigRequest(req.Sections); ok {
		if err := saveRawConfig(configPath, raw); err != nil {
			InternalError(c, err.Error())
			return
		}
		Success(c, gin.H{"message": "配置已保存", "path": configPath})
		return
	}

	config := &service.MySQLConfig{
		FilePath: configPath,
	}
	for _, s := range req.Sections {
		config.Sections = append(config.Sections, service.ConfigSection{
			Name:   s.Name,
			Params: s.Params,
		})
	}

	if err := service.SavePostgreSQLConfig(config); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{"message": "配置已保存", "path": configPath})
}

func (h *ConfigHandler) GetPGCommonParams(c *gin.Context) {
	params := service.GetPostgreSQLCommonParams()
	Success(c, params)
}

// --- Redis Config ---

func (h *ConfigHandler) GetRedisConfig(c *gin.Context) {
	configPath := service.FindRedisConfig()
	if configPath == "" {
		Success(c, gin.H{"found": false, "message": "未找到 Redis 配置文件"})
		return
	}

	config, err := service.ParseRedisConfig(configPath)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	response := gin.H{
		"found":    true,
		"config":   config,
		"sections": gin.H{},
	}
	sections := response["sections"].(gin.H)
	for _, section := range config.Sections {
		sections[section.Name] = gin.H{
			"params": section.Params,
			"meta":   service.GetRedisCommonParams(),
		}
	}

	Success(c, response)
}

func (h *ConfigHandler) SaveRedisConfig(c *gin.Context) {
	var req struct {
		Sections []struct {
			Name   string            `json:"name"`
			Params map[string]string `json:"params"`
		} `json:"sections"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	configPath := service.FindRedisConfig()
	if configPath == "" {
		BadRequest(c, "未找到 Redis 配置文件")
		return
	}

	// Handle raw text save from the raw text modal
	if raw, ok := isRawConfigRequest(req.Sections); ok {
		if err := saveRawConfig(configPath, raw); err != nil {
			InternalError(c, err.Error())
			return
		}
		Success(c, gin.H{"message": "配置已保存", "path": configPath})
		return
	}

	config := &service.MySQLConfig{
		FilePath: configPath,
	}
	for _, s := range req.Sections {
		config.Sections = append(config.Sections, service.ConfigSection{
			Name:   s.Name,
			Params: s.Params,
		})
	}

	if err := service.SaveRedisConfig(config); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{"message": "配置已保存", "path": configPath})
}

func (h *ConfigHandler) GetRedisCommonParams(c *gin.Context) {
	params := service.GetRedisCommonParams()
	Success(c, params)
}

// --- Helper functions ---

func saveRawConfig(filePath, content string) error {
	backupPath := filePath + ".bak." + time.Now().Format("20060102150405")
	if data, err := os.ReadFile(filePath); err == nil {
		if err := os.WriteFile(backupPath, data, 0644); err != nil {
			return fmt.Errorf("backup config: %w", err)
		}
	}
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	return os.WriteFile(filePath, []byte(content), 0644)
}

func isRawConfigRequest(sections []struct {
	Name   string            `json:"name"`
	Params map[string]string `json:"params"`
}) (string, bool) {
	if len(sections) == 1 && sections[0].Name == "custom" {
		if raw, ok := sections[0].Params["raw"]; ok {
			return raw, true
		}
	}
	return "", false
}
