package zapConfig

import (
	"github.com/natefinch/lumberjack"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var SugarLogger *zap.SugaredLogger

type zapLogger struct {
	sugarLogger *zap.SugaredLogger
}

func NewLogger(logNum int) *zapLogger { //创建一个日志对象sugarLogger
	//zapcore.Core需要三个配置—— Encoder，WriteSyncer，LogLevel。
	var logFile string
	switch logNum {
	case 1:
		logFile = "./log/node1.log"
	case 2:
		logFile = "./log/node2.log"
	case 3:
		logFile = "./log/node3.log"
	case 4:
		logFile = "./log/node4.log"

	}

	writeSyncer := getLogWriter(logFile)                              //WriterSyncer ：指定日志将写到哪里去。
	encoder := getEncoder()                                           //Encoder:编码器(如何写入日志)。
	core := zapcore.NewCore(encoder, writeSyncer, zapcore.DebugLevel) //Log Level：哪种级别的日志将被写入。

	logger := zap.New(core, zap.AddCaller()) //添加zap.AddCaller()可以让日志在记录时保存:文件名/行号/函数名

	zl := new(zapLogger)
	zl.sugarLogger = logger.Sugar()

	return zl

}

// 配置编码器
func getEncoder() zapcore.Encoder {
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder   //修改时间编码器(时间格式为ISO8601)
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder //在日志文件中使用大写字母记录日志级别
	return zapcore.NewConsoleEncoder(encoderConfig)         //如果需要使用JSON Encoder,可以改用NewJSONEncoder()而不是NewConsoleEncoder()
}

// 配置日志如何写出(也就是日志的记录方式)
func getLogWriter(logFile string) zapcore.WriteSyncer {
	lumberJackLogger := &lumberjack.Logger{ //使用Lumberjack进行日志切割,Zap本身不支持切割归档日志文件
		Filename:   logFile, //日志文件的位置
		MaxSize:    1,       //在进行切割之前，日志文件的最大大小（以MB为单位）
		MaxBackups: 5,       //保留旧日志文件的最大个数
		MaxAge:     30,      //保留旧文件的最大天数
		Compress:   false,   //是否压缩/归档旧文件
	}
	return zapcore.AddSync(lumberJackLogger)
}

func (zl *zapLogger) GetSugarLogger() *zap.SugaredLogger {
	return zl.sugarLogger
}

func (zl *zapLogger) Close() {
	zl.sugarLogger.Sync()
}
