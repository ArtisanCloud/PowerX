package httpx

import (
	"io"
	"net/http"
)

func HttpResponseSend(rs *http.Response, writer http.ResponseWriter) (err error) {

	// set write body
	body, err := io.ReadAll(rs.Body)
	if err != nil {
		return err
	}

	// set header code
	if rs.StatusCode > 0 {
		writer.WriteHeader(rs.StatusCode)
	}

	_, err = writer.Write(body)
	return err
}
