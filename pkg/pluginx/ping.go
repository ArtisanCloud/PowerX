package pluginx

import "net/http"

func IsServerReady(url string) bool {
	res, err := http.Get(url)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	return res.StatusCode == http.StatusOK
}
