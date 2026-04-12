package cdiscovery

import (
	"fmt"
	"time"

	cstring "github.com/actorgo-game/actorgo/extend/string"
	cfacade "github.com/actorgo-game/actorgo/facade"
	clog "github.com/actorgo-game/actorgo/logger"
	cnats "github.com/actorgo-game/actorgo/net/nats"
	cproto "github.com/actorgo-game/actorgo/net/proto"
	cprofile "github.com/actorgo-game/actorgo/profile"

	"github.com/nats-io/nats.go"
)

// DiscoveryNats master节点模式(master为单节点)
// 先启动一个master节点
// 其他节点启动时Request(actorgo.discovery.register)，到master节点注册
// master节点subscribe(actorgo.discovery.register)，返回已注册节点列表
// master节点publish(actorgo.discovery.addMember)，当前已注册的节点到
// 所有客户端节点subscribe(actorgo.discovery.addMember)，接收新节点
// 所有节点subscribe(actorgo.discovery.unregister)，退出时注销节点
type DiscoveryNats struct {
	DiscoveryDefault
	app               cfacade.IApplication
	thisMember        cfacade.IMember
	thisMemberBytes   []byte
	masterMember      cfacade.IMember
	registerSubject   string
	unregisterSubject string
	addSubject        string
	checkSubject      string
}

var instanceNats *DiscoveryNats

func GetNats() *DiscoveryNats {
	return instanceNats
}

func NewDiscoveryNats() *DiscoveryNats {
	instanceNats = &DiscoveryNats{}
	return instanceNats
}

func (m *DiscoveryNats) Name() string {
	return "nats"
}

func (m *DiscoveryNats) isMaster() bool {
	return m.app.NodeID() == m.masterMember.GetNodeID()
}

func (m *DiscoveryNats) isClient() bool {
	return m.app.NodeID() != m.masterMember.GetNodeID()
}

func (m *DiscoveryNats) buildSubject(subject string) string {
	return fmt.Sprintf(subject, cprofile.Env(), m.masterMember.GetNodeID())
}

func (m *DiscoveryNats) Load(app cfacade.IApplication) {
	clog.Info("Load discovery [mode = %s].", m.Name())
	m.DiscoveryDefault.PreInit()
	m.app = app
	m.loadMember()
	m.init()
}

func (m *DiscoveryNats) loadMember() {
	m.thisMember = &cproto.Member{
		NodeID:   m.app.NodeID(),
		NodeType: m.app.NodeType(),
		Address:  m.app.RpcAddress(),
		Settings: make(map[string]string),
	}

	memberBytes, err := m.app.Serializer().Marshal(m.thisMember)
	if err != nil {
		clog.Warn("err = %s", err)
		return
	}

	m.thisMemberBytes = memberBytes

	//get nats config
	config := cprofile.GetConfig("cluster").GetConfig(m.Name())
	if config.LastError() != nil {
		clog.Error("nats config parameter not found. err = %v", config.LastError())
	}

	// get master node id
	masterId := config.GetString("master_node_id")
	if masterId == "" {
		clog.Error("master node id not in config.")
	}

	masterType := cstring.ToString(cfacade.GetNodeType(cstring.ToUint64D(masterId)))
	if masterType == "" {
		clog.Error("master node type not in config.")
	}
	clog.Info("master masterId[%s] masterType[%s]", masterId, masterType)

	// load master node config
	masterNode, err := cprofile.LoadNode(masterId, masterType)
	if err != nil {
		clog.Error(err.Error())
	}

	m.masterMember = &cproto.Member{
		NodeID:   masterNode.NodeID(),
		NodeType: masterNode.NodeType(),
		Address:  masterNode.RpcAddress(),
		Settings: make(map[string]string),
	}
}

func (m *DiscoveryNats) init() {
	m.registerSubject = m.buildSubject("actorgo.discovery.%s.%s.register")
	m.unregisterSubject = m.buildSubject("actorgo.discovery.%s.%s.unregister")
	m.addSubject = m.buildSubject("actorgo.discovery.%s.%s.addMember")
	m.checkSubject = m.buildSubject("actorgo.discovery.%s.%s.check")

	clog.Info("registerSubject[%v] unregisterSubject[%v] addSubject[%v] checkSubject[%v]",
		m.registerSubject, m.unregisterSubject, m.addSubject, m.checkSubject)

	m.subscribe(m.unregisterSubject, func(msg *nats.Msg) {
		unregisterMember := &cproto.Member{}
		err := m.app.Serializer().Unmarshal(msg.Data, unregisterMember)
		if err != nil {
			clog.Warn("err = %s", err)
			return
		}

		clog.Info("unregister Subject NodeID[%v]", unregisterMember.NodeID)
		if unregisterMember.NodeID == m.app.NodeID() {
			return
		}

		// remove member
		m.RemoveMember(unregisterMember.NodeID)
	})

	clog.Info("appNodeId[%v] appNodeType[%v] masterMember[%v] GetNodeId[%v]",
		m.app.NodeID(), m.app.NodeType(), m.masterMember, m.masterMember.GetNodeID())
	m.serverInit()
	m.clientInit()

	clog.Info("[discovery = %s] is running.", m.Name())
}

