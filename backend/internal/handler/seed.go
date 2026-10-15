package handler

import (
	"log"

	"langhui/internal/auth"
	"langhui/internal/config"
	"langhui/internal/model"

	"gorm.io/gorm"
)

// SeedAdmin 首次启动时若 admin 表为空，则按环境变量创建初始超管，避免手工写库
func SeedAdmin(db *gorm.DB, cfg config.Config) {
	var cnt int64
	db.Model(&model.Admin{}).Count(&cnt)
	if cnt > 0 {
		return
	}
	hash, err := auth.HashPassword(cfg.AdminPass)
	if err != nil {
		log.Printf("seed admin: hash password failed: %v", err)
		return
	}
	if err := db.Create(&model.Admin{Username: cfg.AdminUser, PasswordHash: hash}).Error; err != nil {
		log.Printf("seed admin: create failed: %v", err)
		return
	}
	log.Printf("seeded initial admin user=%q, please login and change the password", cfg.AdminUser)
}
