# 黑马点评
> Golang + Gin + GORM + MySQL + Redis + Docker复现并优化过的原Java黑马点评，应该是已经实现了原Java中的所有模块，并对一些问题和Go与Java的差异性之间做了一定的优化，目前相对原始Java完成度在100%以上；

## 项目架构
项目使用基于Golang的经典三层架构，分为Handler、Service、Repository三层，分别对应HTTP请求处理、业务逻辑处理和数据访问处理。每一层都可以独立测试和维护，保证了代码的可读性和可维护性。
> Golang提倡"Clean Architecture"或"DDD"的设计理念，强调分层和模块化设计。核心思想是高内聚、低耦合，关于其后端架构的设计理念可以参考`golang-standards/project-layout`，其中详细的介绍了Go后端项目的一种流传颇广的目录骨架，本项目几乎完全学习并采用了其中的理念。
```
DianPingGo/
├── build/                          # 构建输出目录（可忽略）
├── cmd/server/main.go              # 程序入口，解析 -config 参数，启动 App
├── configs/config.yaml             # 配置文件（端口、MySQL、Redis）
├── deploy/nginx/                   # 前端nginx.conf以及前端资源
├── internal/
│   ├── app/app.go                  # App 生命周期：初始化配置→MySQL→Redis→路由→HTTP Server
│   ├── config/config.go            # 配置结构体 + Viper 加载（全局 GlobalConfig）
│   ├── infra/
│   │   ├── mysql.go                # GORM 初始化，连接池配置，全局 *gorm.DB
│   │   └── redis.go                # Redis 初始化，全局 *redis.Client
│   ├── middleware/                 # 中间件（待开发：鉴权、限流、日志等）
│   ├── cache/                      # 缓存层抽象（待开发）
│   ├── router/router.go            # 路由注册：手动组装 Handler → Service → Repository 依赖链
│   └── module/                     # 业务模块（每个模块一个子目录）
│       ├── user/                   # 用户模块
│       │   ├── constants.go        # Redis Key 模板 + TTL 常量
│       │   ├── model.go            # GORM 模型（对应数据表）
│       │   ├── dto.go              # 请求/响应 DTO（含 binding 标签）
│       │   ├── handler.go          # Gin Handler — 参数绑定 → 调 Service → 统一响应
│       │   ├── service.go          # 业务逻辑层
│       │   └── repository.go       # 数据访问层（GORM 操作）
│       ├── blog/                   # 博客/探店笔记模块
│       ├── follow/                 # 关注模块
│       ├── shop/                   # 店铺模块
│       ├── shoptype/               # 店铺类型模块
│       ├── voucher/                # 优惠券模块
│       └── voucherorder/           # 优惠券订单模块
├── pkg/
│   ├── errmsg/errmsg.go            # 统一错误码体系（CustomError：HTTP状态码 + 业务码 + 消息）
│   ├── response/response.go        # 统一 API 响应格式（OK / Fail 两个出口）
│   └── validator/
│       ├── validator.go            # 自定义正则校验（手机号、邮箱、用户名）
│       └── register.go             # 向 Gin validator 注册自定义校验（如 zh_mobile）
├── script/                         # 脚本
├── hmdp.sql                        # 数据库初始化 SQL
├── go.mod
└── go.sum
```
## Quick Start
后端前端均可通过Docker部署，后端也可以通过Make部署，最快的部署方案即为：
```sh
docker compose up -d --build
```
这应该会自动的补全相应的go依赖并启动前后端服务，接下来直接在浏览器中访问`http://127.0.0.1:8080`即可访问服务。如果有必要，也可以通过`make build`和`make run`来分别构建和运行后端服务。
> 关于前端，基础的运行应该没什么问题，个人没有太关心前端的精细实现和启动，一个是因为前端是复用的Java版本，内部有大量通过camelCase(如`data.shopId`)访问后端返回的JSON数据的代码，但是Go版本的后端返回的JSON数据是通过snake_case(如`data.shop_id`)访问的，所以前端有大量的代码需要修改；另外一个是因为前端的代码本身实现的就十分简陋，很多功能、按钮、界面都没有实现，压测也不用前端直接用Postman和其他的工具就可以完成，如果想学全栈，这个前端代码我也认为没有太多的学习价值。

如果想精细化了解本项目的实现和优化，最好还是用LLM分析，因为我的博客还没写完和上线:)

## Further
如果发现我写的代码有问题，或者有更好的实现方案，欢迎提交Issue或PR，这也是我第一份完整的Go后端项目，出错应该也是必然，虽然我已经拿codex扫了三遍需要优化处理的地方了...

主分支的代码已经足够完整，应该不会做什么大的更新，未来应该会开新分支做一些魔改和重构，比如添加消息队列、多层缓存、可观测与故障分析等等等等...........

唉，要学的还有很多