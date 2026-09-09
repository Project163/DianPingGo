local key = KEYS[1]
local ttl = tonumber(ARGV[1])
local refreshBelow = tonumber(ARGV[2])

-- 返回值为{状态， 用户ID， 原因， 维护操作错误}

local function corrupt(reason)
    local deleted = redis.pcall('DEL', key)
    local detail = ''
    if type(deleted) == 'table' and deleted.err then
        detail = deleted.err
    end
    return { 'corrupt', '', reason, detail }
end

-- TYPE区分不存在和类型损坏
local kind = redis.call('TYPE', key).ok
if kind == 'none' then
    return { 'missing', '', 'not_found', '' }
end
if kind ~= 'hash' then
    return corrupt('wrong_type')
end

-- 仅HGET id，不拉取完整userMap
local id = redis.call('HGET', key, 'id')
if not id then
    return corrupt('missing_id')
end

-- 校验ID长度、格式、uint64上限
if #id > 20 or not string.match(id, '^[1-9][0-9]*$')
    or (#id == 20 and id > '18446744073709551615') then
    return corrupt('invalid_id')
end

-- 检查TTL
local remaining = redis.call('PTTL', key)
if remaining == -1 then
    return corrupt('missing_ttl')
end
if remaining <= 0 then
    return { 'missing', '', 'expired', '' }
end

--
if remaining > refreshBelow then
    return { 'valid', id, 'ttl_sufficient', '' }
end

-- 仅有续期失败时会写维护操作错误，本失败允许放行但需要记录错误
local renewed = redis.pcall('PEXPIRE', key, ttl)
if type(renewed) == 'table' and renewed.err then
    return { 'valid', id, 'refresh_failed', renewed.err }
end
if renewed ~= 1 then
    return { 'missing', '', 'expired', '' }
end
return { 'valid', id, 'refreshed', '' }
