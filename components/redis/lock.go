package credis

import (
	"context"
	"errors"
	"sync"
	"time"

	cnuid "github.com/actorgo-game/actorgo/extend/nuid"
	clog "github.com/actorgo-game/actorgo/logger"

	"github.com/go-redis/redis/v8"
)

var (
	// ErrLockNotObtained 在规定时间/重试次数内未能获取到锁时返回
	ErrLockNotObtained = errors.New("redis lock: lock not obtained")
	// ErrLockNotHeld 释放或续期时，发现锁已不属于当前持有者（已过期或被他人持有）
	ErrLockNotHeld = errors.New("redis lock: lock not held")
	// ErrClientNil redis client 为空（通常是 redis 组件未初始化）
	ErrClientNil = errors.New("redis lock: redis client is nil")
)

const (
	// 默认锁过期时间
	defaultLockTTL = 30 * time.Second
	// 默认阻塞获取锁时的重试间隔
	defaultRetryInterval = 100 * time.Millisecond
)

// 释放锁：只有 token 匹配时才删除，避免误删他人持有的锁
var unlockScript = redis.NewScript(`
if redis.call("get", KEYS[1]) == ARGV[1] then
	return redis.call("del", KEYS[1])
else
	return 0
end
`)

// 续期：只有 token 匹配时才重设过期时间
var renewScript = redis.NewScript(`
if redis.call("get", KEYS[1]) == ARGV[1] then
	return redis.call("pexpire", KEYS[1], ARGV[2])
else
	return 0
end
`)

type (
	// Lock 基于 Redis 实现的分布式锁。
	//
	// 通过 SET key token NX PX 获取锁，使用唯一 token 标识持有者，
	// 释放/续期均通过 Lua 脚本校验 token，保证只有持有者才能操作，避免误删。
	// 可选开启看门狗(watchdog)在持锁期间自动续期，防止业务未结束锁就过期。
	Lock struct {
		rdb           *redis.Client
		key           string
		ttl           time.Duration
		retryInterval time.Duration
		autoRenew     bool

		mu       sync.Mutex
		token    string        // 当前持有者标识，未持锁时为空
		stopCh   chan struct{} // 关闭以停止看门狗
		watching bool
	}

	// Option 配置分布式锁的可选项
	Option func(*Lock)
)

// WithTTL 设置锁的过期时间，默认 30s。
func WithTTL(ttl time.Duration) Option {
	return func(l *Lock) {
		if ttl > 0 {
			l.ttl = ttl
		}
	}
}

// WithRetryInterval 设置阻塞获取锁(Lock)时的重试间隔，默认 100ms。
func WithRetryInterval(interval time.Duration) Option {
	return func(l *Lock) {
		if interval > 0 {
			l.retryInterval = interval
		}
	}
}

// WithAutoRenew 开启看门狗，在成功获取锁后自动定时续期，
// 直到调用 Unlock 或获取锁时使用的 ctx 被取消。
func WithAutoRenew() Option {
	return func(l *Lock) {
		l.autoRenew = true
	}
}

// NewLock 使用指定的 redis client 创建一把分布式锁。
func NewLock(rdb *redis.Client, key string, opts ...Option) *Lock {
	l := &Lock{
		rdb:           rdb,
		key:           key,
		ttl:           defaultLockTTL,
		retryInterval: defaultRetryInterval,
	}

	for _, opt := range opts {
		opt(l)
	}

	return l
}

// NewLockWithInstance 使用全局 redis 组件实例创建分布式锁，
// 并自动拼接 redis 组件配置的 PrefixKey 前缀。
func NewLockWithInstance(key string, opts ...Option) *Lock {
	rdb := Instance()

	fullKey := key
	if instance != nil && instance.PrefixKey != "" {
		fullKey = instance.PrefixKey + key
	}

	return NewLock(rdb, fullKey, opts...)
}

// Key 返回锁对应的 redis key。
func (l *Lock) Key() string {
	return l.key
}

