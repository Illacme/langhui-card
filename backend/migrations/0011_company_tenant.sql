-- 0011：多租户第一期（后台多租户 + 数据隔离）
-- 设计：
--   * company 为租户表；朗汇机电为 1 号租户，存量数据全部归属（company_id 默认 1）。
--   * company_info / company_profile / home_layout 由 id=1 单例改为按 company_id 唯一（1:1）。
--   * employee / achievement_category / project_case / biz_service / open_apply /
--     card_modify_apply 增加 company_id（NOT NULL DEFAULT 1 + 索引）。
--   * admin 增加 company_id：0 = 平台超级管理员（可见全部租户），>0 = 企业管理员（仅本企业）。
--   * 小程序端公开接口缺省落 company_id=1（朗汇），行为不变；按企业分发留给第二期。
-- 与 devserver AutoMigrate（model 包）保持一致。

CREATE TABLE IF NOT EXISTS company (
  id         BIGINT PRIMARY KEY AUTO_INCREMENT,
  name       VARCHAR(128) NOT NULL DEFAULT '',
  status     VARCHAR(16)  NOT NULL DEFAULT 'enabled',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 朗汇机电 = 1 号租户（存量数据的所有者）
INSERT INTO company (id, name, status) VALUES
  (1, '廊坊市朗汇机电设备有限公司', 'enabled');

ALTER TABLE company_info    ADD COLUMN company_id BIGINT NOT NULL DEFAULT 1;
ALTER TABLE company_profile ADD COLUMN company_id BIGINT NOT NULL DEFAULT 1;
ALTER TABLE home_layout     ADD COLUMN company_id BIGINT NOT NULL DEFAULT 1;
ALTER TABLE employee        ADD COLUMN company_id BIGINT NOT NULL DEFAULT 1;
ALTER TABLE achievement_category ADD COLUMN company_id BIGINT NOT NULL DEFAULT 1;
ALTER TABLE project_case    ADD COLUMN company_id BIGINT NOT NULL DEFAULT 1;
ALTER TABLE biz_service     ADD COLUMN company_id BIGINT NOT NULL DEFAULT 1;
ALTER TABLE open_apply      ADD COLUMN company_id BIGINT NOT NULL DEFAULT 1;
ALTER TABLE card_modify_apply ADD COLUMN company_id BIGINT NOT NULL DEFAULT 1;
ALTER TABLE admin           ADD COLUMN company_id BIGINT NOT NULL DEFAULT 0;

-- 单例表按租户唯一；存量行已由 DEFAULT 回填为 1（朗汇）
ALTER TABLE company_info    ADD UNIQUE INDEX uk_company_info_company (company_id);
ALTER TABLE company_profile ADD UNIQUE INDEX uk_company_profile_company (company_id);
ALTER TABLE home_layout     ADD UNIQUE INDEX uk_home_layout_company (company_id);

-- 业务表租户索引（后台列表按企业过滤）
ALTER TABLE employee        ADD INDEX idx_employee_company (company_id);
ALTER TABLE achievement_category ADD INDEX idx_category_company (company_id);
ALTER TABLE project_case    ADD INDEX idx_project_company (company_id);
ALTER TABLE biz_service     ADD INDEX idx_service_company (company_id);
ALTER TABLE open_apply      ADD INDEX idx_open_apply_company (company_id);
ALTER TABLE card_modify_apply ADD INDEX idx_modify_apply_company (company_id);
ALTER TABLE admin           ADD INDEX idx_admin_company (company_id);
