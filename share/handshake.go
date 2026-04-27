package share

import (
	"errors"
	"net"

	"TengShe/protocol"
	"TengShe/share/transport"
)

type ConnPrepareStage string

const (
	ConnPrepareStageTLS     ConnPrepareStage = "tls"
	ConnPrepareStagePreAuth ConnPrepareStage = "preauth"
)

type ConnPrepareError struct {
	Stage ConnPrepareStage
	Err   error
}

func (err *ConnPrepareError) Error() string {
	if err == nil || err.Err == nil {
		return ""
	}
	return err.Err.Error()
}

func (err *ConnPrepareError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Err
}

func IsConnPrepareStage(err error, stage ConnPrepareStage) bool {
	var prepareErr *ConnPrepareError
	if errors.As(err, &prepareErr) {
		return prepareErr.Stage == stage
	}
	return false
}

func PrepareActiveUpstreamConn(conn net.Conn, tlsEnable bool, domain string) (net.Conn, bool, error) {
	conn, tlsWrapped, err := prepareTLSClient(conn, tlsEnable, domain)
	if err != nil {
		return conn, tlsWrapped, err
	}

	negotiateActive(protocol.NewUpProto(&protocol.NegParam{Conn: conn, Domain: domain}))
	if err := ActivePreAuth(conn); err != nil {
		Debugf("active upstream preauth failed: %v", err)
		return conn, tlsWrapped, &ConnPrepareError{Stage: ConnPrepareStagePreAuth, Err: err}
	}

	return conn, tlsWrapped, nil
}

func PreparePassiveUpstreamConn(conn net.Conn, tlsEnable bool) (net.Conn, bool, error) {
	conn, tlsWrapped, err := PreparePassiveUpstreamTransport(conn, tlsEnable)
	if err != nil {
		return conn, tlsWrapped, err
	}

	if err := PassivePreAuth(conn); err != nil {
		Debugf("passive upstream preauth failed: %v", err)
		return conn, tlsWrapped, &ConnPrepareError{Stage: ConnPrepareStagePreAuth, Err: err}
	}

	return conn, tlsWrapped, nil
}

func PreparePassiveUpstreamTransport(conn net.Conn, tlsEnable bool) (net.Conn, bool, error) {
	conn, tlsWrapped, err := prepareTLSServer(conn, tlsEnable)
	if err != nil {
		return conn, tlsWrapped, err
	}

	negotiatePassive(protocol.NewUpProto(&protocol.NegParam{Conn: conn}))
	return conn, tlsWrapped, nil
}

func PrepareActiveDownstreamConn(conn net.Conn, tlsEnable bool, domain string) (net.Conn, bool, error) {
	conn, tlsWrapped, err := prepareTLSClient(conn, tlsEnable, domain)
	if err != nil {
		return conn, tlsWrapped, err
	}

	negotiateActive(protocol.NewDownProto(&protocol.NegParam{Conn: conn, Domain: domain}))
	if err := ActivePreAuth(conn); err != nil {
		Debugf("active downstream preauth failed: %v", err)
		return conn, tlsWrapped, &ConnPrepareError{Stage: ConnPrepareStagePreAuth, Err: err}
	}

	return conn, tlsWrapped, nil
}

func PreparePassiveDownstreamConn(conn net.Conn, tlsEnable bool) (net.Conn, bool, error) {
	conn, tlsWrapped, err := prepareTLSServer(conn, tlsEnable)
	if err != nil {
		return conn, tlsWrapped, err
	}

	negotiatePassive(protocol.NewDownProto(&protocol.NegParam{Conn: conn}))
	if err := PassivePreAuth(conn); err != nil {
		Debugf("passive downstream preauth failed: %v", err)
		return conn, tlsWrapped, &ConnPrepareError{Stage: ConnPrepareStagePreAuth, Err: err}
	}

	return conn, tlsWrapped, nil
}

func prepareTLSClient(conn net.Conn, tlsEnable bool, domain string) (net.Conn, bool, error) {
	if !tlsEnable {
		return conn, false, nil
	}

	tlsConfig, err := transport.NewClientTLSConfig(domain)
	if err != nil {
		Debugf("prepare tls client config failed: %v", err)
		return conn, false, &ConnPrepareError{Stage: ConnPrepareStageTLS, Err: err}
	}

	return transport.WrapTLSClientConn(conn, tlsConfig), true, nil
}

func prepareTLSServer(conn net.Conn, tlsEnable bool) (net.Conn, bool, error) {
	if !tlsEnable {
		return conn, false, nil
	}

	tlsConfig, err := transport.NewServerTLSConfig()
	if err != nil {
		Debugf("prepare tls server config failed: %v", err)
		return conn, false, &ConnPrepareError{Stage: ConnPrepareStageTLS, Err: err}
	}

	return transport.WrapTLSServerConn(conn, tlsConfig), true, nil
}

func negotiateActive(proto protocol.Proto) {
	if proto != nil {
		if err := proto.CNegotiate(); err != nil {
			Debugf("active protocol negotiate failed: %v", err)
		}
	}
}

func negotiatePassive(proto protocol.Proto) {
	if proto != nil {
		if err := proto.SNegotiate(); err != nil {
			Debugf("passive protocol negotiate failed: %v", err)
		}
	}
}
