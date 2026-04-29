package handler

import (
	"io/ioutil"

	"TengShe/admin/manager"
	"TengShe/admin/printer"
	tsruntime "TengShe/internal/runtime"
	"TengShe/protocol"
	"TengShe/utils"
)

const (
	UPMETHOD = iota
	CERMETHOD
)

type SSH struct {
	Method          int
	Addr            string
	Username        string
	Password        string
	CertificatePath string
	Certificate     []byte
}

func NewSSH(addr string) *SSH {
	ssh := new(SSH)
	ssh.Addr = addr
	return ssh
}

func (ssh *SSH) LetSSH(route string, uuid string) error {
	_, _, err := utils.CheckIPPort(ssh.Addr)
	if err != nil {
		return err
	}

	if ssh.Method == CERMETHOD {
		if err := ssh.getCertificate(); err != nil {
			return err
		}
	}

	sMessage := tsruntime.NewDownstreamMessage(uuid, route)

	header := &protocol.Header{
		Sender:      protocol.ADMIN_UUID,
		Accepter:    uuid,
		MessageType: protocol.SSHREQ,
		RouteLen:    uint32(len([]byte(route))),
		Route:       route,
	}

	sshReqMess := &protocol.SSHReq{
		Method:         uint16(ssh.Method),
		AddrLen:        uint16(len(ssh.Addr)),
		Addr:           ssh.Addr,
		UsernameLen:    uint64(len(ssh.Username)),
		Username:       ssh.Username,
		PasswordLen:    uint64(len(ssh.Password)),
		Password:       ssh.Password,
		CertificateLen: uint64(len(ssh.Certificate)),
		Certificate:    ssh.Certificate,
	}

	protocol.ConstructMessage(sMessage, header, sshReqMess, false)
	sMessage.SendMessage()

	return nil
}

func (ssh *SSH) getCertificate() (err error) {
	ssh.Certificate, err = ioutil.ReadFile(ssh.CertificatePath)
	if err != nil {
		return
	}
	return
}

func DispatchSSHMess(mgr *manager.Manager) {
	for {
		message := <-mgr.SSHManager.SSHMessChan

		switch mess := message.(type) {
		case *protocol.SSHRes:
			if mess.OK == 1 {
				mgr.ConsoleManager.OK <- true
			} else {
				mgr.ConsoleManager.OK <- false
			}
		case *protocol.SSHResult:
			printer.Print("%s", mess.Result)
		case *protocol.SSHExit:
			mgr.ConsoleManager.Exit <- true
		}
	}
}
