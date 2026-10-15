CREATE TABLE IF NOT EXISTS employee (
  staff_id      BIGINT PRIMARY KEY,
  name          VARCHAR(64)  NOT NULL DEFAULT '',
  title         VARCHAR(64)  NOT NULL DEFAULT '',
  avatar        VARCHAR(255) NOT NULL DEFAULT '',
  phone         VARCHAR(32)  NOT NULL DEFAULT '',
  email         VARCHAR(128) NOT NULL DEFAULT '',
  password_hash VARCHAR(255) NOT NULL DEFAULT '',
  status        VARCHAR(16)  NOT NULL DEFAULT 'enabled',
  created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS company_info (
  id        BIGINT PRIMARY KEY AUTO_INCREMENT,
  full_name VARCHAR(128) NOT NULL DEFAULT '',
  address   VARCHAR(255) NOT NULL DEFAULT '',
  tel       VARCHAR(32)  NOT NULL DEFAULT '',
  logo      VARCHAR(255) NOT NULL DEFAULT ''
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS company_profile (
  id               BIGINT PRIMARY KEY AUTO_INCREMENT,
  intro_text       TEXT,
  video_channel_id VARCHAR(64)  NOT NULL DEFAULT '',
  backup_mp4_url   VARCHAR(512) NOT NULL DEFAULT '',
  cover            VARCHAR(512) NOT NULL DEFAULT ''
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS achievement_category (
  id        BIGINT PRIMARY KEY AUTO_INCREMENT,
  name      VARCHAR(64) NOT NULL DEFAULT '',
  icon      VARCHAR(512) NOT NULL DEFAULT '',
  sort      INT NOT NULL DEFAULT 0,
  is_locked TINYINT(1) NOT NULL DEFAULT 0
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS project_case (
  id               BIGINT PRIMARY KEY AUTO_INCREMENT,
  category_id      BIGINT NOT NULL DEFAULT 0,
  name             VARCHAR(128) NOT NULL DEFAULT '',
  summary          VARCHAR(512) NOT NULL DEFAULT '',
  detail           TEXT,
  cover            VARCHAR(512) NOT NULL DEFAULT '',
  album            JSON,
  is_home_featured TINYINT(1) NOT NULL DEFAULT 0,
  enabled          TINYINT(1) NOT NULL DEFAULT 1,
  sort             INT NOT NULL DEFAULT 0,
  created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY idx_cat (category_id),
  KEY idx_featured (is_home_featured, enabled)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS biz_service (
  id        BIGINT PRIMARY KEY AUTO_INCREMENT,
  title     VARCHAR(128) NOT NULL DEFAULT '',
  content   TEXT,
  sort      INT NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS home_layout (
  id        BIGINT PRIMARY KEY AUTO_INCREMENT,
  order_col JSON
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS stat_event (
  id         BIGINT PRIMARY KEY AUTO_INCREMENT,
  staff_id   BIGINT NOT NULL DEFAULT 0,
  event_type VARCHAR(16) NOT NULL DEFAULT '',
  stat_date  DATE NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY idx_staff_date (staff_id, stat_date),
  KEY idx_event (event_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 种子：经典业绩 7 分类（客户确认）
INSERT INTO achievement_category (id, name, icon, sort, is_locked) VALUES
  (1, '居住住宅',             '', 1, 0),
  (2, '政务办公',             '', 2, 0),
  (3, '医院、学校',           '', 3, 0),
  (4, '商业、酒店',           '', 4, 0),
  (5, '公共交通',             '', 5, 0),
  (6, '生产制造、仓储物流',   '', 6, 0),
  (7, '私人住宅',             '', 7, 0);
