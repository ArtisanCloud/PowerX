package email

type SendServer interface {
	SendEmail()
	Close()
}
