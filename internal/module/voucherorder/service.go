package voucherorder

import (
	"context"
	"errors"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"dianping/internal/module/seckillvoucher"
	"dianping/internal/tx"
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
local activityKey = KEYS[1]
local orderKey = KEYS[2]
local streamKey = KEYS[3]

local voucherId = ARGV[1]
local userId = ARGV[2]
local orderId = ARGV[3]

if redis.call('TYPE', activityKey).ok ~= 'hash' then
    return 3
end

local data = redis.call('HMGET', activityKey,
    'state', 'begin_ms', 'end_ms', 'stock',
    'init_token', 'initial_stock')

local beginMs = tonumber(data[2])
local endMs = tonumber(data[3])
local stock = tonumber(data[4])
local initial = tonumber(data[6])

if data[1] ~= 'READY'
    or not data[5] or data[5] == ''
    or not beginMs or not endMs or endMs <= beginMs
    or not initial or initial <= 0
    or not stock or stock < 0 or stock > initial
    or stock ~= math.floor(stock) then
    return 3
end

local t = redis.call('TIME')
local nowMs = tonumber(t[1]) * 1000
    + math.floor(tonumber(t[2]) / 1000)

if nowMs < beginMs then
    return 4
end

if nowMs >= endMs then
    return 5
end

-- 在任何写入前检查类型，避免可预见的 WRONGTYPE 部分写入。
local orderType = redis.call('TYPE', orderKey).ok
local streamType = redis.call('TYPE', streamKey).ok

if orderType ~= 'none' and orderType ~= 'set' then
    return 6
end

if streamType ~= 'none' and streamType ~= 'stream' then
    return 6
end

if redis.call('SISMEMBER', orderKey, userId) == 1 then
    return 2
end

if stock <= 0 then
    return 1
end

redis.call('HINCRBY', activityKey, 'stock', -1)
redis.call('SADD', orderKey, userId)
redis.call('XADD', streamKey, '*',
    'user_id', userId,
    'voucher_id', voucherId,
    'order_id', orderId)

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
	txManager   tx.Manager

	mu      sync.Mutex
	started bool

	ctx    context.Context
	cancel context.CancelFunc
}

func NewService(repo VoucherOrderRepository, seckillRepo SeckillVoucherRepository, rdb redis.Cmdable, idWorker *idgen.RedisIDWorker, txManager tx.Manager) *Service {
	return &Service{
		repo:        repo,
		seckillRepo: seckillRepo,
		rdb:         rdb,
		idWorker:    idWorker,
		txManager:   txManager,
	}
}

// Start 启动服务，创建消费者组并启动消息消费 goroutine，消费者组的消息来自Lua脚本(XADD)
func (s *Service) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return
	}
	s.ctx, s.cancel = context.WithCancel(context.Background())

	// 在服务启动时创建消费者组，组键为 StreamOrderKey，组名为 StreamGroupName，起始ID为 "0"
	if err := s.rdb.XGroupCreateMkStream(context.Background(), StreamOrderKey, StreamGroupName, "0").Err(); err != nil {
		if !strings.HasPrefix(err.Error(), "BUSYGROUP") {
			log.Printf("创建消费者组失败：%v", err)
			return
		}
	}

	s.started = true

	go s.consumeNewMessages()
	go s.consumePendingMessages()
}

func (s *Service) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started {
		return
	}
	if s.cancel != nil {
		s.cancel()
	}
	s.started = false
}

