CREATE TABLE `tb_user_info` (
    `user_id` bigint(20) unsigned NOT NULL COMMENT '用户ID',
    `city` varchar(64) NOT NULL DEFAULT '' COMMENT '城市',
    `introduce` varchar(128) DEFAULT NULL COMMENT '简介',
    `fans` int(8) unsigned NOT NULL DEFAULT 0 COMMENT '粉丝数',
    `followee` int(8) unsigned NOT NULL DEFAULT 0 COMMENT '关注数',
    `gender` tinyint(1) unsigned NOT NULL DEFAULT 0 COMMENT '性别 0:未设置 1:男 2:女',
    `birthday` date DEFAULT NULL COMMENT '生日',
    `credits` int(8) unsigned NOT NULL DEFAULT 0 COMMENT '积分',
    `level` tinyint(1) unsigned NOT NULL DEFAULT 0 COMMENT '等级',
    `created_time` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    `updated_time` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    PRIMARY KEY (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci COMMENT='用户信息表';
