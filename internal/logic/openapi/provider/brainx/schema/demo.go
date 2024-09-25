package schema

type ResponseHelloWorld struct {
	Message string `json:"message"`
}

type ResponseEchoLongTime struct {
	Message string `json:"message"`
}
