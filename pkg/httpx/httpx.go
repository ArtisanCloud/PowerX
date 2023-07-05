package httpx

import (
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
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

func GetURL(baseURL string, port int, fullPath string) (string, error) {
	// 解析文件服务器根 URL
	baseURLParsed, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}

	// 判断主机名是否为空，如果为空则使用 "localhost"
	if baseURLParsed.Hostname() == "" {
		baseURLParsed.Host = "localhost"
		baseURLParsed.Path = ""
	}

	// 拼接主机名和端口号
	hostWithPort := baseURLParsed.Host
	if port != 0 {
		hostWithPort = baseURLParsed.Hostname() + ":" + strconv.Itoa(port)
	}

	// 去掉 fullPath 中的文件服务器根路径
	relativePath := strings.TrimPrefix(fullPath, baseURLParsed.Path)

	// 构建文件 URL
	scheme := "http://"
	if baseURLParsed.Scheme != "" {
		scheme = baseURLParsed.Scheme + "://"
	}
	fileURL := scheme + hostWithPort + filepath.Join(baseURLParsed.Path, relativePath)

	return fileURL, nil
}

func AppendURIs(baseURL string, uris ...string) (string, error) {
	// 解析基础 URL
	baseURLParsed, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}

	// 拼接所有的 URI
	for _, uri := range uris {
		// 去掉 URI 中的前导斜杠
		uri = strings.TrimPrefix(uri, "/")

		// 在基础 URL 的路径后面追加 URI
		baseURLParsed.Path = strings.TrimSuffix(baseURLParsed.Path, "/") + "/" + uri
	}
	// fixme , only return path
	combinedURL := strings.ReplaceAll(baseURLParsed.Path, `\`, `/`)
	//combinedURL := baseURLParsed.String()

	return combinedURL, nil
}
