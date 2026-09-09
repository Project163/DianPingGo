package session

import (
	"context"
	"dianping/pkg/errmsg"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const DefaultTokenPrefix = "login:token:"
const DefaultTokenTTL = 30 * time.Minute

type Config struct {
	TokenPrefix     string
	TokenTTL        time.Duration
	RefreshInterval time.Duration
	RedisTimeout    time.Duration
}

func DefaultConfig() Config {
	return Config{DefaultTokenPrefix, DefaultTokenTTL, 5 * time.Minute, 200 * time.Millisecond}
}

// 区分鉴权结果和维护失败
type Result struct {
	UserID         uint64
	Outcome        string
	Reason         string
	MaintenanceErr error
}

type Store struct {
	rdb redis.Cmdable
	cfg Config
}

// go:embed authenticate.lua
var authenticateLua string
var authenticateScript = redis.NewScript(authenticateLua)

func NewStore(rdb redis.Cmdable, cfg Config) (*Store, error) {
	if rdb == nil || cfg.TokenPrefix == "" || cfg.TokenTTL.Milliseconds() <= 0 ||
		cfg.RefreshInterval.Milliseconds() <= 0 || cfg.RefreshInterval >= cfg.TokenTTL ||
		cfg.RedisTimeout <= 0 {
		return nil, errmsg.NewError(errmsg.ErrInternalSec, fmt.Errorf("invalid session configuration"))
	}
	return &Store{rdb: rdb, cfg: cfg}, nil
}

func (s *Store) Authenticate(ctx context.Context, token string) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{Outcome: "request_stopped"}, err
	}
	opCtx, cancel := context.WithTimeout(ctx, s.cfg.RedisTimeout)
	defer cancel()

	cmd := authenticateScript.Run(opCtx, s.rdb, []string{s.cfg.TokenPrefix + token},
		s.cfg.TokenTTL.Milliseconds(), (s.cfg.TokenTTL - s.cfg.RefreshInterval).Milliseconds())
	if err := cmd.Err(); err != nil {
		// 父请求已取消时不记为Redis故障
		if parentErr := ctx.Err(); parentErr != nil {
			return Result{Outcome: "request_stopped"}, parentErr
		}
		base := errmsg.ErrDependencyUnavailable
		outcome := "dependency_error"
		var serverErr redis.Error
		if errors.As(err, &serverErr) && !temporaryRedisError(err) {
			base, outcome = errmsg.ErrInternalSec, "internal_error"
		}
		return Result{Outcome: outcome, Reason: "redis_command"}, errmsg.NewError(base, err)
	}

	values, err := cmd.StringSlice()
	if err != nil || len(values) != 4 {
		return Result{Outcome: "internal_error", Reason: "invalid_reply"}, fmt.Errorf("invalid auth script reply: %w", errors.Join(err, fmt.Errorf("expected four strings")))
	}
	result := Result{Outcome: values[0], Reason: values[2]}
	if values[3] != "" {
		result.MaintenanceErr = fmt.Errorf("session maintenance: %s", values[3])
	}
	// 结果仅三种：鉴权未通过，内部服务错误（脚本执行中出错），鉴权通过
	switch result.Outcome {
	case "missing":
		return result, &errmsg.ErrUnauthorized
	case "corrupt":
		return result, errmsg.NewError(errmsg.ErrInternalSec, fmt.Errorf("session corrupted: %s", result.Reason))
	case "valid":
		// 鉴权通过但Golang的解析和Lua的解析出现冲突结果时直接修改结果返回错误
		id, parseErr := strconv.ParseUint(values[1], 10, 64)
		if parseErr != nil || id == 0 {
			result.Outcome, result.Reason = "internal_error", "invalid_script_id"
			return result, errmsg.NewError(errmsg.ErrInternalSec, fmt.Errorf("auth script returned invalid user ID"))
		}
		result.UserID = id
		return result, nil
	default:
		return Result{Outcome: "internal_error", Reason: "unknown_status"},
			errmsg.NewError(errmsg.ErrInternalSec, fmt.Errorf("unknown auth script status"))
	}
}

func temporaryRedisError(err error) bool {
	for _, prefix := range []string{"LOADING", "BUSY", "TRYAGAIN", "CLUSTERDOWN", "MASTERDOWN", "READONLY", "OOM", "MISCONF"} {
		if redis.HasErrorPrefix(err, prefix) {
			return true
		}
	}
	return false
}
