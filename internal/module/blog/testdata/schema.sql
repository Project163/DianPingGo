CREATE TABLE `tb_blog` (
    `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
    `shop_id` bigint(20) unsigned NOT NULL COMMENT '商户id',
    `user_id` bigint(20) unsigned NOT NULL COMMENT '用户id',
    `title` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL DEFAULT '' COMMENT '标题',
    `images` varchar(2048) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL DEFAULT '' COMMENT '探店的照片，最多9张，多张以逗号分隔',
    `content` varchar(2048) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL DEFAULT '' COMMENT '探店的文字描述',
    `liked` int(9) unsigned NOT NULL DEFAULT '0' COMMENT '点赞数量',
    `comments` int(9) unsigned DEFAULT '0' COMMENT '评论数量',
    `create_time` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    `update_time` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci COMMENT='探店笔记表';
