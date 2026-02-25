# cron组件
- 支持cron表达式
- 根据设定的时间规则定时执行函数

## Install

### Prerequisites
- GO >= 1.17

### Using go get
```
go get github.com/actorgo-game/components/cron@latest
```


## Quick Start
```
import ccron "github.com/actorgo-game/components/cron"
```


```
// 以组件方式注入到actorgo引擎
func Run(path, env, node string) {
    // 加载profile配置
    actorgo.Configure(path, env, node)
    // cron以组件方式注册到actorgo引擎
    ccron.RegisterComponent()
    // 启动actorgo引擎
    actorgo.Run(false, actorgo.Cluster)
}

// 手工方式启动cron
func main() {
    ccron.Init()

    for i := 0; i <= 23; i++ {
        ccron.AddEveryDayFunc(func() {
            now := actorgoTime.Now()
            actorgoLogger.Infof("每天第%d点%d分%d秒运行", now.Hour(), now.Minute(), now.Second())
        }, i, 12, 34)
        actorgoLogger.Infof("添加 每天第%d点执行的定时器", i)
    }

    for i := 0; i <= 59; i++ {
        ccron.AddEveryHourFunc(func() {
            actorgoLogger.Infof("每小时第%d分执行一次", actorgoTime.Now().Minute())
        }, i, 0)
        actorgoLogger.Infof("添加 每小时第%d分的定时器", i)
    }

    ccron.Run()
}

```

## example
- [示例代码跳转](cron_test.go)