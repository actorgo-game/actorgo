package cprofile

import (
	"path/filepath"
	"strings"

	cerror "github.com/actorgo-game/actorgo/error"
	cfile "github.com/actorgo-game/actorgo/extend/file"
	cjson "github.com/actorgo-game/actorgo/extend/json"
	cstring "github.com/actorgo-game/actorgo/extend/string"
	cfacade "github.com/actorgo-game/actorgo/facade"
)

var (
	cfg = &struct {
		profilePath    string  // profile root dir
		profileName    string  // profile name
		jsonConfig     *Config // profile-x.json parse to json object
		env            string  // env name
		debug          bool    // debug default is true
		printLevel     string  // log print level
		nodeId         string
		nodeType       string
		nodeIdStr      string
		bigWorldId     string // BigWorldId
		printLogPath   string
		configPath     string
		actorTimeOut   int64
		arrivalTimeOut int64
		nodeName       string
	}{}
)

func ArrivalTimeOut() int64 {
	return cfg.arrivalTimeOut
}

func Path() string {
	return cfg.profilePath
}

func Name() string {
	return cfg.profileName
}

func Env() string {
	return cfg.env
}

func Debug() bool {
	return cfg.debug
}

func PrintLevel() string {
	return cfg.printLevel
}

func Init(filePath, nodeIdStr string) (cfacade.INode, error) {
	if filePath == "" {
		return nil, cerror.Error("File path is nil.")
	}

	if nodeIdStr == "" {
		return nil, cerror.Error("NodeID is nil.")
	}

	judgePath, ok := cfile.JudgeFile(filePath)
	if !ok {
		return nil, cerror.Errorf("File path error. filePath = %s", filePath)
	}

	p, f := filepath.Split(judgePath)
	jsonConfig, err := LoadFile(p, f)
	if err != nil || jsonConfig.Any == nil || jsonConfig.LastError() != nil {
		return nil, cerror.Errorf("Load profile file error. [err = %v]", err)
	}

	nodeId, err := cfacade.GenNodeIdByStr(nodeIdStr)
	if err != nil {
		return nil, cerror.Errorf("Failed to generate node ID. [err = %v]", err)
	}

	nodeType := cfacade.GetNodeType(nodeId)

	node, err := GetNodeWithConfig(jsonConfig, cstring.ToString(nodeId), cstring.ToString(nodeType))
	if err != nil {
		return nil, cerror.Errorf("Failed to get node config from profile file. [err = %v]", err)
	}

	// init cfg
	cfg.profilePath = p
	cfg.profileName = f
	cfg.jsonConfig = jsonConfig
	cfg.env = jsonConfig.GetString("env", "default")
	cfg.debug = jsonConfig.GetBool("debug", true)
	cfg.printLevel = jsonConfig.GetString("print_level", "debug")
	cfg.printLogPath = jsonConfig.GetString("print_logpath", "./log/")
	cfg.arrivalTimeOut = jsonConfig.GetInt64("arrival_timeout", 100)
	cfg.configPath = jsonConfig.GetString("config_path", "./config/")
	cfg.nodeId = node.NodeID()
	cfg.nodeType = node.NodeType()
	cfg.actorTimeOut = node.Settings().GetInt64("actor_time_out", 0)
	cfg.nodeIdStr = nodeIdStr
	cfg.bigWorldId = strings.Split(nodeIdStr, ".")[0]
	cfg.nodeName = node.Settings().GetString("node_name", "default")

	return node, nil
}

func LoadNode(nodeId, nodeType string) (cfacade.INode, error) {
	return GetNodeWithConfig(cfg.jsonConfig, nodeId, nodeType)
}

func GetConfig(path ...any) cfacade.ProfileJSON {
	return cfg.jsonConfig.GetConfig(path...)
}

func JsonConfig() *Config {
	return cfg.jsonConfig
}

func ConfigPath() string {
	return cfg.configPath
}

func NodeName() string {
	return cfg.nodeName
}

func PrintLogPath() string {
	return cfg.printLogPath + "/" + cfg.nodeName + "." + cfg.nodeIdStr + "/"
}

func DiscoveryMode() string {
	//set default discovery mode to nats
	config := GetConfig("cluster").GetConfig("discovery")
	if config == nil {
		return "nats"
	}

	mode := config.GetString("mode")
	if mode == "" {
		return "nats"
	}

	return mode
}

func LoadFile(filePath, fileName string) (*Config, error) {
	var (
		profileMaps = make(map[string]any)
		includeMaps = make(map[string]any)
		rootMaps    = make(map[string]any)
	)

	// read profile json file
	fileNamePath := filepath.Join(filePath, fileName)
	if err := cjson.ReadMaps(fileNamePath, profileMaps); err != nil {
		return nil, err
	}

	// read include json file
	if v, found := profileMaps["include"].([]any); found {
		paths := cstring.ToStringSlice(v)
		for _, p := range paths {
			includePath := filepath.Join(filePath, p)
			if err := cjson.ReadMaps(includePath, includeMaps); err != nil {
				return nil, err
			}
		}
	}

	mergeMap(rootMaps, includeMaps)
	mergeMap(rootMaps, profileMaps)

	return Wrap(rootMaps), nil
}

func mergeMap(dst, src map[string]any) {
	for key, value := range src {
		if v, ok := dst[key]; ok {
			if m1, ok := v.(map[string]any); ok {
				if m2, ok := value.(map[string]any); ok {
					mergeMap(m1, m2)
				} else {
					dst[key] = value
				}
			} else {
				dst[key] = value
			}
		} else {
			dst[key] = value
		}
	}
}
