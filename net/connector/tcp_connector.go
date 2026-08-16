package cconnector

import (
	cfacade "github.com/actorgo-game/actorgo/facade"
	clog "github.com/actorgo-game/actorgo/logger"
)

type (
	// TCPConnector accepts TCP streams and hands them to the parser component.
	TCPConnector struct {
		cfacade.Component
		*Connector
		Options
	}
)

func (*TCPConnector) Name() string {
	return "tcp_connector"
}

// OnStop closes the listener and stops connection dispatch.
func (t *TCPConnector) OnStop() {
	t.Connector.Stop()
}

// NewTCP creates a TCP connector for the supplied listen address.
func NewTCP(address string, opts ...Option) *TCPConnector {
	if address == "" {
		clog.Warn("Create tcp connector fail. Address is null.")
		return nil
	}

	tcp := &TCPConnector{
		Options: Options{
			address:  address,
			certFile: "",
			keyFile:  "",
			chanSize: 256,
		},
	}

	for _, opt := range opts {
		opt(&tcp.Options)
	}

	tcp.Connector = NewConnector(tcp.chanSize)

	return tcp
}

// Start listens and accepts connections until the connector is stopped.
func (t *TCPConnector) Start() {
	listener, err := t.GetListener(t.certFile, t.keyFile, t.address)
	if err != nil {
		clog.Fatal("failed to listen: %s", err)
	}

	clog.Info("Tcp connector listening at Address %s", t.address)
	if t.certFile != "" || t.keyFile != "" {
		clog.Info("certFile = %s, keyFile = %s", t.certFile, t.keyFile)
	}

	t.Connector.Start()

	for t.Running() {
		conn, err := listener.Accept()
		if err != nil {
			if !t.Running() {
				return
			}
			clog.Error("Failed to accept TCP connection: %s", err.Error())
			continue
		}

		t.InChan(conn)
	}
}