// SeckillVoucher 处理秒杀优惠券请求，接受优惠券ID和用户ID作为参数，调用Lua脚本进行秒杀逻辑，并返回订单ID或错误
func (s *Service) SeckillVoucher(
	ctx context.Context,
	voucherID uint64,
	userID uint64,
) (int64, error) {
	if voucherID == 0 || userID == 0 {
		return 0, &errmsg.ErrInvalidParam
	}

	orderID, err := s.idWorker.NextID(ctx, "order")
	if err != nil {
		return 0, errmsg.NewError(errmsg.ErrInternalSec, err)
	}

	voucherText := strconv.FormatUint(voucherID, 10)
	userText := strconv.FormatUint(userID, 10)

	keys := []string{
		seckillvoucher.ActivityKey(voucherID),
		"seckill:order:" + voucherText + ":" + userText,
		StreamOrderKey,
	}

	result, err := SeckillLuaScript.Run(
		ctx, s.rdb, keys, voucherID, userID, orderID,
	).Int()
	if err != nil {
		return 0, errmsg.NewError(errmsg.ErrInternalSec, err)
	}

	switch result {
	case SeckillSuccess:
		return orderID, nil
	case SeckillNoStock:
		return 0, &errmsg.ErrNoStock
	case SeckillRepeatedOrder:
		return 0, &errmsg.ErrRepeatedOrder
	case SeckillNotReady:
		return 0, &errmsg.ErrSeckillNotReady
	case SeckillNotStarted:
		return 0, &errmsg.ErrSeckillNotStarted
	case SeckillEnded:
		return 0, &errmsg.ErrSeckillEnded
	case SeckillDataError:
		return 0, &errmsg.ErrInternalSec
	default:
		return 0, &errmsg.ErrInternalSec
	}
}

// CreateVoucherOrder 在数据库中创建一个新的优惠券订单记录，接受上下文和优惠券订单对象作为参数，返回错误
func (s *Service) CreateVoucherOrder(ctx context.Context, order *VoucherOrder) error {
	return s.txManager.Transaction(ctx, func(ctx context.Context) error {
		ok, err := s.seckillRepo.DeductStock(ctx, order.VoucherID)
		if err != nil {
			return err
		}
		if !ok {
			return &errmsg.ErrNoStock
		}
		return s.repo.CreateVoucherOrder(ctx, order)
	})
}

// GetVoucherOrderByID 根据订单ID查询优惠券订单，接受上下文和订单ID作为参数，返回优惠券订单对象和错误
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

// HandleVoucherOrder 处理优惠券订单，实现一人一单
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

func (s *Service) consumeNewMessages() {
	// 处理新消息的逻辑
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			// 读取新消息并处理
		}

		msgs, err := s.rdb.XReadGroup(s.ctx, &redis.XReadGroupArgs{
			Group:    StreamGroupName,
			Consumer: StreamConsumerName,
			Streams:  []string{StreamOrderKey, ">"},
			Count:    1,
			Block:    2 * time.Second,
		}).Result()

		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			if errors.Is(err, redis.Nil) {
				continue
			}
			log.Printf("读取Stream新消息失败：%v", err)
			continue
		}

		for _, msg := range msgs[0].Messages {
			order := ParseMessage(msg.Values)
			s.processMessage(s.ctx, msg.ID, order)
		}
	}
}

func (s *Service) consumePendingMessages() {
	// 处理待处理消息的逻辑
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			s.processPendingList()
			return
		case <-ticker.C:
			s.processPendingList()
		}
	}
}

func (s *Service) processPendingList() {
	ctx := context.Background()
	// 通过“0”得到已投递但未处理的消息列表
	msgs, err := s.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    StreamGroupName,
		Consumer: StreamConsumerName,
		Streams:  []string{StreamOrderKey, "0"},
		Count:    int64(PendingBatchSize),
		Block:    0,
	}).Result()

	if err != nil || len(msgs) == 0 || len(msgs[0].Messages) == 0 {
		return
	}
	// 对于每条待处理的消息，进行处理
	for _, msg := range msgs[0].Messages {
		order := ParseMessage(msg.Values)
		s.processMessage(ctx, msg.ID, order)
	}
}

