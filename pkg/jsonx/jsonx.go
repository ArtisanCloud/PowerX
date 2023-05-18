package jsonx

import (
	"encoding/json"
	"io"
	"io/fs"
	"os"
)

func JsonEncode(v interface{}) (string, error) {
	buffer, err := json.Marshal(v)

	if err != nil {
		return "", err
	}
	return string(buffer), nil
}

func JsonDecode(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

func JsonEscape(str string) (string, error) {
	b, err := json.Marshal(str)
	if err != nil {
		return "", err
	}
	return string(b[1 : len(b)-1]), err
}

func LoadObjectFromFile(jsonPath string, obj interface{}) (err error) {

	// open json file
	jsonFile, err := os.Open(jsonPath)

	defer jsonFile.Close()
	if err != nil {
		return err
	}

	// parse file to buffer
	byteValue, _ := io.ReadAll(jsonFile)

	// parse buffer to object
	err = json.Unmarshal(byteValue, obj)

	return err
}

func SaveObjectToFile(obj interface{}, filePath string, perm fs.FileMode) (err error) {
	buff, err := json.MarshalIndent(obj, "", " ")
	if err != nil {
		return err
	}

	file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.Write(buff)
	return err
}
