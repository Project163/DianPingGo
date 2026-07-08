CREATE TABLE `tb_voucher` (
    `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '优惠券ID',
    `shop_id` bigint(20) unsigned NOT NULL COMMENT '商户ID',
    `title` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '优惠券标题',
    `sub_title` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '优惠券副标题',
    `rules` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '使用规则',
    `pay_value` bigint(20) unsigned NOT NULL COMMENT '支付金额',
    `actual_value` bigint(20) unsigned NOT NULL COMMENT '实际价值',
    `type` tinyint(1) unsigned NOT NULL DEFAULT 0 COMMENT '优惠券类型：0 普通券，1 秒杀券',
    `status` tinyint(1) unsigned NOT NULL DEFAULT 1 COMMENT '状态：0 禁用，1 启用',
    `stock` int(10) unsigned NOT NULL COMMENT '库存',
    `begin_time` datetime NOT NULL COMMENT '开始时间',
    `end_time` datetime NOT NULL COMMENT '结束时间',
    `create_time` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    `update_time` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    PRIMARY KEY (`id`),
    KEY `idx_shop_id` (`shop_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci COMMENT='优惠券表';

CREATE TABLE `tb_seckill_voucher` (
    `voucher_id` bigint(20) unsigned NOT NULL COMMENT '关联优惠券ID',
    `stock` int(10) unsigned NOT NULL COMMENT '秒杀库存',
    `begin_time` datetime NOT NULL COMMENT '秒杀开始时间',
    `end_time` datetime NOT NULL COMMENT '秒杀结束时间',
    `create_time` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    `update_time` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    PRIMARY KEY (`voucher_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci COMMENT='秒杀优惠券表';
