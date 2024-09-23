package schema

type AccessToken struct {
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type ResponseAuthToken struct {
	Platform string      `json:"platform"`
	Token    AccessToken `json:"token"`
}
