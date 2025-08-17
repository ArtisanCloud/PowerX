package fmt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
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
		//fmt.Print("[nil]\r\n")

	} else if reflect.TypeOf(data).Kind() != reflect.String {
		prettyJson, err = PrettyJson(data)

	} else {
		strData = data.(string)
		prettyJson, err = PrettyJson(strData)
	}

	if err != nil {
		fmt.Printf("convert pretty fmt error:%v \r\n", err)
	}
	fmt.Printf("%+v \r\n", prettyJson)
}

func PrintSlice(s []int) {
	fmt.Printf("len=%d cap=%d %v\n", len(s), cap(s), s)
}
