package config

import (
	"encoding/json"
	"fmt"
	"os"
)

type Config struct {
	Server   ServerConfig   `json:"server"`
	Database DatabaseConfig `json:"database"`
	Redis    *RedisConfig   `json:"redis,omitempty"` // nil = use in-memory store
	Admin    AdminConfig    `json:"admin"`
	LogLevel string         `json:"log_level"` // "debug", "info" (default), "warn", "error"
}

type ServerConfig struct {
	Port int    `json:"port"`
	Host string `json:"host"`
}

type DatabaseConfig struct {
	Driver   string `json:"driver"`   // "sqlite" (default) or "postgres"
	DSN      string `json:"dsn"`      // file path or connection string
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	DBName   string `json:"dbname"`
}

type RedisConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Password string `json:"password"`
	DB       int    `json:"db"`
}

type AdminConfig struct {
	Password string `json:"password"`
	APIKey   string `json:"api_key"`
}

func (d DatabaseConfig) GetDriver() string {
	if d.Driver != "" {
		return d.Driver
	}
	return "sqlite"
}

func (d DatabaseConfig) GetDSN() string {
	if d.DSN != "" {
		return d.DSN
	}
	if d.GetDriver() == "sqlite" {
		return "data/cc2api.db"
	}
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		d.User, d.Password, d.Host, d.Port, d.DBName)
}

func (r RedisConfig) Addr() string {
	return fmt.Sprintf("%s:%d", r.Host, r.Port)
}

func Load(path string) (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{Port: 8080, Host: "0.0.0.0"},
		Admin:  AdminConfig{Password: "admin", APIKey: "cc2api-key"},
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}
