package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"cc2api/internal/config"
	"cc2api/internal/handler"
	"cc2api/internal/logger"
	"cc2api/internal/service"
	"cc2api/internal/store"
)

func main() {
	cfgPath := flag.String("config", "config.json", "config file path")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	logger.SetLevel(cfg.LogLevel)

	driver := cfg.Database.GetDriver()
	dsn := cfg.Database.GetDSN()
	// Ensure parent directory exists for SQLite
	if driver == "sqlite" {
		if dir := filepath.Dir(dsn); dir != "." {
			os.MkdirAll(dir, 0755)
		}
	}
	log.Printf("database: %s (%s)", driver, dsn)

	db, err := store.InitDB(driver, dsn)
	if err != nil {
		log.Fatalf("init db: %v", err)
	}
	defer db.Close()

	if err := store.Migrate(db, driver); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	// Cache: Redis if configured, otherwise in-memory
	var cache store.CacheStore
	if cfg.Redis != nil {
		redis, err := store.NewRedisStore(cfg.Redis.Addr(), cfg.Redis.Password, cfg.Redis.DB)
		if err != nil {
			log.Printf("redis unavailable, using in-memory cache: %v", err)
			cache = store.NewMemoryStore()
		} else {
			cache = redis
		}
	} else {
		log.Println("no redis configured, using in-memory cache")
		cache = store.NewMemoryStore()
	}
	defer cache.Close()

	accountStore := store.NewAccountStore(db, cfg.Database.GetDriver())
	usageStore := store.NewUsageStore(db, driver)

	accountSvc := service.NewAccountService(accountStore, cache)
	usageSvc := service.NewUsageService(usageStore)
	gatewaySvc := service.NewGatewayService(accountSvc, usageSvc)
	tokenTester := service.NewTokenTester()

	router := handler.NewRouter(cfg, gatewaySvc, accountSvc, usageSvc, tokenTester)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Printf("cc2api listening on %s", addr)
	if err := router.Run(addr); err != nil {
		log.Fatalf("server: %v", err)
	}
}
