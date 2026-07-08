CREATE TABLE `tb_seckill_voucher` (
    `voucher_id` bigint(20) unsigned NOT NULL COMMENT '关联优惠券ID',
    `stock` int(10) unsigned NOT NULL COMMENT '秒杀库存',
    `begin_time` datetime NOT NULL COMMENT '秒杀开始时间',
    `end_time` datetime NOT NULL COMMENT '秒杀结束时间',
    `create_time` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    `update_time` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    PRIMARY KEY (`voucher_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci COMMENT='秒杀优惠券表';
