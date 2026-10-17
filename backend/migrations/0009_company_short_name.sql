-- 0009：企业简称（小程序顶栏居中展示），与 devserver AutoMigrate 保持一致

ALTER TABLE company_info ADD COLUMN short_name VARCHAR(64) NOT NULL DEFAULT '';