// TryLock 尝试获取一次锁，立即返回。
// 获取成功返回 nil，未获取到返回 ErrLockNotObtained。
func (l *Lock) TryLock(ctx context.Context) error {
	if l.rdb == nil {
		return ErrClientNil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	token := cnuid.Next()
	ok, err := l.rdb.SetNX(ctx, l.key, token, l.ttl).Result()
	if err != nil {
		return err
	}

	if !ok {
		return ErrLockNotObtained
	}

	l.token = token
	if l.autoRenew {
		l.startWatchdog(ctx)
	}

	return nil
}

// Lock 阻塞式获取锁，按 retryInterval 重试，直到获取成功或 ctx 结束。
// 若 ctx 超时/取消仍未获取到，返回 ctx.Err() 或 ErrLockNotObtained。
func (l *Lock) Lock(ctx context.Context) error {
	if l.rdb == nil {
		return ErrClientNil
	}

	ticker := time.NewTicker(l.retryInterval)
	defer ticker.Stop()

	for {
		err := l.TryLock(ctx)
		if err == nil {
			return nil
		}

		if !errors.Is(err, ErrLockNotObtained) {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// Unlock 释放锁，仅当当前 token 仍持有锁时才会删除。
// 若锁已过期或被他人持有，返回 ErrLockNotHeld。
func (l *Lock) Unlock(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.token == "" {
		return ErrLockNotHeld
	}

	l.stopWatchdog()

	res, err := unlockScript.Run(ctx, l.rdb, []string{l.key}, l.token).Result()
	l.token = ""
	if err != nil {
		return err
	}

	if n, ok := res.(int64); !ok || n == 0 {
		return ErrLockNotHeld
	}

	return nil
}

// Renew 手动续期，将锁的过期时间重设为 ttl。
// 仅当前 token 持有锁时有效，否则返回 ErrLockNotHeld。
func (l *Lock) Renew(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.renew(ctx)
}

func (l *Lock) renew(ctx context.Context) error {
	if l.token == "" {
		return ErrLockNotHeld
	}

	res, err := renewScript.Run(ctx, l.rdb, []string{l.key}, l.token, l.ttl.Milliseconds()).Result()
	if err != nil {
		return err
	}

	if n, ok := res.(int64); !ok || n == 0 {
		return ErrLockNotHeld
	}

	return nil
}

// startWatchdog 启动看门狗 goroutine，定时续期。调用方需持有 l.mu。
func (l *Lock) startWatchdog(ctx context.Context) {
	if l.watching {
		return
	}

	l.watching = true
	l.stopCh = make(chan struct{})

	stopCh := l.stopCh
	token := l.token
	// 在 ttl 的 1/3 处续期，留出足够的容错窗口
	interval := l.ttl / 3
	if interval <= 0 {
		interval = l.ttl
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-stopCh:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				l.mu.Lock()
				// 锁已被释放或已易主，停止续期
				if l.token != token {
					l.mu.Unlock()
					return
				}

				err := l.renew(ctx)
				l.mu.Unlock()

				if err != nil {
					clog.Warn("[redis lock] auto renew failed, key=%s err=%v", l.key, err)
					return
				}
			}
		}
	}()
}

// stopWatchdog 停止看门狗。调用方需持有 l.mu。
func (l *Lock) stopWatchdog() {
	if l.watching {
		close(l.stopCh)
		l.watching = false
	}
}

// WithLock 是一个便捷封装：获取锁 -> 执行 fn -> 释放锁。
// 获取锁失败时直接返回错误，不会执行 fn。
func WithLock(ctx context.Context, rdb *redis.Client, key string, fn func() error, opts ...Option) error {
	lock := NewLock(rdb, key, opts...)
	if err := lock.Lock(ctx); err != nil {
		return err
	}

	defer func() {
		if err := lock.Unlock(ctx); err != nil && !errors.Is(err, ErrLockNotHeld) {
			clog.Warn("[redis lock] unlock failed, key=%s err=%v", key, err)
		}
	}()

	return fn()
}
