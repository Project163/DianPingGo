CREATE TABLE `tb_shop` (
    `id` bigint(20) UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
    `name` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '商铺名称',
    `type_id` bigint(20) UNSIGNED NOT NULL COMMENT '商铺类型的id',
    `images` varchar(1024) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '商铺图片',
    `area` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NULL DEFAULT NULL COMMENT '商圈',
    `address` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT '地址',
    `x` double UNSIGNED NOT NULL COMMENT '经度',
    `y` double UNSIGNED NOT NULL COMMENT '维度',
    `avg_price` bigint(10) UNSIGNED NULL DEFAULT NULL COMMENT '均价',
    `sold` int(10) UNSIGNED ZEROFILL NOT NULL COMMENT '销量',
    `comments` int(10) UNSIGNED ZEROFILL NOT NULL COMMENT '评论数量',
    `score` int(2) UNSIGNED ZEROFILL NOT NULL COMMENT '评分',
    `open_hours` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NULL DEFAULT NULL COMMENT '营业时间',
    `create_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    `update_time` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    PRIMARY KEY (`id`) USING BTREE,
    INDEX `foreign_key_type`(`type_id`) USING BTREE
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_general_ci;
