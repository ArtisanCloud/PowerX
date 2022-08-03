package logger

import (
	logger2 "github.com/ArtisanCloud/PowerLibs/v2/logger"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerX/config"
	UBT "github.com/ArtisanCloud/ubt-go"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
)

var zapLogger *zap.Logger
var UBTHandler *UBT.UBT
var Logger *logger2.Logger

func SetupLog() (err error) {

	//UBTHandler = UBT.Init(config2.UBTConfig)
	////fmt.Dump(config2.UBTConfig)
	//if UBTHandler == nil {
	//	return errors.New("init ubt error")
	//}

	strArtisanCloudPath := os.Getenv("log_path")
	strOutputPath := strArtisanCloudPath + "/output.log"
	strErrorPath := strArtisanCloudPath + "/errors.log"
	//fmt.Dump(strOutputPath)
	err = initLogPath(strArtisanCloudPath, strOutputPath, strErrorPath)
	if err != nil {
		panic(err)
	}

	Logger, err = logger2.NewLogger("", &object.HashMap{
		"env":        config.AppConfigure.Env,
		"outputPath": strOutputPath,
		"errorPath":  strErrorPath,
	})

	if err != nil {
		panic(err)
	}

	return err
}

func initLogPath(path string, files ...string) (err error) {
	if _, err = os.Stat(path); os.IsNotExist(err) {
		err = os.MkdirAll(path, os.ModePerm)
		if err != nil {
			return err
		}
	} else if os.IsPermission(err) {
		return err
	}

	for _, fileName := range files {
		if _, err = os.Stat(fileName); os.IsNotExist(err) {
			_, err = os.Create(fileName)
			if err != nil {
				return err
			}
		}
	}

	return err

}

func init() {
	config := zap.NewDevelopmentConfig()
	config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder

	zapLogger, _ = config.Build()

	//Logger.Info("123", zap.String("key", "value"))

}

func Info(msg string, fields map[string]interface{}) {

	zapFields := map2ZapFields(fields)

	zapLogger.Info(msg, zapFields...)

}

func Warn(msg string, fields map[string]interface{}) {

	zapFields := map2ZapFields(fields)

	zapLogger.Warn(msg, zapFields...)

}

func Error(msg string, fields map[string]interface{}) {

	zapFields := map2ZapFields(fields)

	zapLogger.Error(msg, zapFields...)

}

func Fatal(msg string, fields map[string]interface{}) {

	zapFields := map2ZapFields(fields)

	zapLogger.Fatal(msg, zapFields...)

}

func Debug(msg string, fields map[string]interface{}) {

	zapFields := map2ZapFields(fields)

	zapLogger.Debug(msg, zapFields...)

}

func Panic(msg string, fields map[string]interface{}) {

	zapFields := map2ZapFields(fields)

	zapLogger.Panic(msg, zapFields...)

}

func map2ZapFields(m map[string]interface{}) []zap.Field {
	fields := make([]zap.Field, 0, len(m))
	for k, v := range m {
		fields = append(fields, zap.Any(k, v))
	}
	return fields
}
