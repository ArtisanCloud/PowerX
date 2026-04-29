package fmt

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"reflect"

	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

const (
	empty = ""
	tab   = "\t"
)

func PrettyJson(data interface{}) (string, error) {
	buffer := new(bytes.Buffer)
	encoder := json.NewEncoder(buffer)
	encoder.SetIndent(empty, tab)

	err := encoder.Encode(data)
	if err != nil {
		return empty, err
	}
	return buffer.String(), nil
}

func DD(datas ...interface{}) {
	Dump(datas)
	os.Exit(0)
}

func Dump(datas ...interface{}) {
	for _, data := range datas {
		dump(data)
	}
}

func dump(data interface{}) {
	var (
		prettyJson interface{}
		strData    string
		err        error
	)
	if data == nil {
	} else if reflect.TypeOf(data).Kind() != reflect.String {
		prettyJson, err = PrettyJson(data)

	} else {
		strData = data.(string)
		prettyJson, err = PrettyJson(strData)
	}

	if err != nil {
		logger.ErrorF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "legacy"}), "convert pretty fmt error:%v", err)
	}
	logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "legacy"}), "%+v", prettyJson)
}

func PrintSlice(s []int) {
	logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "legacy"}), "len=%d cap=%d %v", len(s), cap(s), s)
}
