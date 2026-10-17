-- 0010：微信身份采集（openid + 昵称），与 devserver AutoMigrate 保持一致
-- openid：用户在本小程序内的唯一标识（wx.login → jscode2session 换取），供后台核实申请人/登录者身份。
-- nickname：微信昵称（用户授权后采集）。申请开通时记录，审核通过一并写入员工表。

ALTER TABLE open_apply ADD COLUMN openid VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE open_apply ADD COLUMN nickname VARCHAR(64) NOT NULL DEFAULT '';

ALTER TABLE employee ADD COLUMN openid VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE employee ADD COLUMN nickname VARCHAR(64) NOT NULL DEFAULT '';
