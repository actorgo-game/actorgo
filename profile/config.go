package cprofile

import (
	"time"

	cfacade "github.com/actorgo-game/actorgo/facade"
	jsoniter "github.com/json-iterator/go"
)

type (
	Config struct {
		jsoniter.Any
	}
)

func Wrap(val any) *Config {
	return &Config{
		Any: jsoniter.Wrap(val),
	}
}

func (p *Config) GetConfig(path ...any) cfacade.ProfileJSON {
	return &Config{
		Any: p.Any.Get(path...),
	}
}

func (p *Config) GetString(path any, defaultVal ...string) string {
	result := p.Get(path)
	if result.LastError() != nil {
		if len(defaultVal) > 0 {
			return defaultVal[0]
		}
		return ""
	}
	return result.ToString()
}

func (p *Config) GetBool(path any, defaultVal ...bool) bool {
	result := p.Get(path)
	if result.LastError() != nil {
		if len(defaultVal) > 0 {
			return defaultVal[0]
		}

		return false
	}

	return result.ToBool()
}

func (p *Config) GetInt(path any, defaultVal ...int) int {
	result := p.Get(path)
	if result.LastError() != nil {
		if len(defaultVal) > 0 {
			return defaultVal[0]
		}
		return 0
	}

	return result.ToInt()
}

func (p *Config) GetInt32(path any, defaultVal ...int32) int32 {
	result := p.Get(path)
	if result.LastError() != nil {
		if len(defaultVal) > 0 {
			return defaultVal[0]
		}
		return 0
	}

	return result.ToInt32()
}

func (p *Config) GetInt64(path any, defaultVal ...int64) int64 {
	result := p.Get(path)
	if result.LastError() != nil {
		if len(defaultVal) > 0 {
			return defaultVal[0]
		}
		return 0
	}

	return result.ToInt64()
}

func (p *Config) GetDuration(path any, defaultVal ...time.Duration) time.Duration {
	result := p.Get(path)
	if result.LastError() != nil {
		if len(defaultVal) > 0 {
			return defaultVal[0]
		}
		return 0
	}

	return time.Duration(result.ToInt64())
}

func (p *Config) Unmarshal(value any) error {
	if p.LastError() != nil {
		return p.LastError()
	}
	return jsoniter.UnmarshalFromString(p.ToString(), value)
}
