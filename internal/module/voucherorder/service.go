package voucherorder

import (
	"context"
	"strconv"
	"sync"
	"time"

	"dianping/internal/module/seckillvoucher"
	"dianping/pkg/errmsg"
	"dianping/pkg/idgen"

	"github.com/redis/go-redis/v9"
)

type VoucherOrderRepository interface {
	CreateVoucherOrder(ctx context.Context, order *VoucherOrder) error
	CountByUserAndVoucher(ctx context.Context, userID uint64, voucherID uint64) (int64, error)
	GetVoucherOrderByID(ctx context.Context, orderID uint64) (*VoucherOrder, error)
}

type SeckillVoucherRepository interface {
	GetSeckillVoucherByID(ctx context.Context, voucherID uint64) (*seckillvoucher.SeckillVoucher, error)
	DeductStock(ctx context.Context, voucherID uint64) (bool, error)
}

// 先用SISMEMBER来判断用户是否已经下过单
// 如果已经下过单，返回2表示重复下单
// 如果没下过单，继续判断库存是否足够
// 如果库存不足，返回1表示库存不足
// 如果库存足够，扣减库存，SADD用户ID到订单集合中，XADD消息到Stream中，返回0表示下单成功
var SeckillLuaScript = redis.NewScript(`
local voucherId = ARGV[1]
local userId = ARGV[2]
local orderId = ARGV[3]

local stockKey = 'seckill:stock:' .. voucherId
local orderKey = 'seckill:order:' .. voucherId .. ':' .. userId
local streamKey = 'stream.orders'

local exists = redis.call('SISMEMBER', orderKey, userId)
if exists == 1 then
    return 2
end

local stock = redis.call('GET', stockKey)
if not stock or tonumber(stock) <= 0 then
    return 1
end

redis.call('DECRBY', stockKey, 1)

redis.call('SADD', orderKey, userId)

redis.call('XADD', streamKey, '*', 'userId', userId, 'voucherId', voucherId, 'orderId', orderId)

return 0
`)

var unlockScript = redis.NewScript(`
local key = KEYS[1]
local token = ARGV[1]

if redis.call("GET", key) == token then
	return redis.call("DEL", key)
else
	return 0
end
`)

type Service struct {
	rdb         redis.Cmdable
	repo        VoucherOrderRepository
	seckillRepo SeckillVoucherRepository
	idWorker    *idgen.RedisIDWorker

	mu      sync.Mutex
	started bool
}

func NewService(repo VoucherOrderRepository, seckillRepo SeckillVoucherRepository, rdb redis.Cmdable, idWorker *idgen.RedisIDWorker) *Service {
	return &Service{
		repo:        repo,
		seckillRepo: seckillRepo,
		rdb:         rdb,
		idWorker:    idWorker,
	}
}

func (s *Service) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return
	}
	s.started = true

	// 在服务启动时创建消费者组，组键为 StreamOrderKey，组名为 StreamGroupName，起始ID为 "0"
	s.rdb.XGroupCreateMkStream(context.Background(), StreamOrderKey, StreamGroupName, "0")
	// 启动一个 goroutine 来消费订单消息
	go s.StreamConsumer()
}

func (s *Service) SeckillVoucher(ctx context.Context, voucherID uint64, userID uint64) (int64, error) {
	orderID, err := s.idWorker.NextID(ctx, "order")
	if err != nil {
		return 0, err
	}
	res, err := SeckillLuaScript.Run(ctx, s.rdb, []string{}, voucherID, userID, orderID).Int()

	if err != nil {
		return 0, err
	}

	switch res {
	case SeckillSuccess:
		return orderID, nil
	case SeckillNoStock:
		return 0, &errmsg.ErrNoStock
	case SeckillRepeatedOrder:
		return 0, &errmsg.ErrRepeatedOrder
	default:
		return 0, &errmsg.ErrInternalSec
	}
}

func (s *Service) CreateVoucherOrder(ctx context.Context, order *VoucherOrder) error {
	ok, err := s.seckillRepo.DeductStock(ctx, order.VoucherID)
	if err != nil {
		return err
	}
	if !ok {
		return &errmsg.ErrNoStock
	}
	return s.repo.CreateVoucherOrder(ctx, order)
}

