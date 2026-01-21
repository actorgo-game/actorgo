package simple

import (
	cfacade "github.com/actorgo-game/actorgo/facade"
	clog "github.com/actorgo-game/actorgo/logger"
	cactor "github.com/actorgo-game/actorgo/net/actor"
	cproto "github.com/actorgo-game/actorgo/net/proto"
)

type ActorBase struct {
	cactor.Base
}

func (p *ActorBase) Response(session *cproto.Session, mid uint32, v interface{}) {
	Response(p, session, mid, v)
}

func Response(iActor cfacade.IActor, session *cproto.Session, mid uint32, v interface{}) {
	data, err := iActor.App().Serializer().Marshal(v)
	if err != nil {
		clog.Warn("[Response] Marshal error. v = %+v", v)
		return
	}

	rsp := &cproto.PomeloResponse{
		Sid:  session.Sid,
		Mid:  mid,
		Data: data,
	}

	iActor.Call(session.AgentPath, ResponseFuncName, rsp)
}
