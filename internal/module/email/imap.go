package email

import (
	"crypto/tls"
	"github.com/pkg/errors"
	"net"
	"net/smtp"
)

type SMTPConfig struct {
	Addr      string
	TLSConfig *tls.Config
	UserName  string
	Password  string
}

type SMTPServer struct {
	*smtp.Client
}

func (S SMTPServer) SendEmail() {
	//TODO implement me
	panic("implement me")
}

func (S SMTPServer) Close() {
	//TODO implement me
	panic("implement me")
}

func NewIMAPSendServer(config *SMTPConfig) (s SendServer, err error) {
	client, err := smtp.Dial(config.Addr)
	if err != nil {
		return nil, errors.Wrap(err, "dial imap server failed")
	}
	if config.TLSConfig != nil {
		err = client.StartTLS(config.TLSConfig)
		if err != nil {
			return nil, errors.Wrap(err, "dial imap server start tls failed")
		}
	}
	host, _, err := net.SplitHostPort(config.Addr)
	if err != nil {
		return nil, errors.Wrap(err, "invalid imap addr")
	}
	err = client.Auth(smtp.PlainAuth("", config.UserName, config.Password, host))
	if err != nil {
		return nil, errors.Wrap(err, "dial imap server use auth failed")
	}
	return &SMTPServer{
		Client: client,
	}, nil
}
