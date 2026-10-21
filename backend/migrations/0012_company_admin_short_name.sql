-- 0012：企业简称（company.short_name）——PC 后台侧边栏「xxxx · 管理后台」品牌名，
-- 并作为开通企业时公司信息简称（company_infos.short_name，仅小程序展示）的初始值。
-- 与 0009 的 company_infos.short_name 相互独立：后台侧边栏读本列，小程序读公司信息。
-- 与 devserver AutoMigrate 保持一致。

ALTER TABLE company ADD COLUMN short_name VARCHAR(64) NOT NULL DEFAULT '';
-- 存量 1 号租户（朗汇）种子简称，与其公司信息一致
UPDATE company SET short_name = '朗汇机电' WHERE id = 1;
