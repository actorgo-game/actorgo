关键点：

获取锁：SET key token NX PX ttl，token 用框架自带的 cnuid.Next() 生成，保证唯一。
安全释放/续期：用 Lua 脚本先比对 token 再 del / pexpire，避免误删他人持有的锁。
看门狗自动续期：WithAutoRenew() 开启后，在 ttl/3 处自动续期，防止业务没跑完锁就过期；Unlock 或 ctx 取消时自动停止。
并发安全：用 sync.Mutex 保护 token 和看门狗状态。
提供的 API
NewLock(rdb, key, opts...) / NewLockWithInstance(key, opts...)（后者用全局 redis 组件实例，并自动拼接配置里的 PrefixKey）
TryLock(ctx)：尝试一次，立即返回
Lock(ctx)：阻塞重试直到成功或 ctx 结束
Unlock(ctx) / Renew(ctx)
WithLock(ctx, rdb, key, fn, opts...)：获取→执行→自动释放 的便捷封装
选项：WithTTL、WithRetryInterval、WithAutoRenew
使用示例

import (
    "context"
    "time"
    credis "github.com/actorgo-game/actorgo/components/redis"
)

// 方式一：手动控制
lock := credis.NewLockWithInstance("order:1001", credis.WithTTL(10*time.Second), credis.WithAutoRenew())
if err := lock.Lock(context.Background()); err != nil {
    return err
}
defer lock.Unlock(context.Background())
// ... 临界区业务 ...

// 方式二：便捷封装
err := credis.WithLock(ctx, credis.Instance(), "order:1001", func() error {
    // ... 临界区业务 ...
    return nil
}, credis.WithTTL(10*time.Second))

锁依赖 redis 组件已初始化（即 credis.NewComponent() 已注册并 Init）。