func (s *Service) GetVoucherOrderByID(ctx context.Context, orderID uint64) (*VoucherOrder, error) {
	order, err := s.repo.GetVoucherOrderByID(ctx, orderID)
	if order == nil {
		return nil, &errmsg.ErrOrderNotFound
	}
	if err != nil {
		return nil, err
	}
	return order, nil
}

func (s *Service) HandleVoucherOrder(ctx context.Context, order *VoucherOrder) error {
	// 对于单条订单，用分布式锁来控制同一用户的并发请求，锁的键为 LockOrderKey + userID，过期时间为 10 秒
	lockKey := LockOrderKey + strconv.FormatUint(order.UserID, 10)
	ok, err := s.rdb.SetNX(ctx, lockKey, order.UserID, 10*time.Second).Result()
	if err != nil {
		return err
	}
	if !ok {
		return &errmsg.ErrRepeatedOrder
	}
	defer func() {
		unlockScript.Run(ctx, s.rdb, []string{lockKey}, order.UserID)
	}()

	return s.CreateVoucherOrder(ctx, order)
}

func (s *Service) StreamConsumer() {
	ctx := context.Background()
	// 创建一个循环持续订阅消息
	for {
		// 从 Redis Stream 中读取消息，使用消费者组的方式，阻塞等待新消息到来
		msgs, err := s.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    StreamGroupName,
			Consumer: StreamConsumerName,
			Streams:  []string{StreamOrderKey, ">"},
			Count:    1,
			Block:    2 * time.Second,
		}).Result()
		// 如果消费者异常崩溃或处理失败（无XACK），消息会留在消费者组的待处理列表中（Pending List）
		// 通过HandlePendingList方法来处理这些待处理的消息，确保消息不会丢失
		if err != nil {
			s.HandlePendingList()
			continue
		}
		// 对于读取到的每条消息，进行处理，streams是一个包含StreamOrderKey和消息列表的结构体
		// msgs是一个包含多个这样的结构体的切片
		for _, stream := range msgs {
			// 解析消息内容，构造订单对象，并调用HandleVoucherOrder方法处理订单
			for _, msg := range stream.Messages {
				order := ParseMessage(msg.Values)
				if order == nil {
					continue
				}
				// 对于每条解析后的订单消息，调用HandleVoucherOrder方法进行处理
				// 处理完成后无论成功与否，都使用XACK命令确认消息已被处理，从消费者组的待处理列表中移除该消息
				// 防止消息重复处理和阻塞，确保系统的可靠性和一致性
				if err := s.HandleVoucherOrder(ctx, order); err != nil {
					s.rdb.XAck(ctx, StreamOrderKey, StreamGroupName, msg.ID)
					continue
				}
				s.rdb.XAck(ctx, StreamOrderKey, StreamGroupName, msg.ID)
			}
		}
	}
}

func (s *Service) HandlePendingList() {
	ctx := context.Background()
	// 通过“0”得到已投递但未处理的消息列表
	for {
		msgs, err := s.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    StreamGroupName,
			Consumer: StreamConsumerName,
			Streams:  []string{StreamOrderKey, "0"},
			Count:    1,
		}).Result()
		if err != nil || len(msgs) == 0 || len(msgs[0].Messages) == 0 {
			return
		}

		// 对于每条待处理的消息，进行处理
		for _, stream := range msgs {
			for _, msg := range stream.Messages {
				order := ParseMessage(msg.Values)
				if order == nil {
					continue
				}
				s.HandleVoucherOrder(ctx, order)
				s.rdb.XAck(ctx, StreamOrderKey, StreamGroupName, msg.ID)
			}
		}
	}
}

func ParseMessage(values map[string]interface{}) *VoucherOrder {
	if values == nil {
		return nil
	}
	vid, _ := strconv.ParseUint(strVal(values["voucherId"]), 10, 64)
	uid, _ := strconv.ParseUint(strVal(values["userId"]), 10, 64)
	oid, _ := strconv.ParseUint(strVal(values["orderId"]), 10, 64)

	if vid == 0 || uid == 0 || oid == 0 {
		return nil
	}
	return &VoucherOrder{
		ID:        oid,
		UserID:    uid,
		VoucherID: vid,
	}
}

func strVal(v interface{}) string {
	if v == nil {
		return ""
	}
	return v.(string)
}
