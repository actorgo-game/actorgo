package cdiscovery

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	cfacade "github.com/actorgo-game/actorgo/facade"
	clog "github.com/actorgo-game/actorgo/logger"
	cproto "github.com/actorgo-game/actorgo/net/proto"
	cprofile "github.com/actorgo-game/actorgo/profile"

	jsoniter "github.com/json-iterator/go"
	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/namespace"
)

var (
	keyPrefix         = "/actorgo/node/"
	registerKeyFormat = keyPrefix + "%s/%s"
	envKeyFormat      = keyPrefix + "%s/"
)

// DiscoveryETCD etcd方式发现服务
type DiscoveryETCD struct {
	app cfacade.IApplication
	DiscoveryDefault
	prefix  string
	config  clientv3.Config
	ttl     int64
	cli     *clientv3.Client // etcdagent client
	leaseID clientv3.LeaseID // get lease id
}

var instanceETCD *DiscoveryETCD

func ETCDClient() *clientv3.Client {
	if instanceETCD == nil {
		return nil
	}
	return instanceETCD.cli
}

func GetETCD() *DiscoveryETCD {
	return instanceETCD
}

func NewDiscoveryETCD() *DiscoveryETCD {
	instanceETCD = &DiscoveryETCD{}
	return instanceETCD
}

func (p *DiscoveryETCD) Name() string {
	return "etcd"
}

func (p *DiscoveryETCD) Load(app cfacade.IApplication) {
	p.DiscoveryDefault.PreInit()
	p.app = app
	p.ttl = 10

	clusterConfig := cprofile.GetConfig("cluster").GetConfig(p.Name())
	if clusterConfig.LastError() != nil {
		clog.Error("[etcdagent] config not found. err = %v", clusterConfig.LastError())
		return
	}

	p.loadConfig(clusterConfig)

	p.init()
	//p.getLeaseId()
	p.watch()

	p.register()

	//after register,get all
	p.forceSync(false)

	clog.Info("[etcdagent] init complete! [endpoints = %v] [leaseId = %d]", p.config.Endpoints, p.leaseID)
}

func (p *DiscoveryETCD) Stop() {
	key := p.GetRisterKeyPrefix(p.app.NodeID())
	_, err := p.cli.Delete(context.Background(), key)
	clog.Info("[etcdagent] stopping! err = %v key[%v]", err, key)

	err = p.cli.Close()
	if err != nil {
		clog.Warn("[etcdagent] stopping error! err = %v", err)
	}
}

func getDialTimeout(config jsoniter.Any) time.Duration {
	t := time.Duration(config.Get("dial_timeout_second").ToInt64()) * time.Second
	if t < 1*time.Second {
		t = 3 * time.Second
	}

	return t
}

func getEndPoints(config jsoniter.Any) []string {
	return strings.Split(config.Get("end_points").ToString(), ",")
}

func (p *DiscoveryETCD) loadConfig(config cfacade.ProfileJSON) {
	p.config = clientv3.Config{
		Logger: nil,
	}

	p.config.Endpoints = getEndPoints(config)
	p.config.DialTimeout = getDialTimeout(config)
	p.config.Username = config.GetString("user")
	p.config.Password = config.GetString("password")
	p.ttl = config.GetInt64("ttl", 5)
	p.prefix = config.GetString("prefix", "actorgo")
	clog.Info("[etcdagent] load config %+v ttl[%v] prefix[%v]", p.config, p.ttl, p.prefix)
}

func (p *DiscoveryETCD) init() {
	var err error
	p.cli, err = clientv3.New(p.config)
	if err != nil {
		clog.Error("[etcdagent] connect fail. err = %v", err)
		return
	}
	clog.Info("[etcdagent] init complete! [endpoints = %v]", p.config.Endpoints)

	// set namespace
	p.cli.KV = namespace.NewKV(p.cli.KV, p.prefix)
	p.cli.Watcher = namespace.NewWatcher(p.cli.Watcher, p.prefix)
	p.cli.Lease = namespace.NewLease(p.cli.Lease, p.prefix)
}

