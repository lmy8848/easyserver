package api

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"easyserver/internal/database_mgmt"

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
	configPath := database_mgmt.FindMySQLConfig()
	if configPath == "" {
		Success(c, gin.H{"found": false, "message": "未找到 MySQL 配置文件"})
		return
	}

	config, err := database_mgmt.ParseMySQLConfig(configPath)
	if err != nil {
		c.Error(WrapError(err))
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
			"meta":   database_mgmt.GetCommonParams(section.Name),
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
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	configPath := database_mgmt.FindMySQLConfig()
	if configPath == "" {
		c.Error(ErrBadRequest.WithMessage("未找到 MySQL 配置文件"))
		return
	}

	// Handle raw text save from the raw text modal
	if raw, ok := isRawConfigRequest(req.Sections); ok {
		if err := saveRawConfig(configPath, raw); err != nil {
			c.Error(WrapError(err))
			return
		}
		Success(c, gin.H{"message": "配置已保存", "path": configPath})
		return
	}

	config := &database_mgmt.DBConfig{
		FilePath: configPath,
	}
	for _, s := range req.Sections {
		config.Sections = append(config.Sections, database_mgmt.ConfigSection{
			Name:   s.Name,
			Params: s.Params,
		})
	}

	if err := database_mgmt.SaveMySQLConfig(config); err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, gin.H{"message": "配置已保存", "path": configPath})
}

func (h *ConfigHandler) GetMySQLCommonParams(c *gin.Context) {
	section := c.DefaultQuery("section", "mysqld")
	params := database_mgmt.GetCommonParams(section)
	Success(c, params)
}

// --- PostgreSQL Config ---

func (h *ConfigHandler) GetPostgreSQLConfig(c *gin.Context) {
	configPath := database_mgmt.FindPostgreSQLConfig()
	if configPath == "" {
		Success(c, gin.H{"found": false, "message": "未找到 PostgreSQL 配置文件"})
		return
	}

	config, err := database_mgmt.ParsePostgreSQLConfig(configPath)
	if err != nil {
		c.Error(WrapError(err))
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
			"meta":   database_mgmt.GetPostgreSQLCommonParams(),
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
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	configPath := database_mgmt.FindPostgreSQLConfig()
	if configPath == "" {
		c.Error(ErrBadRequest.WithMessage("未找到 PostgreSQL 配置文件"))
		return
	}

	// Handle raw text save from the raw text modal
	if raw, ok := isRawConfigRequest(req.Sections); ok {
		if err := saveRawConfig(configPath, raw); err != nil {
			c.Error(WrapError(err))
			return
		}
		Success(c, gin.H{"message": "配置已保存", "path": configPath})
		return
	}

	config := &database_mgmt.DBConfig{
		FilePath: configPath,
	}
	for _, s := range req.Sections {
		config.Sections = append(config.Sections, database_mgmt.ConfigSection{
			Name:   s.Name,
			Params: s.Params,
		})
	}

	if err := database_mgmt.SavePostgreSQLConfig(config); err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, gin.H{"message": "配置已保存", "path": configPath})
}

func (h *ConfigHandler) GetPGCommonParams(c *gin.Context) {
	params := database_mgmt.GetPostgreSQLCommonParams()
	Success(c, params)
}

// --- Redis Config ---

func (h *ConfigHandler) GetRedisConfig(c *gin.Context) {
	configPath := database_mgmt.FindRedisConfig()
	if configPath == "" {
		Success(c, gin.H{"found": false, "message": "未找到 Redis 配置文件"})
		return
	}

	config, err := database_mgmt.ParseRedisConfig(configPath)
	if err != nil {
		c.Error(WrapError(err))
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
			"meta":   database_mgmt.GetRedisCommonParams(),
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
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	configPath := database_mgmt.FindRedisConfig()
	if configPath == "" {
		c.Error(ErrBadRequest.WithMessage("未找到 Redis 配置文件"))
		return
	}

	// Handle raw text save from the raw text modal
	if raw, ok := isRawConfigRequest(req.Sections); ok {
		if err := saveRawConfig(configPath, raw); err != nil {
			c.Error(WrapError(err))
			return
		}
		Success(c, gin.H{"message": "配置已保存", "path": configPath})
		return
	}

	config := &database_mgmt.DBConfig{
		FilePath: configPath,
	}
	for _, s := range req.Sections {
		config.Sections = append(config.Sections, database_mgmt.ConfigSection{
			Name:   s.Name,
			Params: s.Params,
		})
	}

	if err := database_mgmt.SaveRedisConfig(config); err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, gin.H{"message": "配置已保存", "path": configPath})
}

func (h *ConfigHandler) GetRedisCommonParams(c *gin.Context) {
	params := database_mgmt.GetRedisCommonParams()
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
