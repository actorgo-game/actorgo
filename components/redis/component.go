package credis

import (
	"sync"

	cfacade "github.com/actorgo-game/actorgo/facade"
	clog "github.com/actorgo-game/actorgo/logger"
	cprofile "github.com/actorgo-game/actorgo/profile"

	"github.com/go-redis/redis/v8"
)

const (
	Name = "redis_component"
)

type (
	Component struct {
		cfacade.Component
		redisConfig
		rdb *redis.Client
	}

	redisConfig struct {
		Address      string `json:"address"`        // redis地址
		Password     string `json:"password"`       // 密码
		DB           int    `json:"db"`             // db index
		PrefixKey    string `json:"prefix_key"`     // 前缀
		SubscribeKey string `json:"subscribe_key"`  // 订阅key
		PoolSize     int    `json:"pool_size"`      // 最大活跃连接数
		MinIdleConns int    `json:"min_idle_conns"` // 最小空闲连接
	}
)

var instance *Component
var once sync.Once

func Instance() *redis.Client {
	if instance == nil {
		return nil
	}

	return instance.rdb
}

func NewComponent() *Component {
	once.Do(func() {
		instance = &Component{}
	})
	return instance
}

func (*Component) Name() string {
	return Name
}

func (s *Component) Init() {
	//read data_config->file node
	dataConfig := cprofile.GetConfig("data_config").GetConfig("redis")
	if dataConfig.Unmarshal(&s.redisConfig) != nil {
		clog.Warn("[data_config]->[%s] node in `%s` file not found.", s.Name(), cprofile.Name())
		return
	}
	clog.Info("start init redis redisConfig[%v]", s.redisConfig)

	s.rdb = redis.NewClient(&redis.Options{
		Addr:     s.Address,
		Password: s.Password,
		DB:       s.DB,

		// 连接池配置
		PoolSize:     s.PoolSize,     // 最大活跃连接数
		MinIdleConns: s.MinIdleConns, // 最小空闲连接
	})
	clog.Info("start init connect rdb[%v]", s.rdb)
}

func (s *Component) OnAfterInit() {
}

func (s *Component) OnStop() {
	if s.rdb != nil {
		s.rdb.Close()
	}
}
