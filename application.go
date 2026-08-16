package actorgo

import (
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"

	cconst "github.com/actorgo-game/actorgo/const"
	ctime "github.com/actorgo-game/actorgo/extend/time"
	cutils "github.com/actorgo-game/actorgo/extend/utils"
	cfacade "github.com/actorgo-game/actorgo/facade"
	clog "github.com/actorgo-game/actorgo/logger"
	cactor "github.com/actorgo-game/actorgo/net/actor"
	cmethod "github.com/actorgo-game/actorgo/net/method"
	cserializer "github.com/actorgo-game/actorgo/net/serializer"
	cprofile "github.com/actorgo-game/actorgo/profile"
)

const (
	Cluster    NodeMode = 1 // 集群模式
	Standalone NodeMode = 2 // 单机模式
)

type (
	NodeMode byte

	// Application owns the node configuration and coordinates codecs,
	// components, Actor routing, discovery, and cluster lifecycle.
	Application struct {
		cfacade.INode
		nodeMode     NodeMode
		startTime    ctime.ActorGoTime     // application start time
		running      int32                 // is running
		dieChan      chan bool             // wait for end application
		onShutdownFn []func()              // on shutdown execute functions
		components   []cfacade.IComponent  // all components
		codecs       *cserializer.Registry // JSON/PB body codec allow-list
		discovery    cfacade.IDiscovery    // discovery component
		cluster      cfacade.ICluster      // cluster component
		actorSystem  *cactor.Component     // actor system
		methods      cfacade.IMethodTable  // automatically populated Actor method routes
	}
)

// NewApp create new application instance
// It resolves the node entry from the supplied profile before constructing it.
func NewApp(profileFilePath, nodeIDStr string, mode NodeMode) *Application {
	node, err := cprofile.Init(profileFilePath, nodeIDStr)
	if err != nil {
		panic(err)
	}

	return NewAppNode(node, mode)
}

// NewAppNode creates an application from an already resolved node definition.
func NewAppNode(node cfacade.INode, mode NodeMode) *Application {
	// set logger
	clog.SetNodeLogger(node)

	// print version info
	clog.Info(cconst.GetLOGO())

	protobufCodec := cserializer.NewProtobuf()
	jsonCodec := cserializer.NewJSON()
	app := &Application{
		INode:       node,
		codecs:      cserializer.NewRegistry(protobufCodec, jsonCodec),
		nodeMode:    mode,
		startTime:   ctime.Now(),
		running:     0,
		dieChan:     make(chan bool),
		actorSystem: cactor.New(),
	}
	app.methods = cmethod.NewTable(app)

	return app
}

func (a *Application) NodeMode() NodeMode {
	return a.nodeMode
}

func (a *Application) Running() bool {
	return a.running > 0
}

func (a *Application) DieChan() chan bool {
	return a.dieChan
}

func (a *Application) Register(components ...cfacade.IComponent) {
	if a.Running() {
		return
	}

	for _, c := range components {
		if c == nil || c.Name() == "" {
			clog.Error("[component = %T] name is nil", c)
			return
		}

		result := a.Find(c.Name())
		if result != nil {
			clog.Error("[component name = %s] is duplicate.", c.Name())
			return
		}

		a.components = append(a.components, c)
	}
}

func (a *Application) Find(name string) cfacade.IComponent {
	if name == "" {
		return nil
	}

	for _, component := range a.components {
		if component.Name() == name {
			return component
		}
	}
	return nil
}

// Remove component by name
func (a *Application) Remove(name string) cfacade.IComponent {
	if name == "" {
		return nil
	}

	var removeComponent cfacade.IComponent
	for i := 0; i < len(a.components); i++ {
		if a.components[i].Name() == name {
			removeComponent = a.components[i]
			a.components = append(a.components[:i], a.components[i+1:]...)
			i--
		}
	}
	return removeComponent
}

func (a *Application) All() []cfacade.IComponent {
	return a.components
}

func (a *Application) OnShutdown(fn ...func()) {
	a.onShutdownFn = append(a.onShutdownFn, fn...)
}

