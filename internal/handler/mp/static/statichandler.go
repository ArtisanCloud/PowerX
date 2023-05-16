package static

import (
	"github.com/zeromicro/go-zero/rest/httpx"
	"net/http"
	"strings"
)

type RequestFile struct {
	FileName string `path:"filename"`
}

func FileHandler(filepath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req RequestFile
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		//imagePath := filepath + "/" + req.FileName
		imagePath := strings.TrimLeft(r.RequestURI, "/")
		//fmt.Dump(r.RequestURI)
		//fmt.Dump(filepath, req.FileName, imagePath)
		http.ServeFile(w, r, imagePath)
	}
}
