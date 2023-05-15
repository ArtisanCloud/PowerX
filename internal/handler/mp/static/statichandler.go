package static

import (
	"github.com/zeromicro/go-zero/rest/httpx"
	"net/http"
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

		imagePath := filepath + "/" + req.FileName
		//fmt.Dump(imagePath)
		http.ServeFile(w, r, imagePath)
	}
}