// Startup load components before startup
func (a *Application) Startup() {
	defer func() {
		if r := recover(); r != nil {
			clog.Error("%v", r)
		}
	}()

	if a.Running() {
		clog.Error("Application has running.")
		return
	}

	defer func() {
		clog.Flush()
	}()

	// register actor system
	// ActorSystem must initialize before network servers and create actors before listeners start.
	if a.Find(a.actorSystem.Name()) == nil {
		a.components = append([]cfacade.IComponent{a.actorSystem}, a.components...)
	}

	clog.Info("-------------------------------------------------")
	clog.Info("[nodeID      = %s] application is starting...", a.NodeID())
	clog.Info("[nodeType    = %s]", a.NodeType())
	clog.Info("[pid         = %d]", os.Getpid())
	clog.Info("[startTime   = %s]", a.StartTime())
	clog.Info("[profilePath = %s]", cprofile.Path())
	clog.Info("[profileName = %s]", cprofile.Name())
	clog.Info("[env         = %s]", cprofile.Env())
	clog.Info("[debug       = %v]", cprofile.Debug())
	clog.Info("[printLevel  = %s]", cprofile.PrintLevel())
	clog.Info("[logLevel    = %s]", clog.DefaultLogger.LogLevel)
	clog.Info("[stackLevel  = %s]", clog.DefaultLogger.StackLevel)
	clog.Info("[writeFile   = %v]", clog.DefaultLogger.EnableWriteFile)
	clog.Info("[bodyCodec  = %d]", a.codecs.Default())
	clog.Info("-------------------------------------------------")

	// component list
	for _, c := range a.components {
		c.Set(a)
		clog.Info("[component = %s] is added.", c.Name())
	}
	clog.Info("-------------------------------------------------")

	// execute Init()
	for _, c := range a.components {
		clog.Info("[component = %s] -> OnInit().", c.Name())
		c.Init()
	}
	clog.Info("-------------------------------------------------")

	// execute OnAfterInit()
	for _, c := range a.components {
		clog.Info("[component = %s] -> OnAfterInit().", c.Name())
		c.OnAfterInit()
	}

	clog.Info("-------------------------------------------------")
	clog.Info("[spend time = %dms] application is running.", a.startTime.NowDiffMillisecond())
	clog.Info("-------------------------------------------------")

	// set application is running
	atomic.AddInt32(&a.running, 1)

	sg := make(chan os.Signal, 1)
	signal.Notify(sg, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)

	select {
	case <-a.dieChan:
		clog.Info("invoke shutdown().")
	case s := <-sg:
		clog.Info("receive shutdown signal = %v.", s)
	}

	// stop status
	atomic.StoreInt32(&a.running, 0)

	clog.Info("------- application will shutdown -------")

	if a.onShutdownFn != nil {
		for _, f := range a.onShutdownFn {
			cutils.Try(func() {
				f()
			}, func(errString string) {
				clog.Warn("[onShutdownFn] error = %s", errString)
			})
		}
	}

	//all components in reverse order
	for i := len(a.components) - 1; i >= 0; i-- {
		cutils.Try(func() {
			clog.Info("[component = %s] -> OnBeforeStop().", a.components[i].Name())
			a.components[i].OnBeforeStop()
		}, func(errString string) {
			clog.Warn("[component = %s] -> OnBeforeStop(). error = %s", a.components[i].Name(), errString)
		})
	}

	for i := len(a.components) - 1; i >= 0; i-- {
		cutils.Try(func() {
			clog.Info("[component = %s] -> OnStop().", a.components[i].Name())
			a.components[i].OnStop()
		}, func(errString string) {
			clog.Warn("[component = %s] -> OnStop(). error = %s", a.components[i].Name(), errString)
		})
	}

	clog.Info("------- application has been shutdown... -------")
}

func (a *Application) Shutdown() {
	a.dieChan <- true
}

// BodyCodecs returns the shared allow-list used by AGP, HTTP, and cluster calls.
func (a *Application) BodyCodecs() cfacade.IBodyCodecRegistry {
	return a.codecs
}

// SetDefaultBodyCodec selects the default body codec before Startup.
// JSON and protobuf remain registered simultaneously.
func (a *Application) SetDefaultBodyCodec(id int32) {
	if a.Running() {
		return
	}
	if err := a.codecs.SetDefault(id); err != nil {
		clog.Warn("[SetDefaultBodyCodec] %v", err)
	}
}

func (a *Application) Discovery() cfacade.IDiscovery {
	return a.discovery
}

func (a *Application) Cluster() cfacade.ICluster {
	return a.cluster
}

func (a *Application) ActorSystem() cfacade.IActorSystem {
	return a.actorSystem
}

// Methods returns the MethodID-to-Actor route table populated during Actor initialization.
func (a *Application) Methods() cfacade.IMethodTable {
	return a.methods
}

func (a *Application) StartTime() string {
	return a.startTime.ToDateTimeFormat()
}

// SetDiscovery installs discovery before the application starts.
func (a *Application) SetDiscovery(discovery cfacade.IDiscovery) {
	if a.Running() || discovery == nil {
		return
	}

	a.discovery = discovery
}

// SetCluster installs the cross-node transport before the application starts.
func (a *Application) SetCluster(cluster cfacade.ICluster) {
	if a.Running() || cluster == nil {
		return
	}

	a.cluster = cluster
}
