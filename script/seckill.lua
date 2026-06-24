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
