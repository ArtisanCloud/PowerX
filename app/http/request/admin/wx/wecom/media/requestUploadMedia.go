package media

import (
	"fmt"
	"github.com/ArtisanCloud/PowerWeChat/v2/src/kernel/power"
	"github.com/ArtisanCloud/PowerX/app/http"
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/ArtisanCloud/PowerX/configs/global"
	"github.com/gin-gonic/gin"
	"io/ioutil"
	"mime/multipart"
	"os"
	path2 "path"
)

type ParaUploadMedia struct {
	File *multipart.FileHeader `form:"file" json:"file" binding:"required"`
	Data string                `form:"data" json:"data"`
}

func ValidateUploadMedia(context *gin.Context) {
	var form ParaUploadMedia

	err := request.ValidatePara(context, &form)
	if err != nil {
		return
	}

	apiResponse := http.NewAPIResponse(context)

	path, data, err := convertParaUploadMediaForUpload(context, &form)
	if err != nil {
		apiResponse.SetCode(global.API_ERR_CODE_REQUEST_PARAM_ERROR, global.API_RETURN_CODE_ERROR, "", err.Error()).
			ThrowJSONResponse(context)
		return
	}

	context.Set("path", path)
	context.Set("data", data)
	context.Next()
}

func convertParaUploadMediaForUpload(context *gin.Context, form *ParaUploadMedia) (path string, data *power.HashMap, err error) {

	// FormFile returns the first file for the given key `myFile`
	// it also returns the FileHeader so we can get the Filename,
	// the Header and the size of the file
	file, handler, err := context.Request.FormFile("file")
	if err != nil {
		fmt.Println("Error Retrieving the File")
		fmt.Println(err)
		return
	}
	defer file.Close()
	//fmt.Printf("Uploaded File: %+v\n", handler.Filename)
	//fmt.Printf("File Size: %+v\n", handler.Size)
	//fmt.Printf("MIME Header: %+v\n", handler.Header)

	// Create a temporary file within our temp-images directory that follows
	// a particular naming pattern
	osTempPath := path2.Join(os.TempDir(), "")
	//fmt2.Dump(osTempPath)
	tempFile, err := ioutil.TempFile(osTempPath, "upload_*_"+handler.Filename)
	if err != nil {
		fmt.Println(err)
	}
	defer tempFile.Close()

	// read all of the contents of our uploaded file into a
	// byte array
	fileBytes, err := ioutil.ReadAll(file)
	if err != nil {
		fmt.Println(err)
	}
	// write this byte array to our temporary file
	tempFile.Write(fileBytes)
	// return that we have successfully uploaded our file!
	//fmt.Fprintf(w, "Successfully Uploaded File\n")

	path = tempFile.Name()

	return path, data, err
}
