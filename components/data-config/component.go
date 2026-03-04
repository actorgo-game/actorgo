package cdataconfig

import (
	"sync"

	cutils "github.com/actorgo-game/actorgo/extend/utils"
	cfacade "github.com/actorgo-game/actorgo/facade"
	clog "github.com/actorgo-game/actorgo/logger"
	cprofile "github.com/actorgo-game/actorgo/profile"
)

const (
	Name = "data_config_component"
)

type Component struct {
	sync.RWMutex
	cfacade.Component
	dataSource IDataSource
	parser     IDataParser
	configs    []IConfig
}

func New() *Component {
	return &Component{}
}

// Name unique components name
func (d *Component) Name() string {
	return Name
}

func (d *Component) Init() {
	// read data_config node in profile-{env}.json
	dataConfig := cprofile.GetConfig("data_config")
	if dataConfig.LastError() != nil {
		clog.Fatal("`data_config` node in `%s` file not found.", cprofile.Name())
	}

	// get data source
	sourceName := dataConfig.GetString("data_source")
	d.dataSource = GetDataSource(sourceName)
	if d.dataSource == nil {
		clog.Fatal("[sourceName = %s] data source not found.", sourceName)
	}

	// get parser
	parserName := dataConfig.GetString("parser")
	d.parser = GetParser(parserName)
	if d.parser == nil {
		clog.Fatal("[parserName = %s] parser not found.", parserName)
	}

	cutils.Try(func() {
		d.dataSource.Init(d)

		// init
		for _, cfg := range d.configs {
			cfg.Init()
		}

		// read register IConfig
		for _, cfg := range d.configs {
			data, found := d.GetBytes(cfg.Name())
			if !found {
				clog.Warn("[config = %s] load data fail.", cfg.Name())
				continue
			}

			cutils.Try(func() {
				d.onLoadConfig(cfg, data, false)
			}, func(errString string) {
				clog.Error("[config = %s] init config error. [error = %s]", cfg.Name(), errString)
			})
		}

		// on after load
		for _, cfg := range d.configs {
			cfg.OnAfterLoad(false)
		}

		// on change process
		d.dataSource.OnChange(func(configName string, data []byte) {
			iConfig := d.GetIConfig(configName)
			if iConfig != nil {
				d.onLoadConfig(iConfig, data, true)
				iConfig.OnAfterLoad(true)
			}
		})

	}, func(errString string) {
		clog.Error(errString)
	})
}

func (d *Component) onLoadConfig(cfg IConfig, data []byte, reload bool) {
	var parseObject any
	err := d.parser.Unmarshal(data, &parseObject)
	if err != nil {
		clog.Warn("[config = %s] unmarshal error = %v", err, cfg.Name())
		return
	}

	d.Lock()
	defer d.Unlock()

	// load data
	size, err := cfg.OnLoad(parseObject, reload)
	if err != nil {
		clog.Warn("[config = %s] execute Load() error = %s", cfg.Name(), err)
		return
	}

	clog.Info("[config = %s] loaded. [size = %d]", cfg.Name(), size)
}

func (d *Component) OnStop() {
	if d.dataSource != nil {
		d.dataSource.Stop()
	}
}

func (d *Component) Register(configs ...IConfig) {
	if len(configs) < 1 {
		clog.Warn("IConfig size is less than 1.")
		return
	}

	for _, cfg := range configs {
		if cfg != nil {
			d.configs = append(d.configs, cfg)
		}
	}
}

func (d *Component) GetIConfig(name string) IConfig {
	for _, cfg := range d.configs {
		if cfg.Name() == name {
			return cfg
		}
	}
	return nil
}

func (d *Component) GetBytes(configName string) (data []byte, found bool) {
	data, err := d.dataSource.ReadBytes(configName)
	if err != nil {
		clog.Warn(err.Error())
		return nil, false
	}

	return data, true
}

func (d *Component) GetParser() IDataParser {
	return d.parser
}

func (d *Component) GetDataSource() IDataSource {
	return d.dataSource
}
