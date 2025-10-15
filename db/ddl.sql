-- DDL для событийно-ориентированной системы учета товаров
-- Схема одинакова для обоих шардов

-- Таблица товаров
CREATE TABLE good (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    brand VARCHAR(255),
    price DECIMAL(18,2) NOT NULL DEFAULT 0.00,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_name (name),
    INDEX idx_brand (brand)
);

-- Таблица магазинов (связь товар-магазин)
CREATE TABLE shop (
    id BIGINT NOT NULL,
    good_id BIGINT NOT NULL, -- Ссылается на good.id
    count INT NOT NULL DEFAULT 0 CHECK (count >= 0),
    active BOOLEAN NOT NULL DEFAULT TRUE,
    price DECIMAL(18,2) NOT NULL DEFAULT 0.00,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id, good_id),
    INDEX idx_shop_active (id, active),
    INDEX idx_shop_good (good_id)
);

-- Таблица пользователей
CREATE TABLE user (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    balance DECIMAL(18,2) NOT NULL DEFAULT 0.00,
    role ENUM('customer', 'admin', 'manager') NOT NULL DEFAULT 'customer',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_role (role)
);

-- Таблица транзакций по балансам
CREATE TABLE account_transaction (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    user_id BIGINT NOT NULL, -- Ссылается на user.id
    amount DECIMAL(18,2) NOT NULL,
    type ENUM('credit', 'debit') NOT NULL,
    reason VARCHAR(255),
    idempotency_key VARCHAR(255) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY uk_idempotency_key (idempotency_key),
    INDEX idx_user_id (user_id),
    INDEX idx_created_at (created_at)
);

-- Таблица истории покупок
CREATE TABLE purchase_history (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    order_id VARCHAR(255) NOT NULL,
    user_id BIGINT NOT NULL, -- Ссылается на user.id
    good_id BIGINT NOT NULL, -- Ссылается на good.id
    shop_id BIGINT NOT NULL, -- Ссылается на shop.id
    qty INT NOT NULL,
    price_at_order DECIMAL(18,2) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY uk_order_id (order_id),
    INDEX idx_user_id (user_id),
    INDEX idx_shop_id (shop_id),
    INDEX idx_created_at (created_at)
);

-- Таблица возвратов
CREATE TABLE order_return (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    order_id VARCHAR(255) NOT NULL, -- Ссылается на purchase_history.order_id
    qty INT NOT NULL,
    amount DECIMAL(18,2) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_order_id (order_id),
    INDEX idx_created_at (created_at)
);

-- Таблица событий
CREATE TABLE events (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    event_type TINYINT NOT NULL,
    shop_id BIGINT, -- Ссылается на shop.id (может быть NULL для событий без магазина)
    user_id BIGINT, -- Ссылается на user.id (может быть NULL для событий без пользователя)
    timestamp TIMESTAMP NOT NULL,
    shard TINYINT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_event_type (event_type),
    INDEX idx_shop_id (shop_id),
    INDEX idx_user_id (user_id),
    INDEX idx_shard (shard),
    INDEX idx_timestamp (timestamp),
    INDEX idx_created_at (created_at)
);


-- Создание процедур для работы с балансами
DELIMITER //

CREATE PROCEDURE CreditUserBalance(
    IN p_user_id BIGINT,
    IN p_amount DECIMAL(18,2),
    IN p_reason VARCHAR(255),
    IN p_idempotency_key VARCHAR(255)
)
BEGIN
    DECLARE EXIT HANDLER FOR SQLEXCEPTION
    BEGIN
        ROLLBACK;
        RESIGNAL;
    END;
    
    START TRANSACTION;
    
    -- Проверяем, что транзакция с таким ключом еще не существует
    IF NOT EXISTS (SELECT 1 FROM account_transaction WHERE idempotency_key = p_idempotency_key) THEN
        -- Добавляем транзакцию
        INSERT INTO account_transaction (user_id, amount, type, reason, idempotency_key)
        VALUES (p_user_id, p_amount, 'credit', p_reason, p_idempotency_key);
        
        -- Обновляем баланс пользователя
        UPDATE user SET balance = balance + p_amount WHERE id = p_user_id;
    END IF;
    
    COMMIT;
END //

CREATE PROCEDURE DebitUserBalance(
    IN p_user_id BIGINT,
    IN p_amount DECIMAL(18,2),
    IN p_reason VARCHAR(255),
    IN p_idempotency_key VARCHAR(255)
)
BEGIN
    DECLARE EXIT HANDLER FOR SQLEXCEPTION
    BEGIN
        ROLLBACK;
        RESIGNAL;
    END;
    
    START TRANSACTION;
    
    -- Проверяем, что транзакция с таким ключом еще не существует
    IF NOT EXISTS (SELECT 1 FROM account_transaction WHERE idempotency_key = p_idempotency_key) THEN
        -- Проверяем достаточность средств
        IF (SELECT balance FROM user WHERE id = p_user_id) >= p_amount THEN
            -- Добавляем транзакцию
            INSERT INTO account_transaction (user_id, amount, type, reason, idempotency_key)
            VALUES (p_user_id, p_amount, 'debit', p_reason, p_idempotency_key);
            
            -- Обновляем баланс пользователя
            UPDATE user SET balance = balance - p_amount WHERE id = p_user_id;
        ELSE
            SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'Insufficient balance';
        END IF;
    END IF;
    
    COMMIT;
END //

DELIMITER ;
