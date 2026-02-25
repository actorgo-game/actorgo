# etcd组件
- 基于etcd实现发服务和节点集群

## Install

### Prerequisites
- GO >= 1.17

### Using go get
```
go get github.com/actorgo-game/components/etcd@latest
```


## Quick Start
```
import cetcd "github.com/actorgo-game/components/etcd"
```


```
// 注册etcd组件到discovery
func main() {
    cdiscovery.RegisterDiscovery(cetcd.New())
}

// 配置profile文件
// 设置"cluster"->"discovery"->"mode"为"etcd"模式
// 设置“cluster”->"etcd"节点相关的参数

{
    "cluster": {
        "discovery": {
            "mode": "etcd",
        },
        "nats": {
        },
        "etcd": {
            "end_points": "dev.com:2379",
            "@end_points": "dev.com:2379,dev1.com:2379",
            "prefix" : "actorgo",
            "ttl": 5,
            "dial_timeout": 3,
            "dial_keep_alive_time": 1,
            "dial_keep_alive_timeout": 1,
            "user": "",
            "password": ""
        }
    }
}

```

## example
- 示例代码待补充