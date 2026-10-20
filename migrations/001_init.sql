CREATE TABLE IF NOT EXISTS users (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  email VARCHAR(190) NOT NULL UNIQUE,
  password_hash VARCHAR(255) NOT NULL,
  role ENUM('user','admin') NOT NULL DEFAULT 'user',
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS addresses (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  user_id BIGINT NOT NULL,
  recipient VARCHAR(100) NOT NULL,
  phone VARCHAR(32) NOT NULL,
  detail VARCHAR(500) NOT NULL,
  is_default BOOLEAN NOT NULL DEFAULT FALSE,
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  INDEX idx_addresses_user (user_id),
  FOREIGN KEY (user_id) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS revoked_tokens (
  jti CHAR(36) PRIMARY KEY,
  expires_at DATETIME(6) NOT NULL,
  INDEX idx_revoked_expiry (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS products (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  category ENUM('laptop','monitor','dock') NOT NULL,
  brand VARCHAR(100) NOT NULL,
  title VARCHAR(255) NOT NULL,
  description TEXT NOT NULL,
  image_url VARCHAR(1000) NOT NULL DEFAULT '',
  active BOOLEAN NOT NULL DEFAULT TRUE,
  source_kind VARCHAR(32) NOT NULL DEFAULT 'demo',
  source_url VARCHAR(1000) NOT NULL DEFAULT '',
  source_captured_at DATETIME(6) NULL,
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  INDEX idx_product_category_active (category,active),
  FULLTEXT INDEX ft_product_text (title,description) WITH PARSER ngram
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS skus (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  product_id BIGINT NOT NULL,
  label VARCHAR(255) NOT NULL,
  specs JSON NOT NULL,
  price_cents BIGINT NOT NULL,
  price_version BIGINT NOT NULL DEFAULT 1,
  available INT NOT NULL DEFAULT 0,
  reserved INT NOT NULL DEFAULT 0,
  sold INT NOT NULL DEFAULT 0,
  stock_version BIGINT NOT NULL DEFAULT 1,
  active BOOLEAN NOT NULL DEFAULT TRUE,
  updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  FOREIGN KEY (product_id) REFERENCES products(id),
  INDEX idx_sku_product (product_id),
  CONSTRAINT chk_sku_price CHECK (price_cents >= 0),
  CONSTRAINT chk_sku_stock CHECK (available >= 0 AND reserved >= 0 AND sold >= 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS price_history (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  sku_id BIGINT NOT NULL,
  old_price_cents BIGINT NOT NULL,
  new_price_cents BIGINT NOT NULL,
  price_version BIGINT NOT NULL,
  actor_user_id BIGINT NOT NULL,
  reason VARCHAR(500) NOT NULL,
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  FOREIGN KEY (sku_id) REFERENCES skus(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS stock_movements (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  sku_id BIGINT NOT NULL,
  order_id CHAR(36) NULL,
  event_key VARCHAR(120) NOT NULL UNIQUE,
  delta_available INT NOT NULL,
  delta_reserved INT NOT NULL,
  delta_sold INT NOT NULL,
  reason VARCHAR(500) NOT NULL,
  actor_user_id BIGINT NULL,
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  INDEX idx_stock_movements_sku (sku_id),
  FOREIGN KEY (sku_id) REFERENCES skus(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS cart_items (
  user_id BIGINT NOT NULL,
  sku_id BIGINT NOT NULL,
  quantity INT NOT NULL,
  selected BOOLEAN NOT NULL DEFAULT TRUE,
  updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (user_id,sku_id),
  FOREIGN KEY (user_id) REFERENCES users(id),
  FOREIGN KEY (sku_id) REFERENCES skus(id),
  CONSTRAINT chk_cart_qty CHECK (quantity > 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS checkout_previews (
  id CHAR(36) PRIMARY KEY,
  user_id BIGINT NOT NULL,
  address_id BIGINT NOT NULL,
  items_snapshot JSON NOT NULL,
  address_snapshot JSON NOT NULL,
  amount_cents BIGINT NOT NULL,
  expires_at DATETIME(6) NOT NULL,
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  INDEX idx_preview_user (user_id,created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS orders (
  id CHAR(36) PRIMARY KEY,
  user_id BIGINT NOT NULL,
  request_id VARCHAR(100) NOT NULL,
  preview_id CHAR(36) NOT NULL,
  status ENUM('PENDING_PAYMENT','PAID','CANCELLED','CLOSED') NOT NULL,
  amount_cents BIGINT NOT NULL,
  address_snapshot JSON NOT NULL,
  expires_at DATETIME(6) NOT NULL,
  paid_at DATETIME(6) NULL,
  closed_at DATETIME(6) NULL,
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  UNIQUE KEY uniq_order_request (user_id,request_id),
  INDEX idx_orders_due (status,expires_at),
  INDEX idx_orders_user (user_id,created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS order_items (
  order_id CHAR(36) NOT NULL,
  sku_id BIGINT NOT NULL,
  product_title VARCHAR(255) NOT NULL,
  sku_label VARCHAR(255) NOT NULL,
  specs_snapshot JSON NOT NULL,
  unit_price_cents BIGINT NOT NULL,
  price_version BIGINT NOT NULL,
  quantity INT NOT NULL,
  PRIMARY KEY (order_id,sku_id),
  FOREIGN KEY (order_id) REFERENCES orders(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS payment_events (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  event_id CHAR(36) NOT NULL UNIQUE,
  order_id CHAR(36) NOT NULL,
  amount_cents BIGINT NOT NULL,
  outcome VARCHAR(40) NOT NULL,
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  INDEX idx_payment_order (order_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS trade_anomalies (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  order_id CHAR(36) NOT NULL,
  event_id CHAR(36) NOT NULL,
  reason VARCHAR(255) NOT NULL,
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  UNIQUE KEY uniq_anomaly_event (event_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS guide_sessions (
  id CHAR(36) PRIMARY KEY,
  user_id BIGINT NOT NULL,
  intent_json JSON NOT NULL,
  intent_version INT NOT NULL DEFAULT 1,
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  INDEX idx_guide_user (user_id,updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS guide_messages (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  session_id CHAR(36) NOT NULL,
  user_id BIGINT NOT NULL,
  role ENUM('user','assistant') NOT NULL,
  content TEXT NOT NULL,
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  INDEX idx_message_session (session_id,id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS guide_proposals (
  id CHAR(36) PRIMARY KEY,
  session_id CHAR(36) NOT NULL,
  user_id BIGINT NOT NULL,
  intent_version INT NOT NULL,
  proposal_version INT NOT NULL,
  items_json JSON NOT NULL,
  evidence_json JSON NOT NULL,
  amount_cents BIGINT NOT NULL,
  confirmed_at DATETIME(6) NULL,
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  UNIQUE KEY uniq_proposal_version (session_id,proposal_version),
  INDEX idx_proposal_user (user_id,session_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS guide_tool_runs (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  session_id CHAR(36) NOT NULL,
  proposal_id CHAR(36) NULL,
  tool_name VARCHAR(80) NOT NULL,
  input_summary JSON NOT NULL,
  result_summary JSON NOT NULL,
  duration_ms INT NOT NULL,
  error_code VARCHAR(80) NOT NULL DEFAULT '',
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  INDEX idx_tool_session (session_id,id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
