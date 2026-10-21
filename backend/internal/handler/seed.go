package handler

import (
	"log"

	"weichuan/internal/auth"
	"weichuan/internal/config"
	"weichuan/internal/model"

	"gorm.io/gorm"
)

// SeedAdmin 首次启动时若 admin 表为空，则按环境变量创建初始平台超级管理员，避免手工写库。
// 多租户第一期：初始账号为平台超管（company_id=0），可见全部租户；企业管理员由平台方在
// PC 后台「企业管理」中创建（company_id>0，仅见本企业）。
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
	if err := db.Create(&model.Admin{CompanyID: 0, Username: cfg.AdminUser, PasswordHash: hash}).Error; err != nil {
		log.Printf("seed admin: create failed: %v", err)
		return
	}
	log.Printf("seeded initial admin user=%q, please login and change the password", cfg.AdminUser)
}
