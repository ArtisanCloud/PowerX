package media

import (
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/gin-gonic/gin"
)

type ParaGetMedia struct {
	MediaID string `form:"mediaID" json:"mediaID" binding:"required"`
}

func ValidateGetMedia(context *gin.Context) {
	var form ParaGetMedia

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	context.Set("mediaID", form.MediaID)
	context.Next()
}