// processMessage 处理单条消息，调用 HandleVoucherOrder 方法处理订单，并根据处理结果进行确认或重试
func (s *Service) processMessage(ctx context.Context, msgID string, order *VoucherOrder) {
	if order == nil {
		// 对于无法解析的消息，直接确认并删除，避免阻塞消费者组
		s.rdb.XAck(ctx, StreamOrderKey, StreamGroupName, msgID)
		return
	}

	err := s.HandleVoucherOrder(ctx, order)
	if err == nil {
		// 处理成功，确认消息
		s.rdb.XAck(ctx, StreamOrderKey, StreamGroupName, msgID)
		s.rdb.HDel(ctx, RetryKey, msgID)
		return
	}

	// 判断是否为永久性错误，如果是，则将消息移动到死信队列，并确认消息
	if isPermanentError(err) {
		s.moveToDLQ(ctx, msgID, order, err, -1)
		s.rdb.XAck(ctx, StreamOrderKey, StreamGroupName, msgID)
		s.rdb.HDel(ctx, RetryKey, msgID)
		log.Printf("[voucherorder] 订单 %d 重试%d次后仍重试，已加入DLQ：%v",
			order.ID, -1, err)
		return
	}
	// 对于非永久性错误，增加重试计数
	retryCount, _ := s.rdb.HIncrBy(ctx, RetryKey, msgID, 1).Result()
	if retryCount > MaxRetries {
		// 超过最大重试次数，将消息移动到死信队列，并确认消息
		s.moveToDLQ(ctx, msgID, order, err, int(retryCount))
		s.rdb.XAck(ctx, StreamOrderKey, StreamGroupName, msgID)
		s.rdb.HDel(ctx, RetryKey, msgID)
		log.Printf("[voucherorder] 订单 %d 重试%d次后仍重试，已加入DLQ：%v",
			order.ID, retryCount, err)
		return
	}

	log.Printf("[voucherorder] 订单 %d 处理失败(第%d/%d次): %v，留在PEL等待重试",
		order.ID, 1, MaxRetries, err)
}

func isPermanentError(err error) bool {
	var ce *errmsg.CustomError
	// 使用 errors.As 检查错误类型，如果是自定义错误类型，则根据具体的错误类型判断是否为永久性错误
	if errors.As(err, &ce) {
		switch {
		case ce.BusinessCode == errmsg.ErrNoStock.BusinessCode:
			return true
		case ce.BusinessCode == errmsg.ErrRepeatedOrder.BusinessCode:
			return true
		case ce.BusinessCode == errmsg.ErrInvalidParam.BusinessCode:
			return true
		default:
			return false
		}
	}
	return false
}

// moveToDLQ 将处理失败的消息移动到死信队列，记录原始消息ID、订单信息、错误信息和重试次数
func (s *Service) moveToDLQ(ctx context.Context, msgID string, order *VoucherOrder, err error, retryCount int) {
	s.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: DeadStreamKey,
		Values: map[string]interface{}{
			"original_id": msgID,
			"user_id":     order.UserID,
			"voucher_id":  order.VoucherID,
			"order_id":    order.ID,
			"error":       err.Error(),
			"retry_count": retryCount,
			"moved_at":    time.Now().Unix(),
		},
	})
}

// ParseMessage 解析Redis Stream消息，将消息内容转换为VoucherOrder对象，便于后续处理
func ParseMessage(values map[string]interface{}) *VoucherOrder {
	if values == nil {
		return nil
	}
	vid, _ := strconv.ParseUint(strVal(values["voucher_id"]), 10, 64)
	uid, _ := strconv.ParseUint(strVal(values["user_id"]), 10, 64)
	oid, _ := strconv.ParseUint(strVal(values["order_id"]), 10, 64)

	if vid == 0 || uid == 0 || oid == 0 {
		return nil
	}
	return &VoucherOrder{
		ID:        oid,
		UserID:    uid,
		VoucherID: vid,
	}
}

// strVal 将interface{}类型的值转换为字符串，如果值为nil则返回空字符串
func strVal(v interface{}) string {
	if v == nil {
		return ""
	}
	vstr, ok := v.(string)
	if !ok {
		return ""
	}
	return vstr
}
