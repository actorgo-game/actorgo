package cfacade

import "time"

type (
	// IActorSystem creates Actors and routes request/notify calls. Invoke and
	// Notify resolve a top-level target by MethodID; the Target variants address
	// an explicit Actor path, including a dynamic child.
	IActorSystem interface {
		GetIActor(id string) (IActor, bool)
		CreateActor(id string, handler IActorHandler) (IActor, error)
		Invoke(ctx *RequestContext, methodID uint32, payload any) *InvokeResult
		InvokeNode(ctx *RequestContext, nodeID string, methodID uint32, payload any) *InvokeResult
		InvokeTarget(ctx *RequestContext, target string, methodID uint32, payload any) *InvokeResult
		Notify(ctx *RequestContext, methodID uint32, payload any) *InvokeResult
		NotifyNode(ctx *RequestContext, nodeID string, methodID uint32, payload any) *InvokeResult
		NotifyTarget(ctx *RequestContext, target string, methodID uint32, payload any) *InvokeResult
		SetCallTimeout(d time.Duration)
		SetExecutionTimeout(t int64)
	}

	// IActor is the business-facing handle for one serialized Actor instance.
	IActor interface {
		App() IApplication
		ActorID() string
		Path() *ActorPath
		LastAt() int64
		Invoke(ctx *RequestContext, methodID uint32, payload any) *InvokeResult
		Notify(ctx *RequestContext, methodID uint32, payload any) *InvokeResult
		Exit()
	}

	// IActorHandler defines an Actor's identity and lifecycle hooks.
	IActorHandler interface {
		AliasID() string                       // actorID
		OnInit()                               // 当Actor启动前触发该函数
		OnStop()                               // 当Actor停止前触发该函数
		OnFindChild(m *Message) (IActor, bool) // 当actor查找子Actor时触发该函数
	}

	// IActorChild manages the dynamic children owned by one parent Actor.
	IActorChild interface {
		Create(id string, handler IActorHandler) (IActor, error) // 创建子Actor
		Get(id string) (IActor, bool)                            // 获取子Actor
		Remove(id string)                                        // 称除子Actor
		Each(fn func(i IActor))                                  // 遍历所有子Actor
	}
)

// IEventData identifies an event and optionally scopes delivery by UniqueID.
type IEventData interface {
	Name() string    // 事件名
	UniqueID() int64 // 唯一id
}