func (p *DiscoveryETCD) getLeaseId() {
	var err error
	//设置租约时间
	resp, err := p.cli.Grant(context.Background(), p.ttl)
	if err != nil {
		clog.Error("[etcdagent] err[%v]", err.Error())
		return
	}

	p.leaseID = resp.ID

	//设置续租 定期发送需求请求
	keepaliveChan, err := p.cli.KeepAlive(context.Background(), resp.ID)
	if err != nil {
		clog.Error("[etcdagent] err[%v]", err.Error())
		return
	}

	go func() {
		for {
			select {
			case <-keepaliveChan:
				{
				}
			case die := <-p.app.DieChan():
				{
					if die {
						return
					}
				}
			}
		}
	}()
}

func (p *DiscoveryETCD) GetRisterKeyPrefix(nodeId string) string {
	return fmt.Sprintf(registerKeyFormat, cprofile.Env(), nodeId)
}

func (p *DiscoveryETCD) GetEnvKeyPrefix() string {
	return fmt.Sprintf(envKeyFormat, cprofile.Env())
}

func (p *DiscoveryETCD) register() {
	registerMember := &cproto.Member{
		NodeID:   p.app.NodeID(),
		NodeType: p.app.NodeType(),
		Address:  p.app.RpcAddress(),
		Settings: make(map[string]string),
	}
	registerMember.Settings["create_time"] = strconv.FormatInt(time.Now().Unix(), 10)

	jsonString, err := jsoniter.MarshalToString(registerMember)
	if err != nil {
		clog.Error("[etcdagent] err[%v]", err.Error())
		return
	}

	key := p.GetRisterKeyPrefix(p.app.NodeID())
	clog.Info("[etcdagent] key[%v] jsonString[%v] leaseID[%v]", key, jsonString, p.leaseID)
	//_, err = p.cli.Put(context.Background(), key, jsonString, clientv3.WithLease(p.leaseID))
	_, err = p.cli.Put(context.Background(), key, jsonString)
	if err != nil {
		clog.Error("[etcdagent] err[%v]", err.Error())
		return
	}
}

// get all
func (p *DiscoveryETCD) forceSync(fromWatch bool) {
	clog.Info("[etcdagent] sync for all nodes fromWatch[%v]", fromWatch)
	envKeyPrefix := p.GetEnvKeyPrefix()
	resp, err := p.cli.Get(context.Background(), envKeyPrefix, clientv3.WithPrefix())
	if err != nil {
		clog.Error("[etcdagent] Failed to get all nodes: %v", err)
		return
	}

	clog.Info("[etcdagent] Found[%d] nodes", len(resp.Kvs))
	for _, ev := range resp.Kvs {
		p.addMember(ev.Value)
	}
}

func (p *DiscoveryETCD) watch() {
	envKeyPrefix := p.GetEnvKeyPrefix()

	//1.get all
	p.forceSync(true)

	//2.watch
	watchChan := p.cli.Watch(context.Background(), envKeyPrefix, clientv3.WithPrefix())
	go func() {
		for rsp := range watchChan {
			for _, ev := range rsp.Events {
				switch ev.Type {
				case mvccpb.PUT:
					{
						p.addMember(ev.Kv.Value)
					}
				case mvccpb.DELETE:
					{
						p.removeMember(ev.Kv)
					}
				}
			}
		}
	}()
}

func (p *DiscoveryETCD) addMember(data []byte) {
	member := &cproto.Member{}
	err := jsoniter.Unmarshal(data, member)
	if err != nil {
		clog.Error("[etcdagent] err[%v]", err.Error())
		return
	}
	clog.Info("[etcdagent] add member %v", member)

	p.AddMember(member)
}

func (p *DiscoveryETCD) removeMember(kv *mvccpb.KeyValue) {
	key := string(kv.Key)
	envKeyPrefix := p.GetEnvKeyPrefix()
	nodeId := strings.ReplaceAll(key, envKeyPrefix, "")
	if nodeId == "" {
		clog.Warn("[etcdagent] remove member nodeId is empty!")
	}
	clog.Info("[etcdagent] remove member envKeyPrefix[%v] nodeId [%v]", envKeyPrefix, nodeId)

	p.RemoveMember(nodeId)
}
