CREATE TABLE `tb_voucher_order` (
    `id` bigint(20) unsigned NOT NULL COMMENT '订单ID（全局唯一，非自增）',
    `user_id` bigint(20) unsigned NOT NULL COMMENT '用户ID',
    `voucher_id` bigint(20) unsigned NOT NULL COMMENT '优惠券ID',
    `pay_type` tinyint(1) unsigned NOT NULL DEFAULT 0 COMMENT '支付方式',
    `status` tinyint(1) unsigned NOT NULL DEFAULT 0 COMMENT '订单状态',
    `create_time` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    `pay_time` datetime DEFAULT NULL COMMENT '支付时间',
    `use_time` datetime DEFAULT NULL COMMENT '使用时间',
    `refund_time` datetime DEFAULT NULL COMMENT '退款时间',
    `update_time` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    PRIMARY KEY (`id`),
    KEY `idx_user_voucher` (`user_id`, `voucher_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci COMMENT='优惠券订单表';