func (m *DiscoveryNats) serverInit() {
	if !m.isMaster() {
		return
	}

	//addMember master node
	clog.Info("mastermember[%v]", m.masterMember)
	m.AddMember(m.masterMember)

	// subscribe register message
	m.subscribe(m.registerSubject, func(msg *nats.Msg) {
		newMember := &cproto.Member{}
		err := m.app.Serializer().Unmarshal(msg.Data, newMember)
		if err != nil {
			clog.Warn("IMember Unmarshal[name = %s] error. dataLen = %+v, err = %s",
				m.app.Serializer().Name(),
				len(msg.Data),
				err,
			)
			return
		}

		// addMember new member
		clog.Info("newMember[%v] Subject[%v] Reply[%v]", newMember, msg.Subject, msg.Reply)
		m.AddMember(newMember)

		// response member list
		memberList := &cproto.MemberList{}

		m.memberMap.Range(func(key, value any) bool {
			protoMember := value.(*cproto.Member)
			if protoMember.NodeID != newMember.NodeID {
				memberList.List = append(memberList.List, protoMember)
			}

			return true
		})

		rspData, err := m.app.Serializer().Marshal(memberList)
		if err != nil {
			clog.Warn("marshal fail. err = %s", err)
			return
		}

		// response member list
		err = msg.Respond(rspData)
		if err != nil {
			clog.Warn("respond fail. err = %s", err)
			return
		}

		// publish addMember new node
		err = cnats.GetConnect().Publish(m.addSubject, msg.Data)
		if err != nil {
			clog.Warn("publish fail. err = %s", err)
			return
		}
	})

	// subscribe check message
	m.subscribe(m.checkSubject, func(msg *nats.Msg) {
		msg.Respond(nil)
	})
}

func (m *DiscoveryNats) clientInit() {
	if !m.isClient() {
		return
	}
	clog.Info("client init")

	// receive registered node
	m.subscribe(m.addSubject, func(msg *nats.Msg) {
		addMember := &cproto.Member{}
		err := m.app.Serializer().Unmarshal(msg.Data, addMember)
		if err != nil {
			clog.Warn("err = %s", err)
			return
		}

		clog.Info("Subject[%v] Reply[%v] addMember[%v] addSubject[%v]",
			msg.Subject, msg.Reply, addMember, m.addSubject)
		if _, ok := m.GetMember(addMember.NodeID); !ok {
			m.AddMember(addMember)
		}
	})

	go m.checkMaster()
}

func (m *DiscoveryNats) checkMaster() {
	for {
		_, found := m.GetMember(m.masterMember.GetNodeID())
		if !found {
			m.registerToMaster()
		}

		time.Sleep(cnats.ReconnectDelay())
	}
}

func (m *DiscoveryNats) registerToMaster() {
	// register current node to master
	rsp, err := cnats.GetConnect().Request(m.registerSubject, m.thisMemberBytes)
	if err != nil {
		clog.Warn("register node to [master = %s] fail. [address = %s] registerSubject[%v] err[%v] ",
			m.masterMember.GetNodeID(),
			cnats.GetConnect().Address(),
			m.registerSubject,
			err.Error(),
		)
		return
	}

	clog.Info("register node to [master = %s]. [member = %s]",
		m.masterMember,
		m.thisMember,
	)

	memberList := cproto.MemberList{}
	err = m.app.Serializer().Unmarshal(rsp, &memberList)
	if err != nil {
		clog.Warn("err = %s", err)
		return
	}

	clog.Info("memberList[%v]", memberList)
	for _, member := range memberList.GetList() {
		m.AddMember(member)
	}
}

func (m *DiscoveryNats) Stop() {
	err := cnats.GetConnect().Publish(m.unregisterSubject, m.thisMemberBytes)
	if err != nil {
		clog.Warn("publish fail. err = %s", err)
		return
	}

	clog.Debug("[nodeId = %s] unregister node to [master = %s]",
		m.app.NodeID(),
		m.masterMember.GetNodeID(),
	)
}

func (m *DiscoveryNats) subscribe(subject string, cb nats.MsgHandler) {
	err := cnats.GetConnect().Subscribe(subject, cb)
	if err != nil {
		clog.Warn("subscribe fail. err = %s subject[%v] Address[%v]", err, subject, cnats.GetConnect().Address())
		return
	}
}
