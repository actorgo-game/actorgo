# data-config组件
- 自定义数据源
- 读取数据
- 热更新数据

## Install

### Prerequisites
- GO >= 1.17

### Using go get
```
go get github.com/actorgo-game/components/data-config@latest
```


## Quick Start
```
import cdataconfig "github.com/actorgo-game/components/data-config"
```

```
package demo
import (
	"github.com/actorgo-game/actorgo"
	cdataconfig "github.com/actorgo-game/components/data-config"
)

// RegisterComponent 注册struct到data-config
func RegisterComponent() {
	dataConfig := cdataconfig.NewComponent()
	dataConfig.Register(
		&DropList,
		&DropOne,
	)

	//data-config组件注册到actorgo引擎
	actorgo.RegisterComponent(dataConfig)
}

```

## example
- [示例代码跳转](https://github.com/actorgo-game/examples/tree/master/test_data_config)